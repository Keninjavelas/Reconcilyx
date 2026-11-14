package credentials

import (
	"encoding/json"
	"log/slog"
	"strings"
)

const redactedMask = "[REDACTED]"

// SecretString wraps sensitive credential material so accidental formatting,
// JSON/text marshaling, or structured slog logging always emits "[REDACTED]".
// Callers must explicitly invoke Expose() at the boundary where the raw secret
// is passed into an HTTP header, cryptographic signer, or cloud client.
type SecretString struct {
	value string
}

// NewSecretString constructs a SecretString from raw credential material after
// trimming surrounding whitespace and trailing newlines common in file mounts.
func NewSecretString(raw string) SecretString {
	return SecretString{value: strings.TrimSpace(raw)}
}

// Expose returns the underlying secret value for authorized outbound requests.
func (s SecretString) Expose() string {
	return s.value
}

// IsEmpty reports whether the wrapped secret has no non-whitespace content.
func (s SecretString) IsEmpty() bool {
	return s.value == ""
}

// String implements fmt.Stringer without leaking the secret.
func (s SecretString) String() string {
	if s.value == "" {
		return ""
	}
	return redactedMask
}

// GoString implements fmt.GoStringer so %#v formatting never leaks the secret.
func (s SecretString) GoString() string {
	if s.value == "" {
		return `SecretString("")`
	}
	return redactedMask
}

// MarshalText prevents accidental serialization into text configs or YAML.
func (s SecretString) MarshalText() ([]byte, error) {
	if s.value == "" {
		return nil, nil
	}
	return []byte(redactedMask), nil
}

// MarshalJSON prevents accidental serialization into JSON responses or dumps.
func (s SecretString) MarshalJSON() ([]byte, error) {
	if s.value == "" {
		return json.Marshal("")
	}
	return json.Marshal(redactedMask)
}

// LogValue implements slog.LogValuer so structured logs redact the credential.
func (s SecretString) LogValue() slog.Value {
	if s.value == "" {
		return slog.StringValue("")
	}
	return slog.StringValue(redactedMask)
}
