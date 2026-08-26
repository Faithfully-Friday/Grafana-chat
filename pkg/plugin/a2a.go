package plugin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// StreamEvent is a normalized event the backend forwards to the browser,
// regardless of which A2A event kind produced it.
type StreamEvent struct {
	Type      string `json:"type"` // "delta" | "context" | "done" | "error"
	Content   string `json:"content,omitempty"`
	ContextID string `json:"contextId,omitempty"`
	Message   string `json:"message,omitempty"`
}

// A2AClient talks to an A2A (Agent2Agent) protocol server over JSON-RPC/HTTP.
type A2AClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewA2AClient builds a client for the given A2A endpoint URL. The base URL is
// the JSON-RPC endpoint itself and is used as-is for JSON-RPC calls; agent
// cards are resolved relative to it.
func NewA2AClient(baseURL, apiKey string) *A2AClient {
	return &A2AClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{},
	}
}

type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// RPCError is a JSON-RPC error returned by the A2A server.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("A2A error %d: %s", e.Code, e.Message) }

type a2aPart struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type a2aMessage struct {
	Role  string    `json:"role"`
	Parts []a2aPart `json:"parts"`
}

type a2aParams struct {
	Message a2aOutgoingMessage `json:"message"`
}

type a2aOutgoingMessage struct {
	Role      string    `json:"role"`
	MessageID string    `json:"messageId"`
	Parts     []a2aPart `json:"parts"`
	ContextID string    `json:"contextId,omitempty"`
}

// rpcResponse is a JSON-RPC response whose result is one of the A2A streaming
// payloads (Task, TaskStatusUpdateEvent, TaskArtifactUpdateEvent) or, for
// message/send, a Task or Message.
type rpcResponse struct {
	Result *a2aEvent `json:"result"`
	Error  *RPCError `json:"error"`
}

// a2aEvent is a superset of the A2A event payloads, distinguished by "kind":
// "task" | "status-update" | "artifact-update" | "message".
type a2aEvent struct {
	Kind      string `json:"kind"`
	ContextID string `json:"contextId"`
	Final     bool   `json:"final"`
	// status-update
	Status *struct {
		State   string      `json:"state"`
		Message *a2aMessage `json:"message"`
	} `json:"status"`
	// artifact-update
	ArtifactID string `json:"artifactId"`
	Artifact   *struct {
		Parts []a2aPart `json:"parts"`
	} `json:"artifact"`
	Append bool `json:"append"`
	// message (message/send result)
	Role  string    `json:"role"`
	Parts []a2aPart `json:"parts"`
	// task
	History []a2aMessage `json:"history"`
}

func (c *A2AClient) newRequest(ctx context.Context, method string, params a2aParams) (*http.Request, error) {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: uuid.NewString(), Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	return req, nil
}

func chatParams(message, contextID string) a2aParams {
	return a2aParams{Message: a2aOutgoingMessage{
		Role:      "user",
		MessageID: uuid.NewString(),
		Parts:     []a2aPart{{Kind: "text", Text: message}},
		ContextID: contextID,
	}}
}

func textParts(parts []a2aPart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Kind == "text" || p.Kind == "" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// IsContextUnknown reports whether an error looks like an unknown/expired
// contextId, in which case the caller should retry without one.
func IsContextUnknown(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context") || strings.Contains(msg, "404")
}

// MessageStream sends message/stream and returns a channel of normalized
// events. The channel closes after a terminal "done" or "error" event.
func (c *A2AClient) MessageStream(ctx context.Context, message, contextID string) (<-chan StreamEvent, error) {
	req, err := c.newRequest(ctx, "message/stream", chatParams(message, contextID))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("A2A stream failed with HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	ch := make(chan StreamEvent, 16)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		// artifactId -> text seen so far, so artifact-update events that carry
		// the full artifact text (append omitted/false) can be reduced to deltas.
		artifacts := map[string]string{}

		send := func(ev StreamEvent) bool {
			select {
			case ch <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}

		handleFrame := func(payload string) bool {
			var rpcResp rpcResponse
			if err := json.Unmarshal([]byte(payload), &rpcResp); err != nil {
				return true // skip malformed frames
			}
			if rpcResp.Error != nil {
				send(StreamEvent{Type: "error", Message: rpcResp.Error.Error()})
				return false
			}
			ev := rpcResp.Result
			if ev == nil {
				return true
			}
			if ev.ContextID != "" && !send(StreamEvent{Type: "context", ContextID: ev.ContextID}) {
				return false
			}
			switch ev.Kind {
			case "artifact-update":
				if ev.Artifact == nil {
					return true
				}
				text := textParts(ev.Artifact.Parts)
				prev := artifacts[ev.ArtifactID]
				var delta string
				if ev.Append || prev == "" || strings.HasPrefix(text, prev) {
					delta = strings.TrimPrefix(text, prev)
				} else {
					delta = text // replacement that doesn't extend prev: send whole
				}
				artifacts[ev.ArtifactID] = prev + delta
				if delta != "" && !send(StreamEvent{Type: "delta", Content: delta}) {
					return false
				}
			case "status-update":
				if ev.Final || (ev.Status != nil && isTerminalState(ev.Status.State)) {
					// If no artifacts were streamed, the final status message
					// holds the whole reply; emit it as a single delta.
					if len(artifacts) == 0 && ev.Status != nil && ev.Status.Message != nil {
						if text := textParts(ev.Status.Message.Parts); text != "" {
							if !send(StreamEvent{Type: "delta", Content: text}) {
								return false
							}
						}
					}
					send(StreamEvent{Type: "done"})
					return false
				}
			case "task", "message":
				// Initial task snapshot / message echo; nothing incremental.
			}
			return true
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var dataLines []string
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if len(dataLines) > 0 {
					if !handleFrame(strings.Join(dataLines, "\n")) {
						return
					}
					dataLines = dataLines[:0]
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			send(StreamEvent{Type: "error", Message: err.Error()})
			return
		}
		send(StreamEvent{Type: "done"}) // stream ended without a terminal event
	}()
	return ch, nil
}

func isTerminalState(state string) bool {
	switch state {
	case "completed", "failed", "canceled", "rejected":
		return true
	}
	return false
}

// MessageSend performs a blocking message/send call and returns the full reply.
func (c *A2AClient) MessageSend(ctx context.Context, message, contextID string) (reply, newContextID string, err error) {
	req, err := c.newRequest(ctx, "message/send", chatParams(message, contextID))
	if err != nil {
		return "", "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("A2A send failed with HTTP %d: %s", resp.StatusCode, truncate(string(body), 512))
	}
	var rpcResp rpcResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return "", "", fmt.Errorf("decode A2A response: %w", err)
	}
	if rpcResp.Error != nil {
		return "", "", rpcResp.Error
	}
	ev := rpcResp.Result
	if ev == nil {
		return "", "", fmt.Errorf("empty A2A response")
	}
	newContextID = ev.ContextID
	// Result is either a Message (kind "message", parts on the object) or a
	// Task (reply in status message or latest agent history entry).
	if t := textParts(ev.Parts); t != "" {
		return t, newContextID, nil
	}
	if ev.Status != nil && ev.Status.Message != nil {
		if t := textParts(ev.Status.Message.Parts); t != "" {
			return t, newContextID, nil
		}
	}
	for i := len(ev.History) - 1; i >= 0; i-- {
		if ev.History[i].Role == "agent" {
			if t := textParts(ev.History[i].Parts); t != "" {
				return t, newContextID, nil
			}
		}
	}
	return "", newContextID, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// FetchAgentCard retrieves the agent card used for connectivity checks,
// trying the well-known locations defined by the A2A spec.
func (c *A2AClient) FetchAgentCard(ctx context.Context) (json.RawMessage, error) {
	for _, path := range []string{"/.well-known/agent-card.json", "/.well-known/agent.json"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
		if err != nil {
			return nil, err
		}
		if c.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusOK && json.Valid(body) {
			return json.RawMessage(body), nil
		}
	}
	return nil, fmt.Errorf("no agent card found at %s", c.baseURL)
}
