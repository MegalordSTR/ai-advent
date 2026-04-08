package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ai-agent-cli/internal/agent"
	"ai-agent-cli/internal/deepseek"
)

// mockChatService implements service.ChatServiceInterface for testing
type mockChatService struct {
	agents     []agent.Agent
	messages   map[string][]deepseek.Message
	listErr    error
	historyErr error
	sendErr    error
	closeErr   error
}

func (m *mockChatService) ListAgents() ([]agent.Agent, error) {
	return m.agents, m.listErr
}

func (m *mockChatService) GetChatHistory(agentName string) ([]deepseek.Message, error) {
	if m.historyErr != nil {
		return nil, m.historyErr
	}
	msgs, ok := m.messages[agentName]
	if !ok {
		return []deepseek.Message{}, nil
	}
	return msgs, nil
}

func (m *mockChatService) SendMessage(ctx context.Context, agentName, userMessage string) (string, error) {
	if m.sendErr != nil {
		return "", m.sendErr
	}
	return "Mocked response", nil
}

func (m *mockChatService) Close() error {
	return m.closeErr
}

func TestChatHandler_ListAgents(t *testing.T) {
	tests := []struct {
		name       string
		agents     []agent.Agent
		listErr    error
		wantStatus int
		wantBody   string
	}{
		{
			name: "success with agents",
			agents: []agent.Agent{
				{Name: "Helper", Description: "A helpful assistant"},
				{Name: "Translator", Description: "Translates text"},
			},
			wantStatus: http.StatusOK,
			wantBody:   `[{"Name":"Helper","Description":"A helpful assistant","Prompt":""},{"Name":"Translator","Description":"Translates text","Prompt":""}]`,
		},
		{
			name:       "empty agents list",
			agents:     []agent.Agent{},
			wantStatus: http.StatusOK,
			wantBody:   `[]`,
		},
		{
			name:       "service error",
			listErr:    errors.New("failed to load agents"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `Failed to load agents`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := &mockChatService{
				agents:  tt.agents,
				listErr: tt.listErr,
			}
			handler := NewChatHandler(mockSvc)

			req := httptest.NewRequest("GET", "/api/agents", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("ListAgents() status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			body := strings.TrimSpace(w.Body.String())
			if tt.wantBody != "" && !strings.Contains(body, tt.wantBody) {
				t.Errorf("ListAgents() body = %q, want containing %q", body, tt.wantBody)
			}
		})
	}
}

func TestChatHandler_GetMessages(t *testing.T) {
	mockMessages := map[string][]deepseek.Message{
		"Helper": {
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
		},
	}

	tests := []struct {
		name       string
		agentName  string
		messages   map[string][]deepseek.Message
		historyErr error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "success with messages",
			agentName:  "Helper",
			messages:   mockMessages,
			wantStatus: http.StatusOK,
			wantBody:   `[{"role":"user","content":"Hello"},{"role":"assistant","content":"Hi there"}]`,
		},
		{
			name:       "agent with no messages",
			agentName:  "EmptyAgent",
			messages:   map[string][]deepseek.Message{},
			wantStatus: http.StatusOK,
			wantBody:   `[]`,
		},
		{
			name:       "missing agent name",
			agentName:  "",
			wantStatus: http.StatusBadRequest,
			wantBody:   `Agent name is required`,
		},
		{
			name:       "service error",
			agentName:  "Helper",
			messages:   mockMessages,
			historyErr: errors.New("database error"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `Failed to load messages`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := &mockChatService{
				messages:   tt.messages,
				historyErr: tt.historyErr,
			}
			handler := NewChatHandler(mockSvc)

			url := "/api/agents/" + tt.agentName + "/messages"
			if tt.agentName == "" {
				url = "/api/agents//messages"
			}
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("GetMessages() status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			body := strings.TrimSpace(w.Body.String())
			if tt.wantBody != "" && !strings.Contains(body, tt.wantBody) {
				t.Errorf("GetMessages() body = %q, want containing %q", body, tt.wantBody)
			}
		})
	}
}

func TestChatHandler_SendMessage(t *testing.T) {
	tests := []struct {
		name       string
		agentName  string
		message    string
		sendErr    error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "success",
			agentName:  "Helper",
			message:    "Hello, world!",
			wantStatus: http.StatusOK,
			wantBody:   `{"response":"Mocked response"}`,
		},
		{
			name:       "missing agent name",
			agentName:  "",
			message:    "test",
			wantStatus: http.StatusBadRequest,
			wantBody:   `Agent name is required`,
		},
		{
			name:       "empty message body",
			agentName:  "Helper",
			message:    "",
			wantStatus: http.StatusBadRequest,
			wantBody:   `Message is required`,
		},
		{
			name:       "invalid JSON",
			agentName:  "Helper",
			message:    "", // will send invalid JSON
			wantStatus: http.StatusBadRequest,
			wantBody:   `Invalid request body`,
		},
		{
			name:       "service error",
			agentName:  "Helper",
			message:    "test",
			sendErr:    errors.New("agent not found"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `Failed to send message`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := &mockChatService{
				sendErr: tt.sendErr,
			}
			handler := NewChatHandler(mockSvc)

			var reqBody []byte
			if tt.name == "invalid JSON" {
				reqBody = []byte("{invalid json")
			} else {
				reqBody, _ = json.Marshal(map[string]string{"message": tt.message})
			}

			url := "/api/agents/" + tt.agentName + "/messages"
			if tt.agentName == "" {
				url = "/api/agents//messages"
			}
			req := httptest.NewRequest("POST", url, bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("SendMessage() status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			body := strings.TrimSpace(w.Body.String())
			if tt.wantBody != "" && !strings.Contains(body, tt.wantBody) {
				t.Errorf("SendMessage() body = %q, want containing %q", body, tt.wantBody)
			}
		})
	}
}

func TestChatHandler_NotFound(t *testing.T) {
	mockSvc := &mockChatService{}
	handler := NewChatHandler(mockSvc)

	tests := []struct {
		method string
		path   string
	}{
		{"GET", "/api/unknown"},
		{"POST", "/api/agents"},
		{"GET", "/api/agents/Helper"}, // missing /messages
		{"PUT", "/api/agents/Helper/messages"},
		{"DELETE", "/api/agents/Helper/messages"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("%s %s status = %d, want %d", tt.method, tt.path, resp.StatusCode, http.StatusNotFound)
			}
		})
	}
}
