package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/pkoukk/tiktoken-go"
)

type Message struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	TokenCount int    `json:"-"` // token count for this message (optional)
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// EstimateTokens approximates token count for text using cl100k_base encoding (used by DeepSeek models).
func EstimateTokens(text string) (int, error) {
	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return 0, fmt.Errorf("failed to get encoding: %w", err)
	}
	tokens := enc.Encode(text, nil, nil)
	return len(tokens), nil
}

type DeepSeekClient struct {
	apiKey   string
	client   *http.Client
	endpoint string
}

func NewDeepSeekClient(apiKey string) *DeepSeekClient {
	return &DeepSeekClient{
		apiKey:   apiKey,
		client:   &http.Client{Timeout: 180 * time.Second},
		endpoint: "https://api.deepseek.com/v1/chat/completions",
	}
}

// SetEndpoint overrides the API endpoint (primarily for testing)
func (c *DeepSeekClient) SetEndpoint(url string) {
	c.endpoint = url
}

type apiError struct {
	StatusCode int
	Body       string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("API error (%d): %s", e.StatusCode, e.Body)
}

func (c *DeepSeekClient) chatInternal(ctx context.Context, messages []Message) (string, Usage, error) {
	reqBody := struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
		Stream   bool      `json:"stream"`
	}{
		Model:    "deepseek-reasoner",
		Messages: messages,
		Stream:   false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", Usage{}, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", Usage{}, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", Usage{}, fmt.Errorf("sending request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close response body: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", Usage{}, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", Usage{}, &apiError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	var chatResp struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Choices []struct {
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
			Index        int     `json:"index"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", Usage{}, fmt.Errorf("unmarshaling response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", Usage{}, fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, chatResp.Usage, nil
}

// Chat sends messages to DeepSeek API and returns the assistant's response.
func (c *DeepSeekClient) Chat(ctx context.Context, messages []Message) (string, error) {
	content, _, err := c.chatInternal(ctx, messages)
	return content, err
}

// ChatWithUsage sends messages to DeepSeek API and returns both response and token usage.
func (c *DeepSeekClient) ChatWithUsage(ctx context.Context, messages []Message) (string, Usage, error) {
	return c.chatInternal(ctx, messages)
}
