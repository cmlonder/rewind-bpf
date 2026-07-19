package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rewindbpf/rewind/internal/fixture"
	"github.com/rewindbpf/rewind/internal/manifest"
	"github.com/rewindbpf/rewind/internal/policy"
)

const usage = `RewindBPF — AI Agent Safety Runtime

Usage:
  rewind run [options] -- <agent-command>
  rewind status --record PATH
  rewind inspect --record PATH
  rewind events --record PATH
  rewind verify --record PATH
  rewind diff --record PATH
  rewind capabilities
  rewind rollback --record PATH
  rewind recover --record PATH
  rewind commit <run_id>
  rewind sensor attach --object PATH --run-id ID --pid PID   (VM-only telemetry smoke test)
  rewind helper [--plan-file PATH] -- <agent-command>       (internal child helper)
  rewind policy check <policy.yaml>
  rewind fixture create <directory>
  rewind manifest create <directory> [manifest.json]
  rewind manifest verify <directory> <manifest.json>

The run command is Linux-only and requires a disposable VM for OverlayFS/eBPF integration.
`

func main() {
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Print(usage)
		return
	}

	switch os.Args[1] {
	case "fixture":
		handleFixture(os.Args[2:])
	case "manifest":
		handleManifest(os.Args[2:])
	case "policy":
		handlePolicy(os.Args[2:])
	case "sensor":
		handleSensor(os.Args[2:])
	case "helper":
		handleHelper(os.Args[2:])
	case "run":
		handleRun(os.Args[2:])
	case "rollback":
		handleRollback(os.Args[2:])
	case "recover":
		handleRecover(os.Args[2:])
	case "status":
		handleStatus(os.Args[2:])
	case "inspect":
		handleInspect(os.Args[2:])
	case "events":
		handleEvents(os.Args[2:])
	case "verify":
		handleVerify(os.Args[2:])
	case "diff":
		handleDiff(os.Args[2:])
	case "capabilities":
		handleCapabilities(os.Args[2:])
	case "commit":
		fmt.Printf("rewind: command %q is planned; kernel and daemon integration are not enabled yet\n", os.Args[1])
	default:
		fmt.Fprintf(os.Stderr, "rewind: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

func handleFixture(args []string) {
	if len(args) != 2 || args[0] != "create" {
		fatal("usage: rewind fixture create <directory>")
	}
	if err := fixture.Create(args[1]); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("created synthetic fixture at %s\n", args[1])
}

func handleManifest(args []string) {
	if len(args) < 2 {
		fatal("usage: rewind manifest create <directory> [manifest.json] | rewind manifest verify <directory> <manifest.json>")
	}
	switch args[0] {
	case "create":
		if len(args) > 3 {
			fatal("usage: rewind manifest create <directory> [manifest.json]")
		}
		value, err := manifest.Build(args[1])
		if err != nil {
			fatal(err.Error())
		}
		if len(args) == 3 {
			file, err := os.Create(filepath.Clean(args[2]))
			if err != nil {
				fatal(fmt.Sprintf("create manifest output: %v", err))
			}
			err = manifest.WriteJSON(file, value)
			closeErr := file.Close()
			if err != nil {
				fatal(err.Error())
			}
			if closeErr != nil {
				fatal(fmt.Sprintf("close manifest output: %v", closeErr))
			}
			fmt.Printf("wrote manifest with %d entries to %s\n", len(value.Entries), args[2])
			return
		}
		if err := manifest.WriteJSON(os.Stdout, value); err != nil {
			fatal(err.Error())
		}
	case "verify":
		if len(args) != 3 {
			fatal("usage: rewind manifest verify <directory> <manifest.json>")
		}
		file, err := os.Open(filepath.Clean(args[2]))
		if err != nil {
			fatal(fmt.Sprintf("open manifest: %v", err))
		}
		expected, err := manifest.ReadJSON(file)
		closeErr := file.Close()
		if err != nil {
			fatal(err.Error())
		}
		if closeErr != nil {
			fatal(fmt.Sprintf("close manifest: %v", closeErr))
		}
		if err := manifest.Verify(args[1], expected); err != nil {
			fatal(fmt.Sprintf("manifest verification failed: %v", err))
		}
		fmt.Println("manifest verified")
	default:
		fatal("unknown manifest command")
	}
}

func handlePolicy(args []string) {
	if len(args) != 2 || args[0] != "check" {
		fatal("usage: rewind policy check <policy.yaml>")
	}
	value, err := policy.Load(args[1])
	if err != nil {
		fatal(err.Error())
	}
	fmt.Printf("policy valid: read=%s deny=%d allow=%d network=%s\n", value.Read.Mode, len(value.Read.Deny), len(value.Read.Allow), value.Network.Mode)
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, "rewind:", message)
	os.Exit(2)
}
