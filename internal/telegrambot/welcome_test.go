package telegrambot

import (
	"strings"
	"testing"

	"github.com/nub-coders/gogram/telegram"
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

func TestUpgradeUnicodeEmoji(t *testing.T) {
	input := "🎵 Now Playing • 👑 Owner • <code>⚡ 12ms</code> • <tg-emoji emoji-id=\"999\">🎧</tg-emoji>"
	got := richHTML(input)
	if !strings.Contains(got, `<tg-emoji emoji-id="5891249688933305846">🎵</tg-emoji>`) {
		t.Errorf("plain 🎵 was not upgraded: %q", got)
	}
	if !strings.Contains(got, `<tg-emoji emoji-id="5807868868886009920">👑</tg-emoji>`) {
		t.Errorf("plain 👑 was not upgraded: %q", got)
	}
	if !strings.Contains(got, `<code>⚡ 12ms</code>`) {
		t.Errorf("emoji inside <code> was improperly modified: %q", got)
	}
	if !strings.Contains(got, `<tg-emoji emoji-id="999">🎧</tg-emoji>`) {
		t.Errorf("already-tagged emoji was corrupted: %q", got)
	}
}

func TestNormalizeRichHTML(t *testing.T) {
	input := "Line 1\nLine 2\n\nLine 3\n<blockquote>A quote</blockquote>\n<h1>Header</h1>\n<pre>\ncode line 1\ncode line 2\n</pre>\nAfter pre"
	got := normalizeRichHTML(input)

	if !strings.Contains(got, "Line 1<br/>\nLine 2<br/>\n<br/>\nLine 3") {
		t.Errorf("expected line breaks with <br/>, got: %q", got)
	}
	if strings.Contains(got, "</blockquote><br/>\n") {
		t.Errorf("expected blockquote to not have <br/> immediately after it: %q", got)
	}
	if strings.Contains(got, "code line 1<br/>") {
		t.Errorf("expected pre block to not have <br/> inserted: %q", got)
	}
	if !strings.Contains(got, "After pre") {
		t.Errorf("expected content after pre block: %q", got)
	}

	// Test unquoted href
	hrefInput := `<a href=tg://user?id=123>User</a>`
	hrefGot := normalizeRichHTML(hrefInput)
	if !strings.Contains(hrefGot, `href="tg://user?id=123"`) {
		t.Errorf("expected unquoted href to be quoted, got: %q", hrefGot)
	}
}

