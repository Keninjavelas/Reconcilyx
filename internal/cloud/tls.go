package cloud

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"strings"
	"time"
)

// newHTTPClientWithCustomCA builds an HTTP client that trusts the OS root store
// plus any PEM CA bundle specified in RECONCILYX_CA_BUNDLE or SSL_CERT_FILE.
func newHTTPClientWithCustomCA(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	for _, envKey := range []string{"RECONCILYX_CA_BUNDLE", "SSL_CERT_FILE"} {
		caPath := strings.TrimSpace(os.Getenv(envKey))
		if caPath == "" {
			continue
		}
		pemBytes, err := os.ReadFile(caPath)
		if err != nil {
			continue
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if pool.AppendCertsFromPEM(pemBytes) {
			if transport.TLSClientConfig == nil {
				transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
			}
			transport.TLSClientConfig.RootCAs = pool
		}
		break
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
