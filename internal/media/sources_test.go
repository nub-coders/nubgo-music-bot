package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// rewriter forwards requests headed for the Spotify API to a local test server,
// keeping expansion tests offline.
type rewriter struct {
	target *httptest.Server
	inner  http.RoundTripper
}

func (r *rewriter) RoundTrip(request *http.Request) (*http.Response, error) {
	copied := request.Clone(request.Context())
	copied.URL.Scheme = "http"
	copied.URL.Host = strings.TrimPrefix(r.target.URL, "http://")
	copied.Host = ""
	return r.inner.RoundTrip(copied)
}

// spotifyTestServer serves a fixed client-credentials token and a tiny track
// list so expansion can be exercised without network access.
func spotifyTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"expires_in":   3600,
			})
		case "/v1/albums/abc/tracks":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []any{
					map[string]any{
						"name":        "First Song",
						"artists":     []any{map[string]any{"name": "Artist One"}},
						"duration_ms": float64(180_000),
					},
					map[string]any{
						"name":        "Second Song",
						"artists":     []any{map[string]any{"name": "Artist Two"}},
						"duration_ms": float64(210_000),
					},
				},
				"next": nil,
			})
		default:
			t.Errorf("unexpected Spotify request to %s", request.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestSourcesExpandSpotifyAlbum(t *testing.T) {
	server := spotifyTestServer(t)
	sources := &Sources{
		SpotifyClientID:     "client",
		SpotifyClientSecret: "secret",
		Client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &rewriter{
				target: server,
				inner:  http.DefaultTransport,
			},
		},
		MaxPlaylistItems: 50,
	}

	entries := sources.Expand(context.Background(), "https://open.spotify.com/album/abc?si=xyz")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(entries), entries)
	}
	if !strings.Contains(entries[0].Query, "First Song") {
		t.Errorf("first entry query missing song title: %q", entries[0].Query)
	}
	if entries[0].Duration != 180*time.Second {
		t.Errorf("first entry duration = %v, want 3m0s", entries[0].Duration)
	}
	if entries[1].Duration != 210*time.Second {
		t.Errorf("second entry duration = %v, want 3m30s", entries[1].Duration)
	}
}

func TestSourcesExpandPassthrough(t *testing.T) {
	sources := &Sources{MaxPlaylistItems: 50}
	entries := sources.Expand(context.Background(), "never gonna give you up")
	if len(entries) != 1 || entries[0].Query != "never gonna give you up" {
		t.Fatalf("passthrough failed: %+v", entries)
	}
}

func TestSourcesExpandEmpty(t *testing.T) {
	sources := &Sources{}
	if entries := sources.Expand(context.Background(), "   "); entries != nil {
		t.Fatalf("expected nil entries for blank input, got %+v", entries)
	}
}

func TestIsYouTubePlaylist(t *testing.T) {
	cases := map[string]bool{
		"https://www.youtube.com/playlist?list=ABC123":        true,
		"https://music.youtube.com/playlist?list=ABC123":      true,
		"https://www.youtube.com/watch?v=VIDEOID&list=ABC123": false,
		"https://www.youtube.com/watch?v=VIDEOID":             false,
		"some random text": false,
		"https://www.youtube.com/playlist?list=ABC123&v=VIDEOID": true,
	}
	for input, want := range cases {
		if got := isYouTubePlaylist(input); got != want {
			t.Errorf("isYouTubePlaylist(%q) = %v, want %v", input, got, want)
		}
	}
}
