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
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature float64   `json:"temperature"`
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
func performRequest(ctx context.Context, apiKey, prompt, systemContent string, temperature float64) (string, error) {
	messages := []Message{}
	if systemContent != "" {
		messages = append(messages, Message{Role: "system", Content: systemContent})
	}
	messages = append(messages, Message{Role: "user", Content: prompt})

	reqBody := ChatCompletionRequest{
		Model:       "deepseek-reasoner",
		Messages:    messages,
		Stream:      true,
		Temperature: temperature,
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

// saveTemperatureResponse сохраняет ответ с метаданными температуры и запроса
func saveTemperatureResponse(temperature float64, prompt, answer string) {
	// Генерируем имя файла: temperature с точкой заменяем на подчёркивание
	tempStr := fmt.Sprintf("%.1f", temperature)
	tempStr = strings.ReplaceAll(tempStr, ".", "_")
	filename := fmt.Sprintf("answer_temperature_%s.md", tempStr)

	// Формируем содержимое с заголовком и запросом
	content := fmt.Sprintf("# Ответ при temperature=%.1f\n\n## Запрос пользователя\n%s\n\n## Ответ\n%s",
		temperature, prompt, answer)

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

	// Системная инструкция для всех запросов
	systemInstruction := "Ответь на запрос пользователя. Верни результат в формате Markdown"

	// Температуры для экспериментов
	temperatures := []float64{0, 0.7, 1.2}

	fmt.Printf("\nВыполняем запрос с температурами: %v\n\n", temperatures)

	for _, temp := range temperatures {
		fmt.Printf("=== Запрос с temperature=%.1f ===\n", temp)

		answer, err := performRequest(ctx, apiKey, prompt, systemInstruction, temp)
		if err != nil {
			log.Printf("Ошибка при temperature=%.1f: %v", temp, err)
			log.Println("Продолжаем с следующей температурой...")
			continue
		}

		saveTemperatureResponse(temp, prompt, answer)
		fmt.Println()
	}

	fmt.Println("Все ответы сохранены.")
}
