package daemoncmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/floatpane/matcha/config"
	matchaDaemon "github.com/floatpane/matcha/daemon"
	"github.com/floatpane/matcha/daemonclient"
	"github.com/floatpane/matcha/daemonrpc"
)

// Run implements the CLI entrypoint for `matcha daemon <start|stop|status|run>`.
func Run(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: matcha daemon <start|stop|status|run>")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  start   Start the daemon in the background")
		fmt.Println("  stop    Stop the running daemon")
		fmt.Println("  status  Show daemon status")
		fmt.Println("  run     Run the daemon in the foreground")
		os.Exit(1)
	}

	switch args[0] {
	case "start":
		runStart()
	case "stop":
		runStop()
	case "status":
		runStatus()
	case "run":
		runRun()
	default:
		fmt.Fprintf(os.Stderr, "unknown daemon command: %s\n", args[0])
		os.Exit(1)
	}
}

func runStart() {
	pidPath := daemonrpc.PIDPath()
	if pid, running := matchaDaemon.IsRunning(pidPath); running {
		fmt.Printf("Daemon already running (PID %d)\n", pid)
		return
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot find executable: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command(exe, "daemon", "run") //nolint:noctx
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	cmd.SysProcAttr = daemonclient.DaemonProcAttr()

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start daemon: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Daemon started (PID %d)\n", cmd.Process.Pid)
}

func runStop() {
	pidPath := daemonrpc.PIDPath()
	pid, running := matchaDaemon.IsRunning(pidPath)
	if !running {
		fmt.Println("Daemon is not running")
		return
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot find process %d: %v\n", pid, err)
		os.Exit(1)
	}

	if err := process.Signal(os.Interrupt); err != nil {
		fmt.Fprintf(os.Stderr, "failed to stop daemon: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Daemon stopped (PID %d)\n", pid)
}

func runStatus() {
	client, err := daemonclient.Dial()
	if err != nil {
		pidPath := daemonrpc.PIDPath()
		if pid, running := matchaDaemon.IsRunning(pidPath); running {
			fmt.Printf("Daemon running (PID %d) but not responding\n", pid)
		} else {
			fmt.Println("Daemon is not running")
		}
		return
	}
	status, err := client.Status()
	client.Close() //nolint:errcheck,gosec
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get status: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Daemon running (PID %d)\n", status.PID)
	fmt.Printf("Uptime: %s\n", FormatUptime(status.Uptime))
	fmt.Printf("Accounts: %d\n", len(status.Accounts))
	for _, acct := range status.Accounts {
		fmt.Printf("  - %s\n", acct)
	}
}

func runRun() {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	d := matchaDaemon.New(cfg)
	if err := d.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
		os.Exit(1)
	}
}

// FormatUptime returns a human-readable representation of daemon uptime.
func FormatUptime(seconds int64) string {
	d := seconds
	if d < 60 {
		return fmt.Sprintf("%ds", d)
	}
	if d < 3600 {
		return fmt.Sprintf("%dm %ds", d/60, d%60)
	}
	return fmt.Sprintf("%dh %dm", d/3600, (d%3600)/60)
}
