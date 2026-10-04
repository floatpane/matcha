package fetcher

import (
	"bytes"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/emersion/go-message/textproto"
	pgpmail "github.com/emersion/go-pgpmail"

	"github.com/floatpane/matcha/config"
)

const pgpTestPlaintext = "Content-Type: text/plain; charset=utf-8\r\n\r\nhello from the encrypted inbox\r\n"

// newPGPTestAccount writes a throwaway private key to disk and returns an
// account pointing at it along with the matching entity.
func newPGPTestAccount(t *testing.T) (*config.Account, *openpgp.Entity) {
	t.Helper()

	entity, err := openpgp.NewEntity("Test", "", "test@example.com", nil)
	if err != nil {
		t.Fatalf("NewEntity() error: %v", err)
	}

	var key bytes.Buffer
	aw, err := armor.Encode(&key, openpgp.PrivateKeyType, nil)
	if err != nil {
		t.Fatalf("armor.Encode() error: %v", err)
	}
	if err := entity.SerializePrivate(aw, nil); err != nil {
		t.Fatalf("SerializePrivate() error: %v", err)
	}
	if err := aw.Close(); err != nil {
		t.Fatalf("armor close error: %v", err)
	}

	keyPath := filepath.Join(t.TempDir(), "private.asc")
	if err := os.WriteFile(keyPath, key.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return &config.Account{PGPPrivateKey: keyPath}, entity
}

// bareArmoredMessage returns an ASCII-armored OpenPGP message encrypting
// pgpTestPlaintext to entity. This is the shape stored in the
// application/octet-stream part of a server-side encrypted message.
func bareArmoredMessage(t *testing.T, entity *openpgp.Entity) []byte {
	t.Helper()

	var armored bytes.Buffer
	aw, err := armor.Encode(&armored, "PGP MESSAGE", nil)
	if err != nil {
		t.Fatalf("armor.Encode() error: %v", err)
	}
	pw, err := openpgp.Encrypt(aw, []*openpgp.Entity{entity}, nil, nil, nil)
	if err != nil {
		t.Fatalf("openpgp.Encrypt() error: %v", err)
	}
	if _, err := pw.Write([]byte(pgpTestPlaintext)); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if err := pw.Close(); err != nil {
		t.Fatalf("plaintext close error: %v", err)
	}
	if err := aw.Close(); err != nil {
		t.Fatalf("armor close error: %v", err)
	}
	return armored.Bytes()
}

// TestDecryptPGPMessageBareArmor covers the regression from #1715: the fetcher
// extracts the bare armored block from an application/octet-stream part, and it
// must be decrypted directly instead of being handed to a MIME parser.
func TestDecryptPGPMessageBareArmor(t *testing.T) {
	account, entity := newPGPTestAccount(t)
	armored := bareArmoredMessage(t, entity)

	if !bytes.HasPrefix(armored, []byte("-----BEGIN PGP MESSAGE-----")) {
		t.Fatalf("test payload is not bare armor: %q", armored[:40])
	}

	decrypted, err := decryptPGPMessage(armored, account)
	if err != nil {
		t.Fatalf("decryptPGPMessage() error: %v", err)
	}
	if string(decrypted) != pgpTestPlaintext {
		t.Fatalf("decryptPGPMessage() = %q, want %q", decrypted, pgpTestPlaintext)
	}
}

// TestDecryptPGPMessageMIMEEntity ensures a complete RFC 3156
// multipart/encrypted entity still goes through the MIME path.
func TestDecryptPGPMessageMIMEEntity(t *testing.T) {
	account, entity := newPGPTestAccount(t)

	var encrypted bytes.Buffer
	var header textproto.Header
	mw, err := pgpmail.Encrypt(&encrypted, header, []*openpgp.Entity{entity}, nil, nil)
	if err != nil {
		t.Fatalf("pgpmail.Encrypt() error: %v", err)
	}
	if _, err := mw.Write([]byte(pgpTestPlaintext)); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	decrypted, err := decryptPGPMessage(encrypted.Bytes(), account)
	if err != nil {
		t.Fatalf("decryptPGPMessage() error: %v", err)
	}
	if !bytes.Contains(decrypted, []byte("hello from the encrypted inbox")) {
		t.Fatalf("decryptPGPMessage() = %q, want body containing plaintext", decrypted)
	}
}

// TestDecryptPGPMessageServerSideEncryptedPart walks the exact MIME structure
// mailbox.org's Encrypted Inbox stores on the server (multipart/encrypted with
// an anonymous application/octet-stream part) and decrypts the extracted part,
// mirroring what checkPart does for incoming mail.
func TestDecryptPGPMessageServerSideEncryptedPart(t *testing.T) {
	account, entity := newPGPTestAccount(t)
	armored := bareArmoredMessage(t, entity)

	var msg bytes.Buffer
	msg.WriteString("Content-Type: multipart/encrypted;\r\n")
	msg.WriteString("\tprotocol=\"application/pgp-encrypted\";\r\n")
	msg.WriteString("\tboundary=\"pgp-boundary\"\r\n\r\n")
	msg.WriteString("--pgp-boundary\r\n")
	msg.WriteString("Content-Type: application/pgp-encrypted\r\n\r\n")
	msg.WriteString("Version: 1\r\n")
	msg.WriteString("--pgp-boundary\r\n")
	msg.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	msg.Write(armored)
	msg.WriteString("\r\n--pgp-boundary--\r\n")

	octet := octetStreamPart(t, msg.Bytes())
	if octet == nil {
		t.Fatal("no application/octet-stream part found")
	}

	decrypted, err := decryptPGPMessage(octet, account)
	if err != nil {
		t.Fatalf("decryptPGPMessage() error: %v", err)
	}
	if !strings.Contains(string(decrypted), "hello from the encrypted inbox") {
		t.Fatalf("decrypted = %q, want body containing plaintext", decrypted)
	}
}

// octetStreamPart returns the body of the application/octet-stream part of a
// multipart/encrypted message.
func octetStreamPart(t *testing.T, raw []byte) []byte {
	t.Helper()

	_, params, err := mime.ParseMediaType(headerValue(t, raw, "Content-Type"))
	if err != nil {
		t.Fatalf("ParseMediaType() error: %v", err)
	}
	boundary, ok := params["boundary"]
	if !ok {
		t.Fatal("no boundary parameter")
	}

	mr := multipart.NewReader(bytes.NewReader(raw), boundary)
	for {
		part, err := mr.NextPart()
		if err != nil {
			return nil
		}
		if strings.EqualFold(part.Header.Get("Content-Type"), "application/octet-stream") {
			body := new(bytes.Buffer)
			if _, err := body.ReadFrom(part); err != nil {
				t.Fatalf("read part: %v", err)
			}
			return bytes.TrimRight(body.Bytes(), "\r\n")
		}
	}
}

// headerValue returns the value of the first occurrence of name in the header
// block of a raw message.
func headerValue(t *testing.T, raw []byte, name string) string {
	t.Helper()

	headerBlock := raw
	if idx := bytes.Index(raw, []byte("\r\n\r\n")); idx >= 0 {
		headerBlock = raw[:idx]
	}
	lines := strings.Split(strings.ReplaceAll(string(headerBlock), "\r\n", "\n"), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(strings.ToLower(line), strings.ToLower(name)+":") {
			continue
		}
		value := strings.TrimSpace(line[len(name)+1:])
		// Fold continuation lines (leading whitespace).
		for j := i + 1; j < len(lines); j++ {
			if !strings.HasPrefix(lines[j], " ") && !strings.HasPrefix(lines[j], "\t") {
				break
			}
			value += " " + strings.TrimSpace(lines[j])
		}
		return value
	}
	t.Fatalf("header %q not found", name)
	return ""
}
