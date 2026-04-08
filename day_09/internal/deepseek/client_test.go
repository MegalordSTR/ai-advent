package deepseek

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeepSeekClient_Chat_Success(t *testing.T) {
	// Arrange
	responseContent := "Hello, this is a test response"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate request
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("invalid Authorization header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("invalid Content-Type header")
		}

		var reqBody struct {
			Model    string    `json:"model"`
			Messages []Message `json:"messages"`
			Stream   bool      `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
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
						"content": responseContent,
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
	defer ts.Close()

	client := NewDeepSeekClient("test-api-key")
	client.endpoint = ts.URL // override endpoint to test server

	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello"},
	}

	// Act
	ctx := context.Background()
	response, err := client.Chat(ctx, messages)

	// Assert
	if err != nil {
		t.Fatalf("Chat() returned unexpected error: %v", err)
	}
	if response != responseContent {
		t.Errorf("Chat() = %q, want %q", response, responseContent)
	}
}

func TestDeepSeekClient_Chat_APIError(t *testing.T) {
	// Arrange
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Invalid API key",
		})
	}))
	defer ts.Close()

	client := NewDeepSeekClient("invalid-key")
	client.endpoint = ts.URL

	messages := []Message{{Role: "user", Content: "test"}}

	// Act
	ctx := context.Background()
	response, err := client.Chat(ctx, messages)

	// Assert
	if err == nil {
		t.Fatal("Chat() expected error, got nil")
	}
	if response != "" {
		t.Errorf("Chat() expected empty response, got %q", response)
	}
	// Check that error contains status code
	expectedErrSubstring := "401"
	if errStr := err.Error(); !strings.Contains(errStr, expectedErrSubstring) {
		t.Errorf("Chat() error = %q, want containing %q", errStr, expectedErrSubstring)
	}
}

func TestDeepSeekClient_Chat_InvalidJSONResponse(t *testing.T) {
	// Arrange
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json"))
	}))
	defer ts.Close()

	client := NewDeepSeekClient("test-key")
	client.endpoint = ts.URL

	messages := []Message{{Role: "user", Content: "test"}}

	// Act
	ctx := context.Background()
	response, err := client.Chat(ctx, messages)

	// Assert
	if err == nil {
		t.Fatal("Chat() expected error for invalid JSON, got nil")
	}
	if response != "" {
		t.Errorf("Chat() expected empty response, got %q", response)
	}
}

func TestDeepSeekClient_Chat_EmptyChoices(t *testing.T) {
	// Arrange
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chat-123",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "deepseek-reasoner",
			"choices": []map[string]interface{}{}, // empty choices
			"usage": map[string]int{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		})
	}))
	defer ts.Close()

	client := NewDeepSeekClient("test-key")
	client.endpoint = ts.URL

	messages := []Message{{Role: "user", Content: "test"}}

	// Act
	ctx := context.Background()
	response, err := client.Chat(ctx, messages)

	// Assert
	if err == nil {
		t.Fatal("Chat() expected error for empty choices, got nil")
	}
	if response != "" {
		t.Errorf("Chat() expected empty response, got %q", response)
	}
	if errStr := err.Error(); !strings.Contains(errStr, "no choices") {
		t.Errorf("Chat() error = %q, want containing 'no choices'", errStr)
	}
}

func TestDeepSeekClient_Chat_ContextCancelled(t *testing.T) {
	// Arrange
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewDeepSeekClient("test-key")
	client.endpoint = ts.URL
	client.client.Timeout = 50 * time.Millisecond // shorter timeout

	messages := []Message{{Role: "user", Content: "test"}}

	// Act
	ctx := context.Background()
	response, err := client.Chat(ctx, messages)

	// Assert
	if err == nil {
		t.Fatal("Chat() expected timeout error, got nil")
	}
	if response != "" {
		t.Errorf("Chat() expected empty response, got %q", response)
	}
}
