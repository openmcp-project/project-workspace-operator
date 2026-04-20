package config_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/controller-utils/pkg/collections"
	testutils "github.com/openmcp-project/controller-utils/pkg/testing"
	clustersv1alpha1 "github.com/openmcp-project/openmcp-operator/api/clusters/v1alpha1"
	commonapi "github.com/openmcp-project/openmcp-operator/api/common"
	openmcpcorev2alpha1 "github.com/openmcp-project/openmcp-operator/api/core/v2alpha1"
	providerv1alpha1 "github.com/openmcp-project/openmcp-operator/api/provider/v1alpha1"
	"github.com/openmcp-project/openmcp-operator/lib/clusteraccess/advanced"

	pwv1alpha1 "github.com/openmcp-project/platform-service-project-workspace/api/core/v1alpha1"
	"github.com/openmcp-project/platform-service-project-workspace/api/install"
	sharedconfig "github.com/openmcp-project/platform-service-project-workspace/internal/controller/config"
	"github.com/openmcp-project/platform-service-project-workspace/internal/utils"
)

const (
	platformClusterID   = "platform"
	onboardingClusterID = "onboarding"

	pwcRec       = "projectworkspaceconfig-controller"
	providerName = "project-workspace"
	podNamespace = "openmcp-system"
)

var onboardingScheme = install.InstallOperatorAPIsOnboarding(runtime.NewScheme())
var platformScheme = install.InstallOperatorAPIsPlatform(runtime.NewScheme())

// This is for the builtin deletion blocking resources
var alwaysKnownAPIResources = []*metav1.APIResourceList{
	{
		GroupVersion: pwv1alpha1.GroupVersion.String(),
		APIResources: []metav1.APIResource{
			{
				Name:       "workspaces",
				Group:      pwv1alpha1.GroupVersion.Group,
				Version:    pwv1alpha1.GroupVersion.Version,
				Kind:       "Workspace",
				Namespaced: true,
			},
		},
	},
	{
		GroupVersion: openmcpcorev2alpha1.GroupVersion.String(),
		APIResources: []metav1.APIResource{
			{
				Name:       "managedcontrolplanev2s",
				Group:      openmcpcorev2alpha1.GroupVersion.Group,
				Version:    openmcpcorev2alpha1.GroupVersion.Version,
				Kind:       "ManagedControlPlaneV2",
				Namespaced: true,
			},
		},
	},
}
var alwaysExpectedDynamicAccessPermissions = []rbacv1.PolicyRule{
	{
		APIGroups: []string{sharedconfig.OpenMCPV1ApiGroup},
		Resources: []string{
			"workspaces",
			"workspaces/status",
			"managedcontrolplanev2s",
			"managedcontrolplanev2s/status",
		},
		Verbs: utils.ReadOnlyVerbs(),
	},
}

func defaultTestSetup(testDirPath string, knownAPIResources ...*metav1.APIResourceList) (*sharedconfig.PWOConfigController, *testutils.ComplexEnvironment) {
	envb := testutils.NewComplexEnvironmentBuilder().
		WithFakeClient(platformClusterID, platformScheme).
		WithFakeClient(onboardingClusterID, onboardingScheme)
	platformDirPath := filepath.Join(testDirPath, "platform")
	pdi, err := os.Stat(platformDirPath)
	if err != nil {
		if !os.IsNotExist(err) {
			panic(err)
		}
	} else {
		if !pdi.IsDir() {
			panic(fmt.Sprintf("expected platform test dir '%s' to be a directory", platformDirPath))
		}
		envb = envb.WithInitObjectPath(platformClusterID, platformDirPath)
	}
	onboardingDirPath := filepath.Join(testDirPath, "onboarding")
	odi, err := os.Stat(onboardingDirPath)
	if err != nil {
		if !os.IsNotExist(err) {
			panic(err)
		}
	} else {
		if !odi.IsDir() {
			panic(fmt.Sprintf("expected onboarding test dir '%s' to be a directory", onboardingDirPath))
		}
		envb = envb.WithInitObjectPath(onboardingClusterID, onboardingDirPath)
	}
	env := envb.
		WithInitObjects(platformClusterID, &clustersv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "onboarding",
				Namespace: "default",
			},
		}).
		WithDynamicObjectsWithStatus(platformClusterID, &clustersv1alpha1.AccessRequest{}).
		WithReconcilerConstructor(pwcRec, func(c ...client.Client) reconcile.Reconciler {
			pwc, err := sharedconfig.NewPWOConfigController(providerName, clusters.NewTestClusterFromClient(platformClusterID, c[0]), clusters.NewTestClusterFromClient(onboardingClusterID, c[1]), &commonapi.ObjectReference{Name: "onboarding", Namespace: "default"}, nil, podNamespace)
			Expect(err).ToNot(HaveOccurred(), "failed to create PWOConfigController")
			pwc.Car.WithFakingCallback(advanced.FakingCallback_WaitingForAccessRequestReadiness, advanced.FakeAccessRequestReadiness())
			pwc.Car.WithFakingCallback(advanced.FakingCallback_WaitingForAccessRequestDeletion, advanced.FakeAccessRequestDeletion([]string{"clusterprovider"}, nil))
			pwc.Car.WithFakeClientGenerator(func(ctx context.Context, kcfgData []byte, scheme *runtime.Scheme, additionalData ...any) (client.Client, error) {
				// this controller creates AccessRequests only for the onboarding cluster
				// and the permissions are hard to test in unit tests anyway, so let's just return the static onboarding cluster client
				return pwc.OnboardingClusterAccessStatic.Client(), nil
			})
			fd := fakeclientset.NewClientset().Discovery().(*fakediscovery.FakeDiscovery)
			fd.Resources = knownAPIResources
			for _, akrl := range alwaysKnownAPIResources {
				found := false
				for _, krl := range fd.Resources {
					if krl.GroupVersion == akrl.GroupVersion {
						krl.APIResources = append(krl.APIResources, akrl.APIResources...)
						found = true
						break
					}
				}
				if !found {
					fd.Resources = append(fd.Resources, akrl)
				}
			}
			pwc.DiscoveryService = fd
			return pwc
		}, platformClusterID, onboardingClusterID).
		Build()

	pwc, ok := env.Reconciler(pwcRec).(*sharedconfig.PWOConfigController)
	Expect(ok).To(BeTrue(), "Reconciler is not of type *PWOConfigController")

	return pwc, env
}

func sortPolicyRuleFields(rules []rbacv1.PolicyRule) []rbacv1.PolicyRule {
	res := make([]rbacv1.PolicyRule, len(rules))
	for i := range rules {
		sortedRule := rules[i].DeepCopy()
		slices.Sort(sortedRule.APIGroups)
		slices.Sort(sortedRule.NonResourceURLs)
		slices.Sort(sortedRule.Resources)
		slices.Sort(sortedRule.ResourceNames)
		slices.Sort(sortedRule.Verbs)
		res[i] = *sortedRule
	}
	return res
}

type expectedValues struct {
	resourcesBlockingProjectDeletion   []sharedconfig.DeletionBlockingResource
	resourcesBlockingWorkspaceDeletion []sharedconfig.DeletionBlockingResource
	projectPermissionsPerRole          map[pwv1alpha1.ProjectMemberRole][]rbacv1.PolicyRule
	workspacePermissionsPerRole        map[pwv1alpha1.WorkspaceMemberRole][]rbacv1.PolicyRule
	dynamicAccessPermissions           []rbacv1.PolicyRule
}

func (expected *expectedValues) validate(env *testutils.ComplexEnvironment, pwc *sharedconfig.PWOConfigController) {
	req := testutils.RequestFromStrings(providerName)
	EventuallyWithOffset(1, env.ShouldReconcile).WithArguments(pwcRec, req).Should(WithTransform(func(rr reconcile.Result) time.Duration { return rr.RequeueAfter }, BeZero()))

	_, err := pwc.OnboardingClusterStatic(env.Ctx)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	_, err = pwc.OnboardingClusterDynamic(env.Ctx)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	actualResourcesBlockingProjectDeletion, err := pwc.ResourcesBlockingProjectDeletion(env.Ctx)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, actualResourcesBlockingProjectDeletion).To(ConsistOf(expected.resourcesBlockingProjectDeletion), "resources blocking project deletion do not match")

	actualResourcesBlockingWorkspaceDeletion, err := pwc.ResourcesBlockingWorkspaceDeletion(env.Ctx)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, actualResourcesBlockingWorkspaceDeletion).To(ConsistOf(expected.resourcesBlockingWorkspaceDeletion), "resources blocking workspace deletion do not match")

	for role := range utils.ProjectRolesWithVerbs() {
		cr := &rbacv1.ClusterRole{}
		cr.Name = utils.ClusterRoleForRole(role)
		ExpectWithOffset(1, env.Client(onboardingClusterID).Get(env.Ctx, client.ObjectKeyFromObject(cr), cr)).To(Succeed())
		expectedRules := sortPolicyRuleFields(expected.projectPermissionsPerRole[role])
		ExpectWithOffset(1, cr.Rules).To(WithTransform(sortPolicyRuleFields, ConsistOf(expectedRules)))
	}
	for role := range utils.WorkspaceRolesWithVerbs() {
		cr := &rbacv1.ClusterRole{}
		cr.Name = utils.ClusterRoleForRole(role)
		ExpectWithOffset(1, env.Client(onboardingClusterID).Get(env.Ctx, client.ObjectKeyFromObject(cr), cr)).To(Succeed())
		expectedRules := sortPolicyRuleFields(expected.workspacePermissionsPerRole[role])
		ExpectWithOffset(1, cr.Rules).To(WithTransform(sortPolicyRuleFields, ConsistOf(expectedRules)))
	}

	ar, err := pwc.Car.AccessRequest(env.Ctx, req, sharedconfig.ClusterIDOnboardingDynamic)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, ar.Namespace).To(Equal(podNamespace))
	effectiveDynamicAccessPermissions := make([]rbacv1.PolicyRule, 0, len(alwaysExpectedDynamicAccessPermissions)+len(expected.dynamicAccessPermissions))
	effectiveDynamicAccessPermissions = append(effectiveDynamicAccessPermissions, alwaysExpectedDynamicAccessPermissions...)
	effectiveDynamicAccessPermissions = append(effectiveDynamicAccessPermissions, expected.dynamicAccessPermissions...)
	// verify that each requested permission is actually in the AccessRequest
	// order and grouping will be ignored
	for _, expectedRule := range effectiveDynamicAccessPermissions {
		for _, expectedApiGroup := range expectedRule.APIGroups {
			for _, expectedResource := range expectedRule.Resources {
				// verify that the AccessRequest contains permissions for this resource with this apigroup with the expected verbs
				ExpectWithOffset(1, ar.Spec.Token.Permissions).To(ContainElement(WithTransform(func(pr clustersv1alpha1.PermissionsRequest) []rbacv1.PolicyRule {
					return pr.Rules
				}, ContainElement(MatchFields(IgnoreExtras, Fields{
					"APIGroups": ContainElement(expectedApiGroup),
					"Resources": ContainElement(expectedResource),
					"Verbs":     ConsistOf(expectedRule.Verbs),
				})))))
			}
		}
	}
}

func (exp *expectedValues) clone() *expectedValues {
	res := &expectedValues{}

	res.resourcesBlockingProjectDeletion = make([]sharedconfig.DeletionBlockingResource, len(exp.resourcesBlockingProjectDeletion))
	for i := range exp.resourcesBlockingProjectDeletion {
		res.resourcesBlockingProjectDeletion[i] = *exp.resourcesBlockingProjectDeletion[i].DeepCopy()
	}

	res.resourcesBlockingWorkspaceDeletion = make([]sharedconfig.DeletionBlockingResource, len(exp.resourcesBlockingWorkspaceDeletion))
	for i := range exp.resourcesBlockingWorkspaceDeletion {
		res.resourcesBlockingWorkspaceDeletion[i] = *exp.resourcesBlockingWorkspaceDeletion[i].DeepCopy()
	}

	res.projectPermissionsPerRole = make(map[pwv1alpha1.ProjectMemberRole][]rbacv1.PolicyRule, len(exp.projectPermissionsPerRole))
	for role, rules := range exp.projectPermissionsPerRole {
		res.projectPermissionsPerRole[role] = make([]rbacv1.PolicyRule, len(rules))
		for i := range rules {
			res.projectPermissionsPerRole[role][i] = *rules[i].DeepCopy()
		}
	}

	res.workspacePermissionsPerRole = make(map[pwv1alpha1.WorkspaceMemberRole][]rbacv1.PolicyRule, len(exp.workspacePermissionsPerRole))
	for role, rules := range exp.workspacePermissionsPerRole {
		res.workspacePermissionsPerRole[role] = make([]rbacv1.PolicyRule, len(rules))
		for i := range rules {
			res.workspacePermissionsPerRole[role][i] = *rules[i].DeepCopy()
		}
	}

	res.dynamicAccessPermissions = make([]rbacv1.PolicyRule, len(exp.dynamicAccessPermissions))
	for i := range exp.dynamicAccessPermissions {
		res.dynamicAccessPermissions[i] = *exp.dynamicAccessPermissions[i].DeepCopy()
	}

	return res
}

func defaultProjectPermissionsPerRole() map[pwv1alpha1.ProjectMemberRole][]rbacv1.PolicyRule {
	return map[pwv1alpha1.ProjectMemberRole][]rbacv1.PolicyRule{
		pwv1alpha1.ProjectRoleAdmin: {
			{
				APIGroups: []string{pwv1alpha1.GroupVersion.String()},
				Resources: []string{"workspaces"},
				Verbs:     utils.AllVerbs(),
			},
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{"secrets", "serviceaccounts"},
				Verbs:     utils.AllVerbs(),
			},
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{"pods"},
				Verbs:     []string{"list"},
			},
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{"resourcequotas"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{"serviceaccounts/token"},
				Verbs:     []string{"create"},
			},
		},
		pwv1alpha1.ProjectRoleView: {
			{
				APIGroups: []string{pwv1alpha1.GroupVersion.String()},
				Resources: []string{"workspaces"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{"serviceaccounts"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{"pods"},
				Verbs:     []string{"list"},
			},
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{"resourcequotas"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
		},
	}
}

func defaultWorkspacePermissionsPerRole() map[pwv1alpha1.WorkspaceMemberRole][]rbacv1.PolicyRule {
	return map[pwv1alpha1.WorkspaceMemberRole][]rbacv1.PolicyRule{
		pwv1alpha1.WorkspaceRoleAdmin: {
			{
				APIGroups: []string{openmcpcorev2alpha1.GroupVersion.String()},
				Resources: []string{"managedcontrolplanev2s"},
				Verbs:     utils.AllVerbs(),
			},
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{
					"secrets",
					"configmaps",
					"serviceaccounts",
				},
				Verbs: utils.AllVerbs(),
			},
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{"pods"},
				Verbs:     []string{"list"},
			},
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{"resourcequotas"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{"serviceaccounts/token"},
				Verbs:     []string{"create"},
			},
		},
		pwv1alpha1.WorkspaceRoleView: {
			{
				APIGroups: []string{openmcpcorev2alpha1.GroupVersion.String()},
				Resources: []string{"managedcontrolplanev2s"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{
					"secrets",
					"configmaps",
					"serviceaccounts",
				},
				Verbs: utils.ReadOnlyVerbs(),
			},
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{"pods"},
				Verbs:     []string{"list"},
			},
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{"resourcequotas"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
		},
	}
}

var _ = Describe("ProjectWorkspaceConfig Controller Test", Serial, func() {

	BeforeEach(func() {
		sharedconfig.SupportV1 = false
	})

	It("should return default values for an empty config and no ServiceProviders", func() {
		pwc, env := defaultTestSetup(filepath.Join("testdata", "test-01"))

		expected := &expectedValues{}

		expected.resourcesBlockingProjectDeletion = sharedconfig.BuiltinResourcesBlockingProjectDeletion()
		expected.resourcesBlockingWorkspaceDeletion = sharedconfig.BuiltinResourcesBlockingWorkspaceDeletion()

		expected.projectPermissionsPerRole = defaultProjectPermissionsPerRole()
		expected.workspacePermissionsPerRole = defaultWorkspacePermissionsPerRole()

		expected.validate(env, pwc)
	})

	It("should add the v1 resources, if v1 support is enabled", func() {
		sharedconfig.SupportV1 = true
		pwc, env := defaultTestSetup(filepath.Join("testdata", "test-01"), &metav1.APIResourceList{
			GroupVersion: sharedconfig.OpenMCPV1ApiGroup + "/" + sharedconfig.OpenMCPV1ApiVersion,
			APIResources: []metav1.APIResource{
				{
					Name:       "managedcontrolplanes",
					Group:      sharedconfig.OpenMCPV1ApiGroup,
					Version:    sharedconfig.OpenMCPV1ApiVersion,
					Kind:       "ManagedControlPlane",
					Namespaced: true,
				},
				{
					Name:       "clusteradmins",
					Group:      sharedconfig.OpenMCPV1ApiGroup,
					Version:    sharedconfig.OpenMCPV1ApiVersion,
					Kind:       "ClusterAdmin",
					Namespaced: true,
				},
			},
		})

		expected := &expectedValues{}

		expected.resourcesBlockingProjectDeletion = sharedconfig.BuiltinResourcesBlockingProjectDeletion()
		expected.resourcesBlockingWorkspaceDeletion = sharedconfig.BuiltinResourcesBlockingWorkspaceDeletion()
		// verify that the v1 resources are included
		Expect(expected.resourcesBlockingWorkspaceDeletion).To(ContainElements(
			sharedconfig.DeletionBlockingResource{
				GroupVersionKind: metav1.GroupVersionKind{
					Group:   sharedconfig.OpenMCPV1ApiGroup,
					Version: sharedconfig.OpenMCPV1ApiVersion,
					Kind:    "ManagedControlPlane",
				},
				Source: pwv1alpha1.SourceBuiltin,
			},
			sharedconfig.DeletionBlockingResource{
				GroupVersionKind: metav1.GroupVersionKind{
					Group:   sharedconfig.OpenMCPV1ApiGroup,
					Version: sharedconfig.OpenMCPV1ApiVersion,
					Kind:    "ClusterAdmin",
				},
				Source: pwv1alpha1.SourceBuiltin,
			},
		))

		expected.projectPermissionsPerRole = defaultProjectPermissionsPerRole()
		expected.workspacePermissionsPerRole = defaultWorkspacePermissionsPerRole()
		// verify that the permissions for the v1 resources are included
		for role, verbs := range utils.WorkspaceRolesWithVerbs() {
			expected.workspacePermissionsPerRole[role] = sharedconfig.AppendPolicyRules(expected.workspacePermissionsPerRole[role],
				rbacv1.PolicyRule{
					APIGroups: []string{sharedconfig.OpenMCPV1ApiGroup},
					Resources: []string{
						"managedcontrolplanes",
						"clusteradmins",
					},
					Verbs: verbs,
				},
			)
		}

		expected.dynamicAccessPermissions = []rbacv1.PolicyRule{
			{
				APIGroups: []string{sharedconfig.OpenMCPV1ApiGroup},
				Resources: []string{
					"managedcontrolplanes",
					"managedcontrolplanes/status",
					"clusteradmins",
					"clusteradmins/status",
				},
				Verbs: utils.ReadOnlyVerbs(),
			},
		}

		expected.validate(env, pwc)
	})

	It("should correctly handle empty config with ServiceProviders", func() {
		pwc, env := defaultTestSetup(filepath.Join("testdata", "test-02"), &metav1.APIResourceList{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{
					Name:       "services",
					Group:      "",
					Version:    "v1",
					Kind:       "Service",
					Namespaced: true,
				},
				{
					Name:       "pods",
					Group:      "",
					Version:    "v1",
					Kind:       "Pod",
					Namespaced: true,
				},
			},
		})

		expected := &expectedValues{}

		expected.resourcesBlockingProjectDeletion = sharedconfig.BuiltinResourcesBlockingProjectDeletion()
		expected.resourcesBlockingWorkspaceDeletion = append([]sharedconfig.DeletionBlockingResource{}, sharedconfig.BuiltinResourcesBlockingWorkspaceDeletion()...)
		expected.resourcesBlockingWorkspaceDeletion = append(expected.resourcesBlockingWorkspaceDeletion,
			sharedconfig.DeletionBlockingResource{
				GroupVersionKind: metav1.GroupVersionKind{
					Group:   "",
					Kind:    "Service",
					Version: "v1",
				},
				Source: fmt.Sprintf("%s[%s]", pwv1alpha1.SourceServiceProviderPrefix, "dummy-1"),
			},
			sharedconfig.DeletionBlockingResource{
				GroupVersionKind: metav1.GroupVersionKind{
					Group:   "",
					Kind:    "Pod",
					Version: "v1",
				},
				Source: fmt.Sprintf("%s[%s]", pwv1alpha1.SourceServiceProviderPrefix, "dummy-2"),
			},
		)

		expected.projectPermissionsPerRole = defaultProjectPermissionsPerRole()
		expected.workspacePermissionsPerRole = defaultWorkspacePermissionsPerRole()
		expected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleAdmin] = sharedconfig.AppendPolicyRules(expected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleAdmin],
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"services", "pods"},
				Verbs:     utils.AllVerbs(),
			},
		)
		expected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleView] = sharedconfig.AppendPolicyRules(expected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleView],
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"services", "pods"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
		)

		expected.dynamicAccessPermissions = []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"services", "services/status", "pods", "pods/status"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
		}

		expected.validate(env, pwc)
	})

	It("should correctly handle non-empty config without ServiceProviders", func() {
		pwc, env := defaultTestSetup(filepath.Join("testdata", "test-03"), &metav1.APIResourceList{
			GroupVersion: "mygroup.project/v1alpha1",
			APIResources: []metav1.APIResource{
				{
					Name:       "myprojectblockingresources",
					Group:      "mygroup.project",
					Version:    "v1",
					Kind:       "MyProjectBlockingResource",
					Namespaced: true,
				},
				{
					Name:       "myprojectblockingresources/status",
					Group:      "mygroup.project",
					Version:    "v1",
					Kind:       "MyProjectBlockingResource",
					Namespaced: true,
				},
			},
		}, &metav1.APIResourceList{
			GroupVersion: "mygroup.workspace/v1alpha1",
			APIResources: []metav1.APIResource{
				{
					Name:       "myworkspaceblockingresources1",
					Group:      "mygroup.workspace",
					Version:    "v1alpha1",
					Kind:       "MyWorkspaceBlockingResource1",
					Namespaced: true,
				},
				{
					Name:       "myworkspaceblockingresources1/status",
					Group:      "mygroup.workspace",
					Version:    "v1alpha1",
					Kind:       "MyWorkspaceBlockingResource1",
					Namespaced: true,
				},
				{
					Name:       "myworkspaceblockingresources2",
					Group:      "mygroup.workspace",
					Version:    "v1alpha1",
					Kind:       "MyWorkspaceBlockingResource2",
					Namespaced: true,
				},
				{
					Name:       "myworkspaceblockingresources2/status",
					Group:      "mygroup.workspace",
					Version:    "v1alpha1",
					Kind:       "MyWorkspaceBlockingResource2",
					Namespaced: true,
				},
			},
		})

		expected := &expectedValues{}

		cfg := &pwv1alpha1.ProjectWorkspaceConfig{}
		err := env.Client(platformClusterID).Get(env.Ctx, client.ObjectKey{Name: providerName}, cfg)
		Expect(err).ToNot(HaveOccurred())

		expected.resourcesBlockingProjectDeletion = append([]sharedconfig.DeletionBlockingResource{}, sharedconfig.BuiltinResourcesBlockingProjectDeletion()...)
		expected.resourcesBlockingProjectDeletion = append(expected.resourcesBlockingProjectDeletion, collections.ProjectSliceToSlice(cfg.Spec.Project.ResourcesBlockingDeletion, func(gvk metav1.GroupVersionKind) sharedconfig.DeletionBlockingResource {
			return sharedconfig.DeletionBlockingResource{
				GroupVersionKind: gvk,
				Source:           pwv1alpha1.SourceProjectWorkspaceConfig,
			}
		})...)

		expected.resourcesBlockingWorkspaceDeletion = append([]sharedconfig.DeletionBlockingResource{}, sharedconfig.BuiltinResourcesBlockingWorkspaceDeletion()...)
		expected.resourcesBlockingWorkspaceDeletion = append(expected.resourcesBlockingWorkspaceDeletion, collections.ProjectSliceToSlice(cfg.Spec.Workspace.ResourcesBlockingDeletion, func(gvk metav1.GroupVersionKind) sharedconfig.DeletionBlockingResource {
			return sharedconfig.DeletionBlockingResource{
				GroupVersionKind: gvk,
				Source:           pwv1alpha1.SourceProjectWorkspaceConfig,
			}
		})...)

		expected.projectPermissionsPerRole = defaultProjectPermissionsPerRole()
		expected.projectPermissionsPerRole[pwv1alpha1.ProjectRoleAdmin] = sharedconfig.AppendPolicyRules(expected.projectPermissionsPerRole[pwv1alpha1.ProjectRoleAdmin],
			rbacv1.PolicyRule{
				APIGroups: []string{"mygroup.project"},
				Resources: []string{"myprojectadditionalresources1", "myprojectadditionalresources2"},
				Verbs:     []string{"get", "update"},
			},
		)
		expected.projectPermissionsPerRole[pwv1alpha1.ProjectRoleView] = sharedconfig.AppendPolicyRules(expected.projectPermissionsPerRole[pwv1alpha1.ProjectRoleView],
			rbacv1.PolicyRule{
				APIGroups: []string{"mygroup.project"},
				Resources: []string{"myprojectadditionalresources1"},
				Verbs:     []string{"get", "update"},
			},
		)
		expected.workspacePermissionsPerRole = defaultWorkspacePermissionsPerRole()
		expected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleAdmin] = sharedconfig.AppendPolicyRules(expected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleAdmin],
			rbacv1.PolicyRule{
				APIGroups: []string{"mygroup.workspace"},
				Resources: []string{"myworkspaceadditionalresources1", "myworkspaceadditionalresources2"},
				Verbs:     []string{"*"},
			},
		)
		expected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleView] = sharedconfig.AppendPolicyRules(expected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleView],
			rbacv1.PolicyRule{
				APIGroups: []string{"mygroup.workspace"},
				Resources: []string{"myworkspaceadditionalresources1", "myworkspaceadditionalresources2"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
		)

		expected.dynamicAccessPermissions = []rbacv1.PolicyRule{
			{
				APIGroups: []string{"mygroup.project"},
				Resources: []string{"myprojectblockingresources", "myprojectblockingresources/status"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
			{
				APIGroups: []string{"mygroup.workspace"},
				Resources: []string{"myworkspaceblockingresources1", "myworkspaceblockingresources1/status", "myworkspaceblockingresources2", "myworkspaceblockingresources2/status"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
		}

		expected.validate(env, pwc)
	})

	It("should correctly handle non-empty config with ServiceProviders and handle updates correctly", func() {
		pwc, env := defaultTestSetup(filepath.Join("testdata", "test-04"), &metav1.APIResourceList{
			GroupVersion: "mygroup.project/v1alpha1",
			APIResources: []metav1.APIResource{
				{
					Name:       "myprojectblockingresources",
					Group:      "mygroup.project",
					Version:    "v1",
					Kind:       "MyProjectBlockingResource",
					Namespaced: true,
				},
				{
					Name:       "myprojectblockingresources/status",
					Group:      "mygroup.project",
					Version:    "v1",
					Kind:       "MyProjectBlockingResource",
					Namespaced: true,
				},
			},
		}, &metav1.APIResourceList{
			GroupVersion: "mygroup.workspace/v1alpha1",
			APIResources: []metav1.APIResource{
				{
					Name:       "myworkspaceblockingresources1",
					Group:      "mygroup.workspace",
					Version:    "v1alpha1",
					Kind:       "MyWorkspaceBlockingResource1",
					Namespaced: true,
				},
				{
					Name:       "myworkspaceblockingresources1/status",
					Group:      "mygroup.workspace",
					Version:    "v1alpha1",
					Kind:       "MyWorkspaceBlockingResource1",
					Namespaced: true,
				},
				{
					Name:       "myworkspaceblockingresources2",
					Group:      "mygroup.workspace",
					Version:    "v1alpha1",
					Kind:       "MyWorkspaceBlockingResource2",
					Namespaced: true,
				},
				{
					Name:       "myworkspaceblockingresources2/status",
					Group:      "mygroup.workspace",
					Version:    "v1alpha1",
					Kind:       "MyWorkspaceBlockingResource2",
					Namespaced: true,
				},
			},
		}, &metav1.APIResourceList{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{
					Name:       "services",
					Group:      "",
					Version:    "v1",
					Kind:       "Service",
					Namespaced: true,
				},
				{
					Name:       "pods",
					Group:      "",
					Version:    "v1",
					Kind:       "Pod",
					Namespaced: true,
				},
			},
		})

		cfg := &pwv1alpha1.ProjectWorkspaceConfig{}
		err := env.Client(platformClusterID).Get(env.Ctx, client.ObjectKey{Name: providerName}, cfg)
		Expect(err).ToNot(HaveOccurred())

		originallyExpected := &expectedValues{}

		originallyExpected.resourcesBlockingProjectDeletion = append([]sharedconfig.DeletionBlockingResource{}, sharedconfig.BuiltinResourcesBlockingProjectDeletion()...)
		originallyExpected.resourcesBlockingProjectDeletion = append(originallyExpected.resourcesBlockingProjectDeletion, collections.ProjectSliceToSlice(cfg.Spec.Project.ResourcesBlockingDeletion, func(gvk metav1.GroupVersionKind) sharedconfig.DeletionBlockingResource {
			return sharedconfig.DeletionBlockingResource{
				GroupVersionKind: gvk,
				Source:           pwv1alpha1.SourceProjectWorkspaceConfig,
			}
		})...)

		originallyExpected.resourcesBlockingWorkspaceDeletion = append([]sharedconfig.DeletionBlockingResource{}, sharedconfig.BuiltinResourcesBlockingWorkspaceDeletion()...)
		originallyExpected.resourcesBlockingWorkspaceDeletion = append(originallyExpected.resourcesBlockingWorkspaceDeletion, collections.ProjectSliceToSlice(cfg.Spec.Workspace.ResourcesBlockingDeletion, func(gvk metav1.GroupVersionKind) sharedconfig.DeletionBlockingResource {
			return sharedconfig.DeletionBlockingResource{
				GroupVersionKind: gvk,
				Source:           pwv1alpha1.SourceProjectWorkspaceConfig,
			}
		})...)
		originallyExpected.resourcesBlockingWorkspaceDeletion = append(originallyExpected.resourcesBlockingWorkspaceDeletion,
			sharedconfig.DeletionBlockingResource{
				GroupVersionKind: metav1.GroupVersionKind{
					Group:   "",
					Kind:    "Service",
					Version: "v1",
				},
				Source: fmt.Sprintf("%s[%s]", pwv1alpha1.SourceServiceProviderPrefix, "dummy-1"),
			},
		)

		originallyExpected.projectPermissionsPerRole = defaultProjectPermissionsPerRole()
		originallyExpected.projectPermissionsPerRole[pwv1alpha1.ProjectRoleAdmin] = sharedconfig.AppendPolicyRules(originallyExpected.projectPermissionsPerRole[pwv1alpha1.ProjectRoleAdmin],
			rbacv1.PolicyRule{
				APIGroups: []string{"mygroup.project"},
				Resources: []string{"myprojectadditionalresources1", "myprojectadditionalresources2"},
				Verbs:     []string{"get", "update"},
			},
		)
		originallyExpected.workspacePermissionsPerRole = defaultWorkspacePermissionsPerRole()
		originallyExpected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleAdmin] = sharedconfig.AppendPolicyRules(originallyExpected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleAdmin],
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"services"},
				Verbs:     utils.AllVerbs(),
			},
			rbacv1.PolicyRule{
				APIGroups: []string{"mygroup.workspace"},
				Resources: []string{"myworkspaceadditionalresources1", "myworkspaceadditionalresources2"},
				Verbs:     []string{"*"},
			},
		)
		originallyExpected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleView] = sharedconfig.AppendPolicyRules(originallyExpected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleView],
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"services"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
		)

		originallyExpected.dynamicAccessPermissions = []rbacv1.PolicyRule{
			{
				APIGroups: []string{"mygroup.project"},
				Resources: []string{"myprojectblockingresources", "myprojectblockingresources/status"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
			{
				APIGroups: []string{"mygroup.workspace"},
				Resources: []string{"myworkspaceblockingresources1", "myworkspaceblockingresources1/status", "myworkspaceblockingresources2", "myworkspaceblockingresources2/status"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
			{
				APIGroups: []string{""},
				Resources: []string{"services", "services/status"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
		}

		originallyExpected.validate(env, pwc)

		expected := originallyExpected.clone()
		// add a status with a resource to a ServiceProvider
		sp2 := &providerv1alpha1.ServiceProvider{}
		sp2.Name = "dummy-2"
		Expect(env.Client(platformClusterID).Get(env.Ctx, client.ObjectKeyFromObject(sp2), sp2)).To(Succeed())
		sp2.Status.Resources = []metav1.GroupVersionKind{
			{
				Group:   "",
				Version: "v1",
				Kind:    "Pod",
			},
		}
		Expect(env.Client(platformClusterID).Status().Update(env.Ctx, sp2)).To(Succeed())
		// this should add the resource to resources that block workspace deletion
		// and grant workspace viewer and admin permissions
		// as well as modify the AccessRequest to contain get permissions for this resource
		expected.resourcesBlockingWorkspaceDeletion = append(expected.resourcesBlockingWorkspaceDeletion, sharedconfig.DeletionBlockingResource{
			GroupVersionKind: sp2.Status.Resources[0],
			Source:           fmt.Sprintf("%s[%s]", pwv1alpha1.SourceServiceProviderPrefix, sp2.Name),
		})
		expected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleAdmin] = sharedconfig.AppendPolicyRules(expected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleAdmin],
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     utils.AllVerbs(),
			},
		)
		expected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleView] = sharedconfig.AppendPolicyRules(expected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleView],
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
		)

		expected.dynamicAccessPermissions = append(expected.dynamicAccessPermissions, rbacv1.PolicyRule{
			APIGroups: []string{""},
			Resources: []string{"pods", "pods/status"},
			Verbs:     utils.ReadOnlyVerbs(),
		})
		expected.validate(env, pwc)

		// removing the ServiceProvider should undo that change
		Expect(env.Client(platformClusterID).Delete(env.Ctx, sp2)).To(Succeed())
		originallyExpected.validate(env, pwc)

		expected = originallyExpected.clone()
		// modifying the config by adding project and workspace viewer permissions should modify the permissions accordingly
		cfg.Spec.Project.AdditionalPermissions[pwv1alpha1.ProjectRoleView] = []rbacv1.PolicyRule{
			{
				APIGroups: []string{"mygroup.project"},
				Resources: []string{"myprojectadditionalresources1"},
				Verbs:     []string{"get", "update"},
			},
		}
		cfg.Spec.Workspace.AdditionalPermissions[pwv1alpha1.WorkspaceRoleView] = []rbacv1.PolicyRule{
			{
				APIGroups: []string{"mygroup.workspace"},
				Resources: []string{"myworkspaceadditionalresources1", "myworkspaceadditionalresources2"},
				Verbs:     utils.ReadOnlyVerbs(),
			},
		}
		Expect(env.Client(platformClusterID).Update(env.Ctx, cfg)).To(Succeed())
		expected.projectPermissionsPerRole[pwv1alpha1.ProjectRoleView] = sharedconfig.AppendPolicyRules(expected.projectPermissionsPerRole[pwv1alpha1.ProjectRoleView], cfg.Spec.Project.AdditionalPermissions[pwv1alpha1.ProjectRoleView]...)
		expected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleView] = sharedconfig.AppendPolicyRules(expected.workspacePermissionsPerRole[pwv1alpha1.WorkspaceRoleView], cfg.Spec.Workspace.AdditionalPermissions[pwv1alpha1.WorkspaceRoleView]...)
		expected.validate(env, pwc)

		// removing the additional permissions again should undo that change
		delete(cfg.Spec.Project.AdditionalPermissions, pwv1alpha1.ProjectRoleView)
		delete(cfg.Spec.Workspace.AdditionalPermissions, pwv1alpha1.WorkspaceRoleView)
		Expect(env.Client(platformClusterID).Update(env.Ctx, cfg)).To(Succeed())
		originallyExpected.validate(env, pwc)
	})

})
