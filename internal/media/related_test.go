package media

import (
	"testing"
	"time"
)

func TestExtractYTMusicTracks(t *testing.T) {
	response := map[string]any{
		"contents": map[string]any{
			"playlist": map[string]any{
				"contents": []any{
					map[string]any{
						"playlistPanelVideoRenderer": map[string]any{
							"videoId":    "aaaaaaaaaaa",
							"title":      map[string]any{"runs": []any{map[string]any{"text": "First Song"}}},
							"lengthText": map[string]any{"simpleText": "3:45"},
							"shortBylineText": map[string]any{
								"runs": []any{map[string]any{"text": "Artist One"}},
							},
						},
					},
					map[string]any{
						"playlistPanelVideoRenderer": map[string]any{
							"videoId":    "bbbbbbbbbbb",
							"title":      map[string]any{"simpleText": "Second Song"},
							"lengthText": map[string]any{"simpleText": "1:02:03"},
						},
					},
					// Duplicate video id should be ignored.
					map[string]any{
						"playlistPanelVideoRenderer": map[string]any{
							"videoId": "aaaaaaaaaaa",
							"title":   map[string]any{"simpleText": "First Song Again"},
						},
					},
					// Missing title should be skipped.
					map[string]any{
						"playlistPanelVideoRenderer": map[string]any{
							"videoId": "ccccccccccc",
						},
					},
				},
			},
		},
	}

	tracks := extractYTMusicTracks(response)
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d: %+v", len(tracks), tracks)
	}
	if tracks[0].VideoID != "aaaaaaaaaaa" || tracks[0].Title != "First Song" {
		t.Errorf("unexpected first track: %+v", tracks[0])
	}
	if tracks[0].Author != "Artist One" {
		t.Errorf("expected artist from shortBylineText, got %q", tracks[0].Author)
	}
	if tracks[0].Duration != 3*time.Minute+45*time.Second {
		t.Errorf("expected 3:45 duration, got %s", tracks[0].Duration)
	}
	if tracks[0].URL != "https://www.youtube.com/watch?v=aaaaaaaaaaa" {
		t.Errorf("unexpected url: %q", tracks[0].URL)
	}
	if tracks[1].Duration != time.Hour+2*time.Minute+3*time.Second {
		t.Errorf("expected 1:02:03 duration, got %s", tracks[1].Duration)
	}
}

func TestParseClockDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"":        0,
		"0:30":    30 * time.Second,
		"3:45":    3*time.Minute + 45*time.Second,
		"1:02:03": time.Hour + 2*time.Minute + 3*time.Second,
		"bad":     0,
	}
	for input, want := range cases {
		if got := parseClockDuration(input); got != want {
			t.Errorf("parseClockDuration(%q) = %s, want %s", input, got, want)
		}
	}
}
