package fetcher

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"errors"
	"math/big"
	"testing"
	"time"

	"go.mozilla.org/pkcs7"
)

// "MII=" is the base64 transfer encoding of the two bytes 0x30 0x82, which
// makes go.mozilla.org/pkcs7 index out of range in its BER reader. Any
// incoming email with such an application/pkcs7-signature part used to crash
// the TUI when opened or previewed.
func TestSafePKCS7ParseMalformedDoesNotPanic(t *testing.T) {
	payload, err := decodeAttachmentData([]byte("MII="), "base64")
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "0\x82" {
		t.Fatalf("decoded payload = %q", payload)
	}
	if _, err := safePKCS7Parse(payload); err == nil {
		t.Fatal("expected an error for a truncated PKCS#7 header")
	}
	// Same input through the S/MIME helper, including its base64 retry.
	if _, _, err := parseSMIMEPayload([]byte("MII=")); err == nil {
		t.Fatal("parseSMIMEPayload: expected an error")
	}
	if _, _, err := parseSMIMEPayload(payload); err == nil {
		t.Fatal("parseSMIMEPayload: expected an error")
	}
}

func TestParseSMIMEPayloadAcceptsValidAndBase64(t *testing.T) {
	der := signedTestPKCS7(t)
	if p7, raw, err := parseSMIMEPayload(der); err != nil || p7 == nil || len(raw) != len(der) {
		t.Fatalf("DER: p7=%v err=%v", p7, err)
	}
	b64 := []byte(base64.StdEncoding.EncodeToString(der))
	wrapped := append(append([]byte{}, b64[:10]...), append([]byte("\r\n"), b64[10:]...)...)
	if p7, raw, err := parseSMIMEPayload(wrapped); err != nil || p7 == nil || len(raw) != len(der) {
		t.Fatalf("base64: p7=%v err=%v", p7, err)
	}
}

// Decrypting an envelope with an algorithm pkcs7 does not support made the
// library print to stdout (corrupting the TUI); the wrapper must refuse it
// before the library is called.
func TestSafePKCS7DecryptRefusesUnsupportedAlgorithm(t *testing.T) {
	aes192CBC := asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 22}
	raw := envelopedWithAlgorithm(t, aes192CBC)
	if !isEnvelopedData(raw) {
		t.Fatal("isEnvelopedData = false for an envelopedData ContentInfo")
	}
	if hasSupportedContentEncryption(raw) {
		t.Fatal("hasSupportedContentEncryption = true for AES-192-CBC")
	}
	if _, err := safePKCS7Decrypt(&pkcs7.PKCS7{}, raw, nil, nil); !errors.Is(err, pkcs7.ErrUnsupportedAlgorithm) {
		t.Fatalf("err = %v, want ErrUnsupportedAlgorithm", err)
	}
	if !hasSupportedContentEncryption(envelopedWithAlgorithm(t, pkcs7.OIDEncryptionAlgorithmAES256CBC)) {
		t.Fatal("AES-256-CBC should be recognized as supported")
	}
	if isEnvelopedData(signedTestPKCS7(t)) {
		t.Fatal("signedData misdetected as envelopedData")
	}
}

func signedTestPKCS7(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "t"}, NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	sd, err := pkcs7.NewSignedData([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if err := sd.AddSigner(cert, key, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatal(err)
	}
	out, err := sd.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// envelopedWithAlgorithm builds a minimal envelopedData ContentInfo whose
// encryptedContentInfo uses the given content encryption algorithm.
func envelopedWithAlgorithm(t *testing.T, alg asn1.ObjectIdentifier) []byte {
	t.Helper()
	type algID struct {
		Algorithm  asn1.ObjectIdentifier
		Parameters asn1.RawValue `asn1:"optional"`
	}
	type encryptedContentInfo struct {
		ContentType                asn1.ObjectIdentifier
		ContentEncryptionAlgorithm algID
		EncryptedContent           asn1.RawValue `asn1:"tag:0,optional"`
	}
	type envelopedData struct {
		Version              int
		RecipientInfos       []asn1.RawValue `asn1:"set"`
		EncryptedContentInfo encryptedContentInfo
	}
	inner, err := asn1.Marshal(envelopedData{
		EncryptedContentInfo: encryptedContentInfo{
			ContentType:                pkcs7.OIDData,
			ContentEncryptionAlgorithm: algID{Algorithm: alg},
			EncryptedContent:           asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, Bytes: []byte{1, 2, 3}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	type contentInfo struct {
		ContentType asn1.ObjectIdentifier
		Content     asn1.RawValue `asn1:"explicit,optional,tag:0"`
	}
	out, err := asn1.Marshal(contentInfo{
		ContentType: pkcs7.OIDEnvelopedData,
		Content:     asn1.RawValue{FullBytes: append([]byte{0xa0, byte(len(inner))}, inner...)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
