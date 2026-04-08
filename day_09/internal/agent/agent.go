package agent

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Agent struct {
	Name        string
	Description string
	Prompt      string
}

func LoadAgents(agentsDir string) ([]Agent, error) {
	files, err := os.ReadDir(agentsDir)
	if err != nil {
		return nil, fmt.Errorf("reading agents directory: %w", err)
	}

	var agents []Agent
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if strings.HasSuffix(file.Name(), ".md") {
			path := filepath.Join(agentsDir, file.Name())
			agent, err := parseAgentFile(path)
			if err != nil {
				log.Printf("skipping %s: %v", file.Name(), err)
				continue
			}
			agents = append(agents, agent)
		}
	}
	return agents, nil
}

func parseAgentFile(path string) (Agent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Agent{}, fmt.Errorf("reading file: %w", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	var name, description, prompt string
	var inPrompt bool
	var descLines []string

	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			continue
		}
		if strings.HasPrefix(line, "## Prompt") {
			// Prompt starts from next line
			promptLines := lines[i+1:]
			prompt = strings.Join(promptLines, "\n")
			break
		}
		if name != "" && !inPrompt {
			descLines = append(descLines, line)
		}
	}

	if name == "" {
		return Agent{}, fmt.Errorf("agent name not found")
	}

	description = strings.TrimSpace(strings.Join(descLines, "\n"))
	prompt = strings.TrimSpace(prompt)

	return Agent{
		Name:        name,
		Description: description,
		Prompt:      prompt,
	}, nil
}
