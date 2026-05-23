package config

import (
	"os"
	"path/filepath"
)

// signatureFile returns the full path to the global signature file.
func signatureFile() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "signature.txt"), nil
}

// accountSignatureFile returns the path to the per-account signature file.
func accountSignatureFile(accountID string) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "signatures", accountID+".txt"), nil
}

// LoadSignature loads the signature from the global signature file.
func LoadSignature() (string, error) {
	path, err := signatureFile()
	if err != nil {
		return "", err
	}
	data, err := SecureReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// LoadRawAccountSignature loads the per-account signature if one exists,
// without falling back to the global signature.
func LoadRawAccountSignature(account *Account) (string, error) {
	if account == nil || account.ID == "" {
		return "", nil
	}

	// Check for per-account signature file first
	path, err := accountSignatureFile(account.ID)
	if err != nil {
		return "", err
	}
	data, err := SecureReadFile(path)
	if err == nil && len(data) > 0 {
		return string(data), nil
	}

	// Fall back to inline account signature
	if account.Signature != "" {
		return account.Signature, nil
	}

	return "", nil
}

// LoadSignatureForAccount loads the per-account signature if one exists,
// otherwise falls back to the global signature.
func LoadSignatureForAccount(account *Account) (string, error) {
	sig, err := LoadRawAccountSignature(account)
	if err == nil && sig != "" {
		return sig, nil
	}
	// Fall back to global signature
	return LoadSignature()
}

// SaveSignature saves the signature to the global signature file.
func SaveSignature(signature string) error {
	path, err := signatureFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return SecureWriteFile(path, []byte(signature), 0600)
}

// SaveSignatureForAccount saves a per-account signature file.
func SaveSignatureForAccount(accountID, signature string) error {
	path, err := accountSignatureFile(accountID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if signature == "" {
		// Remove the file to fall back to global
		os.Remove(path) //nolint:errcheck,gosec
		return nil
	}
	return SecureWriteFile(path, []byte(signature), 0600)
}

// HasSignature checks if a global signature file exists and is non-empty.
func HasSignature() bool {
	sig, err := LoadSignature()
	if err != nil {
		return false
	}
	return sig != ""
}

// HasAccountSignature checks if an account has its own signature (file or inline).
func HasAccountSignature(account *Account) bool {
	sig, err := LoadRawAccountSignature(account)
	if err != nil {
		return false
	}
	return sig != ""
}
