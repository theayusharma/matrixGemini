package main

import (
	"context"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
	"maunium.net/go/mautrix/crypto/cryptohelper"
)

func main() {
	// load config
	config, err := LoadConfig("config.toml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// get matrix client
	client, err := GetMatrixClient(&config.Matrix)
	if err != nil {
		log.Fatalf("Auth failed: %v", err)
	}

	fmt.Println("✅ Successfully logged in as", config.Matrix.UserID)
	fmt.Println("Device ID:", client.DeviceID)

	// setup Encryption
	if config.Matrix.CryptoDBPath != "" {
		pickleKey := []byte(config.Matrix.PickleKey)
		if len(pickleKey) == 0 {
			pickleKey = []byte("default-pickle-key")
		}

		cryptoHelper, err := cryptohelper.NewCryptoHelper(client, pickleKey, config.Matrix.CryptoDBPath)
		if err != nil {
			log.Fatalf("Failed to create crypto helper: %v", err)
		}

		err = cryptoHelper.Init(context.Background())
		if err != nil {
			log.Fatalf("Failed to init crypto: %v", err)
		}
		client.Crypto = cryptoHelper
		fmt.Println("🔒 End-to-End Encryption initialized")
	} else {
		fmt.Println("⚠️ Warning: Crypto DB path not set. E2EE disabled.")
	}

	// matrix connection check
	rooms, err := client.JoinedRooms(context.Background())
	if err != nil {
		log.Fatalf("Failed to get joined rooms: %v", err)
	}
	fmt.Println("Joined rooms:", len(rooms.JoinedRooms))

	if config.Bot.Name != "" {
		fmt.Printf("Updating profile display name to: %q...\n", config.Bot.Name)
		if err := client.SetDisplayName(context.Background(), config.Bot.Name); err != nil {
			log.Printf("⚠️ Warning: Failed to set display name: %v", err)
		}
	}

	// gemini connection check
	fmt.Println("\nGemini connection...")
	err = TestGemini(&config.Gemini, config.Bot.SystemPrompt)
	if err != nil {
		log.Fatalf("Gemini connection failed: %v", err)
	}

	// initialize credit manager
	creditManager := NewCreditManager(
		config.Credits.FilePath,
		config.Credits.GlobalLimit,
		config.Credits.MasterKey,
	)

	fmt.Println("\n🚀 Starting bot...")

	// create Gemini client
	geminiClient := &GeminiClient{
		APIKey:  config.Gemini.APIKey,
		BaseURL: config.Gemini.BaseURL,
		Model:   config.Gemini.Model,
	}

	// create and start bot
	bot := NewBot(client, geminiClient, &config.Bot, creditManager, config.Matrix.AutoJoinInvites)
	if err := bot.Start(); err != nil {
		log.Fatalf("Bot failed: %v", err)
	}
}
