package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Session represents a single conversation session.
type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}

// Message is a single entry in a session conversation.
type Message struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// SessionStore manages session persistence in the project's .wiki/ directory.
type SessionStore struct {
	mu       sync.RWMutex
	dir      string
	sessions map[string]*Session
	order    []string // session IDs in reverse chronological order
}

// NewSessionStore creates a session store rooted at the given project directory.
func NewSessionStore(projectDir string) *SessionStore {
	ss := &SessionStore{
		dir:      filepath.Join(projectDir, ".wiki", "sessions"),
		sessions: make(map[string]*Session),
	}
	os.MkdirAll(ss.dir, 0700)
	ss.loadAll()
	return ss
}

func (ss *SessionStore) loadAll() {
	entries, err := os.ReadDir(ss.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ss.dir, e.Name()))
		if err != nil {
			continue
		}
		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		ss.sessions[s.ID] = &s
		ss.order = append([]string{s.ID}, ss.order...)
	}
}

func (ss *SessionStore) path(id string) string {
	return filepath.Join(ss.dir, id+".json")
}

// Create creates a new session with the given title.
func (ss *SessionStore) Create(title string) *Session {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	id := time.Now().Format("20060102150405")
	s := &Session{
		ID:        id,
		Title:     title,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	ss.sessions[id] = s
	ss.order = append([]string{id}, ss.order...)

	data, _ := json.Marshal(s)
	os.WriteFile(ss.path(id), data, 0600)
	return s
}

// Get retrieves a session by ID.
func (ss *SessionStore) Get(id string) (*Session, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	s, ok := ss.sessions[id]
	return s, ok
}

// AppendMessage adds a message to a session and persists.
func (ss *SessionStore) AppendMessage(sessionID, role, content string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	s, ok := ss.sessions[sessionID]
	if !ok {
		return
	}

	s.Messages = append(s.Messages, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now().Unix(),
	})
	s.UpdatedAt = time.Now()

	data, _ := json.Marshal(s)
	os.WriteFile(ss.path(sessionID), data, 0600)
}

// List returns all sessions sorted by most recent first.
func (ss *SessionStore) List() []*Session {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	result := make([]*Session, 0, len(ss.order))
	for _, id := range ss.order {
		if s, ok := ss.sessions[id]; ok {
			result = append(result, s)
		}
	}
	return result
}

// Latest returns the most recent session, or creates one if none exist.
func (ss *SessionStore) Latest() *Session {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if len(ss.order) > 0 {
		if s, ok := ss.sessions[ss.order[0]]; ok {
			return s
		}
	}
	// No existing session, return nil (caller should create one)
	return nil
}
