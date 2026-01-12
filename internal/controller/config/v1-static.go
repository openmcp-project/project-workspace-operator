package config

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/controller-utils/pkg/collections"

	pwov1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
	"github.com/openmcp-project/project-workspace-operator/api/install"
)

// V1Config is a global variable containing the v1 implementation of SharedInformation.
// It can only be used after having been initialized via InitializeV1Config.
var V1Config *v1Config

// InitializeV1Config initializes the V1Config variable.
func InitializeV1Config(platformCluster *clusters.Cluster, onboardingClusterConfig *rest.Config, cfg *pwov1alpha1.ProjectWorkspaceConfig) error {
	res := &v1Config{}
	res.onboardingCluster = clusters.New(clusterIDOnboardingInternal).WithRESTConfig(onboardingClusterConfig)
	if err := res.onboardingCluster.InitializeClient(install.InstallOperatorAPIsOnboarding(runtime.NewScheme())); err != nil {
		return fmt.Errorf("error initializing client for onboarding cluster: %w", err)
	}
	res.resourcesBlockingProjectDeletion = collections.ProjectSliceToSlice(cfg.Spec.Project.ResourcesBlockingDeletion, func(gvk metav1.GroupVersionKind) DeletionBlockingResource {
		return DeletionBlockingResource{
			GroupVersionKind: gvk,
			Source:           pwov1alpha1.SourceProjectWorkspaceConfig,
		}
	})
	res.resourcesBlockingProjectDeletion = append(res.resourcesBlockingProjectDeletion, DeletionBlockingResource{
		GroupVersionKind: metav1.GroupVersionKind{
			Group:   pwov1alpha1.GroupVersion.Group,
			Version: pwov1alpha1.GroupVersion.Version,
			Kind:    "Workspace",
		},
		Source: pwov1alpha1.SourceBuiltin,
	})
	res.resourcesBlockingWorkspaceDeletion = collections.ProjectSliceToSlice(cfg.Spec.Workspace.ResourcesBlockingDeletion, func(gvk metav1.GroupVersionKind) DeletionBlockingResource {
		return DeletionBlockingResource{
			GroupVersionKind: gvk,
			Source:           pwov1alpha1.SourceProjectWorkspaceConfig,
		}
	})
	res.resourcesBlockingWorkspaceDeletion = append(res.resourcesBlockingWorkspaceDeletion, DeletionBlockingResource{
		GroupVersionKind: metav1.GroupVersionKind{
			Group:   "core.openmcp.cloud",
			Version: "v1alpha1",
			Kind:    "ManagedControlPlane",
		},
		Source: pwov1alpha1.SourceBuiltin,
	})
	res.projectPermissionsByRole = map[string][]rbacv1.PolicyRule{}
	for role, permissions := range cfg.Spec.Project.AdditionalPermissions {
		roleID := ProjectMemberRoleToRoleID(role)
		if roleID == "" {
			return fmt.Errorf("unable to map project role '%s' to role id", string(role))
		}
		res.projectPermissionsByRole[roleID] = permissions
	}
	res.workspacePermissionsByRole = map[string][]rbacv1.PolicyRule{}
	for role, permissions := range cfg.Spec.Workspace.AdditionalPermissions {
		roleID := WorkspaceMemberRoleToRoleID(role)
		if roleID == "" {
			return fmt.Errorf("unable to map workspace role '%s' to role id", string(role))
		}
		res.workspacePermissionsByRole[roleID] = permissions
	}
	V1Config = res
	return nil
}

type v1Config struct {
	onboardingCluster                  *clusters.Cluster
	resourcesBlockingProjectDeletion   []DeletionBlockingResource
	resourcesBlockingWorkspaceDeletion []DeletionBlockingResource
	projectPermissionsByRole           map[string][]rbacv1.PolicyRule
	workspacePermissionsByRole         map[string][]rbacv1.PolicyRule
}

var _ SharedInformation = &v1Config{}

// OnboardingClusterForProjectController implements SharedInformation.
func (v *v1Config) OnboardingClusterForProjectController(ctx context.Context) (*clusters.Cluster, error) {
	panic("unimplemented")
}

// OnboardingClusterForWorkspaceController implements SharedInformation.
func (v *v1Config) OnboardingClusterForWorkspaceController(ctx context.Context) (*clusters.Cluster, error) {
	panic("unimplemented")
}

// ResourcesBlockingProjectDeletion implements SharedInformation.
func (v *v1Config) ResourcesBlockingProjectDeletion(ctx context.Context) ([]DeletionBlockingResource, error) {
	panic("unimplemented")
}

// ResourcesBlockingWorkspaceDeletion implements SharedInformation.
func (v *v1Config) ResourcesBlockingWorkspaceDeletion(ctx context.Context) ([]DeletionBlockingResource, error) {
	panic("unimplemented")
}

// ProjectPermissionsForRole implements SharedInformation.
func (v *v1Config) ProjectPermissionsForRole(ctx context.Context, roleID string) ([]rbacv1.PolicyRule, error) {
	panic("unimplemented")
}

// WorkspacePermissionsForRole implements SharedInformation.
func (v *v1Config) WorkspacePermissionsForRole(ctx context.Context, roleID string) ([]rbacv1.PolicyRule, error) {
	panic("unimplemented")
}
