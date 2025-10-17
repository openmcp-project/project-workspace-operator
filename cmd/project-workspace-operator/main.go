package main

import (
	"context"
	goflag "flag"
	"fmt"
	"os"

	"github.com/openmcp-project/project-workspace-operator/cmd/project-workspace-operator/app"
	"github.com/openmcp-project/project-workspace-operator/internal/controller/core/config"

	"github.com/openmcp-project/project-workspace-operator/internal/controller/core"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	openmcpctrlutil "github.com/openmcp-project/controller-utils/pkg/controller"
	"github.com/openmcp-project/controller-utils/pkg/logging"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"

	"github.com/openmcp-project/controller-utils/pkg/init/crds"
	"github.com/openmcp-project/controller-utils/pkg/init/webhooks"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
	pwocrds "github.com/openmcp-project/project-workspace-operator/api/crds"
	// +kubebuilder:scaffold:imports
)

const (
	controllerName = "project-workspace-operator"
)

var (
	scheme = core.Scheme
)

func main() {
	cmd := NewProjectWorkspaceOperatorCommand()

	if err := cmd.Execute(); err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
}

func NewProjectWorkspaceOperatorCommand() *cobra.Command {
	options := NewOptions()

	cmd := &cobra.Command{
		Use:   "project-workspace-operator",
		Short: "Runs the Project/Workspace Operator",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("no command specified")
		},
	}

	cmd.AddCommand(newProjectWorkspaceOperatorInitCommand(options))
	cmd.AddCommand(newProjectWorkspaceOperatorStartCommand(options))
	cmd.AddCommand(app.NewPlatformServiceProjectWorkspaceCommand())

	return cmd
}

func (o *Options) run() {
	runContext := context.Background()
	setupLog := o.Log.WithName("setup")

	crateClient, err := client.New(o.CrateClusterConfig, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create crate client")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(o.CrateClusterConfig, ctrl.Options{
		Scheme:                  scheme,
		Metrics:                 metricsserver.Options{BindAddress: o.MetricsAddr},
		HealthProbeBindAddress:  o.ProbeAddr,
		LeaderElection:          o.EnableLeaderElection,
		LeaderElectionID:        "pwo.openmcp.cloud",
		LeaderElectionNamespace: o.LeaseNamespace,
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	rbacSetup := core.NewRBACSetup(setupLog.Logr(), crateClient, controllerName, o.ProjectWorkspaceConfig.Spec)
	if err := rbacSetup.EnsureResources(runContext); err != nil {
		setupLog.Error(err, "unable to create or update RBAC resources")
		os.Exit(1)
	}

	commonReconciler := core.CommonReconciler{
		Client:                     mgr.GetClient(),
		ControllerName:             controllerName,
		ProjectWorkspaceConfigSpec: o.ProjectWorkspaceConfig.Spec,
	}

	if err = (&core.ProjectReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		CommonReconciler: commonReconciler,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Project")
		os.Exit(1)
	}

	if err = (&core.WorkspaceReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		CommonReconciler: commonReconciler,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Workspace")
		os.Exit(1)
	}

	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = (&pwv1alpha1.Project{}).SetupWebhookWithManager(mgr, *o.MemberOverridesName); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Project")
			os.Exit(1)
		}

		if err = (&pwv1alpha1.Workspace{}).SetupWebhookWithManager(mgr, *o.MemberOverridesName); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Workspace")
			os.Exit(1)
		}
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func newProjectWorkspaceOperatorInitCommand(options *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Installs Webhooks and CRDs for the Project/Workspace Operator",
		Run: func(cmd *cobra.Command, args []string) {
			options.runInit()
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return options.Complete()
		},
	}

	options.AddInitFlags(cmd.Flags())
	cmd.Flags().AddGoFlagSet(goflag.CommandLine)

	return cmd
}

func newProjectWorkspaceOperatorStartCommand(options *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Starts the Project/Workspace Operator",
		Run: func(cmd *cobra.Command, args []string) {
			options.run()
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return options.Complete()
		},
	}

	options.AddStartFlags(cmd.Flags())
	cmd.Flags().AddGoFlagSet(goflag.CommandLine)

	return cmd
}

func (o *Options) runInit() {
	initContext := context.Background()
	setupLog := o.Log.WithName("setup")

	setupClient, err := client.New(o.HostClusterConfig, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create setup client")
		os.Exit(1)
	}
	crateClient, err := client.New(o.CrateClusterConfig, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create crate client")
		os.Exit(1)
	}

	if o.WebhooksFlags.Install {
		// Generate webhook certificate
		if err := webhooks.GenerateCertificate(initContext, setupClient, o.WebhooksFlags.CertOptions...); err != nil {
			setupLog.Error(err, "unable to generate webhook certificates")
			os.Exit(1)
		}

		installOptions := o.WebhooksFlags.InstallOptions
		installOptions = append(installOptions, webhooks.WithRemoteClient{Client: crateClient})

		// Install webhooks
		err := webhooks.Install(
			initContext,
			setupClient,
			scheme,
			[]client.Object{
				&pwv1alpha1.Project{},
				&pwv1alpha1.Workspace{},
			},
			installOptions...,
		)
		if err != nil {
			setupLog.Error(err, "unable to configure webhooks")
			os.Exit(1)
		}
	}

	if o.CRDFlags.Install {
		installOptions := o.CRDFlags.InstallOptions
		installOptions = append(installOptions, crds.WithRemoteClient{Client: crateClient})

		// Install CRDs
		if err := crds.Install(initContext, setupClient, pwocrds.CRDFS, installOptions...); err != nil {
			setupLog.Error(err, "unable to install Custom Resource Definitions")
			os.Exit(1)
		}
	}
}

// rawOptions contains the options specified directly via the command line.
// The Options struct then contains these as embedded struct and additionally some options that were derived from the raw options (e.g. by loading files or interpreting raw options).
type rawOptions struct {
	// controller-runtime stuff
	MetricsAddr          string
	EnableLeaderElection bool
	LeaseNamespace       string
	ProbeAddr            string

	// raw options that need to be evaluated
	CrateClusterPath           string
	ProjectWorkspaceConfigPath string

	// raw options that are final
}

type Options struct {
	rawOptions

	// completed options from raw options
	Log                    logging.Logger
	HostClusterConfig      *rest.Config
	CrateClusterConfig     *rest.Config
	CRDFlags               *crds.Flags
	WebhooksFlags          *webhooks.Flags
	MemberOverridesName    *string
	ProjectWorkspaceConfig *pwv1alpha1.ProjectWorkspaceConfig
}

func NewOptions() *Options {
	return &Options{
		CRDFlags:      crds.BindFlags(goflag.CommandLine),
		WebhooksFlags: webhooks.BindFlags(goflag.CommandLine),
		// MemberOverridesFlags: &MemberOverridesOptions{},
	}
}

func (o *Options) addCommonFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.CrateClusterPath, "crate-cluster", "", "Path to the crate cluster kubeconfig file or directory containing either a kubeconfig or host, token, and ca file. Leave empty to use in-cluster config.")
}

func (o *Options) AddStartFlags(fs *flag.FlagSet) {
	// standard stuff
	fs.StringVar(&o.MetricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	fs.StringVar(&o.ProbeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	fs.BoolVar(&o.EnableLeaderElection, "leader-elect", false, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	fs.StringVar(&o.LeaseNamespace, "lease-namespace", "default", "Namespace in which the controller manager's leader election lease will be created.")
	fs.StringVar(&o.ProjectWorkspaceConfigPath, "config", "", "Path to the project workspace config file.")

	o.MemberOverridesName = fs.String("use-member-overrides", "", "Specify a MemberOverrides resources name.")
	// add common flags
	o.addCommonFlags(fs)

	// custom stuff
	logging.InitFlags(fs)

	// fs.StringVar(o.MemberOverridesFlags.MemberOverridesName, "use-member-overrides-name", "", "Specify a MemberOverrides resources name.")
}

func (o *Options) AddInitFlags(fs *flag.FlagSet) {
	// add common flags
	o.addCommonFlags(fs)
}

func (o *Options) Complete() error {
	// build logger
	log, err := logging.GetLogger()
	if err != nil {
		return err
	}
	o.Log = log
	ctrl.SetLogger(o.Log.Logr())

	// load kubeconfigs
	o.HostClusterConfig = ctrl.GetConfigOrDie()
	if o.CrateClusterConfig, err = openmcpctrlutil.LoadKubeconfig(o.CrateClusterPath); err != nil {
		return err
	}

	if o.ProjectWorkspaceConfigPath != "" {
		o.ProjectWorkspaceConfig, err = config.LoadConfig(o.ProjectWorkspaceConfigPath)
		if err != nil {
			return err
		}
	}

	if o.ProjectWorkspaceConfig == nil {
		o.ProjectWorkspaceConfig = &pwv1alpha1.ProjectWorkspaceConfig{}
	}

	o.ProjectWorkspaceConfig.SetDefaults()

	if err = o.ProjectWorkspaceConfig.Validate(); err != nil {
		return err
	}

	return nil
}
