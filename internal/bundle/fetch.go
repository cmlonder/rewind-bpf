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
	"path/filepath"
	"strings"
)

// Fetch downloads a bounded evidence envelope for an explicit operator
// restore. HTTPS is mandatory outside an opted-in loopback test, and an
// expected SHA-256 digest can pin the downloaded bytes before they are used.
func Fetch(ctx context.Context, endpoint, bearer, expectedSHA, output string, allowInsecureLocalhost bool) error {
	target, err := url.Parse(endpoint)
	if err != nil || target.Scheme == "" || target.Host == "" {
		return fmt.Errorf("fetch bundle: endpoint must be an absolute URL")
	}
	if target.Scheme != "https" && (!allowInsecureLocalhost || target.Scheme != "http" || !isLoopback(target.Hostname())) {
		return fmt.Errorf("fetch bundle: HTTPS is required (HTTP is allowed only for explicit loopback tests)")
	}
	if strings.TrimSpace(output) == "" {
		return fmt.Errorf("fetch bundle: output path is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fmt.Errorf("fetch bundle: create request: %w", err)
	}
	if strings.TrimSpace(bearer) != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("fetch bundle: request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("fetch bundle: endpoint returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxPublishBytes+1))
	if err != nil {
		return fmt.Errorf("fetch bundle: read response: %w", err)
	}
	if len(data) == 0 || len(data) > maxPublishBytes {
		return fmt.Errorf("fetch bundle: response must be between 1 and %d bytes", maxPublishBytes)
	}
	digest := sha256.Sum256(data)
	actual := hex.EncodeToString(digest[:])
	if expected := strings.ToLower(strings.TrimSpace(expectedSHA)); expected != "" && expected != actual {
		return fmt.Errorf("fetch bundle: SHA-256 mismatch: expected %s got %s", expected, actual)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
		return fmt.Errorf("fetch bundle: create output directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(output), ".rewind-fetch-*.tmp")
	if err != nil {
		return fmt.Errorf("fetch bundle: create temporary output: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, output); err != nil {
		return fmt.Errorf("fetch bundle: replace output: %w", err)
	}
	return nil
}
