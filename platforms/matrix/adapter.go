package matrix

import (
	"context"
	"log"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"rakka/core"
)

type MatrixAdapter struct {
	Client   *mautrix.Client
	Core     *core.Bot
	Config   *core.BotConfig
	AutoJoin bool
}

func NewMatrixAdapter(client *mautrix.Client, coreBot *core.Bot, config *core.BotConfig, autoJoin bool) *MatrixAdapter {
	return &MatrixAdapter{
		Client:   client,
		Core:     coreBot,
		Config:   config,
		AutoJoin: autoJoin,
	}
}

func (ma *MatrixAdapter) Start() error {
	syncer := ma.Client.Syncer.(*mautrix.DefaultSyncer)

	// handle messages
	syncer.OnEventType(event.EventMessage, ma.handleEvent)
	// syncer.OnEventType(event.EventEncrypted, ma.handleEvent)

	// handle invites
	syncer.OnEventType(event.StateMember, ma.handleInvite)

	log.Println("Starting Matrix adapter...")
	return ma.Client.Sync()
}

func (ma *MatrixAdapter) handleInvite(ctx context.Context, evt *event.Event) {
	if !ma.AutoJoin {
		return
	}

	state := evt.Content.AsMember()
	isForMe := evt.GetStateKey() == ma.Client.UserID.String()

	if state.Membership == event.MembershipInvite && isForMe {
		log.Printf("Received invite from %s for room %s. Joining...", evt.Sender, evt.RoomID)

		_, err := ma.Client.JoinRoom(ctx, evt.RoomID.String(), nil)

		if err != nil {
			log.Printf("Failed to join room: %v", err)
		} else {
			log.Printf("✅ Successfully joined room %s", evt.RoomID)
			ma.SendText(string(evt.RoomID), "Hello! I am connected and ready.")
		}
	}
}

func (ma *MatrixAdapter) SendText(chatID string, text string) error {
	_, err := ma.Client.SendMessageEvent(context.Background(), id.RoomID(chatID), event.EventMessage, &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    text,
	})
	return err
}

func (ma *MatrixAdapter) ReplyText(chatID string, originalMsgID string, text string) error {
	_, err := ma.Client.SendMessageEvent(context.Background(), id.RoomID(chatID), event.EventMessage, &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    text,
		RelatesTo: &event.RelatesTo{
			InReplyTo: &event.InReplyTo{
				EventID: id.EventID(originalMsgID),
			},
		},
	})
	return err
}

func (ma *MatrixAdapter) SendReaction(chatID string, messageID string, emoji string) error {
	_, err := ma.Client.SendMessageEvent(context.Background(), id.RoomID(chatID), event.EventReaction, &event.ReactionEventContent{
		RelatesTo: event.RelatesTo{
			EventID: id.EventID(messageID),
			Key:     emoji,
		},
	})
	return err
}

func (ma *MatrixAdapter) downloadImage(ctx context.Context, content *event.MessageEventContent) ([]byte, string, error) {
	var data []byte
	var err error
	var mimeType string

	if content.File != nil {
		// encrypted image
		cURI, _ := content.File.URL.Parse()
		ciphertext, errDownload := ma.Client.DownloadBytes(ctx, cURI)

		if errDownload == nil {
			err = content.File.DecryptInPlace(ciphertext)
			if err == nil {
				data = ciphertext
			}
		}
	} else if content.URL != "" {
		// standard image
		cURI, _ := content.URL.Parse()
		data, err = ma.Client.DownloadBytes(ctx, cURI)
	}

	if info := content.GetInfo(); info != nil {
		mimeType = info.MimeType
	}

	return data, mimeType, err
}

func (ma *MatrixAdapter) handleEvent(ctx context.Context, evt *event.Event) {
	if evt.Sender == ma.Client.UserID || time.Since(time.UnixMilli(evt.Timestamp)) > 2*time.Minute {
		return
	}

	msgContent, ok := evt.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		return
	}

	incomingMsg := core.IncomingMessage{
		Platform: "matrix",
		UserID:   string(evt.Sender),
		UserName: string(evt.Sender),
		ChatID:   string(evt.RoomID),
		Content:  msgContent.Body,
	}

	if msgContent.MsgType == event.MsgImage {
		data, mime, err := ma.downloadImage(ctx, msgContent)
		if err == nil && data != nil {
			incomingMsg.IsImage = true
			incomingMsg.ImageData = data
			incomingMsg.ImageMimeType = mime
		}
	}

	if !incomingMsg.IsImage && msgContent.RelatesTo != nil && msgContent.RelatesTo.InReplyTo != nil {
		replyID := msgContent.RelatesTo.InReplyTo.EventID

		replyEvt, err := ma.Client.GetEvent(ctx, evt.RoomID, replyID)
		if err != nil {
			log.Printf("❌ Failed to fetch reply event: %v", err)
		} else {
			replyEvt.RoomID = evt.RoomID

			_ = replyEvt.Content.ParseRaw(replyEvt.Type)

			if replyEvt.Type == event.EventEncrypted {
				if ma.Client.Crypto != nil {
					decryptedEvt, err := ma.Client.Crypto.Decrypt(ctx, replyEvt)
					if err == nil {
						replyEvt = decryptedEvt
					} else {
						log.Printf("❌ Failed to decrypt replied event: %v", err)
					}
				}
			}

			_ = replyEvt.Content.ParseRaw(replyEvt.Type)

			if replyContent, ok := replyEvt.Content.Parsed.(*event.MessageEventContent); ok {
				if replyContent.MsgType == event.MsgImage {
					log.Println("🖼️ Found image in reply history. Downloading...")
					data, mime, err := ma.downloadImage(ctx, replyContent)
					if err == nil && data != nil {
						incomingMsg.IsImage = true
						incomingMsg.ImageData = data
						incomingMsg.ImageMimeType = mime
					} else {
						log.Printf("❌ Failed to download/decrypt image data: %v", err)
					}
				}
			}
		}
	}

	go ma.Core.HandleMessage(incomingMsg, ma)
}
