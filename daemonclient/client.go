package daemonclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/floatpane/matcha/daemonrpc"
)

// dialTimeout bounds connecting to the daemon socket.
const dialTimeout = 2 * time.Second

// callTimeout bounds how long a call waits for the daemon's reply. Several
// calls are made on the UI goroutine (Ping while connecting, Subscribe,
// ReloadConfig), so a wedged daemon must not freeze the TUI. Mail operations
// get a generous bound because the daemon talks to the mail server for them.
func callTimeout(method string) time.Duration {
	switch method {
	case daemonrpc.MethodPing, daemonrpc.MethodGetStatus, daemonrpc.MethodGetAccounts,
		daemonrpc.MethodSubscribe, daemonrpc.MethodUnsubscribe, daemonrpc.MethodCancelEmail:
		return 5 * time.Second
	case daemonrpc.MethodReloadConfig, daemonrpc.MethodQueueEmail:
		return time.Minute
	default:
		return 5 * time.Minute
	}
}

// Client connects to the matcha daemon over a Unix domain socket.
type Client struct {
	conn    *daemonrpc.Conn
	nextID  atomic.Uint64
	pending map[uint64]chan *daemonrpc.Response
	mu      sync.Mutex
	events  chan *daemonrpc.Event
	done    chan struct{}
	closeMu sync.Mutex // serializes close(done) between Close and readLoop
}

// Dial connects to the daemon socket.
func Dial() (*Client, error) {
	sockPath := daemonrpc.SocketPath()
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", sockPath)
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

// Call makes a synchronous RPC call to the daemon, giving up after the
// method's callTimeout.
func (c *Client) Call(method string, params interface{}, result interface{}) error {
	return c.CallTimeout(method, params, result, callTimeout(method))
}

// CallTimeout is Call with an explicit timeout (<= 0 waits indefinitely).
// A timed-out call is not retried: the daemon may still execute it.
func (c *Client) CallTimeout(method string, params interface{}, result interface{}, timeout time.Duration) error {
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

	var expired <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		expired = timer.C
	}

	// Wait for response.
	select {
	case <-expired:
		return fmt.Errorf("daemon %s: no reply after %v", method, timeout)
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

// isClosed reports whether the connection has been closed or lost.
func (c *Client) isClosed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// Close closes the connection to the daemon.
func (c *Client) Close() error {
	c.closeMu.Lock()
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	c.closeMu.Unlock()
	return c.conn.Close()
}

// readLoop reads messages from the daemon and dispatches them.
func (c *Client) readLoop() {
	defer close(c.events)

	for {
		msg, err := c.conn.ReceiveMessage()
		if err != nil {
			c.closeMu.Lock()
			select {
			case <-c.done:
			default:
				close(c.done)
			}
			c.closeMu.Unlock()
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
