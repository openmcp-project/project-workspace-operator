package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openmcp-project/project-workspace-operator/internal/controller/core/config"
)

func TestLoadConfig(t *testing.T) {
	pwConfig, err := config.LoadConfig("./testdata/config_valid.yaml")

	if assert.NoError(t, err) {
		assert.NotNil(t, pwConfig)
	}

	pwConfig, err = config.LoadConfig("./testdata/config_invalid.yaml")

	if assert.Error(t, err) {
		assert.Nil(t, pwConfig)
	}

	pwConfig, err = config.LoadConfig("./testdata/config_not_found.yaml")

	if assert.Error(t, err) {
		assert.Nil(t, pwConfig)
	}
}

func TestDefaults(t *testing.T) {
	pwConfig := &config.ProjectWorkspaceConfig{
		Project:   config.ProjectConfig{},
		Workspace: config.WorkspaceConfig{},
	}

	pwConfig.SetDefaults()

	// No defaults yet
}

func TestValidate(t *testing.T) {
	pwConfig := &config.ProjectWorkspaceConfig{
		Project: config.ProjectConfig{
			ResourcesBlockingDeletion: []config.GroupVersionKind{
				{
					Group:   "",
					Version: "v1",
					Kind:    "Secret",
				},
			},
		},
		Workspace: config.WorkspaceConfig{},
	}

	assert.NoError(t, pwConfig.Validate())
}
