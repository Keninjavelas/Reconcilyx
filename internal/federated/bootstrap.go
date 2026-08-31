package federated

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

func FetchGlobalModel(ctx context.Context, hubBaseURL, apiKey string, httpClient *http.Client) (*GlobalQTable, error) {
	if hubBaseURL == "" {
		return nil, errors.New("federated: HubBaseURL is empty; cannot bootstrap")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hubBaseURL+"/api/federated/global-model", http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("federated: hub rejected bootstrap, status=%s body=%s", resp.Status, string(raw))
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(resp.Body, 32*1024*1024)); err != nil {
		return nil, err
	}
	var out GlobalQTable
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("federated: failed to decode global model: %w", err)
	}
	if out.Entries == nil {
		out.Entries = make(map[string]GlobalQEntry)
	}
	return &out, nil
}
