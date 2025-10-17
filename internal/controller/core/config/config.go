package config

import (
	"errors"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

// ToSchemaGVK converts a GroupVersionKind to a schema.GroupVersionKind
func ToSchemaGVK(g metav1.GroupVersionKind) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   g.Group,
		Version: g.Version,
		Kind:    g.Kind,
	}
}

// LoadConfig loads a project workspace configuration from a file.
func LoadConfig(path string) (*pwv1alpha1.ProjectWorkspaceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}
	cfg := &pwv1alpha1.ProjectWorkspaceConfig{}
	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		cfgSpec := &pwv1alpha1.ProjectWorkspaceConfigSpec{}
		err2 := yaml.Unmarshal(data, cfgSpec)
		if err2 != nil {
			return nil, fmt.Errorf("config can neither be parsed as full config nor as spec: %w", errors.Join(err, err2))
		}
		cfg.Spec = *cfgSpec
	}
	return cfg, nil
}
