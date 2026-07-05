package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"droponce/internal/relay"
)

func main() {
	addr := flag.String("addr", ":8088", "HTTP listen address")
	storage := flag.String("storage", "", "directory for temporary relay files")
	publicURL := flag.String("public-url", "", "public base URL, for example https://relay.example.com")
	maxUploadGB := flag.Int64("max-upload-gb", 50, "maximum single file upload size in GiB")
	flag.Parse()

	server, err := relay.NewWithOptions(relay.Options{
		Storage:        *storage,
		PublicURL:      *publicURL,
		MaxUploadBytes: *maxUploadGB * 1024 * 1024 * 1024,
	})
	if err != nil {
		slog.Error("relay init failed", "error", err)
		os.Exit(1)
	}
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       0,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	slog.Info("DropOnce relay listening", "addr", *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("relay stopped", "error", err)
		os.Exit(1)
	}
}
