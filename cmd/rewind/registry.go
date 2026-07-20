package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rewindbpf/rewind/internal/registry"
)

func handleRegistry(args []string) {
	if len(args) == 0 || (args[0] != "fetch" && args[0] != "serve") {
		fatal("usage: rewind registry fetch --endpoint URL --name NAME --version VERSION --output PATH [--trusted-public-keys PATH,...] | rewind registry serve --root PATH [--listen 127.0.0.1:8790 --bearer TOKEN]")
	}
	if args[0] == "serve" {
		handleRegistryServe(args[1:])
		return
	}
	flags := flag.NewFlagSet("registry fetch", flag.ContinueOnError)
	endpoint := flags.String("endpoint", "", "HTTPS registry endpoint")
	name := flags.String("name", "", "policy name")
	version := flags.String("version", "", "policy version")
	output := flags.String("output", "", "verified policy JSON output")
	trusted := flags.String("trusted-public-keys", "", "comma-separated raw Ed25519 public-key files")
	token := flags.String("token", "", "optional bearer token")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *endpoint == "" || *name == "" || *version == "" || *output == "" {
		fatal("usage: rewind registry fetch --endpoint URL --name NAME --version VERSION --output PATH [--trusted-public-keys PATH,...]")
	}
	var keys = parseRegistryKeys(*trusted)
	bundle, err := (registry.Client{Endpoint: *endpoint, Bearer: *token, TrustedKeys: keys}).Fetch(context.Background(), *name, *version)
	if err != nil {
		fatal(err.Error())
	}
	data, _ := json.MarshalIndent(bundle, "", "  ")
	if err := os.WriteFile(filepath.Clean(*output), append(data, '\n'), 0o600); err != nil {
		fatal(fmt.Sprintf("write verified policy: %v", err))
	}
	fmt.Printf("fetched and verified policy: %s/%s -> %s\n", *name, *version, *output)
}

func handleRegistryServe(args []string) {
	flags := flag.NewFlagSet("registry serve", flag.ContinueOnError)
	root := flags.String("root", "", "directory for signed policy envelopes")
	listen := flags.String("listen", "127.0.0.1:8790", "loopback listen address")
	bearer := flags.String("bearer", "", "optional bearer token")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *root == "" || !strings.HasPrefix(*listen, "127.0.0.1:") && !strings.HasPrefix(*listen, "localhost:") {
		fatal("usage: rewind registry serve --root PATH [--listen 127.0.0.1:8790 --bearer TOKEN]")
	}
	server := &http.Server{Addr: *listen, Handler: (registry.Server{Store: registry.FileStore{Root: filepath.Clean(*root)}, Bearer: *bearer}).Handler()}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatal(fmt.Sprintf("registry server: %v", err))
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	_ = server.Close()
}

// loadRegistryKeys is kept intentionally small at the CLI boundary; the
// registry package also accepts an empty trust set for embedded-key verification.
func parseRegistryKeys(value string) (keys []ed25519.PublicKey) {
	for _, raw := range strings.Split(value, ",") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Clean(strings.TrimSpace(raw)))
		if err != nil || len(data) != ed25519.PublicKeySize {
			fatal(fmt.Sprintf("invalid registry public key %s", raw))
		}
		keys = append(keys, ed25519.PublicKey(data))
	}
	return keys
}
