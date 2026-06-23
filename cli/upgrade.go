package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/floatpane/matcha/internal/httpclient"
)

// Release describes a GitHub release and its assets.
type Release struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

const (
	goosDarwin  = "darwin"
	goosLinux   = "linux"
	goosWindows = "windows"
)

const maxBinarySize = 512 * 1024 * 1024 // 512 MiB

// copyLimited copies at most maxBinarySize bytes from src to dst. It is used to
// avoid decompression bomb attacks when extracting binaries from archives.
func copyLimited(dst io.Writer, src io.Reader) error {
	n, err := io.CopyN(dst, src, maxBinarySize+1)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if n > maxBinarySize {
		return fmt.Errorf("extracted binary exceeds maximum size of %d bytes", maxBinarySize)
	}
	return nil
}

// FindAsset returns the name and download URL for a release asset matching the
// given OS and architecture.
func FindAsset(rel *Release, osName, arch string) (string, string, error) {
	for _, a := range rel.Assets {
		n := strings.ToLower(a.Name)
		if strings.Contains(n, osName) && strings.Contains(n, arch) && (strings.HasSuffix(n, ".tar.gz") || strings.HasSuffix(n, ".tgz") || strings.HasSuffix(n, ".zip")) {
			return a.Name, a.BrowserDownloadURL, nil
		}
	}
	for _, a := range rel.Assets {
		n := strings.ToLower(a.Name)
		if strings.Contains(n, "matcha") && (strings.Contains(n, osName) || strings.Contains(n, arch)) {
			return a.Name, a.BrowserDownloadURL, nil
		}
	}
	return "", "", fmt.Errorf("no suitable release artifact found for %s/%s", osName, arch)
}

// UpgradeBinaryFromAsset downloads the named release asset, extracts the matcha
// binary, and replaces the running executable.
func UpgradeBinaryFromAsset(assetURL, assetName, tag, cmdName string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}
	execDir := filepath.Dir(execPath)

	if err := ensureWritable(execDir, cmdName); err != nil {
		return err
	}

	fmt.Printf("Found release asset: %s\n", assetName)
	fmt.Println("Downloading...")

	client := httpclient.NewWithRedirectCap(httpclient.UpdateCheckTimeout, 5)
	respAsset, err := client.Get(assetURL) //nolint:noctx
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer respAsset.Body.Close() //nolint:errcheck

	// Create a temp file for the download.
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

	// Extract binary from archive.
	binPath, err := extractBinaryFromArchive(assetPath, assetName, tmpDir)
	if err != nil {
		return err
	}

	// Replace the executable.
	if err := replaceExecutable(binPath, execDir); err != nil {
		return err
	}

	fmt.Println("Successfully updated matcha to", tag)
	return nil
}

func ensureWritable(execDir, cmdName string) error {
	testFile := filepath.Join(execDir, ".matcha_update_test")
	if _, err := os.Create(testFile); err != nil {
		if runtime.GOOS != goosWindows && os.Geteuid() != 0 {
			fmt.Println("\n⚠️  Permission denied: Cannot write to installation directory.")
			fmt.Printf("   Try running with sudo: sudo %s\n", cmdName)
			fmt.Println("   Or reinstall using your package manager.")
			return fmt.Errorf("permission denied: cannot write to %s", execDir)
		}
		return fmt.Errorf("cannot write to installation directory %s: %w", execDir, err)
	}
	_ = os.Remove(testFile)
	return nil
}

// extractBinaryFromArchive extracts the matcha binary from a tar.gz, tgz, or zip archive.
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
			if name == binaryName || (strings.Contains(strings.ToLower(name), "matcha") && (hdr.Typeflag == tar.TypeReg)) {
				binPath = filepath.Join(tmpDir, binaryName)
				out, err := os.Create(binPath)
				if err != nil {
					return "", fmt.Errorf("could not create binary file: %w", err)
				}
				if err := copyLimited(out, tr); err != nil {
					_ = out.Close()
					return "", fmt.Errorf("could not extract binary: %w", err)
				}
				if err := out.Close(); err != nil {
					return "", fmt.Errorf("could not finalize extracted binary: %w", err)
				}
				if err := os.Chmod(binPath, 0755); err != nil { // #nosec G302 -- binary must be executable
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
			if name == binaryName || (strings.Contains(strings.ToLower(name), "matcha") && !zf.FileInfo().IsDir()) {
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
				if err := copyLimited(out, rc); err != nil {
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
				if err := os.Chmod(binPath, 0755); err != nil { // #nosec G302 -- binary must be executable
					return "", fmt.Errorf("could not make binary executable: %w", err)
				}
				break
			}
		}
	} else {
		binPath = assetPath
		if err := os.Chmod(binPath, 0755); err != nil { // #nosec G302 -- binary must be executable
			fmt.Printf("warning: could not chmod downloaded binary: %v\n", err)
		}
	}

	if binPath == "" {
		return "", fmt.Errorf("could not locate matcha binary inside the release artifact")
	}

	return binPath, nil
}

// replaceExecutable atomically replaces the current executable with a new binary.
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
	defer in.Close()                                                          //nolint:errcheck
	out, err := os.OpenFile(tmpNew, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755) // #nosec G302 -- binary must be executable
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
