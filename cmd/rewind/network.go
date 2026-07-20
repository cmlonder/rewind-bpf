package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rewindbpf/rewind/internal/netns"
)

func handleNetwork(args []string) {
	if len(args) == 0 || args[0] != "plan" {
		fatal("usage: rewind network plan --domains HOST[,HOST...]")
	}
	flags := flag.NewFlagSet("network plan", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	domains := flags.String("domains", "", "comma-separated DNS names")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || strings.TrimSpace(*domains) == "" {
		fatal("usage: rewind network plan --domains HOST[,HOST...]")
	}
	plan, err := netns.BuildAllowlistPlan(strings.Split(*domains, ","))
	if err != nil {
		fatal(fmt.Sprintf("network plan: %v", err))
	}
	if err := json.NewEncoder(os.Stdout).Encode(plan); err != nil {
		fatal(err.Error())
	}
}
