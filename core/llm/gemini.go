package llm

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

var keyRedactor = regexp.MustCompile(`(key=)[^&"\s]+`)

type GeminiProvider struct {
	APIKey  string
	BaseURL string
	Model   string
}

var _ Provider = (*GeminiProvider)(nil)

func (g *GeminiProvider) ID() string { return "gemini" }

type geminiRequest struct {
	Contents         []geminiContent         `json:"contents"`
	GenerationConfig *geminiGenerationConfig `json:"generationConfig,omitempty"`
	Tools            []geminiTool            `json:"tools,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inlineData,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiGenerationConfig struct {
	Temperature     float32 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type geminiTool struct {
	GoogleSearch *googleSearch `json:"googleSearch,omitempty"`
}

type googleSearch struct{}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		TotalTokenCount int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func (g *GeminiProvider) GenerateText(prompt string, cfg RequestConfig) (string, int, error) {
	return g.generateInternal(prompt, nil, "", cfg)
}

func (g *GeminiProvider) GenerateVision(prompt string, imageData []byte, mimeType string, cfg RequestConfig) (string, int, error) {
	return g.generateInternal(prompt, imageData, mimeType, cfg)
}

func (g *GeminiProvider) generateInternal(prompt string, imageData []byte, mimeType string, cfg RequestConfig) (string, int, error) {
	apiKey := g.APIKey
	if cfg.UserKeyOverride != "" {
		apiKey = cfg.UserKeyOverride
	}

	fullPrompt := cfg.SystemPrompt + "\n\n" + prompt

	var parts []geminiPart

	if imageData != nil && len(imageData) > 0 {
		encodedImage := base64.StdEncoding.EncodeToString(imageData)
		parts = append(parts, geminiPart{
			InlineData: &geminiInlineData{
				MimeType: mimeType,
				Data:     encodedImage,
			},
		})
	}

	parts = append(parts, geminiPart{
		Text: fullPrompt,
	})

	var tools []geminiTool
	if cfg.UseSearch {
		tools = []geminiTool{{GoogleSearch: &googleSearch{}}}
	}

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{Parts: parts},
		},
		GenerationConfig: &geminiGenerationConfig{
			Temperature:     cfg.Temperature,
			MaxOutputTokens: cfg.MaxTokens,
		},
		Tools: tools,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", g.BaseURL, g.Model, apiKey)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		safeErr := keyRedactor.ReplaceAllString(err.Error(), "$1[REDACTED]")
		return "", 0, fmt.Errorf("API connection failed: %s", safeErr)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", 0, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		return "", 0, fmt.Errorf("no response candidates")
	}

	candidate := geminiResp.Candidates[0]
	if len(candidate.Content.Parts) == 0 {
		if candidate.FinishReason != "" {
			return "", 0, fmt.Errorf("blocked by safety settings (%s)", candidate.FinishReason)
		}
		return "", 0, fmt.Errorf("empty response from model")
	}

	tokens := 0
	if geminiResp.UsageMetadata.TotalTokenCount > 0 {
		tokens = geminiResp.UsageMetadata.TotalTokenCount
	} else {
		tokens = len(fullPrompt) / 4
	}

	return candidate.Content.Parts[0].Text, tokens, nil
}
