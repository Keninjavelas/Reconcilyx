package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const baseDeployment = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: prod
spec:
  replicas: 2
  template:
    spec:
      containers:
        - name: app
          image: example/web:1
          resources:
            requests:
              cpu: 500m
              memory: 512Mi
`

const resizedDeployment = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: prod
spec:
  replicas: 4
  template:
    spec:
      containers:
        - name: app
          image: example/web:2
          resources:
            requests:
              cpu: "1"
              memory: 1Gi
`

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestBuildManifestDiffReport(t *testing.T) {
	repo := newGitRepo(t)
	manifests := filepath.Join(repo, "manifests")
	workloadFile := filepath.Join(manifests, "app.yaml")
	writeFile(t, workloadFile, baseDeployment)
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "base")

	writeFile(t, workloadFile, resizedDeployment)
	report, err := buildManifestDiffReport(manifests, "HEAD", "", 1, 50, 3)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}

	if report.BaseRef != "HEAD" || report.CarbonIntensityGCO2PerKWh != 450 {
		t.Fatalf("unexpected report metadata: %+v", report)
	}
	if report.Summary.WorkloadsBefore != 1 || report.Summary.WorkloadsAfter != 1 || report.Summary.Changed != 1 {
		t.Fatalf("unexpected workload summary: %+v", report.Summary)
	}
	if got, want := report.Summary.MonthlyCostBeforeUSD, 43.80; !almostEqual(got, want) {
		t.Fatalf("cost before = %v, want %v", got, want)
	}
	if got, want := report.Summary.MonthlyCostAfterUSD, 175.20; !almostEqual(got, want) {
		t.Fatalf("cost after = %v, want %v", got, want)
	}
	if got, want := report.Summary.MonthlyCostDeltaUSD, 131.40; !almostEqual(got, want) {
		t.Fatalf("cost delta = %v, want %v", got, want)
	}
	if report.Summary.MonthlyCarbonDeltaGrams <= 0 {
		t.Fatalf("expected a carbon increase, got %v", report.Summary.MonthlyCarbonDeltaGrams)
	}

	if len(report.Workloads) != 1 {
		t.Fatalf("expected 1 workload, got %d", len(report.Workloads))
	}
	w := report.Workloads[0]
	if w.Status != "changed" || w.Kind != "Deployment" || w.Name != "web" || w.Namespace != "prod" {
		t.Fatalf("unexpected workload diff: %+v", w)
	}
	if w.Before == nil || w.After == nil || w.Before.Replicas != 2 || w.After.Replicas != 4 {
		t.Fatalf("replica diff wrong: %+v", w)
	}
	if len(w.Containers) != 1 || w.Containers[0].CPUAfter != "1" || w.Containers[0].MemoryAfter != "1Gi" {
		t.Fatalf("container diff wrong: %+v", w.Containers)
	}

	rules := map[string]bool{}
	for _, v := range report.Violations {
		rules[v.Rule] = true
	}
	for _, rule := range []string{"replicas-above-maximum", "max-cpu-increase", "max-memory-increase"} {
		if !rules[rule] {
			t.Fatalf("missing guardrail %q in %+v", rule, report.Violations)
		}
	}
	if report.Summary.GuardrailViolations != len(report.Violations) {
		t.Fatalf("violation count mismatch: %+v", report.Summary)
	}

	// Once committed, the working tree matches the base: no changes should be
	// reported and the earlier pointer-identity bug must not resurface.
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "resize")
	unchanged, err := buildManifestDiffReport(manifests, "HEAD", "", 1, 50, 3)
	if err != nil {
		t.Fatalf("diff after commit failed: %v", err)
	}
	if unchanged.Summary.Changed != 0 || unchanged.Summary.MonthlyCostDeltaUSD != 0 || unchanged.Workloads[0].Status != "unchanged" {
		t.Fatalf("expected unchanged workloads after commit: %+v", unchanged.Summary)
	}
}

func TestBuildManifestDiffReportRejectsBadInputs(t *testing.T) {
	repo := newGitRepo(t)
	manifests := filepath.Join(repo, "manifests")
	writeFile(t, filepath.Join(manifests, "app.yaml"), baseDeployment)
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "base")

	if _, err := buildManifestDiffReport("", "HEAD", "", 1, 0, 0); err == nil || !strings.Contains(err.Error(), "--manifests is required") {
		t.Fatalf("expected required-path error, got %v", err)
	}
	if _, err := buildManifestDiffReport(manifests, "does-not-exist", "", 1, 0, 0); err == nil || !strings.Contains(err.Error(), "could not be resolved") {
		t.Fatalf("expected base ref error, got %v", err)
	}
	if _, err := buildManifestDiffReport(filepath.Join(manifests, "missing.yaml"), "HEAD", "", 1, 0, 0); err == nil || !strings.Contains(err.Error(), "manifests path") {
		t.Fatalf("expected missing-path error, got %v", err)
	}
}

func TestBuildManifestDiffReportDaemonSetWarning(t *testing.T) {
	repo := newGitRepo(t)
	manifests := filepath.Join(repo, "manifests")
	writeFile(t, filepath.Join(manifests, "agent.yaml"), `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: agent
spec:
  template:
    spec:
      containers:
        - name: agent
          resources:
            requests:
              cpu: 100m
              memory: 64Mi
`)
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "daemonset")

	report, err := buildManifestDiffReport(manifests, "HEAD", "", 1, 0, 0)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
	if report.Summary.Added != 0 || report.Summary.Changed != 0 {
		t.Fatalf("identical daemonset must be unchanged: %+v", report.Summary)
	}
	if report.Workloads[0].After == nil || !report.Workloads[0].After.ReplicasEstimated {
		t.Fatalf("daemonset replicas should be marked estimated: %+v", report.Workloads[0])
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "assumes one pod") {
		t.Fatalf("expected daemonset warning, got %v", report.Warnings)
	}
}

func TestParseWorkloadFile(t *testing.T) {
	t.Run("init containers use max not sum", func(t *testing.T) {
		workloads, err := parseWorkloadFile("inline.yaml", []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: mixed
spec:
  template:
    spec:
      initContainers:
        - name: init
          resources:
            requests: {cpu: 2, memory: 1Gi}
      containers:
        - name: app
          resources:
            requests: {cpu: 500m, memory: 256Mi}
        - name: sidecar
          resources:
            requests: {cpu: 250m, memory: 128Mi}
`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if len(workloads) != 1 {
			t.Fatalf("expected 1 workload, got %d", len(workloads))
		}
		w := workloads[0]
		if w.Namespace != "default" || w.Replicas != 1 || w.ReplicasEstimated {
			t.Fatalf("defaults wrong: %+v", w)
		}
		// regular containers sum to 750m; the init container's 2000m dominates.
		if w.PodCPUMilli != 2000 {
			t.Fatalf("pod CPU = %d, want 2000", w.PodCPUMilli)
		}
		if w.PodMemoryBytes != 1024*1024*1024 {
			t.Fatalf("pod memory = %d, want 1Gi", w.PodMemoryBytes)
		}
		if len(w.Containers) != 3 || !w.Containers[0].HasCPURequest {
			t.Fatalf("containers not captured: %+v", w.Containers)
		}
	})

	t.Run("limits act as fallback requests", func(t *testing.T) {
		workloads, err := parseWorkloadFile("inline.yaml", []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: limits-only
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            limits: {cpu: 300m, memory: 128Mi}
`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		c := workloads[0].Containers[0]
		if c.HasCPURequest || c.HasMemoryRequest {
			t.Fatalf("missing requests must stay visible to guardrails: %+v", c)
		}
		if c.CPUMilli != 300 || c.MemoryBytes != 128*1024*1024 {
			t.Fatalf("limit fallback wrong: %+v", c)
		}
	})

	t.Run("other kinds are skipped", func(t *testing.T) {
		workloads, err := parseWorkloadFile("inline.yaml", []byte(`apiVersion: v1
kind: Service
metadata:
  name: svc
`))
		if err != nil || len(workloads) != 0 {
			t.Fatalf("expected no workloads, got %d (err %v)", len(workloads), err)
		}
	})

	errorCases := map[string]string{
		"missing name": `apiVersion: apps/v1
kind: Deployment
metadata: {}
`,
		"invalid request": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: bad
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            requests: {cpu: lots}
`,
		"missing container name": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: bad
spec:
  template:
    spec:
      containers:
        - resources:
            requests: {cpu: 100m}
`,
	}
	for name, body := range errorCases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseWorkloadFile("inline.yaml", []byte(body)); err == nil {
				t.Fatal("expected a parse error")
			}
		})
	}
}

func TestEvaluateGuardrails(t *testing.T) {
	limitCPU := int64(100)
	limitMem := int64(64 * 1024 * 1024)
	w := manifestWorkload{
		Key:      "Deployment/prod/web",
		Kind:     "Deployment",
		Replicas: 5,
		Containers: []manifestContainer{
			{Name: "no-requests"},
			{Name: "over-limit", CPUMilli: 200, MemoryBytes: 128 * 1024 * 1024, HasCPURequest: true, HasMemoryRequest: true, CPULimitMilli: &limitCPU, MemoryLimitBytes: &limitMem},
		},
	}
	violations := evaluateGuardrails(w, 2, 3)
	rules := map[string]int{}
	for _, v := range violations {
		rules[v.Rule]++
	}
	for _, rule := range []string{"replicas-above-maximum", "missing-cpu-request", "missing-memory-request", "cpu-request-exceeds-limit", "memory-request-exceeds-limit"} {
		if rules[rule] == 0 {
			t.Fatalf("missing %q in %+v", rule, violations)
		}
	}
	below := map[string]bool{}
	for _, v := range evaluateGuardrails(w, 6, 0) {
		below[v.Rule] = true
	}
	if !below["replicas-below-minimum"] {
		t.Fatal("expected replicas-below-minimum")
	}
	if below["replicas-above-maximum"] {
		t.Fatal("replicas-above-maximum must be disabled when --max-replicas=0")
	}

	daemon := manifestWorkload{Key: "DaemonSet/prod/agent", Kind: "DaemonSet", Replicas: 1, Containers: []manifestContainer{{Name: "a", HasCPURequest: true, HasMemoryRequest: true}}}
	if got := evaluateGuardrails(daemon, 5, 1); len(got) != 0 {
		t.Fatalf("daemonsets must not be subject to replica bounds: %+v", got)
	}
}

func TestEvaluateIncrease(t *testing.T) {
	before := manifestWorkload{PodCPUMilli: 100, PodMemoryBytes: 100}
	if got := evaluateIncrease("key", before, manifestWorkload{PodCPUMilli: 200, PodMemoryBytes: 250}, 50); len(got) != 2 {
		t.Fatalf("expected cpu and memory violations, got %+v", got)
	}
	if got := evaluateIncrease("key", before, manifestWorkload{PodCPUMilli: 120, PodMemoryBytes: 90}, 50); len(got) != 0 {
		t.Fatalf("expected no violations, got %+v", got)
	}
	if got := evaluateIncrease("key", manifestWorkload{}, manifestWorkload{PodCPUMilli: 500}, 10); len(got) != 0 {
		t.Fatalf("new workloads must not trigger increase guardrails: %+v", got)
	}
}

func almostEqual(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 0.0001
}
