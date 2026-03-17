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
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	Stream    bool      `json:"stream"`
	MaxTokens int       `json:"max_tokens,omitempty"`
	Stop      []string  `json:"stop,omitempty"`
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
// Параметры maxTokens и stop могут быть nil/пустыми – тогда они не включаются в запрос.
func performRequest(ctx context.Context, apiKey, prompt, systemContent string, maxTokens *int, stop []string) (string, error) {
	// Формируем сообщения
	messages := []Message{}
	if systemContent != "" {
		messages = append(messages, Message{Role: "system", Content: systemContent})
	}
	messages = append(messages, Message{Role: "user", Content: prompt})

	reqBody := ChatCompletionRequest{
		Model:    "deepseek-chat",
		Messages: messages,
		Stream:   true,
	}

	if maxTokens != nil {
		reqBody.MaxTokens = *maxTokens
	}
	if len(stop) > 0 {
		reqBody.Stop = stop
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

	client := &http.Client{} // таймаут управляется контекстом
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
			// Логируем, но не прерываем – возможно, частичный или невалидный чанк
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
	fmt.Println() // перевод строки после окончания потока
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

// loadForm читает содержимое файла answer_form.md
func loadForm() (string, error) {
	content, err := os.ReadFile("answer_form.md")
	if err != nil {
		return "", fmt.Errorf("не удалось загрузить answer_form.md: %w", err)
	}
	return string(content), nil
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

	// Загружаем форму для второго запроса
	formContent, err := loadForm()
	if err != nil {
		log.Fatalf("ОШИБКА: %v", err)
	}
	fmt.Println("Форма успешно загружена из answer_form.md")

	// Читаем максимальную длину ответа из переменной окружения
	maxAnswerLen := 500 // значение по умолчанию
	if envLen := os.Getenv("MAX_ANSWER_LEN"); envLen != "" {
		if n, err := fmt.Sscanf(envLen, "%d", &maxAnswerLen); err != nil || n != 1 {
			log.Printf("Предупреждение: MAX_ANSWER_LEN=%q не является числом, используется 500", envLen)
			maxAnswerLen = 500
		}
	}

	// Общий контекст с таймаутом и обработкой Ctrl+C
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nПолучен сигнал прерывания, завершаем...")
		cancel()
	}()

	// ---------- ПЕРВЫЙ ЗАПРОС (без дополнительных параметров) ----------
	fmt.Println("\n=== ПЕРВЫЙ ЗАПРОС (обычный) ===")
	answer1, err := performRequest(ctx, apiKey, prompt, "", nil, nil)
	if err != nil {
		log.Fatalf("Ошибка первого запроса: %v", err)
	}
	if err := os.WriteFile("answer1.md", []byte(answer1), 0644); err != nil {
		log.Fatalf("Ошибка записи answer1.md: %v", err)
	}
	fmt.Println("\nОтвет сохранён в answer1.md")

	// ---------- ВТОРОЙ ЗАПРОС (с формой, stop_sequence и max_tokens) ----------
	fmt.Println("\n=== ВТОРОЙ ЗАПРОС (с формой) ===")
	stopSeq := []string{"Остановись сразу после четкого ответа на вопрос"}
	answer2, err := performRequest(ctx, apiKey, prompt, formContent, &maxAnswerLen, stopSeq)
	if err != nil {
		log.Fatalf("Ошибка второго запроса: %v", err)
	}
	if err := os.WriteFile("answer2.md", []byte(answer2), 0644); err != nil {
		log.Fatalf("Ошибка записи answer2.md: %v", err)
	}
	fmt.Println("\nОтвет сохранён в answer2.md")
}
