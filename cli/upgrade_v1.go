package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/floatpane/matcha/internal/httpclient"
)

var v1RCRegex = regexp.MustCompile(`^v?1\.0\.0-rc\d+$`)

// RunUpgradeV1 handles `matcha upgrade-v1` and upgrades a pre-v1 install (or
// release candidate) to the v1 release.
func RunUpgradeV1(args []string) error {
	_ = args

	fmt.Println("Checking for v1 release...")

	rel, tag, err := fetchV1Release()
	if err != nil {
		return err
	}

	fmt.Printf("Target version: v%s\n", tag)

	switch runtime.GOOS {
	case goosDarwin:
		if tryHomebrewV1Upgrade(true) {
			return nil
		}
	case goosLinux:
		if trySnapV1Refresh() {
			return nil
		}
		if tryHomebrewV1Upgrade(false) {
			return nil
		}
	}

	// Windows or fallbacks: download the binary directly.
	osName := runtime.GOOS
	arch := runtime.GOARCH
	assetName, assetURL, err := FindAsset(rel, osName, arch)
	if err != nil {
		return err
	}
	return UpgradeBinaryFromAsset(assetURL, assetName, "v"+tag, "matcha upgrade-v1")
}

func fetchV1Release() (*Release, string, error) {
	client := httpclient.NewWithRedirectCap(httpclient.UpdateCheckTimeout, 5)

	const apiTag = "https://api.github.com/repos/floatpane/matcha/releases/tags/v1.0.0"
	resp, err := client.Get(apiTag) //nolint:noctx
	if err == nil {
		if resp.StatusCode == http.StatusOK {
			defer resp.Body.Close() //nolint:errcheck
			var rel Release
			if err := json.NewDecoder(resp.Body).Decode(&rel); err == nil && !rel.Prerelease {
				tag := strings.TrimPrefix(rel.TagName, "v")
				return &rel, tag, nil
			}
		} else {
			if err := resp.Body.Close(); err != nil {
				fmt.Printf("warning: non-fatal response body close error: %v\n", err)
			}
		}
	}

	const apiList = "https://api.github.com/repos/floatpane/matcha/releases"
	resp, err = client.Get(apiList) //nolint:noctx
	if err != nil {
		return nil, "", fmt.Errorf("could not query releases: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var rels []Release
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return nil, "", fmt.Errorf("could not parse releases: %w", err)
	}

	for i := range rels {
		tag := strings.TrimPrefix(rels[i].TagName, "v")
		if strings.HasPrefix(tag, "1.0.") && !strings.Contains(tag, "-") && !rels[i].Prerelease {
			return &rels[i], tag, nil
		}
	}
	for i := range rels {
		tag := strings.TrimPrefix(rels[i].TagName, "v")
		if v1RCRegex.MatchString(tag) {
			return &rels[i], tag, nil
		}
	}
	return nil, "", fmt.Errorf("no v1 release found")
}

func tryHomebrewV1Upgrade(cask bool) bool {
	if _, err := exec.LookPath("brew"); err != nil {
		return false
	}

	formula := "floatpane/matcha/matcha@v1"
	var installArgs, upgradeArgs []string
	if cask {
		installArgs = []string{"install", "--cask", formula}
		upgradeArgs = []string{"upgrade", "--cask", formula}
		fmt.Println("Attempting to upgrade via Homebrew cask to v1.")
	} else {
		installArgs = []string{"install", formula}
		upgradeArgs = []string{"upgrade", formula}
		fmt.Println("Attempting to upgrade via Homebrew to v1.")
	}

	cmd := exec.Command("brew", installArgs...) //nolint:noctx
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err == nil {
		fmt.Println("Successfully upgraded via Homebrew.")
		return true
	}

	cmd = exec.Command("brew", upgradeArgs...) //nolint:noctx
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err == nil {
		fmt.Println("Successfully upgraded via Homebrew.")
		return true
	}

	fmt.Println("Homebrew v1 upgrade failed.")
	return false
}

func trySnapV1Refresh() bool {
	if _, err := exec.LookPath("snap"); err != nil {
		return false
	}

	cmdCheck := exec.Command("snap", "list", "matcha") //nolint:noctx
	if err := cmdCheck.Run(); err != nil {
		return false
	}

	fmt.Println("Detected Snap package — attempting to refresh to candidate v1.")
	cmd := exec.Command("snap", "refresh", "matcha", "--candidate") //nolint:noctx
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err == nil {
		fmt.Println("Successfully refreshed snap to candidate v1.")
		return true
	}

	fmt.Println("Snap candidate refresh failed.")
	return false
}
