package cli

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/floatpane/matcha/assets"
	"github.com/floatpane/matcha/daemonrpc"
)

//go:embed macos_menubar.swift
var macosMenubarSwift string

const (
	helperBundleID   = "com.floatpane.matcha.menubar-helper"
	helperAppName    = "MatchaHelper"
	helperExecutable = "MatchaHelper"
)

// RunHelper handles `matcha helper <install|uninstall|status>`. It manages the
// macOS menu bar agent that shows the unread count, posts new-mail
// notifications, and lets you open Matcha from the menu bar.
func RunHelper(args []string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("the menu bar helper is only available on macOS")
	}
	sub := "install"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "install":
		return installHelper()
	case "uninstall", "remove":
		return uninstallHelper()
	case "status":
		return helperStatus()
	default:
		return fmt.Errorf("usage: matcha helper <install|uninstall|status>")
	}
}

// helperAppDir returns ~/Applications/MatchaHelper.app.
func helperAppDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	return filepath.Join(home, "Applications", helperAppName+".app"), nil
}

// helperLaunchAgentPath returns the per-user LaunchAgent plist path.
func helperLaunchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", helperBundleID+".plist"), nil
}

func installHelper() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not find executable: %w", err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return fmt.Errorf("could not resolve absolute path: %w", err)
	}

	appDir, err := helperAppDir()
	if err != nil {
		return err
	}
	// Remove any previous install so a stale binary can't linger.
	_ = stopLaunchAgent()
	stopRunningHelper()
	os.RemoveAll(appDir) //nolint:errcheck,gosec

	contentsDir := filepath.Join(appDir, "Contents")
	macosDir := filepath.Join(contentsDir, "MacOS")
	resourcesDir := filepath.Join(contentsDir, "Resources")
	if err := os.MkdirAll(macosDir, 0750); err != nil {
		return err
	}
	if err := os.MkdirAll(resourcesDir, 0750); err != nil {
		return err
	}

	// Write the logo PNG into Resources so the Swift app has a stable path for
	// the menu bar icon and notification image.
	iconPNG := filepath.Join(resourcesDir, "logo.png")
	if err := os.WriteFile(iconPNG, assets.Logo, 0644); err != nil {
		return fmt.Errorf("could not write logo: %w", err)
	}

	// Generate an .icns for the bundle icon from the embedded logo. icns needs
	// a standard square size, so resize a temporary copy to 512x512 first.
	icnsPath := filepath.Join(resourcesDir, helperAppName+".icns")
	tmpIcon := filepath.Join(os.TempDir(), "matcha_helper_icon.png")
	if err := os.WriteFile(tmpIcon, assets.Logo, 0644); err == nil {
		_ = exec.Command("sips", "-z", "512", "512", tmpIcon).Run()                        //nolint:noctx
		_ = exec.Command("sips", "-s", "format", "icns", tmpIcon, "--out", icnsPath).Run() //nolint:noctx
		_ = os.Remove(tmpIcon)
	}

	if err := os.WriteFile(filepath.Join(contentsDir, "Info.plist"), []byte(helperInfoPlist()), 0644); err != nil {
		return err
	}

	// Template the matcha binary path and icon path into the Swift source, then
	// compile it into the bundle's executable.
	swiftSrc := macosMenubarSwift
	swiftSrc = strings.ReplaceAll(swiftSrc, "{{MATCHA_PATH}}", exe)
	swiftSrc = strings.ReplaceAll(swiftSrc, "{{ICON_PATH}}", iconPNG)

	tmpSwift := filepath.Join(os.TempDir(), "matcha_menubar.swift")
	if err := os.WriteFile(tmpSwift, []byte(swiftSrc), 0644); err != nil {
		return err
	}
	defer os.Remove(tmpSwift) //nolint:errcheck

	exeDest := filepath.Join(macosDir, helperExecutable)
	cmd := exec.Command("swiftc", "-O", tmpSwift, "-o", exeDest) //nolint:noctx
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to compile menu bar helper (is the Xcode Command Line Tools' swiftc installed?): %w", err)
	}

	// Ad-hoc code-sign the whole bundle. NSUserNotification only displays for a
	// properly signed bundle (swiftc only linker-signs the inner executable),
	// and signing after writing all bundle contents keeps the seal valid.
	if err := exec.Command("codesign", "--force", "--deep", "--sign", "-", appDir).Run(); err != nil { //nolint:noctx
		fmt.Fprintf(os.Stderr, "warning: could not code-sign helper bundle: %v\n", err)
	}

	// Register the bundle with Launch Services.
	lsregister := "/System/Library/Frameworks/CoreServices.framework/Versions/A/Frameworks/LaunchServices.framework/Versions/A/Support/lsregister"
	_ = exec.Command(lsregister, "-f", appDir).Run() //nolint:noctx

	// Install and load the LaunchAgent so it runs now and at login.
	if err := installLaunchAgent(exeDest); err != nil {
		return err
	}

	fmt.Printf("Installed %s.app and started the Matcha menu bar helper.\n", helperAppName)
	fmt.Println("A 🍵 icon should now appear in your menu bar showing your unread count.")
	fmt.Println("It starts automatically at login. Remove it with: matcha helper uninstall")
	return nil
}

func uninstallHelper() error {
	if err := stopLaunchAgent(); err != nil {
		// Non-fatal: the agent may not be loaded.
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	stopRunningHelper()

	agentPath, err := helperLaunchAgentPath()
	if err != nil {
		return err
	}
	_ = os.Remove(agentPath)

	appDir, err := helperAppDir()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(appDir); err != nil {
		return fmt.Errorf("could not remove %s: %w", appDir, err)
	}

	fmt.Printf("Removed the Matcha menu bar helper (%s.app and LaunchAgent).\n", helperAppName)
	return nil
}

func helperStatus() error {
	appDir, _ := helperAppDir()
	agentPath, _ := helperLaunchAgentPath()

	appInstalled := fileExists(filepath.Join(appDir, "Contents", "MacOS", helperExecutable))
	agentInstalled := fileExists(agentPath)

	fmt.Printf("Menu bar app:  %s\n", installedLabel(appInstalled))
	fmt.Printf("LaunchAgent:   %s\n", installedLabel(agentInstalled))
	fmt.Printf("Daemon socket: %s\n", daemonLabel())
	if !appInstalled {
		fmt.Println("\nRun `matcha helper install` to set it up.")
	}
	return nil
}

func installedLabel(ok bool) string {
	if ok {
		return "installed"
	}
	return "not installed"
}

func daemonLabel() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", daemonrpc.SocketPath())
	if err != nil {
		return "not running"
	}
	_ = conn.Close()
	return "running"
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// installLaunchAgent writes the LaunchAgent plist and loads it. exePath is the
// absolute path to the compiled helper executable inside the bundle.
func installLaunchAgent(exePath string) error {
	agentPath, err := helperLaunchAgentPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(agentPath), 0750); err != nil {
		return err
	}
	if err := os.WriteFile(agentPath, []byte(helperLaunchAgentPlist(exePath)), 0644); err != nil {
		return err
	}

	// Prefer the modern bootstrap interface; fall back to load -w on older
	// systems. Both are best-effort — the agent still runs at next login.
	domain := "gui/" + strconv.Itoa(os.Getuid())
	if err := exec.Command("launchctl", "bootstrap", domain, agentPath).Run(); err != nil { //nolint:noctx
		_ = exec.Command("launchctl", "load", "-w", agentPath).Run() //nolint:noctx
	}
	return nil
}

// stopRunningHelper terminates any running helper instances by name. The
// LaunchAgent's bootout only stops processes launchd itself started, so an
// instance launched another way (or an orphan from a prior install) would
// otherwise linger and post duplicate notifications. Best-effort.
func stopRunningHelper() {
	_ = exec.Command("killall", helperExecutable).Run() //nolint:noctx
}

// stopLaunchAgent unloads the LaunchAgent if present.
func stopLaunchAgent() error {
	agentPath, err := helperLaunchAgentPath()
	if err != nil {
		return err
	}
	if !fileExists(agentPath) {
		return nil
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	if err := exec.Command("launchctl", "bootout", domain+"/"+helperBundleID).Run(); err != nil { //nolint:noctx
		_ = exec.Command("launchctl", "unload", "-w", agentPath).Run() //nolint:noctx
	}
	return nil
}

func helperInfoPlist() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key>
	<string>` + helperExecutable + `</string>
	<key>CFBundleIconFile</key>
	<string>` + helperAppName + `.icns</string>
	<key>CFBundleIdentifier</key>
	<string>` + helperBundleID + `</string>
	<key>CFBundleName</key>
	<string>` + helperAppName + `</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>1.0</string>
	<key>CFBundleVersion</key>
	<string>1</string>
	<key>LSUIElement</key>
	<true/>
	<key>LSMinimumSystemVersion</key>
	<string>11.0</string>
</dict>
</plist>
`
}

func helperLaunchAgentPlist(exePath string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + helperBundleID + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + exePath + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<false/>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`
}
