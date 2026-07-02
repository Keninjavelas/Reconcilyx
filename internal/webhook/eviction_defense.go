package webhook

import (
	"context"
	"encoding/json"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type PodMutator struct {
	Decoder admission.Decoder
}

// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=Ignore,sideEffects=None,groups="",resources=pods,verbs=create;update,versions=v1,name=mpod.reconcilyx.io,admissionReviewVersions=v1

func (m *PodMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}
	if err := m.Decoder.Decode(req, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// 1. Check if defense mode is requested.
	if pod.Annotations == nil || pod.Annotations["reconcilyx.io/defense-mode"] != "active" {
		return admission.Allowed("Defense mode inactive")
	}

	// 2. Store original limits before we override them.
	originalLimits := map[string]string{}
	shouldMutate := false
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		if container.Resources.Limits == nil {
			continue
		}
		
		// Save original limits to annotation.
		if cpu, ok := container.Resources.Limits[corev1.ResourceCPU]; ok {
			originalLimits["cpu"] = cpu.String()
		}
		if mem, ok := container.Resources.Limits[corev1.ResourceMemory]; ok {
			originalLimits["memory"] = mem.String()
		}

		// 3. Force Guaranteed QoS: set Limits == Requests.
		if container.Resources.Requests == nil {
			container.Resources.Requests = corev1.ResourceList{}
		}
		// Copy limits to requests.
		for k, v := range container.Resources.Limits {
			if k == corev1.ResourceCPU || k == corev1.ResourceMemory {
				container.Resources.Requests[k] = v.DeepCopy()
				shouldMutate = true
			}
		}
	}

	if !shouldMutate {
		return admission.Allowed("No limits found to enforce")
	}

	// 4. Marshal original limits into annotation.
	origLimitsJSON, _ := json.Marshal(originalLimits)
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations["reconcilyx.io/original-limits"] = string(origLimitsJSON)

	// 5. Return mutated pod.
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

type EvictionValidator struct {
	Decoder admission.Decoder
}

// +kubebuilder:webhook:path=/validate-v1-pod-eviction,mutating=false,failurePolicy=Ignore,sideEffects=None,groups="",resources=pods/eviction,verbs=create,versions=v1,name=vepod.reconcilyx.io,admissionReviewVersions=v1

func (v *EvictionValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Decode the pod (this is the pod being evicted).
	pod := &corev1.Pod{}
	if err := v.Decoder.Decode(req, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Check defense mode annotation.
	if pod.Annotations == nil || pod.Annotations["reconcilyx.io/defense-mode"] != "active" {
		return admission.Allowed("Pod not under active defense")
	}

	// Check if the optimization is still in progress (Executing or Stabilizing).
	// We need to query the OptimizationProposal status on the Hub.
	// For simplicity, we check a local annotation set by the agent:
	// "reconcilyx.io/optimization-phase".
	if phase := pod.Annotations["reconcilyx.io/optimization-phase"]; phase == "Executing" || phase == "Stabilizing" {
		// Block the eviction.
		return admission.Denied("Blocked: Pod is currently undergoing Reconcilyx optimization. Wait for stabilization.")
	}

	return admission.Allowed("Optimization phase stable or completed")
}
