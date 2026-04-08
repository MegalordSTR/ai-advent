package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAgentFile(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantAgent Agent
		wantErr   bool
	}{
		{
			name: "valid agent file",
			content: `# Помощник
Описание помощника.
## Prompt
Ты полезный ассистент.`,
			wantAgent: Agent{
				Name:        "Помощник",
				Description: "Описание помощника.",
				Prompt:      "Ты полезный ассистент.",
			},
			wantErr: false,
		},
		{
			name: "empty description",
			content: `# Переводчик
## Prompt
Ты переводчик.`,
			wantAgent: Agent{
				Name:        "Переводчик",
				Description: "",
				Prompt:      "Ты переводчик.",
			},
			wantErr: false,
		},
		{
			name: "no name",
			content: `## Prompt
Без названия.`,
			wantAgent: Agent{},
			wantErr:   true,
		},
		{
			name: "multiline prompt",
			content: `# Тестер
Описание.
## Prompt
Первая строка.
Вторая строка.`,
			wantAgent: Agent{
				Name:        "Тестер",
				Description: "Описание.",
				Prompt:      "Первая строка.\nВторая строка.",
			},
			wantErr: false,
		},
		{
			name: "with comments and empty lines",
			content: `# Агент

Описание с пустыми строками.

## Prompt
Промпт.`,
			wantAgent: Agent{
				Name:        "Агент",
				Description: "Описание с пустыми строками.",
				Prompt:      "Промпт.",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "agent.md")
			if err := os.WriteFile(filePath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			gotAgent, err := parseAgentFile(filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseAgentFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if gotAgent.Name != tt.wantAgent.Name {
				t.Errorf("parseAgentFile() Name = %q, want %q", gotAgent.Name, tt.wantAgent.Name)
			}
			if strings.TrimSpace(gotAgent.Description) != strings.TrimSpace(tt.wantAgent.Description) {
				t.Errorf("parseAgentFile() Description = %q, want %q", gotAgent.Description, tt.wantAgent.Description)
			}
			if gotAgent.Prompt != tt.wantAgent.Prompt {
				t.Errorf("parseAgentFile() Prompt = %q, want %q", gotAgent.Prompt, tt.wantAgent.Prompt)
			}
		})
	}
}

func TestLoadAgents(t *testing.T) {
	tmpDir := t.TempDir()

	// Создаем несколько валидных файлов агентов
	agents := []struct {
		name    string
		content string
	}{
		{
			name: "helper.md",
			content: `# Помощник
Описание.
## Prompt
Промпт.`,
		},
		{
			name: "translator.md",
			content: `# Переводчик
Переводит.
## Prompt
Переводчик.`,
		},
	}

	for _, a := range agents {
		filePath := filepath.Join(tmpDir, a.name)
		if err := os.WriteFile(filePath, []byte(a.content), 0644); err != nil {
			t.Fatalf("failed to create test file %s: %v", a.name, err)
		}
	}

	// Создаем не-MD файл (должен игнорироваться)
	ignorePath := filepath.Join(tmpDir, "ignore.txt")
	if err := os.WriteFile(ignorePath, []byte("ignore"), 0644); err != nil {
		t.Fatalf("failed to create ignore file: %v", err)
	}

	// Создаем поддиректорию (должна игнорироваться)
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	gotAgents, err := LoadAgents(tmpDir)
	if err != nil {
		t.Fatalf("LoadAgents() error = %v", err)
	}
	if len(gotAgents) != 2 {
		t.Errorf("LoadAgents() returned %d agents, want 2", len(gotAgents))
	}

	// Проверяем, что агенты загружены в правильном порядке (по алфавиту имени файла)
	// helper.md, translator.md
	if gotAgents[0].Name != "Помощник" {
		t.Errorf("LoadAgents() first agent name = %q, want %q", gotAgents[0].Name, "Помощник")
	}
	if gotAgents[1].Name != "Переводчик" {
		t.Errorf("LoadAgents() second agent name = %q, want %q", gotAgents[1].Name, "Переводчик")
	}
}

func TestLoadAgents_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	gotAgents, err := LoadAgents(tmpDir)
	if err != nil {
		t.Fatalf("LoadAgents() error = %v", err)
	}
	if len(gotAgents) != 0 {
		t.Errorf("LoadAgents() returned %d agents, want 0", len(gotAgents))
	}
}

func TestLoadAgents_NonExistentDir(t *testing.T) {
	_, err := LoadAgents("/non/existent/directory")
	if err == nil {
		t.Error("LoadAgents() expected error for non-existent directory")
	}
}
