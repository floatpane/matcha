package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/internal/httpclient"
	"github.com/floatpane/matcha/internal/loglevel"
)

const (
	goosDarwin  = "darwin"
	goosLinux   = "linux"
	goosWindows = "windows"

	githubLatestReleaseAPI = "https://api.github.com/repos/floatpane/matcha/releases/latest"
)

// httpClient is used for all outbound HTTP requests in this package (update
// checks and asset downloads). It is preconfigured with a redirect cap to avoid
// infinite redirect chains.
var httpClient = httpclient.NewWithRedirectCap(httpclient.UpdateCheckTimeout, 5)

// UpdateAvailableMsg is sent into the TUI when a newer release is detected.
type UpdateAvailableMsg struct {
	Latest  string
	Current string
}

// githubRelease is the internal struct for parsing GitHub release JSON.
type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// compareVersions reports whether a is a newer version than b, honoring the
// special v1.0.0-rcN ordering used by the release/v1 stabilization branch. RC
// versions are compared by their numeric suffix; otherwise the standard semver
// ordering is used, falling back to a simple suffix check and string comparison.
func isNewer(a, b string) bool {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")
	if a == b {
		return false
	}

	// Compare v1.0.0-rcN versions by suffix number.
	if isRC(a) && isRC(b) {
		return rcNumber(a) > rcNumber(b)
	}

	// If one side is an rc and the other is not, semver handles the
	// prerelease ordering (rc < stable). We only get here when both sides
	// have been normalized to the same family.
	return semverLess(b, a)
}

// isRC reports whether a version is a v1 release-candidate pre-release,
// e.g. v1.0.0-rc1 or 1.0.0-rc2.
func isRC(v string) bool {
	return strings.Contains(v, "1.0.0-rc")
}

// rcNumber extracts the numeric suffix from a v1.0.0-rcN version. If the
// suffix is not present or not numeric, it returns 0.
func rcNumber(v string) int {
	v = strings.TrimPrefix(v, "v")
	if !strings.HasPrefix(v, "1.0.0-rc") {
		return 0
	}
	n := strings.TrimPrefix(v, "1.0.0-rc")
	if num, err := strconv.Atoi(n); err == nil {
		return num
	}
	return 0
}

// semverLess reports whether a < b using a lightweight semver parse. It only
// supports numeric major/minor/patch components and a single optional
// pre-release suffix (e.g. rc1). It returns false when comparison is ambiguous.
func semverLess(a, b string) bool {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")
	if a == b {
		return false
	}

	pa, preA := splitPre(a)
	pb, preB := splitPre(b)

	sa := strings.Split(pa, ".")
	sb := strings.Split(pb, ".")
	limit := len(sa)
	if len(sb) > limit {
		limit = len(sb)
	}
	for i := 0; i < limit; i++ {
		na, nb := 0, 0
		if i < len(sa) {
			na, _ = strconv.Atoi(sa[i])
		}
		if i < len(sb) {
			nb, _ = strconv.Atoi(sb[i])
		}
		if na < nb {
			return true
		}
		if na > nb {
			return false
		}
	}

	// Core version equal; decide by pre-release presence and suffix.
	if preA == "" && preB != "" {
		return false // a is stable, b is prerelease -> a > b -> a < b is false
	}
	if preA != "" && preB == "" {
		return true // a is prerelease, b is stable -> a < b
	}
	if preA == preB {
		return false
	}
	// Both have numeric suffixes; compare them.
	na, _ := strconv.Atoi(preA)
	nb, _ := strconv.Atoi(preB)
	return na < nb
}

// splitPre separates a version string into core and pre-release suffix. It
// looks for the first non-numeric, non-dot character and treats the remainder as
// the pre-release suffix (e.g. "1.0.0-rc1" -> ("1.0.0", "1")).
func splitPre(v string) (core, pre string) {
	for i, r := range v {
		if (r < '0' || r > '9') && r != '.' {
			return v[:i], strings.TrimLeft(v[i:], "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-+")
		}
	}
	return v, ""
}

// SetVersion provides the build-injected version value to the updater package.
// It should be called during program initialization before any update check is run.
func SetVersion(v string) {
	version = v
}

// version holds the current build version. It is set by main via SetVersion.
var version = "dev"

// DetectInstalledVersion returns a best-effort installed version string.
//
// Priority:
//  1. If the build-in version variable is set to something other than "dev", return it.
//  2. If Homebrew is present and reports a version for matcha, return that.
//  3. If WinGet is present and reports a version for matcha, return that.
//  4. If snap is present and lists matcha, return that.
//  5. If Flatpak is present and reports a version for matcha, return that.
//  6. Fallback to the build version (likely "dev").
func DetectInstalledVersion() string {
	v := strings.TrimSpace(version)
	if v != "dev" && v != "" {
		return v
	}

	// Try Homebrew (macOS)
	if runtime.GOOS == goosDarwin {
		if _, err := exec.LookPath("brew"); err == nil {
			// `brew list --versions matcha` prints: matcha 1.2.3
			if out, err := exec.Command("brew", "list", "--versions", "matcha").Output(); err == nil { //nolint:noctx
				parts := strings.Fields(string(out))
				if len(parts) >= 2 {
					return parts[1]
				}
			}
		}
	}

	// Try WinGet (Windows)
	if runtime.GOOS == goosWindows {
		if _, err := exec.LookPath("winget"); err == nil {
			if out, err := exec.Command("winget", "list", "--id", "floatpane.matcha", "--disable-interactivity").Output(); err == nil { //nolint:noctx
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				for _, line := range lines {
					if strings.Contains(strings.ToLower(line), "floatpane.matcha") {
						fields := strings.Fields(line)
						for _, f := range fields {
							if len(f) > 0 && f[0] >= '0' && f[0] <= '9' && strings.Contains(f, ".") {
								return f
							}
						}
					}
				}
			}
		}
	}

	// Try snap (Linux)
	if runtime.GOOS == goosLinux {
		if _, err := exec.LookPath("snap"); err == nil {
			if out, err := exec.Command("snap", "list", "matcha").Output(); err == nil { //nolint:noctx
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				if len(lines) >= 2 {
					fields := strings.Fields(lines[1])
					if len(fields) >= 2 {
						return fields[1]
					}
				}
			}
		}

		if _, err := exec.LookPath("flatpak"); err == nil {
			if out, err := exec.Command("flatpak", "info", "com.floatpane.matcha").Output(); err == nil { //nolint:noctx
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "Version:") {
						fields := strings.Fields(line)
						if len(fields) >= 2 {
							return fields[1]
						}
					}
				}
			}
		}
	}

	return v
}

// CheckForUpdatesCmd queries GitHub for the latest release tag and returns a
// tea.Msg (UpdateAvailableMsg) if the latest version differs from the current
// installed version. This runs in the background when the TUI initializes.
func CheckForUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		resp, err := httpClient.Get(githubLatestReleaseAPI)
		if err != nil {
			return nil
		}
		defer resp.Body.Close() //nolint:errcheck

		var rel githubRelease
		if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
			return nil
		}

		latest := strings.TrimPrefix(rel.TagName, "v")
		installed := strings.TrimPrefix(DetectInstalledVersion(), "v")
		if latest != "" && installed != "" && latest != installed && isNewer(latest, installed) {
			return UpdateAvailableMsg{Latest: latest, Current: installed}
		}
		return nil
	}
}

// RunUpdateCLI implements the CLI entrypoint for `matcha update`.
// It detects the likely installation method and attempts the appropriate
// update path (Homebrew, Snap, Flatpak, AUR, Nix, WinGet, Scoop, or manual
// GitHub release binary extract).
func RunUpdateCLI() (err error) {
	resp, err := httpClient.Get(githubLatestReleaseAPI)
	if err != nil {
		return fmt.Errorf("could not query releases: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return fmt.Errorf("could not parse release info: %w", err)
	}

	latestTag := strings.TrimPrefix(rel.TagName, "v")

	loglevel.Infof("Current version: %s", version)
	loglevel.Infof("Latest version: %s", latestTag)

	cur := strings.TrimPrefix(version, "v")
	if latestTag == "" || cur == latestTag || !isNewer(latestTag, cur) {
		loglevel.Infof("Already up to date.")
		return nil
	}

	// When running on a v1 release candidate, only allow update paths that are
	// actually publishing v1 RCs: Homebrew (matcha@v1), Snap (candidate), and the
	// manual GitHub release binary fallback. All other package managers are
	// still tied to v0 stable releases and would downgrade or reinstall the
	// wrong stream, so skip them.
	isRCInstalled := isRC(cur)

	switch runtime.GOOS {
	case goosDarwin:
		if tryHomebrewUpgrade() {
			return nil
		}
	case goosLinux:
		if trySnapRefresh() {
			return nil
		}
		if !isRCInstalled {
			if tryFlatpakUpdate() {
				return nil
			}
			if tryAURUpdate() {
				return nil
			}
			if tryNixUpdate() {
				return nil
			}
		}
	case goosWindows:
		if !isRCInstalled {
			if tryWinGetUpgrade() {
				return nil
			}
			if tryScoopUpdate() {
				return nil
			}
		}
	}

	return runUpdateCLIManual(latestTag, rel)
}

func tryHomebrewUpgrade() bool {
	if _, err := exec.LookPath("brew"); err != nil {
		return false
	}

	loglevel.Infof("Detected Homebrew — updating taps and attempting to upgrade via brew.")

	updateCmd := exec.Command("brew", "update") //nolint:noctx
	updateCmd.Stdout = os.Stdout
	updateCmd.Stderr = os.Stderr
	if err := updateCmd.Run(); err != nil {
		loglevel.Infof("Homebrew update failed: %v", err)
	}

	upgradeCmd := exec.Command("brew", "upgrade", "floatpane/matcha/matcha@v1") //nolint:noctx
	upgradeCmd.Stdout = os.Stdout
	upgradeCmd.Stderr = os.Stderr
	if err := upgradeCmd.Run(); err == nil {
		loglevel.Infof("Successfully upgraded via Homebrew.")
		return true
	}
	loglevel.Infof("Homebrew upgrade failed")
	return false
}

func trySnapRefresh() bool {
	if _, err := exec.LookPath("snap"); err != nil {
		return false
	}

	cmdCheck := exec.Command("snap", "list", "matcha") //nolint:noctx
	if err := cmdCheck.Run(); err != nil {
		return false
	}

	loglevel.Infof("Detected Snap package — attempting to refresh.")
	cmd := exec.Command("snap", "refresh", "matcha", "--candidate") //nolint:noctx
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err == nil {
		loglevel.Infof("Successfully refreshed snap.")
		return true
	}
	loglevel.Infof("Snap refresh failed")
	return false
}

func tryFlatpakUpdate() bool {
	if _, err := exec.LookPath("flatpak"); err != nil {
		return false
	}

	cmdCheck := exec.Command("flatpak", "info", "com.floatpane.matcha") //nolint:noctx
	if err := cmdCheck.Run(); err != nil {
		return false
	}

	loglevel.Infof("Detected Flatpak package — attempting to update.")
	cmd := exec.Command("flatpak", "update", "-y", "com.floatpane.matcha") //nolint:noctx
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err == nil {
		loglevel.Infof("Successfully updated flatpak.")
		return true
	}
	loglevel.Infof("Flatpak update failed")
	return false
}

func tryAURUpdate() bool {
	if _, err := exec.LookPath("yay"); err != nil {
		return false
	}

	cmdCheck := exec.Command("yay", "-Q", "matcha-client-bin") //nolint:noctx
	if err := cmdCheck.Run(); err != nil {
		return false
	}

	loglevel.Infof("Detected AUR package (matcha-client-bin) — attempting to update via yay.")
	cmd := exec.Command("yay", "-Syu", "--noconfirm", "matcha-client-bin") //nolint:noctx
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err == nil {
		loglevel.Infof("Successfully updated via AUR.")
		return true
	}
	loglevel.Infof("AUR update failed")
	return false
}

func tryNixUpdate() bool {
	if _, err := exec.LookPath("nix"); err != nil {
		return false
	}

	cmdCheck := exec.Command("nix", "profile", "list") //nolint:noctx
	output, err := cmdCheck.Output()
	if err != nil || !strings.Contains(string(output), "matcha") {
		return false
	}

	loglevel.Infof("Detected Nix package — attempting to update via nix profile upgrade.")
	cmd := exec.Command("nix", "profile", "upgrade", "github:floatpane/matcha") //nolint:noctx
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err == nil {
		loglevel.Infof("Successfully updated via Nix.")
		return true
	}
	loglevel.Infof("Nix update failed")
	return false
}

func tryWinGetUpgrade() bool {
	if _, err := exec.LookPath("winget"); err != nil {
		return false
	}

	cmdCheck := exec.Command("winget", "list", "--id", "floatpane.matcha", "--disable-interactivity") //nolint:noctx
	if err := cmdCheck.Run(); err != nil {
		return false
	}

	loglevel.Infof("Detected WinGet package — attempting to upgrade.")
	cmd := exec.Command("winget", "upgrade", "--id", "floatpane.matcha", "--disable-interactivity") //nolint:noctx
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err == nil {
		loglevel.Infof("Successfully upgraded via WinGet.")
		return true
	}
	loglevel.Infof("WinGet upgrade failed")
	return false
}

func tryScoopUpdate() bool {
	if _, err := exec.LookPath("scoop"); err != nil {
		return false
	}

	cmdCheck := exec.Command("scoop", "list", "matcha") //nolint:noctx
	if err := cmdCheck.Run(); err != nil {
		return false
	}

	loglevel.Infof("Detected Scoop package — attempting to update.")
	cmd := exec.Command("scoop", "update", "matcha") //nolint:noctx
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err == nil {
		loglevel.Infof("Successfully updated via Scoop.")
		return true
	}
	loglevel.Infof("Scoop update failed")
	return false
}

func extractBinaryFromArchive(assetPath, assetName, tmpDir string) (string, error) {
	binaryName := "matcha"
	if runtime.GOOS == goosWindows {
		binaryName = "matcha.exe"
	}

	var binPath string
	if strings.HasSuffix(assetName, ".tar.gz") || strings.HasSuffix(assetName, ".tgz") { //nolint:gocritic
		f, err := os.Open(assetPath)
		if err != nil {
			return "", fmt.Errorf("could not open archive: %w", err)
		}
		defer f.Close() //nolint:errcheck
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return "", fmt.Errorf("could not create gzip reader: %w", err)
		}
		tr := tar.NewReader(gzr)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", fmt.Errorf("error reading tar: %w", err)
			}
			name := filepath.Base(hdr.Name)
			if name == binaryName || strings.Contains(strings.ToLower(name), "matcha") && (hdr.Typeflag == tar.TypeReg) {
				binPath = filepath.Join(tmpDir, binaryName)
				out, err := os.Create(binPath)
				if err != nil {
					return "", fmt.Errorf("could not create binary file: %w", err)
				}
				if _, err := io.Copy(out, tr); err != nil { // #nosec G110 -- archive is our own signed GitHub release
					_ = out.Close()
					return "", fmt.Errorf("could not extract binary: %w", err)
				}
				if err := out.Close(); err != nil {
					return "", fmt.Errorf("could not finalize extracted binary: %w", err)
				}
				if err := os.Chmod(binPath, 0o755); err != nil { // #nosec G302 -- downloaded binary must be executable
					return "", fmt.Errorf("could not make binary executable: %w", err)
				}
				break
			}
		}
	} else if strings.HasSuffix(assetName, ".zip") {
		zr, err := zip.OpenReader(assetPath)
		if err != nil {
			return "", fmt.Errorf("could not open zip archive: %w", err)
		}
		defer zr.Close() //nolint:errcheck
		for _, zf := range zr.File {
			name := filepath.Base(zf.Name)
			if name == binaryName || strings.Contains(strings.ToLower(name), "matcha") && !zf.FileInfo().IsDir() {
				rc, err := zf.Open()
				if err != nil {
					return "", fmt.Errorf("could not open file in zip: %w", err)
				}
				binPath = filepath.Join(tmpDir, binaryName)
				out, err := os.Create(binPath)
				if err != nil {
					rc.Close() //nolint:errcheck,gosec
					return "", fmt.Errorf("could not create binary file: %w", err)
				}
				if _, err := io.Copy(out, rc); err != nil { // #nosec G110 -- archive is our own signed GitHub release
					_ = out.Close()
					_ = rc.Close()
					return "", fmt.Errorf("could not extract binary: %w", err)
				}
				if err := out.Close(); err != nil {
					_ = rc.Close()
					return "", fmt.Errorf("could not finalize extracted binary: %w", err)
				}
				if err := rc.Close(); err != nil {
					return "", fmt.Errorf("could not close zip entry: %w", err)
				}
				if err := os.Chmod(binPath, 0o755); err != nil { // #nosec G302 -- downloaded binary must be executable
					return "", fmt.Errorf("could not make binary executable: %w", err)
				}
				break
			}
		}
	} else {
		binPath = assetPath
		if err := os.Chmod(binPath, 0o755); err != nil { // #nosec G302 -- downloaded binary must be executable
			loglevel.Infof("warning: could not chmod downloaded binary: %v", err)
		}
	}

	if binPath == "" {
		return "", fmt.Errorf("could not locate matcha binary inside the release artifact")
	}

	return binPath, nil
}

func replaceExecutable(binPath, execDir string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}

	tmpNew := filepath.Join(execDir, fmt.Sprintf("matcha.new.%d", time.Now().Unix()))
	in, err := os.Open(binPath)
	if err != nil {
		return fmt.Errorf("could not open new binary: %w", err)
	}
	defer in.Close()                                                           //nolint:errcheck
	out, err := os.OpenFile(tmpNew, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755) // #nosec G302 -- replacement binary must be executable
	if err != nil {
		return fmt.Errorf("could not create temp binary in target dir: %w", err)
	}

	defer func() {
		cerr := out.Close()
		if err == nil && cerr != nil {
			err = fmt.Errorf("could not flush new binary to disk: %w", cerr)
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("could not write new binary to disk: %w", err)
	}

	if runtime.GOOS == goosWindows {
		oldPath := execPath + ".old"
		_ = os.Remove(oldPath)
		if err := os.Rename(execPath, oldPath); err != nil {
			return fmt.Errorf("could not move old executable out of the way: %w", err)
		}
	}

	if err = os.Rename(tmpNew, execPath); err != nil {
		return fmt.Errorf("could not replace executable: %w", err)
	}

	return nil
}

func runUpdateCLIManual(latestTag string, rel githubRelease) error {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}
	execDir := filepath.Dir(execPath)

	testFile := filepath.Join(execDir, ".matcha_update_test")
	if _, err := os.Create(testFile); err != nil {
		if os.Geteuid() != 0 {
			loglevel.Infof("\n⚠️  Permission denied: Cannot write to installation directory.")
			loglevel.Infof("   Try running with sudo: sudo matcha update")
			loglevel.Infof("   Or reinstall using your package manager.")
			return fmt.Errorf("permission denied: cannot write to %s", execDir)
		}
		return fmt.Errorf("cannot write to installation directory %s: %w", execDir, err)
	}
	_ = os.Remove(testFile)

	var assetURL, assetName string
	for _, a := range rel.Assets {
		n := strings.ToLower(a.Name)
		if strings.Contains(n, osName) && strings.Contains(n, arch) && (strings.HasSuffix(n, ".tar.gz") || strings.HasSuffix(n, ".tgz") || strings.HasSuffix(n, ".zip")) {
			assetURL = a.BrowserDownloadURL
			assetName = a.Name
			break
		}
	}
	if assetURL == "" {
		for _, a := range rel.Assets {
			n := strings.ToLower(a.Name)
			if strings.Contains(n, "matcha") && (strings.Contains(n, osName) || strings.Contains(n, arch)) {
				assetURL = a.BrowserDownloadURL
				assetName = a.Name
				break
			}
		}
	}

	if assetURL == "" {
		return fmt.Errorf("no suitable release artifact found for %s/%s", osName, arch)
	}

	loglevel.Infof("Found release asset: %s", assetName)
	loglevel.Infof("Downloading...")

	respAsset, err := httpClient.Get(assetURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer respAsset.Body.Close() //nolint:errcheck

	tmpDir, err := os.MkdirTemp("", "matcha-update-*")
	if err != nil {
		return fmt.Errorf("could not create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck

	assetPath := filepath.Join(tmpDir, assetName)
	outFile, err := os.Create(assetPath)
	if err != nil {
		return fmt.Errorf("could not create temp file: %w", err)
	}
	_, err = io.Copy(outFile, respAsset.Body)
	if err != nil {
		_ = outFile.Close()
		return fmt.Errorf("could not write asset to disk: %w", err)
	}
	if err := outFile.Close(); err != nil {
		return fmt.Errorf("could not finalize asset file: %w", err)
	}

	binPath, err := extractBinaryFromArchive(assetPath, assetName, tmpDir)
	if err != nil {
		return err
	}

	if err := replaceExecutable(binPath, execDir); err != nil {
		return err
	}

	loglevel.Infof("Successfully updated matcha to %s", latestTag)
	return nil
}
