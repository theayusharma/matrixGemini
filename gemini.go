package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type GeminiClient struct {
	APIKey  string
	BaseURL string
	Model   string
}

type GeminiRequest struct {
	Contents []Content `json:"contents"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates    []Candidate    `json:"candidates"`
	UsageMetadata *UsageMetadata `json:"usageMetadata,omitempty"`
}

type Candidate struct {
	Content Content `json:"content"`
}

type UsageMetadata struct {
	PromptTokenCount int `json:"promptTokenCount"`
	TotalTokenCount  int `json:"totalTokenCount"`
}

func (g *GeminiClient) GenerateResponse(prompt string, systemPrompt string) (string, int, error) {
	fullPrompt := systemPrompt + "\n\n" + prompt

	requestBody := GeminiRequest{
		Contents: []Content{
			{
				Parts: []Part{
					{Text: fullPrompt},
				},
			},
		},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	baseURL := strings.TrimSuffix(g.BaseURL, "/")
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", baseURL, g.Model, g.APIKey)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", 0, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if strings.Contains(string(body), "<html") {
			return "", 0, fmt.Errorf("API error %d (HTML response): check your Base URL and Model name. URL attempted: %s", resp.StatusCode, url)
		}
		return "", 0, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", 0, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		return "", 0, fmt.Errorf("no response candidates from Gemini")
	}

	// Extract token count if available, otherwise estimate
	tokenCount := 0
	if geminiResp.UsageMetadata != nil {
		tokenCount = geminiResp.UsageMetadata.TotalTokenCount
	} else {
		// Rough estimate: ~4 characters per token
		tokenCount = len(fullPrompt) / 4
	}

	responseText := geminiResp.Candidates[0].Content.Parts[0].Text
	return responseText, tokenCount, nil
}

func TestGemini(config *GeminiConfig, systemPrompt string) error {
	client := &GeminiClient{
		APIKey:  config.APIKey,
		BaseURL: config.BaseURL,
		Model:   config.Model,
	}

	response, tokens, err := client.GenerateResponse("Hello! Reply with just your name if you can hear me.", systemPrompt)
	if err != nil {
		return err
	}

	fmt.Printf("✅ Gemini test successful!\n")
	fmt.Printf("Response: %s\n", response)
	fmt.Printf("Tokens used: %d\n", tokens)
	return nil
}
