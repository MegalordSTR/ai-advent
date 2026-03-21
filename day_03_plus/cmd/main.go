package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

// Message представляет одно сообщение в диалоге
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest структура запроса к API
type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// StreamChoice описывает один выбор в потоковом ответе
type StreamChoice struct {
	Delta struct {
		Content string `json:"content"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

// StreamResponse соответствует одному чанку потокового ответа
type StreamResponse struct {
	Choices []StreamChoice `json:"choices"`
}

// performRequest отправляет запрос к API, выводит поток ответа в консоль и возвращает полный текст ответа.
func performRequest(ctx context.Context, apiKey, prompt, systemContent string) (string, error) {
	messages := []Message{}
	if systemContent != "" {
		messages = append(messages, Message{Role: "system", Content: systemContent})
	}
	messages = append(messages, Message{Role: "user", Content: prompt})

	reqBody := ChatCompletionRequest{
		Model:    "deepseek-reasoner",
		Messages: messages,
		Stream:   true,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ошибка маршалинга запроса: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.deepseek.com/v1/chat/completions", strings.NewReader(string(jsonData)))
	if err != nil {
		return "", fmt.Errorf("ошибка создания запроса: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API вернул ошибку (статус %d): %s", resp.StatusCode, string(body))
	}

	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return "", fmt.Errorf("ожидался потоковый ответ (text/event-stream), получен %s", resp.Header.Get("Content-Type"))
	}

	fmt.Println("\nОтвет DeepSeek (потоковый):")
	var fullResponse strings.Builder
	reader := bufio.NewReader(resp.Body)

	for {
		select {
		case <-ctx.Done():
			return fullResponse.String(), ctx.Err()
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fullResponse.String(), fmt.Errorf("ошибка чтения потока: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var streamResp StreamResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			log.Printf("Ошибка парсинга JSON: %v, данные: %s", err, data)
			continue
		}

		if len(streamResp.Choices) > 0 {
			content := streamResp.Choices[0].Delta.Content
			if content != "" {
				fmt.Print(content)
				fullResponse.WriteString(content)
			}
		}
	}
	fmt.Println()
	return fullResponse.String(), nil
}

// getPrompt читает запрос из аргументов командной строки или из stdin
func getPrompt() string {
	if len(os.Args) > 1 {
		return strings.Join(os.Args[1:], " ")
	}

	fmt.Print("Введите ваш запрос: ")
	data, err := io.ReadAll(os.Stdin)
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

	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		log.Fatal("ОШИБКА: не установлена переменная окружения DEEPSEEK_API_KEY")
	}

	// Получаем вопрос от пользователя
	prompt := getPrompt()
	if prompt == "" {
		log.Fatal("ОШИБКА: пустой запрос")
	}

	// Контекст с таймаутом (увеличен для трёх последовательных вызовов)
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
