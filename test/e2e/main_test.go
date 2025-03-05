//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"
	"sigs.k8s.io/e2e-framework/third_party/helm"
)

var (
	e2eClusterName = getEnv("E2E_CLUSTER_NAME", envconf.RandomName("openmcp-pwo-e2e", 16))
	e2eUUTImage    = getEnv("E2E_UUT_IMAGE", "project-workspace-operator")
	e2eUUTTag      = getEnv("E2E_UUT_VERSION", "dev")
	e2eUUTImageTag = fmt.Sprintf("%s:%s", e2eUUTImage, e2eUUTTag)
	e2eUUTChart    = getEnv("E2E_UUT_CHART", "../../charts/project-workspace-operator")
)

func TestMain(m *testing.M) {
	testenv := env.New().
		Setup(
			// create a kind cluster
			envfuncs.CreateCluster(kind.NewProvider(), e2eClusterName),

			// load the operator image to be tested into the cluster
			envfuncs.LoadImageToCluster(e2eClusterName, e2eUUTImageTag),

			// install the operator using the helmchart
			func(ctx context.Context, config *envconf.Config) (context.Context, error) {
				manager := helm.New(config.KubeconfigFile())
				err := manager.RunInstall(
					helm.WithName("project-workspace-operator"),
					helm.WithChart(e2eUUTChart),
					helm.WithArgs(
						"--set", fmt.Sprintf("image.repository=%s", e2eUUTImage),
						"--set", fmt.Sprintf("image.tag=%s", e2eUUTTag),
						"--set", "image.pullPolicy=Never",
						"-f", "../../hack/local-values.yaml", // TODO e2e tests should have its own values file
					),
					helm.WithWait(),
					helm.WithTimeout("10m"),
				)
				return ctx, err
			},
		).
		Finish(
			envfuncs.DestroyCluster(e2eClusterName),
		)

	os.Exit(testenv.Run(m))
}

// getEnv returns the value of the given environment variable or the specified defaultValue if none is set
func getEnv(name string, defaultValue string) string {
	if value, exists := os.LookupEnv(name); exists {
		return value
	}

	return defaultValue
}
