package voice

import (
	"strings"
	"testing"
	"time"
)

func TestMediaDescriptionNoSeekByDefault(t *testing.T) {
	description := mediaDescription("https://example.com/stream.mp3", false, false, 0)
	if description.Microphone == nil {
		t.Fatal("microphone description is nil")
	}
	input := description.Microphone.Input
	if strings.Contains(input, "-ss") {
		t.Errorf("unexpected -ss flag without seek offset: %q", input)
	}
	if !strings.Contains(input, "-i 'https://example.com/stream.mp3'") {
		t.Errorf("audio command missing input URL: %q", input)
	}
	if !strings.Contains(input, "-f s16le -ac 2 -ar 48000") {
		t.Errorf("audio command missing output format: %q", input)
	}
}

func TestMediaDescriptionSeekPrefix(t *testing.T) {
	description := mediaDescription("https://example.com/stream.mp3", false, false, 90*time.Second)
	if description.Microphone == nil {
		t.Fatal("microphone description is nil")
	}
	input := description.Microphone.Input
	if !strings.Contains(input, "-ss '90.000'") {
		t.Errorf("audio command missing seek prefix: %q", input)
	}
	// The seek prefix must appear before the -i input flag.
	if strings.Index(input, "-ss") > strings.Index(input, "-i ") {
		t.Errorf("-ss must precede -i: %q", input)
	}
}

func TestMediaDescriptionVideoAddsCamera(t *testing.T) {
	description := mediaDescription("https://example.com/stream.mp4", true, false, 0)
	if description.Camera == nil {
		t.Fatal("camera description is nil for video track")
	}
	if !strings.Contains(description.Camera.Input, "-pix_fmt yuv420p") {
		t.Errorf("camera command missing rawvideo flags: %q", description.Camera.Input)
	}
}

func TestMediaDescriptionLiveAddsReconnect(t *testing.T) {
	description := mediaDescription("https://example.com/live", true, true, 0)
	if description.Microphone == nil {
		t.Fatal("microphone description is nil")
	}
	input := description.Microphone.Input
	if !strings.Contains(input, "-reconnect 1") {
		t.Errorf("live stream missing reconnect flags: %q", input)
	}
}
