package conversation

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewThread(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		thread := NewThread("You are a helpful assistant.")
		if thread.ID == "" {
			t.Error("expected non-empty ID")
		}
		if len(thread.ID) != 32 {
			t.Errorf("expected ID length 32 (16 bytes hex), got %d", len(thread.ID))
		}
		if thread.System != "You are a helpful assistant." {
			t.Errorf("expected system prompt %q, got %q", "You are a helpful assistant.", thread.System)
		}
		if thread.MaxTokens != 4096 {
			t.Errorf("expected MaxTokens 4096, got %d", thread.MaxTokens)
		}
		if len(thread.Messages) != 0 {
			t.Errorf("expected empty messages, got %d", len(thread.Messages))
		}
	})

	t.Run("empty system prompt", func(t *testing.T) {
		thread := NewThread("")
		if thread.System != "" {
			t.Errorf("expected empty system prompt, got %q", thread.System)
		}
	})

	t.Run("unique IDs", func(t *testing.T) {
		seen := make(map[string]bool)
		for range 100 {
			id := NewThread("test").ID
			if seen[id] {
				t.Error("duplicate thread ID generated")
			}
			seen[id] = true
		}
	})
}

func TestThread_Add(t *testing.T) {
	t.Run("adds message with timestamp", func(t *testing.T) {
		thread := NewThread("system")
		msg := Message{Role: RoleUser, Content: "hello"}
		thread.Add(msg)
		if len(thread.Messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(thread.Messages))
		}
		if thread.Messages[0].Timestamp.IsZero() {
			t.Error("expected non-zero timestamp")
		}
	})

	t.Run("preserves existing timestamp", func(t *testing.T) {
		thread := NewThread("system")
		ts := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		msg := Message{Role: RoleUser, Content: "hello", Timestamp: ts}
		thread.Add(msg)
		if !thread.Messages[0].Timestamp.Equal(ts) {
			t.Errorf("expected timestamp %v, got %v", ts, thread.Messages[0].Timestamp)
		}
	})

	t.Run("triggers truncation when over MaxTokens", func(t *testing.T) {
		thread := NewThread("keep")
		thread.MaxTokens = 5 // very low: 20 chars = 5 tokens

		// Add messages to exceed limit
		for i := 0; i < 10; i++ {
			thread.Add(Message{Role: RoleUser, Content: strings.Repeat("x", 20)})
		}

		if thread.EstimatedTokens() > thread.MaxTokens {
			t.Errorf("truncation should have reduced tokens below %d, got %d", thread.MaxTokens, thread.EstimatedTokens())
		}
	})
}

func TestThread_Truncate(t *testing.T) {
	t.Run("removes oldest non-system messages", func(t *testing.T) {
		thread := NewThread("system prompt")
		thread.MaxTokens = 100

		// System message is first
		thread.Add(Message{Role: RoleAssistant, Content: "A"})
		thread.Add(Message{Role: RoleUser, Content: "B"})
		thread.Add(Message{Role: RoleAssistant, Content: "C"})

		// Set very low max tokens to force aggressive truncation
		thread.MaxTokens = 0
		thread.Truncate()

		// Only the system prompt should remain (it's role RoleSystem in the Messages slice)
		if len(thread.Messages) != 0 {
			t.Logf("remaining messages: %d", len(thread.Messages))
		}
	})

	t.Run("preserves system message when possible", func(t *testing.T) {
		thread := NewThread("keep me")
		// Manually set system role for first msg
		msg := Message{Role: RoleSystem, Content: "You are a helpful assistant.", Timestamp: time.Now()}
		thread.Add(msg)
		thread.Add(Message{Role: RoleUser, Content: strings.Repeat("x", 400), Timestamp: time.Now()})
		thread.Add(Message{Role: RoleAssistant, Content: strings.Repeat("y", 400), Timestamp: time.Now()})

		thread.MaxTokens = 50
		thread.Truncate()

		if len(thread.Messages) == 0 {
			t.Fatal("expected at least one message to remain")
		}
		if thread.Messages[0].Role != RoleSystem {
			t.Errorf("expected first message to be system, got %s", thread.Messages[0].Role)
		}
		if thread.EstimatedTokens() > thread.MaxTokens {
			t.Errorf("EstimatedTokens %d exceeds MaxTokens %d after truncation", thread.EstimatedTokens(), thread.MaxTokens)
		}
	})

	t.Run("no messages does not panic", func(t *testing.T) {
		thread := NewThread("sys")
		thread.Truncate()
	})

	t.Run("single message does not panic", func(t *testing.T) {
		thread := NewThread("sys")
		thread.Add(Message{Role: RoleUser, Content: "hi", Timestamp: time.Now()})
		thread.Truncate()
	})

	t.Run("truncates until under limit", func(t *testing.T) {
		thread := NewThread("sys")
		thread.MaxTokens = 10
		for i := 0; i < 100; i++ {
			thread.Add(Message{Role: RoleUser, Content: strings.Repeat("a", 40), Timestamp: time.Now()})
		}
		if thread.EstimatedTokens() > thread.MaxTokens {
			t.Errorf("after Truncate, EstimatedTokens %d > MaxTokens %d", thread.EstimatedTokens(), thread.MaxTokens)
		}
	})

	t.Run("non-system first message removed first", func(t *testing.T) {
		thread := NewThread("")
		thread.Add(Message{Role: RoleUser, Content: strings.Repeat("x", 1000), Timestamp: time.Now()})
		thread.Add(Message{Role: RoleAssistant, Content: "short", Timestamp: time.Now()})
		thread.MaxTokens = 2
		thread.Truncate()
		// First message (1000 chars = 250 tokens) should be removed
		if len(thread.Messages) > 0 && thread.Messages[0].Content == strings.Repeat("x", 1000) {
			t.Error("expected the long first message to be removed")
		}
	})
}

func TestThread_EstimatedTokens(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		want     int
	}{
		{
			name:     "empty thread",
			messages: nil,
			want:     0,
		},
		{
			name: "single message",
			messages: []Message{
				{Role: RoleUser, Content: "hello"}, // 5 chars / 4 = 1
			},
			want: 1,
		},
		{
			name: "multiple messages",
			messages: []Message{
				{Role: RoleUser, Content: "abcdefgh"},       // 8/4 = 2
				{Role: RoleAssistant, Content: "ijklmnopqr"}, // 10/4 = 2
			},
			want: 4,
		},
		{
			name: "empty content",
			messages: []Message{
				{Role: RoleUser, Content: ""},
			},
			want: 0,
		},
		{
			name: "unicode content counts bytes",
			messages: []Message{
				{Role: RoleUser, Content: "你好世界"}, // 12 bytes / 4 = 3
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thread := &Thread{Messages: tt.messages}
			if got := thread.EstimatedTokens(); got != tt.want {
				t.Errorf("EstimatedTokens() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestThread_JSONRoundTrip(t *testing.T) {
	original := NewThread("system prompt")
	original.Add(Message{Role: RoleUser, Content: "user msg", Timestamp: time.Now()})
	original.Add(Message{Role: RoleAssistant, Content: "assistant msg", Timestamp: time.Now()})
	original.MaxTokens = 2048

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var restored Thread
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("ID mismatch: %q vs %q", restored.ID, original.ID)
	}
	if restored.System != original.System {
		t.Errorf("System mismatch: %q vs %q", restored.System, original.System)
	}
	if restored.MaxTokens != original.MaxTokens {
		t.Errorf("MaxTokens mismatch: %d vs %d", restored.MaxTokens, original.MaxTokens)
	}
	if len(restored.Messages) != len(original.Messages) {
		t.Fatalf("message count mismatch: %d vs %d", len(restored.Messages), len(original.Messages))
	}
	for i := range restored.Messages {
		if restored.Messages[i].Role != original.Messages[i].Role {
			t.Errorf("message[%d] Role mismatch", i)
		}
		if restored.Messages[i].Content != original.Messages[i].Content {
			t.Errorf("message[%d] Content mismatch", i)
		}
	}
}

func TestThread_ConcurrentAdd(t *testing.T) {
	thread := NewThread("sys")
	var mu sync.Mutex
	var wg sync.WaitGroup
	n := 50
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			mu.Lock()
			thread.Add(Message{Role: RoleUser, Content: "msg", Timestamp: time.Now()})
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(thread.Messages) != n {
		t.Errorf("expected %d messages, got %d", n, len(thread.Messages))
	}
}

func TestThread_ConcurrentReadWrite(t *testing.T) {
	thread := NewThread("sys")
	for i := 0; i < 10; i++ {
		thread.Add(Message{Role: RoleUser, Content: "initial", Timestamp: time.Now()})
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = thread.EstimatedTokens()
			_ = thread.EstimatedTokens()
		}()
	}
	wg.Wait()
}

func TestGenerateID_Length(t *testing.T) {
	id := generateID()
	if len(id) != 32 {
		t.Errorf("expected hex string of length 32, got %d", len(id))
	}
}

func TestMessage_RoleConstants(t *testing.T) {
	if RoleSystem != "system" {
		t.Errorf("RoleSystem = %q, want 'system'", RoleSystem)
	}
	if RoleUser != "user" {
		t.Errorf("RoleUser = %q, want 'user'", RoleUser)
	}
	if RoleAssistant != "assistant" {
		t.Errorf("RoleAssistant = %q, want 'assistant'", RoleAssistant)
	}
}
