package daemoncmd

import (
	"fmt"
	"log"
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
		log.Println("Usage: matcha daemon <start|stop|status|run>")
		log.Println()
		log.Println("Commands:")
		log.Println("  start   Start the daemon in the background")
		log.Println("  stop    Stop the running daemon")
		log.Println("  status  Show daemon status")
		log.Println("  run     Run the daemon in the foreground")
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
		log.Fatalf("unknown daemon command: %s", args[0])
	}
}

func runStart() {
	pidPath := daemonrpc.PIDPath()
	if pid, running := matchaDaemon.IsRunning(pidPath); running {
		log.Printf("Daemon already running (PID %d)\n", pid)
		return
	}

	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("cannot find executable: %v", err)
	}

	cmd := exec.Command(exe, "daemon", "run") //nolint:noctx
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	cmd.SysProcAttr = daemonclient.DaemonProcAttr()

	if err := cmd.Start(); err != nil {
		log.Fatalf("failed to start daemon: %v", err)
	}

	log.Printf("Daemon started (PID %d)\n", cmd.Process.Pid)
}

func runStop() {
	pidPath := daemonrpc.PIDPath()
	pid, running := matchaDaemon.IsRunning(pidPath)
	if !running {
		log.Println("Daemon is not running")
		return
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		log.Fatalf("cannot find process %d: %v", pid, err)
	}

	if err := process.Signal(os.Interrupt); err != nil {
		log.Fatalf("failed to stop daemon: %v", err)
	}

	log.Printf("Daemon stopped (PID %d)\n", pid)
}

func runStatus() {
	client, err := daemonclient.Dial()
	if err != nil {
		pidPath := daemonrpc.PIDPath()
		if pid, running := matchaDaemon.IsRunning(pidPath); running {
			log.Printf("Daemon running (PID %d) but not responding\n", pid)
		} else {
			log.Println("Daemon is not running")
		}
		return
	}
	status, err := client.Status()
	client.Close() //nolint:errcheck,gosec
	if err != nil {
		log.Fatalf("failed to get status: %v", err)
	}

	log.Printf("Daemon running (PID %d)\n", status.PID)
	log.Printf("Uptime: %s\n", FormatUptime(status.Uptime))
	log.Printf("Accounts: %d\n", len(status.Accounts))
	for _, acct := range status.Accounts {
		log.Printf("  - %s\n", acct)
	}
}

func runRun() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	d := matchaDaemon.New(cfg)
	if err := d.Run(); err != nil {
		log.Fatalf("daemon error: %v", err)
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
