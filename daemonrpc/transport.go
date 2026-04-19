package daemonrpc

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

// Conn wraps a net.Conn with newline-delimited JSON encoding/decoding.
type Conn struct {
	conn net.Conn
	enc  *json.Encoder
	dec  *json.Decoder
	mu   sync.Mutex // serializes writes
}

// NewConn wraps an existing network connection.
func NewConn(c net.Conn) *Conn {
	return &Conn{
		conn: c,
		enc:  json.NewEncoder(c),
		dec:  json.NewDecoder(c),
	}
}

// Send writes a JSON-encoded message followed by a newline.
// Thread-safe.
func (c *Conn) Send(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enc.Encode(v)
}

// SendResponse sends a successful response with the given result.
func (c *Conn) SendResponse(id uint64, result interface{}) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	return c.Send(&Response{
		ID:     id,
		Result: raw,
	})
}

// SendError sends an error response.
func (c *Conn) SendError(id uint64, code int, message string) error {
	return c.Send(&Response{
		ID:    id,
		Error: &Error{Code: code, Message: message},
	})
}

// SendEvent sends a push event to the client.
func (c *Conn) SendEvent(eventType string, data interface{}) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal event data: %w", err)
	}
	return c.Send(&Event{
		Type: eventType,
		Data: raw,
	})
}

// ReceiveMessage reads and decodes the next JSON message, returning
// a discriminated Message (Request, Response, or Event).
func (c *Conn) ReceiveMessage() (Message, error) {
	var raw json.RawMessage
	if err := c.dec.Decode(&raw); err != nil {
		return Message{}, err
	}
	return DecodeMessage(raw)
}

// Close closes the underlying connection.
func (c *Conn) Close() error {
	return c.conn.Close()
}

// RemoteAddr returns the remote address of the connection.
func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// LocalAddr returns the local address of the connection.
func (c *Conn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}
