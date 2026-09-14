package telegrambot

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nub-coders/gogram/telegram"
	"github.com/nub-coders/nub-go-music-bot/internal/media"
)

func sampleSuggestions() []media.Suggestion {
	return []media.Suggestion{
		{VideoID: "aaaaaaaaaaa", Title: "First Song", Author: "Artist One", Duration: 3*time.Minute + 45*time.Second},
		{VideoID: "bbbbbbbbbbb", Title: "Second Song", Duration: time.Hour + 2*time.Minute + 3*time.Second},
	}
}

func TestSuggestionCardTextAutoplay(t *testing.T) {
	body := suggestionCardText(sampleSuggestions(), true)
	for _, want := range []string{"ǫᴜᴇᴜᴇ ᴇɴᴅᴇᴅ", "ᴀᴜᴛᴏᴘʟᴀʏɪɴɢ #1", "<b>10</b>s", "First Song", "Artist One", "3:45", "1:02:03", "<table"} {
		if !strings.Contains(body, want) {
			t.Errorf("autoplay card missing %q:\n%s", want, body)
		}
	}
	// Every custom emoji must survive as a legacy <emoji id=...> tag so both the
	// rich-text and the plain fallback paths can upgrade it.
	if !strings.Contains(body, `<emoji id="`) {
		t.Errorf("card should embed custom emoji tags:\n%s", body)
	}
}

func TestSuggestionCardTextNoAutoplay(t *testing.T) {
	body := suggestionCardText(sampleSuggestions(), false)
	if !strings.Contains(body, "ᴄʜᴏᴏsᴇ ᴀ sᴏɴɢ ᴛᴏ ᴘʟᴀʏ ɴᴇxᴛ") {
		t.Errorf("no-autoplay card missing prompt:\n%s", body)
	}
	if strings.Contains(body, "ᴀᴜᴛᴏᴘʟᴀʏɪɴɢ #1") {
		t.Errorf("no-autoplay card should not show a countdown:\n%s", body)
	}
}

func TestSuggestionButtonsPayloads(t *testing.T) {
	data := collectCallbackData(suggestionButtons(sampleSuggestions(), true, ""))
	for _, want := range []string{"sgstop", "sgtoggle", "sgclose"} {
		if !data[want] {
			t.Errorf("missing callback %q in %v", want, data)
		}
	}
	if data["sgplay_aaaaaaaaaaa"] {
		t.Error("song candidates should not be in normal inline buttons")
	}
	if !strings.Contains(renderButtonTexts(suggestionButtons(sampleSuggestions(), true, "")), "ᴀᴜᴛᴏᴘʟᴀʏ: ᴏɴ") {
		t.Error("toggle should read ON when enabled")
	}

	channel := collectCallbackData(suggestionButtons(sampleSuggestions(), false, "c"))
	for _, want := range []string{"csgstop", "csgtoggle", "csgclose"} {
		if !channel[want] {
			t.Errorf("missing channel callback %q in %v", want, channel)
		}
	}
	if channel["csgplay_aaaaaaaaaaa"] {
		t.Error("song candidates should not be in normal channel buttons")
	}
	if !strings.Contains(renderButtonTexts(suggestionButtons(sampleSuggestions(), false, "c")), "ᴀᴜᴛᴏᴘʟᴀʏ: ᴏꜰꜰ") {
		t.Error("toggle should read OFF when disabled")
	}
}

func TestSuggestionButtonsUseCustomEmojiIcons(t *testing.T) {
	markup := suggestionButtons(sampleSuggestions(), true, "")
	if markup == nil || len(markup.Rows) == 0 {
		t.Fatal("expected a non-empty keyboard")
	}
	icons := 0
	for _, row := range markup.Rows {
		for _, button := range row.Buttons {
			if button.Style != nil && button.Style.Icon != 0 {
				icons++
			}
		}
	}
	if icons == 0 {
		t.Fatal("expected custom-emoji button icons to be preserved")
	}
}

func TestKeycap(t *testing.T) {
	if got := keycap(1); got != "1\uFE0F\u20E3" {
		t.Errorf("keycap(1) = %q", got)
	}
	if got := keycap(12); got != "1\uFE0F\u20E32\uFE0F\u20E3" {
		t.Errorf("keycap(12) = %q", got)
	}
}

func TestTrimButtonText(t *testing.T) {
	long := trimButtonText(strings.Repeat("x", 80))
	if got := len([]rune(long)); got != 48 {
		t.Fatalf("expected 48 runes, got %d", got)
	}
	if !strings.HasSuffix(long, "…") {
		t.Errorf("expected ellipsis suffix, got %q", long)
	}
	if trimButtonText("  hi  ") != "hi" {
		t.Error("expected trimmed label")
	}
}

func TestAutoplayPanelAndStatus(t *testing.T) {
	panel := autoplayPanel(autoplayStatusText(true))
	for _, want := range []string{"ᴀᴜᴛᴏᴘʟᴀʏ", "ᴇɴᴀʙʟᴇᴅ", "ᴄᴀɴ sᴡɪᴛᴄʜ", "sʜᴏᴡ ᴛʜɪs ᴄᴀʀᴅ ᴏɴʟʏ"} {
		if !strings.Contains(panel, want) {
			t.Errorf("panel missing %q:\n%s", want, panel)
		}
	}
	if !strings.Contains(autoplayStatusText(false), "ᴅɪsᴀʙʟᴇᴅ") {
		t.Error("disabled status text missing")
	}
	if !strings.Contains(autoplayAdminOnly("<b>ᴇɴᴀʙʟᴇᴅ</b>"), "Only admins") {
		t.Error("admin-only note missing")
	}
}

func TestSuggestionDisplayTitleFallback(t *testing.T) {
	if got := suggestionDisplayTitle(media.Suggestion{VideoID: "xyz"}); got != "xyz" {
		t.Errorf("expected video ID fallback, got %q", got)
	}
	if got := suggestionDisplayTitle(media.Suggestion{VideoID: "xyz", Title: " Real "}); got != "Real" {
		t.Errorf("expected trimmed title, got %q", got)
	}
}

func TestSuggestionCallbackPatterns(t *testing.T) {
	// These mirror the anchored patterns registered in Register().
	play := regexp.MustCompile("^c?sgplay_")
	stop := regexp.MustCompile("^c?sgstop$")
	toggle := regexp.MustCompile("^c?sgtoggle$")
	closePattern := regexp.MustCompile("^c?sgclose$")

	playCases := map[string]bool{
		"sgplay_aaaaaaaaaaa":   true,
		"csgplay_aaaaaaaaaaa":  true,
		"sgplay_":              true,
		"sgstop":               false,
		"xnsgplay_aaaaaaaaaaa": false,
	}
	for data, want := range playCases {
		if got := play.MatchString(data); got != want {
			t.Errorf("play pattern on %q = %v, want %v", data, got, want)
		}
	}
	for data, pattern := range map[string]*regexp.Regexp{
		"sgstop": stop, "csgstop": stop,
		"sgtoggle": toggle, "csgtoggle": toggle,
		"sgclose": closePattern, "csgclose": closePattern,
	} {
		if !pattern.MatchString(data) {
			t.Errorf("pattern for %q should match", data)
		}
	}
	if stop.MatchString("sgstop_extra") {
		t.Error("anchored stop pattern must not match a suffixed payload")
	}
}

// collectCallbackData flattens a keyboard into a set of callback payloads.
func collectCallbackData(markup *telegram.ReplyInlineMarkup) map[string]bool {
	data := make(map[string]bool)
	for _, row := range markup.Rows {
		for _, button := range row.Buttons {
			if callback, ok := button.Type.(*telegram.InlineButtonTypeCallback); ok {
				data[string(callback.Data)] = true
			}
		}
	}
	return data
}

// renderButtonTexts joins every button label for substring assertions.
func renderButtonTexts(markup *telegram.ReplyInlineMarkup) string {
	var builder strings.Builder
	for _, row := range markup.Rows {
		for _, button := range row.Buttons {
			builder.WriteString(button.Text)
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}
