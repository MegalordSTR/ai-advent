package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/joho/godotenv"
)

// performRequest отправляет запрос к Claude API с указанной моделью, выводит поток ответа в консоль и возвращает полный текст ответа.
func performRequest(ctx context.Context, apiKey string, model anthropic.Model, prompt, systemContent string) (string, error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	params := anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 25000,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	}

	if systemContent != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: systemContent},
		}
	}

	stream := client.Messages.NewStreaming(ctx, params)

	var fullResponse strings.Builder
	for stream.Next() {
		event := stream.Current()
		switch eventVariant := event.AsAny().(type) {
		case anthropic.ContentBlockDeltaEvent:
			switch deltaVariant := eventVariant.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				fmt.Print(deltaVariant.Text)
				fullResponse.WriteString(deltaVariant.Text)
			}
		}
	}

	fmt.Println()

	if err := stream.Err(); err != nil {
		return fullResponse.String(), fmt.Errorf("ошибка стриминга: %w", err)
	}

	return fullResponse.String(), nil
}

// saveResponse сохраняет текст ответа в файл и выводит сообщение
func saveResponse(filename, content string) {
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		log.Fatalf("Ошибка записи %s: %v", filename, err)
	}
	fmt.Printf("Ответ сохранён в %s\n", filename)
}

func main() {
	// Загружаем .env
	if err := godotenv.Load(); err != nil {
		log.Println("Файл .env не найден, используются системные переменные окружения")
	}

	apiKey := os.Getenv("CLAUDE_API_KEY")
	if apiKey == "" {
		log.Fatal("ОШИБКА: не установлена переменная окружения CLAUDE_API_KEY")
	}

	// Запрашиваем задачу у пользователя
	fmt.Print("Введите вашу задачу: ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	prompt := strings.TrimSpace(scanner.Text())
	if prompt == "" {
		log.Fatal("ОШИБКА: пустой запрос")
	}

	// Контекст с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Обработка Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nПолучен сигнал прерывания, завершаем...")
		cancel()
	}()

	systemPrompt := "Ответь на запрос пользователя. Ответ отдай в формате markdown."

	models := []struct {
		name  string
		model anthropic.Model
		file  string
	}{
		{"Haiku", anthropic.ModelClaudeHaiku4_5, "answer_haiku.md"},
		{"Sonnet", anthropic.ModelClaudeSonnet4_6, "answer_sonnet.md"},
		{"Opus", anthropic.ModelClaudeOpus4_6, "answer_opus.md"},
	}

	for _, m := range models {
		fmt.Printf("\n=== Запрос к %s ===\n", m.name)
		answer, err := performRequest(ctx, apiKey, m.model, prompt, systemPrompt)
		if err != nil {
			log.Fatalf("Ошибка запроса к %s: %v", m.name, err)
		}
		saveResponse(m.file, answer)
	}

	fmt.Println("\nВсе ответы сохранены.")
}
