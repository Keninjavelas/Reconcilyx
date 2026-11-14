package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackfillCommandSendsRequestAndPrintsJSON(t *testing.T) {
	var captured struct {
		Namespace string `json:"namespace"`
		From      string `json:"from"`
		To        string `json:"to"`
		Step      string `json:"step"`
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/telemetry/backfill", r.URL.Path)
		assert.Equal(t, "admin-secret", r.Header.Get("X-API-Key"))
		assert.NotEmpty(t, r.Header.Get("traceparent"))
		assert.NotEmpty(t, r.Header.Get("X-Request-ID"))
		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(raw, &captured))
		_, _ = w.Write([]byte(`{"namespace":"payments","from":"2026-11-13T08:00:00Z","to":"2026-11-13T09:00:00Z","step":"5m0s","windowsEvaluated":12,"bucketsIngested":24,"bucketsSkippedDuplicate":0,"newWatermark":"2026-11-13T09:00:00Z"}`))
	}))
	defer ts.Close()

	prevServer, prevToken, prevNS, prevFrom, prevTo, prevStep, prevOut := apiServerURL, apiToken, namespace, backfillFrom, backfillTo, backfillStep, backfillOutput
	defer func() {
		apiServerURL, apiToken, namespace, backfillFrom, backfillTo, backfillStep, backfillOutput = prevServer, prevToken, prevNS, prevFrom, prevTo, prevStep, prevOut
	}()

	apiServerURL = ts.URL
	apiToken = "admin-secret"
	backfillFrom = "2026-11-13T08:00:00Z"
	backfillTo = "2026-11-13T09:00:00Z"
	backfillStep = "5m"
	backfillOutput = "json"
	require.NoError(t, RootCmd.PersistentFlags().Set("namespace", "payments"))

	oldStdout := os.Stdout
	rPipe, wPipe, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = wPipe

	runErr := backfillCmd.RunE(backfillCmd, nil)

	_ = wPipe.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, rPipe)
	_ = rPipe.Close()

	require.NoError(t, runErr)
	assert.Equal(t, "payments", captured.Namespace)
	assert.Contains(t, buf.String(), `"bucketsIngested":24`)
}

func TestBackfillCommandRejectsInvalidWindow(t *testing.T) {
	prevFrom, prevTo := backfillFrom, backfillTo
	defer func() { backfillFrom, backfillTo = prevFrom, prevTo }()

	backfillFrom = "2026-11-13T10:00:00Z"
	backfillTo = "2026-11-13T09:00:00Z"
	err := backfillCmd.RunE(backfillCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be earlier than --to")
}
