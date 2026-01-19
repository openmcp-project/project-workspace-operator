package core

import (
	"context"
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openmcp-project/controller-utils/pkg/clusters"

	"github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

var (
	ErrNamespaceHasNoLabels       = errors.New("namespace has no labels. Map is nil")
	ErrNamespaceHasNoProjectLabel = errors.New("namespace has no project label")
)

// WorkspaceReconciler reconciles a Workspace object
type WorkspaceReconciler struct {
	OnboardingStatic *clusters.Cluster
	Scheme           *runtime.Scheme
	*CommonReconciler
}

func NewWorkspaceReconciler(scheme *runtime.Scheme, cr *CommonReconciler) (*WorkspaceReconciler, error) {
	wr := &WorkspaceReconciler{
		Scheme:           scheme,
		CommonReconciler: cr,
	}

	onboardingClusterStatic, err := cr.Config.OnboardingClusterStatic(context.Background())
	if err != nil {
		return nil, err
	}
	wr.OnboardingStatic = onboardingClusterStatic

	return wr, nil
}

// +kubebuilder:rbac:groups=core.openmcp.cloud,resources=workspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.openmcp.cloud,resources=workspaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.openmcp.cloud,resources=workspaces/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	workspace := &v1alpha1.Workspace{}
	if err := r.OnboardingStatic.Client().Get(ctx, req.NamespacedName, workspace); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Workspace not found")
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Workspace")
		return ctrl.Result{}, err
	}

	project, err := r.getProjectByNamespace(ctx, workspace.Namespace)
	if err != nil {
		log.Error(err, "unable to fetch Project of Workspace")
		return ctrl.Result{}, err
	}

	workspaceNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceForWorkspace(workspace),
		},
	}

	// Check if there are remaining resources in the namespace that are blocking the deletion of the Workspace
	// If the workspace is not it deletion, this will return false
	hasRemainingContent, err := r.handleRemainingContentBeforeDelete(ctx, workspace)
	if err != nil {
		return ctrl.Result{}, err
	}
	if hasRemainingContent {
		if err := r.OnboardingStatic.Client().Status().Update(ctx, workspace); err != nil {
			log.Error(err, "failed to update status")
		}

		return ctrl.Result{
			RequeueAfter: 3 * time.Second,
		}, nil
	}

	deleted, dresult, err := r.handleDelete(ctx, workspace, func() error {
		if err := r.OnboardingStatic.Client().Delete(ctx, workspaceNamespace); err != nil {
			return client.IgnoreNotFound(err)
		}
		if err := r.deleteClusterRole(ctx, project, workspace); err != nil {
			return err
		}

		return ResourcesRemainingError{RequeueAfter: 3 * time.Second}
	})
	if deleted || err != nil {
		return dresult, err
	}

	if err := r.ensureFinalizer(ctx, workspace); err != nil {
		return ctrl.Result{}, err
	}

	// Always update status
	defer func() {
		if err := r.OnboardingStatic.Client().Status().Update(ctx, workspace); err != nil {
			log.Error(err, "failed to update status")
		}
	}()

	//
	// Namespace Creation
	//

	result, err := controllerutil.CreateOrUpdate(ctx, r.OnboardingStatic.Client(), workspaceNamespace, func() error {
		setWorkspaceLabel(workspaceNamespace, workspace.Name)
		setProjectLabel(workspaceNamespace, project.Name)
		r.applyManagementLabel(workspaceNamespace)
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	logOperationResult(log, workspaceNamespace, result)

	workspace.Status.Namespace = workspaceNamespace.Name

	//
	// Role bindings
	//

	if err := r.createOrUpdateClusterRole(ctx, project, workspace); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.createOrUpdateRoleBinding(ctx, workspace, v1alpha1.WorkspaceRoleAdmin); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.createOrUpdateRoleBinding(ctx, workspace, v1alpha1.WorkspaceRoleView); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *WorkspaceReconciler) getProjectByNamespace(ctx context.Context, namespaceName string) (*v1alpha1.Project, error) {
	namespace := &corev1.Namespace{}
	if err := r.OnboardingStatic.Client().Get(ctx, types.NamespacedName{Name: namespaceName}, namespace); err != nil {
		return nil, err
	}

	if namespace.Labels == nil {
		return nil, ErrNamespaceHasNoLabels
	}

	projectName := namespace.Labels[labelProject]
	if projectName == "" {
		return nil, ErrNamespaceHasNoProjectLabel
	}

	project := &v1alpha1.Project{}
	if err := r.OnboardingStatic.Client().Get(ctx, types.NamespacedName{Name: projectName}, project); err != nil {
		return nil, err
	}

	return project, nil
}

func (r *WorkspaceReconciler) createOrUpdateRoleBinding(ctx context.Context, workspace *v1alpha1.Workspace, workspaceRole v1alpha1.WorkspaceMemberRole) error {
	log := log.FromContext(ctx)
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingForRole(workspaceRole),
			Namespace: workspace.Status.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.OnboardingStatic.Client(), roleBinding, func() error {
		r.applyManagementLabel(roleBinding)

		roleBinding.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     clusterRoleForRole(workspaceRole),
		}

		roleBinding.Subjects = getSubjectsForWorkspaceRole(workspace, workspaceRole)
		return nil
	})
	logOperationResult(log, roleBinding, result)
	return err
}

// createOrUpdateClusterRole manages the ClusterRole and ClusterRoleBinding granting GET permissions to the namespace belonging to the workspace.
func (r *WorkspaceReconciler) createOrUpdateClusterRole(ctx context.Context, project *v1alpha1.Project, ws *v1alpha1.Workspace) error {
	log := log.FromContext(ctx)

	workspaceRoles := []v1alpha1.WorkspaceMemberRole{
		v1alpha1.WorkspaceRoleAdmin,
		v1alpha1.WorkspaceRoleView,
	}

	for _, role := range workspaceRoles {
		clusterRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterRoleForEntityAndRoleWithParent(ws, role, project),
			},
		}

		result, err := controllerutil.CreateOrUpdate(ctx, r.OnboardingStatic.Client(), clusterRole, func() error {
			r.applyManagementLabel(clusterRole)

			clusterRole.Rules = []rbacv1.PolicyRule{
				{
					APIGroups:     []string{""},
					Resources:     []string{"namespaces"},
					ResourceNames: []string{ws.Status.Namespace},
					Verbs:         []string{"get"},
				},
			}

			return nil
		})
		if err != nil {
			return err
		}
		logOperationResult(log, clusterRole, result)

		clusterRoleBinding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterRoleForEntityAndRoleWithParent(ws, role, project),
			},
		}

		result, err = controllerutil.CreateOrUpdate(ctx, r.OnboardingStatic.Client(), clusterRoleBinding, func() error {
			r.applyManagementLabel(clusterRoleBinding)

			clusterRoleBinding.Subjects = getSubjectsForWorkspaceRole(ws, role)
			clusterRoleBinding.RoleRef = rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     clusterRole.Name,
			}

			return nil
		})
		if err != nil {
			return err
		}
		logOperationResult(log, clusterRoleBinding, result)
	}

	return nil
}

// deleteClusterRole deletes the ClusterRole and ClusterRoleBinding that were created for the Workspace.
// It has to be done explicitly because cross-namespace OwnerReferences are not allowed.
func (r *WorkspaceReconciler) deleteClusterRole(ctx context.Context, project *v1alpha1.Project, ws *v1alpha1.Workspace) error {
	log := log.FromContext(ctx)

	workspaceRoles := []v1alpha1.WorkspaceMemberRole{
		v1alpha1.WorkspaceRoleAdmin,
		v1alpha1.WorkspaceRoleView,
	}

	for _, role := range workspaceRoles {
		clusterRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterRoleForEntityAndRoleWithParent(ws, role, project),
			},
		}

		if err := r.OnboardingStatic.Client().Delete(ctx, clusterRole); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			log.Info("Deleted ClusterRole", "name", clusterRole.Name)
		}

		clusterRoleBinding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterRoleForEntityAndRoleWithParent(ws, role, project),
			},
		}

		if err := r.OnboardingStatic.Client().Delete(ctx, clusterRoleBinding); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			log.Info("Deleted ClusterRoleBinding", "name", clusterRoleBinding.Name)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Workspace{}).
		Complete(r)
}

func getSubjectsForWorkspaceRole(workspace *v1alpha1.Workspace, role v1alpha1.WorkspaceMemberRole) []rbacv1.Subject {
	subjects := []rbacv1.Subject{}

	for _, member := range workspace.Spec.Members {
		if hasWorkspaceRole(member, role) {
			subjects = append(subjects, member.RbacV1())
		}
	}

	return subjects
}

func hasWorkspaceRole(member v1alpha1.WorkspaceMember, role v1alpha1.WorkspaceMemberRole) bool {
	for _, memberRole := range member.Roles {
		if memberRole == role {
			return true
		}
	}

	return false
}
