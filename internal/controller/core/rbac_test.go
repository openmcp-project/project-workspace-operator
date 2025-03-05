package core

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

func TestRBACSetup_EnsureResources(t *testing.T) {
	tests := []struct {
		name             string
		interceptorFuncs interceptor.Funcs
		expectedError    *string
		validateFunc     func(ctx context.Context, client client.Client) error
	}{
		{
			name: "Failed to Create/Update Project Cluster Roles",
			interceptorFuncs: interceptor.Funcs{
				Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*rbacv1.ClusterRole); ok {
						return errors.New("some create error")
					}
					return nil
				},
			},
			expectedError: ptr.To("some create error"),
		},
		{
			name: "Failed to Create/Update Workspace Cluster Roles",
			interceptorFuncs: interceptor.Funcs{
				Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if role, ok := obj.(*rbacv1.ClusterRole); ok && role.Name == clusterRoleForRole(v1alpha1.WorkspaceRoleView) {
						return errors.New("some create error")
					}
					return client.Create(ctx, obj)
				},
			},
			expectedError: ptr.To("some create error"),
		},
		{
			name:          "Successfully Create/Update Project and Workspace Cluster Roles",
			expectedError: nil,
			validateFunc: func(ctx context.Context, client client.Client) error {
				clusterRoleProjectAdmin := &rbacv1.ClusterRole{}
				err := client.Get(ctx, types.NamespacedName{Name: clusterRoleForRole(v1alpha1.ProjectRoleAdmin)}, clusterRoleProjectAdmin)
				if err != nil {
					return err
				}

				assert.NotEmpty(t, clusterRoleProjectAdmin.Rules)

				clusterRoleProjectView := &rbacv1.ClusterRole{}
				err = client.Get(ctx, types.NamespacedName{Name: clusterRoleForRole(v1alpha1.ProjectRoleView)}, clusterRoleProjectView)
				if err != nil {
					return err
				}

				assert.NotEmpty(t, clusterRoleProjectView.Rules)

				clusterRoleWorkspaceAdmin := &rbacv1.ClusterRole{}
				err = client.Get(ctx, types.NamespacedName{Name: clusterRoleForRole(v1alpha1.WorkspaceRoleAdmin)}, clusterRoleWorkspaceAdmin)
				if err != nil {
					return err
				}

				assert.NotEmpty(t, clusterRoleWorkspaceAdmin.Rules)

				clusterRoleWorkspaceView := &rbacv1.ClusterRole{}
				err = client.Get(ctx, types.NamespacedName{Name: clusterRoleForRole(v1alpha1.WorkspaceRoleView)}, clusterRoleWorkspaceView)
				if err != nil {
					return err
				}

				assert.NotEmpty(t, clusterRoleWorkspaceView.Rules)

				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			c := fake.NewClientBuilder().WithInterceptorFuncs(tt.interceptorFuncs).Build()
			s := NewRBACSetup(testr.New(t), c, "test-rbac-controller")

			actualError := s.EnsureResources(ctx)

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
