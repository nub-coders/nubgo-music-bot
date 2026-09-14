package telegrambot

import (
	"fmt"
	"strings"

	"github.com/nub-coders/gogram/telegram"
	"github.com/nub-coders/nub-go-music-bot/internal/media"
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

// userMention renders an HTML mention link for a Telegram user, mirroring the
// Python bot's `user.mention()`.
func userMention(user *telegram.UserObj) string {
	if user == nil {
		return "ᴜsᴇʀ"
	}
	name := strings.TrimSpace(user.FirstName)
	if name == "" {
		name = "ᴜsᴇʀ"
	}
	return fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, user.ID, escape(name))
}

// formatWelcome substitutes the supported placeholders in a stored start or
// welcome message. The replacement values are already-formatted HTML.
func formatWelcome(tmpl, name string, id int64, botname string) string {
	replacer := strings.NewReplacer(
		"{name}", name,
		"{id}", fmt.Sprint(id),
		"{botname}", botname,
	)
	return replacer.Replace(tmpl)
}

// groupWelcome is the fixed thank-you card sent when the bot is added to a
// group. adders, groupName and botname arrive as pre-formatted HTML (mentions).
func groupWelcome(adder, groupName, botname string) string {
	if botname == "" {
		botname = "ɴᴜʙ ᴍᴜsɪᴄ ʙᴏᴛ"
	}
	return emoji(emojiMusicNote, "🎵") + " <b>ʜᴇʏ " + adder + "!</b> ᴛʜᴀɴᴋs ꜰᴏʀ ᴀᴅᴅɪɴɢ ᴍᴇ ᴛᴏ <b>" + escape(groupName) + "</b> 🎉\n\n" +
		"ɪ'ᴍ <b>" + botname + "</b> — ʏᴏᴜʀ ᴅᴇᴅɪᴄᴀᴛᴇᴅ ᴍᴜsɪᴄ ʙᴏᴛ.\n\n" +
		emoji(emojiMusicNotes, "🎶") + " ᴄʀʏsᴛᴀʟ-ᴄʟᴇᴀʀ ᴠᴏɪᴄᴇ ᴄʜᴀᴛ sᴛʀᴇᴀᴍɪɴɢ\n" +
		emoji(emojiBolt, "⚡️") + " ʙʟᴀᴢɪɴɢ-ꜰᴀsᴛ ᴘʟᴀʏʙᴀᴄᴋ ᴡɪᴛʜ ǫᴜᴇᴜᴇ\n" +
		emoji(emojiRocket, "🚀") + " sᴍᴀʀᴛ ᴀᴜᴛᴏᴘʟᴀʏ &amp; sᴜɢɢᴇsᴛɪᴏɴs (" + richCode("/autoplay") + ")\n" +
		emoji(emojiGlobe, "🌐") + " ʏᴏᴜᴛᴜʙᴇ, sᴘᴏᴛɪꜰʏ &amp; ᴍᴏʀᴇ\n\n" +
		"<i>ᴜsᴇ " + richCode("/play [song]") + " ᴛᴏ ɢᴇᴛ sᴛᴀʀᴛᴇᴅ!</i>"
}

func startCard(uname, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "there"
	}
	botname := "ɴᴜʙ ᴍᴜsɪᴄ ʙᴏᴛ"
	return emoji(emojiUser, "👤") + " <b>ʜᴇʏ " + escape(name) + "!</b>\n\n" +
		emoji(emojiMusicNote, "🎵") + " <b>ᴡᴇʟᴄᴏᴍᴇ ᴛᴏ " + botname + "</b>\n" +
		"<i>ʏᴏᴜʀ ᴜʟᴛɪᴍᴀᴛᴇ ʜɪɢʜ-ǫᴜᴀʟɪᴛʏ ᴍᴜsɪᴄ &amp; ᴠɪᴅᴇᴏ sᴛʀᴇᴀᴍɪɴɢ ʙᴏᴛ ꜰᴏʀ ᴛᴇʟᴇɢʀᴀᴍ!</i>\n\n" +
		"✨ <b><u>sᴘᴇᴄɪᴀʟ ꜰᴇᴀᴛᴜʀᴇs</u></b> ✨\n\n" +
		"• " + emoji(emojiHeadphones, "🎧") + " <b>ᴜʟᴛʀᴀ-ʜᴅ sᴛʀᴇᴀᴍɪɴɢ:</b> <i>ᴄʀʏsᴛᴀʟ-ᴄʟᴇᴀʀ ᴀᴜᴅɪᴏ &amp; ᴠɪᴅᴇᴏ in ɢʀᴏᴜᴘs &amp; ᴄʜᴀɴɴᴇʟs.</i>\n" +
		"• " + emoji(emojiRocket, "🚀") + " <b>sᴍᴀʀᴛ ᴀᴜᴛᴏᴘʟᴀʏ (" + richCode("/autoplay") + "):</b> <i>ᴀᴜᴛᴏ-ᴘʟᴀʏs ʀᴇʟᴀᴛᴇᴅ sᴏɴɢs sᴏ ᴍᴜsɪᴄ ɴᴇᴠᴇʀ sᴛᴏᴘs.</i>\n" +
		"• " + emoji(emojiBolt, "⚡️") + " <b>ᴍᴜʟᴛɪ-ᴀssɪsᴛᴀɴᴛ:</b> <i>sᴇᴀᴍʟᴇss ʟᴏᴀᴅ-ʙᴀʟᴀɴᴄɪɴɢ ᴀᴄʀᴏss ᴍᴜʟᴛɪᴘʟᴇ ᴀssɪsᴛᴀɴᴛs.</i>\n" +
		"• " + emoji(emojiSettings, "⚙️") + " <b>ᴀᴅᴠᴀɴᴄᴇᴅ ᴄᴏɴᴛʀᴏʟs:</b> <i>sᴇᴇᴋ (" + richCode("/seek") + "), ʟᴏᴏᴘ (" + richCode("/loop") + "), ꩖ᴏʀᴄᴇ-ᴘʟᴀʏ &amp; sʜᴜꜰꜰʟᴇ.</i>\n" +
		"• " + emoji(emojiTools, "🛠️") + " <b>ꜰᴜɴ &amp; ᴜᴛɪʟɪᴛɪᴇs:</b> <i>sᴛɪᴄᴋᴇʀ ᴄʟᴏɴɪɴɢ (" + richCode("/kang") + "), ᴍᴇᴍᴇs (" + richCode("/mmf") + ") &amp; ᴛᴀɢᴀʟʟ.</i>\n\n" +
		"<b><i>👇 ᴜsᴇ ᴛʜᴇ ʙᴜᴛᴛᴏɴs ʙᴇʟᴏᴡ ᴛᴏ ᴇxᴘʟᴏʀᴇ ᴀʟʟ ᴄᴏᴍᴍᴀɴᴅs!</i></b>"
}

func queueCard(track media.Track, position int, botUsername string) string {
	title := track.Title
	if title == "" {
		title = track.OriginalInput
	}
	titleFormatted := escape(title)
	videoID := ""
	if track.Kind == media.SourceYouTube && track.ID != "" {
		videoID = track.ID
	}
	displayTitle := "<b>" + titleFormatted + "</b>"
	if videoID != "" && botUsername != "" {
		displayTitle = fmt.Sprintf(`<a href="https://t.me/%s?start=vidid_%s"><b>%s</b></a>`, botUsername, videoID, titleFormatted)
	} else if link := mediaWwwLink(track); strings.HasPrefix(link, "http") {
		displayTitle = fmt.Sprintf(`<a href="%s"><b>%s</b></a>`, escape(link), titleFormatted)
	}

	duration := "-"
	if track.Duration > 0 {
		duration = formatDuration(track.Duration)
	}

	requester := "ᴜsᴇʀ"
	if track.RequesterID > 0 {
		reqName := track.RequesterName
		if reqName == "" {
			reqName = "ᴜsᴇʀ"
		}
		requester = fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, track.RequesterID, escape(reqName))
	} else if track.RequesterName != "" {
		requester = escape(track.RequesterName)
	}

	mode := "Audio"
	if track.Video {
		mode = "Video"
	}

	return fmt.Sprintf("%s <b>ᴀᴅᴅᴇᴅ ᴛᴏ ǫᴜᴇᴜᴇ</b>\n\n<p>\n<b>‣ ᴛɪᴛʟᴇ:</b> %s<br/>\n<b>‣ ᴅᴜʀᴀᴛɪᴏɴ:</b> <code>%s</code><br/>\n<b>‣ ᴘᴏsɪᴛɪᴏɴ:</b> %s<br/>\n<b>‣ ᴍᴏᴅᴇ:</b> <code>%s</code><br/>\n<b>‣ ʀᴇǫᴜᴇsᴛᴇᴅ ʙʏ:</b> %s\n</p>",
		emoji(emojiAdd, "➕"),
		displayTitle,
		duration,
		positionTag(position),
		mode,
		requester,
	)
}

func msgPaused(req string) string {
	return fmt.Sprintf("<b>%s sᴏɴɢ ᴘᴀᴜsᴇᴅ.</b>\n\n<b>‣ ʀᴇǫᴜᴇsᴛᴇᴅ ʙʏ:</b> %s", emoji(emojiPause, "🔇"), req)
}

func msgResumed(req string) string {
	return fmt.Sprintf("<b>%s sᴏɴɢ ʀᴇsᴜᴍᴇᴅ.</b>\n\n<b>‣ ʀᴇǫᴜᴇsᴛᴇᴅ ʙʏ:</b> %s", emoji(emojiPlay, "🎞"), req)
}

func msgSkipping(req string) string {
	return fmt.Sprintf("%s <b>sᴋɪᴘᴘɪɴɢ ᴄᴜʀʀᴇɴᴛ ᴛʀᴀᴄᴋ...</b>\n<b>‣ ʀᴇǫᴜᴇsᴛᴇᴅ ʙʏ:</b> %s", emoji(emojiSkip, "➡️"), req)
}

func msgSkippedEmpty(req string) string {
	return fmt.Sprintf("%s <b>ǫᴜᴇᴜᴇ ɪs ᴇᴍᴘᴛʏ ɴᴏᴡ.</b>\n<b>‣ ʀᴇǫᴜᴇsᴛᴇᴅ ʙʏ:</b> %s", emoji(emojiSkip, "➡️"), req)
}

func msgStopped(req string) string {
	return fmt.Sprintf("<b>%s ǫᴜᴇᴜᴇ ᴄʟᴇᴀʀᴇᴅ</b>\n<b>‣ sᴛʀᴇᴀᴍɪɴɢ sᴛᴏᴘᴘᴇᴅ</b>\n<b>‣ ʀᴇǫᴜᴇsᴛᴇᴅ ʙʏ:</b> %s", emoji(emojiStop, "🚫"), req)
}

func msgLooped(count, req string) string {
	return fmt.Sprintf("<b>%s ᴄᴜʀʀᴇɴᴛ sᴏɴɢ ᴡɪʟʟ ʙᴇ ʀᴇᴘᴇᴀᴛᴇᴅ %s ᴛɪᴍᴇs!</b>\n\n<b>‣ ʀᴇǫᴜᴇsᴛᴇᴅ ʙʏ:</b> %s", emoji(emojiLoop, "🔄"), count, req)
}

func msgSeeked(seconds, req string) string {
	return fmt.Sprintf("%s <b>sᴇᴇᴋᴇᴅ ᴛᴏ %s!</b>\n\n<b>‣ ʀᴇǫᴜᴇsᴛᴇᴅ ʙʏ:</b> %s", emoji(emojiSuccess, "✅"), seconds, req)
}

func msgShuffled(count int) string {
	return fmt.Sprintf("%s <b>sʜᴜꜰꜰʟᴇᴅ %d ᴜᴘᴄᴏᴍɪɴɢ ᴛʀᴀᴄᴋ(s).</b>", emoji(emojiRefresh, "🔄"), count)
}

func msgQueueEmpty() string {
	return richNote(emoji(emojiQueueIcon, "🗃") + " <b>ǫᴜᴇᴜᴇ ɪs ᴇᴍᴘᴛʏ.</b>")
}

func msgNoStream() string {
	return richNote(emoji(emojiError, "❌") + " <b>ɴᴏ ᴀᴄᴛɪᴠᴇ sᴛʀᴇᴀᴍ ʀɪɢʜᴛ ɴᴏᴡ.</b>")
}

func msgNoActiveVC() string {
	return richNote(emoji(emojiWarning, "⚠️") + " <b>ɴᴏ ᴀᴄᴛɪᴠᴇ ᴠᴏɪᴄᴇ ᴄʜᴀᴛ ꜰᴏᴜɴᴅ.</b>\n<i>ᴘʟᴇᴀsᴇ sᴛᴀʀᴛ ᴛʜᴇ ᴠᴏɪᴄᴇ ᴄʜᴀᴛ ꜰɪʀsᴛ, ᴏʀ ᴍᴀᴋᴇ ᴛʜᴇ ᴀssɪsᴛᴀɴᴛ ᴀɴ ᴀᴅᴍɪɴ ᴛᴏ sᴛᴀʀᴛ ɪᴛ ᴀᴜᴛᴏᴍᴀᴛɪᴄᴀʟʟʏ.</i>")
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
		{emoji(emojiChat, "💬") + " " + cmdMark("/welcome"), "ᴠɪᴇᴡ sᴛᴀʀᴛ ᴡᴇʟᴄᴏᴍᴇ"},
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
		{emoji(emojiPin, "📌") + " " + cmdMark("/setwelcome"), "sᴇᴛ sᴛᴀʀᴛ ᴡᴇʟᴄᴏᴍᴇ (ʀᴇᴘʟʏ)"},
		{emoji(emojiRefresh, "🔄") + " " + cmdMark("/resetwelcome"), "ʀᴇsᴇᴛ ᴡᴇʟᴄᴏᴍᴇ &amp; ʟᴏɢᴏ"},
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
