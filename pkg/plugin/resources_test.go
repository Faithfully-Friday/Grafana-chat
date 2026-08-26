package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestApp builds an App backed by a temp SQLite DB and the given A2A server.
func newTestApp(t *testing.T, a2aURL string) *App {
	t.Helper()
	streaming := true
	return &App{
		settings: settings{BaseURL: a2aURL, StreamingEnabled: &streaming},
		store:    newTestStore(t),
		client:   NewA2AClient(a2aURL, ""),
	}
}

func TestMessageSend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.Method != "message/send" {
			t.Errorf("unexpected method %q", req.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":"1","result":{"kind":"message","role":"agent","contextId":"ctx-9","parts":[{"kind":"text","text":"full reply"}]}}`)
	}))
	defer srv.Close()

	c := NewA2AClient(srv.URL, "secret")
	reply, ctxID, err := c.MessageSend(context.Background(), "hi", "")
	if err != nil {
		t.Fatal(err)
	}
	if reply != "full reply" || ctxID != "ctx-9" {
		t.Fatalf("reply=%q ctx=%q", reply, ctxID)
	}
}

func TestMessageStreamDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		frames := []string{
			`{"jsonrpc":"2.0","id":"1","result":{"kind":"task","contextId":"ctx-1"}}`,
			`{"jsonrpc":"2.0","id":"1","result":{"kind":"artifact-update","contextId":"ctx-1","artifactId":"a1","artifact":{"parts":[{"kind":"text","text":"Hel"}]}}}`,
			`{"jsonrpc":"2.0","id":"1","result":{"kind":"artifact-update","contextId":"ctx-1","artifactId":"a1","append":true,"artifact":{"parts":[{"kind":"text","text":"lo world"}]}}}`,
			`{"jsonrpc":"2.0","id":"1","result":{"kind":"status-update","contextId":"ctx-1","final":true,"status":{"state":"completed"}}}`,
		}
		for _, f := range frames {
			fmt.Fprintf(w, "data: %s\n\n", f)
			fl.Flush()
		}
	}))
	defer srv.Close()

	ch, err := NewA2AClient(srv.URL, "").MessageStream(context.Background(), "hi", "")
	if err != nil {
		t.Fatal(err)
	}
	var deltas, contexts, dones, errs []StreamEvent
	for ev := range ch {
		switch ev.Type {
		case "delta":
			deltas = append(deltas, ev)
		case "context":
			contexts = append(contexts, ev)
		case "done":
			dones = append(dones, ev)
		case "error":
			errs = append(errs, ev)
		}
	}
	if len(errs) != 0 || len(dones) != 1 || len(contexts) == 0 {
		t.Fatalf("errs=%v dones=%d contexts=%d", errs, len(dones), len(contexts))
	}
	got := deltas[0].Content + deltas[1].Content
	if got != "Hello world" {
		t.Fatalf("deltas joined: %q", got)
	}
}

func TestMessageStreamFullReplacementArtifact(t *testing.T) {
	// artifact-update without append carries the full text so far; client must
	// reduce it to deltas so the UI never duplicates content.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{
			`{"jsonrpc":"2.0","id":"1","result":{"kind":"artifact-update","artifactId":"a1","artifact":{"parts":[{"kind":"text","text":"Hel"}]}}}`,
			`{"jsonrpc":"2.0","id":"1","result":{"kind":"artifact-update","artifactId":"a1","artifact":{"parts":[{"kind":"text","text":"Hello"}]}}}`,
			`{"jsonrpc":"2.0","id":"1","result":{"kind":"status-update","final":true,"status":{"state":"completed"}}}`,
		}
		for _, f := range frames {
			fmt.Fprintf(w, "data: %s\n\n", f)
		}
	}))
	defer srv.Close()

	ch, err := NewA2AClient(srv.URL, "").MessageStream(context.Background(), "hi", "")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for ev := range ch {
		if ev.Type == "delta" {
			sb.WriteString(ev.Content)
		}
	}
	if sb.String() != "Hello" {
		t.Fatalf("joined deltas: %q", sb.String())
	}
}

func TestHandleChatNonStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":"1","result":{"kind":"message","role":"agent","contextId":"ctx-5","parts":[{"kind":"text","text":"pong"}]}}`)
	}))
	defer srv.Close()

	app := newTestApp(t, srv.URL)
	conv, err := app.store.CreateConversation(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/conversations/%d/chat", conv.ID),
		strings.NewReader(`{"message":"ping","stream":false}`))
	rec := httptest.NewRecorder()
	app.handleChat(rec, req, conv.ID)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	var resp struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Content != "pong" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}

	msgs, err := app.store.ListMessages(context.Background(), conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("persisted messages: %+v", msgs)
	}
	cid, _ := app.store.ContextID(context.Background(), conv.ID)
	if cid != "ctx-5" {
		t.Fatalf("context id not persisted: %q", cid)
	}
}

func TestHandleChatStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for _, f := range []string{
			`{"jsonrpc":"2.0","id":"1","result":{"kind":"artifact-update","artifactId":"a","contextId":"c2","artifact":{"parts":[{"kind":"text","text":"str"}]}}}`,
			`{"jsonrpc":"2.0","id":"1","result":{"kind":"artifact-update","artifactId":"a","contextId":"c2","append":true,"artifact":{"parts":[{"kind":"text","text":"eamed"}]}}}`,
			`{"jsonrpc":"2.0","id":"1","result":{"kind":"status-update","final":true,"status":{"state":"completed"}}}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", f)
			fl.Flush()
		}
	}))
	defer srv.Close()

	app := newTestApp(t, srv.URL)
	conv, _ := app.store.CreateConversation(context.Background())

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/conversations/%d/chat", conv.ID),
		strings.NewReader(`{"message":"ping","stream":true}`))
	rec := httptest.NewRecorder()
	app.handleChat(rec, req, conv.ID)

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	var deltas int
	var done bool
	sc := bufio.NewScanner(rec.Body)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev StreamEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			t.Fatalf("bad frame %q: %v", line, err)
		}
		if ev.Type == "delta" {
			deltas++
		}
		if ev.Type == "done" {
			done = true
		}
	}
	if deltas != 2 || !done {
		t.Fatalf("deltas=%d done=%v", deltas, done)
	}

	msgs, _ := app.store.ListMessages(context.Background(), conv.ID)
	if len(msgs) != 2 || msgs[1].Content != "streamed" {
		t.Fatalf("persisted messages: %+v", msgs)
	}
}

func TestHandleChatContextRetry(t *testing.T) {
	// First call with a stale contextId fails; the handler must clear it and
	// retry once without a context.
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		params, _ := json.Marshal(req.Params)
		if calls == 1 && strings.Contains(string(params), "stale-ctx") {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":"1","error":{"code":-32602,"message":"unknown contextId"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":"1","result":{"kind":"message","role":"agent","contextId":"fresh-ctx","parts":[{"kind":"text","text":"ok"}]}}`)
	}))
	defer srv.Close()

	app := newTestApp(t, srv.URL)
	conv, _ := app.store.CreateConversation(context.Background())
	if err := app.store.SetContextID(context.Background(), conv.ID, "stale-ctx"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/conversations/%d/chat", conv.ID),
		strings.NewReader(`{"message":"ping","stream":false}`))
	rec := httptest.NewRecorder()
	app.handleChat(rec, req, conv.ID)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	if calls != 2 {
		t.Fatalf("expected retry, calls=%d", calls)
	}
	cid, _ := app.store.ContextID(context.Background(), conv.ID)
	if cid != "fresh-ctx" {
		t.Fatalf("context id: %q", cid)
	}
}

func TestChatRequiresConfig(t *testing.T) {
	app := newTestApp(t, "")
	app.settings.BaseURL = ""
	conv, _ := app.store.CreateConversation(context.Background())
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/conversations/%d/chat", conv.ID),
		strings.NewReader(`{"message":"ping"}`))
	rec := httptest.NewRecorder()
	app.handleChat(rec, req, conv.ID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandleAgentCard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent-card.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"Test Agent","capabilities":{"streaming":true}}`)
	}))
	defer srv.Close()

	app := newTestApp(t, srv.URL)
	req := httptest.NewRequest(http.MethodGet, "/agent-card", nil)
	rec := httptest.NewRecorder()
	app.handleAgentCard(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Test Agent") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
}
