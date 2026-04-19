package daemon

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

// handleSignals listens for OS signals and triggers daemon actions.
// SIGTERM/SIGINT → graceful shutdown
// SIGHUP → config reload
func (d *Daemon) handleSignals() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	for {
		select {
		case sig := <-ch:
			switch sig {
			case syscall.SIGTERM, syscall.SIGINT:
				log.Println("daemon: received shutdown signal")
				d.Shutdown()
				return
			case syscall.SIGHUP:
				log.Println("daemon: received SIGHUP, reloading config")
				if err := d.ReloadConfig(); err != nil {
					log.Printf("daemon: config reload failed: %v", err)
				}
			}
		case <-d.shutdown:
			return
		}
	}
}
