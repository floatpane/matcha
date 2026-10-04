package pgp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	textproto "github.com/emersion/go-message/textproto"
	pgpmail "github.com/emersion/go-pgpmail"

	"github.com/floatpane/matcha/config"
)

// FileBasedProvider implements PGPProvider using PGP keys stored on disk.
type FileBasedProvider struct {
	account *config.Account
	pgpDir  string // directory holding per-address recipient public keys
}

// Sign wraps payload in a RFC 3156 multipart/signed MIME message using the
// account's private key. Passphrase-protected keys are unlocked with
// account.PGPPIN.
func (p *FileBasedProvider) Sign(payload []byte) ([]byte, error) {
	entity, err := p.loadPrivateEntity()
	if err != nil {
		return nil, err
	}

	transportHeader, bodyPayload := splitTransportHeaders(payload)

	var out bytes.Buffer
	mw, err := pgpmail.Sign(&out, transportHeader, entity, nil)
	if err != nil {
		return nil, fmt.Errorf("pgp sign: %w", err)
	}
	if _, err := mw.Write(bodyPayload); err != nil {
		return nil, fmt.Errorf("pgp sign write: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("pgp sign close: %w", err)
	}
	return out.Bytes(), nil
}

// Encrypt wraps payload in a RFC 3156 multipart/encrypted MIME message.
// Recipient public keys are loaded from pgpDir/<email>.asc, .gpg, or .pem.
// The sender's own public key is added so the Sent copy is readable.
func (p *FileBasedProvider) Encrypt(payload []byte, recipients []string) ([]byte, error) {
	entityList, err := p.loadRecipientEntities(recipients)
	if err != nil {
		return nil, err
	}

	var header textproto.Header
	var out bytes.Buffer
	mw, err := pgpmail.Encrypt(&out, header, entityList, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("pgp encrypt: %w", err)
	}
	if _, err := mw.Write(payload); err != nil {
		return nil, fmt.Errorf("pgp encrypt write: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("pgp encrypt close: %w", err)
	}
	return out.Bytes(), nil
}

// Decrypt decrypts a multipart/encrypted MIME payload using the account's
// private key. A bare ASCII-armored OpenPGP message is detected and handled by
// the armor-aware path, because pgpmail.Read expects a complete MIME entity and
// would otherwise fail on the armor's first line with a malformed-header error.
// Server-side encrypted mail (e.g. mailbox.org's Encrypted Inbox) reaches this
// method with the raw body of its application/octet-stream part.
func (p *FileBasedProvider) Decrypt(payload []byte) ([]byte, error) {
	if IsBareArmoredMessage(payload) {
		return p.DecryptBare(payload)
	}

	entity, err := p.loadPrivateEntity()
	if err != nil {
		return nil, err
	}

	mr, err := pgpmail.Read(bytes.NewReader(payload), openpgp.EntityList{entity}, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("pgp decrypt: %w", err)
	}
	if mr.MessageDetails == nil || mr.MessageDetails.UnverifiedBody == nil {
		return nil, errors.New("pgp decrypt: no decrypted content")
	}

	var out bytes.Buffer
	if _, err := io.Copy(&out, mr.MessageDetails.UnverifiedBody); err != nil {
		return nil, fmt.Errorf("pgp decrypt read: %w", err)
	}
	return out.Bytes(), nil
}

// DecryptBare decrypts a bare ASCII-armored OpenPGP ciphertext block using the
// account's private key. Unlike Decrypt, the input is the raw armored block
// (the body of application/octet-stream), not a full multipart/encrypted message.
// A complete MIME entity is delegated back to Decrypt so either entry point
// accepts either form.
func (p *FileBasedProvider) DecryptBare(armored []byte) ([]byte, error) {
	if !IsBareArmoredMessage(armored) {
		return p.Decrypt(armored)
	}

	entity, err := p.loadPrivateEntity()
	if err != nil {
		return nil, err
	}

	block, err := armor.Decode(bytes.NewReader(armored))
	if err != nil {
		return nil, fmt.Errorf("pgp: decode armor: %w", err)
	}

	md, err := openpgp.ReadMessage(block.Body, openpgp.EntityList{entity}, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("pgp: decrypt: %w", err)
	}
	if md.UnverifiedBody == nil {
		return nil, errors.New("pgp: no decrypted content")
	}

	var out bytes.Buffer
	if _, err := io.Copy(&out, md.UnverifiedBody); err != nil {
		return nil, fmt.Errorf("pgp: read decrypted content: %w", err)
	}
	return out.Bytes(), nil
}

// IsBareArmoredMessage reports whether data is a bare ASCII-armored OpenPGP
// message rather than a complete MIME entity. Leading whitespace is ignored so
// armor fetched from an IMAP body section (which may carry a leading CRLF) is
// still recognized.
func IsBareArmoredMessage(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimSpace(data), []byte("-----BEGIN PGP MESSAGE-----"))
}

// Verify checks a detached PGP signature against signedContent (the raw bytes
// of the first MIME body part from a multipart/signed message). Returns
// PGPStatusVerified when the signature is valid and a matching key is in the
// account's keyring, PGPStatusUnverified otherwise.
func (p *FileBasedProvider) Verify(signedContent, signatureData []byte) (PGPStatus, error) {
	keyring := p.loadPublicKeyring()
	if len(keyring) == 0 {
		return PGPStatusUnverified, nil
	}

	return verifyWithKeyring(signedContent, signatureData, keyring)
}

// VerifyWithSender attempts to verify a PGP signature, falling back to WKD
// lookup for the sender's key when the local keyring doesn't contain it.
// If WKD succeeds, the key is cached locally for future use.
func (p *FileBasedProvider) VerifyWithSender(signedContent, signatureData []byte, senderEmail string) (PGPStatus, error) {
	keyring := p.loadPublicKeyring()

	status, err := verifyWithKeyring(signedContent, signatureData, keyring)
	if status == PGPStatusVerified {
		return status, err
	}

	email := extractEmail(senderEmail)
	if email == "" {
		return PGPStatusUnverified, nil
	}

	entity, wkdErr := LookupWKD(email)
	if wkdErr != nil {
		return PGPStatusUnverified, wkdErr
	}

	if cacheErr := CacheWKDKey(p.pgpDir, email, entity); cacheErr != nil {
		_ = cacheErr
	}

	keyring = append(keyring, entity)
	return verifyWithKeyring(signedContent, signatureData, keyring)
}

func verifyWithKeyring(signedContent, signatureData []byte, keyring openpgp.EntityList) (PGPStatus, error) {
	if len(keyring) == 0 {
		return PGPStatusUnverified, nil
	}

	const boundary = "pgp-verify-boundary"
	var msg bytes.Buffer
	msg.WriteString("Content-Type: multipart/signed; boundary=\"" + boundary + "\"; " +
		"micalg=pgp-sha256; protocol=\"application/pgp-signature\"\r\n\r\n")
	msg.WriteString("--" + boundary + "\r\n")
	msg.Write(signedContent)
	msg.WriteString("\r\n--" + boundary + "\r\n")
	msg.WriteString("Content-Type: application/pgp-signature\r\n\r\n")
	msg.Write(signatureData)
	msg.WriteString("\r\n--" + boundary + "--\r\n")

	mr, _ := pgpmail.Read(&msg, keyring, nil, nil)
	if mr == nil || mr.MessageDetails == nil || mr.MessageDetails.UnverifiedBody == nil {
		return PGPStatusUnverified, nil
	}
	_, _ = io.ReadAll(mr.MessageDetails.UnverifiedBody)
	if mr.MessageDetails.SignatureError != nil {
		return PGPStatusUnverified, mr.MessageDetails.SignatureError
	}
	return PGPStatusVerified, nil
}

// loadPrivateEntity reads the account's private key file and, if it is
// passphrase-protected, decrypts it using account.PGPPIN.
func (p *FileBasedProvider) loadPrivateEntity() (*openpgp.Entity, error) {
	if p.account.PGPPrivateKey == "" {
		return nil, errors.New("pgp: private key path not configured")
	}
	data, err := os.ReadFile(p.account.PGPPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("pgp: read private key: %w", err)
	}
	entityList, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(data))
	if err != nil {
		entityList, err = openpgp.ReadKeyRing(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("pgp: parse private key: %w", err)
		}
	}
	if len(entityList) == 0 {
		return nil, errors.New("pgp: no keys in private keyring")
	}
	entity := entityList[0]
	if entity.PrivateKey != nil && entity.PrivateKey.Encrypted {
		if p.account.PGPPIN == "" {
			return nil, errors.New("pgp: private key is encrypted but no passphrase configured")
		}
		if err := entity.DecryptPrivateKeys([]byte(p.account.PGPPIN)); err != nil {
			return nil, fmt.Errorf("pgp: decrypt private key: %w", err)
		}
	}
	return entity, nil
}

func (p *FileBasedProvider) loadRecipientEntities(recipients []string) (openpgp.EntityList, error) {
	var list openpgp.EntityList

	for _, recipient := range recipients {
		email := extractEmail(recipient)
		entity, err := p.loadPublicKeyForEmail(email)
		if err != nil {
			return nil, fmt.Errorf("pgp: no key for %s: %w", email, err)
		}
		list = append(list, entity)
	}

	// Always include sender's own public key so the Sent copy is readable.
	if p.account.PGPPublicKey != "" {
		if entity, err := loadPublicEntity(p.account.PGPPublicKey); err == nil {
			list = append(list, entity)
		}
	}

	if len(list) == 0 {
		return nil, errors.New("pgp: no recipient keys found")
	}
	return list, nil
}

func (p *FileBasedProvider) loadPublicKeyForEmail(email string) (*openpgp.Entity, error) {
	for _, ext := range []string{".asc", ".gpg", ".pem"} {
		if entity, err := loadPublicEntity(filepath.Join(p.pgpDir, email+ext)); err == nil {
			return entity, nil
		}
	}

	entity, err := LookupWKD(email)
	if err != nil {
		return nil, fmt.Errorf("no key file found in %s for %s and WKD lookup failed: %w", p.pgpDir, email, err)
	}

	if cacheErr := CacheWKDKey(p.pgpDir, email, entity); cacheErr != nil {
		// Non-fatal: we have the key even if caching fails.
		_ = cacheErr
	}
	return entity, nil
}

func (p *FileBasedProvider) loadPublicKeyring() openpgp.EntityList {
	var keyring openpgp.EntityList

	addKey := func(path string) {
		if entity, err := loadPublicEntity(path); err == nil {
			keyring = append(keyring, entity)
		}
	}

	if p.account.PGPPublicKey != "" {
		addKey(p.account.PGPPublicKey)
	}

	entries, err := os.ReadDir(p.pgpDir)
	if err != nil {
		return keyring
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".asc") || strings.HasSuffix(name, ".gpg") || strings.HasSuffix(name, ".pem") {
			addKey(filepath.Join(p.pgpDir, name))
		}
	}
	return keyring
}

// loadPublicEntity reads and parses the first entity from an armored or binary
// OpenPGP public key file.
func loadPublicEntity(path string) (*openpgp.Entity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	list, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(data))
	if err != nil {
		list, err = openpgp.ReadKeyRing(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("parse key %s: %w", path, err)
		}
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no keys in %s", path)
	}
	return list[0], nil
}

// splitTransportHeaders separates envelope headers (From, To, Subject, etc.)
// from content headers (Content-Type, MIME-Version) and the message body.
//
// Transport headers are returned as a textproto.Header for the outer
// pgpmail.Sign / pgpmail.Encrypt wrapper. Content headers + blank line + body
// are returned as a single byte slice that becomes the signed or encrypted part.
func splitTransportHeaders(payload []byte) (textproto.Header, []byte) {
	var header textproto.Header
	var contentPart bytes.Buffer

	idx := bytes.Index(payload, []byte("\r\n\r\n"))
	if idx < 0 {
		return header, payload
	}

	for _, line := range bytes.Split(payload[:idx], []byte("\r\n")) {
		if len(line) == 0 {
			continue
		}
		parts := bytes.SplitN(line, []byte(": "), 2)
		if len(parts) != 2 {
			continue
		}
		key := string(parts[0])
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "CONTENT-") || upper == "MIME-VERSION" {
			contentPart.Write(line)
			contentPart.WriteString("\r\n")
		} else {
			header.Set(key, string(parts[1]))
		}
	}

	contentPart.WriteString("\r\n")
	contentPart.Write(payload[idx+4:])
	return header, contentPart.Bytes()
}

func extractEmail(addr string) string {
	addr = strings.TrimSpace(addr)
	if i := strings.Index(addr, "<"); i >= 0 {
		addr = strings.TrimSuffix(addr[i+1:], ">")
	}
	return strings.TrimSpace(addr)
}
