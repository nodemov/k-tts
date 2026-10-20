// Command backend serves the Web Audio Tools API (TTS, server-side ffmpeg,
// AI job proxy) and the built SvelteKit frontend.
//
//	go run ./web-audio-tools/backend            # from the k-tts repo root
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"

	"k-tts/web-audio-tools/backend/internal/ai"
	"k-tts/web-audio-tools/backend/internal/jobs"
	"k-tts/web-audio-tools/backend/internal/media"
	"k-tts/web-audio-tools/backend/internal/server"
	"k-tts/web-audio-tools/backend/internal/uploads"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return fallback
}

func main() {
	addr := flag.String("addr", env("ADDR", ":8080"), "listen address")
	dataDir := flag.String("data", env("DATA_DIR", "web-audio-tools/data"), "uploads and job output directory (shared with the AI service)")
	staticDir := flag.String("static", env("STATIC_DIR", "web-audio-tools/frontend/build"), "built frontend directory ('' to disable)")
	aiURL := flag.String("ai", env("AI_SERVICE_URL", "http://127.0.0.1:8001"), "Python AI service URL")
	workers := flag.Int("workers", envInt("WORKERS", 4), "concurrent jobs")
	maxUploadMB := flag.Int("max-upload-mb", envInt("MAX_UPLOAD_MB", 2048), "maximum upload size")
	cors := flag.String("cors", env("CORS_ORIGIN", "*"), "allowed CORS origin for /api")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	store, err := uploads.NewStore(*dataDir+"/uploads", int64(*maxUploadMB)<<20)
	if err != nil {
		slog.Error("uploads dir", "err", err)
		os.Exit(1)
	}
	manager, err := jobs.NewManager(*dataDir+"/jobs", *workers, 3*time.Hour)
	if err != nil {
		slog.Error("jobs dir", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()
	var ttsClient *texttospeech.Client
	if c, err := texttospeech.NewClient(ctx); err == nil {
		ttsClient = c
		defer c.Close()
		slog.Info("Google Cloud TTS enabled")
	} else {
		slog.Warn("Google Cloud TTS unavailable, using Translate TTS", "err", err)
	}
	if !media.Available() {
		slog.Warn("ffmpeg/ffprobe not found on PATH: TTS and server-side processing will fail")
	}
	if *staticDir != "" {
		if _, err := os.Stat(*staticDir); err != nil {
			slog.Warn("frontend build not found, serving API only", "dir", *staticDir)
			*staticDir = ""
		}
	}

	go func() {
		for range time.Tick(time.Hour) {
			store.Cleanup(24 * time.Hour)
			manager.Cleanup(24 * time.Hour)
		}
	}()

	srv := &http.Server{
		Addr: *addr,
		Handler: server.New(server.Config{
			Uploads:    store,
			Jobs:       manager,
			AI:         ai.NewClient(*aiURL),
			TTS:        ttsClient,
			StaticDir:  *staticDir,
			CORSOrigin: *cors,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("listening", "addr", *addr, "data", *dataDir, "static", *staticDir, "ai", *aiURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
