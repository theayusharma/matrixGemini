package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type Bot struct {
	Client      *mautrix.Client
	Gemini      *GeminiClient
	Config      *BotConfig
	AutoJoin    bool
	UserCredits *CreditManager
	Context     *ContextManager
}

func NewBot(client *mautrix.Client, gemini *GeminiClient, botConfig *BotConfig, creditManager *CreditManager, autoJoin bool) *Bot {
	return &Bot{
		Client:      client,
		Gemini:      gemini,
		Config:      botConfig,
		AutoJoin:    autoJoin,
		UserCredits: creditManager,
		Context:     NewContextManager(botConfig.MaxConversationHistory),
	}
}

func (b *Bot) Start() error {
	syncer := b.Client.Syncer.(*mautrix.DefaultSyncer)

	// handle msgs
	syncer.OnEventType(event.EventMessage, b.handleMessage)

	// handle invites
	syncer.OnEventType(event.StateMember, b.handleInvite)

	return b.Client.Sync()
}

func (b *Bot) handleInvite(ctx context.Context, evt *event.Event) {
	if !b.AutoJoin {
		return
	}

	state := evt.Content.AsMember()
	isForMe := evt.GetStateKey() == b.Client.UserID.String()

	if state.Membership == event.MembershipInvite && isForMe {
		log.Printf("Received invite from %s for room %s. Joining...", evt.Sender, evt.RoomID)

		_, err := b.Client.JoinRoom(ctx, evt.RoomID.String(), nil)

		if err != nil {
			log.Printf("Failed to join room: %v", err)
		} else {
			log.Printf("✅ Successfully joined room %s", evt.RoomID)
			b.sendReply(evt.RoomID, "Hello! I am connected and ready (E2EE Supported).")
		}
	}
}

func (b *Bot) handleImageMessage(ctx context.Context, evt *event.Event, msg *event.MessageEventContent) {
	userKey, err := b.UserCredits.GetUserAPIKey(evt.Sender)
	if err != nil || userKey == "" {
		b.sendReply(evt.RoomID, "To analyze images, you need to set your Gemini API key first. Use `!gemini setkey <your_api_key>`")
		return
	}

	b.sendReply(evt.RoomID, "👀 Analyzing image...")

	var data []byte

	if msg.File != nil {
		cURI, err := msg.File.URL.Parse()
		if err != nil {
			log.Printf("Failed to parse encrypted file URL: %v", err)
			b.sendReply(evt.RoomID, "Error: Invalid file URL.")
			return
		}
		ciphertext, err := b.Client.DownloadBytes(ctx, cURI)
		if err != nil {
			log.Printf("Failed to download encrypted file: %v", err)
			b.sendReply(evt.RoomID, "Error: Could not download encrypted image.")
			return
		}
		data, err = msg.File.Decrypt(ciphertext)
		if err != nil {
			log.Printf("Failed to decrypt file: %v", err)
			b.sendReply(evt.RoomID, "Error: Could not decrypt image.")
			return
		}
	} else if msg.URL != "" {
		cURI, err := msg.URL.Parse()
		if err != nil {
			log.Printf("Failed to parse URL: %v", err)
			b.sendReply(evt.RoomID, "Error: Invalid image URL.")
			return
		}
		data, err = b.Client.DownloadBytes(ctx, cURI)
		if err != nil {
			log.Printf("Failed to download media: %v", err)
			b.sendReply(evt.RoomID, "Error: Could not download image.")
			return
		}
	} else {
		b.sendReply(evt.RoomID, "Error: No image URL or File found in message.")
		return
	}

	prompt := strings.TrimSpace(msg.Body)
	prompt = strings.ReplaceAll(prompt, "@"+string(b.Client.UserID), "")
	prompt = strings.ReplaceAll(prompt, b.Config.Name, "")
	prompt = strings.TrimSpace(prompt)

	if prompt == "" {
		prompt = "Describe what you see in this image in detail."
	}

	mimeType := "image/jpeg"
	if info := msg.GetInfo(); info != nil && info.MimeType != "" {
		mimeType = info.MimeType
	}

	go b.processVisionRequest(evt.RoomID, evt.Sender, prompt, data, mimeType)
}

func (b *Bot) processVisionRequest(roomID id.RoomID, userID id.UserID, prompt string, imageData []byte, mimeType string) {
	clientToUse := *b.Gemini

	userKey, err := b.UserCredits.GetUserAPIKey(userID)
	if err == nil && userKey != "" {
		clientToUse.APIKey = userKey
	}

	response, tokens, err := clientToUse.GenerateVisionResponse(prompt, b.Config.SystemPrompt, imageData, mimeType)
	if err != nil {
		log.Printf("Gemini vision error: %v", err)
		b.sendReply(roomID, "Sorry, I'm having trouble processing the image right now.")
		return
	}

	b.Context.AddMessage(roomID, userID, "user", prompt)
	b.Context.AddMessage(roomID, userID, "assistant", response)

	b.UserCredits.RecordUsage(userID, tokens)

	if len(response) > 2000 {
		response = response[:2000] + "..."
	}
	b.sendReply(roomID, response)
}

func (b *Bot) handleMessage(ctx context.Context, evt *event.Event) {
	// ignore bot's own msgs
	if evt.Sender == b.Client.UserID {
		return
	}

	// ignore msgs older than 2m
	if time.Since(time.UnixMilli(evt.Timestamp)) > 2*time.Minute {
		return
	}

	msg, ok := evt.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		return
	}

	isDirect := false
	if strings.Contains(strings.ToLower(msg.Body), strings.ToLower(b.Config.Name)) {
		isDirect = true
	}
	if strings.HasPrefix(msg.Body, "!gemini") {
		isDirect = true
	}

	if !isDirect {
		return // ignore messages not directed at the bot
	}

	if msg.MsgType == event.MsgImage {
		b.handleImageMessage(ctx, evt, msg)
		return
	}

	if msg.RelatesTo != nil && msg.RelatesTo.InReplyTo != nil {
		originalEventID := msg.RelatesTo.InReplyTo.EventID
		originalEvent, err := b.Client.GetEvent(ctx, evt.RoomID, originalEventID)

		if err == nil && originalEvent.Type == event.EventMessage {
			_ = originalEvent.Content.ParseRaw(originalEvent.Type)

			originalMsg, ok := originalEvent.Content.Parsed.(*event.MessageEventContent)
			if ok && originalMsg.MsgType == event.MsgImage {
				log.Printf("🖼️ Detected reply to image! Processing...")
				originalMsg.Body = msg.Body
				b.handleImageMessage(ctx, originalEvent, originalMsg)
				return
			}
		}
	}

	if msg.MsgType != event.MsgText {
		return
	}

	log.Printf("📩 Processing message from %s: %s", evt.Sender, msg.Body)

	// clean query
	query := strings.TrimSpace(msg.Body)
	query = strings.ReplaceAll(query, "@"+string(b.Client.UserID), "")
	query = strings.ReplaceAll(query, b.Config.Name, "")
	query = strings.TrimSpace(query)

	if query == "" {
		return
	}

	if strings.HasPrefix(msg.Body, "!gemini") {
		b.handleCommand(evt.RoomID, evt.Sender, msg.Body)
		return
	}

	// check credits
	if !b.UserCredits.CanUseAPI(evt.Sender) {
		b.sendReply(evt.RoomID, "Sorry, you've reached your API usage limit. Use `!gemini setkey <your_api_key>` to add your own key.")
		return
	}

	// generate response
	go b.processGeminiRequest(evt.RoomID, evt.Sender, query)
}

func (b *Bot) handleCommand(roomID id.RoomID, userID id.UserID, command string) {
	parts := strings.Fields(command)
	if len(parts) < 2 {
		b.sendReply(roomID, "Usage: `!gemini setkey <api_key>` or `!gemini stats`")
		return
	}

	switch parts[1] {
	case "setkey":
		if len(parts) < 3 {
			b.sendReply(roomID, "Usage: `!gemini setkey <your_gemini_api_key>`")
			return
		}
		err := b.UserCredits.SetUserAPIKey(userID, parts[2])
		if err != nil {
			b.sendReply(roomID, "Failed to save API key: "+err.Error())
			return
		}
		b.sendReply(roomID, "✅ API key saved! Your requests will now use your own key.")

	case "stats":
		tokens, hasKey := b.UserCredits.GetUserStats(userID)
		msg := fmt.Sprintf("Tokens used: %d", tokens)
		if hasKey {
			msg += " (using your own API key)"
		} else {
			msg += fmt.Sprintf(" (global limit: %d)", b.UserCredits.globalLimit)
		}
		b.sendReply(roomID, msg)

	case "clear":
		b.Context.ClearConversation(roomID, userID)
		b.sendReply(roomID, "✅ Conversation history cleared.")

	default:
		b.sendReply(roomID, "Unknown command. Use `!gemini setkey` or `!gemini stats`")
	}
}

func (b *Bot) processGeminiRequest(roomID id.RoomID, userID id.UserID, query string) {
	history := b.Context.GetConversationHistory(roomID, userID)

	fullPrompt := b.Config.SystemPrompt + "\n\n"
	if history != "" {
		fullPrompt += "Conversation history:\n" + history + "\n\n"
	}

	userKey, err := b.UserCredits.GetUserAPIKey(userID)

	clientToUse := *b.Gemini
	if err == nil && userKey != "" {
		clientToUse.APIKey = userKey
	}

	response, tokens, err := clientToUse.GenerateResponse(query, fullPrompt, b.Config.Temperature, b.Config.MaxResponseTokens)
	if err != nil {
		log.Printf("Gemini error: %v", err)
		b.sendReply(roomID, "Sorry, I'm having trouble connecting to Gemini right now.")
		return
	}

	b.Context.AddMessage(roomID, userID, "user", query)
	b.Context.AddMessage(roomID, userID, "assistant", response)

	b.UserCredits.RecordUsage(userID, tokens)

	if len(response) > 2000 {
		response = response[:2000] + "..."
	}
	b.sendReply(roomID, response)
}

func (b *Bot) sendReply(roomID id.RoomID, text string) {
	_, err := b.Client.SendMessageEvent(context.Background(), roomID, event.EventMessage, &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    text,
	})
	if err != nil {
		log.Printf("Failed to send message: %v", err)
	}
}
