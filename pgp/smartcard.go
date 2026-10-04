package pgp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	cardhl "github.com/floatpane/go-openpgp-card-hl"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/internal/loglevel"
)

// releaseSCDaemon asks gpg to drop its scdaemon, which otherwise holds the
// OpenPGP card inside a PC/SC transaction. Without this, a direct shared PC/SC
// session connects but blocks indefinitely on the first APDU transmit because
// scdaemon never releases the card. gpg re-spawns scdaemon on demand, so the
// gpg-agent fallback path still works afterward.
func releaseSCDaemon() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "gpgconf", "--kill", "scdaemon").Run(); err != nil {
		loglevel.Debugf("pgp smartcard: kill scdaemon: %v", err)
	}
}

// SmartcardProvider implements PGPProvider using a hardware OpenPGP token
// (e.g. YubiKey). The device's private key never leaves the hardware.
type SmartcardProvider struct {
	account *config.Account
	pgpDir  string
}

// Sign delegates to BuildPGPSignedMessage, which manages the PC/SC session
// and produces a RFC 3156 multipart/signed message.
func (p *SmartcardProvider) Sign(payload []byte) ([]byte, error) {
	if p.account.PGPPIN == "" {
		return nil, errors.New("pgp smartcard: PIN not configured")
	}
	if p.account.PGPPublicKey == "" {
		return nil, errors.New("pgp smartcard: public key path not configured")
	}
	return BuildPGPSignedMessage(payload, p.account.PGPPIN, p.account.PGPPublicKey)
}

// Encrypt uses recipient public keys from disk; no private key is required,
// so this delegates directly to FileBasedProvider.
func (p *SmartcardProvider) Encrypt(payload []byte, recipients []string) ([]byte, error) {
	return (&FileBasedProvider{account: p.account, pgpDir: p.pgpDir}).Encrypt(payload, recipients)
}

// Decrypt decrypts a multipart/encrypted message using the card's on-device
// decryption key. The private key never leaves the hardware.
//
// Only RSA decryption keys are supported by the PC/SC interface; for
// ECDH/Curve25519 keys use a gpg-agent backed flow instead.
func (p *SmartcardProvider) Decrypt(payload []byte) ([]byte, error) {
	if IsBareArmoredMessage(payload) {
		return p.DecryptBare(payload)
	}
	if p.account.PGPPIN == "" {
		return nil, errors.New("pgp smartcard: PIN not configured")
	}
	if p.account.PGPPublicKey == "" {
		return nil, errors.New("pgp smartcard: public key path not configured")
	}

	// gpg's scdaemon keeps the card locked in a PC/SC transaction; drop it so
	// the direct session below can talk to the card instead of blocking.
	releaseSCDaemon()

	plain, err := p.cardDecrypt(payload)
	if err == nil {
		return plain, nil
	}
	if errors.Is(err, cardhl.ErrUnsupportedKey) || errors.Is(err, errCardTimeout) {
		loglevel.Debugf("pgp smartcard: %v, falling back to gpg-agent", err)
		return gpgAgentDecryptMIME(payload, p.account.PGPPIN)
	}
	return nil, err
}

// errCardTimeout signals the direct PC/SC session hung (typically still-present
// scdaemon contention), so the caller should fall back to the gpg-agent path.
var errCardTimeout = errors.New("pgp smartcard: card session timed out")

// cardDecrypt runs the direct PC/SC open + decrypt under a deadline. PC/SC
// transmits have no native timeout, so a hung card (e.g. held by scdaemon)
// would otherwise block forever; the deadline turns that into errCardTimeout.
func (p *SmartcardProvider) cardDecrypt(payload []byte) ([]byte, error) {
	pubEntity, err := cardhl.LoadEntity(p.account.PGPPublicKey)
	if err != nil {
		return nil, fmt.Errorf("pgp smartcard: load public key: %w", err)
	}

	type result struct {
		plain []byte
		err   error
	}
	ch := make(chan result, 1)

	go func() {
		loglevel.Debugf("pgp smartcard: opening card")
		card, err := cardhl.Open()
		loglevel.Debugf("pgp smartcard: Open returned err=%v", err)
		if err != nil {
			ch <- result{nil, err}
			return
		}
		defer card.Close() //nolint:errcheck

		loglevel.Debugf("pgp smartcard: calling DecryptMIME")
		plain, err := card.DecryptMIME(payload, p.account.PGPPIN, pubEntity)
		loglevel.Debugf("pgp smartcard: DecryptMIME returned len=%d err=%v", len(plain), err)
		ch <- result{plain, err}
	}()

	select {
	case r := <-ch:
		return r.plain, r.err
	case <-time.After(20 * time.Second):
		loglevel.Debugf("pgp smartcard: direct PC/SC session timed out")
		return nil, errCardTimeout
	}
}

// DecryptBare wraps a bare ASCII-armored OpenPGP ciphertext block in a minimal
// multipart/encrypted MIME envelope and delegates to Decrypt, which handles
// both RSA PC/SC decryption and the gpg-agent fallback for ECDH/Curve25519.
func (p *SmartcardProvider) DecryptBare(armored []byte) ([]byte, error) {
	const boundary = "pgp-bare-wrap"
	var mime bytes.Buffer
	fmt.Fprintf(&mime, "Content-Type: multipart/encrypted; protocol=\"application/pgp-encrypted\"; boundary=\"%s\"\r\n\r\n", boundary)
	fmt.Fprintf(&mime, "--%s\r\n", boundary)
	mime.WriteString("Content-Type: application/pgp-encrypted\r\n\r\nVersion: 1\r\n")
	fmt.Fprintf(&mime, "\r\n--%s\r\n", boundary)
	mime.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	mime.Write(armored)
	fmt.Fprintf(&mime, "\r\n--%s--\r\n", boundary)
	return p.Decrypt(mime.Bytes())
}

// Verify delegates to FileBasedProvider since signature verification requires
// only public keys, which are always available on disk.
func (p *SmartcardProvider) Verify(signedContent, signatureData []byte) (PGPStatus, error) {
	return (&FileBasedProvider{account: p.account, pgpDir: p.pgpDir}).Verify(signedContent, signatureData)
}
