package pgp

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/floatpane/matcha/internal/loglevel"
)

// gpgAgentDecryptMIME decrypts a multipart/encrypted MIME message (RFC 3156)
// by piping the OpenPGP ciphertext to the local gpg binary, which delegates
// to gpg-agent. Supports all key types including ECDH/Curve25519 YubiKey slots.
//
// pin is the YubiKey User PIN. It is pre-cached in gpg-agent via
// PRESET_PASSPHRASE so the agent can authorize the card without launching a
// graphical pinentry (which would hang in a TUI context).
func gpgAgentDecryptMIME(payload []byte, pin string) ([]byte, error) {
	armored, err := extractArmoredFromMIME(payload)
	if err != nil {
		return nil, fmt.Errorf("gpg-agent: %w", err)
	}

	// Pre-cache the PIN in gpg-agent under the decryption subkey's grip.
	// Without this, gpg-agent would try to launch a pinentry GUI for the
	// smartcard PIN, which blocks forever in a terminal context.
	presetSmartcardPIN(pin)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gpg",
		"--decrypt",
		"--batch",
		"--yes",
		"--quiet",
		"--no-tty",
	)
	cmd.Stdin = bytes.NewReader(armored)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	loglevel.Debugf("pgp gpg-agent: running gpg --decrypt")
	runErr := cmd.Run()
	loglevel.Debugf("pgp gpg-agent: gpg exited err=%v stderr=%q", runErr, strings.TrimSpace(stderr.String()))
	if runErr != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("gpg-agent decrypt: timed out after 90s — touch your YubiKey if the LED is flashing")
		}
		return nil, fmt.Errorf("gpg-agent decrypt: %w: %s", runErr, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// presetSmartcardPIN pre-caches the smartcard PIN in gpg-agent via the
// PRESET_PASSPHRASE assuan command. The keygrip of the encryption subkey is
// looked up from `gpg --card-status`. gpg-agent will use the cached PIN when
// scdaemon requests it, bypassing any pinentry dialog.
func presetSmartcardPIN(pin string) {
	if pin == "" {
		return
	}
	// PRESET_PASSPHRASE is silently ignored unless gpg-agent.conf opts in with
	// allow-preset-passphrase; without it gpg-agent falls back to pinentry,
	// which clashes with the TUI. Make sure the option is enabled first.
	ensureAllowPresetPassphrase()
	grip, err := decryptKeygrip()
	if err != nil {
		loglevel.Debugf("pgp: preset PIN keygrip lookup: %v", err)
		return
	}
	hexPIN := hex.EncodeToString([]byte(pin))
	// -1 means the cached passphrase does not expire.
	asuCmd := fmt.Sprintf("PRESET_PASSPHRASE %s -1 %s", grip, hexPIN)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "gpg-connect-agent", asuCmd, "/bye").CombinedOutput(); err != nil {
		loglevel.Debugf("pgp: preset PIN failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
}

// ensureAllowPresetPassphrase guarantees gpg-agent.conf contains
// allow-preset-passphrase, the option that lets PRESET_PASSPHRASE (and thus
// presetSmartcardPIN) suppress the pinentry dialog. If the line is added, the
// running agent is reloaded so it takes effect immediately. Best-effort: every
// error is logged and ignored so decryption still proceeds (with pinentry).
func ensureAllowPresetPassphrase() {
	confPath := gpgAgentConfPath()
	data, err := os.ReadFile(confPath)
	if err != nil && !os.IsNotExist(err) {
		loglevel.Debugf("pgp: read gpg-agent.conf: %v", err)
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "allow-preset-passphrase") {
			return // already enabled
		}
	}

	if err := os.MkdirAll(filepath.Dir(confPath), 0o700); err != nil {
		loglevel.Debugf("pgp: create gnupg home: %v", err)
		return
	}
	content := string(data)
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "allow-preset-passphrase\n"
	// confPath is derived from GNUPGHOME or the user's home dir, not from
	// untrusted input, so this is not an attacker-controlled path.
	if err := os.WriteFile(confPath, []byte(content), 0o600); err != nil {
		loglevel.Debugf("pgp: write gpg-agent.conf: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "gpg-connect-agent", "reloadagent", "/bye").CombinedOutput(); err != nil {
		loglevel.Debugf("pgp: reload gpg-agent: %v: %s", err, strings.TrimSpace(string(out)))
	}
}

// gpgAgentConfPath resolves the gpg-agent.conf location, honoring GNUPGHOME and
// falling back to ~/.gnupg.
func gpgAgentConfPath() string {
	if h := os.Getenv("GNUPGHOME"); h != "" {
		return filepath.Join(h, "gpg-agent.conf")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".gnupg", "gpg-agent.conf")
	}
	return filepath.Join(home, ".gnupg", "gpg-agent.conf")
}

// decryptKeygrip returns the keygrip of the encryption subkey on the connected
// OpenPGP card by parsing `gpg --with-keygrip --with-colons --card-status`.
func decryptKeygrip() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gpg", "--with-keygrip", "--with-colons", "--card-status").Output()
	if err != nil {
		// Fallback: list-secret-keys also shows card stubs with their grips.
		out, err = exec.CommandContext(ctx, "gpg", "--with-keygrip", "--with-colons", "--list-secret-keys").Output()
		if err != nil {
			return "", fmt.Errorf("gpg keygrip lookup: %w", err)
		}
	}

	// The colon-delimited output has "sub" or "ssb" lines for subkeys followed
	// immediately by "grp" lines containing the keygrip. Field index 11
	// (1-based: 12) of the sub/ssb line holds the key capabilities; "e" means
	// encryption.
	lines := strings.Split(string(out), "\n")
	isEncryptSub := false
	for _, line := range lines {
		fields := strings.Split(line, ":")
		if len(fields) < 2 {
			isEncryptSub = false
			continue
		}
		switch fields[0] {
		case "sub", "ssb":
			if len(fields) > 11 && strings.Contains(fields[11], "e") {
				isEncryptSub = true
			} else {
				isEncryptSub = false
			}
		case "grp":
			if isEncryptSub && len(fields) > 9 {
				if grip := strings.TrimSpace(fields[9]); grip != "" {
					return grip, nil
				}
			}
			isEncryptSub = false
		default:
			isEncryptSub = false
		}
	}
	return "", fmt.Errorf("no encryption subkey keygrip in card status output")
}

// extractArmoredFromMIME returns the ASCII-armored OpenPGP ciphertext from
// the data part of a multipart/encrypted MIME message. Unlike the cardhl
// library's internal helper, this preserves the armor rather than decoding it,
// so the bytes can be piped directly to gpg.
func extractArmoredFromMIME(payload []byte) ([]byte, error) {
	sep := bytes.Index(payload, []byte("\r\n\r\n"))
	bodyOffset := 4
	if sep < 0 {
		sep = bytes.Index(payload, []byte("\n\n"))
		bodyOffset = 2
	}
	if sep < 0 {
		return nil, fmt.Errorf("no header/body separator")
	}

	unfolded := strings.ReplaceAll(string(payload[:sep]), "\r\n\t", " ")
	unfolded = strings.ReplaceAll(unfolded, "\r\n ", " ")
	unfolded = strings.ReplaceAll(unfolded, "\n\t", " ")
	unfolded = strings.ReplaceAll(unfolded, "\n ", " ")

	var contentType string
	for _, line := range strings.Split(unfolded, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(strings.ToUpper(line), "CONTENT-TYPE:") {
			contentType = strings.TrimSpace(line[len("Content-Type:"):])
			break
		}
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "multipart/encrypted") {
		return nil, fmt.Errorf("expected multipart/encrypted, got %q", mediaType)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, fmt.Errorf("missing boundary parameter")
	}

	mr := multipart.NewReader(bytes.NewReader(payload[sep+bodyOffset:]), boundary)

	// Discard the RFC 3156 control part ("Version: 1").
	if _, err := mr.NextPart(); err != nil {
		return nil, fmt.Errorf("missing control part: %w", err)
	}

	dataPart, err := mr.NextPart()
	if err != nil {
		return nil, fmt.Errorf("missing data part: %w", err)
	}
	defer dataPart.Close() //nolint:errcheck

	return io.ReadAll(dataPart)
}
