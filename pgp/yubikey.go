package pgp

import (
	"bytes"
	"crypto"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"os"
	"time"

	pgpcrypto "github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
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
// publicKeyPath is the path to the account's PGP public key file, used to read
// key metadata (fingerprint, key ID, algorithm) for building a valid OpenPGP
// signature packet.
func BuildPGPSignedMessage(payload []byte, pin string, publicKeyPath string) ([]byte, error) {
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
	privKey, err := card.PrivateKey(openpgp.KeySign, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get signing key from card: %w", err)
	}

	signer, ok := privKey.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("signing key does not implement crypto.Signer")
	}

	// Load the public key entity to get metadata for the signature packet
	signingKey, err := loadSigningPublicKey(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load public key: %w", err)
	}

	// Split payload into headers and body for MIME structure
	headers, body := splitPayload(payload)

	// Build the signed body part (this is what gets hashed)
	boundary := fmt.Sprintf("----=_Part_%d", time.Now().Unix())
	signedPart := buildSignedPart(headers, body, boundary)

	// Build the OpenPGP signature packet
	sigPacket, err := buildSignaturePacket(signedPart, signer, signingKey)
	if err != nil {
		return nil, fmt.Errorf("failed to build signature: %w", err)
	}

	// Armor the signature
	armoredSig, err := armorSignature(sigPacket)
	if err != nil {
		return nil, fmt.Errorf("failed to armor signature: %w", err)
	}

	return buildMultipartSigned(headers, body, boundary, armoredSig), nil
}

// loadSigningPublicKey reads a PGP public key file and returns the signing
// subkey's PublicKey (or the primary key if no signing subkey exists).
func loadSigningPublicKey(path string) (*packet.PublicKey, error) {
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	entities, err := pgpcrypto.ReadArmoredKeyRing(bytes.NewReader(keyData))
	if err != nil {
		entities, err = pgpcrypto.ReadKeyRing(bytes.NewReader(keyData))
		if err != nil {
			return nil, fmt.Errorf("failed to parse PGP key: %w", err)
		}
	}
	if len(entities) == 0 {
		return nil, fmt.Errorf("no keys found in keyring")
	}

	entity := entities[0]

	// Look for a signing subkey first
	now := time.Now()
	for _, subkey := range entity.Subkeys {
		if subkey.Sig != nil && subkey.Sig.FlagsValid && subkey.Sig.FlagSign && !subkey.PublicKey.KeyExpired(subkey.Sig, now) {
			return subkey.PublicKey, nil
		}
	}

	// Fall back to primary key
	return entity.PrimaryKey, nil
}

// buildSignaturePacket creates a valid OpenPGP v4 signature packet.
func buildSignaturePacket(signedContent []byte, signer crypto.Signer, pubKey *packet.PublicKey) ([]byte, error) {
	now := time.Now()
	hashAlgo := crypto.SHA256
	hashAlgoID := byte(8) // SHA-256 in OpenPGP

	// Build hashed subpackets
	var hashedSubpackets bytes.Buffer

	// Subpacket: signature creation time (type 2)
	writeSubpacket(&hashedSubpackets, 2, func(buf *bytes.Buffer) {
		ts := make([]byte, 4)
		binary.BigEndian.PutUint32(ts, uint32(now.Unix()))
		buf.Write(ts)
	})

	// Subpacket: issuer key ID (type 16)
	writeSubpacket(&hashedSubpackets, 16, func(buf *bytes.Buffer) {
		kid := make([]byte, 8)
		binary.BigEndian.PutUint64(kid, pubKey.KeyId)
		buf.Write(kid)
	})

	// Subpacket: issuer fingerprint (type 33)
	writeSubpacket(&hashedSubpackets, 33, func(buf *bytes.Buffer) {
		buf.WriteByte(byte(pubKey.Version))
		buf.Write(pubKey.Fingerprint)
	})

	// Build hash suffix (RFC 4880, Section 5.2.4)
	var hashSuffix bytes.Buffer
	hashSuffix.WriteByte(4)                       // version
	hashSuffix.WriteByte(0x00)                    // signature type: binary
	hashSuffix.WriteByte(byte(pubKey.PubKeyAlgo)) // public key algorithm
	hashSuffix.WriteByte(hashAlgoID)              // hash algorithm
	hsLen := hashedSubpackets.Len()
	hashSuffix.WriteByte(byte(hsLen >> 8))
	hashSuffix.WriteByte(byte(hsLen))
	hashSuffix.Write(hashedSubpackets.Bytes())

	// V4 hash trailer
	trailer := hashSuffix.Bytes()
	var hashTrailer bytes.Buffer
	hashTrailer.WriteByte(4)    // version
	hashTrailer.WriteByte(0xff) // marker
	tLen := make([]byte, 4)
	binary.BigEndian.PutUint32(tLen, uint32(len(trailer)))
	hashTrailer.Write(tLen)

	// Hash the signed content + hash suffix + trailer
	hasher := hashAlgo.New()
	hasher.Write(signedContent)
	hasher.Write(trailer)
	hasher.Write(hashTrailer.Bytes())
	digest := hasher.Sum(nil)

	// Sign with the YubiKey
	rawSig, err := signer.Sign(nil, digest, hashAlgo)
	if err != nil {
		return nil, fmt.Errorf("signing failed: %w", err)
	}

	// Build the complete signature packet body
	var body bytes.Buffer
	body.Write(trailer) // version + sig type + algo + hash algo + hashed subpackets

	// Unhashed subpackets (empty)
	body.WriteByte(0)
	body.WriteByte(0)

	// Hash tag (first 2 bytes of digest)
	body.WriteByte(digest[0])
	body.WriteByte(digest[1])

	// Encode the signature MPIs based on algorithm
	switch pubKey.PubKeyAlgo {
	case packet.PubKeyAlgoEdDSA:
		// EdDSA: raw signature is r || s, 32 bytes each
		if len(rawSig) != 64 {
			return nil, fmt.Errorf("unexpected EdDSA signature length: %d", len(rawSig))
		}
		writeMPI(&body, rawSig[:32]) // r
		writeMPI(&body, rawSig[32:]) // s

	case packet.PubKeyAlgoRSA, packet.PubKeyAlgoRSASignOnly:
		// RSA: single MPI
		writeMPI(&body, rawSig)

	case packet.PubKeyAlgoECDSA:
		// ECDSA: card returns ASN.1 DER encoded (R, S)
		r, s, err := parseASN1Signature(rawSig)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ECDSA signature: %w", err)
		}
		writeMPI(&body, r)
		writeMPI(&body, s)

	default:
		return nil, fmt.Errorf("unsupported key algorithm: %d", pubKey.PubKeyAlgo)
	}

	// Wrap in an OpenPGP packet (new-format header)
	var pkt bytes.Buffer
	bodyBytes := body.Bytes()
	pkt.WriteByte(0xC2) // new-format packet tag for signature (type 2)
	writeNewFormatLength(&pkt, len(bodyBytes))
	pkt.Write(bodyBytes)

	return pkt.Bytes(), nil
}

// armorSignature wraps a binary OpenPGP signature in ASCII armor.
func armorSignature(sigPacket []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, "PGP SIGNATURE", nil)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(sigPacket); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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
func buildSignedPart(headers, body []byte, boundary string) []byte {
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

// writeSubpacket writes a single OpenPGP subpacket.
func writeSubpacket(w *bytes.Buffer, typ byte, writeContent func(*bytes.Buffer)) {
	var content bytes.Buffer
	writeContent(&content)
	length := content.Len() + 1 // +1 for type byte
	if length < 192 {
		w.WriteByte(byte(length))
	} else {
		// Two-octet length
		length -= 192
		w.WriteByte(byte(length>>8) + 192)
		w.WriteByte(byte(length))
	}
	w.WriteByte(typ)
	w.Write(content.Bytes())
}

// writeMPI writes a big-endian integer as an OpenPGP MPI (2-byte bit count + data).
func writeMPI(w io.Writer, data []byte) {
	// Strip leading zero bytes
	for len(data) > 0 && data[0] == 0 {
		data = data[1:]
	}
	if len(data) == 0 {
		data = []byte{0}
	}
	bitLen := uint16((len(data)-1)*8 + bitLength(data[0]))
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, bitLen)
	w.Write(buf)  //nolint:errcheck
	w.Write(data) //nolint:errcheck
}

// bitLength returns the number of significant bits in a byte.
func bitLength(b byte) int {
	n := 0
	for b > 0 {
		n++
		b >>= 1
	}
	return n
}

// writeNewFormatLength writes an OpenPGP new-format packet body length.
func writeNewFormatLength(w *bytes.Buffer, length int) {
	if length < 192 {
		w.WriteByte(byte(length))
	} else if length < 8384 {
		length -= 192
		w.WriteByte(byte(length>>8) + 192)
		w.WriteByte(byte(length))
	} else {
		w.WriteByte(255)
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(length))
		w.Write(buf)
	}
}

// parseASN1Signature extracts r and s from an ASN.1 DER encoded ECDSA signature.
//
// Each intermediate slice access is bounds-checked against len(der). A truncated
// or malformed signature produces a typed error rather than an index-out-of-range
// panic; the minimum-length check up front only rules out obvious runts (#613).
func parseASN1Signature(der []byte) (r, s []byte, err error) {
	// ASN.1 SEQUENCE { INTEGER r, INTEGER s }
	if len(der) < 6 || der[0] != 0x30 {
		return nil, nil, fmt.Errorf("invalid ASN.1 signature")
	}

	pos := 2 // skip SEQUENCE tag and length

	// Parse R
	if pos >= len(der) || der[pos] != 0x02 {
		return nil, nil, fmt.Errorf("expected INTEGER tag for R")
	}
	pos++
	if pos >= len(der) {
		return nil, nil, fmt.Errorf("ASN.1 signature truncated before R length")
	}
	rLen := int(der[pos])
	pos++
	if pos+rLen > len(der) {
		return nil, nil, fmt.Errorf("ASN.1 signature truncated: R length overflow")
	}
	rVal := new(big.Int).SetBytes(der[pos : pos+rLen])
	pos += rLen

	// Parse S
	if pos >= len(der) || der[pos] != 0x02 {
		return nil, nil, fmt.Errorf("expected INTEGER tag for S")
	}
	pos++
	if pos >= len(der) {
		return nil, nil, fmt.Errorf("ASN.1 signature truncated before S length")
	}
	sLen := int(der[pos])
	pos++
	if pos+sLen > len(der) {
		return nil, nil, fmt.Errorf("ASN.1 signature truncated: S length overflow")
	}
	sVal := new(big.Int).SetBytes(der[pos : pos+sLen])

	return rVal.Bytes(), sVal.Bytes(), nil
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
