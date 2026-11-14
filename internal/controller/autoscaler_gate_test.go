package controller

import (
	"context"
	"strings"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func autoscalerTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	s.AddKnownTypeWithName(vpaGVK, &unstructured.Unstructured{})
	s.AddKnownTypeWithName(vpaGVK.GroupVersion().WithKind(vpaGVK.Kind+"List"), &unstructured.UnstructuredList{})
	s.AddKnownTypeWithName(kedaScaledObjectGVK, &unstructured.Unstructured{})
	s.AddKnownTypeWithName(kedaScaledObjectGVK.GroupVersion().WithKind(kedaScaledObjectGVK.Kind+"List"), &unstructured.UnstructuredList{})
	return s
}

func newScaledObject(name, target, triggerType string) *unstructured.Unstructured {
	so := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "keda.sh/v1alpha1",
		"kind":       "ScaledObject",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": "default",
		},
		"spec": map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"kind": "Deployment",
				"name": target,
			},
			"triggers": []interface{}{
				map[string]interface{}{"type": triggerType},
			},
		},
	}}
	so.SetGroupVersionKind(kedaScaledObjectGVK)
	return so
}

func newHPACustomMetric(name string, ref autoscalingv2.CrossVersionObjectReference) *autoscalingv2.HorizontalPodAutoscaler {
	hpa := newHPA(name, ref)
	hpa.Spec.Metrics = []autoscalingv2.MetricSpec{{
		Type: autoscalingv2.ExternalMetricSourceType,
		External: &autoscalingv2.ExternalMetricSource{
			Metric: autoscalingv2.MetricIdentifier{Name: "sqs_queue_depth"},
		},
	}}
	return hpa
}

func newHPACPUResource(name string, ref autoscalingv2.CrossVersionObjectReference) *autoscalingv2.HorizontalPodAutoscaler {
	hpa := newHPA(name, ref)
	hpa.Spec.Metrics = []autoscalingv2.MetricSpec{{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name: corev1.ResourceCPU,
		},
	}}
	return hpa
}

func testWorkload() corev1.ObjectReference {
	return corev1.ObjectReference{Kind: "Deployment", Namespace: "default", Name: "demo"}
}

func newHPA(name string, ref autoscalingv2.CrossVersionObjectReference) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: ref,
			MaxReplicas:    5,
		},
	}
}

func newVPA(name, kind, target, updateMode string) *unstructured.Unstructured {
	vpa := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "autoscaling.k8s.io/v1",
		"kind":       "VerticalPodAutoscaler",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": "default",
		},
		"spec": map[string]interface{}{
			"targetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       kind,
				"name":       target,
			},
			"updatePolicy": map[string]interface{}{
				"updateMode": updateMode,
			},
		},
	}}
	vpa.SetGroupVersionKind(vpaGVK)
	return vpa
}

func TestAutoscalerConflictGate(t *testing.T) {
	deploymentRef := autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: "demo"}

	tests := []struct {
		name       string
		objects    []runtime.Object
		wantSubstr string
	}{
		{
			name:    "no autoscalers",
			objects: nil,
		},
		{
			name:       "HPA targeting the deployment",
			objects:    []runtime.Object{newHPA("demo-hpa", deploymentRef)},
			wantSubstr: "HorizontalPodAutoscaler default/demo-hpa",
		},
		{
			name: "HPA targeting another workload",
			objects: []runtime.Object{newHPA("other-hpa", autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1", Kind: "Deployment", Name: "other",
			})},
		},
		{
			name:       "VPA in Auto mode",
			objects:    []runtime.Object{newVPA("demo-vpa", "Deployment", "demo", "Auto")},
			wantSubstr: "VerticalPodAutoscaler default/demo-vpa",
		},
		{
			name:       "VPA with empty update mode defaults to Auto",
			objects:    []runtime.Object{newVPA("demo-vpa", "Deployment", "demo", "")},
			wantSubstr: "VerticalPodAutoscaler default/demo-vpa",
		},
		{
			name:    "VPA in Off mode is recommendation-only",
			objects: []runtime.Object{newVPA("demo-vpa", "Deployment", "demo", "Off")},
		},
		{
			name:    "VPA targeting another workload",
			objects: []runtime.Object{newVPA("other-vpa", "Deployment", "other", "Auto")},
		},
		{
			name: "HPA and VPA both conflict",
			objects: []runtime.Object{
				newHPA("demo-hpa", deploymentRef),
				newVPA("demo-vpa", "Deployment", "demo", "Recreate"),
			},
			wantSubstr: "VerticalPodAutoscaler default/demo-vpa",
		},
		{
			name:    "HPA scaling exclusively on external custom metric is allowed",
			objects: []runtime.Object{newHPACustomMetric("queue-hpa", deploymentRef)},
		},
		{
			name:       "HPA scaling on CPU resource metric conflicts",
			objects:    []runtime.Object{newHPACPUResource("cpu-hpa", deploymentRef)},
			wantSubstr: "HorizontalPodAutoscaler default/cpu-hpa",
		},
		{
			name:       "VPA in Initial mode conflicts",
			objects:    []runtime.Object{newVPA("init-vpa", "Deployment", "demo", "Initial")},
			wantSubstr: "VerticalPodAutoscaler default/init-vpa",
		},
		{
			name:       "KEDA ScaledObject with CPU trigger conflicts",
			objects:    []runtime.Object{newScaledObject("cpu-so", "demo", "cpu")},
			wantSubstr: "ScaledObject default/cpu-so",
		},
		{
			name:       "KEDA ScaledObject with Memory trigger conflicts",
			objects:    []runtime.Object{newScaledObject("mem-so", "demo", "memory")},
			wantSubstr: "ScaledObject default/mem-so",
		},
		{
			name:    "KEDA ScaledObject with Kafka trigger is allowed",
			objects: []runtime.Object{newScaledObject("kafka-so", "demo", "kafka")},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(autoscalerTestScheme(t)).WithRuntimeObjects(tc.objects...).Build()
			gate := NewAutoscalerConflictGate(c)
			err := gate.Check(context.Background(), testWorkload())
			if tc.wantSubstr == "" {
				if err != nil {
					t.Fatalf("expected no conflict, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected a conflict, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantSubstr)
			}
		})
	}
}

func TestAutoscalerConflictGate_NilClientIsNoop(t *testing.T) {
	gate := NewAutoscalerConflictGate(nil)
	if err := gate.Check(context.Background(), testWorkload()); err != nil {
		t.Fatalf("expected nil-client gate to be a no-op, got %v", err)
	}
}
