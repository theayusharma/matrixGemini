package core

import (
	"fmt"
	"strings"

	"rakka/modules"
)

type CommandContext struct {
	Msg       IncomingMessage
	Responder Responder
	Bot       *Bot
	Args      []string
}

type CommandHandler func(ctx CommandContext) error

type CommandRegistry struct {
	commands map[string]CommandHandler
}

func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands: make(map[string]CommandHandler),
	}
}

func (r *CommandRegistry) Register(name string, handler CommandHandler) {
	r.commands[strings.ToLower(name)] = handler
}

func (r *CommandRegistry) Execute(name string, ctx CommandContext) bool {
	if handler, exists := r.commands[strings.ToLower(name)]; exists {
		if err := handler(ctx); err != nil {
			_ = ctx.Responder.SendText(ctx.Msg.ChatID, fmt.Sprintf("⚠️ Error executing command: %v", err))
		}
		return true
	}
	return false
}

func RegisterDefaultCommands(b *Bot) {
	b.Commands.Register("help", func(ctx CommandContext) error {
		helpText := "Commands: `anime`, `manga`, `wiki`, `urban`, `8ball`, `roulette`.\n" +
			"LLM Tools: `llm setkey`, `llm stats`, `llm clear`, `llm enable search`.\n" +
			"Or just chat with me!"
		return ctx.Responder.SendText(ctx.Msg.ChatID, helpText)
	})

	b.Commands.Register("anime", func(ctx CommandContext) error {
		if len(ctx.Args) < 1 {
			return ctx.Responder.SendText(ctx.Msg.ChatID, "Usage: `anime <title>`")
		}
		res, err := modules.GetAnimeInfo(strings.Join(ctx.Args, " "))
		if err != nil {
			return ctx.Responder.SendText(ctx.Msg.ChatID, "Error finding anime: "+err.Error())
		}
		return ctx.Responder.SendText(ctx.Msg.ChatID, res)
	})

	b.Commands.Register("manga", func(ctx CommandContext) error {
		if len(ctx.Args) < 1 {
			return ctx.Responder.SendText(ctx.Msg.ChatID, "Usage: `manga <title>`")
		}
		res, err := modules.GetMangaInfo(strings.Join(ctx.Args, " "))
		if err != nil {
			return ctx.Responder.SendText(ctx.Msg.ChatID, "Error finding manga: "+err.Error())
		}
		return ctx.Responder.SendText(ctx.Msg.ChatID, res)
	})

	b.Commands.Register("wiki", func(ctx CommandContext) error {
		if len(ctx.Args) < 1 {
			return ctx.Responder.SendText(ctx.Msg.ChatID, "Usage: `wiki <term>`")
		}
		res, err := modules.GetWikiSummary(strings.Join(ctx.Args, " "))
		if err != nil {
			return ctx.Responder.SendText(ctx.Msg.ChatID, "Error: "+err.Error())
		}
		return ctx.Responder.SendText(ctx.Msg.ChatID, res)
	})

	b.Commands.Register("urban", func(ctx CommandContext) error {
		if len(ctx.Args) < 1 {
			return ctx.Responder.SendText(ctx.Msg.ChatID, "Usage: `urban <term>`")
		}
		res, err := modules.GetUrbanDef(strings.Join(ctx.Args, " "))
		if err != nil {
			return ctx.Responder.SendText(ctx.Msg.ChatID, "Error: "+err.Error())
		}
		return ctx.Responder.SendText(ctx.Msg.ChatID, res)
	})

	b.Commands.Register("8ball", func(ctx CommandContext) error {
		if len(ctx.Args) < 1 {
			return ctx.Responder.SendText(ctx.Msg.ChatID, "Usage: `8ball <question>`")
		}
		question := strings.Join(ctx.Args, " ")
		return ctx.Responder.SendText(ctx.Msg.ChatID, modules.Magic8Ball(question))
	})

	b.Commands.Register("roulette", func(ctx CommandContext) error {
		return ctx.Responder.SendText(ctx.Msg.ChatID, modules.RussianRoulette(ctx.Msg.UserName))
	})

	b.Commands.Register("llm", func(ctx CommandContext) error {
		if len(ctx.Args) < 1 {
			return ctx.Responder.SendText(ctx.Msg.ChatID, "Usage: `llm <subcommand> <args>`\nSubcommands: `setkey`, `stats`, `clear`, `enable`, `disable`")
		}

		subcmd := strings.ToLower(ctx.Args[0])
		subargs := ctx.Args[1:]

		switch subcmd {
		case "setkey":
			if len(subargs) != 1 {
				return ctx.Responder.SendText(ctx.Msg.ChatID, "Usage: `llm setkey <your_api_key>`")
			}
			err := ctx.Bot.UserCredits.SetUserAPIKey(ctx.Msg.UserID, subargs[0])
			if err != nil {
				return ctx.Responder.SendText(ctx.Msg.ChatID, "Failed to securely save API key: "+err.Error())
			}
			return ctx.Responder.SendText(ctx.Msg.ChatID, "✅ Your API key has been set securely.")

		case "stats":
			tokens, hasKey := ctx.Bot.UserCredits.GetUserStats(ctx.Msg.UserID)
			resp := fmt.Sprintf("Tokens used: %d", tokens)
			if hasKey {
				resp += " (using your own API key)"
			} else {
				resp += fmt.Sprintf(" (global limit: %d)", ctx.Bot.UserCredits.globalLimit)
			}
			return ctx.Responder.SendText(ctx.Msg.ChatID, resp)

		case "clear":
			ctx.Bot.Context.ClearConversation(ctx.Msg.ChatID, ctx.Msg.UserID)
			return ctx.Responder.SendText(ctx.Msg.ChatID, "✅ Your conversation history has been cleared.")

		case "enable":
			if len(subargs) < 1 {
				return ctx.Responder.SendText(ctx.Msg.ChatID, "Usage: `llm enable <feature>` (e.g., search)")
			}
			feature := strings.ToLower(subargs[0])
			if feature == "search" {
				ctx.Bot.UserCredits.SetSearchEnabled(ctx.Msg.UserID, true)
				return ctx.Responder.SendText(ctx.Msg.ChatID, "✅ Feature `search` has been enabled for you.")
			}
			return ctx.Responder.SendText(ctx.Msg.ChatID, "Unknown feature. Available: `search`")

		case "disable":
			if len(subargs) < 1 {
				return ctx.Responder.SendText(ctx.Msg.ChatID, "Usage: `llm disable <feature>`")
			}
			feature := strings.ToLower(subargs[0])
			if feature == "search" {
				ctx.Bot.UserCredits.SetSearchEnabled(ctx.Msg.UserID, false)
				return ctx.Responder.SendText(ctx.Msg.ChatID, "🚫 Feature `search` has been disabled for you.")
			}
			return ctx.Responder.SendText(ctx.Msg.ChatID, "Unknown feature. Available: `search`")

		default:
			return ctx.Responder.SendText(ctx.Msg.ChatID, "Unknown llm subcommand.")
		}
	})
}
