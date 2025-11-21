package main

import (
	"bytes"
	"encoding/base64"
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
	Contents         []Content         `json:"contents"`
	GenerationConfig *GenerationConfig `json:"generationConfig,omitempty"`
}

type GeminiVisionRequest struct {
	Contents         []VisionContent   `json:"contents"`
	GenerationConfig *GenerationConfig `json:"generationConfig,omitempty"`
}

type GenerationConfig struct {
	Temperature     float32 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type VisionContent struct {
	Parts []VisionPart `json:"parts"`
}

type VisionPart struct {
	Text       string      `json:"text,omitempty"`
	InlineData *InlineData `json:"inlineData,omitempty"`
}

type InlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type Part struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates    []Candidate    `json:"candidates"`
	UsageMetadata *UsageMetadata `json:"usageMetadata,omitempty"`
}

type Candidate struct {
	Content      Content `json:"content"`
	FinishReason string  `json:"finishReason,omitempty"`
}

type UsageMetadata struct {
	PromptTokenCount int `json:"promptTokenCount"`
	TotalTokenCount  int `json:"totalTokenCount"`
}

func (g *GeminiClient) GenerateResponse(prompt string, systemPrompt string, temperature float32, maxTokens int) (string, int, error) {
	fullPrompt := systemPrompt + "\n\n" + prompt

	requestBody := GeminiRequest{
		Contents: []Content{
			{
				Parts: []Part{
					{Text: fullPrompt},
				},
			},
		},
		GenerationConfig: &GenerationConfig{
			Temperature:     temperature,
			MaxOutputTokens: maxTokens,
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

	candidate := geminiResp.Candidates[0]
	if len(candidate.Content.Parts) == 0 {
		if candidate.FinishReason != "" {
			return "", 0, fmt.Errorf("Gemini blocked response. Reason: %s", candidate.FinishReason)
		}
		return "", 0, fmt.Errorf("Gemini returned an empty response")
	}

	tokenCount := 0
	if geminiResp.UsageMetadata != nil {
		tokenCount = geminiResp.UsageMetadata.TotalTokenCount
	} else {
		tokenCount = len(fullPrompt) / 4
	}

	responseText := candidate.Content.Parts[0].Text
	return responseText, tokenCount, nil
}

func (g *GeminiClient) GenerateVisionResponse(prompt string, systemPrompt string, imageData []byte, mimeType string) (string, int, error) {
	fullPrompt := systemPrompt + "\n\n" + prompt

	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	requestBody := GeminiVisionRequest{
		Contents: []VisionContent{
			{
				Parts: []VisionPart{
					{Text: fullPrompt},
					{
						InlineData: &InlineData{
							MimeType: mimeType,
							Data:     imageBase64,
						},
					},
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
		return "", 0, fmt.Errorf("vision API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read vision response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("vision API error %d: %s", resp.StatusCode, string(body))
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", 0, fmt.Errorf("failed to parse vision response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		return "", 0, fmt.Errorf("no response candidates from Gemini vision")
	}

	candidate := geminiResp.Candidates[0]
	if len(candidate.Content.Parts) == 0 {
		if candidate.FinishReason != "" {
			return "", 0, fmt.Errorf("Gemini blocked vision response. Reason: %s", candidate.FinishReason)
		}
		return "", 0, fmt.Errorf("Gemini returned an empty vision response")
	}

	tokenCount := 0
	if geminiResp.UsageMetadata != nil {
		tokenCount = geminiResp.UsageMetadata.TotalTokenCount
	} else {
		tokenCount = len(prompt) / 4
	}

	responseText := candidate.Content.Parts[0].Text
	return responseText, tokenCount, nil
}

func TestGemini(config *GeminiConfig, systemPrompt string) error {
	client := &GeminiClient{
		APIKey:  config.APIKey,
		BaseURL: config.BaseURL,
		Model:   config.Model,
	}

	response, tokens, err := client.GenerateResponse("Hello! Reply with just your name if you can hear me.", systemPrompt, 0.7, 500)
	if err != nil {
		return err
	}

	fmt.Printf("✅ Gemini test successful!\n")
	fmt.Printf("Response: %s\n", response)
	fmt.Printf("Tokens used: %d\n", tokens)
	return nil
}
