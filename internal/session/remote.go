package session

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// RemoteStore is a bearer-authenticated transport for the same lease
// protocol. The supervisor remains the authority; this client only adds
// reconnect-safe retries and keeps lease JSON free of filesystem privileges.
type RemoteStore struct {
	Endpoint string
	Bearer   string
	HTTP     *http.Client
	Attempts int
}

func (s RemoteStore) Apply(ctx context.Context, request Request) (Lease, error) {
	var lease Lease
	if err := s.do(ctx, http.MethodPost, request, &lease); err != nil {
		return Lease{}, err
	}
	return lease, nil
}

func (s RemoteStore) List(ctx context.Context) ([]Lease, error) {
	var leases []Lease
	if err := s.do(ctx, http.MethodGet, nil, &leases); err != nil {
		return nil, err
	}
	return leases, nil
}

func (s RemoteStore) do(ctx context.Context, method string, value any, output any) error {
	endpoint := strings.TrimRight(strings.TrimSpace(s.Endpoint), "/")
	if endpoint == "" {
		return fmt.Errorf("remote session endpoint is required")
	}
	if !strings.HasPrefix(endpoint, "https://") && !strings.HasPrefix(endpoint, "http://127.0.0.1") && !strings.HasPrefix(endpoint, "http://localhost") {
		return fmt.Errorf("remote session endpoint must use HTTPS outside loopback")
	}
	var body *bytes.Reader
	if value == nil {
		body = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	attempts := s.Attempts
	if attempts < 1 {
		attempts = 3
	}
	if attempts > 6 {
		attempts = 6
	}
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, endpoint+"/v1/sessions", body)
		if err != nil {
			return err
		}
		if value != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if s.Bearer != "" {
			req.Header.Set("Authorization", "Bearer "+s.Bearer)
		}
		resp, err := s.client().Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			defer resp.Body.Close()
			return json.NewDecoder(resp.Body).Decode(output)
		}
		if resp != nil {
			resp.Body.Close()
			last = fmt.Errorf("remote session returned %s", resp.Status)
		} else {
			last = err
		}
		if resp != nil && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			break
		}
		time.Sleep(time.Duration(1<<attempt) * 50 * time.Millisecond)
		body.Seek(0, 0)
	}
	return last
}

func (s RemoteStore) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}
