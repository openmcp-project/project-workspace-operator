package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
	"github.com/openmcp-project/project-workspace-operator/internal/controller/core/config"
)

func TestLoadConfig(t *testing.T) {
	for _, validConfig := range []string{
		"./testdata/config_valid.yaml",
		"./testdata/config_valid2.yaml",
	} {
		pwConfig, err := config.LoadConfig(validConfig)

		if assert.NoError(t, err) {
			assert.NotNil(t, pwConfig)
			resourcesBlockingDeletion := []metav1.GroupVersionKind{
				{
					Group:   "",
					Version: "v1",
					Kind:    "Secret",
				},
			}
			assert.ElementsMatch(t, pwConfig.Spec.Project.ResourcesBlockingDeletion, resourcesBlockingDeletion)
			assert.ElementsMatch(t, pwConfig.Spec.Workspace.ResourcesBlockingDeletion, resourcesBlockingDeletion)
		}
	}

	pwConfig, err := config.LoadConfig("./testdata/config_invalid.yaml")

	if assert.Error(t, err) {
		assert.Nil(t, pwConfig)
	}

	pwConfig, err = config.LoadConfig("./testdata/config_not_found.yaml")

	if assert.Error(t, err) {
		assert.Nil(t, pwConfig)
	}
}

func TestDefaults(t *testing.T) {
	pwConfig := &pwv1alpha1.ProjectWorkspaceConfig{
		Spec: pwv1alpha1.ProjectWorkspaceConfigSpec{
			Project:   pwv1alpha1.ProjectConfig{},
			Workspace: pwv1alpha1.WorkspaceConfig{},
		},
	}

	pwConfig.SetDefaults()

	// No defaults yet
}

func TestValidate(t *testing.T) {
	pwConfig := &pwv1alpha1.ProjectWorkspaceConfig{
		Spec: pwv1alpha1.ProjectWorkspaceConfigSpec{
			Project: pwv1alpha1.ProjectConfig{
				ResourcesBlockingDeletion: []metav1.GroupVersionKind{
					{
						Group:   "",
						Version: "v1",
						Kind:    "Secret",
					},
				},
			},
			Workspace: pwv1alpha1.WorkspaceConfig{},
		},
	}

	assert.NoError(t, pwConfig.Validate())
}
