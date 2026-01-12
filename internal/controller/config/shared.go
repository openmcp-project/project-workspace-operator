package config

import (
	"context"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openmcp-project/controller-utils/pkg/clusters"

	pwov1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

// DeletionBlockingResource represents a resource that should block deletion of a project or workspace.
// It contains the GroupVersionKind of the resource and the source of this information for logging purposes.
type DeletionBlockingResource struct {
	// This is the GroupVersionKind of the resource that should block deletion.
	metav1.GroupVersionKind
	// Source is where this GVK comes from, e.g. config or a service provider. It is used for logging purposes.
	Source string
}

// SharedInformation holds information that is required by multiple controllers.
// There should be one instance which every controller can access.
// The implementation has to be thread-safe.
//
// This is an interface so that we can implement a v1 version (where the information is static)
// and a v2 version (where this is populated by the config controller).
// This avoids having v1/v2 splits in the actual controller code.
type SharedInformation interface {
	// ResourcesBlockingProjectDeletion returns a list of resources that should block project deletion.
	// Each entry is a GroupVersionKind with an additional 'Source' field containing a string representation of the source of this information (e.g. config or a service provider).
	ResourcesBlockingProjectDeletion(ctx context.Context) ([]DeletionBlockingResource, error)
	// ResourcesBlockingWorkspaceDeletion returns a list of resources that should block workspace deletion.
	// Each entry is a GroupVersionKind with an additional 'Source' field containing a string representation of the source of this information (e.g. config or a service provider).
	ResourcesBlockingWorkspaceDeletion(ctx context.Context) ([]DeletionBlockingResource, error)

	// ProjectPermissionsForRole returns the permissions that users with the given role should have in a project namespace.
	ProjectPermissionsForRole(ctx context.Context, roleID string) ([]rbacv1.PolicyRule, error)
	// WorkspacePermissionsForRole returns the permissions that users with the given role should have in a workspace namespace.
	WorkspacePermissionsForRole(ctx context.Context, roleID string) ([]rbacv1.PolicyRule, error)

	// OnboardingClusterForProjectController returns access to the onboarding cluster.
	// The returned cluster can be expected to have 'get' permissions for all resources returned by ResourcesBlockingProjectDeletion.
	// Note that the corresponding resources are not part of the client's scheme and therefore can only be retrieved as unstructured objects.
	OnboardingClusterForProjectController(ctx context.Context) (*clusters.Cluster, error)
	// OnboardingClusterForWorkspaceController returns access to the onboarding cluster.
	// The returned cluster can be expected to have 'get' permissions for all resources returned by ResourcesBlockingWorkspaceDeletion.
	// Note that the corresponding resources are not part of the client's scheme and therefore can only be retrieved as unstructured objects.
	OnboardingClusterForWorkspaceController(ctx context.Context) (*clusters.Cluster, error)
}

const (
	AdminRole  = "admin"
	ViewerRole = "viewer"
)

func ProjectMemberRoleToRoleID(role pwov1alpha1.ProjectMemberRole) string {
	switch role {
	case pwov1alpha1.ProjectRoleAdmin:
		return AdminRole
	case pwov1alpha1.ProjectRoleView:
		return ViewerRole
	default:
		return ""
	}
}

func WorkspaceMemberRoleToRoleID(role pwov1alpha1.WorkspaceMemberRole) string {
	switch role {
	case pwov1alpha1.WorkspaceRoleAdmin:
		return AdminRole
	case pwov1alpha1.WorkspaceRoleView:
		return ViewerRole
	default:
		return ""
	}
}
