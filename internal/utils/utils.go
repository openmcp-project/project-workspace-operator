package utils

import (
	"fmt"
	"reflect"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openmcp-project/controller-utils/pkg/logging"

	pwv1alpha1 "github.com/openmcp-project/platform-service-project-workspace/api/v2/core/v1alpha1"
	"github.com/openmcp-project/platform-service-project-workspace/api/v2/entities"
)

func NamespaceForProject(project *pwv1alpha1.Project) string {
	return fmt.Sprintf("project-%s", project.Name)
}

func NamespaceForWorkspace(workspace *pwv1alpha1.Workspace) string {
	return fmt.Sprintf("%s--ws-%s", workspace.Namespace, workspace.Name)
}

// WasDeleted returns true if the supplied object was deleted from the API server.
func WasDeleted(o metav1.Object) bool {
	return !o.GetDeletionTimestamp().IsZero()
}

// SetMetaDataLabel sets the key value pair in the labels section of the given Object.
// If the given Object did not yet have labels, they are initialized.
func SetMetaDataLabel(meta metav1.Object, key, value string) {
	labels := meta.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = value
	meta.SetLabels(labels)
}

func LogOperationResult(log logging.Logger, level logging.LogLevel, obj client.Object, result controllerutil.OperationResult) {
	objType := reflect.ValueOf(obj).Elem().Type()
	if obj.GetNamespace() == "" {
		log.Log(level, fmt.Sprintf("%s %s %s", objType.Name(), obj.GetName(), result))
	} else {
		log.Log(level, fmt.Sprintf("%s %s/%s %s", objType.Name(), obj.GetNamespace(), obj.GetName(), result))
	}
}

const (
	AdminRoleID  = "admin"
	ViewerRoleID = "viewer"
)

func ProjectMemberRoleToRoleID(role pwv1alpha1.ProjectMemberRole) string {
	switch role {
	case pwv1alpha1.ProjectRoleAdmin:
		return AdminRoleID
	case pwv1alpha1.ProjectRoleView:
		return ViewerRoleID
	default:
		return ""
	}
}

func WorkspaceMemberRoleToRoleID(role pwv1alpha1.WorkspaceMemberRole) string {
	switch role {
	case pwv1alpha1.WorkspaceRoleAdmin:
		return AdminRoleID
	case pwv1alpha1.WorkspaceRoleView:
		return ViewerRoleID
	default:
		return ""
	}
}

func ClusterRoleForEntityAndRole(entity entities.AccessEntity, role entities.AccessRole) string {
	if reflect.TypeOf(entity) != reflect.TypeOf(role.EntityType()) {
		panic("AccessEntity/AccessRole mismatch")
	}
	return strings.Join([]string{
		entity.TypeIdentifier(),
		entity.GetName(),
		role.Identifier(),
	}, ":")
}

func ClusterRoleForRole(role entities.AccessRole) string {
	return strings.Join([]string{
		role.EntityType().TypeIdentifier(),
		role.Identifier(),
	}, "-")
}

func RoleBindingForRole(role entities.AccessRole) string {
	// Name of RoleBinding (namespaced) should be the same as ClusterRole.
	return ClusterRoleForRole(role)
}

func ClusterRoleForEntityAndRoleWithParent(entity entities.AccessEntity, role entities.AccessRole, parent entities.AccessEntity) string {
	if reflect.TypeOf(entity) == reflect.TypeOf(parent) {
		panic("AccessEntity/Parent must not be of same type")
	}
	return strings.Join([]string{
		parent.TypeIdentifier(),
		parent.GetName(),
		ClusterRoleForEntityAndRole(entity, role),
	}, ":")
}
