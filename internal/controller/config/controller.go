package config

import (
	"context"
	"fmt"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/controller-utils/pkg/collections"
	"github.com/openmcp-project/controller-utils/pkg/collections/filters"
	ctrlutils "github.com/openmcp-project/controller-utils/pkg/controller"
	"github.com/openmcp-project/controller-utils/pkg/logging"
	clustersv1alpha1 "github.com/openmcp-project/openmcp-operator/api/clusters/v1alpha1"
	commonapi "github.com/openmcp-project/openmcp-operator/api/common"
	apiconst "github.com/openmcp-project/openmcp-operator/api/constants"
	openmcpcorev2alpha1 "github.com/openmcp-project/openmcp-operator/api/core/v2alpha1"
	providerv1alpha1 "github.com/openmcp-project/openmcp-operator/api/provider/v1alpha1"
	"github.com/openmcp-project/openmcp-operator/lib/clusteraccess/advanced"

	pwov1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
	"github.com/openmcp-project/project-workspace-operator/api/install"
)

// Static Stuff //

const (
	ControllerName = "pwo-config"

	ClusterIDOnboardingDynamic = "onboarding-dynamic"
)

var (
	BuiltinResourcesBlockingProjectDeletion = []DeletionBlockingResource{
		{
			GroupVersionKind: metav1.GroupVersionKind{
				Group:   pwov1alpha1.GroupVersion.Group,
				Version: pwov1alpha1.GroupVersion.Version,
				Kind:    "Workspace",
			},
			Source: pwov1alpha1.SourceBuiltin,
		},
	}
	BuiltinResourcesBlockingWorkspaceDeletion = []DeletionBlockingResource{
		{
			GroupVersionKind: metav1.GroupVersionKind{
				Group:   openmcpcorev2alpha1.GroupVersion.Group,
				Version: openmcpcorev2alpha1.GroupVersion.Version,
				Kind:    "ManagedControlPlaneV2",
			},
			Source: pwov1alpha1.SourceBuiltin,
		},
	}
	BuiltinPermissibleProjectResources = APIGroupsWithResourcesList{
		{
			APIGroups: []string{pwov1alpha1.GroupVersion.String()},
			Resources: []string{"workspaces"},
		},
	}
	BuiltinPermissibleWorkspaceResources = APIGroupsWithResourcesList{
		{
			APIGroups: []string{openmcpcorev2alpha1.GroupVersion.String()},
			Resources: []string{"managedcontrolplanev2s"},
		},
	}
)

// Setup //

type PWOConfigController struct {
	providerName                  string
	platformCluster               *clusters.Cluster
	Car                           advanced.ClusterAccessReconciler
	rec                           record.EventRecorder
	OnboardingClusterAccessStatic *clusters.Cluster
	DiscoveryService              discovery.DiscoveryInterface

	// The lock needs to be held when reading or writing any of the fields below.
	lock                               *sync.RWMutex
	resourcesBlockingProjectDeletion   []DeletionBlockingResource
	resourcesBlockingWorkspaceDeletion []DeletionBlockingResource
	permissibleProjectResources        APIGroupsWithResourcesList
	permissibleWorkspaceResources      APIGroupsWithResourcesList
	// the config allows more precise definition of roles, therefore the 'permissible...' fields are not enough to store the permissions from the config
	projectPermissionsFromConfig   map[string][]rbacv1.PolicyRule
	workspacePermissionsFromConfig map[string][]rbacv1.PolicyRule
	onboardingClusterAccessDynamic *clusters.Cluster
	missingConfig                  bool
}

type APIGroupsWithResources struct {
	APIGroups []string
	Resources []string
}
type APIGroupsWithResourcesList []APIGroupsWithResources

// Append appends the given elements to the list and returns the new list.
// If there is already an entry with the same apiGroups, the resources are merged.
// Otherwise, a new entry is appended.
func (l APIGroupsWithResourcesList) Append(elems ...APIGroupsWithResources) APIGroupsWithResourcesList {
	for _, elem := range elems {
		found := false
		for i, existing := range l {
			a := sets.New(existing.APIGroups...)
			b := sets.New(elem.APIGroups...)
			if a.Equal(b) {
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

// NewPWOConfigController creates a new PWOConfigController.
// This controller has the following responsibilities:
// - It watches the ProjectWorkspaceConfig resource belonging to this instance of the PlatformService PWO and reloads it on changes.
// - It watches ServiceProvider resources for their registered resource types in their status and updates permissions and blocking resources accordingly.
// - It can trigger project and workspace reconciliations via the passed-in channels if the config changes in a way that requires it.
// - It implements the SharedInformation interface, so that other controllers can query it for the current configuration.
// - It reconciles the OnboardingCluster AccessRequests for the project and workspace controllers to ensure they can always fetch the the resources that are supposed to block deletion.
//
// Note that this is a pure v2 controller. It does neither work for v1, nor is it required, because in v1 all of this information is statically read from a file.
func NewPWOConfigController(providerName string, platformCluster *clusters.Cluster, onboardingClusterStatic *clusters.Cluster, onboardingClusterRef *commonapi.ObjectReference, rec record.EventRecorder, podNamespace string) (*PWOConfigController, error) {
	scheme := install.InstallOperatorAPIsOnboarding(runtime.NewScheme())
	obRef := advanced.StaticReferenceGenerator(onboardingClusterRef)
	var ds discovery.DiscoveryInterface
	if onboardingClusterStatic != nil && onboardingClusterStatic.RESTConfig() != nil {
		var err error
		ds, err = discovery.NewDiscoveryClientForConfig(onboardingClusterStatic.RESTConfig())
		if err != nil {
			return nil, fmt.Errorf("error creating discovery client for onboarding cluster: %v", err)
		}
	}
	return &PWOConfigController{
		providerName:                  providerName,
		platformCluster:               platformCluster,
		OnboardingClusterAccessStatic: onboardingClusterStatic,
		DiscoveryService:              ds,
		Car: advanced.NewClusterAccessReconciler(platformCluster.Client(), ControllerName).
			Register(advanced.ExistingCluster(ClusterIDOnboardingDynamic, "obdyn", obRef).WithScheme(scheme).WithNamespaceGenerator(func(_ reconcile.Request, _ ...any) (string, error) { return podNamespace, nil }).Build()),
		rec:                                rec,
		lock:                               &sync.RWMutex{},
		resourcesBlockingProjectDeletion:   []DeletionBlockingResource{},
		resourcesBlockingWorkspaceDeletion: []DeletionBlockingResource{},
		permissibleProjectResources:        []APIGroupsWithResources{},
		permissibleWorkspaceResources:      []APIGroupsWithResources{},
	}, nil
}

// discoverResourceNameForGVK tries to discover the resource name for the given GroupVersionKind using the discovery client.
func (c *PWOConfigController) discoverResourceNameForGVK(log logging.Logger, gvk metav1.GroupVersionKind) (string, error) {
	if c.DiscoveryService == nil {
		return "", fmt.Errorf("no discovery client set")
	}
	log.Debug("Discovering resource name", "group", gvk.Group, "version", gvk.Version, "kind", gvk.Kind)
	gvMatches, err := c.DiscoveryService.ServerResourcesForGroupVersion(strings.TrimPrefix(fmt.Sprintf("%s/%s", gvk.Group, gvk.Version), "/"))
	if err != nil {
		return "", fmt.Errorf("failed to discover resource names for apiVersion '%s/%s': %w", gvk.Group, gvk.Version, err)
	}
	resMatches := filters.FilterSlice(gvMatches.APIResources, func(args ...any) bool {
		if len(args) != 1 {
			return false
		}
		apir, ok := args[0].(metav1.APIResource)
		if !ok {
			return false
		}
		if apir.Kind != gvk.Kind {
			return false
		}
		// this will also return subresources, like 'myresources/status', so let's filter out anything with a '/' in the name
		return !strings.Contains(apir.Name, "/")
	})
	if len(resMatches) != 1 {
		return "", fmt.Errorf("unable to unambiguously determine resource name for kind '%s' with apiVersion '%s/%s': found %d potential matches: [%s]", gvk.Kind, gvk.Group, gvk.Version, len(resMatches), strings.Join(collections.ProjectSliceToSlice(resMatches, func(res metav1.APIResource) string { return res.Name }), ", "))
	}
	log.Debug("Successfully discovered resource name", "group", gvk.Group, "version", gvk.Version, "kind", gvk.Kind, "resourceName", resMatches[0].Name)
	return resMatches[0].Name, nil
}

func (c *PWOConfigController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("projectworkspaceconfig").
		WatchesRawSource(source.Kind(c.platformCluster.Cluster().GetCache(), &pwov1alpha1.ProjectWorkspaceConfig{}, &handler.TypedEnqueueRequestForObject[*pwov1alpha1.ProjectWorkspaceConfig]{}, ctrlutils.ToTypedPredicate[*pwov1alpha1.ProjectWorkspaceConfig](
			predicate.Or(
				predicate.GenerationChangedPredicate{},
				ctrlutils.DeletionTimestampChangedPredicate{},
				ctrlutils.GotAnnotationPredicate(apiconst.OperationAnnotation, apiconst.OperationAnnotationValueReconcile),
			),
		))).
		WatchesRawSource(source.Kind(c.platformCluster.Cluster().GetCache(), &providerv1alpha1.ServiceProvider{}, handler.TypedEnqueueRequestsFromMapFunc(func(_ context.Context, _ *providerv1alpha1.ServiceProvider) []ctrl.Request {
			// if any ServiceProvider changes, we need to reconcile the config to update the registered resource types
			return []ctrl.Request{
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name: c.providerName,
					},
				},
			}
		}), ctrlutils.ToTypedPredicate[*providerv1alpha1.ServiceProvider](ctrlutils.StatusChangedPredicate{}))).
		Complete(c)
}

// Reconciler Implementation //

var _ reconcile.Reconciler = &PWOConfigController{}

func (c *PWOConfigController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logging.FromContextOrPanic(ctx).WithName(ControllerName)
	ctx = logging.NewContext(ctx, log)
	log.Info("Starting reconcile")
	cfg, rr, err := c.reconcile(ctx, req)
	if c.rec != nil && cfg != nil {
		if err != nil {
			c.rec.Event(cfg, corev1.EventTypeWarning, pwov1alpha1.EventReasonReconcileFailed, err.Error())
		} else {
			c.rec.Event(cfg, corev1.EventTypeNormal, pwov1alpha1.EventReasonReconcileSucceeded, "Reconciliation successful")
		}
	}
	return rr, err
}

func (c *PWOConfigController) reconcile(ctx context.Context, req reconcile.Request) (*pwov1alpha1.ProjectWorkspaceConfig, reconcile.Result, error) {
	log := logging.FromContextOrPanic(ctx)

	if req.Name != c.providerName {
		log.Info("Ignoring ProjectWorkspaceConfig with unexpected name", "expected", c.providerName, "actual", req.Name)
		return nil, reconcile.Result{}, nil
	}

	// get lock to update internal state
	c.lock.Lock()
	defer c.lock.Unlock()

	reset := func() (reconcile.Result, error) {
		c.permissibleProjectResources = nil
		c.permissibleWorkspaceResources = nil
		c.resourcesBlockingProjectDeletion = nil
		c.resourcesBlockingWorkspaceDeletion = nil
		c.projectPermissionsFromConfig = nil
		c.workspacePermissionsFromConfig = nil
		c.missingConfig = true
		log.Info("Resetting state and deleting AccessRequest because ProjectWorkspaceConfig is missing or in deletion")
		return c.Car.ReconcileDelete(ctx, req)
	}

	// fetch the config
	cfg := &pwov1alpha1.ProjectWorkspaceConfig{}
	if err := c.platformCluster.Client().Get(ctx, req.NamespacedName, cfg); err != nil {
		if apierrors.IsNotFound(err) {
			_, _ = reset()
		}
		return nil, reconcile.Result{}, fmt.Errorf("failed to fetch ProjectWorkspaceConfig: %w", err)
	}

	if !cfg.DeletionTimestamp.IsZero() {
		// config is being deleted, this should not happen
		log.Info("Warning: ProjectWorkspaceConfig is in deletion, this should only happen when the PlatformService is being deleted")
		rr, err := reset()
		return cfg, rr, err
	}
	c.missingConfig = false

	if c.OnboardingClusterAccessStatic == nil {
		return nil, reconcile.Result{}, fmt.Errorf("static onboarding cluster access is not available")
	}

	// use information from config
	newResourcesBlockingProjectDeletion := collections.ProjectSliceToSlice(cfg.Spec.Project.ResourcesBlockingDeletion, func(gvk metav1.GroupVersionKind) DeletionBlockingResource {
		return DeletionBlockingResource{
			GroupVersionKind: gvk,
			Source:           pwov1alpha1.SourceProjectWorkspaceConfig,
		}
	})
	newResourcesBlockingWorkspaceDeletion := collections.ProjectSliceToSlice(cfg.Spec.Workspace.ResourcesBlockingDeletion, func(gvk metav1.GroupVersionKind) DeletionBlockingResource {
		return DeletionBlockingResource{
			GroupVersionKind: gvk,
			Source:           pwov1alpha1.SourceProjectWorkspaceConfig,
		}
	})
	newProjectPermissionsFromConfig := map[string][]rbacv1.PolicyRule{}
	for role, rules := range cfg.Spec.Project.AdditionalPermissions {
		newProjectPermissionsFromConfig[ProjectMemberRoleToRoleID(role)] = rules
	}
	newWorkspacePermissionsFromConfig := map[string][]rbacv1.PolicyRule{}
	for role, rules := range cfg.Spec.Workspace.AdditionalPermissions {
		newWorkspacePermissionsFromConfig[WorkspaceMemberRoleToRoleID(role)] = rules
	}

	// fetch ServiceProvider resources to get their registered resource types
	log.Debug("Fetching ServiceProvider resources to get registered resource types")
	newPermissibleProjectResources := APIGroupsWithResourcesList{}
	newPermissibleWorkspaceResources := APIGroupsWithResourcesList{}
	sps := &providerv1alpha1.ServiceProviderList{}
	if err := c.platformCluster.Client().List(ctx, sps); err != nil {
		return cfg, reconcile.Result{}, fmt.Errorf("failed to list ServiceProviders: %w", err)
	}
	log.Debug("Fetched ServiceProviders", "count", len(sps.Items))
	for _, sp := range sps.Items {
		for _, gvk := range sp.Status.Resources {
			// add resource to list of resources blocking workspace deletion
			// (this needs to be extended for project deletion blocking as well, if we ever allow MCPs on project level)
			newResourcesBlockingWorkspaceDeletion = append(newResourcesBlockingWorkspaceDeletion, DeletionBlockingResource{
				GroupVersionKind: gvk,
				Source:           fmt.Sprintf("%s[%s]", pwov1alpha1.SourceServiceProviderPrefix, sp.Name),
			})
			// add resource to permissible resources
			resourceName, err := c.discoverResourceNameForGVK(log, gvk)
			if err != nil {
				return cfg, reconcile.Result{}, fmt.Errorf("error determining resource name for kind '%s' with apiVersion '%s/%s', registered by ServiceProvider '%s: %w", gvk.Kind, gvk.Group, gvk.Version, sp.Name, err)
			}
			agr := APIGroupsWithResources{
				APIGroups: []string{gvk.Group},
				Resources: []string{resourceName},
			}
			// if we allow MCPs on project level, we need the following line
			// newPermissibleProjectResources = newPermissibleProjectResources.Append(agr)
			newPermissibleWorkspaceResources = newPermissibleWorkspaceResources.Append(agr)
		}
	}
	log.Debug("Finished processing ServiceProviders")

	// now we have all required information, update internal state
	c.resourcesBlockingProjectDeletion = newResourcesBlockingProjectDeletion
	c.resourcesBlockingWorkspaceDeletion = newResourcesBlockingWorkspaceDeletion
	c.permissibleProjectResources = newPermissibleProjectResources
	c.permissibleWorkspaceResources = newPermissibleWorkspaceResources
	c.projectPermissionsFromConfig = newProjectPermissionsFromConfig
	c.workspacePermissionsFromConfig = newWorkspacePermissionsFromConfig

	// update the AccessRequests for the onboarding cluster to ensure that the project and workspace controllers have sufficient permissions to get the resources blocking deletion
	log.Info("Updating AccessRequests to ensure project and workspace controllers have sufficient permissions to get deletion blocking resources")
	permissionGroups := APIGroupsWithResourcesList{}
	for _, res := range c.resourcesBlockingProjectDeletion {
		resourceName, err := c.discoverResourceNameForGVK(log, res.GroupVersionKind)
		if err != nil {
			return cfg, reconcile.Result{}, fmt.Errorf("error determining resource name for kind '%s' with apiVersion '%s/%s': %w", res.Kind, res.Group, res.Version, err)
		}
		permissionGroups = permissionGroups.Append(APIGroupsWithResources{
			APIGroups: []string{res.Group},
			Resources: []string{resourceName, fmt.Sprintf("%s/status", resourceName)},
		})

	}
	for _, res := range c.resourcesBlockingWorkspaceDeletion {
		resourceName, err := c.discoverResourceNameForGVK(log, res.GroupVersionKind)
		if err != nil {
			return cfg, reconcile.Result{}, fmt.Errorf("error determining resource name for kind '%s' with apiVersion '%s/%s': %w", res.Kind, res.Group, res.Version, err)
		}
		permissionGroups = permissionGroups.Append(APIGroupsWithResources{
			APIGroups: []string{res.Group},
			Resources: []string{resourceName, fmt.Sprintf("%s/status", resourceName)},
		})
	}
	permissions := collections.ProjectSliceToSlice(permissionGroups, func(elem APIGroupsWithResources) clustersv1alpha1.PermissionsRequest {
		return clustersv1alpha1.PermissionsRequest{
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: elem.APIGroups,
					Resources: elem.Resources,
					Verbs:     []string{"get", "list", "watch"},
				},
			},
		}
	})
	if err := c.Car.Update(ClusterIDOnboardingDynamic, advanced.UpdateTokenAccess(&clustersv1alpha1.TokenConfig{Permissions: permissions})); err != nil {
		return cfg, reconcile.Result{}, fmt.Errorf("failed to update AccessRequest for onboarding cluster: %w", err)
	}
	rr, err := c.Car.Reconcile(ctx, req)
	if err != nil {
		return cfg, rr, fmt.Errorf("failed to reconcile cluster access to the onboarding cluster: %w", err)
	}
	if rr.RequeueAfter > 0 {
		log.Info("Waiting for dynamic onboarding cluster access to become available/updated")
		return cfg, rr, nil
	}

	// update internal onboarding cluster access references
	access, err := c.Car.Access(ctx, req, ClusterIDOnboardingDynamic)
	if err != nil {
		return cfg, rr, fmt.Errorf("failed to get dynamic onboarding cluster access: %w", err)
	}
	c.onboardingClusterAccessDynamic = access

	log.Info("Successfully reloaded configuration")
	return cfg, reconcile.Result{}, nil
}

// SharedInformation Implementation //

var _ SharedInformation = &PWOConfigController{}

func (c *PWOConfigController) ResourcesBlockingProjectDeletion(ctx context.Context) ([]DeletionBlockingResource, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.missingConfig {
		return nil, fmt.Errorf("ProjectWorkspaceConfig is missing")
	}
	res := make([]DeletionBlockingResource, len(c.resourcesBlockingProjectDeletion)+len(BuiltinResourcesBlockingProjectDeletion))
	copy(res, BuiltinResourcesBlockingProjectDeletion)
	copy(res[len(BuiltinResourcesBlockingProjectDeletion):], c.resourcesBlockingProjectDeletion)
	return res, nil
}

func (c *PWOConfigController) ResourcesBlockingWorkspaceDeletion(ctx context.Context) ([]DeletionBlockingResource, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.missingConfig {
		return nil, fmt.Errorf("ProjectWorkspaceConfig is missing")
	}
	res := make([]DeletionBlockingResource, len(c.resourcesBlockingWorkspaceDeletion)+len(BuiltinResourcesBlockingWorkspaceDeletion))
	copy(res, BuiltinResourcesBlockingWorkspaceDeletion)
	copy(res[len(BuiltinResourcesBlockingWorkspaceDeletion):], c.resourcesBlockingWorkspaceDeletion)
	return res, nil
}

func (c *PWOConfigController) ProjectPermissionsForRole(ctx context.Context, roleID string) ([]rbacv1.PolicyRule, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.missingConfig {
		return nil, fmt.Errorf("ProjectWorkspaceConfig is missing")
	}
	permissibleProjectResources := append([]APIGroupsWithResources{}, BuiltinPermissibleProjectResources...)
	permissibleProjectResources = append(permissibleProjectResources, c.permissibleProjectResources...)
	res := []rbacv1.PolicyRule{}
	tmp, err := permissionsForRoleHelper(roleID, permissibleProjectResources)
	if err != nil {
		return nil, err
	}
	res = append(res, tmp...)
	res = append(res, c.projectPermissionsFromConfig[roleID]...)
	return res, nil
}

func (c *PWOConfigController) WorkspacePermissionsForRole(ctx context.Context, roleID string) ([]rbacv1.PolicyRule, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.missingConfig {
		return nil, fmt.Errorf("ProjectWorkspaceConfig is missing")
	}
	permissibleWorkspaceResources := append([]APIGroupsWithResources{}, BuiltinPermissibleWorkspaceResources...)
	permissibleWorkspaceResources = append(permissibleWorkspaceResources, c.permissibleWorkspaceResources...)
	res := []rbacv1.PolicyRule{}
	tmp, err := permissionsForRoleHelper(roleID, permissibleWorkspaceResources)
	if err != nil {
		return nil, err
	}
	res = append(res, tmp...)
	res = append(res, c.workspacePermissionsFromConfig[roleID]...)
	return res, nil
}

// if the logic for project and workspace role assignment diverges in the future, the logic needs to be moved back into the respective functions above
func permissionsForRoleHelper(roleID string, permissibleResources []APIGroupsWithResources) ([]rbacv1.PolicyRule, error) {
	res := []rbacv1.PolicyRule{}
	switch roleID {
	case AdminRole:
		for _, agr := range permissibleResources {
			res = append(res, rbacv1.PolicyRule{
				APIGroups: agr.APIGroups,
				Resources: agr.Resources,
				Verbs:     []string{"*"},
			})
		}
	case ViewerRole:
		for _, agr := range permissibleResources {
			res = append(res, rbacv1.PolicyRule{
				APIGroups: agr.APIGroups,
				Resources: agr.Resources,
				Verbs:     []string{"get", "list", "watch"},
			})
		}
	default:
		return nil, fmt.Errorf("unknown role ID: %s", roleID)
	}
	return res, nil
}

func (c *PWOConfigController) OnboardingClusterStatic(ctx context.Context) (*clusters.Cluster, error) {
	if c.OnboardingClusterAccessStatic == nil {
		return nil, fmt.Errorf("static onboarding cluster access controller not initialized yet")
	}
	return c.OnboardingClusterAccessStatic, nil
}

func (c *PWOConfigController) OnboardingClusterDynamic(ctx context.Context) (*clusters.Cluster, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.onboardingClusterAccessDynamic == nil {
		return nil, fmt.Errorf("dynamic onboarding cluster access for workspace controller not initialized yet")
	}
	return c.onboardingClusterAccessDynamic, nil
}
