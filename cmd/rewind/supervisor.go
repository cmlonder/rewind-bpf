package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rewindbpf/rewind/internal/controlplane"
	"github.com/rewindbpf/rewind/internal/credentials"
	"github.com/rewindbpf/rewind/internal/history"
	"github.com/rewindbpf/rewind/internal/supervisor"
)

func handleSupervisor(args []string) {
	if runtime.GOOS != "linux" {
		fatal("rewind supervisor currently requires Linux Unix-socket support")
	}
	flags := flag.NewFlagSet("rewind supervisor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	socketPath := flags.String("socket", "", "Unix socket path")
	historyPath := flags.String("history", "", "durable history JSON path")
	tokenPath := flags.String("token-file", "", "bearer token file (created with mode 0600 when absent)")
	configPath := flags.String("config", "", "local policy/workspace config JSON path")
	httpListen := flags.String("http-listen", "", "optional loopback HTTP bridge address, e.g. 127.0.0.1:8787")
	corsOrigin := flags.String("cors-origin", "", "optional exact browser origin allowed for the HTTP bridge")
	trustedPolicyKeys := flags.String("trusted-policy-keys", "", "optional comma-separated raw Ed25519 public-key files for signed policy imports")
	credentialProvider := flags.String("credential-provider-command", "", "optional command-provider executable for short-lived credential leases")
	credentialTimeout := flags.Duration("credential-provider-timeout", 10*time.Second, "timeout for the credential provider command")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		fatal("usage: rewind supervisor --socket PATH --history PATH [--config PATH --http-listen 127.0.0.1:8787 --cors-origin ORIGIN --trusted-policy-keys PATH,... --credential-provider-command PATH]")
	}
	if err := supervisor.ValidateUnixSocketPath(*socketPath); err != nil {
		fatal(err.Error())
	}
	if *historyPath == "" {
		fatal("supervisor history path is required")
	}
	if strings.TrimSpace(*httpListen) != "" {
		if err := supervisor.ValidateHTTPListenAddress(*httpListen); err != nil {
			fatal(err.Error())
		}
		if strings.TrimSpace(*corsOrigin) == "" {
			fatal("--cors-origin is required when --http-listen is set")
		}
	}
	if *tokenPath == "" {
		*tokenPath = *socketPath + ".token"
	}
	if strings.TrimSpace(*configPath) == "" {
		*configPath = *historyPath + ".config.json"
	}
	trustedKeys, err := loadTrustedPolicyKeys(*trustedPolicyKeys)
	if err != nil {
		fatal(err.Error())
	}
	var credentialBroker credentials.Broker
	if strings.TrimSpace(*credentialProvider) != "" {
		if *credentialTimeout <= 0 {
			fatal("--credential-provider-timeout must be positive")
		}
		credentialBroker = &credentials.ManagedBroker{
			Provider: credentials.CommandProvider{Path: strings.TrimSpace(*credentialProvider), Timeout: *credentialTimeout},
			TTL:      5 * time.Minute,
		}
	}
	token, err := loadSupervisorToken(*tokenPath)
	if err != nil {
		fatal(err.Error())
	}
	if info, err := os.Stat(*socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			fatal("refusing to replace a non-socket supervisor path")
		}
		if err := os.Remove(*socketPath); err != nil {
			fatal(fmt.Sprintf("remove stale supervisor socket: %v", err))
		}
	}
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		fatal(fmt.Sprintf("listen supervisor socket: %v", err))
	}
	defer listener.Close()
	if err := os.Chmod(*socketPath, 0o600); err != nil {
		fatal(fmt.Sprintf("protect supervisor socket: %v", err))
	}
	baseServer := supervisor.Server{
		History:           history.Open(*historyPath),
		AuthToken:         token,
		Config:            controlplane.Open(*configPath),
		TrustedPolicyKeys: trustedKeys,
		CredentialBroker:  credentialBroker,
		AuditPath:         *historyPath + ".actions.jsonl",
		AuditMu:           &sync.Mutex{},
		Actions: func(request supervisor.Request) (supervisor.Response, error) {
			return supervisorAction(*historyPath, request)
		},
	}
	server := &http.Server{Handler: baseServer.Handler()}
	var httpServer *http.Server
	var httpListener net.Listener
	if strings.TrimSpace(*httpListen) != "" {
		httpListener, err = net.Listen("tcp", *httpListen)
		if err != nil {
			fatal(fmt.Sprintf("listen supervisor HTTP bridge: %v", err))
		}
		httpBridge := baseServer
		httpBridge.RequireAuth = true
		httpBridge.CORSOrigin = strings.TrimSpace(*corsOrigin)
		httpServer = &http.Server{Handler: httpBridge.Handler()}
		go func() {
			if serveErr := httpServer.Serve(httpListener); serveErr != nil && serveErr != http.ErrServerClosed {
				fatal(fmt.Sprintf("supervisor HTTP bridge: %v", serveErr))
			}
		}()
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		_ = server.Shutdown(context.Background())
		if httpServer != nil {
			_ = httpServer.Shutdown(context.Background())
		}
	}()
	fmt.Printf("rewind supervisor listening: %s token=%s\n", *socketPath, *tokenPath)
	if httpListener != nil {
		fmt.Printf("rewind supervisor HTTP bridge: %s origin=%s\n", *httpListen, *corsOrigin)
	}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fatal(fmt.Sprintf("supervisor serve: %v", err))
	}
}

func loadTrustedPolicyKeys(raw string) ([]ed25519.PublicKey, error) {
	var keys []ed25519.PublicKey
	for _, value := range strings.Split(raw, ",") {
		path := strings.TrimSpace(value)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return nil, fmt.Errorf("read trusted policy key %s: %w", path, err)
		}
		if len(data) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("trusted policy key %s must contain %d raw bytes", path, ed25519.PublicKeySize)
		}
		keys = append(keys, ed25519.PublicKey(append([]byte(nil), data...)))
	}
	return keys, nil
}

func loadSupervisorToken(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		if strings.TrimSpace(string(data)) == "" {
			return "", fmt.Errorf("supervisor token file is empty")
		}
		return strings.TrimSpace(string(data)), nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read supervisor token: %w", err)
	}
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate supervisor token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(value)
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write supervisor token: %w", err)
	}
	return token, nil
}
