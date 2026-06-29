package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

//go:embed web
var webFS embed.FS

// EventKind tags each thing that happens during a streamed turn.
type EventKind string

const (
	EventToken      EventKind = "token"       // a chunk of the assistant's text
	EventToolCall   EventKind = "tool_call"   // about to run a tool
	EventToolResult EventKind = "tool_result" // a tool finished
	EventDone       EventKind = "done"        // the turn is complete
	EventError      EventKind = "error"       // the turn failed
)

// Event is one item in the stream sent to the browser, one JSON object per line.
type Event struct {
	Kind   EventKind `json:"kind"`
	Text   string    `json:"text,omitempty"`   // token text, or error message
	Tool   string    `json:"tool,omitempty"`   // tool name for tool_* events
	Args   string    `json:"args,omitempty"`   // tool arguments, as JSON
	Result string    `json:"result,omitempty"` // tool result text
}

// server wraps one shared Agent. With a single conversation we serialize turns
// with a mutex, protecting agent.history from a double-click race.
type server struct {
	agent *Agent
	mu    sync.Mutex
}

func (s *server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Message) == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")

	// One turn at a time; guards the shared agent.history.
	s.mu.Lock()
	defer s.mu.Unlock()

	enc := json.NewEncoder(w) // Encode appends '\n', giving us NDJSON for free.
	emit := func(ev Event) {
		_ = enc.Encode(ev)
		flusher.Flush()
	}

	// r.Context() is cancelled if the user closes the tab, which stops the turn.
	if err := s.agent.AskStream(r.Context(), body.Message, emit); err != nil {
		emit(Event{Kind: EventError, Text: err.Error()})
	}
}

func (s *server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	s.agent.Reset()
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/reset", s.handleReset)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, err := webFS.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "ui missing", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
	return mux
}

// serveHTTP runs the web chat. It binds localhost by default: read_file acts for
// anyone who can reach the port, so we don't expose it on the network.
func serveHTTP(agent *Agent, addr string) error {
	s := &server{agent: agent}
	fmt.Printf("Web chat on http://%s — open it in your browser. Ctrl-C to quit.\n", addr)
	return http.ListenAndServe(addr, s.routes())
}
