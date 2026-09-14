package telegrambot

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"github.com/nub-coders/gogram/telegram"
)

// Custom emoji document IDs are shared with the Python bot's UI.
const (
	emojiMusicNote  int64 = 5891249688933305846
	emojiMusicNotes int64 = 5915480455603295660
	emojiHeadphones int64 = 6007938409857815902
	emojiMic        int64 = 5897554554894946515
	emojiBroadcast  int64 = 5424818078833715060
	emojiPlay       int64 = 5775981206319402773
	emojiSkip       int64 = 5875506366050734240
	emojiPause      int64 = 5890838600433536921
	emojiStop       int64 = 5872829476143894491
	emojiLoop       int64 = 5839200986022812209
	emojiBolt       int64 = 5843553939672274145
	emojiNowPlaying int64 = 5890831539507302154
	emojiQueueIcon  int64 = 5877316724830768997
	emojiLoading    int64 = 5787237370709413702
	emojiSettings   int64 = 5787237370709413702
	emojiInfo       int64 = 5879785854284599288
	emojiStats      int64 = 5877485980901971030
	emojiSuccess    int64 = 5776375003280838798
	emojiError      int64 = 5778527486270770928
	emojiWarning    int64 = 5881702736843511327
	emojiBlocked    int64 = 5877413297170419326
	emojiLock       int64 = 5879895758202735862
	emojiUnlock     int64 = 6034962180875490251
	emojiShield     int64 = 5926783847453692661
	emojiCrown      int64 = 5807868868886009920
	emojiUser       int64 = 5771887475421090729
	emojiUsers      int64 = 5942877472163892475
	emojiKey        int64 = 6005570495603282482
	emojiFire       int64 = 6008118472066732010
	emojiBack       int64 = 5877629862306385808
	emojiHome       int64 = 5967822972931542886
	emojiRefresh    int64 = 5877410604225924969
	emojiRepo       int64 = 5877465816030515018
	emojiNext       int64 = 5877468380125990242
	emojiAdd        int64 = 5775937998948404844
	emojiPin        int64 = 5908961403917570106
	emojiChat       int64 = 5884179047482659474
	emojiSend       int64 = 5913236481220022288
	emojiRocket     int64 = 5857290546459973028
	emojiGlobe      int64 = 5879585266426973039
	emojiLink       int64 = 5778586619380503542
	emojiTools      int64 = 5988023995125993550
	emojiHelp       int64 = 5879785854284599288
	emojiClose      int64 = 5778527486270770928
	emojiResume     int64 = 5775981206319402773
)

var emojiDigits = map[rune]int64{
	'0': 5771423202341303860,
	'1': 5771785311034021057,
	'2': 5773975314858251177,
	'3': 5771591496339820740,
	'4': 5771376816694498290,
	'5': 5771694571259958563,
	'6': 5773735118812221922,
	'7': 5773913385724809382,
	'8': 5773786959067484290,
	'9': 5774138390471512165,
}

func positionTag(n int) string {
	s := fmt.Sprint(n)
	var b strings.Builder
	for _, r := range s {
		if id, ok := emojiDigits[r]; ok {
			b.WriteString(fmt.Sprintf(`<emoji id="%d">%c️⃣</emoji>`, id, r))
		} else {
			b.WriteRune(r)
			b.WriteString("️⃣")
		}
	}
	return b.String()
}

var (
	richEmojiRE     = regexp.MustCompile(`(?i)<tg-emoji\s+emoji-id="([^"]+)"\s*>(.*?)</tg-emoji>`)
	legacyEmojiRE   = regexp.MustCompile(`(?i)<emoji\s+id="([^"]+)"\s*>(.*?)</emoji>`)
	richOnlyTagRE   = regexp.MustCompile(`(?i)</?(?:h[1-6]|table|thead|tbody|tr|th|td|details|summary|mark|sub|sup|img|tg-button|button)\b[^>]*>`)
	blockBoundaryRE = regexp.MustCompile(`(?i)</(?:h[1-6]|tr|details|summary|blockquote|table|pre)>`)
	cellBoundaryRE  = regexp.MustCompile(`(?i)</(?:th|td)>`)
	brRE            = regexp.MustCompile(`(?i)<br\s*/?>\s*\n?`)
	pOpenRE         = regexp.MustCompile(`(?i)<(?:p|div)(?:\s[^>]*)?>\s*\n?`)
	pCloseRE        = regexp.MustCompile(`(?i)</(?:p|div)>\s*\n?`)
)

var unicodeToCustomEmoji = map[string]int64{
	"🎵": emojiMusicNote,
	"🎶": emojiMusicNotes,
	"🎧": emojiHeadphones,
	"🎤": emojiMic,
	"📢": emojiBroadcast,
	"🚀": emojiRocket,
	"▶️": emojiPlay,
	"▶":  emojiPlay,
	"▷":  emojiResume,
	"🔁": emojiLoop,
	"⚡️": emojiBolt,
	"⚡":  emojiBolt,
	"✅": emojiSuccess,
	"❌": emojiError,
	"⚠️": emojiWarning,
	"🚫": emojiBlocked,
	"🔐": emojiLock,
	"🔒": emojiLock,
	"🔓": emojiUnlock,
	"🛡":  emojiShield,
	"👑": emojiCrown,
	"👤": emojiUser,
	"👥": emojiUsers,
	"🔑": emojiKey,
	"🔥": emojiFire,
	"◀️": emojiBack,
	"◀":  emojiBack,
	"✖":  emojiClose,
	"✖️": emojiClose,
	"🏠": emojiHome,
	"🔄": emojiRefresh,
	"🔗": emojiLink,
	"➡️": emojiSkip,
	"➕": emojiAdd,
	"📌": emojiPin,
	"💬": emojiChat,
	"✉️": emojiSend,
	"✉":  emojiSend,
	"🌐": emojiGlobe,
	"🛠️": emojiTools,
	"🛠":  emojiTools,
	"⚙️": emojiSettings,
	"⚙":  emojiSettings,
	"ℹ️": emojiInfo,
	"ℹ":  emojiInfo,
	"📊": emojiStats,
	"🎞":  emojiPlay,
	"🔇": emojiPause,
	"🗃":  emojiQueueIcon,
	"📁": emojiQueueIcon,
	"⏭️": emojiSkip,
	"⏭":  emojiSkip,
	"⏸️": emojiPause,
	"⏸":  emojiPause,
	"⏹️": emojiStop,
	"⏹":  emojiStop,
	"0️⃣": 5771423202341303860,
	"1️⃣": 5771785311034021057,
	"2️⃣": 5773975314858251177,
	"3️⃣": 5771591496339820740,
	"4️⃣": 5771376816694498290,
	"5️⃣": 5771694571259958563,
	"6️⃣": 5773735118812221922,
	"7️⃣": 5773913385724809382,
	"8️⃣": 5773786959067484290,
	"9️⃣": 5774138390471512165,
	"0⃣":  5771423202341303860,
	"1⃣":  5771785311034021057,
	"2⃣":  5773975314858251177,
	"3⃣":  5771591496339820740,
	"4⃣":  5771376816694498290,
	"5⃣":  5771694571259958563,
	"6⃣":  5773735118812221922,
	"7⃣":  5773913385724809382,
	"8⃣":  5773786959067484290,
	"9⃣":  5774138390471512165,
	"📖": emojiInfo,
	"📋": emojiInfo,
	"🤖": emojiUser,
	"🎨": 5814690801665446789,
	"🎭": 5814690801665446789,
	"🎬": emojiRocket,
	"⬇️": emojiSkip,
	"⬇":  emojiSkip,
	"🔱": emojiCrown,
	"🆔": emojiUser,
	"📛": emojiUser,
	"📝": emojiChat,
	"📄": emojiChat,
	"🏷": emojiPin,
	"🔧": emojiTools,
	"🎚": emojiSettings,
	"📅": emojiStats,
	"🗑": emojiClose,
	"❓": emojiInfo,
	"👋": emojiUser,
	"📥": emojiSend,
	"🔍": emojiInfo,
	"⏱": emojiBolt,
	"💎": 5963312935148195483,
	"⭐️": 5807752501042089473,
	"⭐":  5807752501042089473,
	"🌟": 5989815447459991163,
}

var (
	sortedEmojiKeys []string
	upgradeEmojiRE  *regexp.Regexp
	noUpgradeRE     = regexp.MustCompile(`(?is)(<(?:tg-)?emoji\b[^>]*>.*?</(?:tg-)?emoji>|<code\b[^>]*>.*?</code>|<pre\b[^>]*>.*?</pre>)`)
	verbatimBlockRE = regexp.MustCompile(`(?is)(<(?:table|pre)\b[^>]*>.*?</(?:table|pre)>)`)
	unquotedHrefRE  = regexp.MustCompile(`href=([^\s">]+)`)
)

func init() {
	sortedEmojiKeys = make([]string, 0, len(unicodeToCustomEmoji))
	for k := range unicodeToCustomEmoji {
		sortedEmojiKeys = append(sortedEmojiKeys, k)
	}
	sort.Slice(sortedEmojiKeys, func(i, j int) bool {
		return len(sortedEmojiKeys[i]) > len(sortedEmojiKeys[j])
	})
	escaped := make([]string, len(sortedEmojiKeys))
	for i, k := range sortedEmojiKeys {
		escaped[i] = regexp.QuoteMeta(k)
	}
	upgradeEmojiRE = regexp.MustCompile(strings.Join(escaped, "|"))
}

var brSkipTags = []string{
	"<br/>", "<br>", "<p>", "</p>", "<div>", "</div>",
	"</h1>", "</h2>", "</h3>", "</h4>", "</h5>", "</h6>",
	"</blockquote>", "</summary>", "</details>",
	"</table>", "</pre>", "</li>", "</ul>", "</ol>",
	"<hr/>", "<hr>",
}

func convertNewlinesToBr(text string) string {
	if text == "" {
		return ""
	}
	var sb strings.Builder
	cursor := 0
	for {
		idx := strings.IndexByte(text[cursor:], '\n')
		if idx == -1 {
			sb.WriteString(text[cursor:])
			break
		}
		newlinePos := cursor + idx
		prefix := strings.TrimRight(text[:newlinePos], " \t")
		lower := strings.ToLower(prefix)
		skip := false
		for _, tag := range brSkipTags {
			if strings.HasSuffix(lower, tag) {
				skip = true
				break
			}
		}
		if skip {
			sb.WriteString(text[cursor : newlinePos+1])
		} else {
			sb.WriteString(text[cursor:newlinePos])
			sb.WriteString("<br/>\n")
		}
		cursor = newlinePos + 1
	}
	return sb.String()
}

// normalizeRichHTML converts line breaks outside table and pre tags to <br/>\n
// so Telegram's Rich Message compiler preserves paragraph and line formatting.
func normalizeRichHTML(text string) string {
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = unquotedHrefRE.ReplaceAllString(text, `href="$1"`)

	matches := verbatimBlockRE.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return convertNewlinesToBr(text)
	}

	var sb strings.Builder
	last := 0
	for _, match := range matches {
		if match[0] > last {
			chunk := text[last:match[0]]
			if last > 0 && strings.HasPrefix(chunk, "\n") {
				leadingNL := 0
				for leadingNL < len(chunk) && chunk[leadingNL] == '\n' {
					leadingNL++
				}
				sb.WriteString(chunk[:leadingNL])
				sb.WriteString(convertNewlinesToBr(chunk[leadingNL:]))
			} else {
				sb.WriteString(convertNewlinesToBr(chunk))
			}
		}
		sb.WriteString(text[match[0]:match[1]])
		last = match[1]
	}
	if last < len(text) {
		chunk := text[last:]
		if last > 0 && strings.HasPrefix(chunk, "\n") {
			leadingNL := 0
			for leadingNL < len(chunk) && chunk[leadingNL] == '\n' {
				leadingNL++
			}
			sb.WriteString(chunk[:leadingNL])
			sb.WriteString(convertNewlinesToBr(chunk[leadingNL:]))
		} else {
			sb.WriteString(convertNewlinesToBr(chunk))
		}
	}
	return sb.String()
}

// upgradeUnicodeEmoji scans text and upgrades plain unicode emojis to
// native <tg-emoji emoji-id="..."> tags, mirroring utils/premium_emoji.py
// from nub-music-bot.
func upgradeUnicodeEmoji(text string) string {
	if text == "" {
		return ""
	}
	matches := noUpgradeRE.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return upgradeEmojiRE.ReplaceAllStringFunc(text, func(m string) string {
			if id, ok := unicodeToCustomEmoji[m]; ok {
				return fmt.Sprintf(`<tg-emoji emoji-id="%d">%s</tg-emoji>`, id, m)
			}
			return m
		})
	}
	var sb strings.Builder
	last := 0
	for _, match := range matches {
		if match[0] > last {
			chunk := text[last:match[0]]
			upgraded := upgradeEmojiRE.ReplaceAllStringFunc(chunk, func(m string) string {
				if id, ok := unicodeToCustomEmoji[m]; ok {
					return fmt.Sprintf(`<tg-emoji emoji-id="%d">%s</tg-emoji>`, id, m)
				}
				return m
			})
			sb.WriteString(upgraded)
		}
		sb.WriteString(text[match[0]:match[1]])
		last = match[1]
	}
	if last < len(text) {
		chunk := text[last:]
		upgraded := upgradeEmojiRE.ReplaceAllStringFunc(chunk, func(m string) string {
			if id, ok := unicodeToCustomEmoji[m]; ok {
				return fmt.Sprintf(`<tg-emoji emoji-id="%d">%s</tg-emoji>`, id, m)
			}
			return m
		})
		sb.WriteString(upgraded)
	}
	return sb.String()
}

func emoji(id int64, glyph string) string {
	return fmt.Sprintf(`<emoji id="%d">%s</emoji>`, id, glyph)
}

// richHTML upgrades the legacy parser spelling to the Bot API rich-message
// spelling, automatically upgrades plain unicode emojis to custom emoji tags,
// and normalizes newlines to <br/>\n to avoid Telegram collapsing lines.
func richHTML(body string) string {
	body = richEmojiRE.ReplaceAllString(body, `<tg-emoji emoji-id="$1">$2</tg-emoji>`)
	body = legacyEmojiRE.ReplaceAllString(body, `<tg-emoji emoji-id="$1">$2</tg-emoji>`)
	body = upgradeUnicodeEmoji(body)
	return normalizeRichHTML(body)
}

// normalHTML keeps the tags supported by Gogram's ordinary HTML parser and
// flattens rich-only blocks so a failed rich send still has a useful fallback.
func normalHTML(body string) string {
	body = richEmojiRE.ReplaceAllString(body, `<emoji id="$1">$2</emoji>`)
	body = legacyEmojiRE.ReplaceAllString(body, `<emoji id="$1">$2</emoji>`)
	body = pOpenRE.ReplaceAllString(body, "")
	body = pCloseRE.ReplaceAllString(body, "\n")
	body = brRE.ReplaceAllString(body, "\n")
	body = regexp.MustCompile(`(?i)<h[1-6][^>]*>`).ReplaceAllString(body, "<b>")
	body = regexp.MustCompile(`(?i)</h[1-6]>`).ReplaceAllString(body, "</b>\n")
	body = regexp.MustCompile(`(?i)<details[^>]*>`).ReplaceAllString(body, "\n")
	body = regexp.MustCompile(`(?i)</details>`).ReplaceAllString(body, "\n")
	body = regexp.MustCompile(`(?i)<summary[^>]*>`).ReplaceAllString(body, "<b>")
	body = regexp.MustCompile(`(?i)</summary>`).ReplaceAllString(body, "</b>\n")
	body = regexp.MustCompile(`(?i)<table[^>]*>`).ReplaceAllString(body, "\n")
	body = regexp.MustCompile(`(?i)</table>`).ReplaceAllString(body, "\n")
	body = regexp.MustCompile(`(?i)<tr[^>]*>`).ReplaceAllString(body, "\n")
	body = regexp.MustCompile(`(?i)</tr>`).ReplaceAllString(body, "\n")
	body = regexp.MustCompile(`(?i)<th[^>]*>`).ReplaceAllString(body, "<b>")
	body = regexp.MustCompile(`(?i)</th>`).ReplaceAllString(body, "</b> • ")
	body = regexp.MustCompile(`(?i)<td[^>]*>`).ReplaceAllString(body, "")
	body = cellBoundaryRE.ReplaceAllString(body, " • ")
	body = regexp.MustCompile(`(?i)<mark[^>]*>`).ReplaceAllString(body, "<b>")
	body = regexp.MustCompile(`(?i)</mark>`).ReplaceAllString(body, "</b>")
	body = richOnlyTagRE.ReplaceAllString(body, "")
	body = regexp.MustCompile(`\n{3,}`).ReplaceAllString(body, "\n\n")
	return strings.TrimSpace(body)
}

func richMessage(body string) *telegram.RichBuilder {
	return telegram.NewRichMessage().HTML(richHTML(body))
}

func sendOption(opts ...*telegram.SendOptions) *telegram.SendOptions {
	if len(opts) > 0 && opts[0] != nil {
		return opts[0]
	}
	return htmlOptions()
}

func replyRich(m *telegram.NewMessage, body string, opts ...*telegram.SendOptions) (*telegram.NewMessage, error) {
	option := sendOption(opts...)
	if m == nil {
		return nil, fmt.Errorf("message is nil")
	}
	if sent, err := m.ReplyRich(richMessage(body), option); err == nil {
		return sent, nil
	}
	return m.Reply(normalHTML(body), option)
}

func sendRich(client *telegram.Client, peer any, body string, opts ...*telegram.SendOptions) (*telegram.NewMessage, error) {
	option := sendOption(opts...)
	if client == nil {
		return nil, fmt.Errorf("telegram client is nil")
	}
	if sent, err := client.SendRich(peer, richMessage(body), option); err == nil {
		return sent, nil
	}
	return client.SendMessage(peer, normalHTML(body), option)
}

func editRich(message *telegram.NewMessage, body string, opts ...*telegram.SendOptions) (*telegram.NewMessage, error) {
	option := sendOption(opts...)
	if message == nil || message.Client == nil {
		return nil, fmt.Errorf("message client is unavailable")
	}
	if edited, err := message.EditRich(richMessage(body), option); err == nil {
		return edited, nil
	}
	return message.Client.EditMessage(message.ChannelID(), message.ID, normalHTML(body), option)
}

func editRichPeer(client *telegram.Client, peer any, messageID int32, body string, opts ...*telegram.SendOptions) (*telegram.NewMessage, error) {
	option := sendOption(opts...)
	if client == nil {
		return nil, fmt.Errorf("telegram client is nil")
	}
	if edited, err := client.EditRich(peer, messageID, richMessage(body), option); err == nil {
		return edited, nil
	}
	return client.EditMessage(peer, messageID, normalHTML(body), option)
}

// mediaOptions builds the options for a photo/video card. Rich markup cannot be
// attached as a caption, so the body is flattened to the ordinary HTML parser.
func mediaOptions(body string, markup telegram.ReplyMarkup) *telegram.MediaOptions {
	return &telegram.MediaOptions{
		Caption:     normalHTML(body),
		ParseMode:   "html",
		ReplyMarkup: markup,
	}
}

// sendMediaCard posts mediaPath with the card body as its caption. Gogram picks
// photo vs. video from the file itself, so the same call serves both.
func sendMediaCard(client *telegram.Client, peer any, mediaPath, body string, markup telegram.ReplyMarkup) (*telegram.NewMessage, error) {
	if client == nil {
		return nil, fmt.Errorf("telegram client is nil")
	}
	return client.SendMedia(peer, mediaPath, mediaOptions(body, markup))
}

// replyMediaCard is sendMediaCard as a reply to m.
func replyMediaCard(m *telegram.NewMessage, mediaPath, body string, markup telegram.ReplyMarkup) (*telegram.NewMessage, error) {
	if m == nil {
		return nil, fmt.Errorf("message is nil")
	}
	return m.ReplyMedia(mediaPath, mediaOptions(body, markup))
}

// editCard updates a card in place, whether it was sent as a rich text message
// or as a photo/video. Caption-bound cards keep their media: an edit without
// the original media would strip the photo, so it is passed back through.
func editCard(client *telegram.Client, msg *telegram.NewMessage, body string, opts ...*telegram.SendOptions) (*telegram.NewMessage, error) {
	if msg == nil || client == nil {
		return nil, fmt.Errorf("message client is unavailable")
	}
	if !msg.IsMedia() {
		return editRichPeer(client, msg.ChannelID(), msg.ID, body, opts...)
	}
	option := *sendOption(opts...)
	option.ParseMode = "html"
	option.Entities = nil
	option.Media = msg.Media()
	return client.EditMessage(msg.ChannelID(), msg.ID, normalHTML(body), &option)
}

func escape(value any) string {
	return html.EscapeString(fmt.Sprint(value))
}
