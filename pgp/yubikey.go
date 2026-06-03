package pgp

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"time"

	cardhl "github.com/floatpane/go-openpgp-card-hl"
)

var randRead = rand.Read

// BuildPGPSignedMessage creates a multipart/signed MIME message using a YubiKey.
// publicKeyPath is the path to the account's PGP public key file, used to read
// key metadata (fingerprint, key ID, algorithm) for building a valid OpenPGP
// signature packet.
//
// The card session, signature packet construction, and ASCII armoring are
// handled by github.com/floatpane/go-openpgp-card-hl; this function owns only
// the MIME multipart/signed framing on top of the detached signature.
func BuildPGPSignedMessage(payload []byte, pin string, publicKeyPath string) ([]byte, error) {
	card, err := cardhl.Open()
	if err != nil {
		return nil, err
	}
	defer card.Close() //nolint:errcheck

	// Load the public key entity to get metadata for the signature packet.
	pub, err := cardhl.LoadPublicKey(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load public key: %w", err)
	}

	// Split payload into headers and body for MIME structure.
	headers, body := splitPayload(payload)

	// Build the signed body part (this is what gets hashed and signed).
	boundary := generateMIMEBoundary()
	signedPart := buildSignedPart(headers, body, boundary)

	// Produce a detached, ASCII-armored signature over the signed part.
	armoredSig, err := card.Sign(signedPart, pin, pub)
	if err != nil {
		return nil, err
	}

	return buildMultipartSigned(headers, body, boundary, armoredSig), nil
}

// VerifyYubiKeyAvailable checks if a YubiKey with OpenPGP support is connected.
func VerifyYubiKeyAvailable() error {
	card, err := cardhl.Open()
	if err != nil {
		return err
	}
	return card.Close()
}

// GetYubiKeyInfo returns human-readable information about the connected card.
func GetYubiKeyInfo() (string, error) {
	card, err := cardhl.Open()
	if err != nil {
		return "", err
	}
	defer card.Close() //nolint:errcheck

	info, err := card.Info()
	if err != nil {
		return "", err
	}
	return info.String(), nil
}

func generateMIMEBoundary() string {
	var buf [16]byte
	if n, err := randRead(buf[:]); err == nil && n == len(buf) {
		return fmt.Sprintf("----=_Part_%x", buf[:])
	}
	return fmt.Sprintf("----=_Part_%d", time.Now().UnixNano())
}

// splitPayload splits a MIME message into headers and body.
func splitPayload(payload []byte) (headers, body []byte) {
	if idx := bytes.Index(payload, []byte("\r\n\r\n")); idx >= 0 {
		return payload[:idx], payload[idx+4:]
	}
	return nil, payload
}

// buildSignedPart constructs the first MIME part content that gets hashed.
// This must exactly match what appears between the boundary markers.
func buildSignedPart(headers, body []byte, _ string) []byte {
	var originalContentType []byte
	if len(headers) > 0 {
		for _, line := range bytes.Split(headers, []byte("\r\n")) {
			upper := bytes.ToUpper(line)
			if bytes.HasPrefix(upper, []byte("CONTENT-TYPE:")) {
				originalContentType = line
				break
			}
		}
	}

	var part bytes.Buffer
	if len(originalContentType) > 0 {
		part.Write(originalContentType)
		part.WriteString("\r\n\r\n")
	}
	part.Write(body)
	return part.Bytes()
}

// buildMultipartSigned assembles the complete multipart/signed MIME message.
func buildMultipartSigned(headers, body []byte, boundary string, armoredSig []byte) []byte {
	var result bytes.Buffer

	// Write transport headers (From, To, Subject, etc.) excluding Content-Type and MIME-Version
	var originalContentType []byte
	if len(headers) > 0 {
		for _, line := range bytes.Split(headers, []byte("\r\n")) {
			upper := bytes.ToUpper(line)
			if bytes.HasPrefix(upper, []byte("CONTENT-TYPE:")) {
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
	result.WriteString("\r\n")

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
	result.Write(armoredSig)
	result.WriteString("\r\n")
	result.WriteString("--" + boundary + "--\r\n")

	return result.Bytes()
}
