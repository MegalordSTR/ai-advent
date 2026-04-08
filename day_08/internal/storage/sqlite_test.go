package storage

import (
	"database/sql"
	"fmt"
	"testing"
)

func TestOpenDB(t *testing.T) {
	t.Run("opens in-memory database", func(t *testing.T) {
		db, err := OpenDB(":memory:")
		if err != nil {
			t.Fatalf("OpenDB() error = %v", err)
		}
		defer db.Close()

		if db == nil {
			t.Fatal("OpenDB() returned nil database")
		}
	})

	t.Run("returns error for invalid path", func(t *testing.T) {
		_, err := OpenDB("/invalid/path/that/does/not/exist.db")
		if err == nil {
			t.Fatal("OpenDB() expected error for invalid path, got nil")
		}
	})
}

func TestInitDB(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer db.Close()

	if err := InitDB(db); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}

	// Verify table exists
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name='messages'")
	if err != nil {
		t.Fatalf("failed to query table existence: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("table 'messages' was not created")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
}

func TestSaveMessage(t *testing.T) {
	tests := []struct {
		name      string
		agentName string
		role      string
		content   string
		wantErr   bool
	}{
		{
			name:      "valid user message",
			agentName: "agent1",
			role:      "user",
			content:   "Hello, world!",
			wantErr:   false,
		},
		{
			name:      "valid assistant message",
			agentName: "agent2",
			role:      "assistant",
			content:   "Hi there!",
			wantErr:   false,
		},
		{
			name:      "valid system message",
			agentName: "agent3",
			role:      "system",
			content:   "You are a helpful assistant.",
			wantErr:   false,
		},
		{
			name:      "empty content",
			agentName: "agent4",
			role:      "user",
			content:   "",
			wantErr:   false,
		},
		{
			name:      "nil database",
			agentName: "test-agent",
			role:      "user",
			content:   "test",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var db *sql.DB
			if tt.name == "nil database" {
				db = nil
			} else {
				var err error
				db, err = OpenDB(":memory:")
				if err != nil {
					t.Fatalf("failed to open test database: %v", err)
				}
				defer db.Close()
				if err := InitDB(db); err != nil {
					t.Fatalf("failed to init database: %v", err)
				}
			}

			err := SaveMessage(db, tt.agentName, tt.role, tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("SaveMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			// Verify message was saved (skip for nil db)
			if db != nil {
				rows, err := db.Query("SELECT role, content FROM messages WHERE agent_name = ?", tt.agentName)
				if err != nil {
					t.Fatalf("failed to query saved message: %v", err)
				}
				defer rows.Close()

				if !rows.Next() {
					t.Fatal("saved message not found")
				}
				var role, content string
				if err := rows.Scan(&role, &content); err != nil {
					t.Fatalf("failed to scan saved message: %v", err)
				}
				if role != tt.role {
					t.Errorf("saved message role = %q, want %q", role, tt.role)
				}
				if content != tt.content {
					t.Errorf("saved message content = %q, want %q", content, tt.content)
				}
				// Ensure only one message for this agent
				if rows.Next() {
					t.Error("unexpected extra message for agent")
				}
			}
		})
	}
}

func TestLoadMessages(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer db.Close()

	if err := InitDB(db); err != nil {
		t.Fatalf("failed to init database: %v", err)
	}

	agentName := "test-agent"

	// Insert test messages
	messages := []struct {
		role    string
		content string
	}{
		{"system", "You are a test assistant."},
		{"user", "First message"},
		{"assistant", "First response"},
		{"user", "Second message"},
		{"assistant", "Second response"},
	}

	for _, msg := range messages {
		if err := SaveMessage(db, agentName, msg.role, msg.content); err != nil {
			t.Fatalf("failed to save test message: %v", err)
		}
	}

	t.Run("load existing messages", func(t *testing.T) {
		loaded, err := LoadMessages(db, agentName)
		if err != nil {
			t.Fatalf("LoadMessages() error = %v", err)
		}
		if len(loaded) != len(messages) {
			t.Fatalf("LoadMessages() returned %d messages, want %d", len(loaded), len(messages))
		}
		for i, msg := range loaded {
			expected := messages[i]
			if msg.Role != expected.role {
				t.Errorf("message %d role = %q, want %q", i, msg.Role, expected.role)
			}
			if msg.Content != expected.content {
				t.Errorf("message %d content = %q, want %q", i, msg.Content, expected.content)
			}
		}
	})

	t.Run("load messages for non-existent agent", func(t *testing.T) {
		loaded, err := LoadMessages(db, "non-existent-agent")
		if err != nil {
			t.Fatalf("LoadMessages() error = %v", err)
		}
		if len(loaded) != 0 {
			t.Errorf("LoadMessages() returned %d messages for non-existent agent, want 0", len(loaded))
		}
	})

	t.Run("nil database", func(t *testing.T) {
		_, err := LoadMessages(nil, agentName)
		if err == nil {
			t.Fatal("LoadMessages() expected error for nil database, got nil")
		}
	})
}

func TestDeleteMessages(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer db.Close()

	if err := InitDB(db); err != nil {
		t.Fatalf("failed to init database: %v", err)
	}

	agentName := "test-agent"

	// Insert some messages
	if err := SaveMessage(db, agentName, "user", "message 1"); err != nil {
		t.Fatalf("failed to save test message: %v", err)
	}
	if err := SaveMessage(db, agentName, "assistant", "response 1"); err != nil {
		t.Fatalf("failed to save test message: %v", err)
	}

	// Verify messages exist
	messages, err := LoadMessages(db, agentName)
	if err != nil {
		t.Fatalf("failed to load messages before delete: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages before delete, got %d", len(messages))
	}

	// Delete messages
	if err := DeleteMessages(db, agentName); err != nil {
		t.Fatalf("DeleteMessages() error = %v", err)
	}

	// Verify messages are gone
	messages, err = LoadMessages(db, agentName)
	if err != nil {
		t.Fatalf("failed to load messages after delete: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("expected 0 messages after delete, got %d", len(messages))
	}

	t.Run("delete from non-existent agent", func(t *testing.T) {
		if err := DeleteMessages(db, "non-existent"); err != nil {
			t.Errorf("DeleteMessages() for non-existent agent should not error, got %v", err)
		}
	})

	t.Run("nil database", func(t *testing.T) {
		if err := DeleteMessages(nil, agentName); err == nil {
			t.Fatal("DeleteMessages() expected error for nil database, got nil")
		}
	})
}

func TestPruneMessages(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer db.Close()

	if err := InitDB(db); err != nil {
		t.Fatalf("failed to init database: %v", err)
	}

	agentName := "test-agent"

	// Insert more messages than limit
	for i := 1; i <= 10; i++ {
		if err := SaveMessage(db, agentName, "user", fmt.Sprintf("message %d", i)); err != nil {
			t.Fatalf("failed to save test message: %v", err)
		}
	}

	// Verify we have 10 messages
	messages, err := LoadMessages(db, agentName)
	if err != nil {
		t.Fatalf("failed to load messages before prune: %v", err)
	}
	if len(messages) != 10 {
		t.Fatalf("expected 10 messages before prune, got %d", len(messages))
	}

	// Prune to keep only 5 latest messages
	if err := PruneMessages(db, agentName, 5); err != nil {
		t.Fatalf("PruneMessages() error = %v", err)
	}

	// Verify we have only 5 messages left
	messages, err = LoadMessages(db, agentName)
	if err != nil {
		t.Fatalf("failed to load messages after prune: %v", err)
	}
	if len(messages) != 5 {
		t.Fatalf("expected 5 messages after prune, got %d", len(messages))
	}

	// Verify the kept messages are the latest ones (IDs 6-10, which correspond to messages 6-10)
	expectedContents := []string{"message 6", "message 7", "message 8", "message 9", "message 10"}
	for i, msg := range messages {
		if msg.Content != expectedContents[i] {
			t.Errorf("message %d content = %q, want %q", i, msg.Content, expectedContents[i])
		}
	}

	t.Run("prune with limit larger than existing messages", func(t *testing.T) {
		// Insert 3 more messages
		for i := 11; i <= 13; i++ {
			if err := SaveMessage(db, agentName, "user", fmt.Sprintf("message %d", i)); err != nil {
				t.Fatalf("failed to save test message: %v", err)
			}
		}

		// Prune with limit 20 (should keep all)
		if err := PruneMessages(db, agentName, 20); err != nil {
			t.Fatalf("PruneMessages() with large limit error = %v", err)
		}

		messages, err := LoadMessages(db, agentName)
		if err != nil {
			t.Fatalf("failed to load messages after large limit prune: %v", err)
		}
		// We had 5 + 3 = 8 messages total
		if len(messages) != 8 {
			t.Errorf("expected 8 messages after large limit prune, got %d", len(messages))
		}
	})

	t.Run("nil database", func(t *testing.T) {
		if err := PruneMessages(nil, agentName, 5); err == nil {
			t.Fatal("PruneMessages() expected error for nil database, got nil")
		}
	})
}
