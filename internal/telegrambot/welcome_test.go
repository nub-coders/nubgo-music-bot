package telegrambot

import (
	"strings"
	"testing"

	"github.com/amarnathcjd/gogram/telegram"
)

func TestEntitiesToHTML(t *testing.T) {
	text := "hello world"
	got := entitiesToHTML(text, []telegram.MessageEntity{
		&telegram.MessageEntityBold{Offset: 0, Length: 5},
		&telegram.MessageEntityItalic{Offset: 6, Length: 5},
	})
	want := "<b>hello</b> <i>world</i>"
	if got != want {
		t.Fatalf("entitiesToHTML = %q, want %q", got, want)
	}
}

func TestEntitiesToHTMLEscapesText(t *testing.T) {
	got := entitiesToHTML("a < b & c", []telegram.MessageEntity{
		&telegram.MessageEntityCode{Offset: 0, Length: 9},
	})
	want := "<code>a &lt; b &amp; c</code>"
	if got != want {
		t.Fatalf("entitiesToHTML = %q, want %q", got, want)
	}
}

func TestEntitiesToHTMLUTF16Offsets(t *testing.T) {
	// The emoji is a surrogate pair (2 UTF-16 units), so "hi" offsets are shifted.
	got := entitiesToHTML("🎵hi", []telegram.MessageEntity{
		&telegram.MessageEntityBold{Offset: 2, Length: 2},
	})
	if !strings.Contains(got, "<b>hi</b>") {
		t.Fatalf("entitiesToHTML = %q, want bold hi", got)
	}
}

func TestInvalidPlaceholders(t *testing.T) {
	got := invalidPlaceholders("{name} {id} {botname} {oops} {name}")
	if len(got) != 1 || got[0] != "{oops}" {
		t.Fatalf("invalidPlaceholders = %v, want [{oops}]", got)
	}
}

func TestGroupWelcomeHasPlaceholders(t *testing.T) {
	body := groupWelcome("ʏᴏᴜ", "ᴛᴇsᴛ ɢʀᴏᴜᴘ", "ʙᴏᴛ")
	for _, want := range []string{"ʏᴏᴜ", "ᴛᴇsᴛ ɢʀᴏᴜᴘ", "ʙᴏᴛ", "/autoplay", "/play"} {
		if !strings.Contains(body, want) {
			t.Errorf("groupWelcome missing %q in %q", want, body)
		}
	}
}

func TestFormatWelcomeReplacesPlaceholders(t *testing.T) {
	got := formatWelcome("hey {name}, id {id}, bot {botname}", "ʏᴏᴜ", 42, "ʙᴏᴛ")
	if strings.Contains(got, "{") {
		t.Fatalf("formatWelcome left placeholders: %q", got)
	}
	for _, want := range []string{"ʏᴏᴜ", "42", "ʙᴏᴛ"} {
		if !strings.Contains(got, want) {
			t.Errorf("formatWelcome missing %q in %q", want, got)
		}
	}
}
