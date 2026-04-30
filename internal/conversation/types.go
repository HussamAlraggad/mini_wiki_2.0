// Package conversation defines the core data structures for chat messages
// and conversation threads used throughout the application.
package conversation

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Role identifies the sender of a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Metadata holds auxiliary information about a message's generation.
type Metadata struct {
	Model     string `json:"model,omitempty"`
	TokensIn  int    `json:"tokens_in,omitempty"`
	TokensOut int    `json:"tokens_out,omitempty"`
	Duration  int64  `json:"duration_ns,omitempty"`
	Source    string `json:"source,omitempty"` // e.g., "web", "csv", "user"
}

// Message represents a single turn in a conversation.
type Message struct {
	Role      Role      `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Metadata  Metadata  `json:"metadata,omitempty"`
}

// Thread represents a complete conversation session.
type Thread struct {
	ID        string    `json:"id"`
	Messages  []Message `json:"messages"`
	System    string    `json:"system_prompt"`
	MaxTokens int       `json:"max_tokens"`
}

// NewThread creates a new conversation thread with the given system prompt.
func NewThread(system string) *Thread {
	return &Thread{
		ID:        generateID(),
		System:    system,
		MaxTokens: 4096,
	}
}

// Add appends a message to the thread.
func (t *Thread) Add(msg Message) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	t.Messages = append(t.Messages, msg)
	if t.EstimatedTokens() > t.MaxTokens {
		t.Truncate()
	}
}

// Truncate removes oldest non-system messages until the thread fits within MaxTokens.
func (t *Thread) Truncate() {
	for t.EstimatedTokens() > t.MaxTokens && len(t.Messages) > 1 {
		// Keep the first message if it's a system prompt, otherwise drop from front
		if t.Messages[0].Role == RoleSystem && len(t.Messages) > 1 {
			t.Messages = append(t.Messages[:1], t.Messages[2:]...)
		} else {
			t.Messages = t.Messages[1:]
		}
	}
}

// EstimatedTokens returns a rough estimate of token count (4 chars per token).
func (t *Thread) EstimatedTokens() int {
	total := 0
	for _, msg := range t.Messages {
		total += len(msg.Content) / 4
	}
	return total
}



func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use timestamp-based ID if crypto/rand fails (extremely rare)
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
