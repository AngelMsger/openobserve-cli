package cdp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeCDP serves a WebSocket that echoes a canned result for every command and
// can push events on demand.
func fakeCDP(t *testing.T, handle func(id float64, method string, send func(any))) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer c.CloseNow()
		ctx := r.Context()
		var mu sync.Mutex
		send := func(v any) {
			b, _ := json.Marshal(v)
			mu.Lock()
			defer mu.Unlock()
			_ = c.Write(ctx, websocket.MessageText, b)
		}
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			var msg struct {
				ID     float64 `json:"id"`
				Method string  `json:"method"`
			}
			if json.Unmarshal(data, &msg) != nil {
				return
			}
			handle(msg.ID, msg.Method, send)
		}
	}))
}

func wsURL(s *httptest.Server) string { return "ws" + strings.TrimPrefix(s.URL, "http") }

func TestConnCallReturnsResult(t *testing.T) {
	srv := fakeCDP(t, func(id float64, method string, send func(any)) {
		send(map[string]any{"id": id, "result": map[string]any{"targetId": "T1"}})
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := dial(ctx, wsURL(srv))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	var out struct {
		TargetID string `json:"targetId"`
	}
	if err := c.call(ctx, "", "Target.createTarget", nil, &out); err != nil {
		t.Fatalf("call: %v", err)
	}
	if out.TargetID != "T1" {
		t.Fatalf("targetId = %q, want T1", out.TargetID)
	}
}

// Responses must be routed by id, not by arrival order.
func TestConnCallDemultiplexesOutOfOrderResponses(t *testing.T) {
	var mu sync.Mutex
	var pending []float64
	srv := fakeCDP(t, func(id float64, method string, send func(any)) {
		mu.Lock()
		pending = append(pending, id)
		n := len(pending)
		ids := append([]float64(nil), pending...)
		mu.Unlock()
		if n < 2 {
			return // hold the first response back
		}
		// Answer in reverse order.
		for i := len(ids) - 1; i >= 0; i-- {
			send(map[string]any{"id": ids[i], "result": map[string]any{"n": ids[i]}})
		}
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := dial(ctx, wsURL(srv))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	type res struct {
		N float64 `json:"n"`
	}
	var wg sync.WaitGroup
	got := make([]float64, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var r res
			if err := c.call(ctx, "", "Some.method", nil, &r); err != nil {
				t.Errorf("call %d: %v", i, err)
				return
			}
			got[i] = r.N
		}(i)
	}
	wg.Wait()
	if got[0] == got[1] {
		t.Fatalf("both calls got the same id %v; responses were not demultiplexed", got[0])
	}
}

func TestConnCallSurfacesProtocolError(t *testing.T) {
	srv := fakeCDP(t, func(id float64, method string, send func(any)) {
		send(map[string]any{"id": id, "error": map[string]any{"code": -32000, "message": "boom"}})
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := dial(ctx, wsURL(srv))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	err = c.call(ctx, "", "Bad.method", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want it to carry the protocol message", err)
	}
}

func TestConnDispatchesEvents(t *testing.T) {
	srv := fakeCDP(t, func(id float64, method string, send func(any)) {
		send(map[string]any{"id": id, "result": map[string]any{}})
		send(map[string]any{
			"method":    "Runtime.bindingCalled",
			"sessionId": "S1",
			"params":    map[string]any{"payload": `{"email":"ops@example.com"}`},
		})
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := dial(ctx, wsURL(srv))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	seen := make(chan string, 1)
	c.onEvent("Runtime.bindingCalled", func(sessionID string, params json.RawMessage) {
		var p struct {
			Payload string `json:"payload"`
		}
		_ = json.Unmarshal(params, &p)
		seen <- sessionID + "|" + p.Payload
	})

	if err := c.call(ctx, "", "Runtime.enable", nil, nil); err != nil {
		t.Fatalf("call: %v", err)
	}
	select {
	case got := <-seen:
		if got != `S1|{"email":"ops@example.com"}` {
			t.Fatalf("event payload = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("event handler never fired")
	}
}
