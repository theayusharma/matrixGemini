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
	UserCredits *CreditManager
	AutoJoin    bool
}

func NewBot(client *mautrix.Client, gemini *GeminiClient, botConfig *BotConfig, creditManager *CreditManager, autoJoin bool) *Bot {
	return &Bot{
		Client:      client,
		Gemini:      gemini,
		Config:      botConfig,
		UserCredits: creditManager,
		AutoJoin:    autoJoin,
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

func (b *Bot) handleMessage(ctx context.Context, evt *event.Event) {
	// ignore bot's own msgs
	if evt.Sender == b.Client.UserID {
		return
	}

	// ignore msgs older than 2m
	if time.Since(time.UnixMilli(evt.Timestamp)) > 2*time.Minute {
		return
	}

	// only handle text msgs
	msg, ok := evt.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		return
	}
	if msg.MsgType != event.MsgText {
		return
	}

	// check if mentioned or direct
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

	default:
		b.sendReply(roomID, "Unknown command. Use `!gemini setkey` or `!gemini stats`")
	}
}

func (b *Bot) processGeminiRequest(roomID id.RoomID, userID id.UserID, query string) {
	userKey, err := b.UserCredits.GetUserAPIKey(userID)

	clientToUse := *b.Gemini
	if err == nil && userKey != "" {
		clientToUse.APIKey = userKey
	}

	response, tokens, err := clientToUse.GenerateResponse(query, b.Config.SystemPrompt)
	if err != nil {
		log.Printf("Gemini error: %v", err)
		b.sendReply(roomID, "Sorry, I'm having trouble connecting to Gemini right now.")
		return
	}

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
