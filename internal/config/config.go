package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const defaultSupportGroup = "nub_coder_s"

type Config struct {
	APIID                  int32
	APIHash                string
	BotToken               string
	OwnerID                int64
	SupportGroup           string
	AssistantSessions      []string
	MongoDBURI             string
	DatabaseName           string
	LoggerID               int64
	InitialAdminIDs        []int64
	YTCookiesFile          string
	YouTubeAPIKeys         []string
	NUBAPIToken            string
	NUBAPIBaseURL          string
	SpotifyClientID        string
	SpotifyClientSecret    string
	AllowPrivateStreamURLs bool
	MediaResolveTimeout    time.Duration
	ShutdownTimeout        time.Duration
	WorkingDirectory       string
	CacheDirectory         string
	LogLevel               string

	// Auto-leave idle chats for assistant accounts so they stay under Telegram's 500-group limit.
	AutoLeaveEnabled     bool
	AutoLeaveTime        time.Duration
	AutoLeaveMaxPerSweep int
	AutoLeaveDryRun      bool
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cwd, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("get working directory: %w", err)
	}

	apiID64, err := parseRequiredInt("API_ID", 32)
	if err != nil {
		return Config{}, err
	}
	ownerID, err := parseOptionalInt64("OWNER_ID")
	if err != nil {
		return Config{}, err
	}
	if ownerID < 0 {
		return Config{}, errors.New("OWNER_ID must be a positive Telegram user ID")
	}
	loggerID, err := parseOptionalInt64("LOGGER_ID")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		APIID:                  int32(apiID64),
		APIHash:                strings.TrimSpace(os.Getenv("API_HASH")),
		BotToken:               strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		OwnerID:                ownerID,
		SupportGroup:           envOr("GROUP", defaultSupportGroup),
		AssistantSessions:      assistantSessions(),
		MongoDBURI:             firstNonEmpty("MONGODB_URI", "MONGO_DB_URI"),
		DatabaseName:           envOr("DB_NAME", "musicbot"),
		LoggerID:               loggerID,
		InitialAdminIDs:        parseIDList(os.Getenv("INITIAL_ADMIN_IDS")),
		YTCookiesFile:          strings.TrimSpace(os.Getenv("YT_COOKIES_FILE")),
		YouTubeAPIKeys:         parseStringList(os.Getenv("YOUTUBE_API_KEYS")),
		NUBAPIToken:            firstNonEmpty("YTUBE_API_TOKEN", "YT_API_TOKEN"),
		NUBAPIBaseURL:          firstNonEmptyOr("https://api.nubcoders.com", "YTUBE_API_BASE_URL", "NUB_YT_API_BASE_URL"),
		SpotifyClientID:        strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_ID")),
		SpotifyClientSecret:    strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_SECRET")),
		AllowPrivateStreamURLs: parseBool("ALLOW_PRIVATE_STREAM_URLS", false),
		MediaResolveTimeout:    parseDurationSeconds("MEDIA_RESOLVE_TIMEOUT", 45*time.Second),
		ShutdownTimeout:        parseDurationSeconds("SHUTDOWN_TIMEOUT", 15*time.Second),
		AutoLeaveEnabled:       parseBool("AUTO_LEAVING_ASSISTANT", true),
		AutoLeaveTime:          parseDurationSeconds("ASSISTANT_LEAVE_TIME", 90*time.Minute),
		AutoLeaveMaxPerSweep:   parseInt("ASSISTANT_MAX_LEAVES_PER_SWEEP", 10),
		AutoLeaveDryRun:        parseBool("ASSISTANT_LEAVE_DRY_RUN", false),
		WorkingDirectory:       cwd,
		CacheDirectory:         filepath.Join(cwd, "cache"),
		LogLevel:               strings.ToLower(envOr("LOG_LEVEL", "info")),
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var missing []string
	if c.APIID <= 0 {
		missing = append(missing, "API_ID")
	}
	if c.APIHash == "" {
		missing = append(missing, "API_HASH")
	}
	if c.BotToken == "" {
		missing = append(missing, "BOT_TOKEN")
	}
	if len(c.AssistantSessions) == 0 {
		missing = append(missing, "STRING_SESSION")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}
	if c.YTCookiesFile != "" {
		if info, err := os.Stat(c.YTCookiesFile); err != nil || info.IsDir() {
			return fmt.Errorf("YT_COOKIES_FILE is not a readable file: %s", c.YTCookiesFile)
		}
	}
	return nil
}

func assistantSessions() []string {
	first := strings.TrimSpace(os.Getenv("STRING_SESSION1"))
	if first == "" {
		first = strings.TrimSpace(os.Getenv("STRING_SESSION"))
	}
	result := make([]string, 0, 5)
	if first != "" {
		result = append(result, first)
	}
	for i := 2; i <= 5; i++ {
		if value := strings.TrimSpace(os.Getenv(fmt.Sprintf("STRING_SESSION%d", i))); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func parseRequiredInt(key string, bits int) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, bits)
	if err != nil {
		return 0, fmt.Errorf("%s must be numeric: %w", key, err)
	}
	return parsed, nil
}

func parseOptionalInt64(key string) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be numeric: %w", key, err)
	}
	return parsed, nil
}

func parseStringList(value string) []string {
	var values []string
	for _, field := range strings.Fields(strings.ReplaceAll(value, ",", " ")) {
		if field != "" {
			values = append(values, field)
		}
	}
	return values
}

func parseIDList(value string) []int64 {
	var ids []int64
	for _, field := range strings.Fields(strings.ReplaceAll(value, ",", " ")) {
		if id, err := strconv.ParseInt(field, 10, 64); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func parseBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func parseDurationSeconds(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func firstNonEmptyOr(fallback string, keys ...string) string {
	if value := firstNonEmpty(keys...); value != "" {
		return value
	}
	return fallback
}

func firstNonEmpty(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
