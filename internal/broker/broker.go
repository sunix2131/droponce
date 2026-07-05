package broker

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	mu                 sync.Mutex
	sessions           map[string]*session
	maxSessionDuration time.Duration
	maxInFlightBytes   int64
}

type session struct {
	ID        string
	ExpiresAt time.Time
	Messages  []Message
	NextSeq   uint64
	Notify    chan struct{}
	Bytes     int64
}

type Message struct {
	Seq       uint64    `json:"seq"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Type      string    `json:"type"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

type CreateSessionRequest struct {
	SessionID string `json:"sessionId"`
	ExpiresAt int64  `json:"expiresAt"`
}

type PostMessageRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
	Body string `json:"body"`
}

type ListMessagesResponse struct {
	Messages []Message `json:"messages"`
}

type Options struct {
	MaxSessionDuration time.Duration
	MaxInFlightBytes   int64
}

func New(options Options) *Server {
	maxDuration := options.MaxSessionDuration
	if maxDuration <= 0 {
		maxDuration = 30 * time.Minute
	}
	maxBytes := options.MaxInFlightBytes
	if maxBytes <= 0 {
		maxBytes = 50 * 1024 * 1024 * 1024
	}
	s := &Server{sessions: map[string]*session{}, maxSessionDuration: maxDuration, maxInFlightBytes: maxBytes}
	go s.cleanupLoop()
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sessions", s.createSession)
	mux.HandleFunc("/v1/sessions/", s.sessionRoutes)
	return mux
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionID == "" {
		http.Error(w, "invalid session", http.StatusBadRequest)
		return
	}
	expiresAt := time.Unix(req.ExpiresAt, 0).UTC()
	now := time.Now().UTC()
	if !expiresAt.After(now) || expiresAt.After(now.Add(s.maxSessionDuration)) {
		expiresAt = now.Add(s.maxSessionDuration)
	}
	s.mu.Lock()
	if existing, ok := s.sessions[req.SessionID]; ok {
		existing.ExpiresAt = expiresAt
	} else {
		s.sessions[req.SessionID] = &session{ID: req.SessionID, ExpiresAt: expiresAt, Notify: make(chan struct{})}
	}
	s.mu.Unlock()
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) sessionRoutes(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "messages" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodPost:
		s.postMessage(w, r, parts[0])
	case http.MethodGet:
		s.listMessages(w, r, parts[0])
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) postMessage(w http.ResponseWriter, r *http.Request, sessionID string) {
	var req PostMessageRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid message", http.StatusBadRequest)
		return
	}
	if req.From == "" || req.To == "" || req.Type == "" || req.Body == "" {
		http.Error(w, "invalid message", http.StatusBadRequest)
		return
	}
	msgSize := int64(len(req.Body))
	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	if !ok || !time.Now().UTC().Before(sess.ExpiresAt) {
		s.mu.Unlock()
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if sess.Bytes+msgSize > s.maxInFlightBytes {
		s.mu.Unlock()
		http.Error(w, "session inflight limit exceeded", http.StatusRequestEntityTooLarge)
		return
	}
	sess.NextSeq++
	msg := Message{Seq: sess.NextSeq, From: req.From, To: req.To, Type: req.Type, Body: req.Body, CreatedAt: time.Now().UTC()}
	sess.Messages = append(sess.Messages, msg)
	sess.Bytes += msgSize
	notify := sess.Notify
	sess.Notify = make(chan struct{})
	close(notify)
	s.mu.Unlock()
	writeJSON(w, msg)
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request, sessionID string) {
	to := r.URL.Query().Get("to")
	after, _ := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
	deadline := time.NewTimer(25 * time.Second)
	defer deadline.Stop()
	for {
		messages, notify, err := s.messages(sessionID, to, after)
		if err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		if len(messages) > 0 {
			writeJSON(w, ListMessagesResponse{Messages: messages})
			return
		}
		select {
		case <-notify:
		case <-deadline.C:
			writeJSON(w, ListMessagesResponse{Messages: nil})
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) messages(sessionID, to string, after uint64) ([]Message, chan struct{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok || !time.Now().UTC().Before(sess.ExpiresAt) {
		return nil, nil, errors.New("session not found")
	}
	out := make([]Message, 0)
	for _, msg := range sess.Messages {
		if msg.Seq > after && (to == "" || msg.To == to) {
			out = append(out, msg)
		}
	}
	return out, sess.Notify, nil
}

func (s *Server) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now().UTC()
		s.mu.Lock()
		for id, sess := range s.sessions {
			if !now.Before(sess.ExpiresAt) {
				delete(s.sessions, id)
				close(sess.Notify)
			}
		}
		s.mu.Unlock()
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
