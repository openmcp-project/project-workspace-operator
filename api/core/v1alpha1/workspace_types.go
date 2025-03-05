package v1alpha1

import (
	"fmt"

	authv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openmcp-project/project-workspace-operator/api/entities"
)

var (
	_ entities.AccessEntity = &Workspace{}
	_ entities.AccessRole   = WorkspaceRoleAdmin
	_ entities.AccessRole   = WorkspaceRoleView
)

// +kubebuilder:validation:Enum=admin;view
type WorkspaceMemberRole string

// EntityType implements AccessRole.
func (w WorkspaceMemberRole) EntityType() entities.AccessEntity {
	return &Workspace{}
}

// Identifier implements AccessRole.
func (w WorkspaceMemberRole) Identifier() string {
	return string(w)
}

const (
	WorkspaceRoleAdmin WorkspaceMemberRole = "admin"
	WorkspaceRoleView  WorkspaceMemberRole = "view"
)

// WorkspaceSpec defines the desired state of Workspace
type WorkspaceSpec struct {
	// Members is a list of workspace members.
	Members []WorkspaceMember `json:"members,omitempty"`
}

type WorkspaceMember struct {
	Subject `json:""`

	// Roles defines a list of roles that this workspace member should have.
	Roles []WorkspaceMemberRole `json:"roles"`
}

func (wm *WorkspaceMember) Username() (string, bool) {
	switch wm.Kind {
	case rbacv1.UserKind:
		return wm.Name, true
	case rbacv1.ServiceAccountKind:
		return fmt.Sprintf("system:serviceaccount:%s:%s", wm.Namespace, wm.Name), true
	default:
		return "", false
	}
}

// WorkspaceStatus defines the observed state of Workspace
type WorkspaceStatus struct {
	Namespace string `json:"namespace"`
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

// Workspace is the Schema for the workspaces API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ws
// +kubebuilder:printcolumn:name="Display Name",type="string",JSONPath=".metadata.annotations.openmcp\\.cloud/display-name"
// +kubebuilder:printcolumn:name="Resulting Namespace",type="string",JSONPath=".status.namespace"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:validation:XValidation:rule="size(self.metadata.name) <= 25",message="Name must not be longer than 25 characters"
type Workspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkspaceSpec   `json:"spec,omitempty"`
	Status WorkspaceStatus `json:"status,omitempty"`
}

// TypeIdentifier implements AccessEntity.
func (ws *Workspace) TypeIdentifier() string {
	return "workspace"
}

func (ws *Workspace) UserInfoRoles(userInfo authv1.UserInfo) []WorkspaceMemberRole {
	effectiveRoles := sets.Set[WorkspaceMemberRole]{}

	// check for access through username (including service accounts)
	for _, member := range ws.Spec.Members {
		name, ok := member.Username()
		if name == userInfo.Username && ok {
			effectiveRoles.Insert(member.Roles...)
		}
	}

	// check for access through groups
	for _, userGroup := range userInfo.Groups {
		for _, member := range ws.Spec.Members {
			if member.Kind == rbacv1.GroupKind && member.Name == userGroup {
				effectiveRoles.Insert(member.Roles...)
			}
		}
	}

	return effectiveRoles.UnsortedList()
}

func (ws *Workspace) UserInfoHasRole(userInfo authv1.UserInfo, role WorkspaceMemberRole) bool {
	effectiveRoles := ws.UserInfoRoles(userInfo)
	for _, pmr := range effectiveRoles {
		if pmr == role {
			return true
		}
	}
	return false
}

func (ws *Workspace) SetOrUpdateCondition(condition Condition) {
	var existingCondition *Condition
	for i, c := range ws.Status.Conditions {
		if c.Type == condition.Type {
			existingCondition = &ws.Status.Conditions[i]
			break
		}
	}

	if existingCondition == nil {
		condition.LastTransitionTime = metav1.Now()
		ws.Status.Conditions = append(ws.Status.Conditions, condition)
	} else {
		if existingCondition.Status != condition.Status {
			condition.LastTransitionTime = metav1.Now()
		} else {
			condition.LastTransitionTime = existingCondition.LastTransitionTime
		}
		*existingCondition = condition
	}
}

func (ws *Workspace) RemoveCondition(conditionType ConditionType) {
	var conditions []Condition
	for _, c := range ws.Status.Conditions {
		if c.Type != conditionType {
			conditions = append(conditions, c)
		}
	}
	ws.Status.Conditions = conditions
}

//+kubebuilder:object:root=true

// WorkspaceList contains a list of Workspace
type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workspace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workspace{}, &WorkspaceList{})
}
