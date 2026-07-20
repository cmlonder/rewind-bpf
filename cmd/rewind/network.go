package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rewindbpf/rewind/internal/netns"
)

func handleNetwork(args []string) {
	if len(args) == 0 || args[0] != "plan" {
		fatal("usage: rewind network plan --domains HOST[,HOST...] [--resolve]")
	}
	flags := flag.NewFlagSet("network plan", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	domains := flags.String("domains", "", "comma-separated DNS names")
	resolve := flags.Bool("resolve", false, "resolve domains and host DNS servers into the destination IP set")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || strings.TrimSpace(*domains) == "" {
		fatal("usage: rewind network plan --domains HOST[,HOST...] [--resolve]")
	}
	var plan netns.AllowlistPlan
	var err error
	if *resolve {
		resolved, resolveErr := netns.ResolveDomains(context.Background(), strings.Split(*domains, ","))
		if resolveErr != nil {
			fatal(fmt.Sprintf("network plan: resolve domains: %v", resolveErr))
		}
		resolvers, resolveErr := netns.ResolveNameservers()
		if resolveErr != nil {
			fatal(fmt.Sprintf("network plan: resolve DNS servers: %v", resolveErr))
		}
		plan, err = netns.BuildAllowlistPlanWithIPsAndResolvers(strings.Split(*domains, ","), resolved, resolvers)
	} else {
		plan, err = netns.BuildAllowlistPlan(strings.Split(*domains, ","))
	}
	if err != nil {
		fatal(fmt.Sprintf("network plan: %v", err))
	}
	if err := json.NewEncoder(os.Stdout).Encode(plan); err != nil {
		fatal(err.Error())
	}
}
