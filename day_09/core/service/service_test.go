package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-agent-cli/internal/deepseek"
)

func setupTestService(t *testing.T) (*ChatService, func(), string) {
	t.Helper()

	// Create temp directory for agents
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("failed to create agents directory: %v", err)
	}

	// Create a test agent file
	agentFile := filepath.Join(agentsDir, "helper.md")
	agentContent := `# Помощник
Общий помощник для решения любых вопросов.
## Prompt
Ты полезный ассистент. Отвечай кратко и по делу.`
	if err := os.WriteFile(agentFile, []byte(agentContent), 0644); err != nil {
		t.Fatalf("failed to write agent file: %v", err)
	}

	// Create mock DeepSeek API server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate request
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("invalid Authorization header")
		}

		var reqBody struct {
			Model    string             `json:"model"`
			Messages []deepseek.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		// Send successful response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chat-123",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "deepseek-reasoner",
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": "Mocked response",
					},
					"finish_reason": "stop",
					"index":         0,
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		})
	}))

	// Create service with mock API endpoint and in-memory database
	apiKey := "test-api-key"
	service, err := NewChatService(agentsDir, apiKey, ":memory:")
	if err != nil {
		t.Fatalf("failed to create chat service: %v", err)
	}
	// Override DeepSeek client endpoint to point to mock server
	service.client.SetEndpoint(mockServer.URL)

	cleanup := func() {
		mockServer.Close()
		service.Close()
	}
	return service, cleanup, agentsDir
}

func TestNewChatService(t *testing.T) {
	t.Run("creates service with valid agents", func(t *testing.T) {
		tmpDir := t.TempDir()
		agentsDir := filepath.Join(tmpDir, "agents")
		if err := os.MkdirAll(agentsDir, 0755); err != nil {
			t.Fatalf("failed to create agents directory: %v", err)
		}

		// Create at least one agent file
		agentFile := filepath.Join(agentsDir, "test.md")
		if err := os.WriteFile(agentFile, []byte("# Test\n## Prompt\nTest"), 0644); err != nil {
			t.Fatalf("failed to write agent file: %v", err)
		}

		service, err := NewChatService(agentsDir, "test-key", ":memory:")
		if err != nil {
			t.Fatalf("NewChatService() error = %v", err)
		}
		defer service.Close()

		if service == nil {
			t.Fatal("NewChatService() returned nil service")
		}
	})

	t.Run("returns error for empty agents directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		_, err := NewChatService(tmpDir, "test-key", ":memory:")
		if err == nil {
			t.Fatal("NewChatService() expected error for empty directory, got nil")
		}
	})

	t.Run("returns error for non-existent directory", func(t *testing.T) {
		_, err := NewChatService("/non/existent/directory", "test-key", ":memory:")
		if err == nil {
			t.Fatal("NewChatService() expected error for non-existent directory, got nil")
		}
	})
}

func TestChatService_ListAgents(t *testing.T) {
	service, cleanup, _ := setupTestService(t)
	defer cleanup()

	agents, err := service.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("ListAgents() returned %d agents, want 1", len(agents))
	}
	if agents[0].Name != "Помощник" {
		t.Errorf("ListAgents() agent name = %q, want %q", agents[0].Name, "Помощник")
	}
	if agents[0].Description != "Общий помощник для решения любых вопросов." {
		t.Errorf("ListAgents() agent description = %q, want %q", agents[0].Description, "Общий помощник для решения любых вопросов.")
	}
}

func TestChatService_GetChatHistory(t *testing.T) {
	service, cleanup, _ := setupTestService(t)
	defer cleanup()

	// Initially empty history
	messages, err := service.GetChatHistory("Помощник")
	if err != nil {
		t.Fatalf("GetChatHistory() error = %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("GetChatHistory() returned %d messages for new agent, want 0", len(messages))
	}

	// Send a message to create history
	ctx := context.Background()
	_, err = service.SendMessage(ctx, "Помощник", "Hello")
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	// Now should have 2 messages (user + assistant)
	messages, err = service.GetChatHistory("Помощник")
	if err != nil {
		t.Fatalf("GetChatHistory() after send error = %v", err)
	}
	if len(messages) != 2 {
		t.Errorf("GetChatHistory() after send returned %d messages, want 2", len(messages))
	}
	// First message should be user
	if messages[0].Role != "user" {
		t.Errorf("first message role = %q, want %q", messages[0].Role, "user")
	}
	if messages[0].Content != "Hello" {
		t.Errorf("first message content = %q, want %q", messages[0].Content, "Hello")
	}
	// Second message should be assistant
	if messages[1].Role != "assistant" {
		t.Errorf("second message role = %q, want %q", messages[1].Role, "assistant")
	}
	if messages[1].Content != "Mocked response" {
		t.Errorf("second message content = %q, want %q", messages[1].Content, "Mocked response")
	}
}

func TestChatService_SendMessage(t *testing.T) {
	service, cleanup, _ := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("successful message", func(t *testing.T) {
		response, err := service.SendMessage(ctx, "Помощник", "Hello there")
		if err != nil {
			t.Fatalf("SendMessage() error = %v", err)
		}
		if response != "Mocked response" {
			t.Errorf("SendMessage() response = %q, want %q", response, "Mocked response")
		}
	})

	t.Run("agent not found", func(t *testing.T) {
		_, err := service.SendMessage(ctx, "NonExistentAgent", "Hello")
		if err == nil {
			t.Fatal("SendMessage() expected error for non-existent agent, got nil")
		}
		if errStr := err.Error(); !contains(errStr, "not found") {
			t.Errorf("SendMessage() error = %q, want containing 'not found'", errStr)
		}
	})

	t.Run("empty message", func(t *testing.T) {
		// Note: empty message validation is done at handler level, service will still send it
		response, err := service.SendMessage(ctx, "Помощник", "")
		if err != nil {
			t.Fatalf("SendMessage() with empty message error = %v", err)
		}
		if response != "Mocked response" {
			t.Errorf("SendMessage() with empty message response = %q, want %q", response, "Mocked response")
		}
	})
}

func TestChatService_SendMessage_APIFailure(t *testing.T) {
	// Create a mock server that returns an error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Internal server error",
		})
	}))
	defer mockServer.Close()

	// Create temp agents directory
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("failed to create agents directory: %v", err)
	}
	agentFile := filepath.Join(agentsDir, "helper.md")
	if err := os.WriteFile(agentFile, []byte("# Helper\n## Prompt\nTest"), 0644); err != nil {
		t.Fatalf("failed to write agent file: %v", err)
	}

	service, err := NewChatService(agentsDir, "test-key", ":memory:")
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer service.Close()
	service.client.SetEndpoint(mockServer.URL)

	ctx := context.Background()
	_, err = service.SendMessage(ctx, "Helper", "test")
	if err == nil {
		t.Fatal("SendMessage() expected API error, got nil")
	}
}

func TestChatService_Close(t *testing.T) {
	service, _, _ := setupTestService(t)
	// Closing should not panic
	if err := service.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
	// Double close should also not panic
	if err := service.Close(); err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
