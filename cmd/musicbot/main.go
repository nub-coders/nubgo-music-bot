package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"net/http"

	"github.com/nub-coders/gogram/telegram"
	"github.com/nub-coders/nub-go-music-bot/internal/config"
	"github.com/nub-coders/nub-go-music-bot/internal/media"
	"github.com/nub-coders/nub-go-music-bot/internal/playback"
	"github.com/nub-coders/nub-go-music-bot/internal/session"
	"github.com/nub-coders/nub-go-music-bot/internal/storage"
	"github.com/nub-coders/nub-go-music-bot/internal/telegrambot"
	"github.com/nub-coders/nub-go-music-bot/internal/voice"
	"github.com/nub-coders/nub-go-music-bot/internal/voice/ntg"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fatal("load configuration", err)
	}
	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)
	if err := os.MkdirAll(cfg.CacheDirectory, 0o750); err != nil {
		fatal("create cache directory", err)
	}
	if _, err := os.Stat("/usr/bin/ffmpeg"); err != nil {
		if _, lookupErr := os.Stat("/usr/local/bin/ffmpeg"); lookupErr != nil {
			logger.Warn("ffmpeg was not found at a standard path; playback requires it on PATH")
		}
	}

	rootCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()

	bot, err := telegram.NewClient(telegram.ClientConfig{
		AppID: cfg.APIID, AppHash: cfg.APIHash,
		Session:   filepath.Join(cfg.CacheDirectory, "bot.session"),
		ParseMode: "html",
	})
	if err != nil {
		fatal("create Telegram bot client", err)
	}
	if _, err := bot.Conn(); err != nil {
		fatal("connect Telegram bot client", err)
	}
	authorized, err := bot.IsAuthorized()
	if err != nil && !telegram.MatchError(err, "AUTH_KEY_UNREGISTERED") {
		fatal("check bot authorization", err)
	}
	if !authorized {
		if err := bot.LoginBot(cfg.BotToken); err != nil {
			fatal("authorize Telegram bot", err)
		}
	}
	botUser, err := bot.GetMe()
	if err != nil {
		fatal("get Telegram bot profile", err)
	}
	logger.Info("bot authorized", "id", botUser.ID, "username", botUser.Username)

	var store storage.Access = storage.Noop{}
	if cfg.MongoDBURI != "" {
		ctx, cancel := context.WithTimeout(rootCtx, 15*time.Second)
		mongoStore, openErr := storage.OpenMongo(ctx, cfg.MongoDBURI, cfg.DatabaseName)
		cancel()
		if openErr != nil {
			logger.Error("failed to connect MongoDB; falling back to in-memory storage", "error", openErr)
		} else {
			store = mongoStore
			ctx, cancel = context.WithTimeout(rootCtx, 10*time.Second)
			if err := store.SeedAdmins(ctx, botUser.ID, cfg.InitialAdminIDs); err != nil {
				logger.Error("seed initial admins", "error", err)
			}
			cancel()
			logger.Info("MongoDB connected", "database", cfg.DatabaseName)
		}
	} else {
		logger.Warn("MONGODB_URI is unset; authorization changes will be disabled")
	}

	voiceManager := voice.NewManager(bot, logger)
	for index, rawSession := range cfg.AssistantSessions {
		normalized, normalizeErr := session.Normalize(rawSession, cfg.APIID)
		if normalizeErr != nil {
			logger.Error("skip invalid assistant session", "assistant", index+1, "error", normalizeErr)
			continue
		}
		assistantClient, clientErr := telegram.NewClient(telegram.ClientConfig{
			AppID: cfg.APIID, AppHash: cfg.APIHash, StringSession: normalized, MemorySession: true,
		})
		if clientErr != nil {
			logger.Error("create assistant", "assistant", index+1, "error", clientErr)
			continue
		}
		if _, clientErr = assistantClient.Conn(); clientErr != nil {
			logger.Error("connect assistant", "assistant", index+1, "error", clientErr)
			continue
		}
		assistantUser, profileErr := assistantClient.GetMe()
		if profileErr != nil {
			logger.Error("get assistant profile", "assistant", index+1, "error", profileErr)
			assistantClient.Stop()
			continue
		}
		voiceManager.RegisterAssistant(&voice.Assistant{Index: index + 1, Client: assistantClient, Calls: ntg.NTgCalls(), User: assistantUser})
		logger.Info("assistant authorized", "assistant", index+1, "id", assistantUser.ID, "username", assistantUser.Username)
	}
	if voiceManager.AssistantCount() == 0 {
		fatal("initialize assistants", errors.New("no valid assistant session was authorized"))
	}

	urlGuard := media.URLGuard{AllowPrivate: cfg.AllowPrivateStreamURLs}
	ytdlpResolver := &media.YTDLPResolver{CookiesFile: cfg.YTCookiesFile, Guard: urlGuard}
	resolver := &media.PriorityResolver{
		Inner:   &media.InnerTubeResolver{Guard: urlGuard},
		NUB:     &media.NUBAPIResolver{BaseURL: cfg.NUBAPIBaseURL, Token: cfg.NUBAPIToken, Guard: urlGuard},
		DataAPI: &media.YouTubeDataSearch{Keys: cfg.YouTubeAPIKeys},
		YTDLP:   ytdlpResolver,
		Logger:  logger,
	}
	sources := &media.Sources{
		YTDLP:               ytdlpResolver,
		SpotifyClientID:     cfg.SpotifyClientID,
		SpotifyClientSecret: cfg.SpotifyClientSecret,
		Client:              &http.Client{Timeout: 15 * time.Second},
		MaxPlaylistItems:    50,
	}
	player := playback.New(resolver, voiceManager, logger)
	voiceManager.SetHandlers(player.NotifyStreamEnd, player.NotifyFailure)
	authorizer := telegrambot.NewAuthorizer(bot, store, botUser.ID, cfg.OwnerID)
	handlers := telegrambot.NewHandlers(bot, player, authorizer, store, sources, botUser.ID, cfg.OwnerID, cfg.SupportGroup, cfg.MediaResolveTimeout, logger, voiceManager, filepath.Join(cfg.CacheDirectory, "welcome"), filepath.Join(cfg.AssetsDirectory, "music.jpg"), cfg.BotToken, cfg.CacheDirectory)
	player.SetObserver(handlers)
	handlers.Register()

	go voiceManager.AutoLeaveLoop(rootCtx, voice.AutoLeaveOptions{
		Enabled:     cfg.AutoLeaveEnabled,
		IdleTimeout: cfg.AutoLeaveTime,
		MaxPerSweep: cfg.AutoLeaveMaxPerSweep,
		DryRun:      cfg.AutoLeaveDryRun,
		LoggerID:    cfg.LoggerID,
		BotID:       botUser.ID,
		Store:       store,
	})

	logger.Info("NUB Go Music Bot started", "assistants", voiceManager.AssistantCount())
	<-rootCtx.Done()
	logger.Info("shutdown requested")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancelShutdown()
	if err := player.Close(shutdownCtx); err != nil {
		logger.Error("close playback", "error", err)
	}
	if err := voiceManager.Close(shutdownCtx); err != nil {
		logger.Error("close voice manager", "error", err)
	}
	if err := store.Close(shutdownCtx); err != nil {
		logger.Error("close storage", "error", err)
	}
	bot.Stop()
	logger.Info("shutdown complete")
}

func newLogger(levelName string) *slog.Logger {
	level := slog.LevelInfo
	switch levelName {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
func fatal(operation string, err error) {
	slog.Error(operation, "error", err)
	os.Exit(1)
}
