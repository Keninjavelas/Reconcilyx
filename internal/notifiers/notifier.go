package notifiers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
)

// EventType enumerates notification kinds.
type EventType string

const (
	EventProposalCreated    EventType = "proposal.created"
	EventProposalApproved   EventType = "proposal.approved"
	EventProposalRejected   EventType = "proposal.rejected"
	EventProposalRolledBack EventType = "proposal.rolled_back"
	EventHighAnomaly        EventType = "anomaly.high"
)

// Event is the generic envelope passed to all notifiers.
type Event struct {
	Type      EventType
	Proposal  *optimizationv1alpha1.OptimizationProposal
	Actor     string
	Reason    string
	Extra     map[string]string
	Timestamp time.Time
}

// Notifier is the per-channel interface.
type Notifier interface {
	Name() string
	Notify(ctx context.Context, evt Event) error
}

// Dispatcher fans-out to registered notifiers. Failures are logged but not returned.
type Dispatcher struct {
	notifiers []Notifier
	Timeout   time.Duration
	Client    client.Client // optional, used by the AuditPersister
}

func NewDispatcher(notifiers ...Notifier) *Dispatcher {
	return &Dispatcher{notifiers: notifiers, Timeout: 5 * time.Second}
}

func (d *Dispatcher) Register(n Notifier) { d.notifiers = append(d.notifiers, n) }

func (d *Dispatcher) Dispatch(ctx context.Context, evt Event) error {
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}
	if d == nil {
		return nil
	}
	for _, n := range d.notifiers {
		n := n
		go func() {
			c, cancel := context.WithTimeout(ctx, d.Timeout)
			defer cancel()
			if err := n.Notify(c, evt); err != nil {
				fmt.Fprintf(os.Stderr, "[notifier=%s] dispatch failed: %v\n", n.Name(), err)
			}
		}()
	}
	return nil
}

// --- AuditEntry / AuditLog (in-memory ring buffer + persistence) --------------

type AuditEntry struct {
	Time      time.Time `json:"time"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Proposal  string    `json:"proposal,omitempty"`
	Namespace string    `json:"namespace,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	ClientIP  string    `json:"clientIP,omitempty"`
}

type AuditLog struct {
	mu      sync.RWMutex
	entries []AuditEntry
	cap     int
	k8s     client.Client
}

func NewAuditLog(capacity int, optionalK8sClient client.Client) *AuditLog {
	if capacity <= 0 {
		capacity = 10000
	}
	return &AuditLog{entries: make([]AuditEntry, 0, capacity), cap: capacity, k8s: optionalK8sClient}
}

func (a *AuditLog) Append(e AuditEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for len(a.entries) >= a.cap {
		a.entries = a.entries[1:]
	}
	a.entries = append(a.entries, e)
}

// Tail returns the last n entries, optionally filtered by proposal name substring.
func (a *AuditLog) Tail(n int, proposalFilter string) []AuditEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]AuditEntry, 0, n)
	for i := len(a.entries) - 1; i >= 0 && len(out) < n; i-- {
		e := a.entries[i]
		if proposalFilter != "" && !strings.Contains(e.Proposal, proposalFilter) {
			continue
		}
		out = append(out, e)
	}
	// reverse to chronological
	for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
		out[l], out[r] = out[r], out[l]
	}
	return out
}

// PersistToConfigMap snapshots recent entries to a ConfigMap every minute. Call in a goroutine.
func (a *AuditLog) PersistToConfigMap(ctx context.Context, namespace, name string) {
	t := time.NewTicker(1 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if a.k8s == nil {
				continue
			}
			_ = a.flushOnce(ctx, namespace, name)
		}
	}
}

func (a *AuditLog) flushOnce(ctx context.Context, namespace, name string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if len(a.entries) == 0 {
		return nil
	}
	tail := a.entries
	if len(tail) > 1000 {
		tail = tail[len(tail)-1000:]
	}
	raw, err := json.Marshal(tail)
	if err != nil {
		return err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Data:       map[string]string{"audit.json": string(raw)},
	}
	var existing corev1.ConfigMap
	err = a.k8s.Get(ctx, client.ObjectKeyFromObject(cm), &existing)
	switch {
	case err == nil:
		existing.Data = cm.Data
		return a.k8s.Update(ctx, &existing)
	case apierrors.IsNotFound(err):
		return a.k8s.Create(ctx, cm)
	default:
		return err
	}
}

// --- SlackNotifier ----------------------------------------------------------

type SlackNotifier struct {
	WebhookURL string
	Channel    string
	BaseURL    string
	HTTPClient *http.Client
}

func NewSlackNotifierFromEnv() *SlackNotifier {
	url := os.Getenv("SLACK_WEBHOOK_URL")
	if url == "" {
		return nil
	}
	return &SlackNotifier{
		WebhookURL: url,
		Channel:    envDefault("SLACK_CHANNEL", "#reconcilyx"),
		BaseURL:    envDefault("RECONCILYX_BASE_URL", "http://localhost:8082"),
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *SlackNotifier) Name() string { return "slack" }

func (s *SlackNotifier) Notify(ctx context.Context, evt Event) error {
	if s == nil || s.WebhookURL == "" {
		return nil
	}
	title, color := slackFormat(evt)
	link := ""
	if evt.Proposal != nil {
		link = fmt.Sprintf("<%s/proposals/%s?namespace=%s|Open in Reconcilyx UI>", s.BaseURL, evt.Proposal.Name, evt.Proposal.Namespace)
	}
	body := map[string]any{
		"channel": s.Channel,
		"attachments": []any{
			map[string]any{
				"title":     title,
				"color":     color,
				"ts":        evt.Timestamp.Unix(),
				"text":      slackText(evt) + "\n" + link,
				"mrkdwn_in": []string{"text"},
			},
		},
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.WebhookURL, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := s.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		slurp, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return fmt.Errorf("slack webhook %d: %s", res.StatusCode, string(slurp))
	}
	return nil
}

func slackFormat(evt Event) (title, color string) {
	switch evt.Type {
	case EventProposalCreated:
		return fmt.Sprintf("🔎 Proposal created (%s/%s)", safe(evt.Proposal), safeNs(evt.Proposal)), "#06B6D4"
	case EventProposalApproved:
		return fmt.Sprintf("✅ Proposal approved by %s", evt.Actor), "#22C55E"
	case EventProposalRejected:
		return fmt.Sprintf("🚫 Proposal rejected by %s", evt.Actor), "#EF4444"
	case EventProposalRolledBack:
		return fmt.Sprintf("⏪ Proposal rolled back by %s", evt.Actor), "#F97316"
	case EventHighAnomaly:
		return "⚠️ High-severity anomaly detected", "#EF4444"
	}
	return string(evt.Type), "#6366F1"
}

func slackText(evt Event) string {
	lines := []string{}
	if evt.Proposal != nil {
		p := evt.Proposal
		lines = append(lines, fmt.Sprintf("• Workload: `%s/%s (%s)`", p.Spec.TargetWorkload.Namespace, p.Spec.TargetWorkload.Name, p.Spec.TargetWorkload.Kind))
		lines = append(lines, fmt.Sprintf("• Recommendation: `%s`", p.Spec.Recommendation))
		if p.Spec.ExpectedImpact.EstimatedSavingsUSD != nil {
			lines = append(lines, fmt.Sprintf("• Estimated savings: $%.2f / month", p.Spec.ExpectedImpact.EstimatedSavingsUSD.AsApproximateFloat64()))
		}
		lines = append(lines, fmt.Sprintf("• Confidence %d / Risk %d", p.Spec.DecisionMetadata.ConfidenceScore, p.Spec.DecisionMetadata.RiskScore))
	}
	if evt.Reason != "" {
		lines = append(lines, fmt.Sprintf("• Reason: %s", evt.Reason))
	}
	return strings.Join(lines, "\n")
}

func safe(p *optimizationv1alpha1.OptimizationProposal) string {
	if p == nil {
		return ""
	}
	return p.Name
}
func safeNs(p *optimizationv1alpha1.OptimizationProposal) string {
	if p == nil {
		return ""
	}
	return p.Namespace
}

// --- SMTP Email Notifier ----------------------------------------------------

type EmailNotifier struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       []string
	BaseURL  string
}

func NewEmailNotifierFromEnv() *EmailNotifier {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		return nil
	}
	port, _ := strconv.Atoi(envDefault("SMTP_PORT", "587"))
	to := os.Getenv("RECONCILYX_EMAIL_TO")
	if to == "" {
		return nil
	}
	return &EmailNotifier{
		Host:     host,
		Port:     port,
		Username: os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASS"),
		From:     envDefault("RECONCILYX_EMAIL_FROM", "reconcilyx@local"),
		To:       strings.Split(to, ","),
		BaseURL:  envDefault("RECONCILYX_BASE_URL", "http://localhost:8082"),
	}
}

func (e *EmailNotifier) Name() string { return "smtp" }

func (e *EmailNotifier) Notify(ctx context.Context, evt Event) error {
	if e == nil || len(e.To) == 0 {
		return nil
	}
	subject, body := emailFormat(evt, e.BaseURL)
	msg := "From: " + e.From + "\r\n" +
		"To: " + strings.Join(e.To, ", ") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		body + "\r\n"
	addr := e.Host + ":" + strconv.Itoa(e.Port)
	var auth smtp.Auth
	if e.Username != "" {
		auth = smtp.PlainAuth("", e.Username, e.Password, e.Host)
	}
	// golang.org/x/net/context-aware smtp.SendMail isn't available universally; call in goroutine with ctx deadline handled already by dispatcher.
	return smtp.SendMail(addr, auth, e.From, e.To, []byte(msg))
}

func emailFormat(evt Event, baseURL string) (subject, body string) {
	title, _ := slackFormat(evt)
	subject = "[Reconcilyx] " + title
	body = title + "\n\n" + slackText(evt) + "\n\nOpen UI: " + baseURL
	return subject, body
}

func envDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
