# Configuration Controller

Which resources are expected to exist within projects and workspaces can change dynamically during the platform service's runtime, either because the configuration is modified or because the available service resources change. The configuration controller is responsible for detecting and handling these changes.

It watches the following resources:
- `ProjectWorkspaceConfig`
  - reacts to changes to the generation, deletion timestamp, and the `openmcp.cloud/operation` label
  - ignores changes to resources whose name differs from the name of the `PlatformService` that created the controller
- `ServiceProvider`
  - reacts to status changes only

> [!NOTE]
> **Service Resources**
>
> Some familiarity with the openmcp v2 architecture is required in order to understand this documentation. To quickly summarize the relevant aspect: `ServiceProvider` resources (which are managed by the landscape operators) can register so-called 'service resources' (by listing their GVK in their `ServiceProvider`'s status). End-users can then create instances of these service resources next to a `ManagedControlPlaneV2` within their workspaces to make the respective services available via the corresponding cluster.

## End-User Permissions

### Project Permissions

Which permissions project members should have within a project's namespace depends on the following aspects:
- [hard-coded](../../internal/controller/config/builtin.go) permissions
- [configured](../config/config.md) permissions
- role of the user

The config controller maintains a `ClusterRole` for each known project role (`admin` and `view`). The `ClusterRole` contains RBAC rules for the configured as well as the hard-coded resources and is updated whenever something changes. With a few exceptions, 'admin' users usually have read and write permissions, while 'view' users only have read permissions. The configuration takes additional permissions by mapping roles to RBAC rules, thereby allowing fine-grained control over what end-users can do.

Disabling the builtin permissions is not supported.

### Workspace Permissions

Workspace permissions depend on the following aspects:
- [hard-coded](../../internal/controller/config/builtin.go) permissions
- [configured](../config/config.md) permissions
- role of the user
- known service resources

While mostly similar to projects, a significant difference is that end-users are expected to create service resources within their workspaces. This means that the permissions need to be adapted whenever the available service resources change (usually due to a `ServiceProvider` being created or deleted). 
As for projects, a `ClusterRole` is maintained for each workspace role (`admin` and `view`). In addition to the builtin and configured RBAC rules, the roles also contain RBAC rules for the known service resources (read and write permissions for `admin`, read permissions for `view`).

Disabling the builtin permissions or excluding specific service resources is not supported.

## Deletion Blocking Resources

### Projects

Which resources block the deletion of a `Project` depends on the following aspects:
- [hard-coded](../../internal/controller/config/builtin.go) resources
- [configured](../config/config.md) resources

The controller maintains a list of resources it needs to check for internally and updates it whenever the configuration changes.

### Workspaces

The list of resources that block a `Workspace`'s deletion is built from the following sources:
- [hard-coded](../../internal/controller/config/builtin.go) resources
- [configured](../config/config.md) resources
- known service resources

Each known service resource automatically blocks the deletion of the workspace it is in until it is deleted.

### Managing its own Permissions

> [!NOTE]
> This paragraph covers an implementation detail with no direct effect on the platform service's functionality.

The platform service requires read permissions for all resources that block the deletion of a `Project` or `Workspace`, in order to check whether still instances of these resources exist. Since the list of blocking resources can change dynamically (see above), the controller needs to adapt its own permissions accordingly. 

To achieve this, the platform service uses two `AccessRequest`s for accessing the onboarding cluster (not counting the one from the `init` step).

#### Static Onboarding Cluster Access

One `AccessRequest` is static, with hard-coded permission requests. It is created during startup of the platform service and requests full permissions for projects, workspaces, namespaces, RBAC stuff (clusterroles, clusterrolebindings, rolebindings), and the `SelfSubjectReview` API. The last one is required for figuring out its own identity, so that the validation webhooks can ignore changes that come from this platform service itself. All of the other permissions are required for the core functionality of this platform service.

The static `AccessRequest` is used for all interactions with the onboarding cluster, _except for_ detecting deletion blocking resources.

#### Dynamic Onboarding Cluster Access

The second `AccessRequest` is created and continuously updated by the configuration controller. It requests read permissions for all resources that block project or workspace deletion, which includes all known service resources.

It is only used to check for deletion blocking resources, all other interactions with the onboarding cluster use the static `AccessRequest`.

The dynamic `AccessRequest` lives in the same namespace as the `PlatformService` resource and has an `obdyn` suffix.
