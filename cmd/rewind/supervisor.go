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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rewindbpf/rewind/internal/controlplane"
	"github.com/rewindbpf/rewind/internal/credentials"
	"github.com/rewindbpf/rewind/internal/history"
	"github.com/rewindbpf/rewind/internal/registry"
	"github.com/rewindbpf/rewind/internal/session"
	"github.com/rewindbpf/rewind/internal/supervisor"
)

func handleSupervisor(args []string) {
	// The control plane is portable: Linux uses it for the privileged runtime,
	// while macOS exposes the native APFS/Seatbelt record lifecycle through the
	// same authenticated bridge. Windows remains capability/read-only until its
	// signed filesystem helper is installed.
	flags := flag.NewFlagSet("rewind supervisor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	socketPath := flags.String("socket", "", "Unix socket path")
	httpOnly := flags.Bool("http-only", false, "run only the loopback HTTP bridge; useful on Windows")
	historyPath := flags.String("history", "", "durable history JSON path")
	tokenPath := flags.String("token-file", "", "bearer token file (created with mode 0600 when absent)")
	configPath := flags.String("config", "", "local policy/workspace config JSON path")
	httpListen := flags.String("http-listen", "", "optional loopback HTTP bridge address, e.g. 127.0.0.1:8787")
	corsOrigin := flags.String("cors-origin", "", "optional exact browser origin allowed for the HTTP bridge")
	trustedPolicyKeys := flags.String("trusted-policy-keys", "", "optional comma-separated raw Ed25519 public-key files for signed policy imports")
	registryEndpoint := flags.String("registry-endpoint", "", "optional HTTPS trusted-policy registry endpoint proxied by supervisor")
	registryToken := flags.String("registry-token", "", "optional bearer token for the trusted-policy registry (never sent to browser)")
	credentialProvider := flags.String("credential-provider-command", "", "optional command-provider executable for short-lived credential leases")
	credentialTimeout := flags.Duration("credential-provider-timeout", 10*time.Second, "timeout for the credential provider command")
	sessionBackend := flags.String("session-backend", "local", "session backend: local or sqlite")
	sessionPath := flags.String("session-path", "", "optional session store path (defaults beside history)")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		fatal("usage: rewind supervisor [--socket PATH|--http-only] --history PATH [--config PATH --http-listen 127.0.0.1:8787 --cors-origin ORIGIN --trusted-policy-keys PATH,... --registry-endpoint URL --registry-token TOKEN --credential-provider-command PATH]")
	}
	if strings.TrimSpace(*socketPath) == "" && !*httpOnly {
		fatal("--socket is required unless --http-only is set")
	}
	if *httpOnly && strings.TrimSpace(*httpListen) == "" {
		fatal("--http-only requires --http-listen")
	}
	if strings.TrimSpace(*socketPath) != "" {
		if err := supervisor.ValidateUnixSocketPath(*socketPath); err != nil {
			fatal(err.Error())
		}
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
		if strings.TrimSpace(*socketPath) != "" {
			*tokenPath = *socketPath + ".token"
		} else {
			*tokenPath = *historyPath + ".token"
		}
	}
	if strings.TrimSpace(*configPath) == "" {
		*configPath = *historyPath + ".config.json"
	}
	trustedKeys, err := loadTrustedPolicyKeys(*trustedPolicyKeys)
	if err != nil {
		fatal(err.Error())
	}
	var registryClient *registry.Client
	if strings.TrimSpace(*registryEndpoint) != "" {
		registryClient = &registry.Client{Endpoint: strings.TrimSpace(*registryEndpoint), Bearer: *registryToken, TrustedKeys: trustedKeys}
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
	var listener net.Listener
	if strings.TrimSpace(*socketPath) != "" {
		if info, statErr := os.Stat(*socketPath); statErr == nil {
			if info.Mode()&os.ModeSocket == 0 {
				fatal("refusing to replace a non-socket supervisor path")
			}
			if removeErr := os.Remove(*socketPath); removeErr != nil {
				fatal(fmt.Sprintf("remove stale supervisor socket: %v", removeErr))
			}
		}
		var listenErr error
		listener, listenErr = net.Listen("unix", *socketPath)
		if listenErr != nil {
			fatal(fmt.Sprintf("listen supervisor socket: %v", listenErr))
		}
		defer listener.Close()
		if err := os.Chmod(*socketPath, 0o600); err != nil {
			fatal(fmt.Sprintf("protect supervisor socket: %v", err))
		}
	}
	if *sessionPath == "" {
		*sessionPath = *historyPath + ".sessions.json"
	}
	var sessionStore session.LeaseStore
	var closeSessionStore func() error
	switch strings.ToLower(strings.TrimSpace(*sessionBackend)) {
	case "local", "json":
		store := session.Open(*sessionPath)
		sessionStore = store
	case "sqlite":
		store, openErr := session.OpenSQLite(*sessionPath)
		if openErr != nil {
			fatal(openErr.Error())
		}
		sessionStore = store
		closeSessionStore = store.Close
	default:
		fatal("--session-backend must be local or sqlite")
	}
	if closeSessionStore != nil {
		defer closeSessionStore()
	}
	baseServer := supervisor.Server{
		History:           history.Open(*historyPath),
		AuthToken:         token,
		Config:            controlplane.Open(*configPath),
		TrustedPolicyKeys: trustedKeys,
		CredentialBroker:  credentialBroker,
		Sessions:          sessionStore,
		SessionPath:       *sessionPath,
		AuditPath:         *historyPath + ".actions.jsonl",
		AuditMu:           &sync.Mutex{},
		Registry:          registryClient,
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
	shutdown := func() {
		_ = server.Shutdown(context.Background())
		if httpServer != nil {
			_ = httpServer.Shutdown(context.Background())
		}
	}
	if listener != nil {
		fmt.Printf("rewind supervisor listening: %s token=%s\n", *socketPath, *tokenPath)
	} else {
		fmt.Printf("rewind supervisor HTTP-only token=%s\n", *tokenPath)
	}
	if httpListener != nil {
		fmt.Printf("rewind supervisor HTTP bridge: %s origin=%s\n", *httpListen, *corsOrigin)
	}
	if listener == nil {
		<-stop
		shutdown()
		return
	}
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()
	select {
	case <-stop:
		shutdown()
	case serveErr := <-serveResult:
		if serveErr != nil && serveErr != http.ErrServerClosed {
			fatal(fmt.Sprintf("supervisor serve: %v", serveErr))
		}
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
