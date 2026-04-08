package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

// GetMessages handles GET /api/agents/{agentName}/messages
func (h *ChatHandler) GetMessages(w http.ResponseWriter, r *http.Request) {
	agentName := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	agentName = strings.TrimSuffix(agentName, "/messages")
	if agentName == "" {
		http.Error(w, "Agent name is required", http.StatusBadRequest)
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
	agentName := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	agentName = strings.TrimSuffix(agentName, "/messages")
	if agentName == "" {
		http.Error(w, "Agent name is required", http.StatusBadRequest)
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
