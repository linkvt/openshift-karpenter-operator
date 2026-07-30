package main

import (
	"flag"
	"os"
	"runtime"

	"github.com/openshift/karpenter-operator/pkg/operator"
	"github.com/openshift/karpenter-operator/pkg/version"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

func main() {
	var opts operator.Options

	flag.StringVar(&opts.Namespace, "namespace", "", "The namespace to deploy karpenter into")
	flag.StringVar(&opts.MetricsAddr, "metrics-bind-address", ":8080", "The address the metrics endpoint binds to")
	flag.StringVar(&opts.ProbeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to")
	flag.BoolVar(&opts.LeaderElect, "leader-elect", false, "Enable leader election for controller manager")
	// TODO(maxcao13): this is a no-op flag for now. We need it to make HCP not complaining about an unsupported flag.
	// Making this flag manage Karpenter resources in HCP guest cluster is tracked in https://redhat.atlassian.net/browse/AUTOSCALE-877
	flag.StringVar(&opts.GuestKubeconfig, "guest-kubeconfig", "", "Path to guest side kubeconfig file. Optional flag, but required for HyperShift mode")

	zapOpts := zap.Options{Development: false}
	zapOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))

	setupLog.Info("starting", "version", version.String, "go", runtime.Version(), "os", runtime.GOOS, "arch", runtime.GOARCH)

	if err := opts.LoadEnv(); err != nil {
		setupLog.Error(err, "failed to load environment")
		os.Exit(1)
	}

	if err := opts.Validate(); err != nil {
		setupLog.Error(err, "invalid configuration")
		os.Exit(1)
	}

	if err := operator.Run(ctrl.SetupSignalHandler(), opts); err != nil {
		setupLog.Error(err, "unable to run operator")
		os.Exit(1)
	}
}
