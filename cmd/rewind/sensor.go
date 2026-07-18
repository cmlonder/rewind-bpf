package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rewindbpf/rewind/internal/ebpfload"
)

func handleSensor(args []string) {
	if len(args) == 0 || args[0] != "attach" {
		fatal("usage: rewind sensor attach --object PATH --run-id ID --pid PID")
	}

	flags := flag.NewFlagSet("rewind sensor attach", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	objectPath := flags.String("object", "", "compiled eBPF object path")
	runID := flags.String("run-id", "", "active protected run ID")
	targetPID := flags.Uint("pid", 0, "agent PID to scope telemetry to")
	if err := flags.Parse(args[1:]); err != nil {
		fatal(err.Error())
	}
	if flags.NArg() != 0 {
		fatal("usage: rewind sensor attach --object PATH --run-id ID --pid PID")
	}
	if uint64(*targetPID) > uint64(^uint32(0)) {
		fatal("pid is outside the supported uint32 range")
	}

	session, err := ebpfload.Load(*objectPath, *runID, uint32(*targetPID))
	if err != nil {
		fatal(err.Error())
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	stopped := make(chan struct{})
	go func() {
		<-stop
		close(stopped)
		_ = session.Close()
	}()
	defer session.Close()

	fmt.Fprintf(os.Stderr, "telemetry attached: run_id=%s pid=%d; press Ctrl-C to detach\n", *runID, *targetPID)
	encoder := json.NewEncoder(os.Stdout)
	for {
		value, err := session.Events().Read()
		if err != nil {
			select {
			case <-stopped:
				return
			default:
			}
			fatal(err.Error())
		}
		if err := encoder.Encode(value); err != nil {
			fatal(fmt.Sprintf("write telemetry event: %v", err))
		}
	}
}
