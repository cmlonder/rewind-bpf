package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/rewindbpf/rewind/internal/platform"
)

func handlePlatform(args []string) {
	if len(args) == 0 || (args[0] != "plan" && args[0] != "contract") {
		fatal("usage: rewind platform plan --workspace PATH | rewind platform contract --platform darwin|windows --workspace PATH")
	}
	if args[0] == "contract" {
		flags := flag.NewFlagSet("platform contract", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		platformName := flags.String("platform", "", "target platform: darwin or windows")
		workspace := flags.String("workspace", "", "workspace directory")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || strings.TrimSpace(*platformName) == "" || strings.TrimSpace(*workspace) == "" {
			fatal("usage: rewind platform contract --platform darwin|windows --workspace PATH")
		}
		contract, err := platform.BuildNativeContract(*platformName, *workspace)
		if err != nil {
			fatal(err.Error())
		}
		if err := json.NewEncoder(os.Stdout).Encode(contract); err != nil {
			fatal(err.Error())
		}
		return
	}
	flags := flag.NewFlagSet("platform plan", flag.ContinueOnError)
	workspace := flags.String("workspace", "", "workspace directory to probe")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || strings.TrimSpace(*workspace) == "" {
		fatal("usage: rewind platform plan --workspace PATH")
	}
	switch runtime.GOOS {
	case "darwin":
		plan, err := platform.PlanForWorkspace(context.Background(), *workspace)
		if err != nil {
			fatal(fmt.Sprintf("macOS platform plan: %v", err))
		}
		if err := json.NewEncoder(os.Stdout).Encode(plan); err != nil {
			fatal(fmt.Sprintf("encode macOS platform plan: %v", err))
		}
	case "windows":
		plan, err := platform.PlanForWindowsWorkspace(context.Background(), *workspace)
		if err != nil {
			fatal(fmt.Sprintf("Windows platform plan: %v", err))
		}
		if err := json.NewEncoder(os.Stdout).Encode(plan); err != nil {
			fatal(fmt.Sprintf("encode Windows platform plan: %v", err))
		}
	default:
		fatal("rewind platform plan targets native macOS or Windows hosts")
	}
}
