package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"ai-agent-cli/internal/agent"
	"ai-agent-cli/internal/deepseek"
	"ai-agent-cli/internal/storage"
	_ "modernc.org/sqlite"
)

// ChatService provides business logic for AI agent chat.
type ChatService struct {
	agentsDir string
	apiKey    string
	db        *sql.DB
	client    *deepseek.DeepSeekClient
}

// NewChatService creates a new chat service.
// If dbPath is empty or not provided, defaults to "chat_history.db".
func NewChatService(agentsDir, apiKey string, dbPath ...string) (*ChatService, error) {
	// Load agents to validate directory
	agents, err := agent.LoadAgents(agentsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load agents: %w", err)
	}
	if len(agents) == 0 {
		return nil, fmt.Errorf("no agents found in %s", agentsDir)
	}

	// Determine database path
	dbFile := "chat_history.db"
	if len(dbPath) > 0 && dbPath[0] != "" {
		dbFile = dbPath[0]
	}

	// Initialize database
	db, err := storage.OpenDB(dbFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := storage.InitDB(db); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("failed to close database after init error: %v", closeErr)
		}
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	client := deepseek.NewDeepSeekClient(apiKey)

	return &ChatService{
		agentsDir: agentsDir,
		apiKey:    apiKey,
		db:        db,
		client:    client,
	}, nil
}

// Close releases resources.
func (s *ChatService) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// ListAgents returns all available agents.
func (s *ChatService) ListAgents() ([]agent.Agent, error) {
	return agent.LoadAgents(s.agentsDir)
}

// GetChatHistory returns messages for a specific agent.
func (s *ChatService) GetChatHistory(agentName string) ([]deepseek.Message, error) {
	return storage.LoadMessages(s.db, agentName)
}

// SendMessage sends a user message to the specified agent and returns the assistant's response.
func (s *ChatService) SendMessage(ctx context.Context, agentName, userMessage string) (string, error) {
	// Load agent to get prompt
	agents, err := agent.LoadAgents(s.agentsDir)
	if err != nil {
		return "", fmt.Errorf("failed to load agents: %w", err)
	}
	var selectedAgent *agent.Agent
	for _, a := range agents {
		if a.Name == agentName {
			selectedAgent = &a
			break
		}
	}
	if selectedAgent == nil {
		return "", fmt.Errorf("agent %s not found", agentName)
	}

	// Load existing messages
	messages, err := storage.LoadMessages(s.db, agentName)
	if err != nil {
		log.Printf("warning: failed to load messages for agent %s: %v", agentName, err)
	}

	// Build conversation: system prompt + history + new user message
	conversation := []deepseek.Message{
		{Role: "system", Content: selectedAgent.Prompt},
	}
	conversation = append(conversation, messages...)
	conversation = append(conversation, deepseek.Message{Role: "user", Content: userMessage})

	// Call DeepSeek API
	response, err := s.client.Chat(ctx, conversation)
	if err != nil {
		return "", fmt.Errorf("API call failed: %w", err)
	}

	// Save both user message and assistant response
	if err := storage.SaveMessage(s.db, agentName, "user", userMessage); err != nil {
		log.Printf("warning: failed to save user message: %v", err)
	}
	if err := storage.SaveMessage(s.db, agentName, "assistant", response); err != nil {
		log.Printf("warning: failed to save assistant message: %v", err)
	}

	// Prune old messages (keep last 200)
	if err := storage.PruneMessages(s.db, agentName, 200); err != nil {
		log.Printf("warning: failed to prune messages: %v", err)
	}

	return response, nil
}

// GetClient returns the DeepSeek client (primarily for testing)
func (s *ChatService) GetClient() *deepseek.DeepSeekClient {
	return s.client
}

// ChatServiceInterface defines the contract for chat services (used for testing)
type ChatServiceInterface interface {
	ListAgents() ([]agent.Agent, error)
	GetChatHistory(agentName string) ([]deepseek.Message, error)
	SendMessage(ctx context.Context, agentName, userMessage string) (string, error)
	Close() error
}
