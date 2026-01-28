package config

import (
	"context"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
)

func NewFakeSharedInformation(onboardingClient client.Client, projectPermissionsByRole map[string][]rbacv1.PolicyRule, workspacePermissionsByRole map[string][]rbacv1.PolicyRule, resourcesBlockingProjectDeletion []DeletionBlockingResource, resourcesBlockingWorkspaceDeletion []DeletionBlockingResource) *FakeSharedInformation {
	return &FakeSharedInformation{
		onboardingCluster:                  clusters.NewTestClusterFromClient("onboarding", onboardingClient),
		projectPermissionsByRole:           projectPermissionsByRole,
		workspacePermissionsByRole:         workspacePermissionsByRole,
		resourcesBlockingProjectDeletion:   resourcesBlockingProjectDeletion,
		resourcesBlockingWorkspaceDeletion: resourcesBlockingWorkspaceDeletion,
	}
}

// FakeSharedInformation is a dummy implementation of the SharedInformation interface.
// It is meant for unit tests and should not be used anywhere else.
type FakeSharedInformation struct {
	onboardingCluster                  *clusters.Cluster
	projectPermissionsByRole           map[string][]rbacv1.PolicyRule
	workspacePermissionsByRole         map[string][]rbacv1.PolicyRule
	resourcesBlockingProjectDeletion   []DeletionBlockingResource
	resourcesBlockingWorkspaceDeletion []DeletionBlockingResource
}

var _ SharedInformation = &FakeSharedInformation{}

// OnboardingClusterDynamic implements SharedInformation.
func (f *FakeSharedInformation) OnboardingClusterDynamic(ctx context.Context) (*clusters.Cluster, error) {
	return f.onboardingCluster, nil
}

// OnboardingClusterStatic implements SharedInformation.
func (f *FakeSharedInformation) OnboardingClusterStatic(ctx context.Context) (*clusters.Cluster, error) {
	return f.onboardingCluster, nil
}

// ProjectPermissionsForRole implements SharedInformation.
func (f *FakeSharedInformation) ProjectPermissionsForRole(ctx context.Context, roleID string) ([]rbacv1.PolicyRule, error) {
	return f.projectPermissionsByRole[roleID], nil
}

// ResourcesBlockingProjectDeletion implements SharedInformation.
func (f *FakeSharedInformation) ResourcesBlockingProjectDeletion(ctx context.Context) ([]DeletionBlockingResource, error) {
	return f.resourcesBlockingProjectDeletion, nil
}

// ResourcesBlockingWorkspaceDeletion implements SharedInformation.
func (f *FakeSharedInformation) ResourcesBlockingWorkspaceDeletion(ctx context.Context) ([]DeletionBlockingResource, error) {
	return f.resourcesBlockingWorkspaceDeletion, nil
}

// WorkspacePermissionsForRole implements SharedInformation.
func (f *FakeSharedInformation) WorkspacePermissionsForRole(ctx context.Context, roleID string) ([]rbacv1.PolicyRule, error) {
	return f.workspacePermissionsByRole[roleID], nil
}
