package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

const (
	HeaderTraceparent = "traceparent"
	HeaderTracestate  = "tracestate"
	HeaderRequestID   = "X-Request-ID"
)

type traceCtxKey struct{}

// TraceContext holds W3C Trace Context identifiers alongside Reconcilyx's
// correlation X-Request-ID for structured logging and cross-service propagation.
type TraceContext struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Flags        string
	RequestID    string
}

// Traceparent formats the active span as a W3C traceparent header (`00-<traceID>-<spanID>-<flags>`).
func (tc TraceContext) Traceparent() string {
	flags := tc.Flags
	if len(flags) != 2 || !isLowerHex(flags) {
		flags = "01"
	}
	if tc.TraceID == "" || tc.SpanID == "" {
		return ""
	}
	return fmt.Sprintf("00-%s-%s-%s", tc.TraceID, tc.SpanID, flags)
}

// ContextWithTrace attaches tc to ctx.
func ContextWithTrace(ctx context.Context, tc TraceContext) context.Context {
	return context.WithValue(ctx, traceCtxKey{}, tc)
}

// FromContext extracts TraceContext from ctx if present.
func FromContext(ctx context.Context) (TraceContext, bool) {
	if ctx == nil {
		return TraceContext{}, false
	}
	tc, ok := ctx.Value(traceCtxKey{}).(TraceContext)
	return tc, ok && tc.TraceID != "" && tc.SpanID != ""
}

// ParseTraceparent validates and parses a W3C traceparent header value (`00-<32hex>-<16hex>-<2hex>`).
func ParseTraceparent(header string) (traceID, parentSpanID, flags string, ok bool) {
	parts := strings.Split(strings.TrimSpace(strings.ToLower(header)), "-")
	if len(parts) < 4 {
		return "", "", "", false
	}
	version, tid, sid, fl := parts[0], parts[1], parts[2], parts[3]
	if version == "ff" || len(version) != 2 || !isLowerHex(version) {
		return "", "", "", false
	}
	if len(tid) != 32 || !isLowerHex(tid) || isAllZeroHex(tid) {
		return "", "", "", false
	}
	if len(sid) != 16 || !isLowerHex(sid) || isAllZeroHex(sid) {
		return "", "", "", false
	}
	if len(fl) != 2 || !isLowerHex(fl) {
		return "", "", "", false
	}
	return tid, sid, fl, true
}

// StartServerSpan extracts or initializes a W3C trace span and request ID from HTTP headers.
// When an incoming W3C traceparent is valid, its trace_id is preserved and a new child span_id
// is minted for this hop while recording parent_span_id.
func StartServerSpan(ctx context.Context, header http.Header) (context.Context, TraceContext) {
	var tc TraceContext
	if header != nil {
		if tid, parentID, flags, ok := ParseTraceparent(header.Get(HeaderTraceparent)); ok {
			tc.TraceID = tid
			tc.ParentSpanID = parentID
			tc.Flags = flags
		}
		tc.RequestID = strings.TrimSpace(header.Get(HeaderRequestID))
	}
	if tc.TraceID == "" {
		tc.TraceID = randomHex(16)
	}
	if tc.Flags == "" {
		tc.Flags = "01"
	}
	tc.SpanID = randomHex(8)
	if tc.RequestID == "" {
		tc.RequestID = randomHex(12)
	}
	return ContextWithTrace(ctx, tc), tc
}

// InjectRequest attaches W3C traceparent and X-Request-ID headers to an outbound HTTP request,
// continuing the active trace context from req.Context() or initializing one when absent.
func InjectRequest(req *http.Request) {
	if req == nil {
		return
	}
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	tc, ok := FromContext(req.Context())
	if !ok {
		ctx, created := StartServerSpan(req.Context(), req.Header)
		tc = created
		*req = *req.WithContext(ctx)
	}
	if req.Header.Get(HeaderTraceparent) == "" {
		if tp := tc.Traceparent(); tp != "" {
			req.Header.Set(HeaderTraceparent, tp)
		}
	}
	if req.Header.Get(HeaderRequestID) == "" && tc.RequestID != "" {
		req.Header.Set(HeaderRequestID, tc.RequestID)
	}
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func isLowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return len(s) > 0
}

func isAllZeroHex(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '0' {
			return false
		}
	}
	return true
}
