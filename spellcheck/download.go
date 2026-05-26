package spellcheck

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/floatpane/matcha/internal/httpclient"
)

// DefaultLanguage is the language code installed automatically the first
// time the composer opens.
const DefaultLanguage = "en"

// DictURLTemplate is the URL used to fetch Hunspell .dic files. It is a
// variable to allow tests and the CLI to override the source.
var DictURLTemplate = "https://raw.githubusercontent.com/wooorm/dictionaries/main/dictionaries/%s/index.dic"

// Download fetches the dictionary for lang from DictURLTemplate and writes
// it atomically to the dicts directory.
func Download(lang string) (string, error) {
	if lang == "" {
		return "", fmt.Errorf("empty language code")
	}
	dest, err := DictPath(lang)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf(DictURLTemplate, urlPathLang(lang))
	client := httpclient.New(httpclient.InstallTimeout)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", lang, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: status %d", lang, resp.StatusCode)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".dl-*")
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) //nolint:errcheck

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close() //nolint:errcheck,gosec
		return "", fmt.Errorf("write dict: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close dict: %w", err)
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return "", fmt.Errorf("install dict: %w", err)
	}
	return dest, nil
}

// EnsureDefault downloads the default English dictionary if it is not
// already installed and returns the language code that is available.
func EnsureDefault() (string, error) {
	if DictInstalled(DefaultLanguage) {
		return DefaultLanguage, nil
	}
	_, err := Download(DefaultLanguage)
	if err != nil {
		return "", err
	}
	return DefaultLanguage, nil
}

// urlPathLang converts a language code into the directory name used by the
// wooorm/dictionaries repository ("en", "en-GB", "de", ...). The code is
// passed through after normalising the region separator.
func urlPathLang(lang string) string {
	lang = strings.TrimSpace(lang)
	lang = strings.ReplaceAll(lang, "_", "-")
	return lang
}
