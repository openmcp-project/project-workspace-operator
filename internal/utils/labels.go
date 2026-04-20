package utils

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openmcpconst "github.com/openmcp-project/openmcp-operator/api/constants"

	pwv1alpha1 "github.com/openmcp-project/platform-service-project-workspace/api/v2/core/v1alpha1"
)

const (
	LabelProject   = pwv1alpha1.GroupName + "/project"
	LabelWorkspace = pwv1alpha1.GroupName + "/workspace"

	Purpose = "project-workspace-management"
)

func SetManagementLabels(obj metav1.Object, providerName string) {
	SetMetaDataLabel(obj, openmcpconst.ManagedByLabel, providerName)
	SetMetaDataLabel(obj, openmcpconst.ManagedPurposeLabel, Purpose)
}

func SetProjectLabel(obj metav1.Object, project string) {
	SetMetaDataLabel(obj, LabelProject, project)
}

func SetWorkspaceLabel(obj metav1.Object, workspace string) {
	SetMetaDataLabel(obj, LabelWorkspace, workspace)
}
