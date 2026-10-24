package gitops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

const proposalAnnotation = "reconcilyx.io/last-proposal"

// ManifestPatch is the resolved repository file plus its patched contents.
type ManifestPatch struct {
	Path    string
	Content []byte
}

// ResolveManifest locates the Deployment manifest for the workload, applies
// the proposed requests to the tracked containers, and returns the complete
// file contents. It never fabricates a manifest: an existing file is required
// so a GitOps controller cannot receive a partial document that prunes fields.
func ResolveManifest(
	ctx context.Context,
	provider GitProvider,
	workload corev1.ObjectReference,
	proposed corev1.ResourceRequirements,
	proposedByContainer map[string]corev1.ResourceRequirements,
	current map[string]corev1.ResourceRequirements,
	manifestsPath, baseBranch, proposalName string,
) (*ManifestPatch, error) {
	if provider == nil {
		return nil, errors.New("gitops: provider is nil")
	}
	if len(current) == 0 {
		return nil, errors.New("gitops: current container resources are required to patch a manifest")
	}

	var tried []string
	for _, candidate := range manifestCandidates(workload, manifestsPath) {
		tried = append(tried, candidate)
		data, err := provider.GetFile(ctx, candidate, baseBranch)
		if err != nil {
			if errors.Is(err, ErrFileNotFound) {
				continue
			}
			return nil, fmt.Errorf("read manifest %s: %w", candidate, err)
		}

		patched, found, err := patchDeploymentManifest(data, workload, proposed, proposedByContainer, current, proposalName)
		if err != nil {
			return nil, fmt.Errorf("patch manifest %s: %w", candidate, err)
		}
		if !found {
			continue
		}
		return &ManifestPatch{Path: candidate, Content: patched}, nil
	}

	return nil, fmt.Errorf("no Deployment manifest for %s/%s found under %q (tried %s)",
		workload.Namespace, workload.Name, defaultManifestsPath(manifestsPath), strings.Join(tried, ", "))
}

func defaultManifestsPath(manifestsPath string) string {
	cleaned := path.Clean(strings.TrimSpace(manifestsPath))
	if cleaned == "" || cleaned == "." {
		return "manifests"
	}
	return cleaned
}

func manifestCandidates(workload corev1.ObjectReference, manifestsPath string) []string {
	base := defaultManifestsPath(manifestsPath)
	if strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".yml") {
		return []string{base}
	}

	name := workload.Name
	candidates := []string{
		path.Join(base, name+".yaml"),
		path.Join(base, name+".yml"),
		path.Join(base, name, "deployment.yaml"),
		path.Join(base, name, "deployment.yml"),
	}
	if workload.Kind == "Deployment" && name == "" {
		candidates = []string{path.Join(base, "deployment.yaml"), path.Join(base, "deployment.yml")}
	}

	seen := make(map[string]struct{}, len(candidates))
	out := candidates[:0]
	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func patchDeploymentManifest(
	data []byte,
	workload corev1.ObjectReference,
	proposed corev1.ResourceRequirements,
	proposedByContainer map[string]corev1.ResourceRequirements,
	current map[string]corev1.ResourceRequirements,
	proposalName string,
) ([]byte, bool, error) {
	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	var docs []map[string]interface{}
	found := false

	for {
		doc := map[string]interface{}{}
		err := decoder.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, false, fmt.Errorf("decode YAML: %w", err)
		}
		if len(doc) == 0 {
			continue
		}
		if isTargetDeployment(doc, workload) {
			if err := applyProposedResources(doc, proposed, proposedByContainer, current); err != nil {
				return nil, false, err
			}
			annotateDeployment(doc, proposalName)
			found = true
		}
		docs = append(docs, doc)
	}

	if !found {
		return data, false, nil
	}

	var out bytes.Buffer
	for i, doc := range docs {
		encoded, err := yaml.Marshal(doc)
		if err != nil {
			return nil, false, fmt.Errorf("encode YAML: %w", err)
		}
		if i > 0 {
			out.WriteString("---\n")
		}
		out.Write(encoded)
	}
	return out.Bytes(), true, nil
}

func isTargetDeployment(doc map[string]interface{}, workload corev1.ObjectReference) bool {
	kind, _ := doc["kind"].(string)
	if !strings.EqualFold(kind, "Deployment") {
		return false
	}
	meta, ok := doc["metadata"].(map[string]interface{})
	if !ok {
		return false
	}
	name, _ := meta["name"].(string)
	if name != workload.Name {
		return false
	}
	if ns, _ := meta["namespace"].(string); ns != "" && workload.Namespace != "" && ns != workload.Namespace {
		return false
	}
	return true
}

func annotateDeployment(doc map[string]interface{}, proposalName string) {
	if proposalName == "" {
		return
	}
	meta, ok := doc["metadata"].(map[string]interface{})
	if !ok {
		meta = map[string]interface{}{}
		doc["metadata"] = meta
	}
	annotations, ok := meta["annotations"].(map[string]interface{})
	if !ok {
		annotations = map[string]interface{}{}
		meta["annotations"] = annotations
	}
	annotations[proposalAnnotation] = proposalName
}

func applyProposedResources(doc map[string]interface{}, proposed corev1.ResourceRequirements, proposedByContainer map[string]corev1.ResourceRequirements, current map[string]corev1.ResourceRequirements) error {
	spec, ok := doc["spec"].(map[string]interface{})
	if !ok {
		return errors.New("Deployment has no spec")
	}
	template, ok := spec["template"].(map[string]interface{})
	if !ok {
		return errors.New("Deployment has no pod template")
	}
	podSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		return errors.New("Deployment has no pod spec")
	}
	containers, ok := podSpec["containers"].([]interface{})
	if !ok || len(containers) == 0 {
		return errors.New("Deployment has no containers")
	}

	patched := 0
	for _, raw := range containers {
		container, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := container["name"].(string)
		if _, tracked := current[name]; !tracked {
			continue
		}
		desired, selected := selectProposedResources(proposed, proposedByContainer, name)
		if !selected {
			continue
		}
		resources, ok := container["resources"].(map[string]interface{})
		if !ok {
			resources = map[string]interface{}{}
			container["resources"] = resources
		}
		if err := mergeResourceList(resources, "requests", desired.Requests); err != nil {
			return fmt.Errorf("container %s requests: %w", name, err)
		}
		// A request above its limit is rejected by the API server. Raise the
		// limit only when the new request would otherwise be invalid.
		if err := raiseLimitsForRequests(resources, desired.Requests); err != nil {
			return fmt.Errorf("container %s limits: %w", name, err)
		}
		patched++
	}

	if patched == 0 {
		return fmt.Errorf("none of the proposed containers %s are present in the Deployment manifest", trackedContainerNames(current))
	}
	return nil
}

func selectProposedResources(proposed corev1.ResourceRequirements, proposedByContainer map[string]corev1.ResourceRequirements, container string) (corev1.ResourceRequirements, bool) {
	if len(proposedByContainer) > 0 {
		resources, ok := proposedByContainer[container]
		return resources, ok
	}
	return proposed, true
}

func mergeResourceList(resources map[string]interface{}, field string, quantities corev1.ResourceList) error {
	if len(quantities) == 0 {
		return nil
	}
	list, ok := resources[field].(map[string]interface{})
	if !ok {
		list = map[string]interface{}{}
		resources[field] = list
	}
	for name, quantity := range quantities {
		q := quantity.DeepCopy()
		list[string(name)] = q.String()
	}
	return nil
}

func raiseLimitsForRequests(resources map[string]interface{}, requests corev1.ResourceList) error {
	limits, ok := resources["limits"].(map[string]interface{})
	if !ok {
		return nil
	}
	for name, request := range requests {
		rawLimit, exists := limits[string(name)]
		if !exists {
			continue
		}
		limit, err := parseQuantity(rawLimit)
		if err != nil {
			return fmt.Errorf("parse %s limit: %w", name, err)
		}
		if limit.Cmp(request) < 0 {
			q := request.DeepCopy()
			limits[string(name)] = q.String()
		}
	}
	return nil
}

func parseQuantity(value interface{}) (resource.Quantity, error) {
	return resource.ParseQuantity(fmt.Sprint(value))
}

func trackedContainerNames(current map[string]corev1.ResourceRequirements) string {
	names := make([]string, 0, len(current))
	for name := range current {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}
