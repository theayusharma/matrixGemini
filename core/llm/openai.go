package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type OpenAIProvider struct {
	APIKey  string
	BaseURL string
	Model   string
}

var _ Provider = (*OpenAIProvider)(nil)

func (o *OpenAIProvider) ID() string { return "openai" }

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Temperature float32         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func (o *OpenAIProvider) GenerateText(prompt string, cfg RequestConfig) (string, int, error) {
	apiKey := o.APIKey
	if cfg.UserKeyOverride != "" {
		apiKey = cfg.UserKeyOverride
	}

	// OpenAI prefers System prompt as a separate message
	messages := []openAIMessage{}
	if cfg.SystemPrompt != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: cfg.SystemPrompt})
	}
	messages = append(messages, openAIMessage{Role: "user", Content: prompt})

	reqBody := openAIRequest{
		Model:       o.Model,
		Messages:    messages,
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
	}

	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", o.BaseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("OpenAI Error %d: %s", resp.StatusCode, string(body))
	}

	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, err
	}

	if len(result.Choices) == 0 {
		return "", 0, fmt.Errorf("empty response from OpenAI")
	}

	return result.Choices[0].Message.Content, result.Usage.TotalTokens, nil
}

func (o *OpenAIProvider) GenerateVision(prompt string, data []byte, mime string, cfg RequestConfig) (string, int, error) {
	// OpenAI Vision implementation requires Base64 URL format
	// Implementation omitted for brevity, but follows similar pattern to Text
	return "OpenAI Vision Not Implemented Yet", 0, nil
}
