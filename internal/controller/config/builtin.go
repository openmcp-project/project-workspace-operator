package config

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	openmcpcorev2alpha1 "github.com/openmcp-project/openmcp-operator/api/core/v2alpha1"

	pwv1alpha1 "github.com/openmcp-project/platform-service-project-workspace/api/v2/core/v1alpha1"
	"github.com/openmcp-project/platform-service-project-workspace/internal/utils"
)

const (
	OpenMCPV1ApiGroup   = "core.openmcp.cloud"
	OpenMCPV1ApiVersion = "v1alpha1"
)

var SupportV1 bool

func BuiltinResourcesBlockingProjectDeletion() []DeletionBlockingResource {
	return []DeletionBlockingResource{
		{
			GroupVersionKind: metav1.GroupVersionKind{
				Group:   pwv1alpha1.GroupVersion.Group,
				Version: pwv1alpha1.GroupVersion.Version,
				Kind:    "Workspace",
			},
			Source: pwv1alpha1.SourceBuiltin,
		},
	}
}

func BuiltinResourcesBlockingWorkspaceDeletion() []DeletionBlockingResource {
	res := []DeletionBlockingResource{
		{
			GroupVersionKind: metav1.GroupVersionKind{
				Group:   openmcpcorev2alpha1.GroupVersion.Group,
				Version: openmcpcorev2alpha1.GroupVersion.Version,
				Kind:    "ManagedControlPlaneV2",
			},
			Source: pwv1alpha1.SourceBuiltin,
		},
	}
	if SupportV1 {
		res = append(res, DeletionBlockingResource{
			GroupVersionKind: metav1.GroupVersionKind{
				Group:   OpenMCPV1ApiGroup,
				Version: OpenMCPV1ApiVersion,
				Kind:    "ManagedControlPlane",
			},
			Source: pwv1alpha1.SourceBuiltin,
		}, DeletionBlockingResource{
			GroupVersionKind: metav1.GroupVersionKind{
				Group:   OpenMCPV1ApiGroup,
				Version: OpenMCPV1ApiVersion,
				Kind:    "ClusterAdmin",
			},
			Source: pwv1alpha1.SourceBuiltin,
		})
	}
	return res
}

func BuiltinPermissibleProjectResources() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{pwv1alpha1.GroupVersion.String()},
			Resources: []string{"workspaces"},
		},
		{
			APIGroups: []string{corev1.GroupName},
			Resources: []string{"serviceaccounts"},
		},
		{
			APIGroups: []string{corev1.GroupName}, // this rule prevents k9s from crashing
			Resources: []string{"pods"},
			Verbs:     []string{"list"},
		},
		{
			APIGroups: []string{corev1.GroupName},
			Resources: []string{"resourcequotas"},
			Verbs:     utils.ReadOnlyVerbs(),
		},
	}
}

func BuiltinPermissibleProjectResourcesAdminOnly() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{corev1.GroupName},
			Resources: []string{"serviceaccounts/token"},
			Verbs:     []string{"create"},
		},
		{
			APIGroups: []string{corev1.GroupName},
			Resources: []string{"secrets"},
		},
	}
}

func BuiltinPermissibleWorkspaceResources() []rbacv1.PolicyRule {
	res := []rbacv1.PolicyRule{
		{
			APIGroups: []string{openmcpcorev2alpha1.GroupVersion.String()},
			Resources: []string{"managedcontrolplanev2s"},
		},
		{
			APIGroups: []string{corev1.GroupName},
			Resources: []string{
				"secrets",
				"configmaps",
				"serviceaccounts",
			},
		},
		{
			APIGroups: []string{corev1.GroupName}, // this rule prevents k9s from crashing
			Resources: []string{"pods"},
			Verbs:     []string{"list"},
		},
		{
			APIGroups: []string{corev1.GroupName},
			Resources: []string{"resourcequotas"},
			Verbs:     utils.ReadOnlyVerbs(),
		},
	}
	if SupportV1 {
		res = AppendPolicyRules(res, rbacv1.PolicyRule{
			APIGroups: []string{OpenMCPV1ApiGroup},
			Resources: []string{"managedcontrolplanes", "clusteradmins"},
		})
	}
	return res
}

func BuiltinPermissibleWorkspaceResourcesAdminOnly() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{corev1.GroupName},
			Resources: []string{"serviceaccounts/token"},
			Verbs:     []string{"create"},
		},
	}
}

// AppendPolicyRules appends the given elements to the list and returns the new list.
// If there is already an entry with the same apiGroups, the resources are merged.
// Otherwise, a new entry is appended.
func AppendPolicyRules(l []rbacv1.PolicyRule, elems ...rbacv1.PolicyRule) []rbacv1.PolicyRule {
	for _, elem := range elems {
		found := false
		for i, existing := range l {
			apiGroupsA := sets.New(existing.APIGroups...)
			apiGroupsB := sets.New(elem.APIGroups...)
			verbsA := sets.New(existing.Verbs...)
			verbsB := sets.New(elem.Verbs...)
			if apiGroupsA.Equal(apiGroupsB) && verbsA.Equal(verbsB) {
				found = true
				uniqueness := sets.New(existing.Resources...)
				for _, res := range elem.Resources {
					if !uniqueness.Has(res) {
						existing.Resources = append(existing.Resources, res)
						uniqueness.Insert(res)
					}
				}
				l[i] = existing
				break
			}
		}
		if !found {
			l = append(l, elem)
		}
	}
	return l
}

// InjectMissingVerbs takes a role and a list of rbac policy rules.
// Each policy rule which is missing 'verbs' will have the default verbs for the given role injected.
// The policy rules are modified in-place.
func InjectMissingVerbs(roleID string, permissibleResources []rbacv1.PolicyRule) error {
	var verbGenerator func() []string
	switch roleID {
	case utils.AdminRoleID:
		verbGenerator = utils.AllVerbs
	case utils.ViewerRoleID:
		verbGenerator = utils.ReadOnlyVerbs
	default:
		return fmt.Errorf("unknown role ID: %s", roleID)
	}

	for i := range permissibleResources {
		agr := &permissibleResources[i]
		if agr.Verbs == nil {
			agr.Verbs = verbGenerator()
		}
	}

	return nil
}
