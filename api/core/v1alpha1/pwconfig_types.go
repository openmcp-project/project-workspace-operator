package v1alpha1

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectWorkspaceConfigSpec defines the desired state of ProjectWorkspaceConfig
type ProjectWorkspaceConfigSpec struct {
	// +optional
	Project ProjectConfig `json:"project"`
	// +optional
	Workspace WorkspaceConfig `json:"workspace"`
	// MemberOverridesName is the name of the MemberOverrides resource that should be used to manage admin access to the projects and workspaces.
	// Leave empty to disable.
	// +optional
	MemberOverridesName string `json:"memberOverridesName,omitempty"`
	// Webhook contains the configuration for the webhooks.
	// +optional
	Webhook WebhookConfig `json:"webhook"`
}

// ProjectWorkspaceConfig is the Schema for the ProjectWorkspaceConfigs API
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=pwcfg
// +kubebuilder:metadata:labels="openmcp.cloud/cluster=platform"
type ProjectWorkspaceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec ProjectWorkspaceConfigSpec `json:"spec"`
}

// ProjectConfig contains the configuration for projects.
type ProjectConfig struct {
	// +optional
	ResourcesBlockingDeletion []metav1.GroupVersionKind `json:"resourcesBlockingDeletion,omitempty"`
	// AdditionalPermissions defines additional permissions users should have in a project, depending on their role.
	// +optional
	AdditionalPermissions map[ProjectMemberRole][]rbacv1.PolicyRule `json:"additionalPermissions,omitempty"`
}

// WorkspaceConfig contains the configuration for workspaces.
type WorkspaceConfig struct {
	// +optional
	ResourcesBlockingDeletion []metav1.GroupVersionKind `json:"resourcesBlockingDeletion,omitempty"`
	// AdditionalPermissions defines additional permissions users should have in a workspace, depending on their role.
	// +optional
	AdditionalPermissions map[WorkspaceMemberRole][]rbacv1.PolicyRule `json:"additionalPermissions,omitempty"`
}

type WebhookConfig struct {
	// Disabled specifies whether the webhooks should be disabled.
	// +optional
	Disabled bool `json:"disabled"`
}

// +kubebuilder:object:root=true

// ProjectWorkspaceConfigList contains a list of ProjectWorkspaceConfig
type ProjectWorkspaceConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []ProjectWorkspaceConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProjectWorkspaceConfig{}, &ProjectWorkspaceConfigList{})
}

// SetDefaults sets the default values for the project workspace configuration when not set.
func (pwc *ProjectWorkspaceConfig) SetDefaults() {}

// Validate validates the project workspace configuration.
func (pwc *ProjectWorkspaceConfig) Validate() error {
	return nil
}
