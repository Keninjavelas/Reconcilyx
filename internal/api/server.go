package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/engine"
	"github.com/reconcilyx/operator/internal/federated"
	"github.com/reconcilyx/operator/internal/notifiers"
)

type ServerDeps struct {
	Client              client.Client
	Analyzer            engine.Analyzer
	ArbitrageAnalyzer   *engine.ArbitrageAnalyzer
	NotifierDispatcher  *notifiers.Dispatcher
	AuditLog            *notifiers.AuditLog
	UIDist              fs.FS
	UIEnabled           bool
	FederatedAggregator *federated.Aggregator
	FederatedNamespace  string
	FederatedModelName  string
}

type Server struct {
	addr    string
	logger  *slog.Logger
	mux     *http.ServeMux
	handler http.Handler
	deps    ServerDeps
	apiKey  string
	srv     *http.Server

	fedAggregator *federated.Aggregator
	fedNamespace  string
	fedModelName  string
	modelRegistry *federated.ModelRegistry
}

func NewServer(addr, apiKey string, deps ServerDeps) *Server {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{AddSource: false})).With("component", "api-server")
	agg := deps.FederatedAggregator
	if agg == nil {
		agg = federated.NewAggregator(0)
	}
	ns := deps.FederatedNamespace
	if ns == "" {
		ns = "reconcilyx-system"
	}
	modelName := deps.FederatedModelName
	if modelName == "" {
		modelName = federated.DefaultConfigMapName
	}
	s := &Server{
		addr:          addr,
		logger:        logger,
		mux:           http.NewServeMux(),
		deps:          deps,
		apiKey:        apiKey,
		fedAggregator: agg,
		fedNamespace:  ns,
		fedModelName:  modelName,
		modelRegistry: federated.NewModelRegistry(),
	}
	if snap := agg.CurrentSnapshot(); snap != nil {
		s.registerSnapshot(snap)
	}
	s.routes()
	s.handler = RecoverMiddleware(
		LoggingMiddleware(logger,
			CORSMiddleware(parseCORSOrigins(os.Getenv("RECONCILYX_CORS_ORIGINS")),
				RequestIDMiddleware(
					APIKeyAuthMiddleware(apiKey, publicRoutes(), s.mux),
				),
			),
		),
	)
	return s
}

// parseCORSOrigins splits the comma-separated environment value, trims spaces
// and trailing slashes, and falls back to the local development origins.
func parseCORSOrigins(raw string) []string {
	origins := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		origin := strings.TrimRight(strings.TrimSpace(part), "/")
		if origin != "" {
			origins = append(origins, origin)
		}
	}
	if len(origins) == 0 {
		return []string{"http://localhost:5173", "http://localhost:8082"}
	}
	return origins
}

func publicRoutes() map[string]struct{} {
	return map[string]struct{}{
		"/healthz":     {},
		"/":            {},
		"/index.html":  {},
		"/favicon.svg": {},
	}
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "time": time.Now().UTC().Format(time.RFC3339)})
	})

	s.mux.HandleFunc("GET /api/proposals", s.handleListProposals)
	s.mux.HandleFunc("GET /api/proposals/{name}", s.handleGetProposal)
	s.mux.HandleFunc("POST /api/proposals/{name}/approve", s.handleApproveProposal)
	s.mux.HandleFunc("POST /api/proposals/{name}/reject", s.handleRejectProposal)
	s.mux.HandleFunc("POST /api/proposals/{name}/rollback", s.handleRollbackProposal)

	s.mux.HandleFunc("GET /api/policies", s.handleListPolicies)
	s.mux.HandleFunc("POST /api/policies", s.handleCreatePolicy)
	s.mux.HandleFunc("GET /api/policies/{name}", s.handleGetPolicy)

	s.mux.HandleFunc("GET /api/metrics/savings", s.handleSavingsMetrics)
	s.mux.HandleFunc("GET /api/forecast/{workload}", s.handleForecast)
	s.mux.HandleFunc("GET /api/anomalies", s.handleAnomalies)
	s.mux.HandleFunc("GET /api/clusters", s.handleListClusters)

	s.mux.HandleFunc("POST /api/what-if", s.handleWhatIf)

	s.mux.HandleFunc("GET /api/audit", s.handleAuditTail)

	s.mux.HandleFunc("POST /api/federated/deltas", s.handleFederatedDeltas)
	s.mux.HandleFunc("GET /api/federated/global-model", s.handleFederatedGlobalModel)
	s.mux.HandleFunc("GET /api/federated/releases", s.handleFederatedReleases)
	s.mux.HandleFunc("DELETE /api/federated/releases/canary", s.handleFederatedCanaryRollback)

	s.mux.HandleFunc("POST /api/chat", s.handleChat)

	s.mux.HandleFunc("/", s.handleStaticOrSPA)
}

// Handler returns the full middleware-wrapped http.Handler (useful for httptest).
func (s *Server) Handler() http.Handler { return s.handler }

// Start runs the listener; blocks until the server exits.
func (s *Server) Start(ctx context.Context) error {
	s.srv = &http.Server{
		Addr:              s.addr,
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	aggCtx, aggCancel := context.WithCancel(ctx)
	defer aggCancel()
	go s.runFederatedLoop(aggCtx)

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("Starting Reconcilyx API + UI server", "addr", s.addr, "ui_embedded", s.deps.UIEnabled, "auth", s.authStatus(), "federated", "enabled")
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Server) runFederatedLoop(ctx context.Context) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	snapshot := s.fedAggregator.CurrentSnapshot()
	if snapshot != nil && len(snapshot.Entries) == 0 && s.deps.Client != nil {
		if loaded, err := federated.LoadGlobalModel(ctx, s.deps.Client, s.fedNamespace, s.fedModelName); err == nil && len(loaded.Entries) > 0 {
			s.fedAggregator.LoadSnapshot(loaded)
			s.registerSnapshot(loaded)
			s.logger.Info("Restored global Q-table from ConfigMap", "entries", len(loaded.Entries), "version", loaded.Version, "namespace", s.fedNamespace, "name", s.fedModelName)
		}
	}
	syncGauges(s.fedAggregator)
	for {
		select {
		case <-ctx.Done():
			_ = s.compactAndPersist(context.Background())
			return
		case <-t.C:
			_ = s.compactAndPersist(ctx)
		}
	}
}

func (s *Server) compactAndPersist(ctx context.Context) error {
	snap := s.fedAggregator.CompactAndSnapshot()
	s.registerSnapshot(snap)
	syncGauges(s.fedAggregator)
	if s.deps.Client == nil || snap == nil {
		return nil
	}
	if err := federated.SaveGlobalModel(ctx, s.deps.Client, s.fedNamespace, s.fedModelName, snap); err != nil {
		s.logger.Error("Failed to persist global Q-table", "error", err.Error())
		return err
	}
	s.logger.Info("Persisted global Q-table snapshot", "version", snap.Version, "entries", len(snap.Entries))
	return nil
}

// registerSnapshot publishes an aggregated snapshot onto the matching rollout
// channel: converged models become stable, everything else stays canary-only.
func (s *Server) registerSnapshot(snap *federated.GlobalQTable) {
	if snap == nil || len(snap.Entries) == 0 {
		return
	}
	version := snap.VersionStr
	if version == "" {
		version = fmt.Sprintf("v%d", snap.Version)
	}
	tag := federated.ModelTagCanary
	if snap.IsStable {
		tag = federated.ModelTagStable
	}
	if err := s.modelRegistry.RegisterModel(version, tag, snap); err != nil {
		s.logger.Error("Failed to register federated model release", "version", version, "tag", tag, "error", err.Error())
		return
	}
	s.logger.Info("Registered federated model release", "version", version, "tag", tag, "entries", len(snap.Entries))
}

func syncGauges(a *federated.Aggregator) {
	snap := a.CurrentSnapshot()
	telemetryFederatedVersionGauge(float64(snap.Version))
	telemetryFederatedEntriesGauge(float64(len(snap.Entries)))
}

var telemetryFederatedVersionGauge = func(v float64) {}
var telemetryFederatedEntriesGauge = func(v float64) {}

func (s *Server) authStatus() string {
	if s.apiKey == "" {
		return "dev-unauthenticated (set RECONCILYX_API_KEY to enable)"
	}
	return "api-key"
}

// compile-time assertion: we use the types import somewhere so it's not removed by automated refactors.
var _ = optimizationv1alpha1.PhaseDetected
