package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
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

	var olmMachine *crypto.OlmMachine
	fmt.Println("[INFO] Encryption support disabled - bot will only work in unencrypted rooms")

	gemini, err := NewGeminiClient(geminiAPIKey)
	if err != nil {
		log.Fatal("Gemini init failed:", err)
	}

	syncer := client.Syncer.(*mautrix.DefaultSyncer)

	joinedRooms := make(map[id.RoomID]bool)
	
	startTime := time.Now()
	fmt.Printf("[INFO] Bot started at: %v - will only respond to messages after this time\n", startTime)
	
	existingRooms, err := client.JoinedRooms(ctx)
	if err == nil {
		for _, roomID := range existingRooms.JoinedRooms {
			joinedRooms[roomID] = true
		}
		fmt.Printf("[INFO] Already in %d rooms\n", len(existingRooms.JoinedRooms))
	}

	syncer.OnEventType(event.StateMember, func(ctx context.Context, ev *event.Event) {
		if ev.GetStateKey() == client.UserID.String() && ev.Content.AsMember().Membership == event.MembershipInvite {
			if joinedRooms[ev.RoomID] {
				return
			}

			resp, err := client.JoinRoomByID(ctx, ev.RoomID)
			if err != nil {
				log.Printf("[ERROR] Failed to join room %s: %v", ev.RoomID, err)
			} else {
				joinedRooms[ev.RoomID] = true
				fmt.Printf("[JOINED] Room: %s\n", resp.RoomID)
			}
		}
	})

	syncer.OnEventType(event.EventEncrypted, func(ctx context.Context, ev *event.Event) {
		if olmMachine == nil {
			fmt.Println("[DEBUG] Encrypted event received but no OlmMachine")
			return
		}

		if time.UnixMilli(ev.Timestamp).Before(startTime) {
			fmt.Println("[INFO] Skipping old encrypted message from before bot start")
			return
		}

		fmt.Printf("[DEBUG] Attempting to decrypt message from %s in room %s\n", ev.Sender, ev.RoomID)
		
		decrypted, err := olmMachine.DecryptMegolmEvent(ctx, ev)
		if err != nil {
			fmt.Printf("[ERROR] Failed to decrypt message: %v\n", err)
			fmt.Printf("[ERROR] Event details - Sender: %s, Room: %s, Type: %s\n", ev.Sender, ev.RoomID, ev.Type)
			return
		}

		fmt.Printf("[SUCCESS] Decrypted message successfully\n")

		if decrypted.Type != event.EventMessage {
			return
		}

		if decrypted.Sender == client.UserID {
			return
		}

		msg := decrypted.Content.AsMessage()
		if msg == nil {
			return
		}

		handleMessage(ctx, client, gemini, decrypted.RoomID, decrypted.Sender, msg)
	})

	syncer.OnEventType(event.EventMessage, func(ctx context.Context, ev *event.Event) {
		if ev.Sender == client.UserID {
			return
		}

		if time.UnixMilli(ev.Timestamp).Before(startTime) {
			fmt.Println("[INFO] Skipping old unencrypted message from before bot start")
			return
		}

		msg := ev.Content.AsMessage()
		if msg == nil {
			return
		}

		handleMessage(ctx, client, gemini, ev.RoomID, ev.Sender, msg)
	})

	syncer.OnEvent(func(ctx context.Context, ev *event.Event) {
		fmt.Printf("[EVENT] Type: %s, Room: %s, Sender: %s\n", ev.Type.Type, ev.RoomID, ev.Sender)
		
		if ev.Type == event.EventEncrypted && olmMachine == nil {
			fmt.Printf("[WARNING] Received encrypted message - bot cannot read encrypted rooms!\n")
			fmt.Printf("[WARNING] Please use the bot in an unencrypted room or disable encryption.\n")
		}
	})

	fmt.Println("Bot started. Listening for messages...")
	fmt.Println("Press Ctrl+C to stop")

	fmt.Println("[SYNC] Starting sync...")
	
	go func() {
		err := client.Sync()
		if err != nil {
			log.Printf("[ERROR] Sync error: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down gracefully...")
	time.Sleep(1 * time.Second)
}

func handleMessage(ctx context.Context, client *mautrix.Client, gemini *GeminiClient, roomID id.RoomID, sender id.UserID, msg *event.MessageEventContent) {
	body := strings.TrimSpace(msg.Body)
	bodyLower := strings.ToLower(body)
	formattedBody := ""
	if msg.FormattedBody != "" {
		formattedBody = msg.FormattedBody
	}

	fmt.Printf("\n[MESSAGE] Room: %s\n", roomID)
	fmt.Printf("   From: %s\n", sender)
	fmt.Printf("   Content: %s\n", body)
	fmt.Printf("   FormattedBody: %s\n", formattedBody)

	if strings.HasPrefix(bodyLower, "/about") {
		fmt.Println("   [COMMAND] /about")
		reply := "**Matrix Gemini Bot**\n\n" +
			"**Creator:** Ayush Sharma\n" +
			"**GitHub:** https://github.com/theayusharma/matrixGemini\n" +
			"**Model:** Google Gemini 2.0 Flash Exp\n\n" +
			"Mention me or use @gemini to chat!"
		_, _ = client.SendText(ctx, roomID, reply)
		fmt.Println("   [SUCCESS] Sent /about response")
		return
	}

	botMention := client.UserID.String()
	fmt.Printf("   [DEBUG] Bot username: %s\n", botMention)
	fmt.Printf("   [DEBUG] Checking for bot mention in: %s\n", body)
	
	hasBotMention := strings.Contains(body, botMention)
	hasGeminiMention := strings.Contains(bodyLower, "@gemini")
	
	fmt.Printf("   [DEBUG] hasBotMention: %v\n", hasBotMention)
	fmt.Printf("   [DEBUG] hasGeminiMention: %v\n", hasGeminiMention)

	if !hasBotMention && !hasGeminiMention {
		fmt.Println("   [SKIP] No bot mention found")
		return
	}

	if hasBotMention {
		fmt.Printf("   [DETECTED] Bot mention: %s\n", botMention)
	}
	if hasGeminiMention {
		fmt.Println("   [DETECTED] @gemini mention")
	}

	clean := body
	clean = strings.ReplaceAll(clean, botMention, "")
	clean = strings.ReplaceAll(clean, "@gemini", "")
	clean = strings.ReplaceAll(clean, "@Gemini", "")
	clean = strings.ReplaceAll(clean, "@GEMINI", "")
	clean = strings.TrimSpace(clean)

	if clean == "" {
		fmt.Println("   [WARNING] Empty message after removing mentions")
		return
	}

	fmt.Printf("   [PROMPT] %s\n", clean)

	go func() {
		_, _ = client.UserTyping(ctx, roomID, true, 30000)
	}()

	fmt.Println("   [PROCESSING] Asking Gemini...")

	reply, err := gemini.Ask(ctx, clean)
	if err != nil {
		reply = "Sorry, I encountered an error: " + err.Error()
		log.Printf("   [ERROR] Gemini error: %v", err)
	} else {
		fmt.Printf("   [SUCCESS] Gemini response received (%d chars)\n", len(reply))
	}

	_, err = client.SendText(ctx, roomID, reply)
	if err != nil {
		log.Printf("   [ERROR] Failed to send message: %v", err)
	} else {
		fmt.Println("   [SENT] Response sent to room")
	}

	_, _ = client.UserTyping(ctx, roomID, false, 0)
}
