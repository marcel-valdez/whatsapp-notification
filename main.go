// Package main provides a background daemon that monitors WhatsApp messages
// for specific target JIDs and triggers system notifications or custom commands.
//
// It includes a "grace period" logic where it waits a few seconds before notifying
// to see if a read receipt is received from another device, preventing redundant
// notifications for messages already seen on a phone or web client.
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

// targetFlags is a custom flag type to handle multiple target JIDs
// passed via the command line (e.g., -target jid1 -target jid2 or -target jid1,jid2).
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
	targets          targetFlags
	notificationExec string
	// pendingNotifications tracks messages currently in their grace period.
	// The key is the WhatsApp Message ID, and the value is a channel used to cancel the notification.
	pendingNotifications = make(map[string]chan bool)
	pendingMutex         sync.Mutex
)

// notify triggers the actual alert. It prioritizes the custom -exec command
// if provided; otherwise, it falls back to the Linux native notify-send.
func notify(title, body, senderJID string) {
	if notificationExec != "" {
		fmt.Printf("Executing custom notification: %s '%s' '%s'\n", notificationExec, title, body)
		err := exec.Command(notificationExec, title, body).Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing custom notification command: %v\n", err)
		}
		return
	}

	// Default behavior: use notify-send (Linux)
	// Timeout is set to 12 hours so critical messages persist on the desktop.
	timeout := 1000 * 60 * 60 * 12
	err := exec.Command("/usr/bin/notify-send", "--app-name", "WhatsApp", "--urgency", "normal", "--icon", "user-available", "--expire-time", strconv.Itoa(timeout), title, body).Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing notify-send: %v\n", err)
	}
}

// isTarget checks if the provided sender JID matches any of the user-defined targets.
func isTarget(senderJID string) bool {
	for _, t := range targets {
		if senderJID == t {
			return true
		}
	}
	return false
}

// messageHandler processes incoming events from the whatsmeow client.
func messageHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		senderJID := v.Info.Sender.ToNonAD().String()

		// Filter for "real" messages: exclude edits unless they are the final version,
		// and verify the sender is in our VIP target list.
		isNotEdit := ((v.Info.Edit == "" && v.Info.MsgBotInfo.EditType == "") || v.Info.MsgBotInfo.EditType == "last")
		if isNotEdit && isTarget(senderJID) {
			msgID := v.Info.ID
			title := v.Info.PushName
			if title == "" {
				title = senderJID
			}
			body := v.Message.GetConversation()
			if body == "" {
				body = "[Media/Non-text Message]"
			}

			// DEBUG: Statement
			// fmt.Printf("\n--- DEBUG INFO ---\n%+v\n------------------\n", v)
			fmt.Printf("Got message from: %s, id: %s, body: %s\n", senderJID, msgID, body)

			// Create a cancellation channel for the grace-period timer.
			stopChan := make(chan bool, 1)
			pendingMutex.Lock()
			pendingNotifications[msgID] = stopChan
			pendingMutex.Unlock()

			// GRACE PERIOD LOGIC:
			// Start a goroutine that waits 4 seconds before triggering the notification.
			// If a "Read" receipt arrives for this message ID within those 4 seconds,
			// the stopChan will be signaled and the notification will be suppressed.
			go func(id, t, b, jid string, stop chan bool) {
				timer := time.NewTimer(4 * time.Second)
				defer timer.Stop()

				select {
				case <-timer.C:
					fmt.Printf("No read receipt received. Notifying on message [id:%s] from %s\n", id, jid)
					notify(t, b, jid)
				case <-stop:
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
		// Handle read receipts to cancel pending notifications.
		senderJID := v.MessageSender.User + "@" + v.MessageSender.Server
		// Only care about "Read" receipts
		if isTarget(senderJID) && v.Type == types.ReceiptTypeRead {
			pendingMutex.Lock()
			for _, id := range v.MessageIDs {
				fmt.Println("Got a read receipt for message with id:", id)
				if stop, ok := pendingNotifications[id]; ok {
					// Signal the goroutine to cancel the pending notification.
					select {
					case stop <- true:
					default:
					}
				}
			}
			pendingMutex.Unlock()
		}
	}
}

// connect manages the initial connection and authentication (QR code) flow.
func connect(client *whatsmeow.Client, ctx context.Context) {
	if client.Store.ID != nil {
		// Existing session found in SQLite.
		err := client.Connect()
		if err != nil {
			panic(err)
		}
		return
	}

	// No session: Generate QR code for terminal scanning.
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
	flag.StringVar(&notificationExec, "exec", "", "Custom command/script to execute for notifications. Called with 'title' and 'body' as args.")
	flag.Parse()

	if len(targets) == 0 {
		fmt.Println("Error: Provide at least one target JID via -target.")
		os.Exit(1)
	}

	ctx := context.Background()

	// Initialize the local SQLite store for persistent sessions.
	container, err := sqlstore.New(ctx, "sqlite3", "file:whatsapp-notification.db?_foreign_keys=on", nil)
	if err != nil {
		panic(err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		panic(err)
	}

	client := whatsmeow.NewClient(deviceStore, nil)
	connect(client, ctx)

	client.AddEventHandler(messageHandler)

	// Keep the application running.
	select {}
}
