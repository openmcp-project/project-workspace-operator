package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"

	crdutil "github.com/openmcp-project/controller-utils/pkg/crds"
	clustersv1alpha1 "github.com/openmcp-project/openmcp-operator/api/clusters/v1alpha1"
	openmcpconst "github.com/openmcp-project/openmcp-operator/api/constants"
	"github.com/openmcp-project/openmcp-operator/lib/clusteraccess"

	"github.com/openmcp-project/project-workspace-operator/api/crds"
	providerscheme "github.com/openmcp-project/project-workspace-operator/api/install"
	"github.com/openmcp-project/project-workspace-operator/internal/controller/core"
)

func NewInitCommand(so *SharedOptions) *cobra.Command {
	opts := &InitOptions{
		SharedOptions: so,
	}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Platform Service ProjectWorkspace",
		Run: func(cmd *cobra.Command, args []string) {
			opts.PrintRawOptions(cmd)
			if err := opts.Complete(cmd.Context()); err != nil {
				panic(fmt.Errorf("error completing options: %w", err))
			}
			opts.PrintCompletedOptions(cmd)
			if opts.DryRun {
				cmd.Println("=== END OF DRY RUN ===")
				return
			}
			if err := opts.Run(cmd.Context()); err != nil {
				panic(err)
			}
		},
	}
	opts.AddFlags(cmd)

	return cmd
}

type InitOptions struct {
	*SharedOptions
}

func (o *InitOptions) AddFlags(cmd *cobra.Command) {}

func (o *InitOptions) Complete(ctx context.Context) error {
	if err := o.SharedOptions.Complete(); err != nil {
		return err
	}

	return nil
}

func (o *InitOptions) Run(ctx context.Context) error {
	if err := o.PlatformCluster.InitializeClient(providerscheme.InstallCRDAPIs(runtime.NewScheme())); err != nil {
		return err
	}

	log := o.Log.WithName("main")
	log.Info("Environment", "value", o.Environment)
	log.Info("ProviderName", "value", o.ProviderName)

	log.Info("Getting access to the onboarding cluster")
	onboardingScheme := runtime.NewScheme()
	providerscheme.InstallCRDAPIs(onboardingScheme)

	providerSystemNamespace := os.Getenv(openmcpconst.EnvVariablePodNamespace)
	if providerSystemNamespace == "" {
		return fmt.Errorf("environment variable %s is not set", openmcpconst.EnvVariablePodNamespace)
	}

	clusterAccessManager := clusteraccess.NewClusterAccessManager(o.PlatformCluster.Client(), core.ControllerName, providerSystemNamespace)
	clusterAccessManager.WithLogger(&log).
		WithInterval(10 * time.Second).
		WithTimeout(30 * time.Minute)

	onboardingCluster, err := clusterAccessManager.CreateAndWaitForCluster(ctx, clustersv1alpha1.PURPOSE_ONBOARDING+"-init", clustersv1alpha1.PURPOSE_ONBOARDING,
		onboardingScheme, []clustersv1alpha1.PermissionsRequest{
			{
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"apiextensions.k8s.io"},
						Resources: []string{"customresourcedefinitions"},
						Verbs:     []string{"*"},
					},
				},
			},
		})

	if err != nil {
		return fmt.Errorf("error creating/updating onboarding cluster: %w", err)
	}

	// apply CRDs
	crdManager := crdutil.NewCRDManager(openmcpconst.ClusterLabel, crds.CRDs)
	crdManager.AddCRDLabelToClusterMapping(clustersv1alpha1.PURPOSE_PLATFORM, o.PlatformCluster)
	crdManager.AddCRDLabelToClusterMapping(clustersv1alpha1.PURPOSE_ONBOARDING, onboardingCluster)
	if err := crdManager.CreateOrUpdateCRDs(ctx, &log); err != nil {
		return fmt.Errorf("error creating/updating CRDs: %w", err)
	}

	log.Info("Finished init command")
	return nil
}
