package core

type IncomingMessage struct {
	Platform      string
	UserID        string
	UserName      string
	ChatID        string
	Content       string
	IsImage       bool
	ImageData     []byte
	ImageMimeType string
	ReplyTo       *IncomingMessage
}

type Responder interface {
	SendText(chatID string, text string) error
	ReplyText(chatID string, originalMsgID string, text string) error
	SendReaction(chatID string, messageID string, emoji string) error
}
