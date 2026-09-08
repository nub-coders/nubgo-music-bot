package media

import "testing"

func TestExtractYouTubeVideoID(t *testing.T) {
	cases := map[string]string{
		"dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ":                "dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ":  "dQw4w9WgXcQ",
	}
	for input, expected := range cases {
		if got := extractYouTubeVideoID(input); got != expected {
			t.Errorf("extractYouTubeVideoID(%q) = %q, want %q", input, got, expected)
		}
	}
}
