package fetcher

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"fmt"

	"go.mozilla.org/pkcs7"
)

// The helpers below wrap go.mozilla.org/pkcs7, which parses attacker-controlled
// S/MIME payloads from incoming mail. The library panics on some malformed
// input (e.g. pkcs7.Parse([]byte("0\x82")) indexes out of range in its BER
// reader) and prints to stdout when it meets an unsupported content
// encryption algorithm. Either would kill or corrupt the TUI, so every call
// site goes through these wrappers instead.

// supportedContentEncryptionOIDs mirrors the algorithms pkcs7.Decrypt accepts
// (see the library's encryptedContentInfo.decrypt). For anything else the
// library writes "Unsupported Content Encryption Algorithm" to stdout.
var supportedContentEncryptionOIDs = []asn1.ObjectIdentifier{
	pkcs7.OIDEncryptionAlgorithmDESCBC,
	pkcs7.OIDEncryptionAlgorithmDESEDE3CBC,
	pkcs7.OIDEncryptionAlgorithmAES256CBC,
	pkcs7.OIDEncryptionAlgorithmAES128CBC,
	pkcs7.OIDEncryptionAlgorithmAES128GCM,
	pkcs7.OIDEncryptionAlgorithmAES256GCM,
}

// safePKCS7Parse parses a PKCS#7 blob, converting library panics into errors.
func safePKCS7Parse(data []byte) (p7 *pkcs7.PKCS7, err error) {
	defer func() {
		if r := recover(); r != nil {
			p7 = nil
			err = fmt.Errorf("pkcs7: malformed payload: %v", r)
		}
	}()
	return pkcs7.Parse(data)
}

// safePKCS7Decrypt decrypts an enveloped PKCS#7 payload. raw must be the bytes
// p7 was parsed from; they are used to refuse unsupported content encryption
// algorithms before the library gets a chance to print to stdout.
func safePKCS7Decrypt(p7 *pkcs7.PKCS7, raw []byte, cert *x509.Certificate, key crypto.PrivateKey) (out []byte, err error) {
	if isEnvelopedData(raw) && !hasSupportedContentEncryption(raw) {
		return nil, pkcs7.ErrUnsupportedAlgorithm
	}
	defer func() {
		if r := recover(); r != nil {
			out = nil
			err = fmt.Errorf("pkcs7: decrypt failed on malformed payload: %v", r)
		}
	}()
	return p7.Decrypt(cert, key)
}

// safePKCS7VerifyWithChain verifies signatures, converting panics into errors.
func safePKCS7VerifyWithChain(p7 *pkcs7.PKCS7, roots *x509.CertPool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pkcs7: verify failed on malformed payload: %v", r)
		}
	}()
	return p7.VerifyWithChain(roots)
}

// isEnvelopedData reports whether a (BER or DER) ContentInfo starts with the
// envelopedData content type OID.
func isEnvelopedData(raw []byte) bool {
	if len(raw) < 2 || raw[0] != 0x30 {
		return false
	}
	// Skip the outer SEQUENCE length (short form, long form, or BER indefinite).
	off := 2
	if l := raw[1]; l > 0x80 {
		off += int(l & 0x7f)
	}
	oid, err := asn1.Marshal(pkcs7.OIDEnvelopedData)
	if err != nil || off > len(raw) {
		return false
	}
	return bytes.HasPrefix(raw[off:], oid)
}

// hasSupportedContentEncryption reports whether raw contains the encoding of
// a content encryption algorithm pkcs7 can decrypt. OIDs are primitive, so
// their encoding is contiguous in both BER and DER input.
func hasSupportedContentEncryption(raw []byte) bool {
	for _, oid := range supportedContentEncryptionOIDs {
		enc, err := asn1.Marshal(oid)
		if err == nil && bytes.Contains(raw, enc) {
			return true
		}
	}
	return false
}

// parseSMIMEPayload parses an S/MIME PKCS#7 part. IMAP servers sometimes drop
// the transfer-encoding header, so on failure it retries after stripping line
// breaks and base64-decoding. It returns the parsed structure together with
// the bytes it was parsed from.
func parseSMIMEPayload(data []byte) (*pkcs7.PKCS7, []byte, error) {
	p7, err := safePKCS7Parse(data)
	if err == nil {
		return p7, data, nil
	}
	clean := bytes.ReplaceAll(data, []byte("\n"), nil)
	clean = bytes.ReplaceAll(clean, []byte("\r"), nil)
	decoded, b64err := base64.StdEncoding.DecodeString(string(clean))
	if b64err != nil {
		return nil, nil, err
	}
	p7, err = safePKCS7Parse(decoded)
	if err != nil {
		return nil, nil, err
	}
	return p7, decoded, nil
}
