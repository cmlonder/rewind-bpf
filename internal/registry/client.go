// Package registry implements a small HTTPS policy registry protocol. The
// registry stores signed envelopes; clients verify the envelope before a
// policy is ever admitted to a run.
package registry

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rewindbpf/rewind/internal/policybundle"
)

type Client struct {
	Endpoint    string
	Bearer      string
	HTTP        *http.Client
	Attempts    int
	TrustedKeys []ed25519.PublicKey
}

func (c Client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	base, err := url.Parse(strings.TrimRight(c.Endpoint, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("registry endpoint is invalid")
	}
	if base.Scheme != "https" && !strings.HasPrefix(base.Host, "127.0.0.1") && !strings.HasPrefix(base.Host, "localhost") {
		return nil, fmt.Errorf("registry endpoint must use HTTPS")
	}
	attempts := c.Attempts
	if attempts <= 0 {
		attempts = 3
	}
	var last error
	for i := 0; i < attempts; i++ {
		req, reqErr := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Endpoint, "/")+path, bytes.NewReader(body))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Accept", "application/json")
		if len(body) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.Bearer != "" {
			req.Header.Set("Authorization", "Bearer "+c.Bearer)
		}
		client := c.HTTP
		if client == nil {
			client = &http.Client{Timeout: 15 * time.Second}
		}
		resp, callErr := client.Do(req)
		if callErr != nil {
			last = callErr
		} else {
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			resp.Body.Close()
			if readErr != nil {
				last = readErr
			} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return data, nil
			} else {
				last = fmt.Errorf("registry returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
			}
		}
		if i+1 < attempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(i+1) * 100 * time.Millisecond):
			}
		}
	}
	return nil, last
}

func (c Client) Publish(ctx context.Context, signed policybundle.Signed) error {
	if signed.Version == 0 || signed.Payload == "" || signed.Signature == "" {
		return fmt.Errorf("registry publish requires signed policy bundle")
	}
	_, err := c.do(ctx, http.MethodPost, "/v1/registry/policies", mustJSON(signed))
	return err
}

func (c Client) Fetch(ctx context.Context, name, version string) (policybundle.Bundle, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
		return policybundle.Bundle{}, fmt.Errorf("registry name and version are required")
	}
	data, err := c.do(ctx, http.MethodGet, "/v1/registry/policies/"+url.PathEscape(name)+"/"+url.PathEscape(version), nil)
	if err != nil {
		return policybundle.Bundle{}, err
	}
	var signed policybundle.Signed
	if err := json.Unmarshal(data, &signed); err != nil {
		return policybundle.Bundle{}, fmt.Errorf("decode registry envelope: %w", err)
	}
	bundle, err := policybundle.Verify(signed, c.TrustedKeys...)
	if err != nil {
		return policybundle.Bundle{}, fmt.Errorf("verify registry policy: %w", err)
	}
	return bundle, nil
}

func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }
