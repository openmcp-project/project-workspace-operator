package config

import (
	"context"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/controller-utils/pkg/clusters"

	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

func NewFakeSharedInformation(onboardingClient client.Client, projectPermissionsByRole map[string][]rbacv1.PolicyRule, workspacePermissionsByRole map[string][]rbacv1.PolicyRule, resourcesBlockingProjectDeletion []DeletionBlockingResource, resourcesBlockingWorkspaceDeletion []DeletionBlockingResource, memberOverrides pwv1alpha1.MemberOverridesV2) *FakeSharedInformation {
	return &FakeSharedInformation{
		OnboardingCluster:                      clusters.NewTestClusterFromClient("onboarding", onboardingClient),
		ProjectPermissionsByRole:               projectPermissionsByRole,
		WorkspacePermissionsByRole:             workspacePermissionsByRole,
		ResourcesBlockingProjectDeletionData:   resourcesBlockingProjectDeletion,
		ResourcesBlockingWorkspaceDeletionData: resourcesBlockingWorkspaceDeletion,
		MemberOverridesData:                    memberOverrides,
	}
}

// FakeSharedInformation is a dummy implementation of the SharedInformation interface.
// It is meant for unit tests and should not be used anywhere else.
type FakeSharedInformation struct {
	OnboardingCluster                      *clusters.Cluster
	ProjectPermissionsByRole               map[string][]rbacv1.PolicyRule
	WorkspacePermissionsByRole             map[string][]rbacv1.PolicyRule
	ResourcesBlockingProjectDeletionData   []DeletionBlockingResource
	ResourcesBlockingWorkspaceDeletionData []DeletionBlockingResource
	MemberOverridesData                    pwv1alpha1.MemberOverridesV2
}

var _ SharedInformation = &FakeSharedInformation{}

// MemberOverrides implements SharedInformation.
func (f *FakeSharedInformation) MemberOverrides(ctx context.Context) (pwv1alpha1.MemberOverridesV2, error) {
	if f == nil {
		return nil, nil
	}
	return f.MemberOverridesData, nil
}

// OnboardingClusterDynamic implements SharedInformation.
func (f *FakeSharedInformation) OnboardingClusterDynamic(ctx context.Context) (*clusters.Cluster, error) {
	if f == nil {
		return nil, nil
	}
	return f.OnboardingCluster, nil
}

// OnboardingClusterStatic implements SharedInformation.
func (f *FakeSharedInformation) OnboardingClusterStatic(ctx context.Context) (*clusters.Cluster, error) {
	if f == nil {
		return nil, nil
	}
	return f.OnboardingCluster, nil
}

// ProjectPermissionsForRole implements SharedInformation.
func (f *FakeSharedInformation) ProjectPermissionsForRole(ctx context.Context, roleID string) ([]rbacv1.PolicyRule, error) {
	if f == nil {
		return nil, nil
	}
	return f.ProjectPermissionsByRole[roleID], nil
}

// ResourcesBlockingProjectDeletion implements SharedInformation.
func (f *FakeSharedInformation) ResourcesBlockingProjectDeletion(ctx context.Context) ([]DeletionBlockingResource, error) {
	if f == nil {
		return nil, nil
	}
	return f.ResourcesBlockingProjectDeletionData, nil
}

// ResourcesBlockingWorkspaceDeletion implements SharedInformation.
func (f *FakeSharedInformation) ResourcesBlockingWorkspaceDeletion(ctx context.Context) ([]DeletionBlockingResource, error) {
	if f == nil {
		return nil, nil
	}
	return f.ResourcesBlockingWorkspaceDeletionData, nil
}

// WorkspacePermissionsForRole implements SharedInformation.
func (f *FakeSharedInformation) WorkspacePermissionsForRole(ctx context.Context, roleID string) ([]rbacv1.PolicyRule, error) {
	if f == nil {
		return nil, nil
	}
	return f.WorkspacePermissionsByRole[roleID], nil
}
