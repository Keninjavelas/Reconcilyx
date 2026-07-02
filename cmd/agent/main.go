package main

import (
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/agent"
)

func main() {
	var hubKubeconfigPath string
	var clusterName string
	flag.StringVar(&hubKubeconfigPath, "hub-kubeconfig", "", "Path to kubeconfig for Hub cluster (empty = in-cluster)")
	flag.StringVar(&clusterName, "cluster-name", os.Getenv("SPOKE_CLUSTER_NAME"), "Name of this spoke cluster")
	flag.DurationVar(&agent.RollbackWindow, "rollback-window", 5*time.Minute, "Rollback monitoring window")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// 1. Build Local Client (uses KUBECONFIG env or ~/.kube/config)
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	localConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		panic("failed to get local config: " + err.Error())
	}
	localClient, err := client.New(localConfig, client.Options{})
	if err != nil {
		panic("failed to build local client: " + err.Error())
	}

	// 2. Build Hub Client (remote)
	var hubConfig *rest.Config
	if hubKubeconfigPath != "" {
		hubConfig, err = clientcmd.BuildConfigFromFlags("", hubKubeconfigPath)
	} else {
		// If running locally for testing, we might want to fallback to local kubeconfig
		hubConfig, err = rest.InClusterConfig()
	}
	if err != nil {
		panic("failed to build hub config: " + err.Error())
	}
	
	// 3. Setup Hub Manager (this manager talks to the Hub API)
	scheme := runtime.NewScheme()
	_ = optimizationv1alpha1.AddToScheme(scheme)

	mgr, err := ctrl.NewManager(hubConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	if err != nil {
		panic("failed to create manager: " + err.Error())
	}

	reconciler := &agent.RemoteReconciler{
		HubClient:       mgr.GetClient(), // uses hub config
		LocalClient:     localClient,
		Scheme:          scheme,
		ClusterName:     clusterName,
		Recorder:        mgr.GetEventRecorderFor("reconcilyx-agent"),
		RollbackWindow:  agent.RollbackWindow,
	}

	if err := reconciler.SetupWithManager(mgr); err != nil {
		panic("failed to setup controller: " + err.Error())
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		panic("manager stopped: " + err.Error())
	}
}
