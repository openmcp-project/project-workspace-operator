package app

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/spf13/cobra"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/controller-utils/pkg/logging"
	openmcpconst "github.com/openmcp-project/openmcp-operator/api/constants"
)

func NewPlatformServiceProjectWorkspaceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "platformservice <init|run>",
		Short:   "Handles projects and workspaces",
		Aliases: []string{"ps"},
	}
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	so := &SharedOptions{
		RawSharedOptions: &RawSharedOptions{},
		PlatformCluster:  clusters.New("platform"),
	}
	so.AddPersistentFlags(cmd)
	cmd.AddCommand(NewInitCommand(so))
	cmd.AddCommand(NewRunCommand(so))

	return cmd
}

type RawSharedOptions struct {
	Environment  string `json:"environment"`
	ProviderName string `json:"provider-name"`
	DryRun       bool   `json:"dry-run"`
}

type SharedOptions struct {
	*RawSharedOptions
	PlatformCluster *clusters.Cluster

	// fields filled in Complete()
	Log logging.Logger
}

func (o *SharedOptions) AddPersistentFlags(cmd *cobra.Command) {
	// logging
	logging.InitFlags(cmd.PersistentFlags())
	// clusters
	o.PlatformCluster.RegisterSingleConfigPathFlag(cmd.PersistentFlags())
	// environment
	cmd.PersistentFlags().StringVar(&o.Environment, "environment", "", "Environment name. Required. This is used to distinguish between different environments that are watching the same Onboarding cluster. Must be globally unique.")
	// provider name
	cmd.PersistentFlags().StringVar(&o.ProviderName, "provider-name", "", "Name of the provider resource.")
	cmd.PersistentFlags().BoolVar(&o.DryRun, "dry-run", false, "If set, the command aborts after evaluation of the given flags.")
}

func (o *SharedOptions) Complete() error {
	if o.Environment == "" {
		return fmt.Errorf("environment must not be empty")
	}
	if o.ProviderName == "" {
		return fmt.Errorf("provider-name must not be empty")
	}

	// build logger
	log, err := logging.GetLogger()
	if err != nil {
		return err
	}
	o.Log = log
	ctrl.SetLogger(o.Log.Logr())

	if err := o.PlatformCluster.InitializeRESTConfig(); err != nil {
		return err
	}

	return nil
}

func resolveWebhookPort(ctx context.Context, platformClusterClient client.Client, targetPort intstr.IntOrString) (int, error) {
	log := logging.FromContextOrDiscard(ctx)
	webhookPort := targetPort.IntValue()
	if webhookPort == 0 {
		// this should only have happened if the user configured a named port
		portName := targetPort.StrVal
		if portName == "" {
			return 0, fmt.Errorf("invalid webhook target port configuration: %v", targetPort)
		}
		log.Info("Resolving webhook port from named port", "portName", portName)
		pod := &corev1.Pod{}
		pod.Name = os.Getenv(openmcpconst.EnvVariablePodName)
		pod.Namespace = os.Getenv(openmcpconst.EnvVariablePodNamespace)
		if pod.Name == "" || pod.Namespace == "" {
			return 0, fmt.Errorf("environment variables %s and %s must be set to resolve webhook port from named port", openmcpconst.EnvVariablePodName, openmcpconst.EnvVariablePodNamespace)
		}
		if err := platformClusterClient.Get(ctx, client.ObjectKey{Name: pod.Name, Namespace: pod.Namespace}, pod); err != nil {
			return 0, fmt.Errorf("unable to get pod '%s/%s' to resolve webhook port from named port: %w", pod.Namespace, pod.Name, err)
		}
		namedPorts := pod.Spec.Containers[0].Ports
		found := false
		for _, p := range namedPorts {
			if p.Name == portName {
				webhookPort = int(p.ContainerPort)
				found = true
				log.Info("Resolved webhook port from named port", "portName", portName, "port", webhookPort)
				break
			}
		}
		if !found {
			return 0, fmt.Errorf("unable to find named port '%s' in pod '%s/%s' to resolve webhook port", portName, pod.Namespace, pod.Name)
		}
	}
	return webhookPort, nil
}
