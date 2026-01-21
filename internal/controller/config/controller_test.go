package config_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
	providerv1alpha1 "github.com/openmcp-project/openmcp-operator/api/provider/v1alpha1"
	"github.com/openmcp-project/openmcp-operator/lib/clusteraccess/advanced"

	pwov1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
	"github.com/openmcp-project/project-workspace-operator/api/install"
	sharedconfig "github.com/openmcp-project/project-workspace-operator/internal/controller/config"
)

const (
	platformClusterID   = "platform"
	onboardingClusterID = "onboarding"

	pwcRec       = "projectworkspaceconfig-controller"
	providerName = "project-workspace-operator"
)

var onboardingScheme = install.InstallOperatorAPIsOnboarding(runtime.NewScheme())
var platformScheme = install.InstallOperatorAPIsPlatform(runtime.NewScheme())

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
			pwc, err := sharedconfig.NewPWOConfigController(providerName, clusters.NewTestClusterFromClient(platformClusterID, c[0]), clusters.NewTestClusterFromClient(onboardingClusterID, c[1]), &commonapi.ObjectReference{Name: "onboarding", Namespace: "default"}, nil)
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
			pwc.DiscoveryService = fd
			return pwc
		}, platformClusterID, onboardingClusterID).
		Build()

	pwc, ok := env.Reconciler(pwcRec).(*sharedconfig.PWOConfigController)
	Expect(ok).To(BeTrue(), "Reconciler is not of type *PWOConfigController")

	return pwc, env
}

type expectedValues struct {
	resourcesBlockingProjectDeletion   []sharedconfig.DeletionBlockingResource
	resourcesBlockingWorkspaceDeletion []sharedconfig.DeletionBlockingResource
	projectViewerPermissions           []rbacv1.PolicyRule
	projectAdminPermissions            []rbacv1.PolicyRule
	workspaceViewerPermissions         []rbacv1.PolicyRule
	workspaceAdminPermissions          []rbacv1.PolicyRule
	dynamicAccessPermissions           *clustersv1alpha1.TokenConfig
}

func (expected *expectedValues) validate(env *testutils.ComplexEnvironment, pwc *sharedconfig.PWOConfigController) {
	req := testutils.RequestFromStrings(providerName)
	EventuallyWithOffset(1, env.ShouldReconcile).WithArguments(pwcRec, req).Should(WithTransform(func(rr reconcile.Result) time.Duration { return rr.RequeueAfter }, BeZero()))

	_, err := pwc.OnboardingClusterStatic(env.Ctx)
	Expect(err).ToNot(HaveOccurred())
	_, err = pwc.OnboardingClusterDynamic(env.Ctx)
	Expect(err).ToNot(HaveOccurred())

	actualResourcesBlockingProjectDeletion, err := pwc.ResourcesBlockingProjectDeletion(env.Ctx)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, actualResourcesBlockingProjectDeletion).To(ConsistOf(expected.resourcesBlockingProjectDeletion), "resources blocking project deletion do not match")

	actualResourcesBlockingWorkspaceDeletion, err := pwc.ResourcesBlockingWorkspaceDeletion(env.Ctx)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, actualResourcesBlockingWorkspaceDeletion).To(ConsistOf(expected.resourcesBlockingWorkspaceDeletion), "resources blocking workspace deletion do not match")

	actualProjectViewerPermissions, err := pwc.ProjectPermissionsForRole(env.Ctx, sharedconfig.ViewerRole)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, actualProjectViewerPermissions).To(ConsistOf(expected.projectViewerPermissions), "project viewer permissions do not match")

	actualProjectAdminPermissions, err := pwc.ProjectPermissionsForRole(env.Ctx, sharedconfig.AdminRole)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, actualProjectAdminPermissions).To(ConsistOf(expected.projectAdminPermissions), "project admin permissions do not match")

	actualWorkspaceViewerPermissions, err := pwc.WorkspacePermissionsForRole(env.Ctx, sharedconfig.ViewerRole)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, actualWorkspaceViewerPermissions).To(ConsistOf(expected.workspaceViewerPermissions), "workspace viewer permissions do not match")

	actualWorkspaceAdminPermissions, err := pwc.WorkspacePermissionsForRole(env.Ctx, sharedconfig.AdminRole)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, actualWorkspaceAdminPermissions).To(ConsistOf(expected.workspaceAdminPermissions), "workspace admin permissions do not match")

	ar, err := pwc.Car.AccessRequest(env.Ctx, req, sharedconfig.ClusterIDOnboardingDynamic)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, ar.Spec.Token).To(Equal(expected.dynamicAccessPermissions), "dynamic access permissions do not match")
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

	res.projectViewerPermissions = make([]rbacv1.PolicyRule, len(exp.projectViewerPermissions))
	for i := range exp.projectViewerPermissions {
		res.projectViewerPermissions[i] = *exp.projectViewerPermissions[i].DeepCopy()
	}

	res.projectAdminPermissions = make([]rbacv1.PolicyRule, len(exp.projectAdminPermissions))
	for i := range exp.projectAdminPermissions {
		res.projectAdminPermissions[i] = *exp.projectAdminPermissions[i].DeepCopy()
	}

	res.workspaceViewerPermissions = make([]rbacv1.PolicyRule, len(exp.workspaceViewerPermissions))
	for i := range exp.workspaceViewerPermissions {
		res.workspaceViewerPermissions[i] = *exp.workspaceViewerPermissions[i].DeepCopy()
	}

	res.workspaceAdminPermissions = make([]rbacv1.PolicyRule, len(exp.workspaceAdminPermissions))
	for i := range exp.workspaceAdminPermissions {
		res.workspaceAdminPermissions[i] = *exp.workspaceAdminPermissions[i].DeepCopy()
	}

	res.dynamicAccessPermissions = exp.dynamicAccessPermissions.DeepCopy()

	return res
}

func apiGroupsWithResourcesToPolicyRuleGenerator(verbs []string) func(sharedconfig.APIGroupsWithResources) rbacv1.PolicyRule {
	return func(elem sharedconfig.APIGroupsWithResources) rbacv1.PolicyRule {
		return rbacv1.PolicyRule{
			APIGroups: elem.APIGroups,
			Resources: elem.Resources,
			Verbs:     verbs,
		}
	}
}

var _ = Describe("ProjectWorkspaceConfig Controller Test", func() {

	It("should return default values for an empty config and no ServiceProviders", func() {
		pwc, env := defaultTestSetup(filepath.Join("testdata", "test-01"))

		expected := &expectedValues{}

		expected.resourcesBlockingProjectDeletion = sharedconfig.BuiltinResourcesBlockingProjectDeletion
		expected.resourcesBlockingWorkspaceDeletion = sharedconfig.BuiltinResourcesBlockingWorkspaceDeletion

		verbs := []string{"get", "list", "watch"}
		expected.projectViewerPermissions = collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleProjectResources, apiGroupsWithResourcesToPolicyRuleGenerator(verbs))
		expected.workspaceViewerPermissions = collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleWorkspaceResources, apiGroupsWithResourcesToPolicyRuleGenerator(verbs))
		verbs = []string{"*"}
		expected.projectAdminPermissions = collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleProjectResources, apiGroupsWithResourcesToPolicyRuleGenerator(verbs))
		expected.workspaceAdminPermissions = collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleWorkspaceResources, apiGroupsWithResourcesToPolicyRuleGenerator(verbs))

		expected.dynamicAccessPermissions = &clustersv1alpha1.TokenConfig{}

		expected.validate(env, pwc)
	})

	It("should correctly handle empty config with ServiceProviders", func() {
		pwc, env := defaultTestSetup(filepath.Join("testdata", "test-02"), &metav1.APIResourceList{
			GroupVersion: "/v1",
			APIResources: []metav1.APIResource{
				{
					Name:       "secrets",
					Group:      "",
					Version:    "v1",
					Kind:       "Secret",
					Namespaced: true,
				},
				{
					Name:       "configmaps",
					Group:      "",
					Version:    "v1",
					Kind:       "ConfigMap",
					Namespaced: true,
				},
			},
		})

		expected := &expectedValues{}

		expected.resourcesBlockingProjectDeletion = sharedconfig.BuiltinResourcesBlockingProjectDeletion
		expected.resourcesBlockingWorkspaceDeletion = append([]sharedconfig.DeletionBlockingResource{}, sharedconfig.BuiltinResourcesBlockingWorkspaceDeletion...)
		expected.resourcesBlockingWorkspaceDeletion = append(expected.resourcesBlockingWorkspaceDeletion,
			sharedconfig.DeletionBlockingResource{
				GroupVersionKind: metav1.GroupVersionKind{
					Group:   "",
					Kind:    "ConfigMap",
					Version: "v1",
				},
				Source: fmt.Sprintf("%s[%s]", pwov1alpha1.SourceServiceProviderPrefix, "dummy-1"),
			},
			sharedconfig.DeletionBlockingResource{
				GroupVersionKind: metav1.GroupVersionKind{
					Group:   "",
					Kind:    "Secret",
					Version: "v1",
				},
				Source: fmt.Sprintf("%s[%s]", pwov1alpha1.SourceServiceProviderPrefix, "dummy-2"),
			},
		)

		verbs := []string{"get", "list", "watch"}
		expected.projectViewerPermissions = collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleProjectResources, apiGroupsWithResourcesToPolicyRuleGenerator(verbs))
		expected.workspaceViewerPermissions = collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleWorkspaceResources, apiGroupsWithResourcesToPolicyRuleGenerator(verbs))
		expected.workspaceViewerPermissions = append(expected.workspaceViewerPermissions,
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"configmaps", "secrets"},
				Verbs:     verbs,
			},
		)
		verbs = []string{"*"}
		expected.projectAdminPermissions = collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleProjectResources, apiGroupsWithResourcesToPolicyRuleGenerator(verbs))
		expected.workspaceAdminPermissions = collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleWorkspaceResources, apiGroupsWithResourcesToPolicyRuleGenerator(verbs))
		expected.workspaceAdminPermissions = append(expected.workspaceAdminPermissions,
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"configmaps", "secrets"},
				Verbs:     verbs,
			},
		)

		expected.dynamicAccessPermissions = &clustersv1alpha1.TokenConfig{
			Permissions: []clustersv1alpha1.PermissionsRequest{
				{
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"configmaps", "configmaps/status", "secrets", "secrets/status"},
							Verbs:     []string{"get", "list", "watch"},
						},
					},
				},
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
					Name:       "myworkspaceblockingresources2",
					Group:      "mygroup.workspace",
					Version:    "v1alpha1",
					Kind:       "MyWorkspaceBlockingResource2",
					Namespaced: true,
				},
			},
		})

		expected := &expectedValues{}

		cfg := &pwov1alpha1.ProjectWorkspaceConfig{}
		err := env.Client(platformClusterID).Get(env.Ctx, client.ObjectKey{Name: providerName}, cfg)
		Expect(err).ToNot(HaveOccurred())

		expected.resourcesBlockingProjectDeletion = append([]sharedconfig.DeletionBlockingResource{}, sharedconfig.BuiltinResourcesBlockingProjectDeletion...)
		expected.resourcesBlockingProjectDeletion = append(expected.resourcesBlockingProjectDeletion, collections.ProjectSliceToSlice(cfg.Spec.Project.ResourcesBlockingDeletion, func(gvk metav1.GroupVersionKind) sharedconfig.DeletionBlockingResource {
			return sharedconfig.DeletionBlockingResource{
				GroupVersionKind: gvk,
				Source:           pwov1alpha1.SourceProjectWorkspaceConfig,
			}
		})...)

		expected.resourcesBlockingWorkspaceDeletion = append([]sharedconfig.DeletionBlockingResource{}, sharedconfig.BuiltinResourcesBlockingWorkspaceDeletion...)
		expected.resourcesBlockingWorkspaceDeletion = append(expected.resourcesBlockingWorkspaceDeletion, collections.ProjectSliceToSlice(cfg.Spec.Workspace.ResourcesBlockingDeletion, func(gvk metav1.GroupVersionKind) sharedconfig.DeletionBlockingResource {
			return sharedconfig.DeletionBlockingResource{
				GroupVersionKind: gvk,
				Source:           pwov1alpha1.SourceProjectWorkspaceConfig,
			}
		})...)

		verbs := []string{"get", "list", "watch"}
		expected.projectViewerPermissions = collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleProjectResources, apiGroupsWithResourcesToPolicyRuleGenerator(verbs))
		expected.projectViewerPermissions = append(expected.projectViewerPermissions,
			rbacv1.PolicyRule{
				APIGroups: []string{"mygroup.project"},
				Resources: []string{"myprojectadditionalresources1"},
				Verbs:     []string{"get", "update"},
			},
		)
		expected.workspaceViewerPermissions = collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleWorkspaceResources, apiGroupsWithResourcesToPolicyRuleGenerator(verbs))
		expected.workspaceViewerPermissions = append(expected.workspaceViewerPermissions,
			rbacv1.PolicyRule{
				APIGroups: []string{"mygroup.workspace"},
				Resources: []string{"myworkspaceadditionalresources1", "myworkspaceadditionalresources2"},
				Verbs:     []string{"list", "watch", "get"},
			},
		)

		verbs = []string{"*"}
		expected.projectAdminPermissions = collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleProjectResources, apiGroupsWithResourcesToPolicyRuleGenerator(verbs))
		expected.projectAdminPermissions = append(expected.projectAdminPermissions,
			rbacv1.PolicyRule{
				APIGroups: []string{"mygroup.project"},
				Resources: []string{"myprojectadditionalresources1", "myprojectadditionalresources2"},
				Verbs:     []string{"get", "update"},
			},
		)
		expected.workspaceAdminPermissions = collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleWorkspaceResources, apiGroupsWithResourcesToPolicyRuleGenerator(verbs))
		expected.workspaceAdminPermissions = append(expected.workspaceAdminPermissions,
			rbacv1.PolicyRule{
				APIGroups: []string{"mygroup.workspace"},
				Resources: []string{"myworkspaceadditionalresources1", "myworkspaceadditionalresources2"},
				Verbs:     verbs,
			},
		)

		expected.dynamicAccessPermissions = &clustersv1alpha1.TokenConfig{
			Permissions: []clustersv1alpha1.PermissionsRequest{
				{
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"mygroup.project"},
							Resources: []string{"myprojectblockingresources", "myprojectblockingresources/status"},
							Verbs:     []string{"get", "list", "watch"},
						},
					},
				},
				{
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"mygroup.workspace"},
							Resources: []string{"myworkspaceblockingresources1", "myworkspaceblockingresources1/status", "myworkspaceblockingresources2", "myworkspaceblockingresources2/status"},
							Verbs:     []string{"get", "list", "watch"},
						},
					},
				},
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
					Name:       "myworkspaceblockingresources2",
					Group:      "mygroup.workspace",
					Version:    "v1alpha1",
					Kind:       "MyWorkspaceBlockingResource2",
					Namespaced: true,
				},
			},
		}, &metav1.APIResourceList{
			GroupVersion: "/v1",
			APIResources: []metav1.APIResource{
				{
					Name:       "secrets",
					Group:      "",
					Version:    "v1",
					Kind:       "Secret",
					Namespaced: true,
				},
				{
					Name:       "configmaps",
					Group:      "",
					Version:    "v1",
					Kind:       "ConfigMap",
					Namespaced: true,
				},
			},
		})

		cfg := &pwov1alpha1.ProjectWorkspaceConfig{}
		err := env.Client(platformClusterID).Get(env.Ctx, client.ObjectKey{Name: providerName}, cfg)
		Expect(err).ToNot(HaveOccurred())

		originallyExpected := &expectedValues{}

		originallyExpected.resourcesBlockingProjectDeletion = append([]sharedconfig.DeletionBlockingResource{}, sharedconfig.BuiltinResourcesBlockingProjectDeletion...)
		originallyExpected.resourcesBlockingProjectDeletion = append(originallyExpected.resourcesBlockingProjectDeletion, collections.ProjectSliceToSlice(cfg.Spec.Project.ResourcesBlockingDeletion, func(gvk metav1.GroupVersionKind) sharedconfig.DeletionBlockingResource {
			return sharedconfig.DeletionBlockingResource{
				GroupVersionKind: gvk,
				Source:           pwov1alpha1.SourceProjectWorkspaceConfig,
			}
		})...)

		originallyExpected.resourcesBlockingWorkspaceDeletion = append([]sharedconfig.DeletionBlockingResource{}, sharedconfig.BuiltinResourcesBlockingWorkspaceDeletion...)
		originallyExpected.resourcesBlockingWorkspaceDeletion = append(originallyExpected.resourcesBlockingWorkspaceDeletion, collections.ProjectSliceToSlice(cfg.Spec.Workspace.ResourcesBlockingDeletion, func(gvk metav1.GroupVersionKind) sharedconfig.DeletionBlockingResource {
			return sharedconfig.DeletionBlockingResource{
				GroupVersionKind: gvk,
				Source:           pwov1alpha1.SourceProjectWorkspaceConfig,
			}
		})...)
		originallyExpected.resourcesBlockingWorkspaceDeletion = append(originallyExpected.resourcesBlockingWorkspaceDeletion,
			sharedconfig.DeletionBlockingResource{
				GroupVersionKind: metav1.GroupVersionKind{
					Group:   "",
					Kind:    "ConfigMap",
					Version: "v1",
				},
				Source: fmt.Sprintf("%s[%s]", pwov1alpha1.SourceServiceProviderPrefix, "dummy-1"),
			},
		)

		originallyExpected.projectViewerPermissions = []rbacv1.PolicyRule{}
		originallyExpected.projectViewerPermissions = append(originallyExpected.projectViewerPermissions, collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleProjectResources, func(elem sharedconfig.APIGroupsWithResources) rbacv1.PolicyRule {
			return rbacv1.PolicyRule{
				APIGroups: elem.APIGroups,
				Resources: elem.Resources,
				Verbs:     []string{"get", "list", "watch"},
			}
		})...)

		originallyExpected.projectAdminPermissions = []rbacv1.PolicyRule{
			{
				APIGroups: []string{"mygroup.project"},
				Resources: []string{"myprojectadditionalresources1", "myprojectadditionalresources2"},
				Verbs:     []string{"get", "update"},
			},
		}
		originallyExpected.projectAdminPermissions = append(originallyExpected.projectAdminPermissions, collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleProjectResources, func(elem sharedconfig.APIGroupsWithResources) rbacv1.PolicyRule {
			return rbacv1.PolicyRule{
				APIGroups: elem.APIGroups,
				Resources: elem.Resources,
				Verbs:     []string{"*"},
			}
		})...)

		originallyExpected.workspaceViewerPermissions = []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"get", "list", "watch"},
			},
		}
		originallyExpected.workspaceViewerPermissions = append(originallyExpected.workspaceViewerPermissions, collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleWorkspaceResources, func(elem sharedconfig.APIGroupsWithResources) rbacv1.PolicyRule {
			return rbacv1.PolicyRule{
				APIGroups: elem.APIGroups,
				Resources: elem.Resources,
				Verbs:     []string{"get", "list", "watch"},
			}
		})...)

		originallyExpected.workspaceAdminPermissions = []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"*"},
			},
			{
				APIGroups: []string{"mygroup.workspace"},
				Resources: []string{"myworkspaceadditionalresources1", "myworkspaceadditionalresources2"},
				Verbs:     []string{"*"},
			},
		}
		originallyExpected.workspaceAdminPermissions = append(originallyExpected.workspaceAdminPermissions, collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleWorkspaceResources, func(elem sharedconfig.APIGroupsWithResources) rbacv1.PolicyRule {
			return rbacv1.PolicyRule{
				APIGroups: elem.APIGroups,
				Resources: elem.Resources,
				Verbs:     []string{"*"},
			}
		})...)

		originallyExpected.dynamicAccessPermissions = &clustersv1alpha1.TokenConfig{
			Permissions: []clustersv1alpha1.PermissionsRequest{
				{
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"mygroup.project"},
							Resources: []string{"myprojectblockingresources", "myprojectblockingresources/status"},
							Verbs:     []string{"get", "list", "watch"},
						},
					},
				},
				{
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"mygroup.workspace"},
							Resources: []string{"myworkspaceblockingresources1", "myworkspaceblockingresources1/status", "myworkspaceblockingresources2", "myworkspaceblockingresources2/status"},
							Verbs:     []string{"get", "list", "watch"},
						},
					},
				},
				{
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"configmaps", "configmaps/status"},
							Verbs:     []string{"get", "list", "watch"},
						},
					},
				},
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
				Kind:    "Secret",
			},
		}
		Expect(env.Client(platformClusterID).Status().Update(env.Ctx, sp2)).To(Succeed())
		// this should add the resource to resources that block workspace deletion
		// and grant workspace viewer and admin permissions
		// as well as modify the AccessRequest to contain get permissions for this resource
		expected.resourcesBlockingWorkspaceDeletion = append(expected.resourcesBlockingWorkspaceDeletion, sharedconfig.DeletionBlockingResource{
			GroupVersionKind: sp2.Status.Resources[0],
			Source:           fmt.Sprintf("%s[%s]", pwov1alpha1.SourceServiceProviderPrefix, sp2.Name),
		})
		expected.workspaceViewerPermissions[0].Resources = append(expected.workspaceViewerPermissions[0].Resources, "secrets")
		expected.workspaceAdminPermissions[0].Resources = append(expected.workspaceAdminPermissions[0].Resources, "secrets")
		expected.dynamicAccessPermissions.Permissions[2].Rules[0].Resources = append(expected.dynamicAccessPermissions.Permissions[2].Rules[0].Resources, "secrets", "secrets/status")
		expected.validate(env, pwc)

		// removing the ServiceProvider should undo that change
		Expect(env.Client(platformClusterID).Delete(env.Ctx, sp2)).To(Succeed())
		originallyExpected.validate(env, pwc)

		expected = originallyExpected.clone()
		// modifying the config by adding project and workspace viewer permissions should modify the permissions accordingly
		cfg.Spec.Project.AdditionalPermissions[pwov1alpha1.ProjectRoleView] = []rbacv1.PolicyRule{
			{
				APIGroups: []string{"mygroup.project"},
				Resources: []string{"myprojectadditionalresources1"},
				Verbs:     []string{"get", "update"},
			},
		}
		cfg.Spec.Workspace.AdditionalPermissions[pwov1alpha1.WorkspaceRoleView] = []rbacv1.PolicyRule{
			{
				APIGroups: []string{"mygroup.workspace"},
				Resources: []string{"myworkspaceadditionalresources1", "myworkspaceadditionalresources2"},
				Verbs:     []string{"list", "watch", "get"},
			},
		}
		Expect(env.Client(platformClusterID).Update(env.Ctx, cfg)).To(Succeed())
		expected.projectViewerPermissions = append([]rbacv1.PolicyRule{
			cfg.Spec.Project.AdditionalPermissions[pwov1alpha1.ProjectRoleView][0],
		}, expected.projectViewerPermissions...)
		expected.workspaceViewerPermissions = append([]rbacv1.PolicyRule{
			cfg.Spec.Workspace.AdditionalPermissions[pwov1alpha1.WorkspaceRoleView][0],
		}, expected.workspaceViewerPermissions...)
		expected.validate(env, pwc)

		// removing the additional permissions again should undo that change
		delete(cfg.Spec.Project.AdditionalPermissions, pwov1alpha1.ProjectRoleView)
		delete(cfg.Spec.Workspace.AdditionalPermissions, pwov1alpha1.WorkspaceRoleView)
		Expect(env.Client(platformClusterID).Update(env.Ctx, cfg)).To(Succeed())
		originallyExpected.validate(env, pwc)
	})

})
