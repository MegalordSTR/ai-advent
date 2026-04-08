package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ai-agent-cli/core/service"
	"ai-agent-cli/server/handlers"
	"ai-agent-cli/server/middleware"
)

// setupTestServer creates a test HTTP server with in-memory SQLite database
// and mocked DeepSeek client. Returns the server and a function to close it.
func setupTestServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	// Use the actual agents directory for testing
	agentsDir := filepath.Join("..", "agents")
	absAgentsDir, err := filepath.Abs(agentsDir)
	if err != nil {
		t.Fatalf("failed to get absolute path for agents directory: %v", err)
	}
	if _, err := os.Stat(absAgentsDir); err != nil {
		t.Fatalf("agents directory not found: %v (searched at %s)", err, absAgentsDir)
	}

	// Create chat service with in-memory database
	chatService, err := service.NewChatService(absAgentsDir, "test-api-key", ":memory:")
	if err != nil {
		t.Fatalf("failed to create chat service: %v", err)
	}

	// Mock the DeepSeek client to avoid real API calls
	// Use the exported GetClient method to access the client
	client := chatService.GetClient()
	if client == nil {
		t.Fatal("client is nil")
	}

	// Create a mock HTTP server for DeepSeek API
	mockDeepSeekServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate successful DeepSeek response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "This is a mock response from DeepSeek",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))

	// Configure the client to use our mock server
	client.SetEndpoint(mockDeepSeekServer.URL)

	// Create HTTP handler
	handler := handlers.NewChatHandler(chatService)

	// Setup routes (matching main.go)
	mux := http.NewServeMux()
	mux.Handle("/api/agents", handler)
	mux.Handle("/api/agents/", handler)

	// Serve static files for web UI if directory exists
	staticDir := filepath.Join("..", "web-ui/public")
	if _, err := os.Stat(staticDir); err == nil {
		absStaticDir, err := filepath.Abs(staticDir)
		if err == nil {
			mux.Handle("/", http.FileServer(http.Dir(absStaticDir)))
		}
	}

	// Wrap with CORS middleware
	handlerWithCORS := middleware.CORS(mux)

	// Create test server
	server := httptest.NewServer(handlerWithCORS)

	cleanup := func() {
		server.Close()
		chatService.Close()
		mockDeepSeekServer.Close()
	}

	return server, cleanup
}

// TestIntegration_ListAgents tests the GET /api/agents endpoint
func TestIntegration_ListAgents(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Make request to list agents
	resp, err := http.Get(server.URL + "/api/agents")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	// Check CORS headers
	origin := resp.Header.Get("Access-Control-Allow-Origin")
	if origin != "*" {
		t.Errorf("expected Access-Control-Allow-Origin: *, got %s", origin)
	}

	// Parse response
	var agents []struct {
		Name        string `json:"Name"`
		Description string `json:"Description"`
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if err := json.Unmarshal(body, &agents); err != nil {
		t.Fatalf("failed to unmarshal response: %v, body: %s", err, string(body))
	}

	// Verify we got some agents
	if len(agents) == 0 {
		t.Error("expected at least one agent, got none")
	}

	// Check that we have the expected agents (helper and translator)
	foundHelper := false
	foundTranslator := false
	for _, agent := range agents {
		if agent.Name == "Помощник" {
			foundHelper = true
		}
		if agent.Name == "Переводчик" {
			foundTranslator = true
		}
	}
	if !foundHelper {
		t.Error("expected to find 'Помощник' agent")
	}
	if !foundTranslator {
		t.Error("expected to find 'Переводчик' agent")
	}
}

// TestIntegration_GetAgentMessages tests the GET /api/agents/{agent}/messages endpoint
func TestIntegration_GetAgentMessages(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Test with existing agent
	agentName := "Помощник"
	resp, err := http.Get(server.URL + "/api/agents/" + agentName + "/messages")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	// Parse messages
	var messages []struct {
		Role    string `json:"Role"`
		Content string `json:"Content"`
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if err := json.Unmarshal(body, &messages); err != nil {
		t.Fatalf("failed to unmarshal messages: %v, body: %s", err, string(body))
	}
	// Initially should be empty or contain system messages only
	// No assertions about content since it depends on DB state
}

// TestIntegration_GetAgentMessages_NotFound tests getting messages for non-existent agent
func TestIntegration_GetAgentMessages_NotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Test with non-existent agent
	agentName := "NonExistentAgent"
	resp, err := http.Get(server.URL + "/api/agents/" + agentName + "/messages")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Should return 200 with empty array (agent may not exist but messages can be empty)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for non-existent agent (empty messages), got %d", resp.StatusCode)
	}
	// Verify response is empty JSON array
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	var messages []interface{}
	if err := json.Unmarshal(body, &messages); err != nil {
		t.Errorf("failed to unmarshal response as array: %v, body: %s", err, string(body))
	}
	if len(messages) != 0 {
		t.Errorf("expected empty array, got %d elements", len(messages))
	}
}

// TestIntegration_SendMessage tests the POST /api/agents/{agent}/messages endpoint
func TestIntegration_SendMessage(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	agentName := "Помощник"
	url := server.URL + "/api/agents/" + agentName + "/messages"

	// Prepare request body
	requestBody := map[string]string{
		"message": "Hello, assistant!",
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}

	// Send POST request
	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	// Parse response
	var response struct {
		Response string `json:"response"`
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v, body: %s", err, string(body))
	}

	// Check that we got a response
	if response.Response == "" {
		t.Error("expected non-empty response")
	}

	// Verify the response matches our mock
	expectedResponse := "This is a mock response from DeepSeek"
	if response.Response != expectedResponse {
		t.Errorf("expected response %q, got %q", expectedResponse, response.Response)
	}
}

// TestIntegration_SendMessage_InvalidAgent tests sending message to non-existent agent
func TestIntegration_SendMessage_InvalidAgent(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	agentName := "NonExistentAgent"
	url := server.URL + "/api/agents/" + agentName + "/messages"

	requestBody := map[string]string{
		"message": "Hello?",
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Should return 500 (agent not found triggers internal server error)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500 for non-existent agent, got %d", resp.StatusCode)
	}
}

// TestIntegration_SendMessage_EmptyMessage tests sending empty message
func TestIntegration_SendMessage_EmptyMessage(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	agentName := "Помощник"
	url := server.URL + "/api/agents/" + agentName + "/messages"

	// Empty message
	requestBody := map[string]string{
		"message": "",
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Should return 400 Bad Request
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty message, got %d", resp.StatusCode)
	}
}

// TestIntegration_CORS_Preflight tests OPTIONS request for CORS preflight
func TestIntegration_CORS_Preflight(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create OPTIONS request
	req, err := http.NewRequest("OPTIONS", server.URL+"/api/agents", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check CORS headers
	origin := resp.Header.Get("Access-Control-Allow-Origin")
	if origin != "https://example.com" {
		t.Errorf("expected Access-Control-Allow-Origin: https://example.com, got %s", origin)
	}

	methods := resp.Header.Get("Access-Control-Allow-Methods")
	if methods == "" {
		t.Error("expected Access-Control-Allow-Methods header")
	}

	headers := resp.Header.Get("Access-Control-Allow-Headers")
	if headers == "" {
		t.Error("expected Access-Control-Allow-Headers header")
	}

	// OPTIONS should return 200 OK
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for OPTIONS, got %d", resp.StatusCode)
	}
}

// TestIntegration_StaticFiles tests that static files are served
func TestIntegration_StaticFiles(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Check if static directory exists
	staticDir := filepath.Join("..", "web-ui/public")
	if _, err := os.Stat(staticDir); err != nil {
		t.Skip("static directory not found, skipping static file test")
	}

	// Request the main page
	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Should return 200 OK
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for static file, got %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("expected Content-Type text/html; charset=utf-8, got %s", contentType)
	}

	// Verify it's HTML
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if !bytes.Contains(body, []byte("<!DOCTYPE html>")) {
		t.Error("response body does not contain HTML doctype")
	}
}

// TestIntegration_NotFound tests 404 for unknown routes
func TestIntegration_NotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := http.Get(server.URL + "/api/nonexistent")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Should return 404
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404 for unknown route, got %d", resp.StatusCode)
	}
}
