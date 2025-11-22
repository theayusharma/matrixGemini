package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/mattn/go-sqlite3"

	"rakka/core"
	"rakka/core/llm"
	"rakka/platforms/discord"
	"rakka/platforms/matrix"
)

func main() {
	// parse flags
	var configPath string
	flag.StringVar(&configPath, "config", "config.toml", "Path to config file")
	flag.StringVar(&configPath, "c", "config.toml", "Path to config file (shorthand)")
	flag.Parse()

	// load config
	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// initialize core
	credits := core.NewCreditManager(cfg.Credits)
	defer credits.ForceSave()

	ctxMgr := core.NewContextManager(cfg.Bot.MaxHistory)

	llmProvider, err := llm.New(cfg.LLM)
	if err != nil {
		log.Fatalf("Failed to init LLM: %v", err)
	}

	brain := core.NewBot(llmProvider, &cfg.Bot, credits, ctxMgr)
	core.RegisterDefaultCommands(brain)

	// initialize matrix platform
	if cfg.Matrix.UserID != "" {
		go func() {
			matrixClient, err := matrix.GetMatrixClient(&cfg.Matrix)
			if err != nil {
				log.Printf("❌ Failed to create Matrix client: %v", err)
				return
			}

			err = matrix.InitCrypto(matrixClient, cfg.Matrix.CryptoDBPath, cfg.Matrix.PickleKey)
			if err != nil {
				log.Printf("❌ Failed to initialize Matrix crypto: %v", err)
				return
			}

			adapter := matrix.NewMatrixAdapter(matrixClient, brain, &cfg.Bot, cfg.Matrix.AutoJoinInvites)
			log.Println("🚀 Starting Matrix bot...")
			if err := adapter.Start(); err != nil {
				log.Printf("Matrix Bot failed: %v", err)
			}
		}()
	}

	var discordBot *discord.DiscordAdapter
	if cfg.Discord.Enabled && cfg.Discord.Token != "" {
		discordBot, err = discord.NewDiscordAdapter(cfg.Discord.Token, brain)
		if err != nil {
			log.Fatalf("Failed to create Discord client: %v", err)
		}

		if err := discordBot.Start(); err != nil {
			log.Fatalf("Failed to start Discord bot: %v", err)
		}
		defer discordBot.Close()
	}

	log.Println("🤖 Bot is running on enabled platforms. Press CTRL+C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	log.Println("Shutting down...")
}
