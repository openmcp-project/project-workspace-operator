package v1alpha1

import (
	"encoding/json"
	"fmt"
	"strings"

	authv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ChargingTargetLabel string = "openmcp.cloud.sap/charging-target"
)

var (
	CreatedByAnnotation        = fmt.Sprintf("%s/created-by", GroupVersion.Group)
	DisplayNameAnnotation      = fmt.Sprintf("%s/display-name", GroupVersion.Group)
	EnforceChargingTargetLabel = false
)

// Subject contains a reference to the object or user identities a role binding applies to. This can either hold a direct API object reference,
// or a value for non-objects such as user and group names.
// +kubebuilder:validation:XValidation:rule="self.kind == 'ServiceAccount' || !has(self.__namespace__)",message="Namespace must not be specified if Kind is User or Group"
// +kubebuilder:validation:XValidation:rule="self.kind != 'ServiceAccount' || has(self.__namespace__)",message="Namespace is required for ServiceAccount"
type Subject struct {
	// Kind of object being referenced. Can be "User", "Group", or "ServiceAccount".
	// +kubebuilder:validation:Enum=User;Group;ServiceAccount
	Kind string `json:"kind"`

	// Name of the object being referenced.
	Name string `json:"name"`

	// Namespace of the referenced object. Required if Kind is "ServiceAccount". Must not be specified if Kind is "User" or "Group".
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

func (s Subject) RbacV1() rbacv1.Subject {
	rs := rbacv1.Subject{
		Kind:      s.Kind,
		Name:      s.Name,
		Namespace: s.Namespace,
	}
	if s.Kind != rbacv1.ServiceAccountKind {
		rs.APIGroup = rbacv1.GroupName
	}
	return rs
}

// MemberOverrides is a resource used to Manage admin access to the Project/Workspace operator resources.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
type MemberOverrides struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MemberOverridesSpec   `json:"spec,omitempty"`
	Status MemberOverridesStatus `json:"status,omitempty"`
}
type MemberOverridesSpec struct {
	MemberOverrides []MemberOverride `json:"memberOverrides"`
}

type MemberOverridesStatus struct{}

// +kubebuilder:validation:Enum=admin;view
type OverrideRole string

const (
	OverrideRoleAdmin OverrideRole = "admin"
	OverrideRoleView  OverrideRole = "view"
)

type MemberOverride struct {
	Subject `json:",inline"`
	// Roles defines a list of roles that this override subject should have.
	Roles []OverrideRole `json:"roles"`
	// Resources defines an optional list of projects/workspaces that this override applies to.
	Resources []OverrideResource `json:"resources,omitempty"`
}

type OverrideResource struct {
	// +kubebuilder:validation:Enum=project;workspace
	Kind string `json:"kind"`
	// Name of the object being referenced.
	Name string `json:"name"`
}

// +kubebuilder:object:root=true
type MemberOverridesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MemberOverrides `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MemberOverrides{}, &MemberOverridesList{})
}

func (m *MemberOverrides) HasAdminOverrideForResource(userInfo *authv1.UserInfo, resourceName, resourceKind string) bool {
	for _, override := range m.Spec.MemberOverrides {
		if !override.hasAdminRole() {
			continue
		}
		// all-resources admin override for a user
		if override.hasUserOrSA(userInfo) && len(override.Resources) == 0 {
			return true
		}
		// all-resources admin override for group
		if override.hasGroup(userInfo) && len(override.Resources) == 0 {
			return true
		}
		// resource specific admin user/sa/group override
		if override.hasResource(resourceName, resourceKind) &&
			(override.hasUserOrSA(userInfo) || override.hasGroup(userInfo)) {
			return true
		}
	}
	return false
}

func (m *MemberOverride) hasResource(resourceName, resourceKind string) bool {
	for _, resource := range m.Resources {
		if strings.EqualFold(resource.Kind, resourceKind) && strings.EqualFold(resource.Name, resourceName) {
			return true
		}
	}
	return false
}

func (m *MemberOverride) hasUserOrSA(userInfo *authv1.UserInfo) bool {
	name, isUserOrSA := m.Username()
	if !isUserOrSA {
		return false
	}

	return name == userInfo.Username
}

func (m *MemberOverride) hasGroup(userInfo *authv1.UserInfo) bool {
	if m.Kind != rbacv1.GroupKind {
		return false
	}
	for _, groupName := range userInfo.Groups {
		if m.Name == groupName {
			return true
		}
	}
	return false
}

func (m *MemberOverride) hasAdminRole() bool {
	for _, role := range m.Roles {
		if role == OverrideRoleAdmin {
			return true
		}
	}
	return false
}

func (m *MemberOverride) Username() (string, bool) {
	switch m.Kind {
	case rbacv1.UserKind:
		return m.Name, true
	case rbacv1.ServiceAccountKind:
		return fmt.Sprintf("system:serviceaccount:%s:%s", m.Namespace, m.Name), true
	default:
		return "", false
	}
}

// RemainingContentResource is a resource used to track remaining content in a workspace.
// It is solely used as an information resource to inform the user about remaining content.
type RemainingContentResource struct {
	// APIGroup is the group of the resource.
	APIGroup string `json:"apiGroup"`
	// Kind is the kind of the resource.
	Kind string `json:"kind"`
	// Name is the name of the resource.
	Name string `json:"name"`
	// Namespace is the namespace of the resource.
	Namespace string `json:"namespace"`
}

const (
	// ConditionTypeContentRemaining is a condition type that indicates that there is content in a project/workspace
	// that is preventing the deletion.
	ConditionTypeContentRemaining ConditionType = "ContentRemaining"

	// ConditionReasonResourcesRemaining is a condition reason that indicates that there are remaining resources in a
	// project/workspace that are preventing the deletion.
	ConditionReasonResourcesRemaining ConditionReason = "SomeResourcesRemain"

	// ConditionStatusTrue indicates that the condition is currently active.
	ConditionStatusTrue ConditionStatus = "True"
	// ConditionStatusFalse indicates that the condition is not currently active.
	ConditionStatusFalse ConditionStatus = "False"
	// ConditionStatusUnknown indicates that the condition status is unknown.
	ConditionStatusUnknown ConditionStatus = "Unknown"
)

// ConditionType is a type of condition.
type ConditionType string

// ConditionReason is a reason for why a condition is set.
type ConditionReason string

// ConditionStatus is the status of a condition.
type ConditionStatus string

// Condition is part of all conditions that a project/ workspace can have.
type Condition struct {
	// Type is the type of the condition.
	Type ConditionType `json:"type"`
	// Status is the status of the condition.
	// +kubebuilder:validation:Enum=True;False;Unknown
	Status ConditionStatus `json:"status"`
	// LastTransitionTime is the time when the condition last transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Reason is the reason for the condition.
	// +optional
	Reason ConditionReason `json:"reason"`
	// Message is a human-readable message indicating details about the condition.
	// +optional
	Message string `json:"message,omitempty"`
	// Details is an object that can contain additional information about the condition.
	// The content is specific to the condition type.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +optional
	Details json.RawMessage `json:"details,omitempty"`
}
