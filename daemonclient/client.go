package daemonclient

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/floatpane/matcha/daemonrpc"
)

// Client connects to the matcha daemon over a Unix domain socket.
type Client struct {
	conn    *daemonrpc.Conn
	nextID  atomic.Uint64
	pending map[uint64]chan *daemonrpc.Response
	mu      sync.Mutex
	events  chan *daemonrpc.Event
	done    chan struct{}
}

// Dial connects to the daemon socket.
func Dial() (*Client, error) {
	sockPath := daemonrpc.SocketPath()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("connect to daemon: %w", err)
	}

	c := &Client{
		conn:    daemonrpc.NewConn(conn),
		pending: make(map[uint64]chan *daemonrpc.Response),
		events:  make(chan *daemonrpc.Event, 64),
		done:    make(chan struct{}),
	}

	go c.readLoop()
	return c, nil
}

// Call makes a synchronous RPC call to the daemon.
func (c *Client) Call(method string, params interface{}, result interface{}) error {
	id := c.nextID.Add(1)

	// Marshal params.
	var rawParams json.RawMessage
	if params != nil {
		var err error
		rawParams, err = json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal params: %w", err)
		}
	}

	// Register pending response channel.
	ch := make(chan *daemonrpc.Response, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	// Send request.
	req := &daemonrpc.Request{
		ID:     id,
		Method: method,
		Params: rawParams,
	}
	if err := c.conn.Send(req); err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	// Wait for response.
	select {
	case resp := <-ch:
		if resp.Error != nil {
			return resp.Error
		}
		if result != nil && resp.Result != nil {
			return json.Unmarshal(resp.Result, result)
		}
		return nil
	case <-c.done:
		return fmt.Errorf("connection closed")
	}
}

// Events returns the channel that receives push events from the daemon.
func (c *Client) Events() <-chan *daemonrpc.Event {
	return c.events
}

// Close closes the connection to the daemon.
func (c *Client) Close() error {
	select {
	case <-c.done:
		return nil
	default:
		close(c.done)
	}
	return c.conn.Close()
}

// readLoop reads messages from the daemon and dispatches them.
func (c *Client) readLoop() {
	defer close(c.events)

	for {
		msg, err := c.conn.ReceiveMessage()
		if err != nil {
			select {
			case <-c.done:
			default:
				close(c.done)
			}
			return
		}

		if msg.Response != nil {
			c.mu.Lock()
			ch, ok := c.pending[msg.Response.ID]
			c.mu.Unlock()
			if ok {
				ch <- msg.Response
			}
		}

		if msg.Event != nil {
			select {
			case c.events <- msg.Event:
			default:
				// Drop event if channel full.
			}
		}
	}
}

// Ping checks if the daemon is responsive.
func (c *Client) Ping() error {
	var result daemonrpc.PingResult
	return c.Call(daemonrpc.MethodPing, nil, &result)
}

// Status returns daemon status info.
func (c *Client) Status() (*daemonrpc.StatusResult, error) {
	var result daemonrpc.StatusResult
	if err := c.Call(daemonrpc.MethodGetStatus, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
