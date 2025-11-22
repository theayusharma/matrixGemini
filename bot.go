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

	"geminiMatrix/modules"
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
		prefix := "!" + strings.ToLower(b.Config.Name)
		b.sendReply(evt.RoomID, fmt.Sprintf("To analyze images, you need to set your Gemini API key first. Use `!%s setkey <your_api_key>`", prefix))
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

	cmdPrefix := "!" + strings.ToLower(b.Config.Name)
	msgBodyLower := strings.ToLower(msg.Body)

	isDirect := false
	if strings.Contains(msgBodyLower, strings.ToLower(b.Config.Name)) {
		isDirect = true
	}
	if strings.HasPrefix(msgBodyLower, cmdPrefix) {
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
		log.Printf("🔍 Found reply to event ID: %s. Fetching original...", originalEventID)

		originalEvent, err := b.Client.GetEvent(ctx, evt.RoomID, originalEventID)
		if err != nil {
			log.Printf("❌ Failed to fetch original event: %v", err)
			return
		}

		originalEvent.RoomID = evt.RoomID

		err = originalEvent.Content.ParseRaw(originalEvent.Type)
		if err != nil {
			log.Printf("❌ Failed to parse raw event content: %v", err)
			return
		}

		if originalEvent.Type == event.EventEncrypted {
			log.Printf("🔐 Original event is Encrypted. Attempting to decrypt...")

			if b.Client.Crypto == nil {
				log.Printf("❌ Crypto is disabled in config!")
				return
			}

			decryptedEvent, err := b.Client.Crypto.Decrypt(ctx, originalEvent)
			if err != nil {
				log.Printf("❌ Decryption failed: %v", err)
				b.sendReply(evt.RoomID, "Error: Decryption failed. (Note: If you just verified me, please resend the image).")
				return
			}

			originalEvent = decryptedEvent
			log.Printf("🔓 Decryption successful!")
		}

		_ = originalEvent.Content.ParseRaw(originalEvent.Type)

		originalMsg, ok := originalEvent.Content.Parsed.(*event.MessageEventContent)
		if ok && originalMsg.MsgType == event.MsgImage {
			log.Printf("🖼️ Detected reply to IMAGE! Processing...")
			originalMsg.Body = msg.Body
			b.handleImageMessage(ctx, originalEvent, originalMsg)
			return
		} else {
			log.Printf("ℹ️ Reply target was not an image (Type: %s)", originalEvent.Type.String())
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

	if strings.HasPrefix(msgBodyLower, cmdPrefix) {
		b.handleCommand(evt.RoomID, evt.Sender, msg.Body)
		return
	}

	// check credits
	if !b.UserCredits.CanUseAPI(evt.Sender) {
		b.sendReply(evt.RoomID, fmt.Sprintf("Sorry, you've reached your API usage limit. Use `!%s setkey <your_api_key>` to add your own key.", cmdPrefix))
		return
	}

	// generate response
	go b.processGeminiRequest(evt.RoomID, evt.Sender, query)
}

func (b *Bot) handleCommand(roomID id.RoomID, userID id.UserID, command string) {
	parts := strings.Fields(command)
	prefix := "!" + b.Config.Name

	if len(parts) < 2 {
		b.sendReply(roomID, fmt.Sprintf("Usage: `%s setkey`, `%s anime`, `%s urban`, `%s remind`", prefix, prefix, prefix, prefix))
		return
	}

	switch strings.ToLower(parts[1]) {
	case "setkey":
		if len(parts) < 3 {
			b.sendReply(roomID, fmt.Sprintf("Usage: `%s setkey <your_api_key>`", prefix))
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

	case "enable":
		if len(parts) >= 3 && parts[2] == "search" {
			b.UserCredits.SetSearchEnabled(userID, true)
			b.sendReply(roomID, "✅ Google Search Grounding ENABLED for you.")
		} else {
			b.sendReply(roomID, fmt.Sprintf("Usage: `%s enable search`", prefix))
		}

	case "disable":
		if len(parts) >= 3 && parts[2] == "search" {
			b.UserCredits.SetSearchEnabled(userID, false)
			b.sendReply(roomID, "❌ Google Search Grounding DISABLED for you.")
		} else {
			b.sendReply(roomID, fmt.Sprintf("Usage: `%s disable search`", prefix))
		}

	case "anime":
		if len(parts) < 3 {
			b.sendReply(roomID, fmt.Sprintf("Usage: `%s anime <title>`", prefix))
			return
		}
		query := strings.Join(parts[2:], " ")
		b.sendReply(roomID, "🔍 Searching AniList...")

		result, err := modules.GetAnimeInfo(query)
		if err != nil {
			b.sendReply(roomID, "Error: "+err.Error())
			return
		}
		b.sendReply(roomID, result)

	case "urban":
		if len(parts) < 3 {
			b.sendReply(roomID, fmt.Sprintf("Usage: `%s urban <term>`", prefix))
			return
		}
		term := strings.Join(parts[2:], " ")

		result, err := modules.GetUrbanDef(term)
		if err != nil {
			b.sendReply(roomID, "Error: "+err.Error())
			return
		}
		b.sendReply(roomID, result)

	case "remind":
		if len(parts) < 4 {
			b.sendReply(roomID, fmt.Sprintf("Usage: `%s remind <time> <text>`", prefix))
			return
		}
		result, err := modules.SetReminder(b.Client, roomID, userID, parts[2:])
		if err != nil {
			b.sendReply(roomID, "Error: "+err.Error())
			return
		}
		b.sendReply(roomID, result)

	case "wiki":
		if len(parts) < 3 {
			b.sendReply(roomID, fmt.Sprintf("Usage: `%s wiki <topic>`", prefix))
			return
		}
		query := strings.Join(parts[2:], " ")
		b.sendReply(roomID, "📖 Searching Wikipedia...")

		result, err := modules.GetWikiSummary(query)
		if err != nil {
			b.sendReply(roomID, "Error: "+err.Error())
			return
		}
		b.sendReply(roomID, result)

	case "8ball":
		question := strings.Join(parts[2:], " ")
		b.sendReply(roomID, modules.Magic8Ball(question))

	case "roulette":
		b.sendReply(roomID, modules.RussianRoulette(string(userID)))

	case "poll":
		rawArgs := strings.Fields(command)
		if len(rawArgs) < 3 {
			b.sendReply(roomID, fmt.Sprintf("Usage: `%s poll \"Question\" \"Opt1\" \"Opt2\"`", prefix))
			return
		}

		err := modules.CreatePoll(b.Client, roomID, rawArgs[2:])
		if err != nil {
			b.sendReply(roomID, "Error: "+err.Error())
		}

	default:
		b.sendReply(roomID, fmt.Sprintf("Unknown command. Try `%s help` or `%s anime`", prefix, prefix))
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

	useSearch := b.UserCredits.IsSearchEnabled(userID)

	response, tokens, err := clientToUse.GenerateResponse(query, fullPrompt, b.Config.Temperature, b.Config.MaxResponseTokens, useSearch)
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
