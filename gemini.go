package main

import (
	"context"
	"fmt"

	"google.golang.org/api/option"
	"google.golang.org/genai"
)

type GeminiClient struct {
	client *genai.Client
	model  *genai.GenerativeModel
}

func NewGeminiClient(apiKey string) (*GeminiClient, error) {
	ctx := context.Background()

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	model := client.GenerativeModel("gemini-2.0-flash-exp")
	//
	// model.SetTemperature(0.7)
	// model.SetTopP(0.95)
	// model.SetTopK(40)
	model.SetMaxOutputTokens(8192) // Max response length

	// Set system instruction
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text("You are a helpful AI assistant in a Matrix chat room. " +
				"Provide clear, concise, and friendly responses. " +
				"Use markdown formatting when appropriate."),
		},
	}

	return &GeminiClient{
		client: client,
		model:  model,
	}, nil
}

func (g *GeminiClient) Ask(ctx context.Context, prompt string) (string, error) {
	session := g.model.StartChat()

	resp, err := session.SendMessage(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("no response candidates returned")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", fmt.Errorf("empty response content")
	}

	// concatenate all text parts
	var responseText string
	for _, part := range candidate.Content.Parts {
		if textPart, ok := part.(genai.Text); ok {
			responseText += string(textPart)
		}
	}

	if responseText == "" {
		return "", fmt.Errorf("no text in response")
	}

	return responseText, nil
}

func (g *GeminiClient) Close() error {
	return g.client.Close()
}
