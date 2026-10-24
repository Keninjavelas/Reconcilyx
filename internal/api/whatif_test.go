package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func postWhatIf(t *testing.T, body any) (*http.Response, []byte) {
	t.Helper()
	server := NewServer(":0", "", ServerDeps{})
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	payload, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := ts.Client().Post(ts.URL+"/api/what-if", "application/json", bytes.NewReader(payload))
	require.NoError(t, err)
	return resp, []byte(readResponseBody(t, resp))
}

func TestWhatIfSignedCostDelta(t *testing.T) {
	t.Run("reduction reports negative cost delta", func(t *testing.T) {
		resp, raw := postWhatIf(t, whatIfRequest{
			CurrentCPU:     "2",
			CurrentMemory:  "4Gi",
			ProposedCPU:    "1",
			ProposedMemory: "2Gi",
		})
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var out whatIfResponse
		require.NoError(t, json.Unmarshal(raw, &out))
		assert.Equal(t, "25.75", out.SavingsUSDPerMonth)
		assert.InDelta(t, -25.75, out.CostDeltaUSDPerMonth, 0.001)
		assert.NotContains(t, out.Rationale, "increases spend")
		assert.Contains(t, out.Rationale, "recommends approval")
	})

	t.Run("increase reports positive cost delta and zero savings", func(t *testing.T) {
		resp, raw := postWhatIf(t, whatIfRequest{
			CurrentCPU:     "2",
			CurrentMemory:  "4Gi",
			ProposedCPU:    "4",
			ProposedMemory: "8Gi",
		})
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var out whatIfResponse
		require.NoError(t, json.Unmarshal(raw, &out))
		assert.Equal(t, "0.00", out.SavingsUSDPerMonth)
		assert.InDelta(t, 51.5, out.CostDeltaUSDPerMonth, 0.001)
		assert.Contains(t, out.Rationale, "increases spend")
		assert.NotContains(t, out.Rationale, "recommends approval")
	})

	t.Run("invalid quantity is rejected", func(t *testing.T) {
		resp, raw := postWhatIf(t, whatIfRequest{
			CurrentCPU:     "not-a-quantity",
			CurrentMemory:  "4Gi",
			ProposedCPU:    "1",
			ProposedMemory: "2Gi",
		})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, string(raw), "Invalid currentCPU")
	})
}
