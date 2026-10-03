package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
)

// usageTotal is the token total the scripted model reports: one input and
// one output token.
const usageTotal = 2

// ScriptedModel is a local stand-in for a model API. It answers the Anthropic
// Messages API (Claude Code), the OpenAI Responses API (Codex) and OpenAI chat
// completions (opencode) by replaying the script embedded in the prompt, so a
// real harness binary runs real tools and hooks with no credentials, no cost
// and no model choices that could make a check flaky.
type ScriptedModel struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []ModelRequest
	ids      atomic.Int64
}

// ModelRequest is one request a harness sent to the scripted model.
type ModelRequest struct {
	Path string
	Body string
}

// NewScriptedModel starts a scripted model on a loopback port.
func NewScriptedModel() *ScriptedModel {
	m := &ScriptedModel{}
	m.server = httptest.NewServer(http.HandlerFunc(m.serve))

	return m
}

// URL is the base URL of the scripted model, without a /v1 suffix.
func (m *ScriptedModel) URL() string {
	return m.server.URL
}

// Close stops the scripted model.
func (m *ScriptedModel) Close() {
	m.server.Close()
}

// Requests returns every request received so far.
func (m *ScriptedModel) Requests() []ModelRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]ModelRequest(nil), m.requests...)
}

// Saw reports whether any request body contains text, such as a hook reason
// the harness handed back to the model.
func (m *ScriptedModel) Saw(text string) bool {
	for _, request := range m.Requests() {
		if strings.Contains(request.Body, text) {
			return true
		}
	}

	return false
}

func (m *ScriptedModel) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	m.mu.Lock()
	m.requests = append(m.requests, ModelRequest{Path: r.URL.Path, Body: string(body)})
	m.mu.Unlock()

	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/messages"):
		m.serveAnthropic(w, body)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/responses"):
		m.serveResponses(w, body)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/chat/completions"):
		m.serveChat(w, body)
	case r.Method == http.MethodHead || r.Method == http.MethodGet:
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}

func (m *ScriptedModel) nextID(prefix string) string {
	return fmt.Sprintf("%s_%04d", prefix, m.ids.Add(1))
}

// plan decides the reply to a conversation: the calls of the next turn, or the
// final text. A conversation without a script (title generation and other
// side requests) gets a short text answer.
func plan(script Script, found bool, callsMade int) ([]Call, string) {
	if !found {
		return nil, "ok"
	}

	if calls, ok := script.next(callsMade); ok {
		return calls, ""
	}

	return nil, script.final()
}

func writeSSE(w http.ResponseWriter, event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	if event != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", event)
	}

	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

func startSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
}

func writeJSON(w http.ResponseWriter, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func encodeArgs(call Call) string {
	if call.Args == nil {
		return "{}"
	}

	data, err := json.Marshal(call.Args)
	if err != nil {
		return "{}"
	}

	return string(data)
}

// textOf flattens a message content value (a string or a list of blocks with
// text) into one string.
func textOf(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}

	var blocks []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		parts = append(parts, block.Text)
	}

	return strings.Join(parts, "\n")
}
