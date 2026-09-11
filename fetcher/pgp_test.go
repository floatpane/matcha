package fetcher

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/emersion/go-message/textproto"
	"github.com/emersion/go-pgpmail"

	"github.com/floatpane/matcha/config"
)

const pgpTestPlaintext = "Content-Type: text/plain; charset=utf-8\r\n\r\nhello from the encrypted inbox\r\n"

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

func TestDecryptPGPMessageBareArmor(t *testing.T) {
	account, entity := newPGPTestAccount(t)

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

	if !bytes.HasPrefix(armored.Bytes(), []byte("-----BEGIN PGP MESSAGE-----")) {
		t.Fatalf("test payload is not bare armor: %q", armored.String()[:40])
	}

	decrypted, err := decryptPGPMessage(armored.Bytes(), account)
	if err != nil {
		t.Fatalf("decryptPGPMessage() error: %v", err)
	}
	if string(decrypted) != pgpTestPlaintext {
		t.Fatalf("decryptPGPMessage() = %q, want %q", decrypted, pgpTestPlaintext)
	}
}

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
