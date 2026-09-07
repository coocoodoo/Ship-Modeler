// Package bridge provides an authenticated, loopback-only command mailbox.
// The HTTP goroutines never touch the document or GPU; the app drains jobs on
// its window thread between frames.
package bridge

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

type Request struct {
	ID   string          `json:"id"`
	Type string          `json:"type"` // state, capture, commands, disconnect
	Ops  json.RawMessage `json:"ops,omitempty"`
}
type Job struct {
	Request Request         `json:"request"`
	Status  string          `json:"status"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}
type Session struct {
	Protocol   int    `json:"protocol"`
	PID        int    `json:"pid"`
	URL        string `json:"url"`
	Token      string `json:"token"`
	Executable string `json:"executable"`
}
type Server struct {
	Session Session
	path    string
	http    *http.Server
	queue   chan string
	mu      sync.Mutex
	jobs    map[string]*Job
	closed  bool
}

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func Start(dir string, tools any) (*Server, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Server, error) { listener.Close(); return nil, err }
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fail(err)
	}
	exe, _ := os.Executable()
	s := &Server{Session: Session{1, os.Getpid(), "http://" + listener.Addr().String(), hex.EncodeToString(secret), exe}, queue: make(chan string, 16), jobs: make(map[string]*Job)}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fail(err)
	}
	s.path = filepath.Join(dir, fmt.Sprintf("session-%d.json", s.Session.PID))
	data, _ := json.MarshalIndent(s.Session, "", "  ")
	if err := os.WriteFile(s.path, data, 0600); err != nil {
		return fail(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"protocol": 1, "pid": s.Session.PID})
	})
	mux.HandleFunc("GET /v1/tools", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, tools) })
	mux.HandleFunc("POST /v1/jobs", s.submit)
	mux.HandleFunc("GET /v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		job, ok := s.jobs[r.PathValue("id")]
		if !ok {
			reply(w, 404, map[string]string{"error": "unknown job"})
			return
		}
		reply(w, 200, job)
	})
	s.http = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Header.Get("Origin") != "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.Session.Token)) != 1 {
			reply(w, 403, map[string]string{"error": "local session authentication required"})
			return
		}
		mux.ServeHTTP(w, r)
	}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	go s.http.Serve(listener)
	return s, nil
}

func reply(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	var req Request
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		reply(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		reply(w, 400, map[string]string{"error": "one JSON request required"})
		return
	}
	validType := req.Type == "state" || req.Type == "capture" || req.Type == "commands" || req.Type == "disconnect"
	if !validID.MatchString(req.ID) || !validType {
		reply(w, 400, map[string]string{"error": "valid id and request type required"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		reply(w, 503, map[string]string{"error": "connection stopped"})
		return
	}
	if old, ok := s.jobs[req.ID]; ok {
		if old.Request.Type != req.Type || string(old.Request.Ops) != string(req.Ops) {
			reply(w, 409, map[string]string{"error": "job id already used for a different request"})
			return
		}
		reply(w, 200, old)
		return
	}
	// Keep every accepted ID until disconnect, so an uncertain retry can never
	// replay an edit. Reconnect starts a new session when this limit is reached.
	if len(s.jobs) >= 4096 {
		reply(w, 429, map[string]string{"error": "session job limit reached; reconnect"})
		return
	}
	job := &Job{Request: req, Status: "queued"}
	select {
	case s.queue <- req.ID:
		s.jobs[req.ID] = job
		reply(w, 202, job)
	default:
		reply(w, 429, map[string]string{"error": "command queue full"})
	}
}

func (s *Server) Next() (Request, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Request{}, false
	}
	select {
	case id := <-s.queue:
		job := s.jobs[id]
		job.Status = "running"
		return job.Request, true
	default:
		return Request{}, false
	}
}
func (s *Server) Complete(id string, result any, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job := s.jobs[id]; job != nil {
		var encodeErr error
		job.Result, encodeErr = json.Marshal(result)
		if encodeErr != nil {
			err = fmt.Errorf("encode response: %w", encodeErr)
		}
		job.Status = "done"
		if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
		}
	}
}
func (s *Server) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	for _, job := range s.jobs {
		if job.Status == "queued" {
			job.Status = "cancelled"
		}
	}
	s.mu.Unlock()
	_ = s.http.Close()
	_ = os.Remove(s.path)
}
