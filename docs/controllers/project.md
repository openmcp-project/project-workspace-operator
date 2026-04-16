# Project Controller and Webhook

The project controller reconciles `Project` resources.

## The 'Project' Resource

```yaml
apiVersion: core.openmcp.cloud/v1alpha1
kind: Project
metadata:
  name: my-project
  annotations:
    openmcp.cloud/display-name: My Super Cool Project
spec:
  members:
  - kind: User
    name: john.doe@example.com
    roles:
    - admin
  - kind: User
    name: jane.doe@example.com
    roles:
    - view
status:
  namespace: project-my-project
```

`Project` is a cluster-scoped resource. The only configuration is a list of members, with each entry containing a standard RBAC identity definition and a list of project roles that this identity should have. Valid project roles are `admin` and `view`, the former one will grant read and write permissions for some resources within that project's namespace, while the latter one only grants read permissions. Admins are also allowed to modify the `Project` resource itself.

The `openmcp.cloud/display-name` annotation can be used to add a display name to the resource, which will be shown in a custom column when listing projects via `kubectl.

The project controller reconciles `Project` resources and creates a corresponding namespace for each new `Project`. The namespace's name - usually `project-<project-name>` - can be found in the project's status. The controller also creates `RoleBinding`s within the project namespace, which bind the identities specified in the member list to corresponding `ClusterRole`s, granting them the respective permissions. More details about these permissions can be found in the [config controller documentation](./config.md).

There are some resources which can prevent a `Project` from being deleted, see the documentation of the [configuration](../config/config.md) and the [config controller](./config.md) for more details.

## Webhook

Unless disabled via the config, the platform service comes with a webhook for projects. It serves the following purposes:
- It injects a `core.openmcp.cloud/created-by` annotation into a newly created `Project`, containing the identity of the entity that issued the creation.
- It rejects any update to a `Project` after which the issuing entity would not have admin permissions on the project. This also affects project creation.
  - While this logic successfully prevents users from accidentally 'locking themselves out' of their own project, it also prevents landscape operators from modifying a `Project`, unless they add themselves to the project's member list. This problem can be solved via [member overrides](../config/member_overrides.md).
