package config

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/yaml"
)

// GroupVersionKind represents a Kubernetes GroupVersionKind
type GroupVersionKind struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

// ToSchemaGVK converts a GroupVersionKind to a schema.GroupVersionKind
func (g *GroupVersionKind) ToSchemaGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   g.Group,
		Version: g.Version,
		Kind:    g.Kind,
	}
}

// ProjectConfig contains the configuration for projects.
type ProjectConfig struct {
	// +optional
	ResourcesBlockingDeletion []GroupVersionKind `json:"resourcesBlockingDeletion,omitempty"`
}

// WorkspaceConfig contains the configuration for workspaces.
type WorkspaceConfig struct {
	// +optional
	ResourcesBlockingDeletion []GroupVersionKind `json:"resourcesBlockingDeletion,omitempty"`
}

// ProjectWorkspaceConfig contains the configuration for projects and workspaces.
type ProjectWorkspaceConfig struct {
	// +optional
	Project ProjectConfig `json:"project,omitempty"`
	// +optional
	Workspace WorkspaceConfig `json:"workspace,omitempty"`
}

// SetDefaults sets the default values for the project workspace configuration when not set.
func (pwc *ProjectWorkspaceConfig) SetDefaults() {
}

// Validate validates the project workspace configuration.
func (pwc *ProjectWorkspaceConfig) Validate() error {
	errs := field.ErrorList{}
	return errs.ToAggregate()
}

// LoadConfig loads a project workspace configuration from a file.
func LoadConfig(path string) (*ProjectWorkspaceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}
	cfg := &ProjectWorkspaceConfig{}
	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}
	return cfg, nil
}
