package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrlutils "github.com/openmcp-project/controller-utils/pkg/controller"
	crdutil "github.com/openmcp-project/controller-utils/pkg/crds"
	"github.com/openmcp-project/controller-utils/pkg/init/webhooks"
	clustersv1alpha1 "github.com/openmcp-project/openmcp-operator/api/clusters/v1alpha1"
	openmcpconst "github.com/openmcp-project/openmcp-operator/api/constants"
	"github.com/openmcp-project/openmcp-operator/lib/clusteraccess"

	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
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
	if err := o.PlatformCluster.InitializeClient(providerscheme.InstallOperatorAPIsPlatform(runtime.NewScheme())); err != nil {
		return err
	}

	log := o.Log.WithName("main")
	log.Info("Environment", "value", o.Environment)
	log.Info("ProviderName", "value", o.ProviderName)

	log.Info("Getting access to the onboarding cluster")
	onboardingScheme := runtime.NewScheme()
	providerscheme.InstallOperatorAPIsOnboarding(onboardingScheme)

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
					{
						APIGroups: []string{"admissionregistration.k8s.io"},
						Resources: []string{"mutatingwebhookconfigurations", "validatingwebhookconfigurations"},
						Verbs:     []string{"*"},
					},
					{
						APIGroups: []string{""},
						Resources: []string{"secrets", "services"},
						Verbs:     []string{"*"},
					},
				},
			},
		})

	if err != nil {
		return fmt.Errorf("error creating/updating onboarding cluster: %w", err)
	}

	// apply CRDs
	log.Info("Creating/updating CRDs")
	crdManager := crdutil.NewCRDManager(openmcpconst.ClusterLabel, crds.CRDs)
	crdManager.AddCRDLabelToClusterMapping(clustersv1alpha1.PURPOSE_PLATFORM, o.PlatformCluster)
	crdManager.AddCRDLabelToClusterMapping(clustersv1alpha1.PURPOSE_ONBOARDING, onboardingCluster)
	if err := crdManager.CreateOrUpdateCRDs(ctx, &log); err != nil {
		return fmt.Errorf("error creating/updating CRDs: %w", err)
	}

	// initialize webhooks
	log.Info("Initializing webhooks")

	log.Info("Fetching ProjectWorkspaceConfig")
	// this will likely fail a few times while the crd is being registered
	pwc := &pwv1alpha1.ProjectWorkspaceConfig{}
	if err := o.PlatformCluster.Client().Get(ctx, client.ObjectKey{Name: o.ProviderName}, pwc); err != nil {
		return fmt.Errorf("unable to get ProjectWorkspaceConfig '%s': %w", o.ProviderName, err)
	}
	pwc.SetDefaults()
	if err := pwc.Validate(); err != nil {
		return fmt.Errorf("invalid ProjectWorkspaceConfig '%s': %w", o.ProviderName, err)
	}

	suffix := "-webhook"
	whServiceName := ctrlutils.ShortenToXCharactersUnsafe(o.ProviderName, ctrlutils.K8sMaxNameLength-len(suffix)) + suffix
	suffix = "-webhook-tls"
	whSecretName := ctrlutils.ShortenToXCharactersUnsafe(o.ProviderName, ctrlutils.K8sMaxNameLength-len(suffix)) + suffix

	opts := []webhooks.InstallOption{
		webhooks.WithWebhookService{Name: whServiceName, Namespace: providerSystemNamespace},
		webhooks.WithWebhookSecret{Name: whSecretName, Namespace: providerSystemNamespace},
		webhooks.WithRemoteClient{Client: onboardingCluster.Client()},
		webhooks.WithManagedWebhookService{
			TargetPort: *pwc.Spec.Webhook.TargetPort,
			SelectorLabels: map[string]string{
				"app.kubernetes.io/component":  "controller",
				"app.kubernetes.io/managed-by": "openmcp-operator",
				"app.kubernetes.io/name":       "PlatformService",
				"app.kubernetes.io/instance":   o.ProviderName,
			},
		},
	}

	// webhook options we might or might not support at a later time
	/*
		opts = append(opts, webhooks.WithoutCA)
		opts = append(opts, webhooks.WithCustomBaseURL("todo"))
		opts = append(opts, webhooks.WithCustomCA{todo})
	*/

	if !pwc.Spec.Webhook.Disabled {
		log.Info("Webhooks are enabled, ensuring required resources ...")

		// Generate webhook certificate
		if err := webhooks.GenerateCertificate(ctx, o.PlatformCluster.Client(),
			webhooks.WithWebhookService{Name: whServiceName, Namespace: providerSystemNamespace},
			webhooks.WithWebhookSecret{Name: whSecretName, Namespace: providerSystemNamespace},
		); err != nil {
			return fmt.Errorf("unable to generate webhook certificate: %w", err)
		}

		// Install webhooks
		err := webhooks.Install(
			ctx,
			o.PlatformCluster.Client(),
			onboardingScheme,
			[]client.Object{
				&pwv1alpha1.Project{},
				&pwv1alpha1.Workspace{},
			},
			opts...,
		)
		if err != nil {
			return fmt.Errorf("unable to install webhooks: %w", err)
		}
	} else {
		log.Info("Webhooks are disabled, removing webhook resources if they exist ...")

		// Uninstall webhooks
		err := webhooks.Uninstall(
			ctx,
			o.PlatformCluster.Client(),
			onboardingScheme,
			[]client.Object{
				&pwv1alpha1.Project{},
				&pwv1alpha1.Workspace{},
			},
			opts...,
		)
		if err != nil {
			return fmt.Errorf("unable to uninstall webhooks: %w", err)
		}
	}

	log.Info("Finished init command")
	return nil
}
