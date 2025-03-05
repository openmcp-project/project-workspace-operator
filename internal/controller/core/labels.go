package core

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openmcpv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

const (
	labelManagedBy string = "app.kubernetes.io/managed-by"
)

var ( // TODO make those constant
	labelProject   = fmt.Sprintf("%s/project", openmcpv1alpha1.GroupVersion.Group)
	labelWorkspace = fmt.Sprintf("%s/workspace", openmcpv1alpha1.GroupVersion.Group)
)

func setManagementLabel(obj metav1.Object, controllerName string) {
	setMetaDataLabel(obj, labelManagedBy, controllerName)
}

func setProjectLabel(obj metav1.Object, project string) {
	setMetaDataLabel(obj, labelProject, project)
}

func setWorkspaceLabel(obj metav1.Object, workspace string) {
	setMetaDataLabel(obj, labelWorkspace, workspace)
}
