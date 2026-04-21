package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type targetFlags []string

func (i *targetFlags) String() string {
	return strings.Join(*i, ", ")
}

func (i *targetFlags) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		trimmedItem := strings.TrimSpace(item)
		fmt.Println("Using target: ", trimmedItem)
		*i = append(*i, trimmedItem)
	}
	return nil
}

var (
	targets              targetFlags
	pendingNotifications = make(map[string]chan bool)
	pendingMutex         sync.Mutex
)

func notify(title, body, senderJID string) {
	timeout := 1000 * 60 * 60 * 12 // 12 hours
	exec.Command("/usr/bin/notify-send", "--app-name", "WhatsApp", "--urgency", "critical", "--icon", "user-available", "--expire-time", strconv.Itoa(timeout), title, body).Run()
}

func isTarget(senderJID string) bool {
	for _, t := range targets {
		if senderJID == t {
			return true
		}
	}
	return false
}

func messageHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		senderJID := v.Info.Sender.ToNonAD().String()

		isNotEdit := ((v.Info.Edit == "" && v.Info.MsgBotInfo.EditType == "") || v.Info.MsgBotInfo.EditType == "last")
		if isNotEdit && isTarget(senderJID) {
			msgID := v.Info.ID
			title := v.Info.PushName
			body := v.Message.GetConversation()
			if body == "" {
				body = "[Media/Non-text Message]"
			}

			// DEBUG: Statement
			// fmt.Printf("\n--- DEBUG INFO ---\n%+v\n------------------\n", v)
			fmt.Printf("Got message from: %s, id: %s, body: %s\n", senderJID, msgID, body)
			// Create a cancellation channel for this specific message
			stopChan := make(chan bool, 1)
			pendingMutex.Lock()
			pendingNotifications[msgID] = stopChan
			pendingMutex.Unlock()

			// Start the grace period timer
			go func(id, t, b, jid string, stop chan bool) {
				timer := time.NewTimer(4 * time.Second)
				defer timer.Stop()

				select {
				case <-timer.C:
					// 2 seconds passed without a "Read" receipt
					fmt.Printf("No read receipt received. Notifying on message [id:%s] from %s\n", id, jid)
					notify(t, b, jid)
				case <-stop:
					// "Read" receipt arrived within 2 seconds
					fmt.Printf("Notification [id:%s] suppressed for %s (Message read on another device)\n", id, jid)
				}

				pendingMutex.Lock()
				delete(pendingNotifications, id)
				pendingMutex.Unlock()
			}(msgID, title, body, senderJID, stopChan)

		} else {
			fmt.Printf("Ignoring message from JID: %s\n", senderJID)
		}

	case *events.Receipt:
		// fmt.Printf("\n--- DEBUG INFO ---\n%+v\n------------------\n", v)
		senderJID := v.MessageSender.User + "@" + v.MessageSender.Server
		// Only care about "Read" receipts
		if isTarget(senderJID) && v.Type == types.ReceiptTypeRead {
			pendingMutex.Lock()
			for _, id := range v.MessageIDs {
				fmt.Println("Got a read receipt for message with id:", id)
				if stop, ok := pendingNotifications[id]; ok {
					// Signal the goroutine to stop/cancel the notification
					select {
					case stop <- true:
					default:
					}
				}
			}
			pendingMutex.Unlock()
		} else {
			for _, id := range v.MessageIDs {
				fmt.Printf("Ignoring read receipt for message with id: %s from %s\n", id, senderJID)
			}
		}
	}
}

func connect(client *whatsmeow.Client, ctx context.Context) {
	if client.Store.ID != nil {
		err := client.Connect()
		if err != nil {
			panic(err)
		}
		return
	}

	qrChan, _ := client.GetQRChannel(ctx)
	err := client.Connect()
	if err != nil {
		panic(err)
	}

	for evt := range qrChan {
		if evt.Event == "code" {
			fmt.Println("Scan this QR code with WhatsApp:")
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
		} else {
			fmt.Println("Login event:", evt.Event)
		}
	}
}

func main() {
	flag.Var(&targets, "target", "JID of VIP(s). Can be comma-separated or repeated.")
	flag.Parse()

	if len(targets) == 0 {
		fmt.Println("Error: Provide at least one target JID via -target.")
		os.Exit(1)
	}

	ctx := context.Background()

	// Using the original DB name from your context
	// Added 'ctx' as the first argument as required by the library version
	container, err := sqlstore.New(ctx, "sqlite3", "file:whatsapp-notification.db?_foreign_keys=on", nil)
	if err != nil {
		panic(err)
	}

	// Added 'ctx' as an argument as required by the library version
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		panic(err)
	}

	client := whatsmeow.NewClient(deviceStore, nil)
	connect(client, ctx)

	client.AddEventHandler(messageHandler)

	select {}
}
