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
	_ entities.AccessEntity = &Project{}
	_ entities.AccessRole   = ProjectRoleAdmin
	_ entities.AccessRole   = ProjectRoleView
)

// +kubebuilder:validation:Enum=admin;view
type ProjectMemberRole string

// Identifier implements AccessRole.
func (p ProjectMemberRole) Identifier() string {
	return string(p)
}

// EntityType implements AccessRole.
func (ProjectMemberRole) EntityType() entities.AccessEntity {
	return &Project{}
}

const (
	ProjectRoleAdmin ProjectMemberRole = "admin"
	ProjectRoleView  ProjectMemberRole = "view"
)

// ProjectSpec defines the desired state of Project
type ProjectSpec struct {
	// Members is a list of project members.
	Members []ProjectMember `json:"members,omitempty"`
}

type ProjectMember struct {
	Subject `json:""`

	// Roles defines a list of roles that this project member should have.
	Roles []ProjectMemberRole `json:"roles"`
}

func (pm *ProjectMember) Username() (string, bool) {
	switch pm.Kind {
	case rbacv1.UserKind:
		return pm.Name, true
	case rbacv1.ServiceAccountKind:
		return fmt.Sprintf("system:serviceaccount:%s:%s", pm.Namespace, pm.Name), true
	default:
		return "", false
	}
}

// ProjectStatus defines the observed state of Project
type ProjectStatus struct {
	Namespace string `json:"namespace"`
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

// Project is the Schema for the projects API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Display Name",type="string",JSONPath=".metadata.annotations.openmcp\\.cloud/display-name"
// +kubebuilder:printcolumn:name="Resulting Namespace",type="string",JSONPath=".status.namespace"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:validation:XValidation:rule="size(self.metadata.name) <= 25",message="Name must not be longer than 25 characters"
// +kubebuilder:metadata:labels="openmcp.cloud/cluster=onboarding"
type Project struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectSpec   `json:"spec,omitempty"`
	Status ProjectStatus `json:"status,omitempty"`
}

// TypeIdentifier implements AccessEntity.
func (p *Project) TypeIdentifier() string {
	return "project"
}

func (p *Project) UserInfoRoles(userInfo authv1.UserInfo) []ProjectMemberRole {
	effectiveRoles := sets.Set[ProjectMemberRole]{}

	// check for access through username (including service accounts)
	for _, member := range p.Spec.Members {
		name, ok := member.Username()
		if name == userInfo.Username && ok {
			effectiveRoles.Insert(member.Roles...)
		}
	}

	// check for access through groups
	for _, userGroup := range userInfo.Groups {
		for _, member := range p.Spec.Members {
			if member.Kind == rbacv1.GroupKind && member.Name == userGroup {
				effectiveRoles.Insert(member.Roles...)
			}
		}
	}

	return effectiveRoles.UnsortedList()
}

func (p *Project) UserInfoHasRole(userInfo authv1.UserInfo, role ProjectMemberRole) bool {
	effectiveRoles := p.UserInfoRoles(userInfo)
	for _, pmr := range effectiveRoles {
		if pmr == role {
			return true
		}
	}
	return false
}

func (p *Project) SetOrUpdateCondition(condition Condition) {
	var existingCondition *Condition
	for i, c := range p.Status.Conditions {
		if c.Type == condition.Type {
			existingCondition = &p.Status.Conditions[i]
			break
		}
	}

	if existingCondition == nil {
		condition.LastTransitionTime = metav1.Now()
		p.Status.Conditions = append(p.Status.Conditions, condition)
	} else {
		if existingCondition.Status != condition.Status {
			condition.LastTransitionTime = metav1.Now()
		} else {
			condition.LastTransitionTime = existingCondition.LastTransitionTime
		}
		*existingCondition = condition
	}
}

func (p *Project) RemoveCondition(conditionType ConditionType) {
	var conditions []Condition
	for _, c := range p.Status.Conditions {
		if c.Type != conditionType {
			conditions = append(conditions, c)
		}
	}
	p.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// ProjectList contains a list of Project
type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Project `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Project{}, &ProjectList{})
}
