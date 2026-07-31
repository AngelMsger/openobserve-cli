// Package cdp captures an OpenObserve browser session by driving a
// Chromium-family browser over the Chrome DevTools Protocol. It implements
// webauth.Driver for openobserve-cli, and for o3 on platforms with no native
// WebView shell.
package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"
)

// readLimit raises the WebSocket read limit well above the library default of
// 32 KiB. Storage.getCookies on an instance with many cookies, and any
// Runtime.evaluate returning a large value, exceed the default and would
// otherwise fail the whole connection rather than one call.
const readLimit = 32 << 20 // 32 MiB

// conn is a DevTools Protocol connection: one WebSocket carrying command
// responses (keyed by id) and events (keyed by method), demultiplexed by a
// single reader goroutine.
type conn struct {
	ws     *websocket.Conn
	nextID atomic.Int64

	mu       sync.Mutex
	pending  map[int64]chan rpcResponse
	handlers map[string][]func(sessionID string, params json.RawMessage)
	closed   bool

	readerDone chan struct{}
	readErr    error
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcMessage struct {
	ID        *int64          `json:"id"`
	Method    string          `json:"method"`
	SessionID string          `json:"sessionId"`
	Params    json.RawMessage `json:"params"`
	Result    json.RawMessage `json:"result"`
	Error     *rpcError       `json:"error"`
}

// dial opens a DevTools connection and starts its reader.
func dial(ctx context.Context, wsURL string) (*conn, error) {
	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to devtools: %w", err)
	}
	ws.SetReadLimit(readLimit)
	c := &conn{
		ws:         ws,
		pending:    map[int64]chan rpcResponse{},
		handlers:   map[string][]func(string, json.RawMessage){},
		readerDone: make(chan struct{}),
	}
	go c.read()
	return c, nil
}

// read is the single reader: it routes responses to their waiting caller and
// fans events out to registered handlers.
func (c *conn) read() {
	defer close(c.readerDone)
	for {
		_, data, err := c.ws.Read(context.Background())
		if err != nil {
			c.mu.Lock()
			c.readErr = err
			// Mark the connection dead so no LATER call can register work the
			// reader will never serve. Without this a call issued after a
			// spontaneous disconnect writes to a dead socket and waits forever.
			c.closed = true
			for id, ch := range c.pending {
				close(ch)
				delete(c.pending, id)
			}
			c.mu.Unlock()
			return
		}
		var msg rpcMessage
		if json.Unmarshal(data, &msg) != nil {
			continue // a message we cannot parse is not fatal to the session
		}
		if msg.ID != nil {
			c.mu.Lock()
			ch, ok := c.pending[*msg.ID]
			delete(c.pending, *msg.ID)
			c.mu.Unlock()
			if ok {
				ch <- rpcResponse{Result: msg.Result, Error: msg.Error}
				close(ch)
			}
			continue
		}
		if msg.Method == "" {
			continue
		}
		c.mu.Lock()
		fns := append([]func(string, json.RawMessage){}, c.handlers[msg.Method]...)
		c.mu.Unlock()
		for _, fn := range fns {
			fn(msg.SessionID, msg.Params)
		}
	}
}

// onEvent registers a handler for a DevTools event. Handlers run on the reader
// goroutine, so they must not block or call back into conn.call.
func (c *conn) onEvent(method string, fn func(sessionID string, params json.RawMessage)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[method] = append(c.handlers[method], fn)
}

// call issues a command and decodes its result into out (which may be nil).
// sessionID scopes the command to an attached target; "" addresses the browser.
func (c *conn) call(ctx context.Context, sessionID, method string, params map[string]any, out any) error {
	id := c.nextID.Add(1)
	req := map[string]any{"id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	if sessionID != "" {
		req["sessionId"] = sessionID
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	ch := make(chan rpcResponse, 1)
	c.mu.Lock()
	if c.closed {
		readErr := c.readErr
		c.mu.Unlock()
		if readErr != nil {
			return fmt.Errorf("%s: devtools connection lost: %w", method, readErr)
		}
		return fmt.Errorf("%s: devtools connection closed", method)
	}
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.ws.Write(ctx, websocket.MessageText, body); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("%s: %w", method, err)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return fmt.Errorf("%s: devtools connection lost", method)
		}
		if resp.Error != nil {
			return fmt.Errorf("%s: %s (code %d)", method, resp.Error.Message, resp.Error.Code)
		}
		if out != nil && len(resp.Result) > 0 {
			return json.Unmarshal(resp.Result, out)
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	}
}

// Close tears the connection down; in-flight calls fail rather than hang, and
// the reader goroutine has exited by the time this returns.
func (c *conn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		<-c.readerDone
		return nil
	}
	c.closed = true
	c.mu.Unlock()
	err := c.ws.CloseNow()
	<-c.readerDone
	return err
}
