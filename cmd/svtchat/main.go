package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/AlexGustafsson/svtchat/svt"
)

func main() {
	id := "67d8a71ba5107a7114cb72e0bbc5b40b" // Den Stora Älgvandringen

	client, err := svt.NewChatClient(context.Background())
	if err != nil {
		slog.Error("Failed to create chat client", slog.Any("error", err))
		os.Exit(1)
	}
	defer client.Close()

	var wg sync.WaitGroup

	var mutex sync.Mutex
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")

	wg.Go(func() {
		messages, err := client.Pinned(context.Background(), id)
		if err != nil {
			slog.Error("Failed to get messages", slog.Any("error", err))
			os.Exit(1)
		}

		for m := range messages {
			mutex.Lock()
			encoder.Encode(m)
			mutex.Unlock()
		}
	})

	wg.Go(func() {
		messages, err := client.Messages(context.Background(), id)
		if err != nil {
			slog.Error("Failed to get messages", slog.Any("error", err))
			os.Exit(1)
		}

		for m := range messages {
			mutex.Lock()
			encoder.Encode(m)
			mutex.Unlock()
		}
	})

	wg.Go(func() {
		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			rain, err := client.GetEmojiRain(context.Background(), id)
			if err != nil {
				slog.Error("Failed to get emoji rain", slog.Any("error", err))
				os.Exit(1)
			}

			mutex.Lock()
			encoder.Encode(rain)
			mutex.Unlock()
		}
	})

	wg.Wait()
}
