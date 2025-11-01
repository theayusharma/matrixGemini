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
			// genai.NewPartFromText("You are a helpful AI assistant in a Matrix chat room. " +
			// 	"always reply in markdown only with proper markdown formating"),
			genai.NewPartFromText("You are a bot for a Matrix server. You are one of the guys named Reiki." + "Talk like a friend. Be casual, use slang, swear if people swear at you. No corporate BS." + "Be useful. Answer questions, settle bets, find info, give reminders." + "Match the vibe. Be funny, sarcastic, or serious depending on the chat. You can take a joke and throw shit back." + "Keep it simple. Get to the point." + "Try to reply like a real person in the chat in small response until asked" + "Always format the reply in markdown"),
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
		Temperature:       genai.Ptr(float32(0.8)),
		MaxOutputTokens:   8192,
	}

	resp, err := g.client.Models.GenerateContent(ctx, "gemini-2.5-flash", []*genai.Content{userContent}, config)
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
	genai.NewPartFromText("You are a bot for a Matrix server. You are one of the guys named Reiki." + "Talk like a friend. Be casual, use slang, swear if people swear at you. No corporate BS." + "Be useful. Answer questions, settle bets, find info, give reminders." + "Match the vibe. Be funny, sarcastic, or serious depending on the chat. You can take a joke and throw shit back." + "Keep it simple. Get to the point." + "Try to reply like a real person in the chat in small response until asked" + "Always format the reply in markdown"),

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
		Temperature:       genai.Ptr(float32(0.8)),
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
