package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

func main() {
	homeserver := os.Getenv("SERVER_UURRLL")
	username := os.Getenv("USERNAME")
	password := os.Getenv("PASS")

	client, err := mautrix.NewClient(homeserver, "", "")
	if err != nil {
		log.Fatal("failed to create client:", err)
	}

	resp, err := client.Login(&mautrix.ReqLogin{
		Type:     "m.login.password",
		User:     username,
		Password: password,
	})
	if err != nil {
		log.Fatal("login failed:", err)
	}
	client.AccessToken = resp.AccessToken
	client.UserID = resp.UserID
	fmt.Println("Logged in as:", client.UserID)

	syncer := client.Syncer.(*mautrix.DefaultSyncer)
	syncer.OnEventType(event.EventMessage, func(source mautrix.EventSource, ev *event.Event) {
		if ev.Sender == client.UserID {
			return
		}

		body := ev.Content.AsMessage().Body
		if strings.ToLower(body) == "ping" {
			// show typing for 2 seconds
			client.SendTyping(ev.RoomID, true, 2*time.Second)
			time.Sleep(2 * time.Second)
			client.SendTyping(ev.RoomID, false, 0)
			client.SendText(ev.RoomID, "pong")
		}
	})

	log.Println("Bot started. Listening for messages...")
	err = client.Sync()
	if err != nil {
		log.Fatal("sync error:", err)
	}
}
