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

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func main() {

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

	gemini, err := NewGeminiClient(geminiAPIKey)
	if err != nil {
		log.Fatal("Gemini init failed:", err)
	}

	syncer := client.Syncer.(*mautrix.DefaultSyncer)
	syncer.OnEventType(event.EventMessage, func(_ mautrix.EventSource, ev *event.Event) {
		if ev.Sender == client.UserID {
			return
		}

		msg := ev.Content.AsMessage()
		if msg == nil {
			return
		}

		body := strings.TrimSpace(msg.Body)
		botID := client.UserID.String()

		if strings.HasPrefix(strings.ToLower(body), "/about") {
			reply := "**Matrix Gemini Bot**\n\n"
			// +
			// "**Creator:** Ayush Sharma\n" +
			// "**GitHub:** https://github.com/theayusharma/matrixGemini\n" +
			// "**Model:** Google Gemini 2.5 Pro\n\n" +
			// "Mention me with a message to chat!"
			_, _ = client.SendText(ctx, ev.RoomID, reply)
			return
		}

		if !strings.Contains(body, botID) {
			return
		}

		clean := strings.ReplaceAll(body, botID, "")
		clean = strings.TrimSpace(clean)

		if clean == "" {
			return
		}

		go func() {
			_, _ = client.UserTyping(ctx, ev.RoomID, true, 30000) // 30 seconds
		}()

		// Gemini response
		reply, err := gemini.Ask(ctx, clean)
		if err != nil {
			reply = "Sorry, I encountered an error: " + err.Error()
			log.Printf("Gemini error: %v", err)
		}
		_, err = client.SendText(ctx, ev.RoomID, reply)
		if err != nil {
			log.Printf("Failed to send message: %v", err)
		}

		_, _ = client.UserTyping(ctx, ev.RoomID, false, 0)
	})

	fmt.Println("Bot started. Listening for messages...")
	fmt.Println("Press Ctrl+C to stop")

	syncCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		if err := client.SyncWithContext(syncCtx); err != nil {
			log.Printf("Sync error: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down gracefully...")
	cancel()
	time.Sleep(1 * time.Second)
}
