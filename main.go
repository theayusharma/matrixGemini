package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var debugMode bool

func debugPrint(format string, args ...interface{}) {
	if debugMode {
		fmt.Printf(format, args...)
	}
}

func debugLog(format string, args ...interface{}) {
	if debugMode {
		log.Printf(format, args...)
	}
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	}

	debugMode = os.Getenv("DEBUG") == "true"
	if !debugMode {
		log.SetOutput(io.Discard)
	}

	ctx := context.Background()
	homeserver := os.Getenv("SERVER_UURRLL")
	username := os.Getenv("USERNAME")

	password := os.Getenv("PASSS")
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")

	if homeserver == "" || username == "" || password == "" || geminiAPIKey == "" {
		log.Fatal("Missing required environment variables: SERVER_URL, USERNAME, PASSWORD, GEMINI_API_KEY")
	}

	client, err := mautrix.NewClient(homeserver, "", "")
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	// Login
	resp, err := client.Login(ctx, &mautrix.ReqLogin{
		Type: mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{
			Type: mautrix.IdentifierTypeUser,
			User: username,
		},
		Password:         password,
		StoreCredentials: true,
	})
	if err != nil {
		log.Fatal("Login failed:", err)
	}

	fmt.Println("Logged in as", resp.UserID)
	fmt.Println("Access Token:", resp.AccessToken)

	debugPrint("[INFO] Setting up encryption support...\n")
	cryptoHelper, err := cryptohelper.NewCryptoHelper(client, []byte("go-neb-password"), "matrix-bot-crypto.db")
	if err != nil {
		debugLog("[WARNING] Failed to create crypto helper: %v", err)
		debugPrint("[WARNING] Bot will work in unencrypted rooms only\n")
		cryptoHelper = nil
	} else {
		cryptoHelper.LoginAs = &mautrix.ReqLogin{
			Type: mautrix.AuthTypePassword,
			Identifier: mautrix.UserIdentifier{
				Type: mautrix.IdentifierTypeUser,
				User: username,
			},
			Password: password,
		}

		err = cryptoHelper.Init(ctx)
		if err != nil {
			debugLog("[WARNING] Failed to initialize crypto: %v", err)
			debugPrint("[WARNING] Bot will work in unencrypted rooms only\n")
			cryptoHelper = nil
		} else {
			client.Crypto = cryptoHelper

			syncer := client.Syncer.(*mautrix.DefaultSyncer)
			syncer.ParseEventContent = true
			syncer.ParseErrorHandler = func(evt *event.Event, err error) bool {
				debugLog("[CRYPTO ERROR] Failed to parse/decrypt event %s in %s: %v", evt.ID, evt.RoomID, err)
				return true
			}

			debugPrint("[SUCCESS] Encryption support enabled\n")
		}
	}

	gemini, err := NewGeminiClient(geminiAPIKey)
	if err != nil {
		log.Fatal("Gemini init failed:", err)
	}

	syncer := client.Syncer.(*mautrix.DefaultSyncer)

	joinedRooms := make(map[id.RoomID]bool)

	startTime := time.Now()
	debugPrint("[INFO] Bot started at: %v - will only respond to messages after this time\n", startTime)

	existingRooms, err := client.JoinedRooms(ctx)
	if err == nil {
		for _, roomID := range existingRooms.JoinedRooms {
			joinedRooms[roomID] = true
		}
		debugPrint("[INFO] Already in %d rooms\n", len(existingRooms.JoinedRooms))
	}

	syncer.OnEventType(event.StateMember, func(ctx context.Context, ev *event.Event) {
		if ev.GetStateKey() == client.UserID.String() && ev.Content.AsMember().Membership == event.MembershipInvite {
			if joinedRooms[ev.RoomID] {
				return
			}

			resp, err := client.JoinRoomByID(ctx, ev.RoomID)
			if err != nil {
				debugLog("[ERROR] Failed to join room %s: %v", ev.RoomID, err)
			} else {
				joinedRooms[ev.RoomID] = true
				debugPrint("[JOINED] Room: %s\n", resp.RoomID)
			}
		}
	})

	syncer.OnEventType(event.EventEncrypted, func(ctx context.Context, ev *event.Event) {
		debugPrint("[ENCRYPTED] Received encrypted event in room %s from %s\n", ev.RoomID, ev.Sender)
		debugPrint("[ENCRYPTED] Event will be auto-decrypted by cryptohelper\n")
	})

	syncer.OnEventType(event.EventMessage, func(ctx context.Context, ev *event.Event) {
		if ev.Sender == client.UserID {
			return
		}

		if time.UnixMilli(ev.Timestamp).Before(startTime) {
			debugPrint("[INFO] Skipping old unencrypted message from before bot start\n")
			return
		}

		msg := ev.Content.AsMessage()
		if msg == nil {
			return
		}

		handleMessage(ctx, client, gemini, ev.RoomID, ev.Sender, msg)
	})

	syncer.OnEvent(func(ctx context.Context, ev *event.Event) {
		debugPrint("[EVENT] Type: %s, Room: %s, Sender: %s\n", ev.Type.Type, ev.RoomID, ev.Sender)
	})

	debugPrint("Bot started. Listening for messages...\n")
	debugPrint("Press Ctrl+C to stop\n")

	debugPrint("[SYNC] Starting sync...\n")

	go func() {
		err := client.Sync()
		if err != nil {
			debugLog("[ERROR] Sync error: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	debugPrint("\nShutting down gracefully...\n")
	time.Sleep(1 * time.Second)
}

func handleMessage(ctx context.Context, client *mautrix.Client, gemini *GeminiClient, roomID id.RoomID, sender id.UserID, msg *event.MessageEventContent) {
	body := strings.TrimSpace(msg.Body)
	bodyLower := strings.ToLower(body)
	formattedBody := ""
	if msg.FormattedBody != "" {
		formattedBody = msg.FormattedBody
	}

	debugPrint("\n[MESSAGE] Room: %s\n", roomID)
	debugPrint("   From: %s\n", sender)
	debugPrint("   Content: %s\n", body)
	debugPrint("   FormattedBody: %s\n", formattedBody)

	var replyContext string
	if msg.RelatesTo != nil && msg.RelatesTo.InReplyTo != nil {
		replyEventID := msg.RelatesTo.InReplyTo.EventID
		debugPrint("   [DEBUG] Message is a reply to: %s\n", replyEventID)

		replyEvent, err := client.GetEvent(ctx, roomID, replyEventID)
		if err != nil {
			debugPrint("   [WARNING] Could not fetch replied message: %v\n", err)
		} else {
			debugPrint("   [DEBUG] Reply event type: %s\n", replyEvent.Type)

			var replyBody string
			var replySender string

			if replyEvent.Content.AsMessage() != nil {
				replyMsg := replyEvent.Content.AsMessage()
				replyBody = replyMsg.Body
				replySender = replyEvent.Sender.String()
			}

			if replyEvent.Content.Raw != nil {
				if rawBody, ok := replyEvent.Content.Raw["body"].(string); ok && rawBody != "" {
					replyBody = rawBody
				}
			}

			if replyBody != "" {
				replyContext = fmt.Sprintf("[Replying to message from %s: \"%s\"]\n\n", replySender, replyBody)
				debugPrint("   [CONTEXT] Got replied message: %s\n", replyBody)
			} else {
				debugPrint("   [WARNING] Could not extract body from replied message\n")
				debugPrint("   [DEBUG] Reply event content: %+v\n", replyEvent.Content)
			}
		}
	}

	botMention := client.UserID.String()
	debugPrint("   [DEBUG] Bot username: %s\n", botMention)
	debugPrint("   [DEBUG] Checking for bot mention in: %s\n", body)

	hasBotMention := strings.Contains(body, botMention)
	hasGeminiMention := strings.Contains(bodyLower, "@gemini")
	hasAboutCommand := strings.Contains(bodyLower, "/about")
	hasProCommand := strings.Contains(bodyLower, "/pro")

	debugPrint("   [DEBUG] hasBotMention: %v\n", hasBotMention)
	debugPrint("   [DEBUG] hasGeminiMention: %v\n", hasGeminiMention)

	if (hasBotMention || hasGeminiMention) && hasAboutCommand {
		debugPrint("   [COMMAND] /about\n")
		reply := "**Matrix Gemini Bot**\n\n" +
			"**Created by:** Ayush Sharma (@theayusharma)\n" +
			"**GitHub:** https://github.com/theayusharma/matrixGemini\n" +
			"**AI Model:** Google Gemini 2.0 Flash Exp\n" +
			"**Encryption:** Enabled (E2EE Supported)\n\n" +
			"**Usage:**\n" +
			"Mention me with `@test:localhost` or `@gemini` to chat\n" +
			"Use `/about` to see this information\n" +
			"Use `/pro` to use Gemini 2.0 Flash Thinking model for your query\n\n" +
			"*Powered by Matrix & Google Gemini AI*"
		_, _ = client.SendText(ctx, roomID, reply)
		debugPrint("   [SUCCESS] Sent /about response\n")
		return
	}

	if !hasBotMention && !hasGeminiMention {
		debugPrint("   [SKIP] No bot mention found\n")
		return
	}

	if hasBotMention {
		debugPrint("   [DETECTED] Bot mention: %s\n", botMention)
	}
	if hasGeminiMention {
		debugPrint("   [DETECTED] @gemini mention\n")
	}

	clean := body
	clean = strings.ReplaceAll(clean, botMention, "")
	clean = strings.ReplaceAll(clean, "@gemini", "")
	clean = strings.ReplaceAll(clean, "@Gemini", "")
	clean = strings.ReplaceAll(clean, "@GEMINI", "")

	usePro := hasProCommand
	if usePro {
		clean = strings.ReplaceAll(clean, "/pro", "")
		clean = strings.ReplaceAll(clean, "/Pro", "")
		clean = strings.ReplaceAll(clean, "/PRO", "")
	}

	clean = strings.TrimSpace(clean)

	if clean == "" {
		debugPrint("   [WARNING] Empty message after removing mentions\n")
		return
	}

	finalPrompt := clean
	if replyContext != "" {
		finalPrompt = replyContext + clean
	}

	if usePro {
		debugPrint("   [PROMPT] %s (using Gemini Pro)\n", finalPrompt)
	} else {
		debugPrint("   [PROMPT] %s\n", finalPrompt)
	}

	_, _ = client.UserTyping(ctx, roomID, true, 10000)

	debugPrint("   [PROCESSING] Asking Gemini...\n")

	var reply string
	var err error

	if usePro {
		reply, err = gemini.AskPro(ctx, finalPrompt)
	} else {
		reply, err = gemini.Ask(ctx, finalPrompt)
	}

	_, _ = client.UserTyping(ctx, roomID, false, 0)

	if err != nil {
		reply = "Sorry, I encountered an error: " + err.Error()
		debugLog("   [ERROR] Gemini error: %v", err)
	} else {
		debugPrint("   [SUCCESS] Gemini response received (%d chars)\n", len(reply))
	}

	_, err = client.SendText(ctx, roomID, reply)
	if err != nil {
		debugLog("   [ERROR] Failed to send message: %v", err)
	} else {
		debugPrint("   [SENT] Response sent to room\n")
	}

	_, _ = client.UserTyping(ctx, roomID, false, 0)
}
