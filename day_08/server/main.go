package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"ai-agent-cli/core/service"
	"ai-agent-cli/server/handlers"
	"ai-agent-cli/server/middleware"
)

const (
	defaultPort = "8080"
	agentsDir   = "agents"
)

func loadAPIKey() (string, error) {
	// First try to load from .env.local file
	file, err := os.Open(".env.local")
	if err == nil {
		defer func() {
			if err := file.Close(); err != nil {
				log.Printf("failed to close file: %v", err)
			}
		}()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				// Remove quotes if present
				value = strings.Trim(value, `"'`)
				if key == "DEEPSEEK_API_KEY" {
					log.Printf("Loaded API key from .env.local")
					return value, nil
				}
			}
		}
		if err := scanner.Err(); err != nil {
			log.Printf("warning: error reading .env.local: %v", err)
		}
	}
	// Fall back to environment variable
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("DEEPSEEK_API_KEY not found in .env.local file or environment variable")
	}
	log.Printf("Loaded API key from environment variable")
	return apiKey, nil
}

func main() {
	apiKey, err := loadAPIKey()
	if err != nil {
		log.Fatal(err)
	}

	// Initialize chat service
	chatService, err := service.NewChatService(agentsDir, apiKey)
	if err != nil {
		log.Fatalf("Failed to create chat service: %v", err)
	}
	defer func() {
		if err := chatService.Close(); err != nil {
			log.Printf("failed to close chat service: %v", err)
		}
	}()

	// Create HTTP handler
	handler := handlers.NewChatHandler(chatService)

	// Setup routes
	mux := http.NewServeMux()
	mux.Handle("/api/agents", handler)
	mux.Handle("/api/agents/", handler)

	// Serve static files for web UI if directory exists
	staticDir := "web-ui/public"
	if _, err := os.Stat(staticDir); err == nil {
		log.Printf("Serving static files from %s", staticDir)
		mux.Handle("/", http.FileServer(http.Dir(staticDir)))
	} else {
		log.Printf("Static directory %s not found, UI will not be served", staticDir)
	}

	// Wrap with CORS middleware
	handlerWithCORS := middleware.CORS(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	log.Printf("Starting HTTP server on :%s", port)
	log.Printf("API endpoints:")
	log.Printf("  GET  /api/agents")
	log.Printf("  GET  /api/agents/{agent}/messages")
	log.Printf("  POST /api/agents/{agent}/messages")
	log.Printf("Agents directory: %s", agentsDir)

	if err := http.ListenAndServe(":"+port, handlerWithCORS); err != nil {
		log.Fatal(err)
	}
}
