package fetcher

import (
	"fmt"

	"github.com/emersion/go-sasl"
)

// xoauth2Client implements the XOAUTH2 SASL mechanism for Gmail.
// See https://developers.google.com/gmail/imap/xoauth2-protocol
type xoauth2Client struct {
	Username string
	Token    string
}

func (a *xoauth2Client) Start() (mech string, ir []byte, err error) {
	// XOAUTH2 initial response format:
	// "user=" {User} "\x01auth=Bearer " {Access Token} "\x01\x01"
	ir = []byte(fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", a.Username, a.Token))
	return "XOAUTH2", ir, nil
}

func (a *xoauth2Client) Next(challenge []byte) ([]byte, error) {
	// Server sent an error challenge; respond with empty to complete the exchange.
	return []byte{}, nil
}

// Verify xoauth2Client implements sasl.Client at compile time.
var _ sasl.Client = (*xoauth2Client)(nil)

// newXOAuth2Client creates a new XOAUTH2 SASL client.
func newXOAuth2Client(username, token string) sasl.Client {
	return &xoauth2Client{Username: username, Token: token}
}
