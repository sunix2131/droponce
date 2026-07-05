package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"droponce/internal/broker"
)

func main() {
	addr := flag.String("addr", ":8091", "HTTP listen address")
	maxSessionMinutes := flag.Int("max-session-minutes", 30, "maximum session lifetime in minutes")
	maxInFlightGB := flag.Int64("max-inflight-gb", 50, "maximum encrypted in-flight message budget per session in GiB")
	flag.Parse()

	server := broker.New(broker.Options{
		MaxSessionDuration: time.Duration(*maxSessionMinutes) * time.Minute,
		MaxInFlightBytes:   *maxInFlightGB * 1024 * 1024 * 1024,
	})
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       0,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	slog.Info("DropOnce broker listening", "addr", *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("broker stopped", "error", err)
		os.Exit(1)
	}
}
