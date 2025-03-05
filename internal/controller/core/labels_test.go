package core

import (
	"testing"

	"github.com/stretchr/testify/assert"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetManagementLabels(t *testing.T) {
	t.Run("set's the 'app.kubernetes.io/managed-by' label", func(t *testing.T) {
		var obj metav1.ObjectMeta

		setManagementLabel(&obj, "test")

		assert.Equal(t, map[string]string{labelManagedBy: "test"}, obj.Labels)
	})
	t.Run("set's the 'app.kubernetes.io/managed-by' label which embeds metav1.ObjectMeta", func(t *testing.T) {
		var obj rbacv1.ClusterRole

		setManagementLabel(&obj, "test")

		assert.Equal(t, map[string]string{labelManagedBy: "test"}, obj.Labels)
	})
	t.Run("overwrites label if already set", func(t *testing.T) {
		var obj metav1.ObjectMeta
		setManagementLabel(&obj, "first")
		setManagementLabel(&obj, "second")

		assert.Equal(t, map[string]string{labelManagedBy: "second"}, obj.Labels)
	})
	t.Run("doesn't overwrite other labels", func(t *testing.T) {
		var obj metav1.ObjectMeta
		obj.Labels = map[string]string{
			"existing": "shouldn't be touched",
		}

		setManagementLabel(&obj, "test")

		assert.Equal(t, map[string]string{labelManagedBy: "test", "existing": "shouldn't be touched"}, obj.Labels)
	})
}

func TestSetProjectLabel(t *testing.T) {
	t.Run("set's the 'openmcp.cloud/project' label", func(t *testing.T) {
		var obj metav1.ObjectMeta

		setProjectLabel(&obj, "test")

		assert.Equal(t, map[string]string{labelProject: "test"}, obj.Labels)
	})
	t.Run("set's the 'openmcp.cloud/project' label which embeds metav1.ObjectMeta", func(t *testing.T) {
		var obj rbacv1.ClusterRole

		setProjectLabel(&obj, "test")

		assert.Equal(t, map[string]string{labelProject: "test"}, obj.Labels)
	})
	t.Run("overwrites label if already set", func(t *testing.T) {
		var obj metav1.ObjectMeta
		setProjectLabel(&obj, "first")
		setProjectLabel(&obj, "second")

		assert.Equal(t, map[string]string{labelProject: "second"}, obj.Labels)
	})
	t.Run("doesn't overwrite other labels", func(t *testing.T) {
		var obj metav1.ObjectMeta
		obj.Labels = map[string]string{
			"existing": "shouldn't be touched",
		}

		setProjectLabel(&obj, "test")

		assert.Equal(t, map[string]string{labelProject: "test", "existing": "shouldn't be touched"}, obj.Labels)
	})
}

func TestSetWorkspaceLabel(t *testing.T) {
	t.Run("set's the 'openmcp.cloud/workspace' label", func(t *testing.T) {
		var obj metav1.ObjectMeta

		setWorkspaceLabel(&obj, "test")

		assert.Equal(t, map[string]string{labelWorkspace: "test"}, obj.Labels)
	})
	t.Run("set's the 'openmcp.cloud/workspace' label which embeds metav1.ObjectMeta", func(t *testing.T) {
		var obj rbacv1.ClusterRole

		setWorkspaceLabel(&obj, "test")

		assert.Equal(t, map[string]string{labelWorkspace: "test"}, obj.Labels)
	})
	t.Run("overwrites label if already set", func(t *testing.T) {
		var obj metav1.ObjectMeta
		setWorkspaceLabel(&obj, "first")
		setWorkspaceLabel(&obj, "second")

		assert.Equal(t, map[string]string{labelWorkspace: "second"}, obj.Labels)
	})
	t.Run("doesn't overwrite other labels", func(t *testing.T) {
		var obj metav1.ObjectMeta
		obj.Labels = map[string]string{
			"existing": "shouldn't be touched",
		}

		setWorkspaceLabel(&obj, "test")

		assert.Equal(t, map[string]string{labelWorkspace: "test", "existing": "shouldn't be touched"}, obj.Labels)
	})
}
