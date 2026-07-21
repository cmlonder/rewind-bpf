package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rewindbpf/rewind/internal/retention"
)

func handleRetention(args []string) {
	if len(args) == 0 {
		fatal("usage: rewind retention put|get ...")
	}
	switch args[0] {
	case "put":
		retentionPut(args[1:])
	case "get":
		retentionGet(args[1:])
	default:
		fatal("usage: rewind retention put|get ...")
	}
}

func retentionPut(args []string) {
	flags := flag.NewFlagSet("retention put", flag.ContinueOnError)
	endpoint := flags.String("endpoint", "", "S3-compatible HTTPS object endpoint")
	key := flags.String("key", "", "object key")
	input := flags.String("input", "", "local payload path")
	token := flags.String("token", "", "optional bearer token")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*endpoint) == "" || strings.TrimSpace(*key) == "" || strings.TrimSpace(*input) == "" {
		fatal("usage: rewind retention put --endpoint URL --key KEY --input PATH [--token TOKEN]")
	}
	if err := (retention.Client{Endpoint: *endpoint, Bearer: *token}).PutFile(context.Background(), *key, filepath.Clean(*input)); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("retention object uploaded: key=%s\n", *key)
}

func retentionGet(args []string) {
	flags := flag.NewFlagSet("retention get", flag.ContinueOnError)
	endpoint := flags.String("endpoint", "", "S3-compatible HTTPS object endpoint")
	key := flags.String("key", "", "object key")
	output := flags.String("output", "", "local output path")
	token := flags.String("token", "", "optional bearer token")
	digest := flags.String("sha256", "", "expected SHA-256 digest; restore is refused on mismatch")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*endpoint) == "" || strings.TrimSpace(*key) == "" || strings.TrimSpace(*output) == "" {
		fatal("usage: rewind retention get --endpoint URL --key KEY --output PATH [--token TOKEN --sha256 HEX]")
	}
	if err := (retention.Client{Endpoint: *endpoint, Bearer: *token}).GetFile(context.Background(), *key, filepath.Clean(*output), *digest); err != nil {
		fatal(fmt.Sprintf("retention get: restore output: %v", err))
	}
	fmt.Printf("retention object downloaded: key=%s output=%s\n", *key, filepath.Clean(*output))
}
