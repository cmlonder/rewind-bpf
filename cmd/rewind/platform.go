package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/rewindbpf/rewind/internal/platform"
)

func handlePlatform(args []string) {
	if len(args) == 0 || args[0] != "plan" {
		fatal("usage: rewind platform plan --workspace PATH")
	}
	flags := flag.NewFlagSet("platform plan", flag.ContinueOnError)
	workspace := flags.String("workspace", "", "workspace directory to probe")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || strings.TrimSpace(*workspace) == "" {
		fatal("usage: rewind platform plan --workspace PATH")
	}
	if runtime.GOOS != "darwin" {
		fatal("rewind platform plan currently targets macOS")
	}
	plan, err := platform.PlanForWorkspace(context.Background(), *workspace)
	if err != nil {
		fatal(fmt.Sprintf("macOS platform plan: %v", err))
	}
	if err := json.NewEncoder(os.Stdout).Encode(plan); err != nil {
		fatal(fmt.Sprintf("encode macOS platform plan: %v", err))
	}
}
