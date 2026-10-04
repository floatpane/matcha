package pgp

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	pgpmail "github.com/emersion/go-pgpmail"
	"github.com/floatpane/matcha/config"
)

// armorEntity serializes entity to ASCII-armored format.
// private=true writes the private key block, false writes the public key block.
func armorEntity(t *testing.T, entity *openpgp.Entity, private bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	blockType := openpgp.PublicKeyType
	if private {
		blockType = openpgp.PrivateKeyType
	}
	w, err := armor.Encode(&buf, blockType, nil)
	if err != nil {
		t.Fatalf("armor.Encode: %v", err)
	}
	if private {
		if err := entity.SerializePrivate(w, nil); err != nil {
			t.Fatalf("SerializePrivate: %v", err)
		}
	} else {
		if err := entity.Serialize(w); err != nil {
			t.Fatalf("Serialize: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("armor close: %v", err)
	}
	return buf.Bytes()
}

// newTestProvider creates a FileBasedProvider backed by a freshly generated key.
// A new entity is generated for each call; key generation can take ~50–200 ms.
func newTestProvider(t *testing.T) (*FileBasedProvider, *openpgp.Entity) {
	t.Helper()
	entity, err := openpgp.NewEntity("Test User", "", "test@test.com", nil)
	if err != nil {
		t.Fatalf("NewEntity: %v", err)
	}

	dir := t.TempDir()
	pgpDir := filepath.Join(dir, "pgp")
	if err := os.Mkdir(pgpDir, 0700); err != nil {
		t.Fatalf("mkdir pgp: %v", err)
	}

	privPath := filepath.Join(dir, "private.asc")
	pubPath := filepath.Join(dir, "public.asc")

	if err := os.WriteFile(privPath, armorEntity(t, entity, true), 0600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	if err := os.WriteFile(pubPath, armorEntity(t, entity, false), 0644); err != nil {
		t.Fatalf("write public key: %v", err)
	}
	// Place the public key in pgpDir under the sender address for loadPublicKeyring.
	if err := os.WriteFile(filepath.Join(pgpDir, "test@test.com.asc"), armorEntity(t, entity, false), 0644); err != nil {
		t.Fatalf("write recipient key: %v", err)
	}

	account := &config.Account{
		PGPPrivateKey: privPath,
		PGPPublicKey:  pubPath,
		PGPKeySource:  "file",
	}
	return &FileBasedProvider{account: account, pgpDir: pgpDir}, entity
}

func TestSplitTransportHeaders(t *testing.T) {
	payload := []byte("From: alice@example.com\r\nTo: bob@example.com\r\nSubject: Test\r\n" +
		"Content-Type: text/plain\r\nMIME-Version: 1.0\r\n\r\nHello world")

	header, body := splitTransportHeaders(payload)

	if got := header.Get("From"); got != "alice@example.com" {
		t.Errorf("From = %q, want %q", got, "alice@example.com")
	}
	if got := header.Get("Subject"); got != "Test" {
		t.Errorf("Subject = %q, want %q", got, "Test")
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "Content-Type: text/plain") {
		t.Error("body part missing Content-Type header")
	}
	if !strings.Contains(bodyStr, "MIME-Version: 1.0") {
		t.Error("body part missing MIME-Version header")
	}
	if !strings.Contains(bodyStr, "Hello world") {
		t.Error("body part missing message body")
	}
	if strings.Contains(bodyStr, "From:") {
		t.Error("body part must not contain transport From header")
	}
	if strings.Contains(bodyStr, "To:") {
		t.Error("body part must not contain transport To header")
	}
}

func TestSplitTransportHeadersNoSeparator(t *testing.T) {
	payload := []byte("just a body with no headers")
	_, body := splitTransportHeaders(payload)
	if !bytes.Equal(body, payload) {
		t.Errorf("payload without separator: got %q, want %q", body, payload)
	}
}

func TestFileBasedProviderSignMIMEStructure(t *testing.T) {
	provider, _ := newTestProvider(t)

	payload := []byte("From: test@test.com\r\nSubject: Hello\r\n" +
		"Content-Type: text/plain\r\nMIME-Version: 1.0\r\n\r\nHello world")

	signed, err := provider.Sign(payload)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	out := string(signed)

	// RFC 3156 §3: top-level Content-Type must be multipart/signed.
	if !strings.Contains(out, "multipart/signed") {
		t.Error("missing multipart/signed Content-Type")
	}
	if !strings.Contains(out, `protocol="application/pgp-signature"`) {
		t.Error("missing protocol=application/pgp-signature parameter")
	}
	if !strings.Contains(out, "micalg=pgp-sha256") {
		t.Error("missing micalg=pgp-sha256 parameter")
	}

	// Second part must declare its Content-Type.
	if !strings.Contains(out, "application/pgp-signature") {
		t.Error("missing application/pgp-signature in signature part")
	}

	// A real PGP signature must be present.
	if !strings.Contains(out, "-----BEGIN PGP") {
		t.Error("missing PGP armor block in output")
	}

	// Original body text must appear in the signed part.
	if !strings.Contains(out, "Hello world") {
		t.Error("signed body text missing from output")
	}
}

func TestFileBasedProviderEncryptMIMEStructure(t *testing.T) {
	provider, _ := newTestProvider(t)

	payload := []byte("Content-Type: text/plain\r\nMIME-Version: 1.0\r\n\r\nSecret message")

	encrypted, err := provider.Encrypt(payload, []string{"test@test.com"})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	out := string(encrypted)

	// RFC 3156 §4: top-level Content-Type must be multipart/encrypted.
	if !strings.Contains(out, "multipart/encrypted") {
		t.Error("missing multipart/encrypted Content-Type")
	}
	if !strings.Contains(out, `protocol="application/pgp-encrypted"`) {
		t.Error("missing protocol=application/pgp-encrypted parameter")
	}

	// Must contain the OpenPGP ciphertext armor block.
	if !strings.Contains(out, "-----BEGIN PGP MESSAGE-----") {
		t.Error("missing BEGIN PGP MESSAGE armor block")
	}

	// Plain body must not appear in the encrypted output.
	if strings.Contains(out, "Secret message") {
		t.Error("plaintext body must not appear in encrypted output")
	}
}

func TestFileBasedProviderEncryptDecryptRoundtrip(t *testing.T) {
	provider, _ := newTestProvider(t)

	const want = "Confidential message body"
	payload := []byte("Content-Type: text/plain\r\nMIME-Version: 1.0\r\n\r\n" + want)

	encrypted, err := provider.Encrypt(payload, []string{"test@test.com"})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	decrypted, err := provider.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !strings.Contains(string(decrypted), want) {
		t.Errorf("decrypted output missing %q; got:\n%s", want, decrypted)
	}
}

// TestFileBasedProviderDecryptBareArmor ensures the MIME-oriented Decrypt entry
// point also accepts a bare ASCII-armored message, which is what server-side
// encryption (e.g. mailbox.org's Encrypted Inbox) stores in the
// application/octet-stream part. Handing that armor to a MIME parser is the bug
// from #1715: it fails with "malformed MIME header line" before the key is used.
func TestFileBasedProviderDecryptBareArmor(t *testing.T) {
	provider, entity := newTestProvider(t)

	const want = "server-side encrypted body"
	var armored bytes.Buffer
	aw, err := armor.Encode(&armored, "PGP MESSAGE", nil)
	if err != nil {
		t.Fatalf("armor.Encode: %v", err)
	}
	pw, err := openpgp.Encrypt(aw, []*openpgp.Entity{entity}, nil, nil, nil)
	if err != nil {
		t.Fatalf("openpgp.Encrypt: %v", err)
	}
	if _, err := pw.Write([]byte(want)); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}
	if err := pw.Close(); err != nil {
		t.Fatalf("plaintext close: %v", err)
	}
	if err := aw.Close(); err != nil {
		t.Fatalf("armor close: %v", err)
	}

	decrypted, err := provider.Decrypt(armored.Bytes())
	if err != nil {
		t.Fatalf("Decrypt(bare armor): %v", err)
	}
	if string(decrypted) != want {
		t.Fatalf("Decrypt(bare armor) = %q, want %q", decrypted, want)
	}
}

// TestFileBasedProviderDecryptBareAcceptsMIME ensures DecryptBare delegates a
// complete multipart/encrypted entity to the MIME path instead of failing to
// decode it as armor.
func TestFileBasedProviderDecryptBareAcceptsMIME(t *testing.T) {
	provider, _ := newTestProvider(t)

	const want = "Confidential message body"
	payload := []byte("Content-Type: text/plain\r\nMIME-Version: 1.0\r\n\r\n" + want)

	encrypted, err := provider.Encrypt(payload, []string{"test@test.com"})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	decrypted, err := provider.DecryptBare(encrypted)
	if err != nil {
		t.Fatalf("DecryptBare(MIME entity): %v", err)
	}
	if !strings.Contains(string(decrypted), want) {
		t.Fatalf("DecryptBare(MIME entity) missing %q; got:\n%s", want, decrypted)
	}
}

func TestFileBasedProviderSignVerifyRoundtrip(t *testing.T) {
	provider, _ := newTestProvider(t)

	payload := []byte("From: test@test.com\r\n" +
		"Content-Type: text/plain\r\nMIME-Version: 1.0\r\n\r\nHello from sign+verify test")

	// Sign the payload.
	signedMsg, err := provider.Sign(payload)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Confirm the signed message verifies using pgpmail.Read directly.
	keyring := provider.loadPublicKeyring()
	mr, err := pgpmail.Read(bytes.NewReader(signedMsg), keyring, nil, nil)
	if err != nil {
		t.Fatalf("pgpmail.Read: %v", err)
	}
	if mr.MessageDetails == nil {
		t.Fatal("pgpmail.Read returned nil MessageDetails")
	}
	if _, err := io.ReadAll(mr.MessageDetails.UnverifiedBody); err != nil {
		t.Fatalf("drain UnverifiedBody: %v", err)
	}
	if mr.MessageDetails.SignatureError != nil {
		t.Fatalf("pgpmail signature error: %v", mr.MessageDetails.SignatureError)
	}

	// Extract the exact MIME parts from the signed output and test Verify.
	signedPart, sigData := extractSignedPartsRaw(t, signedMsg)
	status, err := provider.Verify(signedPart, sigData)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if status != PGPStatusVerified {
		t.Errorf("Verify = %v, want PGPStatusVerified", status)
	}
}

func TestFileBasedProviderVerifyUnknownKey(t *testing.T) {
	provider, _ := newTestProvider(t)

	// Sign with a different (unknown) key.
	other, err := openpgp.NewEntity("Other", "", "other@test.com", nil)
	if err != nil {
		t.Fatalf("NewEntity: %v", err)
	}

	const signedContent = "Content-Type: text/plain\r\n\r\nTampered message"
	var sigBuf bytes.Buffer
	if err := openpgp.DetachSign(&sigBuf, other, strings.NewReader(signedContent), nil); err != nil {
		t.Fatalf("DetachSign: %v", err)
	}

	// Armor the signature.
	var armoredSig bytes.Buffer
	aw, _ := armor.Encode(&armoredSig, "PGP SIGNATURE", nil)
	aw.Write(sigBuf.Bytes())
	aw.Close()

	status, _ := provider.Verify([]byte(signedContent), armoredSig.Bytes())
	if status == PGPStatusVerified {
		t.Error("Verify with unknown key must not return PGPStatusVerified")
	}
}

func TestPGPStatusString(t *testing.T) {
	cases := []struct {
		status PGPStatus
		want   string
	}{
		{PGPStatusNone, ""},
		{PGPStatusVerified, "[PGP: Verified]"},
		{PGPStatusUnverified, "[PGP: Unverified]"},
		{PGPStatusEncrypted, "[PGP: Encrypted]"},
	}
	for _, tc := range cases {
		if got := tc.status.String(); got != tc.want {
			t.Errorf("PGPStatus(%d).String() = %q, want %q", tc.status, got, tc.want)
		}
	}
}

// extractSignedPartsRaw extracts the first MIME body part bytes (signedPart)
// and the PGP armor block from the second MIME part (sigData) using raw boundary
// splitting — avoiding any map-iteration-order issues from mime/multipart headers.
func extractSignedPartsRaw(t *testing.T, signedMsg []byte) (signedPart, sigData []byte) {
	t.Helper()

	s := string(signedMsg)

	// Locate Content-Type header to retrieve the boundary.
	// Unfold RFC 2822 header continuation lines (CRLF followed by WSP).
	headerEnd := strings.Index(s, "\r\n\r\n")
	if headerEnd < 0 {
		t.Fatal("extractSignedPartsRaw: no header/body separator")
	}
	unfolded := strings.ReplaceAll(s[:headerEnd], "\r\n\t", " ")
	unfolded = strings.ReplaceAll(unfolded, "\r\n ", " ")

	var boundary string
	for _, line := range strings.Split(unfolded, "\r\n") {
		if strings.HasPrefix(strings.ToUpper(line), "CONTENT-TYPE:") {
			ct := strings.TrimSpace(line[len("Content-Type:"):])
			_, params, _ := mime.ParseMediaType(ct)
			boundary = params["boundary"]
		}
	}
	if boundary == "" {
		t.Fatal("extractSignedPartsRaw: no boundary in Content-Type")
	}

	body := s[headerEnd+4:]

	delim1 := "--" + boundary + "\r\n"     // start of each part
	delim2 := "\r\n--" + boundary + "\r\n" // separator between parts
	delimEnd := "\r\n--" + boundary + "--" // closing boundary

	// Find the content of part 1.
	idx1 := strings.Index(body, delim1)
	if idx1 < 0 {
		t.Fatal("part 1 boundary not found")
	}
	partStart := idx1 + len(delim1)

	// Find the separator between part 1 and part 2.
	idx2 := strings.Index(body[partStart:], delim2)
	if idx2 < 0 {
		// Try also the alternative without leading \r\n (when part 1 ends at EOF).
		t.Fatal("part 2 separator not found")
	}
	signedPart = []byte(body[partStart : partStart+idx2])

	// Part 2 starts after the separator.
	part2Start := partStart + idx2 + len(delim2)

	// Find the closing boundary of part 2.
	idx3 := strings.Index(body[part2Start:], delimEnd)
	if idx3 < 0 {
		t.Fatal("end boundary not found")
	}
	part2Raw := body[part2Start : part2Start+idx3]

	// Strip the part 2 Content-Type header to get the bare signature block.
	if hi := strings.Index(part2Raw, "\r\n\r\n"); hi >= 0 {
		sigData = []byte(part2Raw[hi+4:])
	} else {
		sigData = []byte(part2Raw)
	}

	// Also parse via mime/multipart to double-check boundary count.
	mr := multipart.NewReader(strings.NewReader(body), boundary)
	if _, err := mr.NextPart(); err != nil {
		t.Fatalf("extractSignedPartsRaw multipart check part 1: %v", err)
	}
	if _, err := mr.NextPart(); err != nil {
		t.Fatalf("extractSignedPartsRaw multipart check part 2: %v", err)
	}

	return
}
