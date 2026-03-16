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

// Структуры для запроса (аналогичны не-потоковому режиму)
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// Структура для одного чанка потокового ответа
type StreamChoice struct {
	Delta struct {
		Content string `json:"content"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

type StreamResponse struct {
	Choices []StreamChoice `json:"choices"`
}

func main() {
	// 1. Загружаем .env файл
	if err := godotenv.Load(); err != nil {
		log.Println("Файл .env не найден, используются системные переменные окружения")
	}

	// 2. Получаем API ключ
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		log.Fatal("ОШИБКА: не установлена переменная окружения DEEPSEEK_API_KEY")
	}

	// 3. Получаем текст запроса
	prompt := getPrompt()
	if prompt == "" {
		log.Fatal("ОШИБКА: пустой запрос")
	}

	// 4. Создаём контекст с таймаутом и возможностью отмены по Ctrl+C
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Слушаем сигналы прерывания (Ctrl+C)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nПолучен сигнал прерывания, завершаем...")
		cancel()
	}()

	// 5. Формируем тело запроса с включённым streaming
	reqBody := ChatCompletionRequest{
		Model: "deepseek-chat", // или другая модель, если нужно
		Messages: []Message{
			{Role: "user", Content: prompt},
		},
		Stream: true,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		log.Fatalf("Ошибка маршалинга запроса: %v", err)
	}

	// 6. Создаём HTTP запрос с контекстом
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.deepseek.com/v1/chat/completions", strings.NewReader(string(jsonData)))
	if err != nil {
		log.Fatalf("Ошибка создания запроса: %v", err)
	}

	// Заголовки
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	// 7. Выполняем запрос
	client := &http.Client{
		Timeout: 0, // Таймаут управляется через контекст, поэтому тут 0 (без ограничений)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		// Проверяем, не из-за отмены ли контекста
		if ctx.Err() != nil {
			log.Fatal("Запрос отменён или превышен таймаут")
		}
		log.Fatalf("Ошибка выполнения запроса: %v", err)
	}
	defer resp.Body.Close()

	// Проверяем статус код
	if resp.StatusCode != http.StatusOK {
		// Попробуем прочитать тело ошибки (оно может быть не потоковым)
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("API вернул ошибку (статус %d): %s", resp.StatusCode, string(body))
	}

	// Проверяем, что сервер действительно шлёт поток
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		log.Fatal("Ожидался потоковый ответ (text/event-stream), получен другой Content-Type")
	}

	// 8. Читаем поток событий построчно
	fmt.Println("\nОтвет DeepSeek (потоковый):")
	reader := bufio.NewReader(resp.Body)
	for {
		select {
		case <-ctx.Done():
			// Если контекст завершён (таймаут или Ctrl+C) – выходим
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break // Конец потока
			}
			// Если контекст отменён, ошибка может быть из-за закрытия соединения – игнорируем
			if ctx.Err() != nil {
				return
			}
			log.Fatalf("Ошибка чтения потока: %v", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue // Пустые строки пропускаем
		}

		// Обрабатываем только строки, начинающиеся с "data:"
		if !strings.HasPrefix(line, "data:") {
			// Возможно, это комментарий или мета-информация – игнорируем
			continue
		}

		// Убираем префикс "data:" и лишние пробелы
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

		// Проверяем маркер окончания потока
		if data == "[DONE]" {
			break
		}

		// Парсим JSON
		var streamResp StreamResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			// Если не удалось распарсить, возможно, это невалидный JSON – логируем и пропускаем
			log.Printf("Ошибка парсинга JSON: %v, данные: %s", err, data)
			continue
		}

		// Извлекаем содержимое из первого choice (если есть)
		if len(streamResp.Choices) > 0 {
			content := streamResp.Choices[0].Delta.Content
			if content != "" {
				fmt.Print(content)
			}
			// Если пришёл finish_reason, можно обработать, но обычно мы просто выходим по [DONE]
		}
	}
	fmt.Println() // Переход на новую строку после окончания
}

// getPrompt читает запрос из аргументов командной строки или из stdin
func getPrompt() string {
	if len(os.Args) > 1 {
		return strings.Join(os.Args[1:], " ")
	}

	// Читаем из stdin (интерактивный режим)
	fmt.Print("Введите ваш запрос: ")
	data, err := io.ReadAll(os.Stdin) // CTRL+D, чтобы сымитировать EOF
	if err != nil {
		log.Fatalf("Ошибка чтения ввода: %v", err)
	}
	return strings.TrimSpace(string(data))
}
