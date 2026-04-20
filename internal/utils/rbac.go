package utils

import (
	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

func AllVerbs() []string {
	return []string{
		"create",
		"delete",
		"get",
		"list",
		"patch",
		"update",
		"watch",
	}
}

func ReadOnlyVerbs() []string {
	return []string{
		"get",
		"list",
		"watch",
	}
}

func ProjectRolesWithVerbs() map[pwv1alpha1.ProjectMemberRole][]string {
	return map[pwv1alpha1.ProjectMemberRole][]string{
		pwv1alpha1.ProjectRoleAdmin: AllVerbs(),
		pwv1alpha1.ProjectRoleView:  ReadOnlyVerbs(),
	}
}

func WorkspaceRolesWithVerbs() map[pwv1alpha1.WorkspaceMemberRole][]string {
	return map[pwv1alpha1.WorkspaceMemberRole][]string{
		pwv1alpha1.WorkspaceRoleAdmin: AllVerbs(),
		pwv1alpha1.WorkspaceRoleView:  ReadOnlyVerbs(),
	}
}
