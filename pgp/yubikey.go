package pgp

import (
	"bytes"
	"crypto"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/ebfe/scard"

	iso "cunicu.li/go-iso7816"
	"cunicu.li/go-iso7816/drivers/pcsc"
	"cunicu.li/go-iso7816/filter"

	openpgp "cunicu.li/go-openpgp-card"
)

// openCard connects to the first available OpenPGP smartcard via PC/SC.
func openCard() (*openpgp.Card, error) {
	ctx, err := scard.EstablishContext()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to connect to PC/SC daemon: %w\n"+
				"Make sure pcscd is running:\n"+
				"  sudo systemctl enable --now pcscd.socket\n"+
				"You may also need the ccid package for USB smartcard support.",
			err,
		)
	}

	pcscCard, err := pcsc.OpenFirstCard(ctx, filter.HasApplet(iso.AidOpenPGP), true)
	if err != nil {
		ctx.Release()
		return nil, fmt.Errorf(
			"no OpenPGP smartcard found: %w\n"+
				"Make sure your YubiKey is plugged in and has an OpenPGP key configured.",
			err,
		)
	}

	isoCard := iso.NewCard(pcscCard)
	card, err := openpgp.NewCard(isoCard)
	if err != nil {
		pcscCard.Close()
		ctx.Release()
		return nil, fmt.Errorf("failed to initialize OpenPGP card: %w", err)
	}

	return card, nil
}

// BuildPGPSignedMessage creates a multipart/signed MIME message using a YubiKey.
func BuildPGPSignedMessage(payload []byte, pin string) ([]byte, error) {
	card, err := openCard()
	if err != nil {
		return nil, err
	}
	defer card.Close()

	// Verify PIN (PW1 for signing operations)
	if err := card.VerifyPassword(openpgp.PW1, pin); err != nil {
		return nil, fmt.Errorf("PIN verification failed: %w", err)
	}

	// Get the signing private key from the card.
	// The second argument is a public key hint; nil lets the card figure it out.
	privKey, err := card.PrivateKey(openpgp.KeySign, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get signing key from card: %w", err)
	}

	signer, ok := privKey.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("signing key does not implement crypto.Signer")
	}

	// Hash the payload with SHA-256
	hash := crypto.SHA256
	hasher := hash.New()
	hasher.Write(payload)
	digest := hasher.Sum(nil)

	// Sign on the card
	signature, err := signer.Sign(nil, digest, hash)
	if err != nil {
		return nil, fmt.Errorf("signing on card failed: %w", err)
	}

	return buildMultipartSigned(payload, signature), nil
}

// VerifyYubiKeyAvailable checks if a YubiKey with OpenPGP support is connected.
func VerifyYubiKeyAvailable() error {
	card, err := openCard()
	if err != nil {
		return err
	}
	card.Close()
	return nil
}

// GetYubiKeyInfo returns human-readable information about the connected card.
func GetYubiKeyInfo() (string, error) {
	card, err := openCard()
	if err != nil {
		return "", err
	}
	defer card.Close()

	var info string

	aid := card.ApplicationRelated.AID
	info += fmt.Sprintf("Manufacturer: %s\n", aid.Manufacturer)
	info += fmt.Sprintf("Serial:       %X\n", aid.Serial)
	info += fmt.Sprintf("Version:      %s\n", aid.Version)

	ch, err := card.GetCardholder()
	if err == nil && ch.Name != "" {
		info += fmt.Sprintf("Cardholder:   %s\n", ch.Name)
	}

	if keys := card.ApplicationRelated.Keys; keys != nil {
		if ki, ok := keys[openpgp.KeySign]; ok {
			info += fmt.Sprintf("Sign Key:     %s", ki.AlgAttrs)
			if ki.Status == openpgp.KeyGenerated {
				info += " (generated)"
			} else if ki.Status == openpgp.KeyImported {
				info += " (imported)"
			}
			info += "\n"
		}
	}

	return info, nil
}

// buildMultipartSigned creates the MIME structure for a PGP signed message.
// The payload contains the full message (headers + body). We split them so
// the top-level headers (From, To, Subject, etc.) stay at the top and only
// the body is wrapped into the multipart/signed structure. The original
// Content-Type header is moved to the first signed part so mail clients
// can properly parse the body content.
func buildMultipartSigned(payload, signature []byte) []byte {
	boundary := fmt.Sprintf("----=_Part_%d", time.Now().Unix())
	var result bytes.Buffer

	// Split payload into headers and body at the first blank line (\r\n\r\n)
	var headers, body []byte
	if idx := bytes.Index(payload, []byte("\r\n\r\n")); idx >= 0 {
		headers = payload[:idx]
		body = payload[idx+4:] // skip the \r\n\r\n separator
	} else {
		// No headers found, treat entire payload as body
		body = payload
	}

	// Separate transport headers (From, To, etc.) from content headers (Content-Type)
	var originalContentType []byte
	if len(headers) > 0 {
		for _, line := range bytes.Split(headers, []byte("\r\n")) {
			upper := bytes.ToUpper(line)
			if bytes.HasPrefix(upper, []byte("CONTENT-TYPE:")) {
				// Save the original Content-Type for the signed body part
				originalContentType = line
				continue
			}
			if bytes.HasPrefix(upper, []byte("MIME-VERSION:")) {
				continue
			}
			if len(line) > 0 {
				result.Write(line)
				result.WriteString("\r\n")
			}
		}
	}

	// Write the new top-level Content-Type for multipart/signed
	result.WriteString("MIME-Version: 1.0\r\n")
	result.WriteString("Content-Type: multipart/signed; ")
	result.WriteString("boundary=\"" + boundary + "\"; ")
	result.WriteString("micalg=pgp-sha256; ")
	result.WriteString("protocol=\"application/pgp-signature\"\r\n")
	result.WriteString("\r\n") // end of headers

	// Write first part (original body with its original Content-Type)
	result.WriteString("--" + boundary + "\r\n")
	if len(originalContentType) > 0 {
		result.Write(originalContentType)
		result.WriteString("\r\n\r\n")
	}
	result.Write(body)
	result.WriteString("\r\n")

	// Write second part (signature)
	result.WriteString("--" + boundary + "\r\n")
	result.WriteString("Content-Type: application/pgp-signature; name=\"signature.asc\"\r\n")
	result.WriteString("Content-Description: OpenPGP digital signature\r\n")
	result.WriteString("Content-Disposition: attachment; filename=\"signature.asc\"\r\n\r\n")

	// Write ASCII-armored signature
	result.WriteString("-----BEGIN PGP SIGNATURE-----\r\n\r\n")

	// Base64 encode signature in 76-character lines (per OpenPGP armor spec)
	encoded := base64.StdEncoding.EncodeToString(signature)
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		result.WriteString(encoded[i:end])
		result.WriteString("\r\n")
	}

	result.WriteString("-----END PGP SIGNATURE-----\r\n")
	result.WriteString("--" + boundary + "--\r\n")

	return result.Bytes()
}
