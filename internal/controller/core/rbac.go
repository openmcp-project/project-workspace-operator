package core

import (
	"context"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

var (
	AllVerbs = []string{
		"get",
		"list",
		"watch",
		"create",
		"update",
		"patch",
		"delete",
	}
	ReadOnlyVerbs = []string{
		"get",
		"list",
		"watch",
	}
)

func NewRBACSetup(setupLog logr.Logger, c client.Client, controllerName string, cfg pwv1alpha1.ProjectWorkspaceConfigSpec) *RBACSetup {
	return &RBACSetup{
		log:            setupLog,
		client:         c,
		controllerName: controllerName,
		config:         cfg,
	}
}

type RBACSetup struct {
	log            logr.Logger
	client         client.Client
	controllerName string
	config         pwv1alpha1.ProjectWorkspaceConfigSpec
}

func (setup *RBACSetup) EnsureResources(ctx context.Context) error {
	if err := setup.createOrUpdateProjectClusterRoles(ctx); err != nil {
		return err
	}

	if err := setup.createOrUpdateWorkspaceClusterRoles(ctx); err != nil {
		return err
	}

	return nil
}

func (setup *RBACSetup) createOrUpdateProjectClusterRoles(ctx context.Context) error {
	projectRoles := map[pwv1alpha1.ProjectMemberRole][]string{
		pwv1alpha1.ProjectRoleAdmin: AllVerbs,
		pwv1alpha1.ProjectRoleView:  ReadOnlyVerbs,
	}

	for role, verbs := range projectRoles {
		clusterRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterRoleForRole(role),
			},
		}

		result, err := controllerutil.CreateOrUpdate(ctx, setup.client, clusterRole, func() error {
			setManagementLabel(clusterRole, setup.controllerName)

			clusterRole.Rules = []rbacv1.PolicyRule{
				{
					APIGroups: []string{pwv1alpha1.GroupVersion.Group},
					Resources: []string{"workspaces"},
					Verbs:     verbs,
				},
				{
					APIGroups: []string{corev1.GroupName},
					Resources: []string{"serviceaccounts"},
					Verbs:     verbs,
				},
				{
					APIGroups: []string{corev1.GroupName}, // this rule prevents k9s from crashing
					Resources: []string{"pods"},
					Verbs:     []string{"list"},
				},
				{
					APIGroups: []string{corev1.GroupName},
					Resources: []string{"resourcequotas"},
					Verbs:     ReadOnlyVerbs,
				},
			}

			if role == pwv1alpha1.ProjectRoleAdmin {
				clusterRole.Rules = append(clusterRole.Rules, rbacv1.PolicyRule{
					APIGroups: []string{corev1.GroupName},
					Resources: []string{"serviceaccounts/token"},
					Verbs:     []string{"create"},
				})
			}

			// add roles from config, if defined
			clusterRole.Rules = append(clusterRole.Rules, setup.config.Project.AdditionalPermissions[role]...)

			return nil
		})
		if err != nil {
			return err
		}
		logOperationResult(setup.log, clusterRole, result)
	}

	return nil
}

func (setup *RBACSetup) createOrUpdateWorkspaceClusterRoles(ctx context.Context) error {
	workspaceRoles := map[pwv1alpha1.WorkspaceMemberRole][]string{
		pwv1alpha1.WorkspaceRoleAdmin: AllVerbs,
		pwv1alpha1.WorkspaceRoleView:  ReadOnlyVerbs,
	}

	for role, verbs := range workspaceRoles {
		clusterRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterRoleForRole(role),
			},
		}

		result, err := controllerutil.CreateOrUpdate(ctx, setup.client, clusterRole, func() error {
			setManagementLabel(clusterRole, setup.controllerName)

			clusterRole.Rules = []rbacv1.PolicyRule{
				{
					APIGroups: []string{pwv1alpha1.GroupVersion.Group},
					Resources: []string{"managedcontrolplanes", "clusteradmins"},
					Verbs:     verbs,
				},
				{
					APIGroups: []string{corev1.GroupName},
					Resources: []string{
						"secrets",
						"configmaps",
						"serviceaccounts",
					},
					Verbs: verbs,
				},
				{
					APIGroups: []string{corev1.GroupName}, // this rule prevents k9s from crashing
					Resources: []string{"pods"},
					Verbs:     []string{"list"},
				},
				{
					APIGroups: []string{corev1.GroupName},
					Resources: []string{"resourcequotas"},
					Verbs:     ReadOnlyVerbs,
				},
			}

			if role == pwv1alpha1.WorkspaceRoleAdmin {
				clusterRole.Rules = append(clusterRole.Rules, rbacv1.PolicyRule{
					APIGroups: []string{corev1.GroupName},
					Resources: []string{"serviceaccounts/token"},
					Verbs:     []string{"create"},
				})
			}

			// add roles from config, if defined
			clusterRole.Rules = append(clusterRole.Rules, setup.config.Workspace.AdditionalPermissions[role]...)

			return nil
		})
		if err != nil {
			return err
		}
		logOperationResult(setup.log, clusterRole, result)
	}

	return nil
}
