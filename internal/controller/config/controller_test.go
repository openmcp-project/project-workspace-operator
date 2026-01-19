package config_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/controller-utils/pkg/collections"
	testutils "github.com/openmcp-project/controller-utils/pkg/testing"
	clustersv1alpha1 "github.com/openmcp-project/openmcp-operator/api/clusters/v1alpha1"
	commonapi "github.com/openmcp-project/openmcp-operator/api/common"
	"github.com/openmcp-project/openmcp-operator/lib/clusteraccess/advanced"

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

func defaultTestSetup(testDirPathSegments ...string) (*sharedconfig.PWOConfigController, *testutils.ComplexEnvironment) {
	env := testutils.NewComplexEnvironmentBuilder().
		WithFakeClient(platformClusterID, platformScheme).
		WithFakeClient(onboardingClusterID, onboardingScheme).
		WithInitObjectPath(platformClusterID, append(testDirPathSegments, "platform")...).
		WithInitObjectPath(onboardingClusterID, append(testDirPathSegments, "onboarding")...).
		WithInitObjects(platformClusterID, &clustersv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "onboarding",
				Namespace: "default",
			},
		}).
		WithDynamicObjectsWithStatus(platformClusterID, &clustersv1alpha1.AccessRequest{}).
		WithReconcilerConstructor(pwcRec, func(c ...client.Client) reconcile.Reconciler {
			pwc := sharedconfig.NewPWOConfigController(providerName, clusters.NewTestClusterFromClient(platformClusterID, c[0]), clusters.NewTestClusterFromClient(onboardingClusterID, c[1]), &commonapi.ObjectReference{Name: "onboarding", Namespace: "default"}, nil)
			pwc.Car.WithFakingCallback(advanced.FakingCallback_WaitingForAccessRequestReadiness, advanced.FakeAccessRequestReadiness())
			pwc.Car.WithFakingCallback(advanced.FakingCallback_WaitingForAccessRequestDeletion, advanced.FakeAccessRequestDeletion([]string{"clusterprovider"}, nil))
			pwc.Car.WithFakeClientGenerator(func(ctx context.Context, kcfgData []byte, scheme *runtime.Scheme, additionalData ...any) (client.Client, error) {
				// this controller creates AccessRequests only for the onboarding cluster
				// and the permissions are hard to test in unit tests anyway, so let's just return the static onboarding cluster client
				return pwc.OnboardingClusterAccessStatic.Client(), nil
			})
			return pwc
		}, platformClusterID, onboardingClusterID).
		Build()

	pwc, ok := env.Reconciler(pwcRec).(*sharedconfig.PWOConfigController)
	Expect(ok).To(BeTrue(), "Reconciler is not of type *PWOConfigController")

	return pwc, env
}

var _ = Describe("ProjectWorkspaceConfig Controller Test", func() {

	It("should return default values for an empty config and no ServiceProviders", func() {
		pwc, env := defaultTestSetup("testdata", "test-01")

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

})
