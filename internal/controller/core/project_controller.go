package core

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/controller-utils/pkg/logging"

	pwv1alpha1 "github.com/openmcp-project/platform-service-project-workspace/api/core/v1alpha1"
	"github.com/openmcp-project/platform-service-project-workspace/internal/utils"
)

const ProjectControllerName = "project"

// ProjectReconciler reconciles a Project object
type ProjectReconciler struct {
	OnboardingStatic *clusters.Cluster
	Scheme           *runtime.Scheme
	*CommonReconciler
}

func NewProjectReconciler(scheme *runtime.Scheme, cr *CommonReconciler) (*ProjectReconciler, error) {
	pr := &ProjectReconciler{
		Scheme:           scheme,
		CommonReconciler: cr,
	}

	onboardingClusterStatic, err := cr.Config.OnboardingClusterStatic(context.Background())
	if err != nil {
		return nil, err
	}
	pr.OnboardingStatic = onboardingClusterStatic

	return pr, nil
}

// +kubebuilder:rbac:groups=core.openmcp.cloud,resources=projects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.openmcp.cloud,resources=projects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.openmcp.cloud,resources=projects/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;rolebindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logging.FromContextOrPanic(ctx).WithName(ProjectControllerName)
	ctx = logging.NewContext(ctx, log)
	log.Info("Reconcile started")
	rr, err := r.reconcile(ctx, req)
	if rr.RequeueAfter > 0 {
		log.Debug("Requeuing request", "requeueAfter", rr.RequeueAfter, "nextReconciliationTime", time.Now().Add(rr.RequeueAfter))
	}
	return rr, err
}

func (r *ProjectReconciler) reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logging.FromContextOrPanic(ctx)

	project := &pwv1alpha1.Project{}
	project.SetName(req.Name)
	if project.GroupVersionKind().Kind == "" {
		project.SetGroupVersionKind(pwv1alpha1.GroupVersion.WithKind("Project"))
	}
	sr := r.sr.For(project)
	if err := r.OnboardingStatic.Client().Get(ctx, req.NamespacedName, project); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Project not found")
			return sr.StopRequeue()
		}
		return sr.ReturnError(fmt.Errorf("error fetching project: %w", err))
	}

	projectNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.NamespaceForProject(project),
		},
	}

	// Check if there are remaining resources in the namespace that are blocking the deletion of the project
	// If the project is not it deletion, this will return false
	hasRemainingContent, err := r.handleRemainingContentBeforeDelete(ctx, project)
	if err != nil {
		return sr.ReturnError(err)
	}
	if hasRemainingContent {
		if err := r.OnboardingStatic.Client().Status().Update(ctx, project); err != nil {
			log.Error(err, "failed to update status")
		}

		return sr.IsStable() // naming is unintuitive, this requeues with increasing backoff
	}

	deleted, rqt, err := r.handleDelete(ctx, project, func() error {
		if err := r.OnboardingStatic.Client().Delete(ctx, projectNamespace); err != nil {
			return client.IgnoreNotFound(err)
		}

		return ResourcesRemainingError{}
	})
	if deleted || err != nil {
		switch rqt {
		case RequeueError:
			return sr.ReturnError(err)
		case RequeueWithMinInterval:
			return sr.IsProgressing()
		case RequeueWithBackoff:
			return sr.IsStable()
		default:
			return sr.StopRequeue()
		}
	}

	if err := r.ensureFinalizer(ctx, project); err != nil {
		return sr.ReturnError(err)
	}

	// Always update status
	defer func() {
		if err := r.OnboardingStatic.Client().Status().Update(ctx, project); err != nil {
			log.Error(err, "failed to update status")
		}
	}()

	//
	// Namespace Creation
	//

	result, err := controllerutil.CreateOrUpdate(ctx, r.OnboardingStatic.Client(), projectNamespace, func() error {
		utils.SetProjectLabel(projectNamespace, project.Name)
		r.applyManagementLabel(projectNamespace)
		return nil
	})
	if err != nil {
		return sr.ReturnError(err)
	}
	utils.LogOperationResult(log, logging.INFO, projectNamespace, result)

	project.Status.Namespace = projectNamespace.Name

	//
	// Role bindings
	//

	if err := r.createOrUpdateClusterRole(ctx, project); err != nil {
		return sr.ReturnError(err)
	}
	if err := r.createOrUpdateRoleBinding(ctx, project, pwv1alpha1.ProjectRoleAdmin); err != nil {
		return sr.ReturnError(err)
	}
	if err := r.createOrUpdateRoleBinding(ctx, project, pwv1alpha1.ProjectRoleView); err != nil {
		return sr.ReturnError(err)
	}

	return sr.StopRequeue()
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pwv1alpha1.Project{}).
		Complete(r)
}

func (r *ProjectReconciler) createOrUpdateRoleBinding(ctx context.Context, project *pwv1alpha1.Project, role pwv1alpha1.ProjectMemberRole) error {
	log := logging.FromContextOrPanic(ctx)

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.RoleBindingForRole(role),
			Namespace: project.Status.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.OnboardingStatic.Client(), roleBinding, func() error {
		r.applyManagementLabel(roleBinding)

		roleBinding.Subjects = getSubjectsForProjectRole(project, role)
		roleBinding.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     utils.ClusterRoleForRole(role),
		}

		return controllerutil.SetOwnerReference(project, roleBinding, r.Scheme)
	})
	utils.LogOperationResult(log, logging.INFO, roleBinding, result)
	return err
}

func getSubjectsForProjectRole(project *pwv1alpha1.Project, role pwv1alpha1.ProjectMemberRole) []rbacv1.Subject {
	subjects := []rbacv1.Subject{}

	for _, member := range project.Spec.Members {
		if hasProjectRole(member, role) {
			subjects = append(subjects, member.RbacV1())
		}
	}

	return subjects
}

func hasProjectRole(member pwv1alpha1.ProjectMember, role pwv1alpha1.ProjectMemberRole) bool {
	for _, memberRole := range member.Roles {
		if memberRole == role {
			return true
		}
	}

	return false
}

func (r *ProjectReconciler) createOrUpdateClusterRole(ctx context.Context, project *pwv1alpha1.Project) error {
	log := logging.FromContextOrPanic(ctx)

	projectRoles := map[pwv1alpha1.ProjectMemberRole][]string{
		pwv1alpha1.ProjectRoleAdmin: utils.AllVerbs(),
		pwv1alpha1.ProjectRoleView:  utils.ReadOnlyVerbs(),
	}

	for role, verbs := range projectRoles {
		clusterRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: utils.ClusterRoleForEntityAndRole(project, role),
			},
		}

		result, err := controllerutil.CreateOrUpdate(ctx, r.OnboardingStatic.Client(), clusterRole, func() error {
			r.applyManagementLabel(clusterRole)

			clusterRole.Rules = []rbacv1.PolicyRule{
				{
					APIGroups:     []string{pwv1alpha1.GroupVersion.Group},
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
		utils.LogOperationResult(log, logging.INFO, clusterRole, result)

		clusterRoleBinding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: utils.ClusterRoleForEntityAndRole(project, role),
			},
		}

		result, err = controllerutil.CreateOrUpdate(ctx, r.OnboardingStatic.Client(), clusterRoleBinding, func() error {
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
		utils.LogOperationResult(log, logging.INFO, clusterRoleBinding, result)
	}

	return nil
}
