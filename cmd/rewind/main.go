package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rewindbpf/rewind/internal/fixture"
	"github.com/rewindbpf/rewind/internal/manifest"
	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/policylearn"
)

const usage = `RewindBPF — AI Agent Safety Runtime

Usage:
  rewind run [options] -- <agent-command>
  rewind status --record PATH
  rewind inspect --record PATH
  rewind events --record PATH
  rewind verify --record PATH
  rewind evidence verify --record PATH
  rewind diff --record PATH
  rewind export --record PATH --output PATH [--format json|patch]
  rewind capabilities
  rewind supervisor --socket PATH --history PATH [--config PATH --http-listen 127.0.0.1:8787 --cors-origin ORIGIN]
  rewind rollback --record PATH
  rewind recover --record PATH
  rewind commit --record PATH --confirm
  rewind sensor attach --object PATH --run-id ID --pid PID   (VM-only telemetry smoke test)
  rewind helper [--plan-file PATH] -- <agent-command>       (internal child helper)
  rewind policy check <policy.yaml>
  rewind policy explain <policy.yaml> <path>
  rewind policy learn --events PATH --output PATH [--max-paths N]
  rewind policy keygen --private PATH --public PATH
  rewind policy sign <policy.yaml> --name NAME --version VERSION --private-key PATH --output PATH
  rewind policy verify <bundle.json> --public-key PATH
  rewind fixture create <directory>
  rewind manifest create <directory> [manifest.json]
  rewind manifest verify <directory> <manifest.json>

The run command is Linux-only and requires a disposable VM for OverlayFS/eBPF integration.
Successful runs discard the temporary write layer by default; use --on-success review to hold it for inspection.
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
	case "evidence":
		handleEvidence(os.Args[2:])
	case "diff":
		handleDiff(os.Args[2:])
	case "export":
		handleExport(os.Args[2:])
	case "capabilities":
		handleCapabilities(os.Args[2:])
	case "supervisor":
		handleSupervisor(os.Args[2:])
	case "commit":
		handleCommit(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "rewind: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

func handleEvidence(args []string) {
	if len(args) < 1 || args[0] != "verify" {
		fatal("usage: rewind evidence verify --record PATH")
	}
	handleVerify(args[1:])
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
	if len(args) == 0 {
		fatal("usage: rewind policy check <policy.yaml> | rewind policy explain <policy.yaml> <path> | rewind policy learn --events PATH --output PATH")
	}
	switch args[0] {
	case "check":
		if len(args) != 2 {
			fatal("usage: rewind policy check <policy.yaml>")
		}
		value, err := policy.Load(args[1])
		if err != nil {
			fatal(err.Error())
		}
		fmt.Printf("policy valid: read=%s deny=%d allow=%d network=%s\n", value.Read.Mode, len(value.Read.Deny), len(value.Read.Allow), value.Network.Mode)
	case "explain":
		if len(args) != 3 {
			fatal("usage: rewind policy explain <policy.yaml> <path>")
		}
		value, err := policy.Load(args[1])
		if err != nil {
			fatal(err.Error())
		}
		if err := json.NewEncoder(os.Stdout).Encode(value.Read.Explain(args[2])); err != nil {
			fatal(fmt.Sprintf("encode policy explanation: %v", err))
		}
	case "learn":
		handlePolicyLearn(args[1:])
	case "keygen":
		handlePolicyKeygen(args[1:])
	case "sign":
		handlePolicySign(args[1:])
	case "verify":
		handlePolicyBundleVerify(args[1:])
	default:
		fatal("unknown policy command")
	}
}

func handlePolicyLearn(args []string) {
	flags := flag.NewFlagSet("policy learn", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	eventsPath := flags.String("events", "", "JSONL telemetry event path")
	outputPath := flags.String("output", "-", "suggested policy output path, or - for stdout")
	maxPaths := flags.Int("max-paths", policylearn.DefaultMaxPaths, "maximum exact paths to suggest")
	if err := flags.Parse(args); err != nil {
		fatal("usage: rewind policy learn --events PATH --output PATH [--max-paths N]")
	}
	if flags.NArg() != 0 || *eventsPath == "" || *maxPaths <= 0 {
		fatal("usage: rewind policy learn --events PATH --output PATH [--max-paths N]")
	}
	file, err := os.Open(filepath.Clean(*eventsPath))
	if err != nil {
		fatal(fmt.Sprintf("open events: %v", err))
	}
	report, learnErr := policylearn.Learn(file, *maxPaths)
	closeErr := file.Close()
	if learnErr != nil {
		fatal(learnErr.Error())
	}
	if closeErr != nil {
		fatal(fmt.Sprintf("close events: %v", closeErr))
	}
	data, err := policylearn.Render(report)
	if err != nil {
		fatal(err.Error())
	}
	if err := policylearn.WriteSuggestion(*outputPath, data); err != nil {
		fatal(err.Error())
	}
	fmt.Fprintf(os.Stderr, "policy suggestion: %d candidate paths from %d read events\n", len(report.Candidates), report.ReadEvents)
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, "rewind:", message)
	os.Exit(2)
}
