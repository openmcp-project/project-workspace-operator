package core

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

// ProjectReconciler reconciles a Project object
type ProjectReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	CommonReconciler
}

//+kubebuilder:rbac:groups=core.openmcp.cloud,resources=projects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core.openmcp.cloud,resources=projects/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core.openmcp.cloud,resources=projects/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=namespaces;secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;rolebindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	project := &v1alpha1.Project{}
	if err := r.Get(ctx, req.NamespacedName, project); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Project not found")
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Project")
		return ctrl.Result{}, err
	}

	projectNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceForProject(project),
		},
	}

	// Check if there are remaining resources in the namespace that are blocking the deletion of the project
	// If the project is not it deletion, this will return false
	hasRemainingContent, err := r.handleRemainingContentBeforeDelete(ctx, project)
	if err != nil {
		return ctrl.Result{}, err
	}
	if hasRemainingContent {
		if err := r.Status().Update(ctx, project); err != nil {
			log.Error(err, "failed to update status")
		}

		return ctrl.Result{
			RequeueAfter: 3 * time.Second,
		}, nil
	}

	deleted, dresult, err := r.handleDelete(ctx, project, func() error {
		if err := r.Delete(ctx, projectNamespace); err != nil {
			return client.IgnoreNotFound(err)
		}

		return ResourcesRemainingError{RequeueAfter: 3 * time.Second}
	})
	if deleted || err != nil {
		return dresult, err
	}

	if err := r.ensureFinalizer(ctx, project); err != nil {
		return ctrl.Result{}, err
	}

	// Always update status
	defer func() {
		if err := r.Status().Update(ctx, project); err != nil {
			log.Error(err, "failed to update status")
		}
	}()

	//
	// Namespace Creation
	//

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, projectNamespace, func() error {
		setProjectLabel(projectNamespace, project.Name)
		r.applyManagementLabel(projectNamespace)
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	logOperationResult(log, projectNamespace, result)

	project.Status.Namespace = projectNamespace.Name

	//
	// Role bindings
	//

	if err := r.createOrUpdateClusterRole(ctx, project); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.createOrUpdateRoleBinding(ctx, project, v1alpha1.ProjectRoleAdmin); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.createOrUpdateRoleBinding(ctx, project, v1alpha1.ProjectRoleView); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Project{}).
		Complete(r)
}

func (r *ProjectReconciler) createOrUpdateRoleBinding(ctx context.Context, project *v1alpha1.Project, role v1alpha1.ProjectMemberRole) error {
	log := log.FromContext(ctx)
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingForRole(role),
			Namespace: project.Status.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, roleBinding, func() error {
		r.applyManagementLabel(roleBinding)

		roleBinding.Subjects = getSubjectsForProjectRole(project, role)
		roleBinding.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     clusterRoleForRole(role),
		}

		return controllerutil.SetOwnerReference(project, roleBinding, r.Scheme)
	})
	logOperationResult(log, roleBinding, result)
	return err
}

func getSubjectsForProjectRole(project *v1alpha1.Project, role v1alpha1.ProjectMemberRole) []rbacv1.Subject {
	subjects := []rbacv1.Subject{}

	for _, member := range project.Spec.Members {
		if hasProjectRole(member, role) {
			subjects = append(subjects, member.RbacV1())
		}
	}

	return subjects
}

func hasProjectRole(member v1alpha1.ProjectMember, role v1alpha1.ProjectMemberRole) bool {
	for _, memberRole := range member.Roles {
		if memberRole == role {
			return true
		}
	}

	return false
}

func (r *ProjectReconciler) createOrUpdateClusterRole(ctx context.Context, project *v1alpha1.Project) error {
	log := log.FromContext(ctx)

	projectRoles := map[v1alpha1.ProjectMemberRole][]string{
		v1alpha1.ProjectRoleAdmin: AllVerbs,
		v1alpha1.ProjectRoleView:  ReadOnlyVerbs,
	}

	for role, verbs := range projectRoles {
		clusterRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterRoleForEntityAndRole(project, role),
			},
		}

		result, err := controllerutil.CreateOrUpdate(ctx, r.Client, clusterRole, func() error {
			r.applyManagementLabel(clusterRole)

			clusterRole.Rules = []rbacv1.PolicyRule{
				{
					APIGroups:     []string{v1alpha1.GroupVersion.Group},
					Resources:     []string{"projects"},
					ResourceNames: []string{project.Name},
					Verbs:         verbs,
				},
				{
					APIGroups:     []string{""},
					Resources:     []string{"namespaces"},
					ResourceNames: []string{project.Status.Namespace},
					Verbs:         []string{"get"},
				},
			}

			// Delete ClusterRole automatically when Project is deleted.
			return controllerutil.SetOwnerReference(project, clusterRole, r.Scheme)
		})
		if err != nil {
			return err
		}
		logOperationResult(log, clusterRole, result)

		clusterRoleBinding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterRoleForEntityAndRole(project, role),
			},
		}

		result, err = controllerutil.CreateOrUpdate(ctx, r.Client, clusterRoleBinding, func() error {
			r.applyManagementLabel(clusterRoleBinding)

			clusterRoleBinding.Subjects = getSubjectsForProjectRole(project, role)
			clusterRoleBinding.RoleRef = rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     clusterRole.Name,
			}

			// Delete ClusterRoleBinding automatically when Project is deleted.
			return controllerutil.SetOwnerReference(project, clusterRoleBinding, r.Scheme)
		})
		if err != nil {
			return err
		}
		logOperationResult(log, clusterRoleBinding, result)
	}

	return nil
}
