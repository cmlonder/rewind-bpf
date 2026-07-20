package netpolicy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/rewindbpf/rewind/internal/policy"
)

func TestProxyEnforcesAllowlistForHTTPClients(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer target.Close()
	targetURL, _ := url.Parse(target.URL)
	plan, err := Compile(policy.NetworkPolicy{Mode: policy.ModeEnforce, AllowDomains: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := ListenProxy(plan)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = proxy.Serve(ctx) }()
	proxyURL, _ := url.Parse(proxy.URL())
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}, Timeout: time.Second}
	response, err := client.Get(targetURL.String())
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", response.StatusCode)
	}
	blocked, err := client.Get("http://not-allowed.invalid/")
	if err != nil {
		t.Fatal(err)
	}
	_ = blocked.Body.Close()
	if blocked.StatusCode != http.StatusForbidden {
		t.Fatalf("blocked status=%d", blocked.StatusCode)
	}
}
