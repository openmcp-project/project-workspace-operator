# Workspace Controller and Webhook

The workspace controller reconciles `Workspace` resources.

## The 'Workspace' Resource

```yaml
apiVersion: core.openmcp.cloud/v1alpha1
kind: Workspace
metadata:
  name: my-workspace
  namespace: project-my-project
  annotations:
    openmcp.cloud/display-name: My Even Cooler Workspace
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
  namespace: project-my-project--ws-my-workspace
```

Workspaces behave very similar to projects, the main difference being that they are namespaced and expected to be created within project namespaces, thereby introducing a second layer of hierarchical structuring. To avoid duplicating documentation, this document focuses on the differences between workspaces and projects, please refer to the [project documentation](./project.md) for more details.

By default, workspaces cannot be created within workspace namespaces, but landscape operators could easily enable this by configuring [additional workspace permissions](../config/config.md#additional-permissions-1), which would allow end-users to create hierarchies of any depth.

As for projects, workspaces distinguish between an `admin` role with read and write access and a `view` role with only read access. Project roles are not automatically propagated to workspaces - if someone is admin in a project, he is not automatically admin for any workspace within that project (although he can easily grant himself the role by editing the `Workspace` resource).

## Webhook

There is also a webhook for `Workspace` resources, which does the same things as [the project one](./project.md#webhook).
