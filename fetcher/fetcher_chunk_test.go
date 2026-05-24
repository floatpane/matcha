package fetcher

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/floatpane/matcha/config"
)

func TestFetchMailboxEmailsUsesRequestedLimitForSmallFetchChunks(t *testing.T) {
	fetchCommands := make(chan string, 1)
	addr, closeServer := startFetchRecorderIMAPServer(t, 100, fetchCommands)
	defer closeServer()

	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("Atoi(%q): %v", portText, err)
	}

	account := &config.Account{
		ID:              "test-account",
		Email:           "user@example.com",
		Password:        "password",
		ServiceProvider: "custom",
		IMAPServer:      host,
		IMAPPort:        port,
		Insecure:        true,
		CatchAll:        true,
		SC:              &config.SessionCache{},
	}
	done := make(chan error, 1)
	go func() {
		_, err := FetchMailboxEmails(account, "INBOX", 5, 0)
		done <- err
	}()

	select {
	case command := <-fetchCommands:
		if !strings.Contains(command, "96:100") {
			t.Fatalf("first FETCH command = %q, want range 96:100", command)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for FETCH command")
	}

	closeServer()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("FetchMailboxEmails did not return after server closed")
	}
}

func startFetchRecorderIMAPServer(t *testing.T, messages uint32, fetchCommands chan<- string) (string, func()) {
	t.Helper()

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{newTestTLSCertificate(t)},
	})
	if err != nil {
		t.Fatalf("starting test IMAP server: %v", err)
	}

	var closeOnce sync.Once
	var connMu sync.Mutex
	var conn net.Conn
	closeServer := func() {
		closeOnce.Do(func() {
			connMu.Lock()
			if conn != nil {
				_ = conn.Close()
			}
			connMu.Unlock()
			_ = listener.Close()
		})
	}

	go func() {
		accepted, err := listener.Accept()
		if err != nil {
			return
		}
		connMu.Lock()
		conn = accepted
		connMu.Unlock()
		serveFetchRecorderIMAPConn(accepted, messages, fetchCommands)
	}()

	return listener.Addr().String(), closeServer
}

func serveFetchRecorderIMAPConn(conn net.Conn, messages uint32, fetchCommands chan<- string) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	writeIMAPLine := func(format string, args ...any) bool {
		if _, err := fmt.Fprintf(writer, format+"\r\n", args...); err != nil {
			return false
		}
		return writer.Flush() == nil
	}

	if !writeIMAPLine("* OK matcha test server") {
		return
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return
		}

		tag := fields[0]
		switch strings.ToUpper(fields[1]) {
		case "CAPABILITY":
			if !writeIMAPLine("* CAPABILITY IMAP4rev1 AUTH=PLAIN") {
				return
			}
			if !writeIMAPLine("%s OK CAPABILITY completed", tag) {
				return
			}
		case "LOGIN":
			if !writeIMAPLine("%s OK LOGIN completed", tag) {
				return
			}
		case "SELECT":
			if !writeIMAPLine("* %d EXISTS", messages) {
				return
			}
			if !writeIMAPLine("* FLAGS (\\Seen)") {
				return
			}
			if !writeIMAPLine("%s OK [READ-WRITE] SELECT completed", tag) {
				return
			}
		case "FETCH":
			fetchCommands <- line
			_ = writeIMAPLine("%s NO recorded FETCH command", tag)
			return
		case "LOGOUT":
			if !writeIMAPLine("* BYE logging out") {
				return
			}
			_ = writeIMAPLine("%s OK LOGOUT completed", tag)
			return
		default:
			if !writeIMAPLine("%s OK completed", tag) {
				return
			}
		}
	}
}

func newTestTLSCertificate(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating private key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("parsing certificate: %v", err)
	}
	return cert
}
