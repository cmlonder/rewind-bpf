package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

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
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		fatal("usage: rewind supervisor --socket PATH --history PATH")
	}
	if err := supervisor.ValidateUnixSocketPath(*socketPath); err != nil {
		fatal(err.Error())
	}
	if *historyPath == "" {
		fatal("supervisor history path is required")
	}
	if *tokenPath == "" {
		*tokenPath = *socketPath + ".token"
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
	server := &http.Server{Handler: supervisor.Server{History: history.Open(*historyPath), AuthToken: token}}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() { <-stop; _ = server.Shutdown(context.Background()) }()
	fmt.Printf("rewind supervisor listening: %s token=%s\n", *socketPath, *tokenPath)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fatal(fmt.Sprintf("supervisor serve: %v", err))
	}
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
