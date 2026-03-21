package main

import (
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

// performRequest отправляет запрос к Claude API, выводит поток ответа в консоль и возвращает полный текст ответа.
func performRequest(ctx context.Context, apiKey, prompt, systemContent string) (string, error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	params := anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeSonnet4_6,
		MaxTokens: 16000,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	}

	if systemContent != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: systemContent},
		}
	}

	fmt.Println("\nОтвет Claude (потоковый):")

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

// getPrompt читает запрос из аргументов командной строки или из stdin
func getPrompt() string {
	if len(os.Args) > 1 {
		return strings.Join(os.Args[1:], " ")
	}

	fmt.Print("Введите ваш запрос: ")
	data, err := os.ReadFile("/dev/stdin")
	if err != nil {
		log.Fatalf("Ошибка чтения ввода: %v", err)
	}
	return strings.TrimSpace(string(data))
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

	// Получаем вопрос от пользователя
	prompt := getPrompt()
	if prompt == "" {
		log.Fatal("ОШИБКА: пустой запрос")
	}

	// Контекст с таймаутом (15 минут для трёх последовательных вызовов)
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

	// ---------- ПЕРВЫЙ ЗАПРОС (без инструкций) ----------
	fmt.Println("\n=== ПЕРВЫЙ ЗАПРОС (обычный) ===")
	answer1, err := performRequest(ctx, apiKey, prompt, "")
	if err != nil {
		log.Fatalf("Ошибка первого запроса: %v", err)
	}
	saveResponse("answer1.md", answer1)

	// ---------- ВТОРОЙ СЦЕНАРИЙ: МУЛЬТИАГЕНТНОЕ РЕШЕНИЕ ----------
	fmt.Println("\n=== ВТОРОЙ ЗАПРОС (мультиагентный) ===")

	// 1. Аналитик
	fmt.Println("\n--- Агент: Аналитик ---")
	systemAnalyst := `Ты — Аналитик. Твоя задача — проанализировать вопрос пользователя и составить подробный промпт для решения задачи, который будет использован инженером-алгоритмистом. Не решай задачу сам, только составь промпт. Промпт должен быть чётким, структурированным и содержать все необходимые детали для разработки решения. Результат предоставь в формате markdown.`
	analystAnswer, err := performRequest(ctx, apiKey, prompt, systemAnalyst)
	if err != nil {
		log.Fatalf("Ошибка при вызове Аналитика: %v", err)
	}
	saveResponse("answer2_analyst.md", analystAnswer)

	// 2. Инженер-Алгоритмист
	fmt.Println("\n--- Агент: Инженер-Алгоритмист ---")
	systemEngineer := `Ты — Инженер-Алгоритмист. Используя промпт, составленный аналитиком, разработай решение задачи. Предложи алгоритм, псевдокод или код, если требуется. Будь максимально конкретен. Результат предоставь в формате markdown.`
	engineerPrompt := fmt.Sprintf("Исходный вопрос пользователя:\n%s\n\nПромпт от аналитика:\n%s", prompt, analystAnswer)
	engineerAnswer, err := performRequest(ctx, apiKey, engineerPrompt, systemEngineer)
	if err != nil {
		log.Fatalf("Ошибка при вызове Инженера: %v", err)
	}
	saveResponse("answer2_engineer.md", engineerAnswer)

	// 3. Критик
	fmt.Println("\n--- Агент: Критик ---")
	systemCritic := `Ты — Критик. Проанализируй решение, предоставленное инженером-алгоритмистом. Выяви слабые места, предложи улучшения, укажи на возможные ошибки. Если необходимо, предложи исправленный вариант решения. Результат предоставь в формате markdown.`
	criticPrompt := fmt.Sprintf("Исходный вопрос пользователя:\n%s\n\nРешение инженера-алгоритмиста:\n%s", prompt, engineerAnswer)
	criticAnswer, err := performRequest(ctx, apiKey, criticPrompt, systemCritic)
	if err != nil {
		log.Fatalf("Ошибка при вызове Критика: %v", err)
	}
	saveResponse("answer2_critic.md", criticAnswer)

	// Формируем итоговый объединённый ответ
	finalContent := fmt.Sprintf(
		"# Мультиагентное решение\n\n"+
			"## Вопрос пользователя\n%s\n\n"+
			"## 1. Аналитик\n%s\n\n"+
			"## 2. Инженер-Алгоритмист\n%s\n\n"+
			"## 3. Критик\n%s\n",
		prompt, analystAnswer, engineerAnswer, criticAnswer)
	saveResponse("answer2.md", finalContent)

	fmt.Println("\nВсе ответы сохранены.")
}
