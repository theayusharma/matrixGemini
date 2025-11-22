package discord

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/bwmarrin/discordgo"
	"rakka/core"
)

type Config struct {
	Enabled bool   `toml:"enabled"`
	Token   string `toml:"token"`
}

type DiscordAdapter struct {
	Session *discordgo.Session
	Core    *core.Bot
	BotID   string
}

func NewDiscordAdapter(token string, coreBot *core.Bot) (*DiscordAdapter, error) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	return &DiscordAdapter{
		Session: dg,
		Core:    coreBot,
	}, nil
}

func (da *DiscordAdapter) Start() error {
	da.Session.AddHandler(da.handleMessage)

	err := da.Session.Open()
	if err != nil {
		return fmt.Errorf("error opening discord connection: %w", err)
	}

	u, err := da.Session.User("@me")
	if err != nil {
		return fmt.Errorf("error fetching self user: %w", err)
	}
	da.BotID = u.ID

	log.Println("Discord adapter started. Logged in as", u.Username)
	return nil
}

func (da *DiscordAdapter) Close() {
	da.Session.Close()
}

func (da *DiscordAdapter) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if m.Author.ID == da.BotID {
		return
	}

	// Prepare the Core IncomingMessage
	incomingMsg := core.IncomingMessage{
		Platform: "discord",
		UserID:   m.Author.ID,
		UserName: m.Author.Username,
		ChatID:   m.ChannelID,
		Content:  m.Content,
	}

	if strings.Contains(m.Content, "<@"+da.BotID+">") || strings.Contains(m.Content, "<@!"+da.BotID+">") {
		incomingMsg.Content = strings.ReplaceAll(incomingMsg.Content, "<@"+da.BotID+">", "")
		incomingMsg.Content = strings.ReplaceAll(incomingMsg.Content, "<@!"+da.BotID+">", "")
		incomingMsg.Content = strings.TrimSpace(incomingMsg.Content)
		incomingMsg.Content = da.Core.Config.Name + " " + incomingMsg.Content
	}

	// Handle Attachments (Images)
	if len(m.Attachments) > 0 {
		att := m.Attachments[0]
		// Simple check for images
		if strings.HasSuffix(strings.ToLower(att.Filename), ".png") ||
			strings.HasSuffix(strings.ToLower(att.Filename), ".jpg") ||
			strings.HasSuffix(strings.ToLower(att.Filename), ".jpeg") ||
			strings.HasSuffix(strings.ToLower(att.Filename), ".webp") {

			data, err := da.downloadAttachment(att.URL)
			if err != nil {
				log.Printf("Failed to download discord attachment: %v", err)
			} else {
				incomingMsg.IsImage = true
				incomingMsg.ImageData = data
				incomingMsg.ImageMimeType = "image/jpeg"
			}
		}
	}

	go da.Core.HandleMessage(incomingMsg, da)
}

func (da *DiscordAdapter) downloadAttachment(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (da *DiscordAdapter) SendText(chatID string, text string) error {
	if len(text) > 2000 {
		text = text[:1990] + "..."
	}
	_, err := da.Session.ChannelMessageSend(chatID, text)
	return err
}

func (da *DiscordAdapter) ReplyText(chatID string, originalMsgID string, text string) error {
	if len(text) > 2000 {
		text = text[:1990] + "..."
	}

	ref := &discordgo.MessageReference{
		MessageID: originalMsgID,
		ChannelID: chatID,
	}

	_, err := da.Session.ChannelMessageSendComplex(chatID, &discordgo.MessageSend{
		Content:   text,
		Reference: ref,
	})
	return err
}

func (da *DiscordAdapter) SendReaction(chatID string, messageID string, emoji string) error {
	return da.Session.MessageReactionAdd(chatID, messageID, emoji)
}
