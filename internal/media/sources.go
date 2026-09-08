package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// SourceEntry is a single playable thing produced by expanding a container
// link (a YouTube playlist or a Spotify album/playlist). Query is the raw input
// that the normal resolver chain turns into a Track.
type SourceEntry struct {
	Query    string
	Title    string
	Duration time.Duration
}

// Sources expands container links into a list of playable entries, mirroring
// the Python bot's sources.py: Spotify first, then YouTube playlists, then a
// plain passthrough of the argument.
type Sources struct {
	// YTDLP is used for flat playlist extraction so cookies apply.
	YTDLP *YTDLPResolver
	// SpotifyClientID / Secret enable Spotify track/album/playlist expansion.
	SpotifyClientID     string
	SpotifyClientSecret string
	Client              *http.Client
	// MaxPlaylistItems caps how many tracks one container link may enqueue.
	MaxPlaylistItems int
}

const (
	youtubePlaylistMaxDefault = 50
)

var (
	spotifyLinkPattern = regexp.MustCompile(`open\.spotify\.com/(?:intl-[a-z]+/)?(track|album|playlist)/([A-Za-z0-9]+)`)
	ytDomainPattern    = regexp.MustCompile(`^(https?://)?(www\.|music\.|m\.)?(youtube\.com|youtu\.be)/`)
)

// Expand turns a raw /play argument into a non-empty list of entries to enqueue.
// Container links are expanded; anything else is passed through unchanged. On a
// recognized-but-failed expansion it falls back to the raw argument (never fails).
func (s *Sources) Expand(ctx context.Context, input string) []SourceEntry {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	if s.MaxPlaylistItems <= 0 {
		s.MaxPlaylistItems = youtubePlaylistMaxDefault
	}
	if s.isSpotify(input) {
		if items, err := s.spotify(ctx, input); err == nil && len(items) > 0 {
			return items
		}
	}
	if isYouTubePlaylist(input) {
		if items, err := s.ytPlaylist(ctx, input); err == nil && len(items) > 0 {
			return items
		}
	}
	return []SourceEntry{{Query: input}}
}

func (s *Sources) isSpotify(input string) bool {
	return s.SpotifyClientID != "" && s.SpotifyClientSecret != "" && spotifyLinkPattern.MatchString(input)
}

func isYouTubePlaylist(input string) bool {
	if !ytDomainPattern.MatchString(input) {
		return false
	}
	if !strings.Contains(input, "list=") {
		return false
	}
	// A watch?v=VIDEO&list=... link is a single video chosen from a playlist.
	return !strings.Contains(input, "v=") || strings.Contains(input, "/playlist")
}

func (s *Sources) ytPlaylist(ctx context.Context, input string) ([]SourceEntry, error) {
	binary := "yt-dlp"
	if s.YTDLP != nil && s.YTDLP.Binary != "" {
		binary = s.YTDLP.Binary
	}
	cookies := ""
	if s.YTDLP != nil {
		cookies = s.YTDLP.CookiesFile
	}
	args := []string{"--flat-playlist", "--dump-single-json", "--no-warnings", "--playlist-end", fmt.Sprintf("%d", s.MaxPlaylistItems)}
	if cookies != "" {
		args = append(args, "--cookies", cookies)
	}
	args = append(args, input)
	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if len(message) > 300 {
			message = message[:300]
		}
		return nil, fmt.Errorf("expand playlist: %w: %s", err, message)
	}
	var result struct {
		Entries []struct {
			ID       string  `json:"id"`
			URL      string  `json:"url"`
			Title    string  `json:"title"`
			Duration float64 `json:"duration"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("decode playlist response: %w", err)
	}
	items := make([]SourceEntry, 0, len(result.Entries))
	for _, entry := range result.Entries {
		videoID := entry.ID
		if videoID == "" {
			videoID = extractYouTubeVideoID(entry.URL)
		}
		if videoID == "" {
			continue
		}
		raw := entry.URL
		if !IsHTTPURL(raw) {
			raw = "https://www.youtube.com/watch?v=" + videoID
		}
		items = append(items, SourceEntry{
			Query:    raw,
			Title:    entry.Title,
			Duration: time.Duration(entry.Duration * float64(time.Second)),
		})
		if len(items) >= s.MaxPlaylistItems {
			break
		}
	}
	if len(items) == 0 {
		return nil, errors.New("playlist contained no entries")
	}
	return items, nil
}

// -- Spotify client-credentials expansion ------------------------------------

func (s *Sources) spotifyToken(ctx context.Context) (string, error) {
	auth := base64.StdEncoding.EncodeToString([]byte(s.SpotifyClientID + ":" + s.SpotifyClientSecret))
	body := strings.NewReader("grant_type=client_credentials")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://accounts.spotify.com/api/token", body)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Basic "+auth)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.http().Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Spotify token endpoint returned HTTP %d", response.StatusCode)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if parsed.AccessToken == "" {
		return "", errors.New("Spotify returned an empty access token")
	}
	return parsed.AccessToken, nil
}

func (s *Sources) spotify(ctx context.Context, input string) ([]SourceEntry, error) {
	matches := spotifyLinkPattern.FindStringSubmatch(input)
	if len(matches) != 3 {
		return nil, errors.New("unsupported Spotify link")
	}
	kind, spotifyID := strings.ToLower(matches[1]), matches[2]
	httpClient := s.http()
	token, err := s.spotifyToken(ctx)
	if err != nil {
		return nil, err
	}
	switch kind {
	case "track":
		info, err := s.spotifyGet(ctx, httpClient, token, "https://api.spotify.com/v1/tracks/"+spotifyID)
		if err != nil {
			return nil, err
		}
		if entry := spotifyTrackToEntry(info); entry != nil {
			return []SourceEntry{*entry}, nil
		}
		return nil, errors.New("Spotify track had no name")
	case "album", "playlist":
		base := "https://api.spotify.com/v1/albums/" + spotifyID + "/tracks"
		if kind == "playlist" {
			base = "https://api.spotify.com/v1/playlists/" + spotifyID + "/tracks"
		}
		return s.spotifyPage(ctx, httpClient, token, base)
	default:
		return nil, errors.New("unsupported Spotify resource")
	}
}

func (s *Sources) spotifyPage(ctx context.Context, httpClient *http.Client, token, base string) ([]SourceEntry, error) {
	var items []SourceEntry
	for next := base; next != "" && len(items) < s.MaxPlaylistItems; {
		payload, err := s.spotifyGet(ctx, httpClient, token, next)
		if err != nil {
			return nil, err
		}
		next = ""
		if entries, ok := payload["items"].([]any); ok {
			for _, raw := range entries {
				var track map[string]any
				switch value := raw.(type) {
				case map[string]any:
					// Playlist items wrap the track under "track".
					if wrapped, ok := value["track"].(map[string]any); ok {
						track = wrapped
					} else {
						track = value
					}
				}
				if entry := spotifyTrackToEntry(track); entry != nil {
					items = append(items, *entry)
				}
				if len(items) >= s.MaxPlaylistItems {
					break
				}
			}
		}
		if pagination, ok := payload["next"].(string); ok {
			next = pagination
		}
	}
	if len(items) == 0 {
		return nil, errors.New("Spotify container contained no tracks")
	}
	return items, nil
}

func (s *Sources) spotifyGet(ctx context.Context, httpClient *http.Client, token, url string) (map[string]any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Spotify API returned HTTP %d", response.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *Sources) http() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func spotifyTrackToEntry(track map[string]any) *SourceEntry {
	name, ok := track["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return nil
	}
	var artists []string
	if raw, ok := track["artists"].([]any); ok {
		for _, value := range raw {
			if artist, ok := value.(map[string]any)["name"].(string); ok && artist != "" {
				artists = append(artists, artist)
			}
		}
	}
	query := strings.TrimSpace(strings.Join(artists, ", ") + " - " + name)
	if query == "" {
		query = name
	}
	var duration time.Duration
	if milliseconds, ok := track["duration_ms"].(float64); ok {
		duration = time.Duration(milliseconds) * time.Millisecond
	}
	return &SourceEntry{Query: query, Title: name, Duration: duration}
}
