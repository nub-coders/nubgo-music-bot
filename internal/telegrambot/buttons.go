package telegrambot

import (
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
)

const (
	defaultSupportGroup = "nub_coder_s"
	repositoryURL       = "https://github.com/nub-coders/nub-music-bot"
)

func buttonStyle(primary, danger, success bool, icon int64) *telegram.ButtonStyle {
	return &telegram.ButtonStyle{Primary: primary, Danger: danger, Success: success, Icon: icon}
}

func styledData(text, data string, style *telegram.ButtonStyle) telegram.KeyboardInlineButton {
	return telegram.Button.Styled(telegram.Button.Data(text, data), style)
}

func styledURL(text, url string, style *telegram.ButtonStyle) telegram.KeyboardInlineButton {
	return telegram.Button.Styled(telegram.Button.URL(text, url), style)
}

func styledProfile(text string, userID int64, style *telegram.ButtonStyle) telegram.KeyboardInlineButton {
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

func playbackButtons(paused bool) *telegram.ReplyInlineMarkup {
	toggleText, toggleData := "▷", "np:resume"
	toggleStyle := buttonStyle(false, false, true, emojiPlay)
	if !paused {
		toggleText, toggleData = "II", "np:pause"
		toggleStyle = buttonStyle(false, false, false, emojiPause)
	}
	keyboard := telegram.NewKeyboard()
	keyboard.AddRow(
		styledData(toggleText, toggleData, toggleStyle),
		styledData("‣‣I", "np:skip", buttonStyle(true, false, false, emojiSkip)),
		styledData("▢", "np:stop", buttonStyle(false, true, false, emojiStop)),
	)
	keyboard.AddRow(styledData("✖ ᴄʟᴏsᴇ", "np:close", buttonStyle(false, true, false, emojiClose)))
	return keyboard.Build()
}
