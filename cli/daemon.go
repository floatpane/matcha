package cli

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/floatpane/matcha/config"
	matchaDaemon "github.com/floatpane/matcha/daemon"
	"github.com/floatpane/matcha/daemonclient"
	"github.com/floatpane/matcha/daemonrpc"
)

// RunDaemon handles the "matcha daemon" subcommand.
func RunDaemon(args []string) {
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
		runDaemonStart()
	case "stop":
		runDaemonStop()
	case "status":
		runDaemonStatus()
	case "run":
		runDaemonRun()
	default:
		fmt.Fprintf(os.Stderr, "unknown daemon command: %s\n", args[0])
		os.Exit(1)
	}
}

func runDaemonStart() {
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

	cmd := exec.Command(exe, "daemon", "run")
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

func runDaemonStop() {
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

func runDaemonStatus() {
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
	defer client.Close()

	status, err := client.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get status: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Daemon running (PID %d)\n", status.PID)
	fmt.Printf("Uptime: %s\n", formatUptime(status.Uptime))
	fmt.Printf("Accounts: %d\n", len(status.Accounts))
	for _, acct := range status.Accounts {
		fmt.Printf("  - %s\n", acct)
	}
}

func runDaemonRun() {
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

func formatUptime(seconds int64) string {
	d := time.Duration(seconds) * time.Second
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}
