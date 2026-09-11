package telegrambot

import (
	"testing"

	"github.com/amarnathcjd/gogram/telegram"
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

// TestButtonsHaveNoPlainEmoji enforces the UI rule that buttons carry their
// glyph through the custom-emoji icon, not a plain emoji in the label. The
// progress bar is exempt by design: it is a disabled text button, not a glyph.
func TestButtonsHaveNoPlainEmoji(t *testing.T) {
	for _, text := range allButtonTexts() {
		for _, r := range text {
			if forbiddenEmojiRune(r) {
				t.Errorf("button %q contains plain emoji %q (U+%04X)", text, r, r)
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
