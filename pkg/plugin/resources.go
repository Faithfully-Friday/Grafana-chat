package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"message": msg})
}

// parseID extracts a numeric path segment after the given prefix,
// e.g. parseID("/conversations/3/messages", "/conversations/") == 3.
func parseID(path, prefix string) (int64, error) {
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.SplitN(rest, "/", 2)[0]
	return strconv.ParseInt(rest, 0, 64)
}

// handleConversations serves GET (list) and POST (create) on /conversations.
func (a *App) handleConversations(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		convs, err := a.store.ListConversations(req.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, convs)
	case http.MethodPost:
		conv, err := a.store.CreateConversation(req.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, conv)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleConversation handles /conversations/{id}[/messages|/chat].
func (a *App) handleConversation(w http.ResponseWriter, req *http.Request) {
	id, err := parseID(req.URL.Path, "/conversations/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	switch {
	case strings.HasSuffix(req.URL.Path, "/messages"):
		a.handleMessages(w, req, id)
	case strings.HasSuffix(req.URL.Path, "/chat"):
		a.handleChat(w, req, id)
	default:
		if req.Method != http.MethodDelete {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := a.store.DeleteConversation(req.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleMessages returns the persisted messages of a conversation.
func (a *App) handleMessages(w http.ResponseWriter, req *http.Request, id int64) {
	if req.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	msgs, err := a.store.ListMessages(req.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

// handleChat invokes the A2A agent with a new user message, either streaming
// the reply as SSE frames or returning it in one JSON response.
func (a *App) handleChat(w http.ResponseWriter, req *http.Request, id int64) {
	if req.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if a.settings.BaseURL == "" {
		writeError(w, http.StatusBadRequest, "A2A endpoint not configured")
		return
	}
	var body struct {
		Message string `json:"message"`
		Stream  bool   `json:"stream"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || strings.TrimSpace(body.Message) == "" {
		writeError(w, http.StatusBadRequest, "a non-empty message is required")
		return
	}
	ctx := req.Context()

	// Persist the user message and auto-title the conversation from it.
	if err := a.store.AddMessage(ctx, id, "user", body.Message); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	title := body.Message
	if len([]rune(title)) > 40 {
		title = string([]rune(title)[:40]) + "…"
	}
	_ = a.store.SetTitle(ctx, id, title)

	contextID, err := a.store.ContextID(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if body.Stream && a.settings.streamingOn() {
		a.streamChat(w, ctx, id, body.Message, contextID)
		return
	}

	reply, newContextID, err := a.client.MessageSend(ctx, body.Message, contextID)
	if err != nil && contextID != "" && IsContextUnknown(err) {
		// Self-healing: drop the expired context and retry once fresh.
		_ = a.store.SetContextID(ctx, id, "")
		reply, newContextID, err = a.client.MessageSend(ctx, body.Message, "")
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if newContextID != "" && newContextID != contextID {
		_ = a.store.SetContextID(ctx, id, newContextID)
	}
	if err := a.store.AddMessage(ctx, id, "assistant", reply); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": reply})
}

// streamChat runs message/stream against the A2A server and forwards
// normalized events to the browser as SSE frames, flushing after each one.
func (a *App) streamChat(w http.ResponseWriter, ctx context.Context, id int64, message, contextID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	sendFrame := func(ev StreamEvent) {
		payload, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
	}

	events, err := a.client.MessageStream(ctx, message, contextID)
	if err != nil && contextID != "" && IsContextUnknown(err) {
		_ = a.store.SetContextID(ctx, id, "")
		contextID = ""
		events, err = a.client.MessageStream(ctx, message, "")
	}
	if err != nil {
		sendFrame(StreamEvent{Type: "error", Message: err.Error()})
		return
	}

	var reply strings.Builder
	for ev := range events {
		switch ev.Type {
		case "delta":
			reply.WriteString(ev.Content)
		case "context":
			if ev.ContextID != contextID {
				_ = a.store.SetContextID(ctx, id, ev.ContextID)
			}
			continue // context frames are backend-internal, not shown in the UI
		}
		sendFrame(ev)
	}

	if text := reply.String(); text != "" {
		if err := a.store.AddMessage(ctx, id, "assistant", text); err != nil {
			sendFrame(StreamEvent{Type: "error", Message: "failed to persist reply: " + err.Error()})
		}
	}
}

// handleAgentCard proxies the agent card so the config page can validate
// connectivity and credentials without exposing them to the browser.
func (a *App) handleAgentCard(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if a.settings.BaseURL == "" {
		writeError(w, http.StatusBadRequest, "A2A endpoint not configured")
		return
	}
	card, err := a.client.FetchAgentCard(req.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(card)
}

// registerRoutes maps resource paths to handlers.
func (a *App) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/conversations", a.handleConversations)
	mux.HandleFunc("/conversations/", a.handleConversation)
	mux.HandleFunc("/agent-card", a.handleAgentCard)
}
