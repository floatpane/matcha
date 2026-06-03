package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/floatpane/go-secretbox"
)

const secureMetaFile = "secure.meta"

// legacyMeta is the secure.meta format written before go-secretbox was
// integrated. It is read-only; the first successful unlock auto-migrates it.
type legacyMeta struct {
	Salt          string `json:"salt"`
	Sentinel      string `json:"sentinel"`
	Argon2Time    uint32 `json:"argon2_time"`
	Argon2Memory  uint32 `json:"argon2_memory"`
	Argon2Threads uint8  `json:"argon2_threads"`
}

// legacySentinel is the plaintext the old code encrypted as the verification token.
const legacySentinel = "matcha-verified"

var (
	vaultMu     sync.Mutex
	cachedVault *secretbox.Vault
)

// getVault returns the shared Vault instance, creating it on first call.
func getVault() *secretbox.Vault {
	vaultMu.Lock()
	defer vaultMu.Unlock()
	if cachedVault == nil {
		path, _ := secureMetaPath()
		cachedVault = secretbox.NewVault(path)
	}
	return cachedVault
}

// secureMetaPath returns the path to the secure.meta file.
func secureMetaPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, secureMetaFile), nil
}

// IsSecureModeEnabled checks whether encryption is active by looking for secure.meta.
func IsSecureModeEnabled() bool {
	return getVault().Initialized()
}

// Encrypt encrypts plaintext using AES-256-GCM. The nonce is prepended to the ciphertext.
func Encrypt(plaintext, key []byte) ([]byte, error) {
	return secretbox.AESGCM{}.Encrypt(plaintext, key)
}

// Decrypt decrypts ciphertext produced by Encrypt using AES-256-GCM.
func Decrypt(ciphertext, key []byte) ([]byte, error) {
	plain, err := secretbox.AESGCM{}.Decrypt(ciphertext, key)
	if errors.Is(err, secretbox.ErrDecrypt) {
		return nil, fmt.Errorf("decryption: %w", err)
	}
	return plain, err
}

// DeriveKey derives an AES-256 key from a password and salt using Argon2id.
func DeriveKey(password string, salt []byte) []byte {
	return secretbox.NewArgon2id(secretbox.DefaultArgon2id).DeriveKey(password, salt, 32)
}

// SetSessionKey is kept for API compatibility. VerifyPassword unlocks the vault
// internally, so callers that pass the returned key back through SetSessionKey
// are no-ops — the vault is already in the correct state.
func SetSessionKey(_ []byte) {}

// GetSessionKey returns the current session key, or nil if the vault is locked.
// The caller must not modify the returned slice.
func GetSessionKey() []byte {
	return getVault().Key()
}

// ClearSessionKey removes the session key from memory.
func ClearSessionKey() {
	getVault().Lock()
}

// VerifyPassword checks the password against the stored sentinel and unlocks
// the vault. Returns the derived key for callers that store it via SetSessionKey.
func VerifyPassword(password string) ([]byte, error) {
	v := getVault()

	if err := migrateLegacyMeta(password); err != nil {
		return nil, err
	}

	// migrateLegacyMeta may have left the vault unlocked via Init; skip Unlock
	// in that case to avoid a redundant (and slow) Argon2id call.
	if v.Locked() {
		if err := v.Unlock(password); err != nil {
			if errors.Is(err, secretbox.ErrWrongPassword) {
				return nil, errors.New("incorrect password")
			}
			return nil, fmt.Errorf("could not read secure metadata: %w", err)
		}
	}
	return v.Key(), nil
}

// EnableSecureMode sets up encryption with the given password. It initialises
// the vault, re-saves the config with passwords embedded in the encrypted JSON,
// and re-encrypts all existing data files.
func EnableSecureMode(password string, cfg *Config) error {
	v := getVault()
	if err := v.Init(password); err != nil {
		return fmt.Errorf("could not initialise secure vault: %w", err)
	}

	rollback := func() {
		v.Lock()
		path, _ := secureMetaPath()
		_ = os.Remove(path)
	}

	if cfg != nil {
		if err := SaveConfig(cfg); err != nil {
			rollback()
			return fmt.Errorf("failed to save encrypted config: %w", err)
		}
	}
	if err := reEncryptCacheFiles(); err != nil {
		rollback()
		return fmt.Errorf("failed to encrypt existing files: %w", err)
	}
	return nil
}

// DisableSecureMode decrypts all files back to plain JSON and removes secure.meta.
// The config must be passed so passwords can be restored to the OS keyring.
func DisableSecureMode(cfg *Config) error {
	v := getVault()

	files, err := collectDataFiles()
	if err != nil {
		return err
	}
	cfgPath, _ := configFile()

	for _, f := range files {
		if f == cfgPath {
			continue
		}
		enc, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		plain, err := v.Decrypt(enc)
		if err != nil {
			continue // file may not be encrypted
		}
		if err := writeDataFile(f, plain, 0o600); err != nil {
			return err
		}
	}

	// Lock before SaveConfig so it writes plain JSON and restores keyring passwords.
	v.Lock()

	if cfg != nil {
		if err := SaveConfig(cfg); err != nil {
			return fmt.Errorf("failed to save plain config: %w", err)
		}
	}

	path, _ := secureMetaPath()
	_ = os.Remove(path)
	return nil
}

// SecureReadFile reads a file, decrypting it if the vault is unlocked.
func SecureReadFile(path string) ([]byte, error) {
	v := getVault()
	if v.Locked() {
		return os.ReadFile(path)
	}
	return v.ReadFile(path)
}

// SecureWriteFile writes data to a file, encrypting it if the vault is unlocked.
func SecureWriteFile(path string, data []byte, perm os.FileMode) error {
	v := getVault()
	if v.Locked() {
		return os.WriteFile(path, data, perm) //nolint:gosec
	}
	return v.WriteFile(path, data, perm)
}

// reEncryptCacheFiles reads all plain data files (excluding config.json) and
// writes them encrypted via the unlocked vault.
func reEncryptCacheFiles() error {
	files, err := collectDataFiles()
	if err != nil {
		return err
	}
	cfgPath, _ := configFile()
	for _, f := range files {
		if f == cfgPath {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if err := secureWriteDataFile(f, data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// collectDataFiles returns paths to all data files that should be encrypted/decrypted.
func collectDataFiles() ([]string, error) {
	var files []string

	cfgDir, err := configDir()
	if err != nil {
		return nil, err
	}
	files = append(files, filepath.Join(cfgDir, "config.json"))

	cDir, err := cacheDir()
	if err != nil {
		return nil, err
	}

	for _, f := range cacheFiles {
		files = append(files, filepath.Join(cDir, f))
	}

	for _, f := range cacheDirectories {
		dir := filepath.Join(cDir, f)
		if entries, err := os.ReadDir(dir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					// filepath.Base strips any directory components from the
					// entry name, preventing traversal via crafted filenames.
					files = append(files, filepath.Join(dir, filepath.Base(entry.Name())))
				}
			}
		}
	}

	sigDir := filepath.Join(cfgDir, "signatures")
	if entries, err := os.ReadDir(sigDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				files = append(files, filepath.Join(sigDir, filepath.Base(entry.Name())))
			}
		}
	}

	return files, nil
}

// migrateLegacyMeta detects a pre-go-secretbox secure.meta and converts it.
// It verifies the password via the old sentinel, decrypts all existing files
// with the old key, rewrites metadata in the new format, and re-encrypts.
// A wrong password during migration returns errors.New("incorrect password").
// If the file is absent or already in the new format, it is a no-op.
func migrateLegacyMeta(password string) error {
	path, err := secureMetaPath()
	if err != nil || path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	// Old format has "argon2_time"; new format does not.
	var probe struct {
		ArgonTime uint32 `json:"argon2_time"`
	}
	if json.Unmarshal(data, &probe) != nil || probe.ArgonTime == 0 {
		return nil // already new format
	}

	var old legacyMeta
	if err := json.Unmarshal(data, &old); err != nil {
		return fmt.Errorf("corrupt legacy metadata: %w", err)
	}
	salt, err := base64.StdEncoding.DecodeString(old.Salt)
	if err != nil {
		return fmt.Errorf("corrupt legacy salt: %w", err)
	}

	// Derive old key and verify password via the legacy sentinel.
	oldKDF := secretbox.NewArgon2id(secretbox.Argon2idParams{
		Time:    old.Argon2Time,
		Memory:  old.Argon2Memory,
		Threads: old.Argon2Threads,
	})
	oldKey := oldKDF.DeriveKey(password, salt, 32)
	defer zeroSlice(oldKey)

	sentinelCipher, err := base64.StdEncoding.DecodeString(old.Sentinel)
	if err != nil {
		return fmt.Errorf("corrupt legacy sentinel: %w", err)
	}
	plain, err := secretbox.AESGCM{}.Decrypt(sentinelCipher, oldKey)
	if err != nil || string(plain) != legacySentinel {
		return errors.New("incorrect password")
	}

	// Decrypt every data file with the old key before we touch the vault.
	type decryptedFile struct {
		path string
		perm os.FileMode
		data []byte
	}
	files, _ := collectDataFiles()
	var pending []decryptedFile
	for _, f := range files {
		enc, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		pt, err := secretbox.AESGCM{}.Decrypt(enc, oldKey)
		if err != nil {
			continue // not encrypted or not with this key
		}
		perm := os.FileMode(0o600)
		if info, err := os.Stat(f); err == nil {
			perm = info.Mode().Perm()
		}
		pending = append(pending, decryptedFile{f, perm, pt})
	}

	// Remove old meta and initialise the new-format vault.
	_ = os.Remove(path)
	v := getVault()
	if err := v.Init(password); err != nil {
		return fmt.Errorf("migration: vault init: %w", err)
	}

	// Re-encrypt with the new vault key.
	for _, p := range pending {
		_ = v.WriteFile(p.path, p.data, p.perm)
	}
	return nil
}

// secureWriteDataFile validates path is within app directories then delegates
// to SecureWriteFile (which encrypts when the vault is unlocked).
func secureWriteDataFile(path string, data []byte, perm os.FileMode) error {
	cfgDir, err := configDir()
	if err != nil {
		return err
	}
	cDir, err := cacheDir()
	if err != nil {
		return err
	}
	clean := filepath.Clean(path)
	if !isUnder(clean, cfgDir) && !isUnder(clean, cDir) {
		return fmt.Errorf("config: refusing write outside app directories: %s", clean)
	}
	return SecureWriteFile(clean, data, perm)
}

// writeDataFile writes data to path only if path is within the application's
// config or cache directories. It cleans the path first. This breaks the taint
// flow for any directory-entry–derived paths returned by collectDataFiles.
func writeDataFile(path string, data []byte, perm os.FileMode) error {
	cfgDir, err := configDir()
	if err != nil {
		return err
	}
	cDir, err := cacheDir()
	if err != nil {
		return err
	}
	clean := filepath.Clean(path)
	if !isUnder(clean, cfgDir) && !isUnder(clean, cDir) {
		return fmt.Errorf("config: refusing write outside app directories: %s", clean)
	}
	return os.WriteFile(clean, data, perm) //nolint:gosec
}

// isUnder reports whether path is inside (or equal to) base.
// Uses filepath.Rel so it is OS-path-separator aware.
func isUnder(path, base string) bool {
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func zeroSlice(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
