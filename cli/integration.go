package cli

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/floatpane/matcha/assets"
)

//go:embed macos_handler.swift
var macosHandlerSwift string

// SetupMailto registers matcha as the default handler for mailto: links.
func SetupMailto() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not find executable: %w", err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return fmt.Errorf("could not resolve absolute path: %w", err)
	}

	switch runtime.GOOS {
	case "linux":
		return setupMailtoLinux(exe)
	case "darwin":
		return setupMailtoDarwin(exe)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func setupMailtoLinux(exe string) error {
	desktopContent := fmt.Sprintf(`[Desktop Entry]
Name=Matcha Email
Comment=Terminal-based email client
Exec=%s %%u
Terminal=true
Type=Application
Icon=matcha
Categories=Network;Email;
MimeType=x-scheme-handler/mailto;
`, exe)

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	iconsDir := filepath.Join(home, ".local", "share", "icons", "hicolor", "512x512", "apps")
	if err := os.MkdirAll(iconsDir, 0755); err == nil {
		iconFile := filepath.Join(iconsDir, "matcha.png")
		_ = os.WriteFile(iconFile, assets.Logo, 0644)
		_ = exec.Command("gtk-update-icon-cache", filepath.Join(home, ".local", "share", "icons", "hicolor")).Run()
	}

	appsDir := filepath.Join(home, ".local", "share", "applications")
	if err := os.MkdirAll(appsDir, 0755); err != nil {
		return err
	}

	desktopFile := filepath.Join(appsDir, "matcha.desktop")
	if err := os.WriteFile(desktopFile, []byte(desktopContent), 0644); err != nil {
		return err
	}

	// Update desktop database
	if err := exec.Command("update-desktop-database", appsDir).Run(); err != nil {
		// Ignore error if command doesn't exist
	}

	// Try to set xdg-mime default
	cmd := exec.Command("xdg-mime", "default", "matcha.desktop", "x-scheme-handler/mailto")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run xdg-mime: %w", err)
	}

	fmt.Printf("Successfully registered %s as default mail handler on Linux\n", exe)
	return nil
}

func setupMailtoDarwin(exe string) error {
	// For macOS, we need to create a tiny AppleScript/Swift app bundle to handle the URL event,
	// because standard terminal programs can't easily register as URL handlers without an app bundle.

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	appDir := filepath.Join(home, "Applications", "MatchaMail.app")
	// Cleanup old version to avoid conflicts
	os.RemoveAll(appDir)

	contentsDir := filepath.Join(appDir, "Contents")
	macosDir := filepath.Join(contentsDir, "MacOS")
	resourcesDir := filepath.Join(contentsDir, "Resources")

	if err := os.MkdirAll(macosDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(resourcesDir, 0755); err != nil {
		return err
	}

	// Generate .icns from embedded logo
	tmpLogo := filepath.Join(os.TempDir(), "matcha_logo.png")
	if err := os.WriteFile(tmpLogo, assets.Logo, 0644); err == nil {
		icnsPath := filepath.Join(resourcesDir, "MatchaMail.icns")
		_ = exec.Command("sips", "-s", "format", "icns", tmpLogo, "--out", icnsPath).Run()
		os.Remove(tmpLogo)
	}

	infoPlist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key>
	<string>MatchaMail</string>
	<key>CFBundleIconFile</key>
	<string>MatchaMail.icns</string>
	<key>CFBundleIdentifier</key>
	<string>com.floatpane.matcha.mailto-handler</string>
	<key>CFBundleName</key>
	<string>MatchaMail</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>1.1</string>
	<key>CFBundleVersion</key>
	<string>1</string>
	<key>LSUIElement</key>
	<true/>
	<key>CFBundleURLTypes</key>
	<array>
		<dict>
			<key>CFBundleURLName</key>
			<string>Email Address</string>
			<key>CFBundleURLSchemes</key>
			<array>
				<string>mailto</string>
			</array>
			<key>LSHandlerRank</key>
			<string>Owner</string>
		</dict>
	</array>
</dict>
</plist>
`
	if err := os.WriteFile(filepath.Join(contentsDir, "Info.plist"), []byte(infoPlist), 0644); err != nil {
		return err
	}

	// Swift source code to handle URL event and launch Terminal.app running matcha
	swiftCode := strings.ReplaceAll(macosHandlerSwift, "{{MATCHA_PATH}}", exe)

	tmpSwiftFile := filepath.Join(os.TempDir(), "matcha_handler.swift")
	if err := os.WriteFile(tmpSwiftFile, []byte(swiftCode), 0644); err != nil {
		return err
	}
	defer os.Remove(tmpSwiftFile)

	exeDest := filepath.Join(macosDir, "MatchaMail")

	// Compile the Swift file
	cmd := exec.Command("swiftc", "-O", tmpSwiftFile, "-o", exeDest)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to compile Swift handler app: %w", err)
	}

	// Register the application
	lsregister := "/System/Library/Frameworks/CoreServices.framework/Versions/A/Frameworks/LaunchServices.framework/Versions/A/Support/lsregister"
	_ = exec.Command(lsregister, "-f", appDir).Run()

	fmt.Printf("Successfully created %s.\n", appDir)

	// Set as default handler
	// macOS does not provide a straightforward CLI to change default handler without 3rd party tools (like duti).
	// We'll instruct the user on how to do it or try our best.
	// Actually, starting from macOS 12, there's no native Apple command for it. But registering it usually makes it show up in Apple Mail -> Preferences -> Default email reader.

	fmt.Printf("Successfully created %s.\n", appDir)
	fmt.Println("To complete the setup on macOS:")
	fmt.Println("1. Open Apple Mail.")
	fmt.Println("2. Go to Mail -> Settings (or Preferences) -> General.")
	fmt.Println("3. Select 'MatchaMail.app' from the 'Default email reader' dropdown.")

	return nil
}
