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

	debugPrint("Logged in as %s\n", resp.UserID)
	debugPrint("Access Token: %s\n", resp.AccessToken)

	debugPrint("[INFO] Setting up encryption support...\n")

	cryptoHelper, err := cryptohelper.NewCryptoHelper(client, []byte("go-neb-password"), "matrix-bot-crypto.db")

	if err != nil {
		debugLog("[WARNING] Failed to create crypto helper: %v", err)
		debugPrint("[WARNING] Bot will work in unencrypted rooms only\n")
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
		} else {
			client.Crypto = cryptoHelper
			debugPrint("[SUCCESS] Encryption support enabled\n")
		}
	}

	syncer := client.Syncer.(*mautrix.DefaultSyncer)
	syncer.ParseEventContent = true
	syncer.ParseErrorHandler = func(evt *event.Event, err error) bool {
		debugLog("[CRYPTO ERROR] Failed to parse/decrypt event %s in %s: %v", evt.ID, evt.RoomID, err)
		return true
	}

	gemini, err := NewGeminiClient(geminiAPIKey)
	if err != nil {
		log.Fatal("Gemini init failed:", err)
	}

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
		debugPrint("[ENCRYPTED] Received encrypted event in room %s from %s at %v\n", ev.RoomID, ev.Sender, time.UnixMilli(ev.Timestamp))
		debugPrint("[ENCRYPTED] Event will be auto-decrypted by cryptohelper\n")
	})

	syncer.OnEventType(event.EventMessage, func(ctx context.Context, ev *event.Event) {
		if ev.Sender == client.UserID {
			debugPrint("[SKIP] Ignoring message from self\n")
			return
		}

		msgTime := time.UnixMilli(ev.Timestamp)
		if msgTime.Before(startTime) {
			debugPrint("[INFO] Skipping old message from before bot start (msg: %v, start: %v)\n", msgTime, startTime)
			return
		}

		msg := ev.Content.AsMessage()
		if msg == nil {
			debugPrint("[WARNING] AsMessage() returned nil for event %s\n", ev.ID)
			return
		}

		debugPrint("[MESSAGE RECEIVED] Processing message from %s in %s\n", ev.Sender, ev.RoomID)
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
		debugPrint("   [DEBUG] Message is a reply, fetching conversation thread...\n")

		const maxThreadDepth = 5
		type ThreadMessage struct {
			Sender string
			Body   string
		}
		var thread []ThreadMessage

		currentEventID := msg.RelatesTo.InReplyTo.EventID

		for i := 0; i < maxThreadDepth && currentEventID != ""; i++ {
			debugPrint("   [DEBUG] Fetching message %d in thread: %s\n", i+1, currentEventID)

			replyEvent, err := client.GetEvent(ctx, roomID, currentEventID)
			if err != nil {
				debugPrint("   [WARNING] Could not fetch message in thread: %v\n", err)
				break
			}

			debugPrint("   [DEBUG] Reply event type: %s\n", replyEvent.Type)

			var messageBody string
			var messageSender string
			var nextEventID id.EventID

			if replyEvent.Content.AsMessage() != nil {
				replyMsg := replyEvent.Content.AsMessage()
				messageBody = replyMsg.Body
				messageSender = replyEvent.Sender.String()
				debugPrint("   [DEBUG] Extracted from AsMessage: body='%s', sender='%s'\n", messageBody, messageSender)

				if replyMsg.RelatesTo != nil && replyMsg.RelatesTo.InReplyTo != nil {
					nextEventID = replyMsg.RelatesTo.InReplyTo.EventID
					debugPrint("   [DEBUG] Found reply chain continues to: %s\n", nextEventID)
				} else {
					debugPrint("   [DEBUG] No more replies in chain (RelatesTo is nil or no InReplyTo)\n")
				}
			} else {
				debugPrint("   [DEBUG] AsMessage() returned nil\n")
			}

			if messageBody == "" && replyEvent.Content.Raw != nil {
				debugPrint("   [DEBUG] Trying to extract from Raw content...\n")
				if rawBody, ok := replyEvent.Content.Raw["body"].(string); ok && rawBody != "" {
					messageBody = rawBody
					messageSender = replyEvent.Sender.String()
					debugPrint("   [DEBUG] Extracted from Raw: body='%s', sender='%s'\n", messageBody, messageSender)

					if relatesTo, ok := replyEvent.Content.Raw["m.relates_to"].(map[string]interface{}); ok {
						if inReplyTo, ok := relatesTo["m.in_reply_to"].(map[string]interface{}); ok {
							if eventID, ok := inReplyTo["event_id"].(string); ok {
								nextEventID = id.EventID(eventID)
								debugPrint("   [DEBUG] Found reply chain in Raw continues to: %s\n", nextEventID)
							}
						}
					}
				}
			}

			if messageBody != "" {
				thread = append(thread, ThreadMessage{
					Sender: messageSender,
					Body:   messageBody,
				})
				debugPrint("   [CONTEXT] Thread message %d from %s: %s\n", i+1, messageSender, messageBody)
				currentEventID = nextEventID
			} else {
				debugPrint("   [WARNING] Could not extract message body, stopping thread traversal\n")
				break
			}
		}

		if len(thread) > 0 {
			replyContext = "[Previous conversation context]\n\n"
			for i := len(thread) - 1; i >= 0; i-- {
				replyContext += fmt.Sprintf("%s: %s\n", thread[i].Sender, thread[i].Body)
			}
			replyContext += "\n[End of context]\n\n"
			debugPrint("   [SUCCESS] Built conversation context with %d messages\n", len(thread))
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
			"**Default Model:** Google Gemini 2.0 Flash :)\n" +
			"**Encryption:** E2EE Supported\n\n" +
			"**Usage:**\n" +
			"Mention me with `@test:localhost` or `@gemini` to chat\n" +
			"Use `/about` to see this information\n" +
			"Use `/pro` to use Gemini 2.5 Pro model for your query\n\n" +
			"*Powered by Matrix & Google Gemini AI & Cats*"
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

	debugPrint("   [DEBUG] Sending typing indicator...\n")
	typingResp, typingErr := client.UserTyping(ctx, roomID, true, 30000)
	if typingErr != nil {
		debugLog("   [ERROR] Failed to send initial typing indicator: %v", typingErr)
	} else {
		debugPrint("   [DEBUG] Typing indicator sent successfully: %+v\n", typingResp)
	}

	time.Sleep(200 * time.Millisecond)

	stopTyping := make(chan bool, 1)
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopTyping:
				return
			case <-ticker.C:
				_, err := client.UserTyping(ctx, roomID, true, 30000)
				if err != nil {
					debugLog("   [ERROR] Failed to refresh typing indicator: %v", err)
				} else {
					debugPrint("   [DEBUG] Typing indicator refreshed at %v\n", time.Now().Format("15:04:05"))
				}
			}
		}
	}()

	debugPrint("   [PROCESSING] Asking Gemini...\n")

	var reply string
	var err error

	if usePro {
		reply, err = gemini.AskPro(ctx, finalPrompt)
	} else {
		reply, err = gemini.Ask(ctx, finalPrompt)
	}

	select {
	case stopTyping <- true:
	default:
	}
	time.Sleep(100 * time.Millisecond)
	_, _ = client.UserTyping(ctx, roomID, false, 0)
	debugPrint("   [DEBUG] Stopped typing indicator\n")

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
}
