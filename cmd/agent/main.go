package main

import (
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	imagev1 "github.com/TheTheatreOfDreams/prepuller/api/v1"
	"github.com/TheTheatreOfDreams/prepuller/internal/agent"
)

const (
	defaultRuntimeEndpoint = "unix:///run/containerd/containerd.sock"
	defaultStateFile       = "/var/lib/prepuller/state.json"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(imagev1.AddToScheme(scheme))
}

func main() {
	var nodeName string
	var nodeSelector string
	var seedNodeSelector string
	var runtimeEndpoint string
	var stateFile string
	var syncPeriod time.Duration

	flag.StringVar(&nodeName, "node-name", os.Getenv("NODE_NAME"), "Name of the Kubernetes node this agent is running on.")
	flag.StringVar(&nodeSelector, "node-selector", os.Getenv("PREPULLER_NODE_SELECTOR"), "Node selector that enables prepulling on matching nodes.")
	flag.StringVar(&seedNodeSelector, "seed-node-selector", os.Getenv("PREPULLER_SEED_NODE_SELECTOR"), "Node selector that makes matching nodes pull every platform digest from every Image CR.")
	flag.StringVar(&runtimeEndpoint, "runtime-endpoint", defaultRuntimeEndpoint, "CRI image service endpoint.")
	flag.StringVar(&stateFile, "state-file", defaultStateFile, "Path to the prepuller-owned state file.")
	flag.DurationVar(&syncPeriod, "sync-period", time.Minute, "How often the node agent reconciles desired images.")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	ctx := ctrl.SetupSignalHandler()

	kubeClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		ctrl.Log.Error(err, "unable to create Kubernetes client")
		os.Exit(1)
	}

	runtimeClient, err := agent.NewCRIRuntime(ctx, runtimeEndpoint)
	if err != nil {
		ctrl.Log.Error(err, "unable to create runtime client")
		os.Exit(1)
	}
	defer func() {
		if err := runtimeClient.Close(); err != nil {
			ctrl.Log.Error(err, "unable to close runtime client")
		}
	}()

	runner := &agent.Agent{
		Client:  kubeClient,
		Runtime: runtimeClient,
		Store:   agent.NewStateStore(stateFile),
		Options: agent.Options{
			NodeName:         nodeName,
			NodeSelector:     nodeSelector,
			SeedNodeSelector: seedNodeSelector,
			RuntimeEndpoint:  runtimeEndpoint,
			StateFile:        stateFile,
			SyncPeriod:       syncPeriod,
		},
	}
	if err := runner.Run(ctx); err != nil {
		ctrl.Log.Error(err, "agent failed")
		os.Exit(1)
	}
}
