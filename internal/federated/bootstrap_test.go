package federated

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchGlobalModel_EmptyURLReturnsError(t *testing.T) {
	_, err := FetchGlobalModel(context.Background(), "", "key", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HubBaseURL is empty")
}

func TestFetchGlobalModel_Success(t *testing.T) {
	expected := &GlobalQTable{
		Version:   17,
		CreatedAt: time.Now().UTC(),
		Entries: map[string]GlobalQEntry{
			"1|2|3|4|5|6": {QValue: 0.75, VisitCount: 42},
			"0|0|0|0|0|0": {QValue: -0.1, VisitCount: 1},
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/federated/global-model", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(expected))
	}))
	defer ts.Close()

	got, err := FetchGlobalModel(context.Background(), ts.URL, "", nil)
	require.NoError(t, err)
	assert.Equal(t, expected.Version, got.Version)
	assert.Equal(t, len(expected.Entries), len(got.Entries))
	for k, v := range expected.Entries {
		assert.InDelta(t, v.QValue, got.Entries[k].QValue, 1e-9)
		assert.Equal(t, v.VisitCount, got.Entries[k].VisitCount)
	}
}

func TestFetchGlobalModel_WithAPIKeyHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "supersecret", r.Header.Get("X-API-Key"))
		empty := &GlobalQTable{Version: 1, Entries: map[string]GlobalQEntry{}}
		require.NoError(t, json.NewEncoder(w).Encode(empty))
	}))
	defer ts.Close()

	_, err := FetchGlobalModel(context.Background(), ts.URL, "supersecret", nil)
	assert.NoError(t, err)
}

func TestFetchGlobalModel_UsesProvidedHTTPClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		empty := &GlobalQTable{Version: 1, Entries: map[string]GlobalQEntry{}}
		require.NoError(t, json.NewEncoder(w).Encode(empty))
	}))
	defer ts.Close()

	customClient := &http.Client{Timeout: 15 * time.Second}
	_, err := FetchGlobalModel(context.Background(), ts.URL, "", customClient)
	assert.NoError(t, err)
}

func TestFetchGlobalModel_HubRejects(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "maintenance", http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	_, err := FetchGlobalModel(context.Background(), ts.URL, "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hub rejected bootstrap")
	assert.Contains(t, err.Error(), "maintenance")
}

func TestFetchGlobalModel_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer ts.Close()

	_, err := FetchGlobalModel(context.Background(), ts.URL, "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode global model")
}

func TestFetchGlobalModel_NilEntriesBecomesEmptyMap(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := []byte(`{"version":5,"createdAt":"2025-01-01T00:00:00Z"}`)
		_, _ = w.Write(payload)
	}))
	defer ts.Close()

	got, err := FetchGlobalModel(context.Background(), ts.URL, "", nil)
	require.NoError(t, err)
	assert.NotNil(t, got.Entries)
	assert.Equal(t, 0, len(got.Entries))
	assert.Equal(t, int64(5), got.Version)
}

func TestFetchGlobalModel_ContextCancelled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := FetchGlobalModel(ctx, ts.URL, "", nil)
	assert.True(t, errors.Is(err, context.Canceled) || err != nil, "should error due to cancelled context")
}
