package core

import (
	"strings"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Conversation struct {
	Messages   []Message `json:"messages"`
	MaxHistory int       `json:"max_history"`
}

type ContextManager struct {
	conversations map[string]*Conversation
	maxHistory    int
}

func NewContextManager(maxHistory int) *ContextManager {
	return &ContextManager{
		conversations: make(map[string]*Conversation),
		maxHistory:    maxHistory,
	}
}

func (cm *ContextManager) GetConversationKey(roomID string, userID string) string {
	return string(roomID) + "|" + string(userID)
}

func (cm *ContextManager) AddMessage(roomID string, userID string, role, content string) {
	key := cm.GetConversationKey(roomID, userID)

	if cm.conversations[key] == nil {
		cm.conversations[key] = &Conversation{
			Messages:   []Message{},
			MaxHistory: cm.maxHistory,
		}
	}

	conv := cm.conversations[key]
	conv.Messages = append(conv.Messages, Message{
		Role:    role,
		Content: content,
	})

	if len(conv.Messages) > cm.maxHistory*2 {
		conv.Messages = conv.Messages[len(conv.Messages)-cm.maxHistory*2:]
	}
}

func (cm *ContextManager) GetConversationHistory(roomID string, userID string) string {
	key := cm.GetConversationKey(roomID, userID)
	conv := cm.conversations[key]

	if conv == nil || len(conv.Messages) == 0 {
		return ""
	}

	var history strings.Builder
	for _, msg := range conv.Messages {
		history.WriteString(msg.Role + ": " + msg.Content + "\n")
	}

	return history.String()
}

func (cm *ContextManager) ClearConversation(roomID string, userID string) {
	key := cm.GetConversationKey(roomID, userID)
	delete(cm.conversations, key)
}
