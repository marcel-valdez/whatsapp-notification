package main

import (
  "os"
	"context"
  "flag"
	"fmt"
	"os/exec"
  "strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
)

type targetFlags []string

func (i *targetFlags) String() string {
	return strings.Join(*i, ", ")
}

func (i *targetFlags) Set(value string) error {
	// Allows comma-separated lists OR repeating the flag
	for _, item := range strings.Split(value, ",") {
    trimmedItem := strings.TrimSpace(item)
    fmt.Println("Using target: ", trimmedItem)
		*i = append(*i, trimmedItem)
	}
	return nil
}

var targets targetFlags

func notify(title, body, senderJID string) {
  exec.Command("/usr/bin/notify-send", "--app-name", "WhatsApp", "--urgency", "critical", "--icon", "user-available", title, body).Run()
  fmt.Println("Got message from: %s, body: %s", senderJID, body)
}

func messageHandler(evt interface{}) {
	if v, ok := evt.(*events.Message); ok {
		senderJID := v.Info.Sender.ToNonAD().String()
    isTargetJID := false
    for _, t := range targets {
      if senderJID == t {
        isTargetJID = true
        break
      }
    }

		if isTargetJID {
      title := v.Info.PushName
			body := v.Message.GetConversation()
      notify(title, body, senderJID)
    } else {
      fmt.Printf("Ignoring message from JID: %s\n", senderJID)
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
  // No saved session? Get the QR channel
  qrChan, _ := client.GetQRChannel(ctx)
  err := client.Connect()
  if err != nil {
    panic(err)
  }
  for evt := range qrChan {
    if evt.Event == "code" {
      // This prints the QR code to your terminal
      fmt.Println("Scan this QR code with WhatsApp:")
      qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
    } else {
      fmt.Println("Login event:", evt.Event)
    }
  }
}

func main() {
  // Register the flag list.
	flag.Var(&targets, "target", "JID of VIP(s). Can be comma-separated or repeated.")
	flag.Parse()

	if len(targets) == 0 {
		fmt.Println("Error: Provide at least one target JID via -target.")
		os.Exit(1)
	}
  
  // 1. Create a background context for the initialization
	ctx := context.Background()

	// 2. Setup DB (Updated arguments: ctx, dialect, address, logger)
	container, err := sqlstore.New(ctx, "sqlite3", "file:whatsapp-notification.db?_foreign_keys=on", nil)
	if err != nil {
		panic(err)
	}

	// 3. Get Device (Updated argument: ctx)
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		panic(err)
	}

  // 4. Connect device
	client := whatsmeow.NewClient(deviceStore, nil)
  connect(client, ctx)

	// 5. The Event Handler
	client.AddEventHandler(messageHandler)

  // 6. Wait for program to be killed (Ctrl+C)
	select {}
}
