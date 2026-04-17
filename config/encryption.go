package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/argon2"
)

const (
	sentinelPlaintext = "matcha-verified"
	secureMetaFile    = "secure.meta"

	// Argon2id parameters
	argon2Time    = 3
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4
	argon2KeyLen  = 32 // AES-256
)

// secureMeta is stored as plain JSON at ~/.config/matcha/secure.meta.
// Its existence signals that secure mode is enabled.
type secureMeta struct {
	Salt          string `json:"salt"`
	Sentinel      string `json:"sentinel"`
	Argon2Time    uint32 `json:"argon2_time"`
	Argon2Memory  uint32 `json:"argon2_memory"`
	Argon2Threads uint8  `json:"argon2_threads"`
}

var (
	sessionKey   []byte
	sessionKeyMu sync.RWMutex
)

// SetSessionKey stores the derived encryption key in memory for this session.
func SetSessionKey(key []byte) {
	sessionKeyMu.Lock()
	defer sessionKeyMu.Unlock()
	sessionKey = key
}

// GetSessionKey returns the current session key, or nil if not set.
func GetSessionKey() []byte {
	sessionKeyMu.RLock()
	defer sessionKeyMu.RUnlock()
	return sessionKey
}

// ClearSessionKey removes the session key from memory.
func ClearSessionKey() {
	sessionKeyMu.Lock()
	defer sessionKeyMu.Unlock()
	// Overwrite key material before clearing
	for i := range sessionKey {
		sessionKey[i] = 0
	}
	sessionKey = nil
}

// DeriveKey derives an AES-256 key from a password and salt using Argon2id.
func DeriveKey(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
}

// deriveKeyWithParams derives a key using specific Argon2id parameters (for loading existing meta).
func deriveKeyWithParams(password string, salt []byte, time, memory uint32, threads uint8) []byte {
	return argon2.IDKey([]byte(password), salt, time, memory, threads, argon2KeyLen)
}

// Encrypt encrypts plaintext using AES-256-GCM. The nonce is prepended to the ciphertext.
func Encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("encryption: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("encryption: %w", err)
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("encryption: %w", err)
	}
	return aesGCM.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts ciphertext produced by Encrypt using AES-256-GCM.
func Decrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("decryption: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("decryption: %w", err)
	}
	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("decryption: ciphertext too short")
	}
	nonce, encrypted := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption: %w", err)
	}
	return plaintext, nil
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
	path, err := secureMetaPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// loadSecureMeta reads and parses the secure.meta file.
func loadSecureMeta() (*secureMeta, error) {
	path, err := secureMetaPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta secureMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// VerifyPassword checks the password against the stored sentinel.
// Returns the derived key on success.
func VerifyPassword(password string) ([]byte, error) {
	meta, err := loadSecureMeta()
	if err != nil {
		return nil, fmt.Errorf("could not read secure metadata: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(meta.Salt)
	if err != nil {
		return nil, fmt.Errorf("invalid salt: %w", err)
	}

	key := deriveKeyWithParams(password, salt, meta.Argon2Time, meta.Argon2Memory, meta.Argon2Threads)

	sentinelCiphertext, err := base64.StdEncoding.DecodeString(meta.Sentinel)
	if err != nil {
		return nil, fmt.Errorf("invalid sentinel: %w", err)
	}

	plaintext, err := Decrypt(sentinelCiphertext, key)
	if err != nil {
		return nil, errors.New("incorrect password")
	}

	if string(plaintext) != sentinelPlaintext {
		return nil, errors.New("incorrect password")
	}

	return key, nil
}

// EnableSecureMode sets up encryption with the given password.
// It generates a salt, derives a key, encrypts the sentinel, saves secure.meta,
// and re-encrypts all existing data files. The config must be passed so that
// passwords (normally stored in the OS keyring) can be written into the encrypted config.
func EnableSecureMode(password string, cfg *Config) error {
	// Generate random salt
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("could not generate salt: %w", err)
	}

	key := DeriveKey(password, salt)

	// Encrypt sentinel
	sentinelCipher, err := Encrypt([]byte(sentinelPlaintext), key)
	if err != nil {
		return fmt.Errorf("could not encrypt sentinel: %w", err)
	}

	meta := secureMeta{
		Salt:          base64.StdEncoding.EncodeToString(salt),
		Sentinel:      base64.StdEncoding.EncodeToString(sentinelCipher),
		Argon2Time:    argon2Time,
		Argon2Memory:  argon2Memory,
		Argon2Threads: argon2Threads,
	}

	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	path, err := secureMetaPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	// Set the session key so SecureWriteFile will encrypt
	SetSessionKey(key)

	// Re-save config first — this writes passwords into the encrypted JSON
	// (SaveConfig uses secureDiskConfig when session key is set)
	if cfg != nil {
		if err := SaveConfig(cfg); err != nil {
			ClearSessionKey()
			return fmt.Errorf("failed to save encrypted config: %w", err)
		}
	}

	// Re-encrypt all remaining data files (caches, signatures, etc.)
	if err := reEncryptCacheFiles(); err != nil {
		ClearSessionKey()
		return fmt.Errorf("failed to encrypt existing files: %w", err)
	}

	// Write secure.meta last (plain JSON, not encrypted)
	if err := os.WriteFile(path, metaData, 0600); err != nil {
		ClearSessionKey()
		return err
	}

	return nil
}

// DisableSecureMode decrypts all files back to plain JSON and removes secure.meta.
// The config must be passed so passwords can be restored to the OS keyring.
func DisableSecureMode(cfg *Config) error {
	// Collect all files that need decryption
	files, err := collectDataFiles()
	if err != nil {
		return err
	}

	// Find config.json path to skip it (handled separately below)
	cfgPath, _ := configFile()

	// Copy the key so ClearSessionKey's in-place zeroing doesn't destroy it.
	origKey := GetSessionKey()
	key := make([]byte, len(origKey))
	copy(key, origKey)

	// Decrypt all cache files and write them back as plain data.
	// We use Decrypt directly instead of toggling the session key, because
	// ClearSessionKey zeroes the slice in-place which would corrupt our copy.
	for _, f := range files {
		if f == cfgPath {
			continue
		}
		encrypted, err := os.ReadFile(f)
		if err != nil {
			continue // File may not exist
		}
		plain, err := Decrypt(encrypted, key)
		if err != nil {
			continue // File may not be encrypted
		}
		if err := os.WriteFile(f, plain, 0600); err != nil {
			return err
		}
	}

	// Clear session key so SaveConfig writes plain JSON and restores passwords to keyring
	ClearSessionKey()

	// Re-save config — this will use the keyring (no session key) and strip passwords from JSON
	if cfg != nil {
		if err := SaveConfig(cfg); err != nil {
			return fmt.Errorf("failed to save plain config: %w", err)
		}
	}

	// Remove secure.meta
	path, err := secureMetaPath()
	if err != nil {
		return err
	}
	_ = os.Remove(path)

	return nil
}

// SecureReadFile reads a file, decrypting it if a session key is set.
func SecureReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key := GetSessionKey()
	if key == nil {
		return data, nil
	}
	return Decrypt(data, key)
}

// SecureWriteFile writes data to a file, encrypting it if a session key is set.
func SecureWriteFile(path string, data []byte, perm os.FileMode) error {
	key := GetSessionKey()
	if key == nil {
		return os.WriteFile(path, data, perm)
	}
	encrypted, err := Encrypt(data, key)
	if err != nil {
		return err
	}
	return os.WriteFile(path, encrypted, perm)
}

// reEncryptCacheFiles reads all plain cache/data files (excluding config.json) and writes them encrypted.
func reEncryptCacheFiles() error {
	files, err := collectDataFiles()
	if err != nil {
		return err
	}

	// Find config.json path to skip it (already saved separately with passwords)
	cfgPath, _ := configFile()

	for _, f := range files {
		if f == cfgPath {
			continue // Already handled by SaveConfig
		}
		plainData, err := os.ReadFile(f)
		if err != nil {
			continue // File may not exist
		}
		// Write encrypted using SecureWriteFile (session key is already set)
		if err := SecureWriteFile(f, plainData, 0600); err != nil {
			return err
		}
	}
	return nil
}

// collectDataFiles returns paths to all data files that should be encrypted/decrypted.
func collectDataFiles() ([]string, error) {
	var files []string

	// Config files
	cfgDir, err := configDir()
	if err != nil {
		return nil, err
	}
	files = append(files, filepath.Join(cfgDir, "config.json"))

	// Cache files
	cDir, err := cacheDir()
	if err != nil {
		return nil, err
	}
	cacheFiles := []string{"email_cache.json", "contacts.json", "drafts.json", "folder_cache.json"}
	for _, f := range cacheFiles {
		files = append(files, filepath.Join(cDir, f))
	}

	// Folder email cache files
	folderDir := filepath.Join(cDir, "folder_emails")
	if entries, err := os.ReadDir(folderDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				files = append(files, filepath.Join(folderDir, entry.Name()))
			}
		}
	}

	// Email body cache files
	bodyDir := filepath.Join(cDir, "email_bodies")
	if entries, err := os.ReadDir(bodyDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				files = append(files, filepath.Join(bodyDir, entry.Name()))
			}
		}
	}

	// Signature files
	sigDir := filepath.Join(cfgDir, "signatures")
	if entries, err := os.ReadDir(sigDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				files = append(files, filepath.Join(sigDir, entry.Name()))
			}
		}
	}

	return files, nil
}
