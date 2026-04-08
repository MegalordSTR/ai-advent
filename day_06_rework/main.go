package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"ai-agent-cli/internal/agent"
	"ai-agent-cli/internal/deepseek"
	tea "github.com/charmbracelet/bubbletea"
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

	agents, err := agent.LoadAgents("agents")
	if err != nil {
		log.Fatalf("failed to load agents: %v", err)
	}
	if len(agents) == 0 {
		log.Fatal("no agents found in agents directory")
	}

	client := deepseek.NewDeepSeekClient(apiKey)

	model := newAgentSelectModel(agents, client)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
