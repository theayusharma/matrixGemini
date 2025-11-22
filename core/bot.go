package core

import (
	"fmt"
	"log"
	"strings"

	"rakka/core/llm"
	"rakka/modules"
)

type BotConfig struct {
	Name              string  `toml:"name"`
	SystemPrompt      string  `toml:"system_prompt"`
	MaxResponseTokens int     `toml:"max_response_tokens"`
	Temperature       float32 `toml:"temperature"`
	MaxHistory        int     `toml:"max_conversational_history"`
}

type Bot struct {
	LLM         llm.Provider
	Config      *BotConfig
	UserCredits *CreditManager
	Context     *ContextManager
	Commands    *CommandRegistry
}

func NewBot(provider llm.Provider, cfg *BotConfig, credits *CreditManager, ctx *ContextManager) *Bot {
	return &Bot{
		LLM:         provider,
		Config:      cfg,
		UserCredits: credits,
		Context:     ctx,
		Commands:    NewCommandRegistry(),
	}
}

func (b *Bot) HandleMessage(msg IncomingMessage, responder Responder) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in HandleMessage: %v", r)
		}
	}()

	prefix := "!" + strings.ToLower(b.Config.Name)
	msgLower := strings.ToLower(msg.Content)

	// command handling
	if strings.HasPrefix(msgLower, prefix) {
		parts := strings.Fields(msg.Content)
		if len(parts) >= 2 {
			cmdName := parts[1]
			args := parts[2:]

			ctx := CommandContext{
				Msg:       msg,
				Responder: responder,
				Bot:       b,
				Args:      args,
			}

			if b.Commands.Execute(cmdName, ctx) {
				return
			}
		}
	}

	// check direct mention
	isDirect := false
	if strings.HasPrefix(msgLower, prefix) {
		isDirect = true
	} else if strings.Contains(msgLower, strings.ToLower(b.Config.Name)) {
		isDirect = true
	}

	if !isDirect {
		return
	}

	// check credits
	if !b.UserCredits.CanUseAPI(msg.UserID) {
		responder.SendText(msg.ChatID, fmt.Sprintf("Sorry, you've reached your API usage limit. Use `!%s llm setkey <your_api_key>` to add your own Gemini API key.", b.Config.Name))
		return
	}

	// process message
	if msg.IsImage {
		b.processImage(&msg, responder)
	} else {
		b.processText(&msg, responder)
	}
}

func (b *Bot) processText(msg *IncomingMessage, responder Responder) {
	prompt := strings.ReplaceAll(msg.Content, b.Config.Name, "")
	prompt = strings.TrimSpace(prompt)

	history := b.Context.GetConversationHistory(msg.ChatID, msg.UserID)
	conversationText := ""
	if history != "" {
		conversationText += "Conversation history:\n" + history + "\n\n"
	}
	conversationText += prompt

	userKey, _ := b.UserCredits.GetUserAPIKey(msg.UserID)
	useSearch := b.UserCredits.IsSearchEnabled(msg.UserID)

	response, tokensUsed, err := b.LLM.GenerateText(conversationText, llm.RequestConfig{
		UserKeyOverride: userKey,
		Temperature:     b.Config.Temperature,
		MaxTokens:       b.Config.MaxResponseTokens,
		SystemPrompt:    b.Config.SystemPrompt,
		UseSearch:       useSearch,
	})
	if err != nil {
		log.Printf("LLM Error: %v", err)
		responder.SendText(msg.ChatID, "I'm having trouble thinking right now.")
		return
	}

	b.Context.AddMessage(msg.ChatID, msg.UserID, "user", prompt)
	b.Context.AddMessage(msg.ChatID, msg.UserID, "bot", response)
	b.UserCredits.RecordUsage(msg.UserID, tokensUsed)

	responder.SendText(msg.ChatID, response)
}

func (b *Bot) processImage(msg *IncomingMessage, responder Responder) {
	responder.SendText(msg.ChatID, "👀 Analyzing image...")

	prompt := strings.ReplaceAll(msg.Content, b.Config.Name, "")
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "Describe the image."
	}

	history := b.Context.GetConversationHistory(msg.ChatID, msg.UserID)

	conversationText := ""
	if history != "" {
		conversationText += "Conversation history:\n" + history + "\n\n"
	}
	conversationText += prompt

	userKey, _ := b.UserCredits.GetUserAPIKey(msg.UserID)
	useSearch := b.UserCredits.IsSearchEnabled(msg.UserID)

	response, tokensUsed, err := b.LLM.GenerateVision(conversationText, msg.ImageData, msg.ImageMimeType, llm.RequestConfig{
		UserKeyOverride: userKey,
		Temperature:     b.Config.Temperature,
		MaxTokens:       b.Config.MaxResponseTokens,
		SystemPrompt:    b.Config.SystemPrompt,
		UseSearch:       useSearch,
	})

	if err != nil {
		log.Printf("Vision Error: %v", err)
		responder.SendText(msg.ChatID, "Error analyzing image.")
		return
	}

	b.Context.AddMessage(msg.ChatID, msg.UserID, "user", prompt)
	b.Context.AddMessage(msg.ChatID, msg.UserID, "bot", response)
	b.UserCredits.RecordUsage(msg.UserID, tokensUsed)

	responder.SendText(msg.ChatID, response)
}

func (b *Bot) handleCommand(msg IncomingMessage, responder Responder) bool {
	parts := strings.Fields(msg.Content)
	if len(parts) < 2 {
		return false
	}

	cmd := strings.ToLower(parts[1])
	args := parts[2:]

	switch cmd {
	case "llm":
		if len(args) < 2 {
			responder.SendText(msg.ChatID, "Usage: `llm <subcommand> <args>`")
			return true
		}
		subcmd := strings.ToLower(args[0])
		subargs := args[1:]

		switch subcmd {
		case "setkey":
			if len(subargs) != 1 {
				responder.SendText(msg.ChatID, "Usage: `llm setkey <your_api_key>`")
				return true
			}
			apiKey := subargs[0]
			b.UserCredits.SetUserAPIKey(msg.UserID, apiKey)
			responder.SendText(msg.ChatID, "✅ Your API key has been set.")

		case "stats":
			tokens, hasKey := b.UserCredits.GetUserStats(msg.UserID)
			resp := fmt.Sprintf("Tokens used: %d", tokens)
			if hasKey {
				resp += " (using your own API key)"
			} else {
				resp += fmt.Sprintf(" (global limit: %d)", b.UserCredits.globalLimit)
			}
			responder.SendText(msg.ChatID, resp)

		case "clear":
			b.Context.ClearConversation(msg.ChatID, msg.UserID)
			responder.SendText(msg.ChatID, "✅ Your conversation history has been cleared.")

		case "enable":
			if len(subargs) < 1 {
				responder.SendText(msg.ChatID, "Usage: `llm enable <feature>`")
				return true
			}
			feature := strings.ToLower(subargs[0])
			b.UserCredits.SetSearchEnabled(msg.UserID, true) // Assuming search is the only feature for now
			responder.SendText(msg.ChatID, fmt.Sprintf("Feature `%s` has been enabled for you.", feature))

		case "disable":
			if len(subargs) < 1 {
				responder.SendText(msg.ChatID, "Usage: `llm disable <feature>`")
				return true
			}
			feature := strings.ToLower(subargs[0])
			b.UserCredits.SetSearchEnabled(msg.UserID, false)
			responder.SendText(msg.ChatID, fmt.Sprintf("Feature `%s` has been disabled for you.", feature))

		default:
			responder.SendText(msg.ChatID, "Unknown llm subcommand. Available subcommands: `setkey`, `enable`, `disable`")
		}
		return true

	case "anime":
		if len(args) < 1 {
			responder.SendText(msg.ChatID, "Usage: `anime <title>`")
			return true
		}
		res, _ := modules.GetAnimeInfo(strings.Join(args, " "))
		responder.SendText(msg.ChatID, res)
		return true

	case "manga":
		if len(args) < 1 {
			responder.SendText(msg.ChatID, "Usage: `manga <title>`")
			return true
		}
		res, _ := modules.GetMangaInfo(strings.Join(args, " "))
		responder.SendText(msg.ChatID, res)
		return true

	case "wiki":
		if len(args) < 1 {
			responder.SendText(msg.ChatID, "Usage: `wiki <term>`")
			return true
		}
		res, _ := modules.GetWikiSummary(strings.Join(args, " "))
		responder.SendText(msg.ChatID, res)
		return true

	case "urban":
		if len(args) < 1 {
			responder.SendText(msg.ChatID, "Usage: `urban <term>`")
			return true
		}
		term := strings.Join(args, " ")
		res, err := modules.GetUrbanDef(term)
		if err != nil {
			responder.SendText(msg.ChatID, "Error: "+err.Error())
		} else {
			responder.SendText(msg.ChatID, res)
		}
		return true

	case "8ball":
		if len(args) < 1 {
			responder.SendText(msg.ChatID, "Usage: `8ball <question>`")
			return true
		}
		question := strings.Join(args, " ")
		responder.SendText(msg.ChatID, modules.Magic8Ball(question))
		return true

	case "roulette":
		responder.SendText(msg.ChatID, modules.RussianRoulette(msg.UserName))
		return true

	case "help":
		responder.SendText(msg.ChatID, "Commands: `anime`, `manga`, `wiki`, `llm setkey`, `llm enable search`.\nOr just chat with me!")
		return true

	default:
		return false
	}
}
