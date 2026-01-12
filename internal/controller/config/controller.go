package config

import (
	"context"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/controller-utils/pkg/collections"
	"github.com/openmcp-project/controller-utils/pkg/logging"
	clustersv1alpha1 "github.com/openmcp-project/openmcp-operator/api/clusters/v1alpha1"
	commonapi "github.com/openmcp-project/openmcp-operator/api/common"
	openmcpcorev2alpha1 "github.com/openmcp-project/openmcp-operator/api/core/v2alpha1"
	providerv1alpha1 "github.com/openmcp-project/openmcp-operator/api/provider/v1alpha1"
	"github.com/openmcp-project/openmcp-operator/lib/clusteraccess/advanced"

	pwov1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
	"github.com/openmcp-project/project-workspace-operator/api/install"
)

/// Static Stuff ///

const (
	ControllerName = "pwo-config"

	clusterIDOnboardingInternal  = "onboarding-internal"
	clusterIDOnboardingProject   = "onboarding-project"
	clusterIDOnboardingWorkspace = "onboarding-workspace"
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
	BuiltinPermissableProjectResources = apiGroupsWithResourcesList{
		{
			apiGroups: []string{pwov1alpha1.GroupVersion.String()},
			resources: []string{"workspaces"},
		},
	}
	BuiltinPermissableWorkspaceResources = apiGroupsWithResourcesList{
		{
			apiGroups: []string{openmcpcorev2alpha1.GroupVersion.String()},
			resources: []string{"managedcontrolplanev2s"},
		},
	}
)

/// Setup ///

type PWOConfigController struct {
	providerName    string
	platformCluster *clusters.Cluster
	car             advanced.ClusterAccessReconciler
	rec             record.EventRecorder

	// The lock needs to be held when reading or writing any of the fields below.
	lock                               *sync.RWMutex
	resourcesBlockingProjectDeletion   []DeletionBlockingResource
	resourcesBlockingWorkspaceDeletion []DeletionBlockingResource
	permissableProjectResources        apiGroupsWithResourcesList
	permissableWorkspaceResources      apiGroupsWithResourcesList
	// the config allows more precise definition of roles, therefore the 'permissable...' fields are not enough to store the permissions from the config
	projectPermissionsFromConfig     map[string][]rbacv1.PolicyRule
	workspacePermissionsFromConfig   map[string][]rbacv1.PolicyRule
	onboardingClusterAccessInternal  *clusters.Cluster
	onboardingClusterAccessProject   *clusters.Cluster
	onboardingClusterAccessWorkspace *clusters.Cluster
	missingConfig                    bool
}

type apiGroupsWithResources struct {
	apiGroups []string
	resources []string
}
type apiGroupsWithResourcesList []apiGroupsWithResources

// Append appends the given elements to the list and returns the new list.
// If there is already an entry with the same apiGroups, the resources are merged.
// Otherwise, a new entry is appended.
func (l apiGroupsWithResourcesList) Append(elems ...apiGroupsWithResources) apiGroupsWithResourcesList {
	for _, elem := range elems {
		found := false
		for i, existing := range l {
			a := sets.New(existing.apiGroups...)
			b := sets.New(elem.apiGroups...)
			if a.Equal(b) {
				found = true
				existing.resources = append(existing.resources, elem.resources...)
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
func NewPWOConfigController(providerName string, platformCluster *clusters.Cluster, onboardingClusterRef *commonapi.ObjectReference, rec record.EventRecorder) *PWOConfigController {
	scheme := install.InstallOperatorAPIsOnboarding(runtime.NewScheme())
	obRef := advanced.StaticReferenceGenerator(onboardingClusterRef)
	return &PWOConfigController{
		providerName:    providerName,
		platformCluster: platformCluster,
		car: advanced.NewClusterAccessReconciler(platformCluster.Client(), ControllerName).
			Register(advanced.ExistingCluster(clusterIDOnboardingInternal, "obi", obRef).WithScheme(scheme).WithTokenAccess(&clustersv1alpha1.TokenConfig{
				Permissions: []clustersv1alpha1.PermissionsRequest{
					{
						Rules: []rbacv1.PolicyRule{
							{
								APIGroups: []string{apiextv1.SchemeGroupVersion.Group},
								Resources: []string{"customresourcedefinitions"},
								Verbs:     []string{"get", "list", "watch"},
							},
						},
					},
				},
			}).Build()).
			Register(advanced.ExistingCluster(clusterIDOnboardingProject, "obp", obRef).WithScheme(scheme).Build()).
			Register(advanced.ExistingCluster(clusterIDOnboardingWorkspace, "obw", obRef).WithScheme(scheme).Build()),
		rec:                                rec,
		lock:                               &sync.RWMutex{},
		resourcesBlockingProjectDeletion:   []DeletionBlockingResource{},
		resourcesBlockingWorkspaceDeletion: []DeletionBlockingResource{},
		permissableProjectResources:        []apiGroupsWithResources{},
		permissableWorkspaceResources:      []apiGroupsWithResources{},
	}
}

/// Reconciler Implementation ///

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

	// fetch the config
	cfg := &pwov1alpha1.ProjectWorkspaceConfig{}
	if err := c.platformCluster.Client().Get(ctx, req.NamespacedName, cfg); err != nil {
		if apierrors.IsNotFound(err) {
			c.missingConfig = true
		}
		return nil, reconcile.Result{}, fmt.Errorf("failed to fetch ProjectWorkspaceConfig: %w", err)
	}

	if !cfg.DeletionTimestamp.IsZero() {
		// config is being deleted, this should not happen
		log.Info("Warning: ProjectWorkspaceConfig is in deletion, this is not supposed to happen")
		c.permissableProjectResources = nil
		c.permissableWorkspaceResources = nil
		c.resourcesBlockingProjectDeletion = nil
		c.resourcesBlockingWorkspaceDeletion = nil
		c.projectPermissionsFromConfig = nil
		c.workspacePermissionsFromConfig = nil
		c.missingConfig = true
		return cfg, reconcile.Result{}, nil
	}
	c.missingConfig = false

	if c.onboardingClusterAccessInternal == nil {
		log.Info("Internal onboarding cluster access not yet available, this should only happen during startup")
		log.Debug("Updating AccessRequests to ensure internal access to the onboarding cluster")
		rr, err := c.car.Reconcile(ctx, req)
		if err != nil {
			return cfg, rr, fmt.Errorf("failed to reconcile cluster access to the onboarding cluster: %w", err)
		}
		if rr.RequeueAfter > 0 {
			log.Info("Waiting for internal onboarding cluster access to become available")
			return cfg, rr, nil
		}
		access, err := c.car.Access(ctx, req, clusterIDOnboardingInternal)
		if err != nil {
			return cfg, rr, fmt.Errorf("failed to get onboarding cluster access for internal use: %w", err)
		}
		c.onboardingClusterAccessInternal = access
		log.Info("Onboarding cluster access for internal use is now available")
		return cfg, reconcile.Result{RequeueAfter: 1}, nil
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
	newPermissableProjectResources := apiGroupsWithResourcesList{}
	newPermissableWorkspaceResources := apiGroupsWithResourcesList{}
	sps := &providerv1alpha1.ServiceProviderList{}
	if err := c.platformCluster.Client().List(ctx, sps); err != nil {
		return cfg, reconcile.Result{}, fmt.Errorf("failed to list ServiceProviders: %w", err)
	}
	log.Debug("Fetched ServiceProviders", "count", len(sps.Items))
	// fetch CRDs from onboarding cluster
	// this is required because we only have the kind, but need the plural form resource name for RBAC
	log.Debug("Fetching CRDs from the onboarding cluster")
	crds := &apiextv1.CustomResourceDefinitionList{}
	if err := c.onboardingClusterAccessInternal.Client().List(ctx, crds); err != nil {
		return cfg, reconcile.Result{}, fmt.Errorf("failed to list CRDs from onboarding cluster: %w", err)
	}
	pluralNames := map[metav1.GroupVersionKind]string{}
	for _, crd := range crds.Items {
		for _, ver := range crd.Spec.Versions {
			gvk := metav1.GroupVersionKind{
				Group:   crd.Spec.Group,
				Version: ver.Name,
				Kind:    crd.Spec.Names.Kind,
			}
			pluralNames[gvk] = crd.Spec.Names.Plural
		}
	}
	for _, sp := range sps.Items {
		for _, gvk := range sp.Status.Resources {
			// add resource to list of resources blocking workspace deletion
			// (this needs to be extended for project deletion blocking as well, if we ever allow MCPs on project level)
			newResourcesBlockingWorkspaceDeletion = append(newResourcesBlockingWorkspaceDeletion, DeletionBlockingResource{
				GroupVersionKind: gvk,
				Source:           fmt.Sprintf("%s[%s]", pwov1alpha1.SourceServiceProviderPrefix, sp.Name),
			})
			// add resource to permissable resources
			pluralName, ok := pluralNames[gvk]
			if !ok {
				return cfg, reconcile.Result{}, fmt.Errorf("unable to find CRD for kind '%s' with apiVersion '%s/%s' registered by ServiceProvider '%s'", gvk.Kind, gvk.Group, gvk.Version, sp.Name)
			}
			agr := apiGroupsWithResources{
				apiGroups: []string{gvk.Group},
				resources: []string{pluralName},
			}
			// if we allow MCPs on project level, we need the following line
			// newPermissableProjectResources = newPermissableProjectResources.Append(agr)
			newPermissableWorkspaceResources = newPermissableWorkspaceResources.Append(agr)
		}
	}
	log.Debug("Finished processing ServiceProviders")

	// now we have all required information, update internal state
	c.resourcesBlockingProjectDeletion = newResourcesBlockingProjectDeletion
	c.resourcesBlockingWorkspaceDeletion = newResourcesBlockingWorkspaceDeletion
	c.permissableProjectResources = newPermissableProjectResources
	c.permissableWorkspaceResources = newPermissableWorkspaceResources
	c.projectPermissionsFromConfig = newProjectPermissionsFromConfig
	c.workspacePermissionsFromConfig = newWorkspacePermissionsFromConfig

	// update the AccessRequests for the onboarding cluster to ensure that the project and workspace controllers have sufficient permissions to get the resources blocking deletion
	log.Info("Updating AccessRequests to ensure project and workspace controllers have sufficient permissions to get deletion blocking resources")
	permissions := []clustersv1alpha1.PermissionsRequest{}
	for _, res := range c.resourcesBlockingProjectDeletion {
		pluralName, ok := pluralNames[res.GroupVersionKind]
		if !ok {
			return cfg, reconcile.Result{}, fmt.Errorf("unable to find CRD for kind '%s' with apiVersion '%s/%s'", res.GroupVersionKind.Kind, res.GroupVersionKind.Group, res.GroupVersionKind.Version)
		}
		permissions = append(permissions, clustersv1alpha1.PermissionsRequest{
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{res.GroupVersionKind.Group},
					Resources: []string{pluralName},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
		})
	}
	c.car.Update(clusterIDOnboardingProject, advanced.UpdateTokenAccess(&clustersv1alpha1.TokenConfig{Permissions: permissions}))
	permissions = []clustersv1alpha1.PermissionsRequest{}
	for _, res := range c.resourcesBlockingWorkspaceDeletion {
		pluralName, ok := pluralNames[res.GroupVersionKind]
		if !ok {
			return cfg, reconcile.Result{}, fmt.Errorf("unable to find CRD for kind '%s' with apiVersion '%s/%s'", res.GroupVersionKind.Kind, res.GroupVersionKind.Group, res.GroupVersionKind.Version)
		}
		permissions = append(permissions, clustersv1alpha1.PermissionsRequest{
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{res.GroupVersionKind.Group},
					Resources: []string{pluralName},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
		})
	}
	c.car.Update(clusterIDOnboardingWorkspace, advanced.UpdateTokenAccess(&clustersv1alpha1.TokenConfig{Permissions: permissions}))
	rr, err := c.car.Reconcile(ctx, req)
	if err != nil {
		return cfg, rr, fmt.Errorf("failed to reconcile cluster access to the onboarding cluster: %w", err)
	}
	if rr.RequeueAfter > 0 {
		log.Info("Waiting for onboarding cluster access for project and/or workspace controller to become available/updated")
		return cfg, rr, nil
	}

	// update internal onboarding cluster access references
	access, err := c.car.Access(ctx, req, clusterIDOnboardingProject)
	if err != nil {
		return cfg, rr, fmt.Errorf("failed to get onboarding cluster access for project controller: %w", err)
	}
	c.onboardingClusterAccessProject = access
	access, err = c.car.Access(ctx, req, clusterIDOnboardingWorkspace)
	if err != nil {
		return cfg, rr, fmt.Errorf("failed to get onboarding cluster access for workspace controller: %w", err)
	}
	c.onboardingClusterAccessWorkspace = access

	log.Info("Successfully reloaded configuration")
	return cfg, reconcile.Result{}, nil
}

/// SharedInformation Implementation ///

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
	permissableProjectResources := append([]apiGroupsWithResources{}, BuiltinPermissableProjectResources...)
	permissableProjectResources = append(permissableProjectResources, c.permissableProjectResources...)
	res := []rbacv1.PolicyRule{}
	tmp, err := permissionsForRoleHelper(roleID, permissableProjectResources)
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
	permissableWorkspaceResources := append([]apiGroupsWithResources{}, BuiltinPermissableWorkspaceResources...)
	permissableWorkspaceResources = append(permissableWorkspaceResources, c.permissableWorkspaceResources...)
	res := []rbacv1.PolicyRule{}
	tmp, err := permissionsForRoleHelper(roleID, permissableWorkspaceResources)
	if err != nil {
		return nil, err
	}
	res = append(res, tmp...)
	res = append(res, c.workspacePermissionsFromConfig[roleID]...)
	return res, nil
}

// if the logic for project and workspace role assignment diverges in the future, the logic needs to be moved back into the respective functions above
func permissionsForRoleHelper(roleID string, permissableResources []apiGroupsWithResources) ([]rbacv1.PolicyRule, error) {
	res := []rbacv1.PolicyRule{}
	switch roleID {
	case AdminRole:
		for _, agr := range permissableResources {
			res = append(res, rbacv1.PolicyRule{
				APIGroups: agr.apiGroups,
				Resources: agr.resources,
				Verbs:     []string{"*"},
			})
		}
	case ViewerRole:
		for _, agr := range permissableResources {
			res = append(res, rbacv1.PolicyRule{
				APIGroups: agr.apiGroups,
				Resources: agr.resources,
				Verbs:     []string{"get", "list", "watch"},
			})
		}
	default:
		return nil, fmt.Errorf("unknown role ID: %s", roleID)
	}
	return res, nil
}

func (c *PWOConfigController) OnboardingClusterForProjectController(ctx context.Context) (*clusters.Cluster, error) {
	if c.onboardingClusterAccessProject == nil {
		return nil, fmt.Errorf("onboarding cluster access for project controller not initialized yet")
	}
	return c.onboardingClusterAccessProject, nil
}

func (c *PWOConfigController) OnboardingClusterForWorkspaceController(ctx context.Context) (*clusters.Cluster, error) {
	if c.onboardingClusterAccessWorkspace == nil {
		return nil, fmt.Errorf("onboarding cluster access for workspace controller not initialized yet")
	}
	return c.onboardingClusterAccessWorkspace, nil
}
