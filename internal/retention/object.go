// Package retention provides a small HTTP object-store adapter for signed
// evidence. It works with S3-compatible gateways that expose authenticated
// PUT/GET URLs; provider-specific SigV4/KMS credentials stay outside the core.
package retention

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

const MaxObjectBytes = 192 << 20

type Client struct {
	Endpoint string
	Bearer   string
	Attempts int
	HTTP     *http.Client
}

func (c Client) Put(ctx context.Context, key string, data []byte) error {
	if len(data) == 0 || len(data) > MaxObjectBytes {
		return fmt.Errorf("retention put: object must be between 1 and %d bytes", MaxObjectBytes)
	}
	request, err := c.request(ctx, http.MethodPut, key, data)
	if err != nil {
		return err
	}
	return c.doWithRetry(request)
}

func (c Client) PutFile(ctx context.Context, key, filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("retention put: read %s: %w", filename, err)
	}
	return c.Put(ctx, key, data)
}

func (c Client) Get(ctx context.Context, key string) ([]byte, error) {
	request, err := c.request(ctx, http.MethodGet, key, nil)
	if err != nil {
		return nil, err
	}
	attempts := c.attempts()
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		current := request
		if attempt > 0 && request.GetBody != nil {
			body, bodyErr := request.GetBody()
			if bodyErr != nil {
				return nil, bodyErr
			}
			current = request.Clone(request.Context())
			current.Body = body
		}
		response, err := c.client().Do(current)
		if err == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			data, readErr := io.ReadAll(io.LimitReader(response.Body, MaxObjectBytes+1))
			_ = response.Body.Close()
			if readErr != nil {
				return nil, fmt.Errorf("retention get: read response: %w", readErr)
			}
			if len(data) == 0 || len(data) > MaxObjectBytes {
				return nil, fmt.Errorf("retention get: response exceeds object limit")
			}
			if expected := strings.TrimSpace(response.Header.Get("X-Rewind-Object-SHA256")); expected != "" {
				digest := sha256.Sum256(data)
				if !strings.EqualFold(expected, hex.EncodeToString(digest[:])) {
					return nil, fmt.Errorf("retention get: digest mismatch")
				}
			}
			return data, nil
		}
		if response != nil {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
			_ = response.Body.Close()
			last = fmt.Errorf("retention get: endpoint returned %s: %s", response.Status, strings.TrimSpace(string(body)))
		} else {
			last = err
		}
		if !retryable(response, err) || attempt+1 == attempts {
			break
		}
		time.Sleep(time.Duration(1<<attempt) * 100 * time.Millisecond)
	}
	return nil, last
}

func (c Client) request(ctx context.Context, method, key string, data []byte) (*http.Request, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("retention: object key is required")
	}
	target, err := url.Parse(strings.TrimSpace(c.Endpoint))
	if err != nil || target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("retention: endpoint must be absolute")
	}
	if target.Scheme != "https" && !(target.Scheme == "http" && isLoopback(target.Hostname())) {
		return nil, fmt.Errorf("retention: HTTPS is required outside loopback")
	}
	target.Path = path.Join(target.Path, key)
	var body io.Reader
	if data != nil {
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("retention: create request: %w", err)
	}
	if c.Bearer != "" {
		request.Header.Set("Authorization", "Bearer "+c.Bearer)
	}
	if data != nil {
		digest := sha256.Sum256(data)
		request.Header.Set("Content-Type", "application/octet-stream")
		request.Header.Set("X-Rewind-Object-SHA256", hex.EncodeToString(digest[:]))
	}
	return request, nil
}

func (c Client) doWithRetry(request *http.Request) error {
	var last error
	for attempt := 0; attempt < c.attempts(); attempt++ {
		current := request
		if attempt > 0 && request.GetBody != nil {
			body, bodyErr := request.GetBody()
			if bodyErr != nil {
				return bodyErr
			}
			current = request.Clone(request.Context())
			current.Body = body
		}
		response, err := c.client().Do(current)
		if err == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			_ = response.Body.Close()
			return nil
		}
		if response != nil {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
			_ = response.Body.Close()
			last = fmt.Errorf("retention put: endpoint returned %s: %s", response.Status, strings.TrimSpace(string(body)))
		} else {
			last = err
		}
		if !retryable(response, err) || attempt+1 == c.attempts() {
			break
		}
		time.Sleep(time.Duration(1<<attempt) * 100 * time.Millisecond)
	}
	return last
}

func (c Client) attempts() int {
	if c.Attempts < 1 {
		return 3
	}
	if c.Attempts > 6 {
		return 6
	}
	return c.Attempts
}
func (c Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}
func retryable(response *http.Response, err error) bool {
	return err != nil || response == nil || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
}
func isLoopback(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}
