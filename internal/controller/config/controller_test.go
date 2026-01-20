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

var _ = Describe("ProjectWorkspaceConfig Controller Test", func() {

	It("should return default values for an empty config and no ServiceProviders", func() {
		pwc, env := defaultTestSetup(filepath.Join("testdata", "test-01"))

		req := testutils.RequestFromStrings(providerName)
		Eventually(env.ShouldReconcile).WithArguments(pwcRec, req).Should(WithTransform(func(rr reconcile.Result) time.Duration { return rr.RequeueAfter }, BeZero()))

		_, err := pwc.OnboardingClusterStatic(env.Ctx)
		Expect(err).ToNot(HaveOccurred())
		_, err = pwc.OnboardingClusterDynamic(env.Ctx)
		Expect(err).ToNot(HaveOccurred())

		resourcesBlockingProjectDeletion, err := pwc.ResourcesBlockingProjectDeletion(env.Ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(resourcesBlockingProjectDeletion).To(ConsistOf(sharedconfig.BuiltinResourcesBlockingProjectDeletion))

		resourcesBlockingWorkspaceDeletion, err := pwc.ResourcesBlockingWorkspaceDeletion(env.Ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(resourcesBlockingWorkspaceDeletion).To(ConsistOf(sharedconfig.BuiltinResourcesBlockingWorkspaceDeletion))

		for _, roleID := range []string{sharedconfig.ViewerRole, sharedconfig.AdminRole} {
			verbs := []string{"get", "list", "watch"}
			if roleID == sharedconfig.AdminRole {
				verbs = []string{"*"}
			}
			perms, err := pwc.ProjectPermissionsForRole(env.Ctx, roleID)
			Expect(err).ToNot(HaveOccurred())
			Expect(perms).To(ConsistOf(collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleProjectResources, func(elem sharedconfig.APIGroupsWithResources) rbacv1.PolicyRule {
				return rbacv1.PolicyRule{
					APIGroups: elem.APIGroups,
					Resources: elem.Resources,
					Verbs:     verbs,
				}
			})))
			perms, err = pwc.WorkspacePermissionsForRole(env.Ctx, roleID)
			Expect(err).ToNot(HaveOccurred())
			Expect(perms).To(ConsistOf(collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleWorkspaceResources, func(elem sharedconfig.APIGroupsWithResources) rbacv1.PolicyRule {
				return rbacv1.PolicyRule{
					APIGroups: elem.APIGroups,
					Resources: elem.Resources,
					Verbs:     verbs,
				}
			})))
		}

		ar, err := pwc.Car.AccessRequest(env.Ctx, req, sharedconfig.ClusterIDOnboardingDynamic)
		Expect(err).ToNot(HaveOccurred())
		Expect(ar.Spec.Token).To(Equal(&clustersv1alpha1.TokenConfig{}))
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

		req := testutils.RequestFromStrings(providerName)
		Eventually(env.ShouldReconcile).WithArguments(pwcRec, req).Should(WithTransform(func(rr reconcile.Result) time.Duration { return rr.RequeueAfter }, BeZero()))

		_, err := pwc.OnboardingClusterStatic(env.Ctx)
		Expect(err).ToNot(HaveOccurred())
		_, err = pwc.OnboardingClusterDynamic(env.Ctx)
		Expect(err).ToNot(HaveOccurred())

		resourcesBlockingProjectDeletion, err := pwc.ResourcesBlockingProjectDeletion(env.Ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(resourcesBlockingProjectDeletion).To(ConsistOf(sharedconfig.BuiltinResourcesBlockingProjectDeletion))

		expectedResourcesBlockingWorkspaceDeletion := append([]sharedconfig.DeletionBlockingResource{}, sharedconfig.BuiltinResourcesBlockingWorkspaceDeletion...)
		expectedResourcesBlockingWorkspaceDeletion = append(expectedResourcesBlockingWorkspaceDeletion,
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
		resourcesBlockingWorkspaceDeletion, err := pwc.ResourcesBlockingWorkspaceDeletion(env.Ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(resourcesBlockingWorkspaceDeletion).To(ConsistOf(expectedResourcesBlockingWorkspaceDeletion))

		permissibleWorkspaceResources := append([]sharedconfig.APIGroupsWithResources{}, sharedconfig.BuiltinPermissibleWorkspaceResources...)
		permissibleWorkspaceResources = append(permissibleWorkspaceResources,
			sharedconfig.APIGroupsWithResources{
				APIGroups: []string{""},
				Resources: []string{"configmaps", "secrets"},
			},
		)
		for _, roleID := range []string{sharedconfig.ViewerRole, sharedconfig.AdminRole} {
			verbs := []string{"get", "list", "watch"}
			if roleID == sharedconfig.AdminRole {
				verbs = []string{"*"}
			}
			perms, err := pwc.ProjectPermissionsForRole(env.Ctx, roleID)
			Expect(err).ToNot(HaveOccurred())
			Expect(perms).To(ConsistOf(collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleProjectResources, func(elem sharedconfig.APIGroupsWithResources) rbacv1.PolicyRule {
				return rbacv1.PolicyRule{
					APIGroups: elem.APIGroups,
					Resources: elem.Resources,
					Verbs:     verbs,
				}
			})))
			perms, err = pwc.WorkspacePermissionsForRole(env.Ctx, roleID)
			Expect(err).ToNot(HaveOccurred())
			Expect(perms).To(ConsistOf(collections.ProjectSliceToSlice(permissibleWorkspaceResources, func(elem sharedconfig.APIGroupsWithResources) rbacv1.PolicyRule {
				return rbacv1.PolicyRule{
					APIGroups: elem.APIGroups,
					Resources: elem.Resources,
					Verbs:     verbs,
				}
			})))
		}

		ar, err := pwc.Car.AccessRequest(env.Ctx, req, sharedconfig.ClusterIDOnboardingDynamic)
		Expect(err).ToNot(HaveOccurred())
		Expect(ar.Spec.Token).To(Equal(&clustersv1alpha1.TokenConfig{
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
		}))
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

		req := testutils.RequestFromStrings(providerName)
		Eventually(env.ShouldReconcile).WithArguments(pwcRec, req).Should(WithTransform(func(rr reconcile.Result) time.Duration { return rr.RequeueAfter }, BeZero()))

		cfg := &pwov1alpha1.ProjectWorkspaceConfig{}
		err := env.Client(platformClusterID).Get(env.Ctx, client.ObjectKey{Name: providerName}, cfg)
		Expect(err).ToNot(HaveOccurred())

		_, err = pwc.OnboardingClusterStatic(env.Ctx)
		Expect(err).ToNot(HaveOccurred())
		_, err = pwc.OnboardingClusterDynamic(env.Ctx)
		Expect(err).ToNot(HaveOccurred())

		expectedResourcesBlockingProjectDeletion := append([]sharedconfig.DeletionBlockingResource{}, sharedconfig.BuiltinResourcesBlockingProjectDeletion...)
		expectedResourcesBlockingProjectDeletion = append(expectedResourcesBlockingProjectDeletion, collections.ProjectSliceToSlice(cfg.Spec.Project.ResourcesBlockingDeletion, func(gvk metav1.GroupVersionKind) sharedconfig.DeletionBlockingResource {
			return sharedconfig.DeletionBlockingResource{
				GroupVersionKind: gvk,
				Source:           pwov1alpha1.SourceProjectWorkspaceConfig,
			}
		})...)

		resourcesBlockingProjectDeletion, err := pwc.ResourcesBlockingProjectDeletion(env.Ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(resourcesBlockingProjectDeletion).To(ConsistOf(expectedResourcesBlockingProjectDeletion))

		expectedResourcesBlockingWorkspaceDeletion := append([]sharedconfig.DeletionBlockingResource{}, sharedconfig.BuiltinResourcesBlockingWorkspaceDeletion...)
		expectedResourcesBlockingWorkspaceDeletion = append(expectedResourcesBlockingWorkspaceDeletion, collections.ProjectSliceToSlice(cfg.Spec.Workspace.ResourcesBlockingDeletion, func(gvk metav1.GroupVersionKind) sharedconfig.DeletionBlockingResource {
			return sharedconfig.DeletionBlockingResource{
				GroupVersionKind: gvk,
				Source:           pwov1alpha1.SourceProjectWorkspaceConfig,
			}
		})...)
		resourcesBlockingWorkspaceDeletion, err := pwc.ResourcesBlockingWorkspaceDeletion(env.Ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(resourcesBlockingWorkspaceDeletion).To(ConsistOf(expectedResourcesBlockingWorkspaceDeletion))

		perms, err := pwc.ProjectPermissionsForRole(env.Ctx, sharedconfig.ViewerRole)
		Expect(err).ToNot(HaveOccurred())
		expectedPerms := []rbacv1.PolicyRule{
			{
				APIGroups: []string{"mygroup.project"},
				Resources: []string{"myprojectadditionalresources1"},
				Verbs:     []string{"get", "update"},
			},
		}
		expectedPerms = append(expectedPerms, collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleProjectResources, func(elem sharedconfig.APIGroupsWithResources) rbacv1.PolicyRule {
			return rbacv1.PolicyRule{
				APIGroups: elem.APIGroups,
				Resources: elem.Resources,
				Verbs:     []string{"get", "list", "watch"},
			}
		})...)
		Expect(perms).To(ConsistOf(expectedPerms))
		perms, err = pwc.ProjectPermissionsForRole(env.Ctx, sharedconfig.AdminRole)
		Expect(err).ToNot(HaveOccurred())
		expectedPerms = []rbacv1.PolicyRule{
			{
				APIGroups: []string{"mygroup.project"},
				Resources: []string{"myprojectadditionalresources1", "myprojectadditionalresources2"},
				Verbs:     []string{"get", "update"},
			},
		}
		expectedPerms = append(expectedPerms, collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleProjectResources, func(elem sharedconfig.APIGroupsWithResources) rbacv1.PolicyRule {
			return rbacv1.PolicyRule{
				APIGroups: elem.APIGroups,
				Resources: elem.Resources,
				Verbs:     []string{"*"},
			}
		})...)
		Expect(perms).To(ConsistOf(expectedPerms))

		perms, err = pwc.WorkspacePermissionsForRole(env.Ctx, sharedconfig.ViewerRole)
		Expect(err).ToNot(HaveOccurred())
		expectedPerms = []rbacv1.PolicyRule{
			{
				APIGroups: []string{"mygroup.workspace"},
				Resources: []string{"myworkspaceadditionalresources1", "myworkspaceadditionalresources2"},
				Verbs:     []string{"list", "watch", "get"},
			},
		}
		expectedPerms = append(expectedPerms, collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleWorkspaceResources, func(elem sharedconfig.APIGroupsWithResources) rbacv1.PolicyRule {
			return rbacv1.PolicyRule{
				APIGroups: elem.APIGroups,
				Resources: elem.Resources,
				Verbs:     []string{"get", "list", "watch"},
			}
		})...)
		Expect(perms).To(ConsistOf(expectedPerms))
		perms, err = pwc.WorkspacePermissionsForRole(env.Ctx, sharedconfig.AdminRole)
		Expect(err).ToNot(HaveOccurred())
		expectedPerms = []rbacv1.PolicyRule{
			{
				APIGroups: []string{"mygroup.workspace"},
				Resources: []string{"myworkspaceadditionalresources1", "myworkspaceadditionalresources2"},
				Verbs:     []string{"*"},
			},
		}
		expectedPerms = append(expectedPerms, collections.ProjectSliceToSlice(sharedconfig.BuiltinPermissibleWorkspaceResources, func(elem sharedconfig.APIGroupsWithResources) rbacv1.PolicyRule {
			return rbacv1.PolicyRule{
				APIGroups: elem.APIGroups,
				Resources: elem.Resources,
				Verbs:     []string{"*"},
			}
		})...)
		Expect(perms).To(ConsistOf(expectedPerms))

		ar, err := pwc.Car.AccessRequest(env.Ctx, req, sharedconfig.ClusterIDOnboardingDynamic)
		Expect(err).ToNot(HaveOccurred())
		Expect(ar.Spec.Token).To(Equal(&clustersv1alpha1.TokenConfig{
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
		}))
	})

})
