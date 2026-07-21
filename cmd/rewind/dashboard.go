package main

// The dashboard command is the opinionated, local-first entry point. It owns
// the boring plumbing (supervisor, token, UI server, and a protected shell)
// so a user can start Rewind with one command instead of copying a developer
// smoke-test sequence.

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const dashboardPolicy = `# Rewind dashboard defaults: deny common secret material and stage writes.
read:
  mode: enforce
  deny:
    - "**/*.env"
    - "**/*.pem"
    - "**/.ssh/**"

write:
  mode: rollback
  scope: workspace

network:
  mode: audit
`

func handleDashboard(args []string) {
	if len(args) == 0 || args[0] != "start" {
		fatal("usage: rewind dashboard start --workspace PATH [--state-dir PATH --policy PATH --ui-dir PATH --supervisor-port PORT --ui-port PORT --no-open --no-shell]")
	}
	flags := flag.NewFlagSet("rewind dashboard start", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workspace := flags.String("workspace", "", "workspace directory to protect")
	stateDir := flags.String("state-dir", "", "persistent dashboard state directory")
	policyPath := flags.String("policy", "", "YAML policy path (defaults to a safe local policy)")
	uiDir := flags.String("ui-dir", "", "static UI directory (defaults to ./ui)")
	supervisorPort := flags.Int("supervisor-port", 0, "loopback supervisor HTTP port (0 chooses a free port)")
	uiPort := flags.Int("ui-port", 0, "loopback static UI port (0 chooses a free port)")
	shellPath := flags.String("shell", "", "protected shell (defaults to $SHELL or /bin/zsh)")
	noOpen := flags.Bool("no-open", false, "do not open the dashboard in the default browser")
	noShell := flags.Bool("no-shell", false, "start the local control plane without starting a protected shell")
	exitAfterShell := flags.Bool("exit-after-shell", false, "stop supervisor/UI when the protected shell exits (automation only)")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		fatal("usage: rewind dashboard start --workspace PATH [--state-dir PATH --policy PATH --ui-dir PATH --supervisor-port PORT --ui-port PORT --no-open --no-shell]")
	}
	if strings.TrimSpace(*workspace) == "" {
		fatal("dashboard requires --workspace PATH")
	}
	if *supervisorPort < 0 || *supervisorPort > 65535 || *uiPort < 0 || *uiPort > 65535 {
		fatal("dashboard ports must be 0 or values between 1024 and 65535")
	}
	if *supervisorPort != 0 && *supervisorPort < 1024 || *uiPort != 0 && *uiPort < 1024 {
		fatal("dashboard ports below 1024 are not allowed")
	}
	chosenSupervisorPort, err := reserveDashboardPort(*supervisorPort)
	if err != nil {
		fatal(fmt.Sprintf("choose supervisor port: %v", err))
	}
	chosenUIPort, err := reserveDashboardPort(*uiPort)
	if err != nil {
		fatal(fmt.Sprintf("choose UI port: %v", err))
	}
	if chosenSupervisorPort == chosenUIPort {
		chosenUIPort, err = reserveDashboardPort(0)
		if err != nil {
			fatal(fmt.Sprintf("choose distinct UI port: %v", err))
		}
	}
	if runtime.GOOS == "linux" && os.Geteuid() != 0 && !*noShell {
		fatal("Linux dashboard protected shell requires root; run it inside the disposable VM with sudo")
	}

	workspaceAbs, err := filepath.Abs(*workspace)
	if err != nil {
		fatal(fmt.Sprintf("resolve dashboard workspace: %v", err))
	}
	workspaceAbs = filepath.Clean(workspaceAbs)
	if info, statErr := os.Stat(workspaceAbs); statErr != nil || !info.IsDir() {
		if statErr != nil {
			fatal(fmt.Sprintf("dashboard workspace: %v", statErr))
		}
		fatal("dashboard workspace is not a directory")
	}

	state := strings.TrimSpace(*stateDir)
	if state == "" {
		cache, cacheErr := os.UserCacheDir()
		if cacheErr != nil {
			cache = os.TempDir()
		}
		state = filepath.Join(cache, "rewindbpf", "dashboard", filepath.Base(workspaceAbs))
	}
	state, err = filepath.Abs(state)
	if err != nil {
		fatal(fmt.Sprintf("resolve dashboard state: %v", err))
	}
	if err := os.MkdirAll(state, 0o700); err != nil {
		fatal(fmt.Sprintf("create dashboard state: %v", err))
	}

	policyFile := strings.TrimSpace(*policyPath)
	if policyFile == "" {
		policyFile = filepath.Join(state, "policy.yaml")
		if _, statErr := os.Stat(policyFile); os.IsNotExist(statErr) {
			if err := os.WriteFile(policyFile, []byte(dashboardPolicy), 0o600); err != nil {
				fatal(fmt.Sprintf("write dashboard policy: %v", err))
			}
		}
	}
	policyFile, err = filepath.Abs(policyFile)
	if err != nil {
		fatal(fmt.Sprintf("resolve dashboard policy: %v", err))
	}

	uiRoot := strings.TrimSpace(*uiDir)
	if uiRoot == "" {
		uiRoot = filepath.Join(mustWorkingDirectory(), "ui")
	}
	uiRoot, err = filepath.Abs(uiRoot)
	if err != nil {
		fatal(fmt.Sprintf("resolve dashboard UI: %v", err))
	}
	if info, statErr := os.Stat(uiRoot); statErr != nil || !info.IsDir() {
		fatal(fmt.Sprintf("dashboard UI directory is unavailable: %s", uiRoot))
	}

	executable, err := os.Executable()
	if err != nil {
		fatal(fmt.Sprintf("resolve rewind executable: %v", err))
	}
	history := filepath.Join(state, "history.json")
	tokenFile := history + ".token"
	supervisorURL := fmt.Sprintf("http://127.0.0.1:%d", chosenSupervisorPort)
	uiURL := fmt.Sprintf("http://127.0.0.1:%d", chosenUIPort)

	supervisor := exec.Command(executable, "supervisor", "--http-only", "--history", history, "--token-file", tokenFile, "--http-listen", fmt.Sprintf("127.0.0.1:%d", chosenSupervisorPort), "--cors-origin", uiURL)
	if err := startDashboardChild(supervisor); err != nil {
		fatal(fmt.Sprintf("start local supervisor: %v", err))
	}
	uiPython, err := exec.LookPath("python3")
	if err != nil {
		_ = stopDashboardChild(supervisor)
		fatal("dashboard requires python3 to serve the static UI")
	}
	uiServer := exec.Command(uiPython, "-m", "http.server", strconv.Itoa(chosenUIPort), "--bind", "127.0.0.1", "--directory", uiRoot)
	if err := startDashboardChild(uiServer); err != nil {
		_ = stopDashboardChild(supervisor)
		fatal(fmt.Sprintf("start dashboard UI: %v", err))
	}
	stop := func() {
		_ = stopDashboardChild(uiServer)
		_ = stopDashboardChild(supervisor)
	}
	defer stop()

	if err := waitDashboardHealth(supervisorURL, 8*time.Second); err != nil {
		fatal(fmt.Sprintf("local supervisor did not become ready: %v", err))
	}
	if err := waitDashboardPage(uiURL, 8*time.Second); err != nil {
		fatal(fmt.Sprintf("dashboard UI did not become ready: %v", err))
	}
	if err := waitDashboardFile(tokenFile, 8*time.Second); err != nil {
		fatal(fmt.Sprintf("local supervisor token did not become ready: %v", err))
	}
	token, err := os.ReadFile(tokenFile)
	if err != nil {
		fatal(fmt.Sprintf("read local supervisor token: %v", err))
	}
	if err := waitDashboardAuthenticated(supervisorURL, strings.TrimSpace(string(token)), 3*time.Second); err != nil {
		fatal(fmt.Sprintf("local supervisor authentication failed (is the selected port already in use?): %v", err))
	}
	fragment := url.Values{}
	fragment.Set("supervisor", supervisorURL)
	fragment.Set("token", strings.TrimSpace(string(token)))
	dashboardURL := uiURL + "/#" + fragment.Encode()
	fmt.Printf("Rewind dashboard ready: %s\n", dashboardURL)
	fmt.Printf("history: %s\n", history)
	fmt.Printf("policy: %s\n", policyFile)
	if !*noOpen {
		if opener, openErr := exec.LookPath("open"); openErr == nil {
			_ = exec.Command(opener, dashboardURL).Start()
		} else if opener, openErr := exec.LookPath("xdg-open"); openErr == nil {
			_ = exec.Command(opener, dashboardURL).Start()
		} else {
			fmt.Fprintln(os.Stderr, "dashboard browser opener unavailable; open the URL above manually")
		}
	}

	if *noShell {
		fmt.Println("protected shell disabled; press Ctrl-C to stop the local control plane")
		waitDashboardSignal()
		return
	}
	if err := runDashboardShell(executable, workspaceAbs, state, policyFile, history, *shellPath); err != nil {
		fmt.Fprintf(os.Stderr, "rewind dashboard shell: %v\n", err)
	}
	if !*exitAfterShell {
		fmt.Println("review is live in the dashboard; use rollback or commit there, then press Ctrl-C to stop Rewind")
		waitDashboardSignal()
	}
}

func runDashboardShell(executable, workspace, state, policy, history, shell string) error {
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		if runtime.GOOS == "darwin" {
			shell = "/bin/zsh"
		} else {
			shell = "/bin/bash"
		}
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	runtimeRoot := filepath.Join(state, "runtime-"+stamp)
	record := filepath.Join(state, "run-"+stamp+".json")
	args := []string{}
	if runtime.GOOS == "darwin" {
		args = []string{"native", "run", "--workspace", workspace, "--runtime-root", runtimeRoot, "--policy", policy, "--record", record, "--history", history, "--on-success", "review", "--", shell, "-i"}
	} else if runtime.GOOS == "linux" {
		args = []string{"run", "--workspace", workspace, "--runtime-root", runtimeRoot, "--policy", policy, "--record", record, "--history", history, "--overlay-backend", "fuse", "--runtime-roots", "/bin,/usr/bin,/lib,/usr/lib,/etc", "--on-success", "review", "--", shell, "-i"}
	} else {
		return fmt.Errorf("native protected shell is not available on %s; install the signed Windows helper before enabling host enforcement", runtime.GOOS)
	}
	cmd := exec.Command(executable, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "REWIND_DASHBOARD=1")
	fmt.Printf("starting protected shell (%s); run `rewind native diff --record %s` after exit to review\n", shell, record)
	return cmd.Run()
}

func startDashboardChild(cmd *exec.Cmd) error {
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Start()
}

func stopDashboardChild(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	_, _ = cmd.Process.Wait()
	return nil
}

func waitDashboardHealth(endpoint string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		response, err := client.Get(endpoint + "/health")
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("GET %s/health timed out", endpoint)
}

func waitDashboardPage(endpoint string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		response, err := client.Get(endpoint + "/")
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("GET %s/ timed out", endpoint)
}

func waitDashboardFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("%s timed out", path)
}

func waitDashboardAuthenticated(endpoint, token string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		request, err := http.NewRequest(http.MethodGet, endpoint+"/v1/history", nil)
		if err == nil {
			request.Header.Set("Authorization", "Bearer "+token)
			response, requestErr := client.Do(request)
			if requestErr == nil {
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return nil
				}
				if response.StatusCode == http.StatusUnauthorized {
					return fmt.Errorf("HTTP 401 bearer token rejected")
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("authenticated history request timed out")
}

func reserveDashboardPort(requested int) (int, error) {
	address := "127.0.0.1:0"
	if requested != 0 {
		address = fmt.Sprintf("127.0.0.1:%d", requested)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	return port, nil
}

func waitDashboardSignal() {
	channel := make(chan os.Signal, 1)
	signal.Notify(channel, os.Interrupt, syscall.SIGTERM)
	<-channel
	signal.Stop(channel)
}

func mustWorkingDirectory() string {
	directory, err := os.Getwd()
	if err != nil {
		return "."
	}
	return directory
}
