package webhook

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
)

func TestWebhookReplayCacheBoundsAndDeduplication(t *testing.T) {
	srv := NewAutoMergeWebhookServer(logr.Discard(), nil, ":0", "secret")

	for i := 0; i < 12000; i++ {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		id := fmt.Sprintf("delivery-%d", i)
		if !srv.verifyReplayWindow(req, id) {
			t.Fatalf("first delivery %s should be accepted", id)
		}
	}

	srv.replayMu.Lock()
	mapLen := len(srv.seenDeliveries)
	listLen := srv.replayOrder.Len()
	srv.replayMu.Unlock()

	if mapLen != maxReplayEntries || listLen != maxReplayEntries {
		t.Fatalf("expected replay cache bounded at %d, got map=%d list=%d", maxReplayEntries, mapLen, listLen)
	}

	// Recent delivery must still be rejected as a replay.
	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	if srv.verifyReplayWindow(req, "delivery-11999") {
		t.Fatal("expected recent delivery-11999 to be rejected as replay")
	}
}
