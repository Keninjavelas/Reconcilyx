package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var vpaGVK = schema.GroupVersionKind{
	Group:   "autoscaling.k8s.io",
	Version: "v1",
	Kind:    "VerticalPodAutoscaler",
}

var kedaScaledObjectGVK = schema.GroupVersionKind{
	Group:   "keda.sh",
	Version: "v1alpha1",
	Kind:    "ScaledObject",
}

// AutoscalerGate refuses execution when another autoscaler already owns the
// sizing of the target workload. Two controllers writing pod resources will
// otherwise fight, each reverting the other's work.
type AutoscalerGate interface {
	Check(ctx context.Context, workload corev1.ObjectReference) error
}

type autoscalerConflictGate struct {
	client client.Client
}

func NewAutoscalerConflictGate(c client.Client) AutoscalerGate {
	return &autoscalerConflictGate{client: c}
}

func (g *autoscalerConflictGate) Check(ctx context.Context, workload corev1.ObjectReference) error {
	if g == nil || g.client == nil {
		return nil
	}

	hpaConflicts, err := g.hpaConflicts(ctx, workload)
	if err != nil {
		return err
	}
	vpaConflicts, err := g.vpaConflicts(ctx, workload)
	if err != nil {
		return err
	}
	kedaConflicts, err := g.kedaConflicts(ctx, workload)
	if err != nil {
		return err
	}

	conflicts := append(hpaConflicts, vpaConflicts...)
	conflicts = append(conflicts, kedaConflicts...)
	if len(conflicts) == 0 {
		return nil
	}
	sort.Strings(conflicts)
	return fmt.Errorf("autoscaler conflict: %s", strings.Join(conflicts, "; "))
}

func (g *autoscalerConflictGate) hpaConflicts(ctx context.Context, workload corev1.ObjectReference) ([]string, error) {
	var list autoscalingv2.HorizontalPodAutoscalerList
	if err := g.client.List(ctx, &list, client.InNamespace(workload.Namespace)); err != nil {
		if meta.IsNoMatchError(err) || apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list HorizontalPodAutoscalers: %w", err)
	}

	var conflicts []string
	for i := range list.Items {
		hpa := &list.Items[i]
		if !strings.EqualFold(hpa.Spec.ScaleTargetRef.Kind, workloadKind(workload)) ||
			hpa.Spec.ScaleTargetRef.Name != workload.Name {
			continue
		}
		if metricName, hasConflict := hpaHasCPUMemoryMetric(hpa); hasConflict {
			conflicts = append(conflicts, fmt.Sprintf("HorizontalPodAutoscaler %s/%s targets %s/%s (%s)", hpa.Namespace, hpa.Name, workloadKind(workload), workload.Name, metricName))
		}
	}
	return conflicts, nil
}

// hpaHasCPUMemoryMetric returns true when an HPA scales on CPU or Memory
// (or specifies no explicit metrics). HPAs scaling exclusively on custom
// External, Object, or non-CPU/Memory Pods metrics do not fight vertical sizing.
func hpaHasCPUMemoryMetric(hpa *autoscalingv2.HorizontalPodAutoscaler) (string, bool) {
	if len(hpa.Spec.Metrics) == 0 {
		return "unspecified/default resource metric", true
	}
	for _, m := range hpa.Spec.Metrics {
		switch m.Type {
		case autoscalingv2.ResourceMetricSourceType:
			if m.Resource != nil && (m.Resource.Name == corev1.ResourceCPU || m.Resource.Name == corev1.ResourceMemory) {
				return "resource:" + string(m.Resource.Name), true
			}
		case autoscalingv2.ContainerResourceMetricSourceType:
			if m.ContainerResource != nil && (m.ContainerResource.Name == corev1.ResourceCPU || m.ContainerResource.Name == corev1.ResourceMemory) {
				return "container-resource:" + string(m.ContainerResource.Name), true
			}
		case autoscalingv2.PodsMetricSourceType:
			if m.Pods != nil {
				lower := strings.ToLower(m.Pods.Metric.Name)
				if strings.Contains(lower, "cpu") || strings.Contains(lower, "memory") {
					return "pods:" + m.Pods.Metric.Name, true
				}
			}
		}
	}
	return "", false
}

func (g *autoscalerConflictGate) vpaConflicts(ctx context.Context, workload corev1.ObjectReference) ([]string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(vpaGVK.GroupVersion().WithKind(vpaGVK.Kind + "List"))

	if err := g.client.List(ctx, list, client.InNamespace(workload.Namespace)); err != nil {
		if meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err) || apierrors.IsNotFound(err) {
			// The VPA CRD is not installed; there is nothing to conflict with.
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list VerticalPodAutoscalers: %w", err)
	}

	var conflicts []string
	for i := range list.Items {
		vpa := &list.Items[i]
		kind, _, _ := unstructured.NestedString(vpa.Object, "spec", "targetRef", "kind")
		name, _, _ := unstructured.NestedString(vpa.Object, "spec", "targetRef", "name")
		if !strings.EqualFold(kind, workloadKind(workload)) || name != workload.Name {
			continue
		}
		mode := vpaUpdateMode(vpa.Object)
		if strings.EqualFold(mode, "Off") {
			// Recommendation-only VPAs never write pod resources.
			continue
		}
		// Block Auto, Recreate, Initial (and unknown modes) safely.
		conflicts = append(conflicts, fmt.Sprintf("VerticalPodAutoscaler %s/%s actively manages %s/%s (updateMode=%s)", vpa.GetNamespace(), vpa.GetName(), workloadKind(workload), workload.Name, mode))
	}
	return conflicts, nil
}

func (g *autoscalerConflictGate) kedaConflicts(ctx context.Context, workload corev1.ObjectReference) ([]string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(kedaScaledObjectGVK.GroupVersion().WithKind(kedaScaledObjectGVK.Kind + "List"))

	if err := g.client.List(ctx, list, client.InNamespace(workload.Namespace)); err != nil {
		if meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err) || apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list KEDA ScaledObjects: %w", err)
	}

	var conflicts []string
	for i := range list.Items {
		so := &list.Items[i]
		kind, _, _ := unstructured.NestedString(so.Object, "spec", "scaleTargetRef", "kind")
		if kind == "" {
			kind = "Deployment"
		}
		name, _, _ := unstructured.NestedString(so.Object, "spec", "scaleTargetRef", "name")
		if !strings.EqualFold(kind, workloadKind(workload)) || name != workload.Name {
			continue
		}
		triggers, found, _ := unstructured.NestedSlice(so.Object, "spec", "triggers")
		if !found {
			continue
		}
		for _, raw := range triggers {
			tm, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			tType, _ := tm["type"].(string)
			lower := strings.ToLower(strings.TrimSpace(tType))
			if lower == "cpu" || lower == "memory" {
				conflicts = append(conflicts, fmt.Sprintf("ScaledObject %s/%s actively scales %s/%s on %s", so.GetNamespace(), so.GetName(), workloadKind(workload), workload.Name, lower))
			}
		}
	}
	return conflicts, nil
}

func vpaUpdateMode(obj map[string]interface{}) string {
	if mode, found, _ := unstructured.NestedString(obj, "spec", "updatePolicy", "updateMode"); found && mode != "" {
		return mode
	}
	if mode, found, _ := unstructured.NestedString(obj, "spec", "updateMode"); found && mode != "" {
		return mode
	}
	// VPA defaults to Auto when no update policy is specified.
	return "Auto"
}

func workloadKind(workload corev1.ObjectReference) string {
	if workload.Kind == "" {
		return "Deployment"
	}
	return workload.Kind
}
