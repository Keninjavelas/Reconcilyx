package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/reconcilyx/operator/internal/carbon"
	"github.com/reconcilyx/operator/internal/engine"
)

type manifestContainer struct {
	Name             string `json:"name"`
	Init             bool   `json:"init,omitempty"`
	CPUMilli         int64  `json:"cpuMilli"`
	MemoryBytes      int64  `json:"memoryBytes"`
	HasCPURequest    bool   `json:"hasCpuRequest"`
	HasMemoryRequest bool   `json:"hasMemoryRequest"`
	CPULimitMilli    *int64 `json:"cpuLimitMilli,omitempty"`
	MemoryLimitBytes *int64 `json:"memoryLimitBytes,omitempty"`
}

type manifestWorkload struct {
	Key                string              `json:"key"`
	Kind               string              `json:"kind"`
	Namespace          string              `json:"namespace"`
	Name               string              `json:"name"`
	Source             string              `json:"source"`
	Replicas           int32               `json:"replicas"`
	ReplicasEstimated  bool                `json:"replicasEstimated,omitempty"`
	PodCPUMilli        int64               `json:"podCpuMilli"`
	PodMemoryBytes     int64               `json:"podMemoryBytes"`
	MonthlyCostUSD     float64             `json:"monthlyCostUsd"`
	MonthlyCarbonGrams float64             `json:"monthlyCarbonGrams"`
	Containers         []manifestContainer `json:"containers,omitempty"`
}

type workloadSide struct {
	Source             string              `json:"source,omitempty"`
	Replicas           int32               `json:"replicas"`
	ReplicasEstimated  bool                `json:"replicasEstimated,omitempty"`
	PodCPUMilli        int64               `json:"podCpuMilli"`
	PodMemoryBytes     int64               `json:"podMemoryBytes"`
	MonthlyCostUSD     float64             `json:"monthlyCostUsd"`
	MonthlyCarbonGrams float64             `json:"monthlyCarbonGrams"`
	Containers         []manifestContainer `json:"containers,omitempty"`
}

type containerDiff struct {
	Name         string `json:"name"`
	Init         bool   `json:"init,omitempty"`
	Status       string `json:"status"`
	CPUBefore    string `json:"cpuBefore,omitempty"`
	CPUAfter     string `json:"cpuAfter,omitempty"`
	MemoryBefore string `json:"memoryBefore,omitempty"`
	MemoryAfter  string `json:"memoryAfter,omitempty"`
}

type workloadDiff struct {
	Key                 string          `json:"key"`
	Kind                string          `json:"kind"`
	Namespace           string          `json:"namespace"`
	Name                string          `json:"name"`
	Status              string          `json:"status"`
	Before              *workloadSide   `json:"before,omitempty"`
	After               *workloadSide   `json:"after,omitempty"`
	MonthlyCostDeltaUSD float64         `json:"monthlyCostDeltaUsd"`
	CarbonDeltaGrams    float64         `json:"carbonDeltaGrams"`
	Containers          []containerDiff `json:"containers,omitempty"`
}

type diffViolation struct {
	Workload  string `json:"workload"`
	Container string `json:"container,omitempty"`
	Rule      string `json:"rule"`
	Message   string `json:"message"`
}

type manifestDiffSummary struct {
	WorkloadsBefore          int     `json:"workloadsBefore"`
	WorkloadsAfter           int     `json:"workloadsAfter"`
	Added                    int     `json:"added"`
	Removed                  int     `json:"removed"`
	Changed                  int     `json:"changed"`
	MonthlyCostBeforeUSD     float64 `json:"monthlyCostBeforeUsd"`
	MonthlyCostAfterUSD      float64 `json:"monthlyCostAfterUsd"`
	MonthlyCostDeltaUSD      float64 `json:"monthlyCostDeltaUsd"`
	MonthlyCarbonBeforeGrams float64 `json:"monthlyCarbonBeforeGrams"`
	MonthlyCarbonAfterGrams  float64 `json:"monthlyCarbonAfterGrams"`
	MonthlyCarbonDeltaGrams  float64 `json:"monthlyCarbonDeltaGrams"`
	GuardrailViolations      int     `json:"guardrailViolations"`
}

type manifestDiffReport struct {
	GeneratedAt               time.Time           `json:"generatedAt"`
	BaseRef                   string              `json:"baseRef"`
	ManifestsPath             string              `json:"manifestsPath"`
	Region                    string              `json:"region,omitempty"`
	CarbonIntensityGCO2PerKWh float64             `json:"carbonIntensityGCO2PerKWh"`
	Summary                   manifestDiffSummary `json:"summary"`
	Workloads                 []workloadDiff      `json:"workloads"`
	Violations                []diffViolation     `json:"violations"`
	Warnings                  []string            `json:"warnings,omitempty"`
}

func buildManifestDiffReport(manifestsPath, baseRef, region string, minReplicas int32, maxIncreasePercent float64, maxReplicas int32) (manifestDiffReport, error) {
	if strings.TrimSpace(manifestsPath) == "" {
		return manifestDiffReport{}, fmt.Errorf("diff failed: --manifests is required")
	}
	if strings.TrimSpace(baseRef) == "" {
		baseRef = "HEAD"
	}

	provider := carbon.NewStaticProvider()
	intensity, err := provider.IntensityForRegion(context.Background(), region)
	if err != nil {
		return manifestDiffReport{}, fmt.Errorf("diff failed: resolve carbon intensity: %w", err)
	}

	base, current, warnings, err := loadManifestWorkloads(manifestsPath, baseRef, intensity.Value)
	if err != nil {
		return manifestDiffReport{}, err
	}

	report := manifestDiffReport{
		GeneratedAt:               time.Now().UTC(),
		BaseRef:                   baseRef,
		ManifestsPath:             manifestsPath,
		Region:                    region,
		CarbonIntensityGCO2PerKWh: intensity.Value,
		Workloads:                 make([]workloadDiff, 0),
		Violations:                make([]diffViolation, 0),
		Warnings:                  warnings,
	}

	keys := make([]string, 0, len(base)+len(current))
	seen := map[string]bool{}
	for key := range base {
		keys = append(keys, key)
		seen[key] = true
	}
	for key := range current {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	for _, key := range keys {
		b, inBase := base[key]
		a, inCurrent := current[key]
		var diff workloadDiff
		switch {
		case inBase && inCurrent:
			diff = workloadDiff{
				Key:        key,
				Kind:       a.Kind,
				Namespace:  a.Namespace,
				Name:       a.Name,
				Before:     workloadSideFrom(b),
				After:      workloadSideFrom(a),
				Containers: diffContainers(b.Containers, a.Containers),
			}
			diff.MonthlyCostDeltaUSD = a.MonthlyCostUSD - b.MonthlyCostUSD
			diff.CarbonDeltaGrams = a.MonthlyCarbonGrams - b.MonthlyCarbonGrams
			diff.Status = "unchanged"
			if workloadChanged(b, a) {
				diff.Status = "changed"
				report.Summary.Changed++
			}
		case inCurrent:
			diff = workloadDiff{
				Key:                 key,
				Kind:                a.Kind,
				Namespace:           a.Namespace,
				Name:                a.Name,
				Status:              "added",
				After:               workloadSideFrom(a),
				Containers:          diffContainers(nil, a.Containers),
				MonthlyCostDeltaUSD: a.MonthlyCostUSD,
				CarbonDeltaGrams:    a.MonthlyCarbonGrams,
			}
			report.Summary.Added++
		default:
			diff = workloadDiff{
				Key:                 key,
				Kind:                b.Kind,
				Namespace:           b.Namespace,
				Name:                b.Name,
				Status:              "removed",
				Before:              workloadSideFrom(b),
				Containers:          diffContainers(b.Containers, nil),
				MonthlyCostDeltaUSD: -b.MonthlyCostUSD,
				CarbonDeltaGrams:    -b.MonthlyCarbonGrams,
			}
			report.Summary.Removed++
		}
		report.Workloads = append(report.Workloads, diff)
	}

	for _, w := range current {
		report.Violations = append(report.Violations, evaluateGuardrails(w, minReplicas, maxReplicas)...)
	}
	if maxIncreasePercent > 0 {
		for key, a := range current {
			b, ok := base[key]
			if !ok {
				continue
			}
			report.Violations = append(report.Violations, evaluateIncrease(key, b, a, maxIncreasePercent)...)
		}
	}
	sort.SliceStable(report.Violations, func(i, j int) bool {
		if report.Violations[i].Workload != report.Violations[j].Workload {
			return report.Violations[i].Workload < report.Violations[j].Workload
		}
		if report.Violations[i].Container != report.Violations[j].Container {
			return report.Violations[i].Container < report.Violations[j].Container
		}
		return report.Violations[i].Rule < report.Violations[j].Rule
	})
	report.Summary.GuardrailViolations = len(report.Violations)

	report.Summary.WorkloadsBefore = len(base)
	report.Summary.WorkloadsAfter = len(current)
	for _, w := range base {
		report.Summary.MonthlyCostBeforeUSD += w.MonthlyCostUSD
		report.Summary.MonthlyCarbonBeforeGrams += w.MonthlyCarbonGrams
	}
	for _, w := range current {
		report.Summary.MonthlyCostAfterUSD += w.MonthlyCostUSD
		report.Summary.MonthlyCarbonAfterGrams += w.MonthlyCarbonGrams
	}
	report.Summary.MonthlyCostDeltaUSD = report.Summary.MonthlyCostAfterUSD - report.Summary.MonthlyCostBeforeUSD
	report.Summary.MonthlyCarbonDeltaGrams = report.Summary.MonthlyCarbonAfterGrams - report.Summary.MonthlyCarbonBeforeGrams

	sort.SliceStable(report.Workloads, func(i, j int) bool {
		ri, rj := workloadStatusRank(report.Workloads[i].Status), workloadStatusRank(report.Workloads[j].Status)
		if ri != rj {
			return ri < rj
		}
		return report.Workloads[i].Key < report.Workloads[j].Key
	})
	return report, nil
}

func workloadStatusRank(status string) int {
	switch status {
	case "changed":
		return 0
	case "added":
		return 1
	case "removed":
		return 2
	default:
		return 3
	}
}

func workloadSideFrom(w manifestWorkload) *workloadSide {
	return &workloadSide{
		Source:             w.Source,
		Replicas:           w.Replicas,
		ReplicasEstimated:  w.ReplicasEstimated,
		PodCPUMilli:        w.PodCPUMilli,
		PodMemoryBytes:     w.PodMemoryBytes,
		MonthlyCostUSD:     w.MonthlyCostUSD,
		MonthlyCarbonGrams: w.MonthlyCarbonGrams,
		Containers:         w.Containers,
	}
}

func workloadChanged(before, after manifestWorkload) bool {
	if before.Replicas != after.Replicas ||
		before.PodCPUMilli != after.PodCPUMilli ||
		before.PodMemoryBytes != after.PodMemoryBytes ||
		len(before.Containers) != len(after.Containers) {
		return true
	}
	for i := range before.Containers {
		if before.Containers[i].Name != after.Containers[i].Name ||
			before.Containers[i].Init != after.Containers[i].Init ||
			before.Containers[i].CPUMilli != after.Containers[i].CPUMilli ||
			before.Containers[i].MemoryBytes != after.Containers[i].MemoryBytes {
			return true
		}
	}
	return false
}

func diffContainers(before, after []manifestContainer) []containerDiff {
	type keyedContainer struct {
		init bool
		name string
	}
	makeKey := func(c manifestContainer) keyedContainer { return keyedContainer{init: c.Init, name: c.Name} }

	beforeMap := make(map[keyedContainer]manifestContainer, len(before))
	afterMap := make(map[keyedContainer]manifestContainer, len(after))
	keys := []keyedContainer{}
	seen := map[keyedContainer]bool{}
	for _, c := range before {
		k := makeKey(c)
		beforeMap[k] = c
		if !seen[k] {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	for _, c := range after {
		k := makeKey(c)
		afterMap[k] = c
		if !seen[k] {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i].init != keys[j].init {
			return !keys[i].init
		}
		return keys[i].name < keys[j].name
	})

	diffs := make([]containerDiff, 0, len(keys))
	for _, key := range keys {
		b, inBefore := beforeMap[key]
		a, inAfter := afterMap[key]
		d := containerDiff{Name: key.name, Init: key.init}
		switch {
		case inBefore && inAfter:
			d.CPUBefore = formatCPUMilli(b.CPUMilli)
			d.CPUAfter = formatCPUMilli(a.CPUMilli)
			d.MemoryBefore = formatMemoryBytes(b.MemoryBytes)
			d.MemoryAfter = formatMemoryBytes(a.MemoryBytes)
			if b.CPUMilli == a.CPUMilli && b.MemoryBytes == a.MemoryBytes {
				d.Status = "unchanged"
			} else {
				d.Status = "changed"
			}
		case inAfter:
			d.Status = "added"
			d.CPUAfter = formatCPUMilli(a.CPUMilli)
			d.MemoryAfter = formatMemoryBytes(a.MemoryBytes)
		default:
			d.Status = "removed"
			d.CPUBefore = formatCPUMilli(b.CPUMilli)
			d.MemoryBefore = formatMemoryBytes(b.MemoryBytes)
		}
		diffs = append(diffs, d)
	}
	return diffs
}

func evaluateGuardrails(w manifestWorkload, minReplicas, maxReplicas int32) []diffViolation {
	violations := []diffViolation{}
	add := func(container, rule, message string) {
		violations = append(violations, diffViolation{Workload: w.Key, Container: container, Rule: rule, Message: message})
	}

	if w.Kind != "DaemonSet" {
		if minReplicas > 0 && w.Replicas < minReplicas {
			add("", "replicas-below-minimum", fmt.Sprintf("workload has %d replica(s), below the minimum of %d", w.Replicas, minReplicas))
		}
		if maxReplicas > 0 && w.Replicas > maxReplicas {
			add("", "replicas-above-maximum", fmt.Sprintf("workload has %d replica(s), above the maximum of %d", w.Replicas, maxReplicas))
		}
	}
	if len(w.Containers) == 0 {
		add("", "no-containers", "workload has no containers")
		return violations
	}
	for _, c := range w.Containers {
		label := c.Name
		if c.Init {
			label = "init:" + c.Name
		}
		if !c.HasCPURequest {
			add(label, "missing-cpu-request", fmt.Sprintf("container %q has no CPU request", label))
		}
		if !c.HasMemoryRequest {
			add(label, "missing-memory-request", fmt.Sprintf("container %q has no memory request", label))
		}
		if c.HasCPURequest && c.CPULimitMilli != nil && c.CPUMilli > *c.CPULimitMilli {
			add(label, "cpu-request-exceeds-limit", fmt.Sprintf("container %q CPU request exceeds its limit", label))
		}
		if c.HasMemoryRequest && c.MemoryLimitBytes != nil && c.MemoryBytes > *c.MemoryLimitBytes {
			add(label, "memory-request-exceeds-limit", fmt.Sprintf("container %q memory request exceeds its limit", label))
		}
	}
	return violations
}

func evaluateIncrease(key string, before, after manifestWorkload, maxPercent float64) []diffViolation {
	var violations []diffViolation
	check := func(resourceName string, oldValue, newValue int64, unit string) {
		if oldValue <= 0 || newValue <= oldValue {
			return
		}
		increase := float64(newValue-oldValue) / float64(oldValue) * 100
		if increase > maxPercent {
			violations = append(violations, diffViolation{
				Workload: key,
				Rule:     "max-" + resourceName + "-increase",
				Message:  fmt.Sprintf("%s request increases by %.1f%% (%s -> %s), above the %.1f%% guardrail", resourceName, increase, formatResourceValue(oldValue, unit), formatResourceValue(newValue, unit), maxPercent),
			})
		}
	}
	check("cpu", before.PodCPUMilli, after.PodCPUMilli, "cpu")
	check("memory", before.PodMemoryBytes, after.PodMemoryBytes, "memory")
	return violations
}

func formatResourceValue(value int64, unit string) string {
	if unit == "cpu" {
		return formatCPUMilli(value)
	}
	return formatMemoryBytes(value)
}

func formatCPUMilli(milli int64) string {
	return resource.NewMilliQuantity(milli, resource.DecimalSI).String()
}

func formatMemoryBytes(bytes int64) string {
	return resource.NewQuantity(bytes, resource.BinarySI).String()
}

// --- Parsing ---------------------------------------------------------------

func parseWorkloadFile(source string, data []byte) ([]manifestWorkload, error) {
	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	workloads := []manifestWorkload{}
	for {
		var obj unstructured.Unstructured
		if err := decoder.Decode(&obj); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("%s: %w", source, err)
		}
		if len(obj.Object) == 0 {
			continue
		}
		kind := obj.GetKind()
		switch kind {
		case "Deployment", "StatefulSet", "DaemonSet":
		default:
			continue
		}

		name := obj.GetName()
		if name == "" {
			return nil, fmt.Errorf("%s: %s document has no metadata.name", source, kind)
		}
		namespace := obj.GetNamespace()
		if namespace == "" {
			namespace = "default"
		}

		replicas := int32(1)
		estimated := false
		if kind == "DaemonSet" {
			estimated = true
		} else if raw, found, err := unstructured.NestedInt64(obj.Object, "spec", "replicas"); err != nil {
			return nil, fmt.Errorf("%s: %s/%s: invalid spec.replicas: %w", source, namespace, name, err)
		} else if found {
			replicas = int32(raw)
		}

		regular, err := parseContainers(obj.Object, "containers", false)
		if err != nil {
			return nil, fmt.Errorf("%s: %s/%s: %w", source, namespace, name, err)
		}
		initContainers, err := parseContainers(obj.Object, "initContainers", true)
		if err != nil {
			return nil, fmt.Errorf("%s: %s/%s: %w", source, namespace, name, err)
		}
		containers := append(regular, initContainers...)

		regularCPU, regularMem := sumContainers(regular)
		initCPU, initMem := maxContainers(initContainers)
		podCPU := regularCPU
		if initCPU > podCPU {
			podCPU = initCPU
		}
		podMem := regularMem
		if initMem > podMem {
			podMem = initMem
		}

		workloads = append(workloads, manifestWorkload{
			Key:               fmt.Sprintf("%s/%s/%s", kind, namespace, name),
			Kind:              kind,
			Namespace:         namespace,
			Name:              name,
			Source:            source,
			Replicas:          replicas,
			ReplicasEstimated: estimated,
			PodCPUMilli:       podCPU,
			PodMemoryBytes:    podMem,
			Containers:        containers,
		})
	}
	return workloads, nil
}

func parseContainers(obj map[string]interface{}, field string, init bool) ([]manifestContainer, error) {
	raw, found, err := unstructured.NestedSlice(obj, "spec", "template", "spec", field)
	if err != nil {
		return nil, fmt.Errorf("invalid spec.template.spec.%s: %w", field, err)
	}
	if !found {
		return nil, nil
	}
	containers := make([]manifestContainer, 0, len(raw))
	for i, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid %s[%d]: expected an object", field, i)
		}
		container, err := parseContainer(m, init)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", field, i, err)
		}
		containers = append(containers, container)
	}
	return containers, nil
}

func parseContainer(obj map[string]interface{}, init bool) (manifestContainer, error) {
	name, _, err := unstructured.NestedString(obj, "name")
	if err != nil || name == "" {
		return manifestContainer{}, fmt.Errorf("container has no valid name")
	}
	resources, _, err := unstructured.NestedMap(obj, "resources")
	if err != nil {
		return manifestContainer{}, fmt.Errorf("container %q has invalid resources: %w", name, err)
	}
	requests, err := nestedQuantityMap(resources, "requests")
	if err != nil {
		return manifestContainer{}, fmt.Errorf("container %q requests: %w", name, err)
	}
	limits, err := nestedQuantityMap(resources, "limits")
	if err != nil {
		return manifestContainer{}, fmt.Errorf("container %q limits: %w", name, err)
	}

	c := manifestContainer{Name: name, Init: init}
	cpuReq, hasCPUReq := requests["cpu"]
	memReq, hasMemReq := requests["memory"]
	cpuLimit, hasCPULimit := limits["cpu"]
	memLimit, hasMemLimit := limits["memory"]
	c.HasCPURequest = hasCPUReq
	c.HasMemoryRequest = hasMemReq
	if hasCPUReq {
		c.CPUMilli = cpuReq.MilliValue()
	} else if hasCPULimit {
		c.CPUMilli = cpuLimit.MilliValue()
	}
	if hasMemReq {
		c.MemoryBytes = memReq.Value()
	} else if hasMemLimit {
		c.MemoryBytes = memLimit.Value()
	}
	if hasCPULimit {
		v := cpuLimit.MilliValue()
		c.CPULimitMilli = &v
	}
	if hasMemLimit {
		v := memLimit.Value()
		c.MemoryLimitBytes = &v
	}
	return c, nil
}

func nestedQuantityMap(parent map[string]interface{}, field string) (map[string]resource.Quantity, error) {
	out := map[string]resource.Quantity{}
	raw, found, err := unstructured.NestedMap(parent, field)
	if err != nil {
		return nil, err
	}
	if !found {
		return out, nil
	}
	for name, value := range raw {
		if value == nil {
			continue
		}
		text := fmt.Sprint(value)
		if strings.TrimSpace(text) == "" {
			continue
		}
		q, err := resource.ParseQuantity(text)
		if err != nil {
			return nil, fmt.Errorf("%s.%s: invalid quantity %q: %w", field, name, text, err)
		}
		out[name] = q
	}
	return out, nil
}

func sumContainers(containers []manifestContainer) (int64, int64) {
	var cpu, mem int64
	for _, c := range containers {
		cpu += c.CPUMilli
		mem += c.MemoryBytes
	}
	return cpu, mem
}

func maxContainers(containers []manifestContainer) (int64, int64) {
	var cpu, mem int64
	for _, c := range containers {
		if c.CPUMilli > cpu {
			cpu = c.CPUMilli
		}
		if c.MemoryBytes > mem {
			mem = c.MemoryBytes
		}
	}
	return cpu, mem
}

func priceWorkload(w *manifestWorkload, intensity float64) {
	w.MonthlyCostUSD = engine.ComputeCostSavedUSDPerMonth(w.PodCPUMilli, w.PodMemoryBytes, w.Replicas)
	w.MonthlyCarbonGrams = engine.ComputeCarbonSavedGramsPerMonth(w.PodCPUMilli, w.PodMemoryBytes, w.Replicas, intensity)
}

// --- Git-backed file loading ----------------------------------------------

type manifestFilePair struct {
	absPath   string
	inCurrent bool
	inBase    bool
}

func loadManifestWorkloads(manifestsPath, baseRef string, intensity float64) (map[string]manifestWorkload, map[string]manifestWorkload, []string, error) {
	absPath, err := filepath.Abs(manifestsPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("diff failed: resolve manifests path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("diff failed: manifests path %q: %w", manifestsPath, err)
	}
	startDir := absPath
	if !info.IsDir() {
		startDir = filepath.Dir(absPath)
	}

	if _, err := exec.LookPath("git"); err != nil {
		return nil, nil, nil, fmt.Errorf("diff failed: git is required for --base comparisons: %w", err)
	}
	root, err := gitTopLevel(startDir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("diff failed: %w", err)
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("diff failed: resolve path relative to git root: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, nil, nil, fmt.Errorf("diff failed: manifests path %q is outside the git repository %q", manifestsPath, root)
	}
	relSlash := filepath.ToSlash(rel)
	if relSlash == "" {
		relSlash = "."
	}

	if _, err := gitExec(startDir, "rev-parse", "--verify", baseRef+"^{commit}"); err != nil {
		return nil, nil, nil, fmt.Errorf("diff failed: base ref %q could not be resolved: %w", baseRef, err)
	}

	files := map[string]*manifestFilePair{}
	addFile := func(repoPath string, current bool) {
		pair, ok := files[repoPath]
		if !ok {
			pair = &manifestFilePair{}
			files[repoPath] = pair
		}
		if current {
			pair.inCurrent = true
			pair.absPath = filepath.Join(root, filepath.FromSlash(repoPath))
		} else {
			pair.inBase = true
		}
	}

	if info.IsDir() {
		err = filepath.WalkDir(absPath, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if !isManifestFile(path) {
				return nil
			}
			fileRel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			addFile(filepath.ToSlash(fileRel), true)
			return nil
		})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("diff failed: walk manifests: %w", err)
		}
	} else {
		if !isManifestFile(absPath) {
			return nil, nil, nil, fmt.Errorf("diff failed: %q is not a .yaml or .yml file", manifestsPath)
		}
		addFile(relSlash, true)
	}

	baseList, err := gitExec(root, "ls-tree", "-r", "--full-tree", "--name-only", baseRef, "--", relSlash)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("diff failed: list base manifests: %w", err)
	}
	for _, line := range strings.Split(string(baseList), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !isManifestFile(line) {
			continue
		}
		addFile(line, false)
	}

	baseWorkloads := map[string]manifestWorkload{}
	currentWorkloads := map[string]manifestWorkload{}
	warnings := []string{}
	warnedDaemonSet := map[string]bool{}

	repoPaths := make([]string, 0, len(files))
	for repoPath := range files {
		repoPaths = append(repoPaths, repoPath)
	}
	sort.Strings(repoPaths)

	for _, repoPath := range repoPaths {
		pair := files[repoPath]
		if pair.inBase {
			data, found, err := gitShowFile(startDir, baseRef, repoPath)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("diff failed: read %s at %s: %w", repoPath, baseRef, err)
			}
			if found {
				workloads, err := parseWorkloadFile(repoPath, data)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("diff failed: parse base file: %w", err)
				}
				for _, w := range workloads {
					priceWorkload(&w, intensity)
					baseWorkloads[w.Key] = w
					warnDaemonSet(&w, warnedDaemonSet, &warnings)
				}
			}
		}
		if pair.inCurrent {
			data, err := os.ReadFile(pair.absPath)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("diff failed: read %s: %w", pair.absPath, err)
			}
			workloads, err := parseWorkloadFile(repoPath, data)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("diff failed: parse current file: %w", err)
			}
			for _, w := range workloads {
				priceWorkload(&w, intensity)
				currentWorkloads[w.Key] = w
				warnDaemonSet(&w, warnedDaemonSet, &warnings)
			}
		}
	}
	return baseWorkloads, currentWorkloads, warnings, nil
}

func warnDaemonSet(w *manifestWorkload, seen map[string]bool, warnings *[]string) {
	if !w.ReplicasEstimated || seen[w.Key] {
		return
	}
	seen[w.Key] = true
	*warnings = append(*warnings, fmt.Sprintf("%s: DaemonSet cost/carbon assumes one pod because node count is unknown", w.Key))
}

func isManifestFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func gitTopLevel(dir string) (string, error) {
	out, err := gitExec(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not a git repository (or git is unavailable): %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", fmt.Errorf("git returned an empty repository root")
	}
	return filepath.Clean(root), nil
}

func gitShowFile(dir, ref, repoPath string) ([]byte, bool, error) {
	out, err := gitExec(dir, "show", ref+":"+repoPath)
	if err != nil {
		// A missing path at the base ref is expected for newly added files.
		return nil, false, nil
	}
	return out, true, nil
}

func gitExec(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return out, nil
}
