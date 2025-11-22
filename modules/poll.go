package modules

import (
	"context"
	"fmt"
	"strings"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var numberEmojis = []string{"1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣", "🔟"}

func CreatePoll(client *mautrix.Client, roomID id.RoomID, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("Usage: `!poll \"Question\" \"Option1\" \"Option2\"...`")
	}

	fullString := strings.Join(args, " ")

	parts := strings.Split(fullString, "\"")
	var cleanParts []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			cleanParts = append(cleanParts, trimmed)
		}
	}

	if len(cleanParts) < 3 {
		return fmt.Errorf("Usage: `!poll \"Question\" \"Option1\" \"Option2\"`")
	}

	question := cleanParts[0]
	options := cleanParts[1:]

	if len(options) > 10 {
		return fmt.Errorf("Max 10 options allowed.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 **%s**\n\n", question))
	for i, opt := range options {
		sb.WriteString(fmt.Sprintf("%s %s\n", numberEmojis[i], opt))
	}

	resp, err := client.SendMessageEvent(context.Background(), roomID, event.EventMessage, &event.MessageEventContent{
		MsgType:       event.MsgText,
		Body:          sb.String(),
		Format:        event.FormatHTML,
		FormattedBody: strings.ReplaceAll(sb.String(), "\n", "<br>"),
	})
	if err != nil {
		return err
	}

	go func() {
		for i := 0; i < len(options); i++ {
			_, _ = client.SendMessageEvent(context.Background(), roomID, event.EventReaction, &event.ReactionEventContent{
				RelatesTo: event.RelatesTo{
					EventID: resp.EventID,
					Type:    event.RelAnnotation,
					Key:     numberEmojis[i],
				},
			})
		}
	}()

	return nil
}
