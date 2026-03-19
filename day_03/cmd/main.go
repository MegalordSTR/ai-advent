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

// ChatCompletionRequest полная структура запроса с поддержкой дополнительных параметров
type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	// MaxTokens и Stop больше не используются, но оставлены для совместимости (опционально)
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
// Параметр systemContent может быть пустым.
func performRequest(ctx context.Context, apiKey, prompt, systemContent string) (string, error) {
	// Формируем сообщения
	messages := []Message{}
	if systemContent != "" {
		messages = append(messages, Message{Role: "system", Content: systemContent})
	}
	messages = append(messages, Message{Role: "user", Content: prompt})

	reqBody := ChatCompletionRequest{
		Model:    "deepseek-reasoner", // используем reasoning модель
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

	// Общий контекст с таймаутом и обработкой Ctrl+C
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

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
	if err := os.WriteFile("answer1.md", []byte(answer1), 0644); err != nil {
		log.Fatalf("Ошибка записи answer1.md: %v", err)
	}
	fmt.Println("Ответ сохранён в answer1.md")

	// ---------- ВТОРОЙ ЗАПРОС (с пошаговой инструкцией) ----------
	fmt.Println("\n=== ВТОРОЙ ЗАПРОС (пошаговое решение с экспертами) ===")
	systemInstruction := `Решай пошагово. Результат предоставь в формате markdown
1. Составь промпт для решения задачи. 
2. Группой экспертов реши задачу. В группе должен быть Аналитик, Инженер-Алгоритмист, Критик. 
3. Получи решение от каждого.`

	answer2, err := performRequest(ctx, apiKey, prompt, systemInstruction)
	if err != nil {
		log.Fatalf("Ошибка второго запроса: %v", err)
	}
	if err := os.WriteFile("answer2.md", []byte(answer2), 0644); err != nil {
		log.Fatalf("Ошибка записи answer2.md: %v", err)
	}
	fmt.Println("Ответ сохранён в answer2.md")
}
