package config

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/controller-utils/pkg/clusters"

	pwv1alpha1 "github.com/openmcp-project/platform-service-project-workspace/api/v2/core/v1alpha1"
)

func NewFakeSharedInformation(onboardingClient client.Client, resourcesBlockingProjectDeletion []DeletionBlockingResource, resourcesBlockingWorkspaceDeletion []DeletionBlockingResource, memberOverrides pwv1alpha1.MemberOverrides) *FakeSharedInformation {
	return &FakeSharedInformation{
		OnboardingCluster:                      clusters.NewTestClusterFromClient("onboarding", onboardingClient),
		ResourcesBlockingProjectDeletionData:   resourcesBlockingProjectDeletion,
		ResourcesBlockingWorkspaceDeletionData: resourcesBlockingWorkspaceDeletion,
		MemberOverridesData:                    memberOverrides,
	}
}

// FakeSharedInformation is a dummy implementation of the SharedInformation interface.
// It is meant for unit tests and should not be used anywhere else.
type FakeSharedInformation struct {
	OnboardingCluster                      *clusters.Cluster
	ResourcesBlockingProjectDeletionData   []DeletionBlockingResource
	ResourcesBlockingWorkspaceDeletionData []DeletionBlockingResource
	MemberOverridesData                    pwv1alpha1.MemberOverrides
}

var _ SharedInformation = &FakeSharedInformation{}

// MemberOverrides implements SharedInformation.
func (f *FakeSharedInformation) MemberOverrides(ctx context.Context) (pwv1alpha1.MemberOverrides, error) {
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
