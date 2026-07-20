package bundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// The limit includes encrypted JSON envelopes, which carry base64 overhead
// around the plaintext archive.
const maxPublishBytes = 192 << 20

// Publish sends an already-created evidence archive to an explicit review
// endpoint. The runtime never publishes automatically: this boundary is an
// operator action, and HTTPS is required except for an explicitly opted-in
// loopback test endpoint.
func Publish(ctx context.Context, archivePath, endpoint, bearer, detachedSignature string, allowInsecureLocalhost bool) (string, error) {
	if strings.TrimSpace(archivePath) == "" || strings.TrimSpace(endpoint) == "" {
		return "", fmt.Errorf("publish bundle: archive and endpoint are required")
	}
	target, err := url.Parse(endpoint)
	if err != nil || target.Scheme == "" || target.Host == "" {
		return "", fmt.Errorf("publish bundle: endpoint must be an absolute URL")
	}
	if target.Scheme != "https" {
		if !allowInsecureLocalhost || target.Scheme != "http" || !isLoopback(target.Hostname()) {
			return "", fmt.Errorf("publish bundle: HTTPS is required (HTTP is allowed only for explicit loopback tests)")
		}
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("publish bundle: open archive: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("publish bundle: stat archive: %w", err)
	}
	if info.Size() <= 0 || info.Size() > maxPublishBytes {
		return "", fmt.Errorf("publish bundle: archive size must be between 1 and %d bytes", maxPublishBytes)
	}
	payload, err := io.ReadAll(io.LimitReader(file, maxPublishBytes+1))
	if err != nil {
		return "", fmt.Errorf("publish bundle: read archive: %w", err)
	}
	if len(payload) > maxPublishBytes {
		return "", fmt.Errorf("publish bundle: archive exceeds %d bytes", maxPublishBytes)
	}
	digest := sha256.Sum256(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), strings.NewReader(string(payload)))
	if err != nil {
		return "", fmt.Errorf("publish bundle: create request: %w", err)
	}
	contentType := "application/gzip"
	if strings.HasSuffix(strings.ToLower(archivePath), ".json") || strings.HasSuffix(strings.ToLower(archivePath), ".enc") {
		contentType = "application/json"
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Rewind-Bundle-SHA256", hex.EncodeToString(digest[:]))
	if strings.TrimSpace(detachedSignature) != "" {
		req.Header.Set("X-Rewind-Bundle-Signature", detachedSignature)
	}
	if strings.TrimSpace(bearer) != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", fmt.Errorf("publish bundle: send request: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("publish bundle: endpoint returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	return strings.TrimSpace(string(body)), nil
}

func isLoopback(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}
