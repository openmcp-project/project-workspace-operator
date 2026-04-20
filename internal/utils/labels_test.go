package utils_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openmcpconst "github.com/openmcp-project/openmcp-operator/api/constants"

	"github.com/openmcp-project/platform-service-project-workspace/internal/utils"
)

func TestSetManagementLabels(t *testing.T) {
	t.Run("set's the management labels", func(t *testing.T) {
		var obj metav1.ObjectMeta

		utils.SetManagementLabels(&obj, "test")
		assert.Equal(t, map[string]string{openmcpconst.ManagedByLabel: "test", openmcpconst.ManagedPurposeLabel: utils.Purpose}, obj.Labels)
	})
	t.Run("set's the management labels which embeds metav1.ObjectMeta", func(t *testing.T) {
		var obj rbacv1.ClusterRole

		utils.SetManagementLabels(&obj, "test")

		assert.Equal(t, map[string]string{openmcpconst.ManagedByLabel: "test", openmcpconst.ManagedPurposeLabel: utils.Purpose}, obj.Labels)
	})
	t.Run("overwrites label if already set", func(t *testing.T) {
		var obj metav1.ObjectMeta
		utils.SetManagementLabels(&obj, "first")
		utils.SetManagementLabels(&obj, "second")

		assert.Equal(t, map[string]string{openmcpconst.ManagedByLabel: "second", openmcpconst.ManagedPurposeLabel: utils.Purpose}, obj.Labels)
	})
	t.Run("doesn't overwrite other labels", func(t *testing.T) {
		var obj metav1.ObjectMeta
		obj.Labels = map[string]string{
			"existing": "shouldn't be touched",
		}

		utils.SetManagementLabels(&obj, "test")

		assert.Equal(t, map[string]string{openmcpconst.ManagedByLabel: "test", openmcpconst.ManagedPurposeLabel: utils.Purpose, "existing": "shouldn't be touched"}, obj.Labels)
	})
}

func TestSetProjectLabel(t *testing.T) {
	t.Run("set's the 'openmcp.cloud/project' label", func(t *testing.T) {
		var obj metav1.ObjectMeta

		utils.SetProjectLabel(&obj, "test")

		assert.Equal(t, map[string]string{utils.LabelProject: "test"}, obj.Labels)
	})
	t.Run("set's the 'openmcp.cloud/project' label which embeds metav1.ObjectMeta", func(t *testing.T) {
		var obj rbacv1.ClusterRole

		utils.SetProjectLabel(&obj, "test")

		assert.Equal(t, map[string]string{utils.LabelProject: "test"}, obj.Labels)
	})
	t.Run("overwrites label if already set", func(t *testing.T) {
		var obj metav1.ObjectMeta
		utils.SetProjectLabel(&obj, "first")
		utils.SetProjectLabel(&obj, "second")

		assert.Equal(t, map[string]string{utils.LabelProject: "second"}, obj.Labels)
	})
	t.Run("doesn't overwrite other labels", func(t *testing.T) {
		var obj metav1.ObjectMeta
		obj.Labels = map[string]string{
			"existing": "shouldn't be touched",
		}

		utils.SetProjectLabel(&obj, "test")

		assert.Equal(t, map[string]string{utils.LabelProject: "test", "existing": "shouldn't be touched"}, obj.Labels)
	})
}

func TestSetWorkspaceLabel(t *testing.T) {
	t.Run("set's the 'openmcp.cloud/workspace' label", func(t *testing.T) {
		var obj metav1.ObjectMeta

		utils.SetWorkspaceLabel(&obj, "test")

		assert.Equal(t, map[string]string{utils.LabelWorkspace: "test"}, obj.Labels)
	})
	t.Run("set's the 'openmcp.cloud/workspace' label which embeds metav1.ObjectMeta", func(t *testing.T) {
		var obj rbacv1.ClusterRole

		utils.SetWorkspaceLabel(&obj, "test")

		assert.Equal(t, map[string]string{utils.LabelWorkspace: "test"}, obj.Labels)
	})
	t.Run("overwrites label if already set", func(t *testing.T) {
		var obj metav1.ObjectMeta
		utils.SetWorkspaceLabel(&obj, "first")
		utils.SetWorkspaceLabel(&obj, "second")

		assert.Equal(t, map[string]string{utils.LabelWorkspace: "second"}, obj.Labels)
	})
	t.Run("doesn't overwrite other labels", func(t *testing.T) {
		var obj metav1.ObjectMeta
		obj.Labels = map[string]string{
			"existing": "shouldn't be touched",
		}

		utils.SetWorkspaceLabel(&obj, "test")

		assert.Equal(t, map[string]string{utils.LabelWorkspace: "test", "existing": "shouldn't be touched"}, obj.Labels)
	})
}
