package config_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/openmcp-project/controller-utils/pkg/collections"

	pwv1alpha1 "github.com/openmcp-project/platform-service-project-workspace/api/v2/core/v1alpha1"
	"github.com/openmcp-project/platform-service-project-workspace/internal/controller/config"
	"github.com/openmcp-project/platform-service-project-workspace/internal/utils"
)

func TestRBACSetup_EnsureResources(t *testing.T) {
	tests := []struct {
		name                        string
		interceptorFuncs            interceptor.Funcs
		expectedError               *string
		validateFunc                func(ctx context.Context, client client.Client) error
		projectPermissionsForRole   map[string][]rbacv1.PolicyRule
		workspacePermissionsForRole map[string][]rbacv1.PolicyRule
	}{
		{
			name: "Failed to Create/Update Project Cluster Roles",
			projectPermissionsForRole: collections.ProjectMapToMap(defaultProjectPermissionsPerRole(), func(k pwv1alpha1.ProjectMemberRole, v []rbacv1.PolicyRule) (string, []rbacv1.PolicyRule) {
				return utils.ProjectMemberRoleToRoleID(k), v
			}),
			workspacePermissionsForRole: collections.ProjectMapToMap(defaultWorkspacePermissionsPerRole(), func(k pwv1alpha1.WorkspaceMemberRole, v []rbacv1.PolicyRule) (string, []rbacv1.PolicyRule) {
				return utils.WorkspaceMemberRoleToRoleID(k), v
			}),
			interceptorFuncs: interceptor.Funcs{
				Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*rbacv1.ClusterRole); ok {
						return errors.New("some create error")
					}
					return nil
				},
			},
			expectedError: new("some create error"),
		},
		{
			name: "Failed to Create/Update Workspace Cluster Roles",
			projectPermissionsForRole: collections.ProjectMapToMap(defaultProjectPermissionsPerRole(), func(k pwv1alpha1.ProjectMemberRole, v []rbacv1.PolicyRule) (string, []rbacv1.PolicyRule) {
				return utils.ProjectMemberRoleToRoleID(k), v
			}),
			workspacePermissionsForRole: collections.ProjectMapToMap(defaultWorkspacePermissionsPerRole(), func(k pwv1alpha1.WorkspaceMemberRole, v []rbacv1.PolicyRule) (string, []rbacv1.PolicyRule) {
				return utils.WorkspaceMemberRoleToRoleID(k), v
			}),
			interceptorFuncs: interceptor.Funcs{
				Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if role, ok := obj.(*rbacv1.ClusterRole); ok && role.Name == utils.ClusterRoleForRole(pwv1alpha1.WorkspaceRoleView) {
						return errors.New("some create error")
					}
					return client.Create(ctx, obj)
				},
			},
			expectedError: new("some create error"),
		},
		{
			name:          "Successfully Create/Update Project and Workspace Cluster Roles",
			expectedError: nil,
			projectPermissionsForRole: collections.ProjectMapToMap(defaultProjectPermissionsPerRole(), func(k pwv1alpha1.ProjectMemberRole, v []rbacv1.PolicyRule) (string, []rbacv1.PolicyRule) {
				return utils.ProjectMemberRoleToRoleID(k), v
			}),
			workspacePermissionsForRole: collections.ProjectMapToMap(defaultWorkspacePermissionsPerRole(), func(k pwv1alpha1.WorkspaceMemberRole, v []rbacv1.PolicyRule) (string, []rbacv1.PolicyRule) {
				return utils.WorkspaceMemberRoleToRoleID(k), v
			}),
			validateFunc: func(ctx context.Context, client client.Client) error {
				clusterRoleProjectAdmin := &rbacv1.ClusterRole{}
				err := client.Get(ctx, types.NamespacedName{Name: utils.ClusterRoleForRole(pwv1alpha1.ProjectRoleAdmin)}, clusterRoleProjectAdmin)
				if err != nil {
					return err
				}

				assert.ElementsMatch(t, clusterRoleProjectAdmin.Rules, defaultProjectPermissionsPerRole()[pwv1alpha1.ProjectRoleAdmin])

				clusterRoleProjectView := &rbacv1.ClusterRole{}
				err = client.Get(ctx, types.NamespacedName{Name: utils.ClusterRoleForRole(pwv1alpha1.ProjectRoleView)}, clusterRoleProjectView)
				if err != nil {
					return err
				}

				assert.ElementsMatch(t, clusterRoleProjectView.Rules, defaultProjectPermissionsPerRole()[pwv1alpha1.ProjectRoleView])

				clusterRoleWorkspaceAdmin := &rbacv1.ClusterRole{}
				err = client.Get(ctx, types.NamespacedName{Name: utils.ClusterRoleForRole(pwv1alpha1.WorkspaceRoleAdmin)}, clusterRoleWorkspaceAdmin)
				if err != nil {
					return err
				}

				assert.ElementsMatch(t, clusterRoleWorkspaceAdmin.Rules, defaultWorkspacePermissionsPerRole()[pwv1alpha1.WorkspaceRoleAdmin])

				clusterRoleWorkspaceView := &rbacv1.ClusterRole{}
				err = client.Get(ctx, types.NamespacedName{Name: utils.ClusterRoleForRole(pwv1alpha1.WorkspaceRoleView)}, clusterRoleWorkspaceView)
				if err != nil {
					return err
				}

				assert.ElementsMatch(t, clusterRoleWorkspaceView.Rules, defaultWorkspacePermissionsPerRole()[pwv1alpha1.WorkspaceRoleView])

				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			c := fake.NewClientBuilder().WithInterceptorFuncs(tt.interceptorFuncs).Build()
			s := config.NewRBACSetup(c, "test-rbac-controller")

			actualError := s.EnsureResources(ctx, func(roleID string) ([]rbacv1.PolicyRule, error) {
				perms, ok := tt.projectPermissionsForRole[roleID]
				if !ok {
					return nil, fmt.Errorf("error finding permissions for role '%s'", roleID)
				}
				return perms, nil
			}, func(roleID string) ([]rbacv1.PolicyRule, error) {
				perms, ok := tt.workspacePermissionsForRole[roleID]
				if !ok {
					return nil, fmt.Errorf("error finding permissions for role '%s'", roleID)
				}
				return perms, nil
			})

			if tt.expectedError != nil {
				assert.EqualError(t, actualError, *tt.expectedError)
			}
			if tt.validateFunc != nil {
				err := tt.validateFunc(ctx, c)
				assert.NoErrorf(t, err, "validation failed unexpected")
			}
		})
	}
}
