// rewind-evidence is the deliberately small, read-only evidence verifier.
// It does not load eBPF, mount filesystems, or start an agent.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rewindbpf/rewind/internal/evidence"
	"github.com/rewindbpf/rewind/internal/runstore"
)

func main() {
	flags := flag.NewFlagSet("rewind-evidence", flag.ExitOnError)
	recordPath := flags.String("record", "", "run record JSON path")
	flags.Usage = func() { fmt.Fprintln(os.Stderr, "usage: rewind-evidence --record PATH") }
	if err := flags.Parse(os.Args[1:]); err != nil || flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" {
		flags.Usage()
		os.Exit(2)
	}
	record, err := runstore.Read(*recordPath)
	if err != nil {
		fatal(err)
	}
	result, err := evidence.Verify(record)
	if err != nil {
		fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fatal(err)
	}
	if !result.Complete {
		os.Exit(2)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "rewind-evidence:", err)
	os.Exit(2)
}
