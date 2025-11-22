package llm

type RequestConfig struct {
	Temperature     float32
	MaxTokens       int
	SystemPrompt    string
	UseSearch       bool
	UserKeyOverride string
}

type Provider interface {
	ID() string

	GenerateText(prompt string, config RequestConfig) (string, int, error)

	GenerateVision(prompt string, imageData []byte, mimeType string, config RequestConfig) (string, int, error)
}
