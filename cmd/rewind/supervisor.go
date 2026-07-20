package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
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
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		fatal("usage: rewind supervisor --socket PATH --history PATH")
	}
	if err := supervisor.ValidateUnixSocketPath(*socketPath); err != nil {
		fatal(err.Error())
	}
	if *historyPath == "" {
		fatal("supervisor history path is required")
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
	server := &http.Server{Handler: supervisor.Server{History: history.Open(*historyPath)}}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() { <-stop; _ = server.Shutdown(context.Background()) }()
	fmt.Printf("rewind supervisor listening: %s\n", *socketPath)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fatal(fmt.Sprintf("supervisor serve: %v", err))
	}
}
