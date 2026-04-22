package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// GithubRelease represents a GitHub API release response.
type GithubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// HTTPClient is used for outbound HTTP requests (update checks, asset downloads).
var HTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("stopped after 5 redirects")
		}
		return nil
	},
}

const githubAPI = "https://api.github.com/repos/floatpane/matcha/releases/latest"

// DetectInstalledVersion returns the installed version of matcha.
// It checks build-time version first, then Homebrew, WinGet, Snap, Flatpak.
func DetectInstalledVersion(buildVersion string) string {
	v := strings.TrimSpace(buildVersion)
	if v != "dev" && v != "" {
		return v
	}

	// Try Homebrew (macOS)
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("brew"); err == nil {
			if out, err := exec.Command("brew", "list", "--versions", "matcha").Output(); err == nil {
				parts := strings.Fields(string(out))
				if len(parts) >= 2 {
					return parts[1]
				}
			}
		}
	}

	// Try WinGet (Windows)
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("winget"); err == nil {
			if out, err := exec.Command("winget", "list", "--id", "floatpane.matcha", "--disable-interactivity").Output(); err == nil {
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
	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath("snap"); err == nil {
			if out, err := exec.Command("snap", "list", "matcha").Output(); err == nil {
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
			if out, err := exec.Command("flatpak", "info", "com.floatpane.matcha").Output(); err == nil {
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

// CheckForUpdates queries GitHub for the latest release and returns
// the latest and current versions. Returns empty strings if check fails.
func CheckForUpdates(buildVersion string) (latest, current string) {
	resp, err := HTTPClient.Get(githubAPI)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	var rel GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", ""
	}

	latest = strings.TrimPrefix(rel.TagName, "v")
	current = strings.TrimPrefix(DetectInstalledVersion(buildVersion), "v")
	return latest, current
}

// RunUpdate implements the CLI entrypoint for `matcha update`.
// It detects the installation method and attempts the appropriate update path.
func RunUpdate(buildVersion string) error {
	resp, err := HTTPClient.Get(githubAPI)
	if err != nil {
		return fmt.Errorf("could not query releases: %w", err)
	}
	defer resp.Body.Close()

	var rel GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return fmt.Errorf("could not parse release info: %w", err)
	}

	latestTag := rel.TagName
	if strings.HasPrefix(latestTag, "v") {
		latestTag = latestTag[1:]
	}

	fmt.Printf("Current version: %s\n", buildVersion)
	fmt.Printf("Latest version: %s\n", latestTag)

	cur := buildVersion
	if strings.HasPrefix(cur, "v") {
		cur = cur[1:]
	}
	if latestTag == "" || cur == latestTag {
		fmt.Println("Already up to date.")
		return nil
	}

	// Detect Homebrew
	if _, err := exec.LookPath("brew"); err == nil {
		fmt.Println("Detected Homebrew — updating taps and attempting to upgrade via brew.")

		updateCmd := exec.Command("brew", "update")
		updateCmd.Stdout = os.Stdout
		updateCmd.Stderr = os.Stderr
		if err := updateCmd.Run(); err != nil {
			fmt.Printf("Homebrew update failed: %v\n", err)
		}

		upgradeCmd := exec.Command("brew", "upgrade", "floatpane/matcha/matcha")
		upgradeCmd.Stdout = os.Stdout
		upgradeCmd.Stderr = os.Stderr
		if err := upgradeCmd.Run(); err == nil {
			fmt.Println("Successfully upgraded via Homebrew.")
			return nil
		}
		fmt.Printf("Homebrew upgrade failed: %v\n", err)
	}

	// Detect snap
	if _, err := exec.LookPath("snap"); err == nil {
		cmdCheck := exec.Command("snap", "list", "matcha")
		if err := cmdCheck.Run(); err == nil {
			fmt.Println("Detected Snap package — attempting to refresh.")
			cmd := exec.Command("snap", "refresh", "matcha")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				fmt.Println("Successfully refreshed snap.")
				return nil
			}
			fmt.Printf("Snap refresh failed: %v\n", err)
		}
	}

	// Detect flatpak
	if _, err := exec.LookPath("flatpak"); err == nil {
		cmdCheck := exec.Command("flatpak", "info", "com.floatpane.matcha")
		if err := cmdCheck.Run(); err == nil {
			fmt.Println("Detected Flatpak package — attempting to update.")
			cmd := exec.Command("flatpak", "update", "-y", "com.floatpane.matcha")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				fmt.Println("Successfully updated flatpak.")
				return nil
			}
			fmt.Printf("Flatpak update failed: %v\n", err)
		}
	}

	// Detect WinGet
	if _, err := exec.LookPath("winget"); err == nil {
		cmdCheck := exec.Command("winget", "list", "--id", "floatpane.matcha", "--disable-interactivity")
		if err := cmdCheck.Run(); err == nil {
			fmt.Println("Detected WinGet package — attempting to upgrade.")
			cmd := exec.Command("winget", "upgrade", "--id", "floatpane.matcha", "--disable-interactivity")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				fmt.Println("Successfully upgraded via WinGet.")
				return nil
			}
			fmt.Printf("WinGet upgrade failed: %v\n", err)
		}
	}

	// Otherwise attempt to download the proper release asset and replace the binary.
	return downloadAndReplace(rel, latestTag)
}

func downloadAndReplace(rel GithubRelease, latestTag string) error {
	osName := runtime.GOOS
	arch := runtime.GOARCH

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

	fmt.Printf("Found release asset: %s\n", assetName)
	fmt.Println("Downloading...")

	respAsset, err := HTTPClient.Get(assetURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer respAsset.Body.Close()

	tmpDir, err := os.MkdirTemp("", "matcha-update-*")
	if err != nil {
		return fmt.Errorf("could not create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	assetPath := filepath.Join(tmpDir, assetName)
	outFile, err := os.Create(assetPath)
	if err != nil {
		return fmt.Errorf("could not create temp file: %w", err)
	}
	_, err = io.Copy(outFile, respAsset.Body)
	outFile.Close()
	if err != nil {
		return fmt.Errorf("could not write asset to disk: %w", err)
	}

	binaryName := "matcha"
	if runtime.GOOS == "windows" {
		binaryName = "matcha.exe"
	}

	binPath, err := extractBinary(assetPath, assetName, tmpDir, binaryName)
	if err != nil {
		return err
	}

	if binPath == "" {
		return fmt.Errorf("could not locate matcha binary inside the release artifact")
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}

	execDir := filepath.Dir(execPath)
	tmpNew := filepath.Join(execDir, fmt.Sprintf("matcha.new.%d", time.Now().Unix()))
	in, err := os.Open(binPath)
	if err != nil {
		return fmt.Errorf("could not open new binary: %w", err)
	}
	out, err := os.OpenFile(tmpNew, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		in.Close()
		return fmt.Errorf("could not create temp binary in target dir: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		in.Close()
		out.Close()
		return fmt.Errorf("could not write new binary to disk: %w", err)
	}
	in.Close()
	out.Close()

	if runtime.GOOS == "windows" {
		oldPath := execPath + ".old"
		_ = os.Remove(oldPath)
		if err := os.Rename(execPath, oldPath); err != nil {
			return fmt.Errorf("could not move old executable out of the way: %w", err)
		}
	}

	if err := os.Rename(tmpNew, execPath); err != nil {
		return fmt.Errorf("could not replace executable: %w", err)
	}

	fmt.Println("Successfully updated matcha to", latestTag)
	return nil
}

func extractBinary(assetPath, assetName, tmpDir, binaryName string) (string, error) {
	if strings.HasSuffix(assetName, ".tar.gz") || strings.HasSuffix(assetName, ".tgz") {
		return extractFromTarGz(assetPath, tmpDir, binaryName)
	}
	if strings.HasSuffix(assetName, ".zip") {
		return extractFromZip(assetPath, tmpDir, binaryName)
	}
	// Non-archive asset: assume it is the binary itself.
	if err := os.Chmod(assetPath, 0755); err != nil {
		fmt.Printf("warning: could not chmod downloaded binary: %v\n", err)
	}
	return assetPath, nil
}

func extractFromTarGz(assetPath, tmpDir, binaryName string) (string, error) {
	f, err := os.Open(assetPath)
	if err != nil {
		return "", fmt.Errorf("could not open archive: %w", err)
	}
	defer f.Close()
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
			binPath := filepath.Join(tmpDir, binaryName)
			out, err := os.Create(binPath)
			if err != nil {
				return "", fmt.Errorf("could not create binary file: %w", err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return "", fmt.Errorf("could not extract binary: %w", err)
			}
			out.Close()
			if err := os.Chmod(binPath, 0755); err != nil {
				return "", fmt.Errorf("could not make binary executable: %w", err)
			}
			return binPath, nil
		}
	}
	return "", nil
}

func extractFromZip(assetPath, tmpDir, binaryName string) (string, error) {
	zr, err := zip.OpenReader(assetPath)
	if err != nil {
		return "", fmt.Errorf("could not open zip archive: %w", err)
	}
	defer zr.Close()
	for _, zf := range zr.File {
		name := filepath.Base(zf.Name)
		if name == binaryName || strings.Contains(strings.ToLower(name), "matcha") && !zf.FileInfo().IsDir() {
			rc, err := zf.Open()
			if err != nil {
				return "", fmt.Errorf("could not open file in zip: %w", err)
			}
			binPath := filepath.Join(tmpDir, binaryName)
			out, err := os.Create(binPath)
			if err != nil {
				rc.Close()
				return "", fmt.Errorf("could not create binary file: %w", err)
			}
			if _, err := io.Copy(out, rc); err != nil {
				out.Close()
				rc.Close()
				return "", fmt.Errorf("could not extract binary: %w", err)
			}
			out.Close()
			rc.Close()
			if err := os.Chmod(binPath, 0755); err != nil {
				return "", fmt.Errorf("could not make binary executable: %w", err)
			}
			return binPath, nil
		}
	}
	return "", nil
}
