package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"ai-agent-cli/core/service"
)

type ChatHandler struct {
	service service.ChatServiceInterface
}

func NewChatHandler(service service.ChatServiceInterface) *ChatHandler {
	return &ChatHandler{service: service}
}

// ListAgents handles GET /api/agents
func (h *ChatHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.service.ListAgents()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load agents: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(agents); err != nil {
		log.Printf("failed to encode agents response: %v", err)
	}
}

// extractAgentName extracts and decodes the agent name from the URL path.
// Expected path format: /api/agents/{agentName}/messages
func extractAgentName(path string) (string, error) {
	// Remove prefix and suffix
	agentName := strings.TrimPrefix(path, "/api/agents/")
	agentName = strings.TrimSuffix(agentName, "/messages")
	if agentName == "" {
		return "", fmt.Errorf("Agent name is required")
	}
	// URL decode the agent name (handles encoded characters like Russian)
	decoded, err := url.PathUnescape(agentName)
	if err != nil {
		// If decoding fails, return the original
		return agentName, nil
	}
	return decoded, nil
}

// GetMessages handles GET /api/agents/{agentName}/messages
func (h *ChatHandler) GetMessages(w http.ResponseWriter, r *http.Request) {
	agentName, err := extractAgentName(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	messages, err := h.service.GetChatHistory(agentName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load messages: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(messages); err != nil {
		log.Printf("failed to encode messages response: %v", err)
	}
}

// SendMessage handles POST /api/agents/{agentName}/messages
func (h *ChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	agentName, err := extractAgentName(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		http.Error(w, "Message is required", http.StatusBadRequest)
		return
	}

	response, err := h.service.SendMessage(r.Context(), agentName, req.Message)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send message: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"response": response}); err != nil {
		log.Printf("failed to encode send message response: %v", err)
	}
}

// ServeHTTP implements http.Handler to route requests.
func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/agents":
		h.ListAgents(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/agents/") && strings.HasSuffix(r.URL.Path, "/messages"):
		h.GetMessages(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/agents/") && strings.HasSuffix(r.URL.Path, "/messages"):
		h.SendMessage(w, r)
	default:
		http.NotFound(w, r)
	}
}
