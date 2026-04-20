package config

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openmcp-project/controller-utils/pkg/clusters"

	pwov1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

// DeletionBlockingResource represents a resource that should block deletion of a project or workspace.
// It contains the GroupVersionKind of the resource and the source of this information for logging purposes.
type DeletionBlockingResource struct {
	// This is the GroupVersionKind of the resource that should block deletion.
	metav1.GroupVersionKind `json:",inline"`
	// Source is where this GVK comes from, e.g. config or a service provider. It is used for logging purposes.
	Source string `json:"source"`
}

func (dbr *DeletionBlockingResource) DeepCopy() *DeletionBlockingResource {
	return &DeletionBlockingResource{
		GroupVersionKind: *dbr.GroupVersionKind.DeepCopy(),
		Source:           dbr.Source,
	}
}

// SharedInformation holds information that is required by multiple controllers.
// There should be one instance which every controller can access.
// The implementation has to be thread-safe.
type SharedInformation interface {
	// ResourcesBlockingProjectDeletion returns a list of resources that should block project deletion.
	// Each entry is a GroupVersionKind with an additional 'Source' field containing a string representation of the source of this information (e.g. config or a service provider).
	ResourcesBlockingProjectDeletion(ctx context.Context) ([]DeletionBlockingResource, error)
	// ResourcesBlockingWorkspaceDeletion returns a list of resources that should block workspace deletion.
	// Each entry is a GroupVersionKind with an additional 'Source' field containing a string representation of the source of this information (e.g. config or a service provider).
	ResourcesBlockingWorkspaceDeletion(ctx context.Context) ([]DeletionBlockingResource, error)

	// MemberOverrides returns the users and groups that should have admin permissions to projects and workspaces, bypassing the 'you must be admin of a project/workspace in order to modify it' check.
	MemberOverrides(ctx context.Context) (pwov1alpha1.MemberOverrides, error)

	// OnboardingClusterStatic returns the static access to the onboarding cluster.
	// It has permissions for namespaces, rbac resources, CRDs, and Project/Workspace resources.
	// For listing resources that potentially block deletion of projects or workspaces, the dynamic client needs to be used.
	OnboardingClusterStatic(ctx context.Context) (*clusters.Cluster, error)
	// OnboardingClusterDynamic returns the dynamic access to the onboarding cluster.
	// It is regularly updated to include get permissions for all resources that might block deletion of projects or workspaces.
	// For interacting with any other resource, the static client needs to be used.
	OnboardingClusterDynamic(ctx context.Context) (*clusters.Cluster, error)
}
