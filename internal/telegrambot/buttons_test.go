package telegrambot

import (
	"testing"

	"github.com/nub-coders/gogram/telegram"
)

// forbiddenEmojiRune reports whether r is a plain (non-custom) emoji that must
// not appear in a button label. Geometric control glyphs the playback row is
// built from (▷ U+25B7, ▢ U+25A2) are deliberately allowed.
func forbiddenEmojiRune(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF: // pictographs, flags, symbols
		return true
	case r >= 0x2600 && r <= 0x27BF: // misc symbols + dingbats (✖, ➕, ℹ, ⚙…)
		return true
	case r >= 0x2B00 && r <= 0x2BFF:
		return true
	case r == 0xFE0F || r == 0x20E3: // variation selector-16, keycap
		return true
	case r == 0x2139 || r == 0x23F3: // ℹ, ⏳
		return true
	case r == 0x25B6 || r == 0x25C0: // ▶, ◀ (the plain play/back triangles)
		return true
	}
	return false
}

// allButtonTexts gathers every button label the bot can render.
func allButtonTexts() []string {
	var texts []string
	collect := func(markup *telegram.ReplyInlineMarkup) {
		for _, row := range markup.Rows {
			for _, button := range row.Buttons {
				texts = append(texts, button.Text)
			}
		}
	}
	collect(startButtons("nubmusicbot", 12345, "nub_coder_s"))
	collect(startButtons("nubmusicbot", 0, "nub_coder_s"))
	collect(groupWelcomeButtons("nubmusicbot", "nub_coder_s"))
	collect(helpButtons(true))
	collect(helpButtons(false))
	collect(helpBackButtons())
	collect(playbackButtons(false, "01:23 ─ ─ ▷ ─ ─ ─ ─ 3:35"))
	collect(playbackButtons(true, ""))
	collect(suggestionButtons(sampleSuggestions(), true, ""))
	collect(suggestionButtons(sampleSuggestions(), false, "c"))
	return texts
}

func buttonData(btn *telegram.KeyboardInlineButton) string {
	if btn != nil && btn.Type != nil {
		if cb, ok := btn.Type.(*telegram.InlineButtonTypeCallback); ok {
			return string(cb.Data)
		}
	}
	return ""
}

// TestPlaybackButtonsLayout verifies that playback markup renders all 4 transport
// controls (resume, pause, skip, end), progress bar, and close button matching nub-music-bot.
func TestPlaybackButtonsLayout(t *testing.T) {
	markup := playbackButtonsPrefixed("01:23 ─ ─ ▷ ─ ─ ─ ─ 3:35", "c")
	if len(markup.Rows) != 3 {
		t.Fatalf("expected 3 rows in playbackButtonsPrefixed, got %d", len(markup.Rows))
	}
	// Row 0: ▷, II, ‣‣I, ▢ (rendered as \u200b with custom icon)
	r0 := markup.Rows[0].Buttons
	if len(r0) != 4 {
		t.Fatalf("expected 4 transport buttons in row 0, got %d", len(r0))
	}
	expectedTransport := []struct {
		text string
		data string
		icon int64
	}{
		{"\u200b", "cresume", emojiPlay},
		{"\u200b", "cpause", emojiPause},
		{"\u200b", "cskip", emojiSkip},
		{"\u200b", "cend", emojiStop},
	}
	for i, exp := range expectedTransport {
		if r0[i].Text != exp.text {
			t.Errorf("row 0 button %d text = %q, want %q", i, r0[i].Text, exp.text)
		}
		if buttonData(r0[i]) != exp.data {
			t.Errorf("row 0 button %d data = %q, want %q", i, buttonData(r0[i]), exp.data)
		}
		if r0[i].Style == nil || r0[i].Style.Icon != exp.icon {
			t.Errorf("row 0 button %d style icon = %v, want %d", i, r0[i].Style, exp.icon)
		}
	}
	// Row 1: progress
	r1 := markup.Rows[1].Buttons
	if len(r1) != 1 || r1[0].Text != "01:23 ─ ─ ▷ ─ ─ ─ ─ 3:35" {
		t.Errorf("row 1 progress button mismatch")
	}
	// Row 2: close
	r2 := markup.Rows[2].Buttons
	if len(r2) != 1 || r2[0].Text != "ᴄʟᴏsᴇ" || buttonData(r2[0]) != "close" || r2[0].Style == nil || r2[0].Style.Icon != emojiClose {
		t.Errorf("row 2 close button mismatch: %v", r2[0])
	}
}

// TestQueueButtonsLayout verifies the queued song card markup.
func TestQueueButtonsLayout(t *testing.T) {
	markup := queueButtons("track_123", false)
	if len(markup.Rows) != 3 {
		t.Fatalf("expected 3 rows in queueButtons, got %d", len(markup.Rows))
	}
	playNow := markup.Rows[1].Buttons[0]
	closeBtn := markup.Rows[2].Buttons[0]
	if playNow.Text != "ᴘʟᴀʏ ɴᴏᴡ" || buttonData(playNow) != "playnow_track_123" || playNow.Style == nil || playNow.Style.Icon != emojiPlay {
		t.Errorf("play now button mismatch: %v", playNow)
	}
	if closeBtn.Text != "ᴄʟᴏsᴇ" || buttonData(closeBtn) != "close" || closeBtn.Style == nil || closeBtn.Style.Icon != emojiClose {
		t.Errorf("close button mismatch: %v", closeBtn)
	}
}

// TestNoPlainEmojiInButtons ensures no button carries duplicate plain Unicode emojis
// when custom emoji icons are used.
func TestNoPlainEmojiInButtons(t *testing.T) {
	texts := allButtonTexts()
	for _, text := range texts {
		if text == "01:23 ─ ─ ▷ ─ ─ ─ ─ 3:35" {
			continue // Progress bar exception
		}
		for _, r := range text {
			if forbiddenEmojiRune(r) {
				t.Errorf("button text %q contains plain unicode emoji %c (U+%04X)", text, r, r)
			}
		}
	}
}

// TestButtonsKeepCustomEmojiIcons guards against the opposite mistake: stripping
// the plain emoji must not also drop the styled custom-emoji icon.
func TestButtonsKeepCustomEmojiIcons(t *testing.T) {
	for _, markup := range []*telegram.ReplyInlineMarkup{
		startButtons("nubmusicbot", 12345, "nub_coder_s"),
		startButtons("nubmusicbot", 0, "nub_coder_s"),
		groupWelcomeButtons("nubmusicbot", "nub_coder_s"),
		helpButtons(true),
		helpButtons(false),
		helpBackButtons(),
		playbackButtons(false, ""),
		suggestionButtons(sampleSuggestions(), true, ""),
	} {
		for _, row := range markup.Rows {
			for _, button := range row.Buttons {
				if button.Style == nil || button.Style.Icon == 0 {
					t.Errorf("button %q is missing its custom-emoji icon", button.Text)
				}
			}
		}
	}
}

// TestProgressBarButtonUnchanged pins the exemption: the progress bar is a bare
// text button and must never be restyled with an icon.
func TestProgressBarButtonUnchanged(t *testing.T) {
	const progress = "01:23 ─ ─ ▷ ─ ─ ─ ─ 3:35"
	markup := playbackButtons(false, progress)
	found := false
	for _, row := range markup.Rows {
		for _, button := range row.Buttons {
			if button.Text == progress {
				found = true
				if button.Style != nil && button.Style.Icon != 0 {
					t.Error("progress bar should not carry a custom-emoji icon")
				}
			}
		}
	}
	if !found {
		t.Fatal("progress bar button not rendered")
	}
}
