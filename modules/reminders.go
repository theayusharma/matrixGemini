package modules

import (
	"context"
	"fmt"
	"strings"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func SetReminder(client *mautrix.Client, roomID id.RoomID, userID id.UserID, args []string) (string, error) {
	if len(args) < 2 {
		return "Usage: `!remind <duration> <message>` (e.g., `!remind 10m Pizza is ready`)", nil
	}

	durationStr := args[0]
	message := strings.Join(args[1:], " ")

	d, err := time.ParseDuration(durationStr)
	if err != nil {
		return "", fmt.Errorf("invalid time format. Use 10m, 1h, 30s, etc.")
	}

	if d > 24*time.Hour {
		return "", fmt.Errorf("max reminder time is 24 hours.")
	}

	go func() {
		time.Sleep(d)

		reminderText := fmt.Sprintf("🔔 **REMINDER** for <@%s>: %s", userID, message)
		_, _ = client.SendMessageEvent(context.Background(), roomID, event.EventMessage, &event.MessageEventContent{
			MsgType:       event.MsgText,
			Body:          reminderText,
			Format:        event.FormatHTML,
			FormattedBody: fmt.Sprintf("🔔 <b>REMINDER</b> for <a href='https://matrix.to/#/%s'>%s</a>: %s", userID, userID, message),
		})
	}()

	return fmt.Sprintf("⏰ I'll remind you in %s: \"%s\"", d.String(), message), nil
}
