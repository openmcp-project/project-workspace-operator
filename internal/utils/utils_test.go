package utils_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pwv1alpha1 "github.com/openmcp-project/platform-service-project-workspace/api/v2/core/v1alpha1"
	"github.com/openmcp-project/platform-service-project-workspace/internal/utils"
)

func newTestProject(name string) *pwv1alpha1.Project {
	return &pwv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func newTestWorkspace(namespace string, name string) *pwv1alpha1.Workspace {
	return &pwv1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func TestNamespaceForProject(t *testing.T) {
	tests := []struct {
		description string
		project     *pwv1alpha1.Project
		expected    string
	}{
		{
			description: "happy path",
			project:     newTestProject("test"),
			expected:    "project-test",
		},
		// FIXME the current implementation panics if the project is nil
		// {
		//     description: "doesn't fail if nil",
		//     expected: "project-default",
		// },
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			uut := utils.NamespaceForProject(test.project)

			assert.Equal(t, test.expected, uut)
		})
	}
}

func TestNamespaceForWorkspace(t *testing.T) {
	tests := []struct {
		description string
		workspace   *pwv1alpha1.Workspace
		expected    string
	}{
		{
			description: "happy path",
			workspace:   newTestWorkspace("my-namespace", "test"),
			expected:    "my-namespace--ws-test",
		},
		// FIXME the current implementation panics if the workspace is nil
		// {
		//     description: "doesn't fail if nil",
		//     expected: "tbd",
		// },
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			uut := utils.NamespaceForWorkspace(test.workspace)

			assert.Equal(t, test.expected, uut)
		})
	}
}

func TestWasDeleted(t *testing.T) {
	t.Run("returns 'true' if the object has been deleted", func(t *testing.T) {
		timestamp := metav1.Now()

		uut := &pwv1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &timestamp,
			},
		}

		assert.True(t, utils.WasDeleted(uut))
	})
	t.Run("returns 'false' if the object is not deleted", func(t *testing.T) {
		uut := &pwv1alpha1.Project{}

		assert.False(t, utils.WasDeleted(uut))
	})
}

func TestSetMetaDataLabel(t *testing.T) {
	t.Run("set's the label on an object which has no other labels set", func(t *testing.T) {
		var obj metav1.ObjectMeta

		utils.SetMetaDataLabel(&obj, "test", "abc")

		assert.Equal(t, map[string]string{"test": "abc"}, obj.GetLabels())
	})
	t.Run("overwrites the label on an object if it was set before", func(t *testing.T) {
		var obj metav1.ObjectMeta

		utils.SetMetaDataLabel(&obj, "test", "abc")
		utils.SetMetaDataLabel(&obj, "test", "def")

		assert.Equal(t, map[string]string{"test": "def"}, obj.GetLabels())
	})
	t.Run("doesn't modify other labels", func(t *testing.T) {
		var obj metav1.ObjectMeta

		utils.SetMetaDataLabel(&obj, "a", "abc")
		utils.SetMetaDataLabel(&obj, "b", "def")

		assert.Equal(t, map[string]string{"a": "abc", "b": "def"}, obj.GetLabels())
	})
}
