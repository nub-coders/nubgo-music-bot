package telegrambot

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

const (
	// welcomeLogoMaxBytes is the largest media accepted as a custom logo.
	welcomeLogoMaxBytes = 5 * 1024 * 1024
	// welcomeTextMaxChars is Telegram's caption/text ceiling.
	welcomeTextMaxChars = 4096
)

var placeholderRE = regexp.MustCompile(`\{([^{}]+)\}`)

var allowedPlaceholders = map[string]bool{
	"{name}":    true,
	"{id}":      true,
	"{botname}": true,
}

// -- logo resolution ---------------------------------------------------------

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (h *Handlers) logoPath(ext string) string {
	return filepath.Join(h.logoDir, "logo"+ext)
}

// welcomeLogo resolves the image used for welcome and start cards, in the same
// order as the Python bot: a user-uploaded logo.mp4, then logo.jpg, then the
// bot's own profile photo (cached to disk), then the bundled fallback asset.
func (h *Handlers) welcomeLogo() string {
	if path := h.logoPath(".mp4"); fileExists(path) {
		return path
	}
	jpg := h.logoPath(".jpg")
	if fileExists(jpg) {
		return jpg
	}
	if h.bot != nil {
		if photos, err := h.bot.GetProfilePhotos("me"); err == nil && len(photos) > 0 {
			target := jpg
			if path, err := h.bot.DownloadMedia(photos[0].Photo, &telegram.DownloadOptions{FileName: target}); err == nil && path != "" {
				return path
			}
		}
	}
	if fileExists(h.fallbackLogo) {
		return h.fallbackLogo
	}
	return ""
}

// startGreeting returns the stored (already-HTML) start message, or "" to use
// the built-in card.
func (h *Handlers) startGreeting(ctx context.Context) string {
	text, err := h.store.GetWelcome(ctx, h.botID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(text)
}

func (h *Handlers) botMention() string {
	if botUser := h.bot.Me(); botUser != nil {
		return userMention(botUser)
	}
	return "ɴᴜʙ ᴍᴜsɪᴄ ʙᴏᴛ"
}

func (h *Handlers) botUsername() string {
	if botUser := h.bot.Me(); botUser != nil {
		return botUser.Username
	}
	return ""
}

// sendGroupWelcome delivers the thank-you card to a chat the bot was added to,
// with the markup, falling back to a link-preview-free text reply when the
// photo cannot be posted (no media rights, missing asset, …).
func (h *Handlers) sendGroupWelcome(chatID int64, groupName string, adder *telegram.UserObj) {
	if chatID == 0 {
		return
	}
	caption := groupWelcome(userMention(adder), groupName, h.botMention())
	markup := groupWelcomeButtons(h.botUsername(), h.supportGroup)

	if logo := h.welcomeLogo(); logo != "" {
		if _, err := sendMediaCard(h.bot, chatID, logo, caption, markup); err == nil {
			return
		} else {
			h.logger.Info("group welcome photo failed, falling back to text", "chat_id", chatID, "error", err)
		}
	}
	_, err := h.bot.SendMessage(chatID, normalHTML(caption), &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	if err != nil {
		h.logger.Warn("group welcome text fallback failed", "chat_id", chatID, "error", err)
	}
}

// onParticipant handles supergroup/channel membership updates.
func (h *Handlers) onParticipant(update *telegram.ParticipantUpdate) error {
	if update == nil || update.Channel == nil {
		return nil
	}
	if update.UserID() != h.botID {
		return nil
	}
	if !update.IsAdded() && !update.IsJoined() {
		return nil
	}
	h.sendGroupWelcome(update.ChannelID(), channelTitle(update.Channel), update.Actor)
	return nil
}

// onChatParticipant handles basic-group membership updates.
func (h *Handlers) onChatParticipant(update *telegram.ChatParticipantUpdate) error {
	if update == nil {
		return nil
	}
	if update.UserID != h.botID || !update.Joined() {
		return nil
	}
	title := "ᴛʜɪs ɢʀᴏᴜᴘ"
	if update.Chat != nil && strings.TrimSpace(update.Chat.Title) != "" {
		title = update.Chat.Title
	}
	// Basic-group chat IDs are positive in MTProto; the sendable peer is the
	// negated id (supergroups use the -100… form handled by ChannelID).
	h.sendGroupWelcome(-update.ChatID, title, update.Actor)
	return nil
}

func channelTitle(channel *telegram.Channel) string {
	if channel != nil && strings.TrimSpace(channel.Title) != "" {
		return channel.Title
	}
	return "ᴛʜɪs ɢʀᴏᴜᴘ"
}

// -- /setwelcome, /resetwelcome ----------------------------------------------

func (h *Handlers) isOwner(userID int64) bool {
	return h.ownerID > 0 && userID == h.ownerID
}

func (h *Handlers) ownerOnly(m *telegram.NewMessage) bool {
	if h.isOwner(m.SenderID()) {
		return true
	}
	_, _ = replyRich(m, richNote(emoji(emojiCrown, "👑")+" <b>ᴛʜɪs ᴄᴏᴍᴍᴀɴᴅ ɪs ᴀᴠᴀɪʟᴀʙʟᴇ ᴛᴏ ʙᴏᴛ ᴏᴡɴᴇʀ ᴏɴʟʏ.</b>"), htmlOptions())
	return false
}

func setWelcomeUsage() string {
	return richHeading(emoji(emojiInfo, "ℹ️")+" sᴇᴛ ᴡᴇʟᴄᴏᴍᴇ ᴍᴇssᴀɢᴇ", 1) +
		richNote("<b>ʀᴇᴘʟʏ ᴛᴏ ᴀ ᴍᴇssᴀɢᴇ ᴛᴏ sᴇᴛ ɪᴛ ᴀs ᴛʜᴇ ᴡᴇʟᴄᴏᴍᴇ ᴍᴇssᴀɢᴇ.</b>") +
		richTable([]string{"ᴘʟᴀᴄᴇʜᴏʟᴅᴇʀ", "ʀᴇᴘʟᴀᴄᴇᴅ ᴡɪᴛʜ"}, [][]string{
			{richCode("{name}"), "ᴜsᴇʀ's ɴᴀᴍᴇ"},
			{richCode("{id}"), "ᴜsᴇʀ's ɪᴅ"},
			{richCode("{botname}"), "ʙᴏᴛ's ᴜsᴇʀɴᴀᴍᴇ"},
		}) +
		richDetails(emoji(emojiHelp, "ℹ️")+" sᴜᴘᴘᴏʀᴛᴇᴅ ᴛʏᴘᴇs &amp; ʟɪᴍɪᴛs", richTable(
			[]string{"ᴛʏᴘᴇ", "ʟɪᴍɪᴛ"}, [][]string{
				{"ᴛᴇxᴛ ᴍᴇssᴀɢᴇ", richCode("4096 ᴄʜᴀʀs")},
				{"ᴘʜᴏᴛᴏ / ᴠɪᴅᴇᴏ / ɢɪꜰ / sᴛɪᴄᴋᴇʀ", richCode("5 ᴍʙ")},
			}), false)
}

func (h *Handlers) setWelcome(m *telegram.NewMessage) error {
	if !h.ownerOnly(m) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	replied, err := m.GetReplyMessage()
	if err != nil || replied == nil {
		_, _ = replyRich(m, setWelcomeUsage(), htmlOptions())
		return nil
	}

	var updated []string

	raw := strings.TrimSpace(replied.MessageText())
	if raw != "" {
		if len([]rune(raw)) > welcomeTextMaxChars {
			_, _ = replyRich(m, richNote(emoji(emojiWarning, "⚠️")+" <b>ᴡᴇʟᴄᴏᴍᴇ ᴍᴇssᴀɢᴇ ɪs ᴛᴏᴏ ʟᴏɴɢ. ᴍᴀx 4096 ᴄʜᴀʀᴀᴄᴛᴇʀs.</b>"), htmlOptions())
			return nil
		}
		processed := entitiesToHTML(raw, replied.Message.Entities)
		if invalid := invalidPlaceholders(processed); len(invalid) > 0 {
			rows := make([][]string, 0, len(invalid))
			for _, p := range invalid {
				rows = append(rows, []string{richCode(p), emoji(emojiError, "❌") + " ɴᴏᴛ ᴀʟʟᴏᴡᴇᴅ"})
			}
			body := richHeading(emoji(emojiError, "❌")+" ɪɴᴠᴀʟɪᴅ ᴘʟᴀᴄᴇʜᴏʟᴅᴇʀs", 1) +
				richTable([]string{"ꜰᴏᴜɴᴅ", "sᴛᴀᴛᴜs"}, rows) +
				richDetails(emoji(emojiInfo, "ℹ️")+" ᴀʟʟᴏᴡᴇᴅ ᴘʟᴀᴄᴇʜᴏʟᴅᴇʀs", richTable(
					[]string{"ᴘʟᴀᴄᴇʜᴏʟᴅᴇʀ", "ᴇxᴀᴍᴘʟᴇ"}, [][]string{
						{richCode("{name}"), richCode("ᴡᴇʟᴄᴏᴍᴇ {name}!")},
						{richCode("{id}"), richCode("ʏᴏᴜʀ ɪᴅ: {id}")},
						{richCode("{botname}"), richCode("ᴡᴇʟᴄᴏᴍᴇ ᴛᴏ {botname}!")},
					}), false)
			_, _ = replyRich(m, body, htmlOptions())
			return nil
		}
		if err := h.store.SetWelcome(ctx, h.botID, processed); err != nil {
			_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
			return nil
		}
		updated = append(updated, "ᴡᴇʟᴄᴏᴍᴇ ᴍᴇssᴀɢᴇ")
	}

	if replied.IsMedia() {
		saved, err := h.saveWelcomeLogo(replied)
		if err != nil {
			_, _ = replyRich(m, richNote(emoji(emojiError, "❌")+" <b>"+html.EscapeString(err.Error())+"</b>"), htmlOptions())
			return nil
		}
		if saved != "" {
			updated = append(updated, "ʟᴏɢᴏ")
		}
	}

	if len(updated) == 0 {
		_, _ = replyRich(m, richNote(emoji(emojiInfo, "ℹ️")+" <b>ɴᴏᴛʜɪɴɢ ᴛᴏ ᴜᴘᴅᴀᴛᴇ.</b>"), htmlOptions())
		return nil
	}

	rows := make([][]string, 0, len(updated))
	for _, item := range updated {
		rows = append(rows, []string{item})
	}
	_, _ = replyRich(m,
		richHeading(emoji(emojiSuccess, "✅")+" ᴜᴘᴅᴀᴛᴇᴅ", 2)+
			richTable([]string{"ᴜᴘᴅᴀᴛᴇᴅ"}, rows)+
			richNote(emoji(emojiInfo, "ℹ️")+" <b>ᴘʀᴇᴠɪᴇᴡ ʙᴇʟᴏᴡ</b>"),
		htmlOptions())

	h.sendWelcomePreview(m)
	return nil
}

// sendWelcomePreview posts the current welcome card to the chat so the owner
// can see exactly what a new user will get.
func (h *Handlers) sendWelcomePreview(m *telegram.NewMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	greeting := h.startGreeting(ctx)
	if greeting == "" {
		greeting = startCard("", "there")
	}
	body := formatWelcome(greeting, userMention(senderOrNil(m)), m.ChannelID(), h.botMention())

	if logo := h.welcomeLogo(); logo != "" {
		if _, err := sendMediaCard(h.bot, m.ChannelID(), logo, body, nil); err == nil {
			return
		} else {
			h.logger.Info("welcome preview photo failed", "error", err)
		}
	}
	_, _ = sendRich(h.bot, m.ChannelID(), body)
}

// saveWelcomeLogo validates and stores the replied message's media as the
// custom logo. It returns the saved path, or "" when nothing was stored.
func (h *Handlers) saveWelcomeLogo(replied *telegram.NewMessage) (string, error) {
	if replied.Photo() == nil && replied.Video() == nil && replied.Sticker() == nil && replied.Animation() == nil {
		return "", fmt.Errorf("ᴏɴʟʏ ᴘʜᴏᴛᴏs, ᴠɪᴅᴇᴏs, ɢɪꜰs, ᴀɴᴅ sᴛɪᴄᴋᴇʀs ᴀʀᴇ ᴀʟʟᴏᴡᴇᴅ.")
	}
	if size := messageFileSize(replied); size > welcomeLogoMaxBytes {
		return "", fmt.Errorf("ᴍᴇᴅɪᴀ sɪᴢᴇ ᴍᴜsᴛ ʙᴇ ʙᴇʟᴏᴡ 5 ᴍʙ.")
	}
	if err := os.MkdirAll(h.logoDir, 0o750); err != nil {
		return "", fmt.Errorf("ᴇʀʀᴏʀ ᴘʀᴏᴄᴇssɪɴɢ ᴍᴇᴅɪᴀ. ᴘʟᴇᴀꜱᴇ ᴛʀʏ ᴀ ᴅɪꜰꜰᴇʀᴇɴᴛ ꜰɪʟᴇ.")
	}

	target := h.logoPath(".jpg")
	if replied.Video() != nil || replied.Animation() != nil {
		target = h.logoPath(".mp4")
	}

	path, err := replied.Download(&telegram.DownloadOptions{FileName: target})
	if err != nil || path == "" {
		h.logger.Error("welcome logo download failed", "error", err)
		return "", fmt.Errorf("ᴇʀʀᴏʀ ᴘʀᴏᴄᴇssɪɴɢ ᴍᴇᴅɪᴀ. ᴘʟᴇᴀꜱᴇ ᴛʀʏ ᴀ ᴅɪꜰꜰᴇʀᴇɴᴛ ꜰɪʟᴇ.")
	}
	if path != target {
		if err := os.Rename(path, target); err != nil {
			h.logger.Error("welcome logo move failed", "from", path, "to", target, "error", err)
			return "", fmt.Errorf("ᴇʀʀᴏʀ ᴘʀᴏᴄᴇssɪɴɢ ᴍᴇᴅɪᴀ. ᴘʟᴇᴀꜱᴇ ᴛʀʏ ᴀ ᴅɪꜰꜰᴇʀᴇɴᴛ ꜰɪʟᴇ.")
		}
	}
	// Drop the counterpart so resolution stays deterministic.
	if strings.HasSuffix(target, ".mp4") {
		_ = os.Remove(h.logoPath(".jpg"))
	} else {
		_ = os.Remove(h.logoPath(".mp4"))
	}
	return target, nil
}

func (h *Handlers) resetWelcome(m *telegram.NewMessage) error {
	if !h.ownerOnly(m) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.store.SetWelcome(ctx, h.botID, ""); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_ = os.Remove(h.logoPath(".jpg"))
	_ = os.Remove(h.logoPath(".mp4"))
	_, _ = replyRich(m, richNote(emoji(emojiSuccess, "✅")+" <b>ᴡᴇʟᴄᴏᴍᴇ ᴍᴇssᴀɢᴇ ᴀɴᴅ ʟᴏɢᴏ ʜᴀᴠᴇ ʙᴇᴇɴ ʀᴇsᴇᴛ.</b>"), htmlOptions())
	return nil
}

// -- helpers -----------------------------------------------------------------

func senderOrNil(m *telegram.NewMessage) *telegram.UserObj {
	if m == nil {
		return nil
	}
	sender, err := m.GetSender()
	if err != nil {
		return nil
	}
	return sender
}

func messageFileSize(m *telegram.NewMessage) int64 {
	if m == nil {
		return 0
	}
	if doc := m.Document(); doc != nil {
		return doc.Size
	}
	if photo := m.Photo(); photo != nil {
		var size int64
		for _, s := range photo.Sizes {
			if ps, ok := s.(*telegram.PhotoSizeObj); ok && int64(ps.Size) > size {
				size = int64(ps.Size)
			}
		}
		return size
	}
	return 0
}

func invalidPlaceholders(text string) []string {
	seen := map[string]bool{}
	var invalid []string
	for _, match := range placeholderRE.FindAllStringSubmatch(text, -1) {
		full := "{" + match[1] + "}"
		if allowedPlaceholders[full] || seen[full] {
			continue
		}
		seen[full] = true
		invalid = append(invalid, full)
	}
	sort.Strings(invalid)
	return invalid
}

type htmlTag struct {
	pos     int
	tag     string
	opening bool
}

// entitiesToHTML converts Telegram message entities into the HTML subset the
// bot's parser understands, preserving bold/italic/underline/strike/spoiler/
// code/pre/blockquote formatting. Offsets are UTF-16 code units.
func entitiesToHTML(text string, entities []telegram.MessageEntity) string {
	var tags []htmlTag
	add := func(offset, length int32, open, close string) {
		if length <= 0 {
			return
		}
		tags = append(tags,
			htmlTag{pos: byteOffsetForUTF16(text, int(offset)), tag: open, opening: true},
			htmlTag{pos: byteOffsetForUTF16(text, int(offset+length)), tag: close, opening: false},
		)
	}

	for _, entity := range entities {
		switch e := entity.(type) {
		case *telegram.MessageEntityBold:
			add(e.Offset, e.Length, "<b>", "</b>")
		case *telegram.MessageEntityItalic:
			add(e.Offset, e.Length, "<i>", "</i>")
		case *telegram.MessageEntityUnderline:
			add(e.Offset, e.Length, "<u>", "</u>")
		case *telegram.MessageEntityStrike:
			add(e.Offset, e.Length, "<s>", "</s>")
		case *telegram.MessageEntitySpoiler:
			add(e.Offset, e.Length, "<spoiler>", "</spoiler>")
		case *telegram.MessageEntityCode:
			add(e.Offset, e.Length, "<code>", "</code>")
		case *telegram.MessageEntityBlockquote:
			add(e.Offset, e.Length, "<blockquote>", "</blockquote>")
		case *telegram.MessageEntityPre:
			open := "<pre>"
			if e.Language != "" {
				open = `<pre language="` + html.EscapeString(e.Language) + `">`
			}
			add(e.Offset, e.Length, open, "</pre>")
		}
	}

	sort.SliceStable(tags, func(i, j int) bool {
		if tags[i].pos != tags[j].pos {
			return tags[i].pos < tags[j].pos
		}
		// Close before opening at the same boundary, so adjacent entity spans
		// nest cleanly.
		return !tags[i].opening && tags[j].opening
	})

	var builder strings.Builder
	cursor := 0
	for _, tag := range tags {
		if tag.pos > cursor {
			builder.WriteString(html.EscapeString(text[cursor:tag.pos]))
			cursor = tag.pos
		}
		builder.WriteString(tag.tag)
	}
	if cursor < len(text) {
		builder.WriteString(html.EscapeString(text[cursor:]))
	}
	return builder.String()
}

// byteOffsetForUTF16 maps a UTF-16 code-unit offset to a byte offset in s.
func byteOffsetForUTF16(s string, u16 int) int {
	if u16 <= 0 {
		return 0
	}
	count := 0
	for index, r := range s {
		if count >= u16 {
			return index
		}
		if r > 0xFFFF {
			count += 2
		} else {
			count++
		}
	}
	return len(s)
}
