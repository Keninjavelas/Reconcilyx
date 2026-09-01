package main

import (
	"context"
	"crypto/tls"
	"flag"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/api"
	"github.com/reconcilyx/operator/internal/carbon"
	"github.com/reconcilyx/operator/internal/cloud"
	"github.com/reconcilyx/operator/internal/controller"
	"github.com/reconcilyx/operator/internal/engine"
	"github.com/reconcilyx/operator/internal/engine/rl"
	"github.com/reconcilyx/operator/internal/federated"
	"github.com/reconcilyx/operator/internal/notifiers"
	"github.com/reconcilyx/operator/internal/telemetry"
	reconcilyxwebhook "github.com/reconcilyx/operator/internal/webhook"
	reconcilyxui "github.com/reconcilyx/operator/ui"
	_ "net/http/pprof"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(optimizationv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func parseIntEnv(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	var concurrency int
	var prometheusURL string
	var batchSize int
	var pprofAddr string
	var apiPort string
	var apiKey string
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.IntVar(&concurrency, "concurrency", parseIntEnv("RECONCILYX_CONCURRENCY", 4),
		"Maximum number of concurrent reconciliations per controller.")
	flag.StringVar(&prometheusURL, "prometheus-url", func() string {
		if v := os.Getenv("PROMETHEUS_URL"); v != "" {
			return v
		}
		return "http://prometheus-operated.monitoring.svc.cluster.local:9090"
	}(), "URL of the Prometheus server used for workload telemetry.")
	flag.IntVar(&batchSize, "batch-size", parseIntEnv("RECONCILYX_BATCH_SIZE", 10),
		"Maximum number of workloads analyzed per policy reconciliation pass.")
	flag.StringVar(&pprofAddr, "pprof-bind-address", func() string {
		if v := os.Getenv("PPROF_ADDR"); v != "" {
			return v
		}
		return ":6060"
	}(), "Bind address for pprof debug endpoints (disabled if empty).")
	flag.StringVar(&apiPort, "api-port", func() string {
		if v := os.Getenv("RECONCILYX_API_PORT"); v != "" {
			if v[0] != ':' {
				return ":" + v
			}
			return v
		}
		return ":8082"
	}(), "Bind address for the Reconcilyx REST API + embedded UI (disabled if empty).")
	flag.StringVar(&apiKey, "api-key", os.Getenv("RECONCILYX_API_KEY"),
		"Shared secret for API calls. The UI and all integrations must send X-API-Key. Unset = dev mode (open).")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("Disabling HTTP/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookTLSOpts := tlsOpts
	webhookServerOptions := webhook.Options{
		TLSOpts: webhookTLSOpts,
	}

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}

	webhookServer := webhook.NewServer(webhookServerOptions)

	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	options := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "fe787433.reconcilyx.io",
	}
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		options.WebhookServer = webhookServer
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "Failed to start manager")
		os.Exit(1)
	}

	analyzer, err := func() (engine.Analyzer, error) {
		carbonCfg := carbon.CarbonConfigFromEnv()
		cp := carbon.NewCarbonProvider(carbonCfg)
		setupLog.Info("Carbon provider configured", "provider", carbonCfg.Provider, "source", func() string {
			if it, err := cp.IntensityForRegion(context.Background(), "us-east-1"); err == nil {
				return it.Source
			}
			return "unknown"
		}())
		return controller.BuildDefaultAnalyzer(prometheusURL, cp)
	}()
	if err != nil {
		setupLog.Error(err, "Failed to build analyzer engine, will run without proposal auto-creation")
	}

	// --- Federated intelligence wiring -------------------------------------
	hubBaseURL := os.Getenv("RECONCILYX_HUB_URL")
	spokeClusterName := os.Getenv("RECONCILYX_CLUSTER_NAME")
	fedNamespace := os.Getenv("RECONCILYX_FED_NAMESPACE")
	if fedNamespace == "" {
		fedNamespace = "reconcilyx-system"
	}
	fedAggregator := federated.NewAggregator(0)

	pricingCfg := cloud.PricingConfigFromEnv()
	pricingProviders := cloud.AllPricingProviders(pricingCfg)
	setupLog.Info("Pricing providers configured",
		"count", len(pricingProviders),
		"aws_enabled", pricingCfg.AWSEnabled,
		"gcp_enabled", pricingCfg.GCPEnabled,
		"azure_enabled", pricingCfg.AzureEnabled,
		"cache_ttl_hours", pricingCfg.CacheTTL.Hours(),
	)
	for _, p := range pricingProviders {
		setupLog.Info("Loaded pricing provider", "name", p.Name())
	}
	pp := make([]cloud.PricingProvider, 0, len(pricingProviders))
	for _, p := range pricingProviders {
		pp = append(pp, p)
	}
	arbitrageAnalyzer := engine.NewArbitrageAnalyzerWithProviders(fedAggregator, pp)

	// --- Build shared learning/telemetry dependencies ---------------------
	var rlAgent rl.Agent
	var telemetryProvider telemetry.TelemetryProvider
	var carbonProvider carbon.Provider
	workloadRegion := engine.DefaultRegionFrom(analyzer)
	carbonWeight := engine.DefaultCarbonWeightFrom(analyzer)

	if analyzer != nil {
		if rl, ok := engine.RLAgentFrom(analyzer); ok {
			rlAgent = rl
		}
		carbonProvider = engine.DefaultCarbonProviderFrom(analyzer)
	}

	// Always build the telemetry provider (used by rollback manager too)
	if promClient, tErr := telemetry.NewPrometheusClient(prometheusURL); tErr == nil {
		retriable := telemetry.NewRetriableMetricsClient(promClient)
		telemetryProvider = telemetry.NewTelemetryProvider(retriable)
	}

	if carbonProvider == nil {
		carbonProvider = carbon.NewStaticProvider()
	}

	if rlAgent != nil {
		if hubBaseURL != "" {
			if spokeClusterName == "" {
				spokeClusterName = "spoke-local"
			}
			cfg := federated.EmitterConfig{
				HubBaseURL:  hubBaseURL,
				ClusterName: spokeClusterName,
				APIKey:      apiKey,
			}
			emitter := federated.NewEmitter(cfg)
			emitter.SetObserver(func(d federated.QTableDelta) {
				api.EmitFederatedDeltaSent(spokeClusterName)
			})
			rlAgent.SetFederatedEmitter(emitter)
			bootstrapCtx, bootstrapCancel := context.WithTimeout(context.Background(), 15*time.Second)
			snap, bErr := federated.FetchGlobalModel(bootstrapCtx, hubBaseURL, apiKey, nil)
			if bErr == nil && snap != nil && len(snap.Entries) > 0 {
				rlAgent.LoadGlobal(bootstrapCtx, snap)
				setupLog.Info("Federated bootstrap loaded global Q-table", "entries", len(snap.Entries), "version", snap.Version, "cluster", spokeClusterName)
				api.EmitFederatedBootstrapOutcome(spokeClusterName, "success")
			} else if bErr != nil {
				setupLog.Info("Federated bootstrap skipped (hub unreachable or empty)", "cluster", spokeClusterName, "error", safeErr(bErr))
				api.EmitFederatedBootstrapOutcome(spokeClusterName, "skipped")
			} else {
				api.EmitFederatedBootstrapOutcome(spokeClusterName, "empty")
			}
			bootstrapCancel()
			go func() {
				mgrCtx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if err := emitter.Run(mgrCtx); err != nil {
					setupLog.Error(err, "Federated emitter loop exited")
				}
			}()
			setupLog.Info("Federated spoke emitter started", "hub", hubBaseURL, "cluster", spokeClusterName)
		}
	} else if analyzer != nil {
		setupLog.Info("Analyzer does not expose RL agent; federated emitter disabled")
	}

	policyReconciler := &controller.OptimizationPolicyReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Analyzer:  analyzer,
		BatchSize: batchSize,
	}
	if err := policyReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "optimizationpolicy")
		os.Exit(1)
	}

	proposalReconciler := &controller.OptimizationProposalReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		RLAgent:           rlAgent,
		TelemetryProvider: telemetryProvider,
		CarbonProvider:    carbonProvider,
		WorkloadRegion:    workloadRegion,
		CarbonWeight:      carbonWeight,
	}
	if err := proposalReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "optimizationproposal")
		os.Exit(1)
	}

	// --- Notifiers + Audit + API + UI ------------------------------------
	dispatcher := notifiers.NewDispatcher()
	if slack := notifiers.NewSlackNotifierFromEnv(); slack != nil {
		dispatcher.Register(slack)
		setupLog.Info("Registered Slack notifier", "channel", slack.Channel)
	}
	if smtp := notifiers.NewEmailNotifierFromEnv(); smtp != nil {
		dispatcher.Register(smtp)
		setupLog.Info("Registered SMTP email notifier", "host", smtp.Host, "to", smtp.To)
	}
	ctx := ctrl.SetupSignalHandler()

	auditLog := notifiers.NewAuditLog(10000, mgr.GetClient())
	go auditLog.PersistToConfigMap(ctx, "reconcilyx-system", "reconcilyx-audit-buffer")

	if apiPort != "" {
		uiDist, uiOK := reconcilyxui.Dist()
		deps := api.ServerDeps{
			Client:              mgr.GetClient(),
			Analyzer:            analyzer,
			ArbitrageAnalyzer:   arbitrageAnalyzer,
			NotifierDispatcher:  dispatcher,
			AuditLog:            auditLog,
			UIDist:              uiDist,
			UIEnabled:           uiOK,
			FederatedAggregator: fedAggregator,
			FederatedNamespace:  fedNamespace,
			FederatedModelName:  federated.DefaultConfigMapName,
		}
		apiServer := api.NewServer(apiPort, apiKey, deps)
		go func() {
			if err := apiServer.Start(ctx); err != nil {
				setupLog.Error(err, "API + UI server terminated with error")
				os.Exit(1)
			}
		}()
		go func() {
			<-mgr.Elected()
			setupLog.V(1).Info("API server lifecycle coupled to manager election")
		}()
		setupLog.Info("Reconcilyx API + UI configured", "addr", apiPort, "ui_embedded", uiOK, "auth", func() string {
			if apiKey == "" {
				return "dev (unauthenticated — set RECONCILYX_API_KEY)"
			}
			return "api-key"
		}())
	}

	if pprofAddr != "" {
		setupLog.Info("Enabling pprof endpoints", "addr", pprofAddr)
		go func() {
			srv := &http.Server{Addr: pprofAddr, Handler: http.DefaultServeMux}
			if serveErr := srv.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
				setupLog.Error(serveErr, "pprof server failed")
			}
		}()
	}

	// +kubebuilder:scaffold:builder

	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		mgr.GetWebhookServer().Register("/mutate-v1-pod", &webhook.Admission{
			Handler: &reconcilyxwebhook.PodMutator{Decoder: admission.NewDecoder(mgr.GetScheme())},
		})
		mgr.GetWebhookServer().Register("/validate-v1-pod-eviction", &webhook.Admission{
			Handler: &reconcilyxwebhook.EvictionValidator{Decoder: admission.NewDecoder(mgr.GetScheme())},
		})
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting manager", "concurrency", concurrency, "prometheus", prometheusURL, "batchSize", batchSize)
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "Failed to run manager")
		os.Exit(1)
	}
}

func rlAgentFromAnalyzer(analyzer engine.Analyzer) (rl.Agent, bool) {
	if analyzer == nil {
		return nil, false
	}
	return engine.RLAgentFrom(analyzer)
}

func safeErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 240 {
		return s[:240] + "..."
	}
	return s
}
