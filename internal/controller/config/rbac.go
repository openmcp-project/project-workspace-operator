package config

import (
	"context"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openmcp-project/controller-utils/pkg/logging"

	"github.com/openmcp-project/project-workspace-operator/internal/utils"
)

func NewRBACSetup(onboardingClient client.Client, providerName string) *RBACSetup {
	return &RBACSetup{
		client:       onboardingClient,
		providerName: providerName,
	}
}

type RBACSetup struct {
	client       client.Client
	providerName string
}

func (setup *RBACSetup) EnsureResources(ctx context.Context, projectPermissionsForRoleGenerator, workspacePermissionsForRoleGenerator func(string) ([]rbacv1.PolicyRule, error)) error {
	if err := setup.createOrUpdateProjectClusterRoles(ctx, projectPermissionsForRoleGenerator); err != nil {
		return err
	}

	if err := setup.createOrUpdateWorkspaceClusterRoles(ctx, workspacePermissionsForRoleGenerator); err != nil {
		return err
	}

	return nil
}

func (setup *RBACSetup) createOrUpdateProjectClusterRoles(ctx context.Context, projectPermissionsForRoleGenerator func(string) ([]rbacv1.PolicyRule, error)) error {
	log := logging.FromContextOrDiscard(ctx)

	for role := range utils.ProjectRolesWithVerbs() {
		clusterRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: utils.ClusterRoleForRole(role),
			},
		}

		result, err := controllerutil.CreateOrUpdate(ctx, setup.client, clusterRole, func() error {
			utils.SetManagementLabels(clusterRole, setup.providerName)

			var err error
			roleID := utils.ProjectMemberRoleToRoleID(role)
			clusterRole.Rules, err = projectPermissionsForRoleGenerator(roleID)
			if err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			return err
		}
		utils.LogOperationResult(log, clusterRole, result)
	}

	return nil
}

func (setup *RBACSetup) createOrUpdateWorkspaceClusterRoles(ctx context.Context, workspacePermissionsForRoleGenerator func(string) ([]rbacv1.PolicyRule, error)) error {
	log := logging.FromContextOrDiscard(ctx)

	for role := range utils.WorkspaceRolesWithVerbs() {
		clusterRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: utils.ClusterRoleForRole(role),
			},
		}

		result, err := controllerutil.CreateOrUpdate(ctx, setup.client, clusterRole, func() error {
			utils.SetManagementLabels(clusterRole, setup.providerName)

			var err error
			roleID := utils.WorkspaceMemberRoleToRoleID(role)
			clusterRole.Rules, err = workspacePermissionsForRoleGenerator(roleID)
			if err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			return err
		}
		utils.LogOperationResult(log, clusterRole, result)
	}

	return nil
}
