package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AlexGustafsson/svtchat/svt"
	"github.com/gorilla/websocket"
)

func main() {
	address := flag.String("address", "0.0.0.0", "listening address")
	port := flag.Int("port", 8080, "listening port")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	client, err := svt.NewChatClient(ctx)
	cancel()
	if err != nil {
		slog.Error("Failed to connect to SVT", slog.Any("error", err))
		os.Exit(1)
	}
	defer client.Close()

	mux := http.NewServeMux()

	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	mux.HandleFunc("GET /api/v1/messages/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// TODO: Reuse stream
		messages, err := client.Messages(r.Context(), id)
		if err != nil {
			slog.Error("Failed to get messages", slog.Any("error", err))
			return
		}

		for m := range messages {
			if err := conn.WriteJSON(m); err != nil {
				return
			}
		}
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		// 200 OK
	})

	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, r *http.Request) {
		// 200 OK
	})

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", *address, *port),
		Handler: mux,
	}

	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
		caught := 0
		for range signals {
			caught++
			if caught == 1 {
				slog.Info("Caught signal, exiting gracefully")
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					server.Shutdown(ctx)
					cancel()
				}()
			} else {
				slog.Warn("Caught signal, exiting now")
				os.Exit(1)
			}
		}
	}()

	err = server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		slog.Error("Failed to serve API", slog.Any("error", err))
		os.Exit(1)
	}
}
