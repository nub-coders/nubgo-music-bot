package telegrambot

import (
	"regexp"
	"strings"

	"github.com/nub-coders/gogram/telegram"
)

const (
	defaultSupportGroup = "nub_coder_s"
	repositoryURL       = "https://github.com/nub-coders/nub-music-bot"
)

var buttonOnlyGlyphs = map[string]int64{
	"II":  emojiPause,
	"‣‣I": emojiSkip,
	"▷":   emojiPlay,
	"▢":   emojiStop,
	"‣":   emojiPlay,
}

var leadingEmojiRE = regexp.MustCompile(`^([\x{2139}]|[^\p{L}\p{N}\s])[\x{fe0e}\x{fe0f}]*\s*`)

func buttonStyle(primary, danger, success bool, icon int64) *telegram.ButtonStyle {
	return &telegram.ButtonStyle{Primary: primary, Danger: danger, Success: success, Icon: icon}
}

// detectAndStripButtonEmoji strips duplicate plain unicode emojis from button labels
// when a custom-emoji icon is used, and turns button-only transport glyphs into
// zero-width space, exactly matching nub-music-bot's _detect_and_strip_button_emoji.
func detectAndStripButtonEmoji(text string, style *telegram.ButtonStyle) (string, *telegram.ButtonStyle) {
	if text == "" {
		return text, style
	}
	if style == nil {
		style = &telegram.ButtonStyle{}
	}

	// 1. A button whose whole label is one of the fallback transport shapes (▷, II, ‣‣I, ▢, ‣):
	// the icon replaces it, so the label goes blank — zero-width space, matching nub-music-bot.
	if icon, ok := buttonOnlyGlyphs[text]; ok {
		if style.Icon == 0 {
			style.Icon = icon
		}
		return "\u200b", style
	}

	// 2. Strip leading 📌 if present (e.g. "📌Pɪɴ ✅")
	if strings.HasPrefix(text, "📌") {
		text = strings.TrimSpace(strings.TrimPrefix(text, "📌"))
		if style.Icon == 0 {
			style.Icon = emojiPin
		}
	}

	// 3. Detect trailing toggle checkmarks/crosses/emojis (e.g. "Group ✅", "From bot ⬇️")
	for _, k := range sortedEmojiKeys {
		if strings.HasSuffix(text, k) || strings.HasSuffix(text, " "+k) {
			clean := strings.TrimSuffix(text, k)
			clean = strings.TrimRight(clean, " ")
			for strings.HasSuffix(clean, k) {
				clean = strings.TrimRight(strings.TrimSuffix(clean, k), " ")
			}
			if clean != "" {
				text = clean
				if style.Icon == 0 {
					style.Icon = unicodeToCustomEmoji[k]
				}
				break
			}
		}
	}

	// 4. If style.Icon is already set, strip any leading emoji from text.
	if style.Icon != 0 {
		for _, k := range sortedEmojiKeys {
			if strings.HasPrefix(text, k) {
				text = strings.TrimSpace(strings.TrimPrefix(text, k))
				break
			}
		}
		text = leadingEmojiRE.ReplaceAllString(text, "")
		return strings.TrimSpace(text), style
	}

	// 5. Otherwise, detect leading emoji and assign it to style.Icon
	for _, k := range sortedEmojiKeys {
		if strings.HasPrefix(text, k) {
			style.Icon = unicodeToCustomEmoji[k]
			text = strings.TrimSpace(strings.TrimPrefix(text, k))
			break
		}
	}
	if style.Icon != 0 {
		text = leadingEmojiRE.ReplaceAllString(text, "")
	}

	return strings.TrimSpace(text), style
}

func ensureButtonIcon(text string, style *telegram.ButtonStyle) *telegram.ButtonStyle {
	_, style = detectAndStripButtonEmoji(text, style)
	return style
}

func styledData(text, data string, style *telegram.ButtonStyle) telegram.KeyboardInlineButton {
	text, style = detectAndStripButtonEmoji(text, style)
	return telegram.Button.Styled(telegram.Button.Data(text, data), style)
}

func styledURL(text, url string, style *telegram.ButtonStyle) telegram.KeyboardInlineButton {
	text, style = detectAndStripButtonEmoji(text, style)
	return telegram.Button.Styled(telegram.Button.URL(text, url), style)
}

func styledProfile(text string, userID int64, style *telegram.ButtonStyle) telegram.KeyboardInlineButton {
	text, style = detectAndStripButtonEmoji(text, style)
	return telegram.Button.Styled(telegram.KeyboardInlineButton{
		Text: text,
		Type: &telegram.InlineButtonTypeUserProfile{UserID: userID},
	}, style)
}

func supportGroup(group string) string {
	group = strings.TrimSpace(strings.TrimPrefix(group, "@"))
	if group == "" {
		return defaultSupportGroup
	}
	return group
}

func botUsername(username string) string {
	return strings.TrimSpace(strings.TrimPrefix(username, "@"))
}

// startButtons mirrors the Python bot's start card, using native custom-emoji
// button icons and styled backgrounds.
func startButtons(username string, ownerID int64, group string) *telegram.ReplyInlineMarkup {
	username = botUsername(username)
	group = supportGroup(group)
	keyboard := telegram.NewKeyboard()
	if username != "" {
		keyboard.AddRow(styledURL(
			"➕ ᴀᴅᴅ ᴍᴇ ᴛᴏ ɢʀᴏᴜᴘ",
			"https://t.me/"+username+"?startgroup=true",
			buttonStyle(true, false, false, emojiAdd),
		))
	}
	keyboard.AddRow(styledData(
		"ℹ️ ʜᴇʟᴘ & ᴄᴏᴍᴍᴀɴᴅs",
		"commands_all",
		buttonStyle(true, false, false, emojiHelp),
	))
	if ownerID > 0 {
		keyboard.AddRow(
			styledProfile("👑 ᴄʀᴇᴀᴛᴏʀ", ownerID, buttonStyle(false, false, false, emojiCrown)),
			styledURL("💬 sᴜᴘᴘᴏʀᴛ ᴄʜᴀᴛ", "https://t.me/"+group, buttonStyle(false, false, false, emojiChat)),
		)
	} else {
		keyboard.AddRow(styledURL(
			"💬 sᴜᴘᴘᴏʀᴛ ᴄʜᴀᴛ",
			"https://t.me/"+group,
			buttonStyle(false, false, false, emojiChat),
		))
	}
	keyboard.AddRow(styledURL(
		"🌐 ʀᴇᴘᴏ",
		repositoryURL,
		buttonStyle(false, false, false, emojiRepo),
	))
	return keyboard.Build()
}

func groupWelcomeButtons(username, group string) *telegram.ReplyInlineMarkup {
	username = botUsername(username)
	group = supportGroup(group)
	keyboard := telegram.NewKeyboard()
	if username != "" {
		keyboard.AddRow(styledURL(
			"📖 ʜᴇʟᴘ & ᴄᴏᴍᴍᴀɴᴅs",
			"https://t.me/"+username+"?start=help",
			buttonStyle(true, false, false, emojiHelp),
		))
	}
	keyboard.AddRow(
		styledURL("➕ ᴀᴅᴅ ᴛᴏ ɢʀᴏᴜᴘ", "https://t.me/"+username+"?startgroup=true", buttonStyle(false, false, false, emojiAdd)),
		styledURL("💬 sᴜᴘᴘᴏʀᴛ", "https://t.me/"+group, buttonStyle(false, false, false, emojiChat)),
	)
	return keyboard.Build()
}

func helpButtons(showAdmin bool) *telegram.ReplyInlineMarkup {
	keyboard := telegram.NewKeyboard()
	keyboard.AddRow(
		styledData("🎵 ᴘʟᴀʏʙᴀᴄᴋ", "commands_playback", buttonStyle(true, false, false, emojiMusicNote)),
		styledData("🛠️ ᴛᴏᴏʟs & ɪɴꜰᴏ", "commands_tools", buttonStyle(false, false, false, emojiTools)),
	)
	if showAdmin {
		keyboard.AddRow(styledData(
			"🔐 ᴀᴅᴍɪɴ & sᴜᴅᴏ",
			"commands_admin",
			buttonStyle(true, false, false, emojiKey),
		))
	}
	keyboard.AddRow(styledData(
		"📋 ᴀʟʟ ᴄᴏᴍᴍᴀɴᴅs (ᴅʀᴏᴘᴅᴏᴡɴs)",
		"commands_all_dropdown",
		buttonStyle(false, false, true, emojiHelp),
	))
	keyboard.AddRow(styledData("🏠 ʜᴏᴍᴇ", "commands_home", buttonStyle(false, false, false, emojiHome)))
	return keyboard.Build()
}

// playbackButtons renders the control row exactly matching nub-music-bot's playback_markup:
// Row 1: ▷ (resume), II (pause), ‣‣I (skip), ▢ (end/stop)
// Row 2: progress bar button (if duration known & not empty)
// Row 3: ✖ ᴄʟᴏsᴇ (close)
func playbackButtons(paused bool, progressText string) *telegram.ReplyInlineMarkup {
	return playbackButtonsPrefixed(progressText, "")
}

func playbackButtonsPrefixed(progressText string, prefix string) *telegram.ReplyInlineMarkup {
	keyboard := telegram.NewKeyboard()
	keyboard.AddRow(
		styledData("▷", prefix+"resume", buttonStyle(false, false, true, emojiPlay)),
		styledData("II", prefix+"pause", buttonStyle(false, false, false, emojiPause)),
		styledData("‣‣I", prefix+"skip", buttonStyle(true, false, false, emojiSkip)),
		styledData("▢", prefix+"end", buttonStyle(false, true, false, emojiStop)),
	)
	if progressText != "" {
		keyboard.AddRow(telegram.Button.Disabled(progressText))
	}
	keyboard.AddRow(styledData("✖ ᴄʟᴏsᴇ", "close", buttonStyle(false, true, false, emojiClose)))
	return keyboard.Build()
}

// queueButtons builds the markup for freshly queued tracks, matching nub-music-bot queue_markup:
// Row 1: ▷, II, ‣‣I, ▢
// Row 2: ‣ ᴘʟᴀʏ ɴᴏᴡ
// Row 3: ✖ ᴄʟᴏsᴇ
func queueButtons(trackID string, channelMode bool) *telegram.ReplyInlineMarkup {
	prefix := ""
	if channelMode {
		prefix = "c"
	}
	keyboard := telegram.NewKeyboard()
	keyboard.AddRow(
		styledData("▷", prefix+"resume", buttonStyle(false, false, true, emojiPlay)),
		styledData("II", prefix+"pause", buttonStyle(false, false, false, emojiPause)),
		styledData("‣‣I", prefix+"skip", buttonStyle(true, false, false, emojiSkip)),
		styledData("▢", prefix+"end", buttonStyle(false, true, false, emojiStop)),
	)
	if trackID != "" {
		keyboard.AddRow(styledData("‣ ᴘʟᴀʏ ɴᴏᴡ", prefix+"playnow_"+trackID, buttonStyle(false, false, true, emojiPlay)))
	}
	keyboard.AddRow(styledData("✖ ᴄʟᴏsᴇ", "close", buttonStyle(false, true, false, emojiClose)))
	return keyboard.Build()
}

// autoleaveButtons mirrors Buttons.autoleave_markup() from nub-music-bot.
func autoleaveButtons() *telegram.ReplyInlineMarkup {
	keyboard := telegram.NewKeyboard()
	keyboard.AddRow(styledURL("🤖 ᴏᴜʀ ʙᴏᴛs", "https://t.me/+FbIuEWrOYlEwYzM1", buttonStyle(true, false, false, emojiUser)))
	return keyboard.Build()
}
