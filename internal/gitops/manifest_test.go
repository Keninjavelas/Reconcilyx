package gitops

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func testWorkload() corev1.ObjectReference {
	return corev1.ObjectReference{Kind: "Deployment", Namespace: "default", Name: "demo"}
}

func TestResolveManifest_RaisesLimitWhenRequestExceedsIt(t *testing.T) {
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: default
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: 100m
            limits:
              cpu: 100m
`
	provider := NewMockProvider()
	provider.Files["apps/demo.yaml"] = []byte(manifest)

	proposed := corev1.ResourceRequirements{Requests: corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("500m"),
	}}
	current := map[string]corev1.ResourceRequirements{
		"app": {Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")}},
	}

	patch, err := ResolveManifest(context.Background(), provider, testWorkload(), proposed, current, "apps", "main", "p1")
	require.NoError(t, err)
	require.NotNil(t, patch)
	assert.Equal(t, "apps/demo.yaml", patch.Path)
	assert.Contains(t, string(patch.Content), "cpu: 500m")
	// Both request and limit must be 500m: a request above its limit is invalid.
	assert.Equal(t, 2, strings.Count(string(patch.Content), "cpu: 500m"))
}

func TestResolveManifest_SelectsDocumentInMultiDocFile(t *testing.T) {
	manifest := `apiVersion: v1
kind: Service
metadata:
  name: demo
spec:
  ports:
    - port: 80
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: default
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: 1
`
	provider := NewMockProvider()
	provider.Files["apps/demo.yaml"] = []byte(manifest)
	proposed := corev1.ResourceRequirements{Requests: corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("250m"),
	}}
	current := map[string]corev1.ResourceRequirements{
		"app": {Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}},
	}

	patch, err := ResolveManifest(context.Background(), provider, testWorkload(), proposed, current, "apps", "main", "p1")
	require.NoError(t, err)
	assert.Contains(t, string(patch.Content), "kind: Service")
	assert.Contains(t, string(patch.Content), "kind: Deployment")
	assert.Contains(t, string(patch.Content), "cpu: 250m")
}

func TestResolveManifest_IgnoresDeploymentWithOtherNamespace(t *testing.T) {
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: production
spec:
  template:
    spec:
      containers:
        - name: app
`
	provider := NewMockProvider()
	provider.Files["apps/demo.yaml"] = []byte(manifest)
	current := map[string]corev1.ResourceRequirements{"app": {}}

	_, err := ResolveManifest(context.Background(), provider, testWorkload(), corev1.ResourceRequirements{}, current, "apps", "main", "p1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Deployment manifest")
}

func TestResolveManifest_RequiresCurrentResources(t *testing.T) {
	provider := NewMockProvider()
	provider.Files["apps/demo.yaml"] = []byte(testManifest)
	_, err := ResolveManifest(context.Background(), provider, testWorkload(), corev1.ResourceRequirements{}, nil, "apps", "main", "p1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "current container resources")
}

func TestManifestCandidates(t *testing.T) {
	workload := testWorkload()
	assert.Equal(t, []string{"apps/demo.yaml", "apps/demo.yml", "apps/demo/deployment.yaml", "apps/demo/deployment.yml"},
		manifestCandidates(workload, "apps/"))
	assert.Equal(t, []string{"custom/path.yaml"}, manifestCandidates(workload, "custom/path.yaml"))
	assert.Equal(t, []string{"manifests/demo.yaml", "manifests/demo.yml", "manifests/demo/deployment.yaml", "manifests/demo/deployment.yml"},
		manifestCandidates(workload, ""))
}
