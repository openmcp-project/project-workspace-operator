# Member Overrides

## Introduction

The Project-Workspace Operator webhook is designed to prevent cluster user from modifying or deleting `Project` and `Workspace` resources if they are not specifically with the `admin` role in the resources `members` spec. 

Unfortunately, this blocks platform or landscape administrators from helping users when there are issues with these resources. The 'member overrides' mechanism is added to the operator webhook to address this limitation by providing a configurable escape-hatch that allows the landscape administrators to manage resources they are not a member of.

The configuration differs slightly, depending on whether the v1 or v2 variant of the Project-Workspace-Operator is used.

## v2

For v2, the configuration of the member overrides is part of the general `ProjectWorkspaceConfig` configuration resource:
```yaml
apiVersion: core.openmcp.cloud/v1alpha1
kind: ProjectWorkspaceConfig
metadata:
  name: project-workspace
spec:
  project: <...>
  workspace: <...>
  memberOverrides:
  - kind: User
    name: kubernetes-admin
    roles:
    - admin
  - kind: User
    name: project-two-support
    resources:
    - kind: Project
      name: two
    roles:
    - admin
  - kind: Group
    name: system:project-one-admins
    resources:
    - kind: Project
      name: one
    roles:
    - admin
  - kind: User
    name: project-three-ws-1-manager
    resources:
    - kind: Project
      name: project-three
    - kind: Workspace
      name: workspace-1
    roles:
    - admin
```

The configuration itself is identical to the v1 one, only where it is specified has changed. See below for some examples.

## v1

For backward compatibility reasons, the v1 variant does not use the member overrides from the configuration, but reads it from the cluster instead.

### Usage
A single `MemberOverrides` resource is created per cluster. To actually use it, you need to explicitly pass the resource name to the operator:

```yaml
--use-member-overrides=<MemberOverridesName>
```

This can be configured using the following helm-values-file entries:
```yaml
webhooks:
  ...
  memberOverrides:
    memberOverridesName: landscape-admins
```

The `MemberOverrides` spec is modeled based on the `Project/Workspace` members spec. A full example looks like this:


```yaml
apiVersion: core.openmcp.cloud/v1alpha1
kind: MemberOverrides
metadata:
  name: landscape-admins
spec:
  memberOverrides:
  - kind: User
    name: kubernetes-admin
    roles:
    - admin
  - kind: User
    name: project-two-support
    resources:
    - kind: Project
      name: two
    roles:
    - admin
  - kind: Group
    name: system:project-one-admins
    resources:
    - kind: Project
      name: one
    roles:
    - admin
  - kind: User
    name: project-three-ws-1-manager
    resources:
    - kind: Project
      name: project-three
    - kind: Workspace
      name: workspace-1
    roles:
    - admin
```

## Use Cases

### General Admin
This is useful in cases where you have a specific user that you want to allow admin access over all Projects/Workspaces on the cluster:
```yaml
  memberOverrides:
    - kind: User
      name: kubernetes-admin
      roles:
      - admin
```

It's also possible to use a group here. If the landscape admins are grouped in a specific group, it's possible to use that group:
```yaml
  memberOverrides:
    - kind: Group
      name: system:masters
      roles:
      - admin
```

### Project/Workspace Admin
It's possible to specify a resource type and name to limit access to a specific project or Workspace resources. This is useful to allow granular permissions or for granting temporary permissions to a specific user during debugging:


```yaml
  memberOverrides:
    - kind: User
      name: project-two-support
      resources:
      - kind: Project
        name: two
      roles:
      - admin
   
    - kind: User
      name: project-three-ws-1-manager
      resources:
      - kind: Project
        name: project-three
      - kind: Workspace
        name: workspace-1
      roles:
      - admin
```

**Note:** Since the `Workspace` doesn't have an explicit reference to the parent `Project`, the override must specify the parent `Project` in the same override configuration for the override to work. 