package main

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

type GeminiClient struct {
	client *genai.Client
}

func NewGeminiClient(apiKey string) (*GeminiClient, error) {
	ctx := context.Background()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	return &GeminiClient{
		client: client,
	}, nil
}

func (g *GeminiClient) Ask(ctx context.Context, prompt string) (string, error) {
	systemInstruction := &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			genai.NewPartFromText("You are a helpful AI assistant in a Matrix chat room. " +
				"always reply in markdown only with proper markdown formating"),
		},
	}

	userContent := &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			genai.NewPartFromText(prompt),
		},
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: systemInstruction,
		Temperature:       genai.Ptr(float32(0.7)),
		MaxOutputTokens:   8192,
	}

	resp, err := g.client.Models.GenerateContent(ctx, "gemini-2.0-flash-exp", []*genai.Content{userContent}, config)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("no response candidates returned")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", fmt.Errorf("empty response content")
	}

	var responseText string
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			responseText += part.Text
		}
	}

	if responseText == "" {
		return "", fmt.Errorf("no text in response")
	}

	return responseText, nil
}

func (g *GeminiClient) AskPro(ctx context.Context, prompt string) (string, error) {
	systemInstruction := &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			genai.NewPartFromText("You are a helpful AI assistant in a Matrix chat room. " +
				"Always format your responses using proper Markdown syntax. " +
				"Use **bold** for emphasis, *italics* for subtle emphasis, " +
				"use code blocks with ``` for code snippets, " +
				"use inline `code` for technical terms, " +
				"use proper headings (##, ###), " +
				"use lists (- or 1.) for structured information, " +
				"and use > for quotes. " +
				"Provide clear, concise, and well-formatted responses."),
		},
	}

	userContent := &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			genai.NewPartFromText(prompt),
		},
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: systemInstruction,
		Temperature:       genai.Ptr(float32(0.7)),
		MaxOutputTokens:   8192,
	}

	resp, err := g.client.Models.GenerateContent(ctx, "gemini-2.5-pro", []*genai.Content{userContent}, config)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("no response candidates returned")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", fmt.Errorf("empty response content")
	}

	var responseText string
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			responseText += part.Text
		}
	}

	if responseText == "" {
		return "", fmt.Errorf("no text in response")
	}

	return responseText, nil
}

func (g *GeminiClient) Close() error {
	return nil
}
