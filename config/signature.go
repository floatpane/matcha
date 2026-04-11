package config

import (
	"os"
	"path/filepath"
)

// signatureFile returns the full path to the signature file.
func signatureFile() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "signature.txt"), nil
}

// LoadSignature loads the signature from the signature file.
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

// SaveSignature saves the signature to the signature file.
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

// HasSignature checks if a signature file exists and is non-empty.
func HasSignature() bool {
	sig, err := LoadSignature()
	if err != nil {
		return false
	}
	return sig != ""
}
