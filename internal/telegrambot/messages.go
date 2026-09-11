package telegrambot

import (
	"fmt"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
)

func richEsc(v any) string { return escape(v) }

func richHeading(t string, l int) string {
	if l < 1 {
		l = 1
	}
	if l > 6 {
		l = 6
	}
	return fmt.Sprintf("<h%d>%s</h%d>", l, t, l)
}

func richCode(v any) string { return "<code>" + escape(v) + "</code>" }

func richNote(t string) string { return "<blockquote>" + t + "</blockquote>" }

func richDetails(summary, body string, open bool) string {
	attr := ""
	if open {
		attr = " open"
	}
	return fmt.Sprintf("<details%s><summary>%s</summary>%s</details>", attr, summary, body)
}

func richTable(headers []string, rows [][]string) string {
	var b strings.Builder
	b.WriteString(`<table border="1">`)
	if len(headers) > 0 {
		b.WriteString("<tr>")
		for _, h := range headers {
			b.WriteString("<th>")
			b.WriteString(h)
			b.WriteString("</th>")
		}
		b.WriteString("</tr>")
	}
	for _, row := range rows {
		b.WriteString("<tr>")
		for _, cell := range row {
			b.WriteString("<td>")
			b.WriteString(cell)
			b.WriteString("</td>")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</table>")
	return b.String()
}

func richKVTable(pairs [][2]string) string {
	rows := make([][]string, 0, len(pairs))
	for _, p := range pairs {
		if p[1] == "" {
			continue
		}
		rows = append(rows, []string{fmt.Sprintf("<b>%s</b>", p[0]), p[1]})
	}
	return richTable(nil, rows)
}

func helpBackButtons() *telegram.ReplyInlineMarkup {
	kb := telegram.NewKeyboard()
	kb.AddRow(styledData("◀️ ʙᴀᴄᴋ", "commands_all", buttonStyle(false, false, false, emojiBack)))
	return kb.Build()
}

func cmdMark(c string) string { return "<mark>" + richCode(c) + "</mark>" }

func startCard(uname, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "there"
	}
	heading := richHeading(emoji(emojiUser, "👤")+" ʜᴇʏ "+escape(name)+"!", 1)
	sub := richHeading(emoji(emojiMusicNote, "🎵")+" ᴡᴇʟᴄᴏᴍᴇ ᴛᴏ ɴᴜʙ ᴍᴜsɪᴄ ʙᴏᴛ", 2)
	intro := "<p><i>ʏᴏᴜʀ ᴜʟᴛɪᴍᴀᴛᴇ ʜɪɢʜ-ǫᴜᴀʟɪᴛʏ ᴍᴜsɪᴄ & ᴠɪᴅᴇᴏ sᴛʀᴇᴀᴍɪɴɢ ʙᴏᴛ ꜰᴏʀ ᴛᴇʟᴇɢʀᴀᴍ!</i></p>"

	ftitle := "<p>✨ <b><u>sᴘᴇᴄɪᴀʟ ꜰᴇᴀᴛᴜʀᴇs</u></b> ✨</p>"
	ftxt := "<p>"
	ftxt += "• " + emoji(emojiHeadphones, "🎧") + " <b>ᴜʟᴛʀᴀ-ʜᴅ sᴛʀᴇᴀᴍɪɴɢ:</b> <i>ᴄʀʏsᴛᴀʟ-ᴄʟᴇᴀʀ ᴀᴜᴅɪᴏ & ᴠɪᴅᴇᴏ ɪɴ ɢʀᴏᴜᴘs</i><br/>"
	ftxt += "• " + emoji(emojiRocket, "🚀") + " <b>sᴍᴀʀᴛ ǫᴜᴇᴜᴇ:</b> <i>ᴘʟᴀʏʟɪsᴛs, ǫᴜᴇᴜᴇ & sʜᴜꜰʟᴇ</i><br/>"
	ftxt += "• " + emoji(emojiBolt, "⚡️") + " <b>ᴍᴜʟᴛɪ-ᴀssɪsᴛᴀɴᴛ:</b> <i>sᴇᴀᴍʟᴇss ʟᴏᴀᴅ-ʙᴀʟᴀɴᴄɪɴɢ ᴀᴄʀᴏss ᴀssɪsᴛᴀɴᴛs</i><br/>"
	ftxt += "• " + emoji(emojiSettings, "⚙️") + " <b>ᴀᴅᴠᴀɴᴄᴇᴅ ᴄᴏɴᴛʀᴏʟs:</b> <i>sᴇᴇᴋ (" + richCode("/seek") + "), ʟᴏᴏᴘ (" + richCode("/loop") + "), sʜᴜꜰʟᴇ</i><br/>"
	ftxt += "• " + emoji(emojiTools, "🛠️") + " <b>ᴜᴛɪʟs:</b> <i>sᴛᴀᴛs, ᴛᴏᴏʟs, ᴍᴇᴅɪᴀ, ᴡᴇʟᴄᴏᴍᴇ</i>"
	ftxt += "</p>"

	cta := richNote(emoji(emojiInfo, "ℹ️") + " <i>👇 ᴜsᴇ ᴛʜᴇ ʙᴜᴛᴛᴏɴs ʙᴇʟᴏᴡ ᴛᴏ ᴇxᴘʟᴏʀᴇ ᴀʟʟ ᴄᴏᴍᴍᴀɴᴅs!</i>")
	_ = uname
	return heading + sub + intro + ftitle + ftxt + cta
}

func helpCategorySelect(showAdmin bool) string {
	_ = showAdmin
	title := "<u><b>" + emoji(emojiInfo, "ℹ️") + " | sᴇʟᴇᴄᴛ ᴀ ᴄᴏᴍᴍᴀɴᴅ ᴄᴀᴛᴇɢᴏʀʏ</b></u>"
	note := richNote(emoji(emojiHelp, "ℹ️") + " <i>ᴄʜᴏᴏsᴇ ᴀ ᴄᴀᴛᴇɢᴏʀʏ ʙᴇʟᴏᴡ. ᴀᴅᴍɪɴ ᴄᴀᴛᴇɢᴏʀɪᴇs ᴀʀᴇ ᴠɪsɪʙʟᴇ ᴛᴏ ᴏᴡɴᴇʀ/sᴜᴅᴏ ᴏɴʟʏ.</i>")
	return title + note
}

func helpCategoryPage(cat string, showAdmin bool) (string, *telegram.ReplyInlineMarkup) {
	cat = strings.ToLower(strings.TrimSpace(cat))
	if cat == "all" || cat == "home" || cat == "help" {
		return helpCategorySelect(showAdmin), helpButtons(showAdmin)
	}

	headers := []string{"ᴄᴏᴍᴍᴀɴᴅ", "ᴅᴇsᴄʀɪᴘᴛɪᴏɴ"}
	playback := [][]string{
		{emoji(emojiPlay, "🎞") + " " + cmdMark("/play") + " " + richCode("/vplay"), "ǫᴜᴇᴜᴇ ʏᴏᴜᴛᴜʙᴇ ᴀᴜᴅɪᴏ/ᴠɪᴅᴇᴏ"},
		{emoji(emojiQueueIcon, "🗃") + " " + cmdMark("/queue") + " " + richCode("/q"), "sʜᴏᴡ ᴄᴜʀʀᴇɴᴛ ǫᴜᴇᴜᴇ"},
		{emoji(emojiMusicNotes, "🎶") + " " + cmdMark("/playlist") + " " + richCode("/myplaylist"), "ᴍᴀɴᴀɢᴇ ʏᴏᴜʀ sᴀᴠᴇᴅ ᴘʟᴀʏʟɪsᴛs"},
		{emoji(emojiPlay, "🎞") + " " + cmdMark("/pl") + " " + richCode("/pplay") + " <name>", "ᴘʟᴀʏ ʏᴏᴜʀ ᴘʟᴀʏʟɪsᴛ"},
		{emoji(emojiNowPlaying, "🎵") + " " + cmdMark("/np") + " " + richCode("/nowplaying"), "sʜᴏᴡ ᴄᴜʀʀᴇɴᴛʟʏ ᴘʟᴀʏɪɴɢ"},
		{emoji(emojiPause, "🔇") + " " + cmdMark("/pause") + " " + richCode("/resume"), "ᴘᴀᴜsᴇ / ʀᴇsᴜᴍᴇ sᴛʀᴇᴀᴍ"},
		{emoji(emojiSkip, "➡️") + " " + cmdMark("/skip"), "ɴᴇxᴛ ᴛʀᴀᴄᴋ"},
		{emoji(emojiStop, "🚫") + " " + cmdMark("/stop") + " " + richCode("/end"), "sᴛᴏᴘ & ᴄʟᴇᴀʀ ǫᴜᴇᴜᴇ"},
		{emoji(emojiRefresh, "🔄") + " " + cmdMark("/shuffle"), "sʜᴜꜰʟᴇ ǫᴜᴇᴜᴇ"},
		{emoji(emojiNext, "➡️") + " " + cmdMark("/seek <sec>") + " " + richCode("/seekback"), "ᴊᴜᴍᴘ ꜰᴏʀᴡᴀʀᴅ / ʙᴀᴄᴋᴡᴀʀᴅ"},
		{emoji(emojiLoop, "🔄") + " " + cmdMark("/loop <off|track|queue>"), "ʟᴏᴏᴘ ᴍᴏᴅᴇ"},
		{emoji(emojiSettings, "⚙️") + " " + cmdMark("/autoplay") + " " + richCode("on|off"), "ᴘʟᴀʏ ʀᴇʟᴀᴛᴇᴅ sᴏɴɢs ᴡʜᴇɴ ᴛʜᴇ ǫᴜᴇᴜᴇ ᴇɴᴅs"},
		{emoji(emojiMic, "🎤") + " " + cmdMark("/play <query|url>"), "ʏᴏᴜᴛᴜʙᴇ / sᴘᴏᴛɪꜰʏ / ᴅɪʀᴇᴄᴛ ᴜʀʟs"},
	}

	tools := [][]string{
		{emoji(emojiBolt, "⚡️") + " " + cmdMark("/ping"), "ʟᴀᴛᴇɴᴄʏ & ᴜᴘᴛɪᴍᴇ"},
		{emoji(emojiStats, "📊") + " " + cmdMark("/stats"), "ʙᴏᴛ ᴜsᴀɢᴇ sᴛᴀᴛs"},
		{emoji(emojiInfo, "ℹ️") + " " + cmdMark("/about"), "ᴀʙᴏᴜᴛ ᴛʜɪs ʙᴏᴛ"},
		{emoji(emojiChat, "💬") + " " + cmdMark("/welcome"), "ᴠɪᴇᴡ ᴄʜᴀᴛ ᴡᴇʟᴄᴏᴍᴇ"},
	}

	admin := [][]string{
		{emoji(emojiLock, "🔒") + " " + cmdMark("/auth <reply|id>"), "ᴀʟʟᴏᴡ ᴜsᴇʀ ᴛᴏ ᴄᴏɴᴛʀᴏʟ ᴘʟᴀʏᴇʀ"},
		{emoji(emojiUnlock, "🔓") + " " + cmdMark("/unauth <reply|id>"), "ʀᴇᴍᴏᴠᴇ ᴘʟᴀʏᴇʀ ᴘᴇʀᴍɪssɪᴏɴ"},
		{emoji(emojiUser, "👤") + " " + cmdMark("/authlist"), "ʟɪsᴛ ᴀᴜᴛʜᴏʀɪᴢᴇᴅ ᴜsᴇʀs"},
		{emoji(emojiBlocked, "🚫") + " " + cmdMark("/block <reply|id>"), "ʙʟᴏᴄᴋ ᴜsᴇʀ"},
		{emoji(emojiSuccess, "✅") + " " + cmdMark("/unblock <reply|id>"), "ᴜɴʙʟᴏᴄᴋ ᴜsᴇʀ"},
		{emoji(emojiUsers, "👥") + " " + cmdMark("/blocklist"), "ᴠɪᴇᴡ ʙʟᴏᴄᴋᴇᴅ ʟɪsᴛ"},
		{emoji(emojiKey, "🔑") + " " + cmdMark("/sudo") + " " + richCode("/delsudo"), "ᴀᴅᴅ/ʀᴇᴍᴏᴠᴇ sᴜᴅᴏ"},
		{emoji(emojiCrown, "👑") + " " + cmdMark("/sudolist"), "ʟɪsᴛ sᴜᴅᴏ ᴜsᴇʀs"},
		{emoji(emojiBroadcast, "📢") + " " + cmdMark("/broadcast") + " " + richCode("/fbroadcast"), "ʙʀᴏᴀᴅᴄᴀsᴛ ᴛᴏ ᴄʜᴀᴛs"},
		{emoji(emojiPin, "📌") + " " + cmdMark("/setwelcome <text>"), "sᴇᴛ ᴄᴜsᴛᴏᴍ ᴡᴇʟᴄᴏᴍᴇ"},
	}

	page := func(title string, rows [][]string, open bool) string {
		return richDetails(title, richTable(headers, rows), open)
	}
	playbackPage := page(emoji(emojiMusicNote, "🎵")+" ᴘʟᴀʏʙᴀᴄᴋ", playback, true)
	toolsPage := page(emoji(emojiTools, "🛠️")+" ᴛᴏᴏʟs", tools, false)
	adminPage := page(emoji(emojiKey, "🔑")+" ᴀᴅᴍɪɴ", admin, false)

	switch cat {
	case "playback":
		return playbackPage, helpBackButtons()
	case "tools":
		return toolsPage, helpBackButtons()
	case "admin":
		if !showAdmin {
			return "", nil
		}
		return adminPage, helpBackButtons()
	case "all_dropdown":
		merged := playbackPage + "\n" + toolsPage
		if showAdmin {
			merged += "\n" + adminPage
		}
		return merged, helpBackButtons()
	default:
		return "", nil
	}
}
