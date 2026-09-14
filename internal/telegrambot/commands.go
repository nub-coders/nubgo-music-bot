package telegrambot

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/nub-coders/gogram/telegram"
	"github.com/nub-coders/nub-go-music-bot/internal/media"
	"github.com/nub-coders/nub-go-music-bot/internal/storage"
)

// -- /start, /help, /ping, /about -------------------------------------------

func (h *Handlers) start(m *telegram.NewMessage) error {
	username := h.botUsername()
	args := strings.TrimSpace(m.Args())
	if strings.EqualFold(args, "help") {
		return h.help(m)
	}
	if strings.HasPrefix(args, "newpl_") {
		text := fmt.Sprintf("%s <b>ᴄʀᴇᴀᴛᴇ ᴀ ɴᴇᴡ ᴘʟᴀʏʟɪsᴛ</b>\n\n%s <b>To create a playlist, send:</b>\n<code>/playlist new &lt;name&gt;</code>\n\n<b>‣ Rules:</b>\n• <i>Maximum 10 characters</i>\n• <i>Letters &amp; numbers only</i>\n• <i>No spaces, symbols, or special characters</i>\n• <i>Maximum 5 playlists total</i>",
			emoji(emojiAdd, "➕"),
			emoji(emojiInfo, "💡"),
		)
		_, err := replyRich(m, text, &telegram.SendOptions{})
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sender := senderOrNil(m)
	name := "there"
	if sender != nil && strings.TrimSpace(sender.FirstName) != "" {
		name = strings.TrimSpace(sender.FirstName)
	}

	greeting := h.startGreeting(ctx)
	if greeting == "" {
		greeting = startCard(username, name)
	}
	body := formatWelcome(greeting, userMention(sender), m.ChannelID(), h.botMention())
	markup := startButtons(username, h.ownerID, h.supportGroup)

	if logo := h.welcomeLogo(); logo != "" {
		if _, err := replyMediaCard(m, logo, body, markup); err == nil {
			return nil
		}
		h.logger.Info("start photo failed, falling back to rich text", "chat_id", m.ChannelID())
	}
	_, err := replyRich(m, body, &telegram.SendOptions{ReplyMarkup: markup})
	return err
}

func (h *Handlers) help(m *telegram.NewMessage) error {
	showAdmin := h.ownerID > 0 && m.SenderID() == h.ownerID
	if sudo, err := h.store.IsSudo(context.Background(), h.botID, m.SenderID()); err == nil && sudo {
		showAdmin = true
	}
	body := helpCategorySelect(showAdmin)
	_, err := replyRich(m, body, &telegram.SendOptions{ReplyMarkup: helpButtons(showAdmin)})
	return err
}

// commandsCallback routes the help-category selector buttons. Each category is
// rendered as a flat block and, when the card lives on a photo, edited as its
// caption so the start-card photo is preserved.
func (h *Handlers) commandsCallback(callback *telegram.CallbackQuery) error {
	data := callback.DataString()
	if !strings.HasPrefix(data, "commands_") {
		return nil
	}
	category := strings.TrimPrefix(data, "commands_")
	showAdmin := h.ownerID > 0 && callback.GetSenderID() == h.ownerID
	if sudo, err := h.store.IsSudo(context.Background(), h.botID, callback.GetSenderID()); err == nil && sudo {
		showAdmin = true
	}

	var (
		body   string
		markup *telegram.ReplyInlineMarkup
	)
	if category == "home" || category == "back" {
		sender, _ := callback.GetSender()
		name := "there"
		if sender != nil && strings.TrimSpace(sender.FirstName) != "" {
			name = strings.TrimSpace(sender.FirstName)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		greeting := h.startGreeting(ctx)
		cancel()
		if greeting == "" {
			greeting = startCard(h.botUsername(), name)
		}
		body = formatWelcome(greeting, userMention(sender), callback.ChannelID(), h.botMention())
		markup = startButtons(h.botUsername(), h.ownerID, h.supportGroup)
	} else {
		body, markup = helpCategoryPage(category, showAdmin)
	}
	if body == "" {
		_, _ = callback.Answer("Unknown category.", &telegram.CallbackOptions{Alert: true})
		return nil
	}
	_, _ = callback.Answer("")
	msg, err := callback.GetMessage()
	if err != nil || msg == nil {
		return nil
	}
	if _, err := editCard(callback.Client, msg, body, &telegram.SendOptions{ReplyMarkup: markup}); err != nil {
		_, _ = callback.Answer("Failed to update help page.", &telegram.CallbackOptions{Alert: true})
	}
	return nil
}

func (h *Handlers) ping(m *telegram.NewMessage) error {
	elapsed := time.Since(h.started)
	_, err := m.Reply(fmt.Sprintf("🏓 <b>Pong!</b>\nUptime: <code>%s</code>", formatDuration(elapsed)), htmlOptions())
	return err
}

func (h *Handlers) about(m *telegram.NewMessage) error {
	_, err := m.Reply(
		"🤖 <b>NUB Music Bot (Go)</b>\n"+
			"MTProto voice-chat music bot written in Go with gogram + NTgCalls.\n\n"+
			"Resolvers: InnerTube → NUB API → YouTube Data API → yt-dlp.\n"+
			"Sources: YouTube, Spotify (optional), direct streams.", htmlOptions())
	return err
}

// -- lists -------------------------------------------------------------------

func (h *Handlers) requirePrivileged(ctx context.Context, m *telegram.NewMessage) bool {
	if h.ownerID > 0 && m.SenderID() == h.ownerID {
		return true
	}
	sudo, err := h.store.IsSudo(ctx, h.botID, m.SenderID())
	if err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return false
	}
	if !sudo {
		_, _ = m.Reply("⛔ Only the owner or sudo users can do that.", htmlOptions())
		return false
	}
	return true
}

func (h *Handlers) sudoList(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ids, err := h.store.ListSudo(ctx, h.botID)
	if err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply("🛡️ <b>Sudo users</b>\n"+formatIDList(ids), htmlOptions())
	return nil
}

func (h *Handlers) authList(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	allowed, err := h.auth.CanManage(ctx, m.ChannelID(), m.SenderID())
	if err != nil || !allowed {
		_, _ = m.Reply("⛔ Only chat administrators, sudo users, or the owner can do that.", htmlOptions())
		return nil
	}
	ids, err := h.store.ListAuthorized(ctx, h.botID, m.ChannelID())
	if err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply("🎧 <b>Authorized users</b>\n"+formatIDList(ids), htmlOptions())
	return nil
}

func (h *Handlers) blockList(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ids, err := h.store.ListBlocked(ctx, h.botID)
	if err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply("🚫 <b>Blocked users</b>\n"+formatIDList(ids), htmlOptions())
	return nil
}

func formatIDList(ids []int64) string {
	if len(ids) == 0 {
		return "<i>None.</i>"
	}
	var builder strings.Builder
	for index, id := range ids {
		if index >= 30 {
			builder.WriteString(fmt.Sprintf("\n…and %d more", len(ids)-index))
			break
		}
		builder.WriteString(fmt.Sprintf("%d. <code>%d</code>\n", index+1, id))
	}
	return strings.TrimRight(builder.String(), "\n")
}

// -- block / unblock ---------------------------------------------------------

func (h *Handlers) block(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if !h.requirePrivileged(ctx, m) {
		return nil
	}
	target := targetUserID(m)
	if target <= 0 {
		_, _ = m.Reply("Reply to a user or provide a numeric user ID.", htmlOptions())
		return nil
	}
	if err := h.store.AddBlocked(ctx, h.botID, target); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply(fmt.Sprintf("🚫 User <code>%d</code> blocked from using the bot.", target), htmlOptions())
	return nil
}

func (h *Handlers) unblock(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if !h.requirePrivileged(ctx, m) {
		return nil
	}
	target := targetUserID(m)
	if target <= 0 {
		_, _ = m.Reply("Reply to a user or provide a numeric user ID.", htmlOptions())
		return nil
	}
	if err := h.store.RemoveBlocked(ctx, h.botID, target); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply(fmt.Sprintf("✅ User <code>%d</code> unblocked.", target), htmlOptions())
	return nil
}

// -- broadcast ---------------------------------------------------------------

func (h *Handlers) broadcast(m *telegram.NewMessage) error {
	return h.doBroadcast(m, false)
}
func (h *Handlers) broadcastForce(m *telegram.NewMessage) error {
	return h.doBroadcast(m, true)
}

func (h *Handlers) doBroadcast(m *telegram.NewMessage, force bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if !h.requirePrivileged(ctx, m) {
		return nil
	}
	text := strings.TrimSpace(m.Args())
	if text == "" {
		_, _ = m.Reply("Usage: <code>/broadcast message text</code>", htmlOptions())
		return nil
	}
	ids, err := h.store.ChatIDs(ctx)
	if err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	if len(ids) == 0 {
		_, _ = m.Reply("No chats with recorded playback were found.", htmlOptions())
		return nil
	}
	status, _ := m.Reply(fmt.Sprintf("📢 Broadcasting to <b>%d</b> chat(s)…", len(ids)), htmlOptions())
	success := 0
	for _, chatID := range ids {
		if _, err := h.bot.SendMessage(chatID, text, htmlOptions()); err != nil {
			h.logger.Warn("broadcast failed to chat", "chat_id", chatID, "error", err)
			continue
		}
		success++
		time.Sleep(50 * time.Millisecond)
	}
	_, _ = status.Edit(fmt.Sprintf("📢 Broadcast finished: <b>%d/%d</b> delivered.", success, len(ids)), htmlOptions())
	return nil
}

// -- welcome -----------------------------------------------------------------

func (h *Handlers) welcome(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	text, err := h.store.GetWelcome(ctx, h.botID)
	if err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	if strings.TrimSpace(text) == "" {
		_, _ = m.Reply("No start welcome message is set. The bot owner can set one with <code>/setwelcome</code> in a reply.", htmlOptions())
		return nil
	}
	body := formatWelcome(text, userMention(senderOrNil(m)), m.ChannelID(), h.botMention())
	_, _ = replyRich(m, body, htmlOptions())
	return nil
}

// -- stats -------------------------------------------------------------------

func (h *Handlers) stats(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if !h.requirePrivileged(ctx, m) {
		return nil
	}
	top, err := h.store.TopChats(ctx, 10)
	if err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	var builder strings.Builder
	builder.WriteString("📊 <b>Top chats by plays</b>\n")
	for index, stat := range top {
		builder.WriteString(fmt.Sprintf("%d. <code>%d</code> — %d plays", index+1, stat.ChatID, stat.PlayCount))
		if stat.LastPlayed > 0 {
			builder.WriteString(fmt.Sprintf(" (last %s)", time.Unix(stat.LastPlayed, 0).Format("2006-01-02")))
		}
		builder.WriteString("\n")
	}
	_, _ = m.Reply(strings.TrimRight(builder.String(), "\n"), htmlOptions())
	return nil
}

// -- playlists ---------------------------------------------------------------

func (h *Handlers) playlist(m *telegram.NewMessage) error {
	args := strings.Fields(m.Args())
	userID := m.SenderID()
	if userID <= 0 {
		_, _ = m.Reply("Please run this from your own account.", htmlOptions())
		return nil
	}
	if len(args) == 0 {
		return h.myPlaylist(m)
	}
	switch strings.ToLower(args[0]) {
	case "new":
		return h.playlistCreate(m, userID, strings.Join(args[1:], " "))
	case "del", "delete", "rmpl":
		return h.playlistDelete(m, userID, strings.Join(args[1:], " "))
	case "add":
		if len(args) < 3 {
			_, _ = m.Reply("Usage: <code>/playlist add name query-or-URL</code>", htmlOptions())
			return nil
		}
		return h.playlistAdd(m, userID, args[1], strings.Join(args[2:], " "))
	case "rm", "remove":
		if len(args) < 3 {
			_, _ = m.Reply("Usage: <code>/playlist rm name index</code>", htmlOptions())
			return nil
		}
		return h.playlistRemove(m, userID, args[1], args[2])
	case "play", "p":
		return h.playPlaylistNamed(m, strings.Join(args[1:], " "))
	default:
		return h.playlistShow(m, userID, strings.Join(args, " "))
	}
}

func (h *Handlers) myPlaylist(m *telegram.NewMessage) error {
	userID := m.SenderID()
	if userID <= 0 {
		_, _ = m.Reply("Please run this from your own account.", htmlOptions())
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	playlists, err := h.store.GetPlaylists(ctx, userID)
	if err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	if len(playlists) == 0 {
		_, _ = m.Reply("You have no playlists. Create one with <code>/playlist new name</code>.", htmlOptions())
		return nil
	}
	var builder strings.Builder
	builder.WriteString("🎼 <b>Your playlists</b>\n")
	for index, playlist := range playlists {
		builder.WriteString(fmt.Sprintf("%d. <code>%s</code> — %d tracks\n", index+1, html.EscapeString(playlist.Name), len(playlist.Tracks)))
	}
	_, _ = m.Reply(strings.TrimRight(builder.String(), "\n"), htmlOptions())
	return nil
}

func (h *Handlers) playlistCreate(m *telegram.NewMessage, userID int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		_, _ = m.Reply("Usage: <code>/playlist new name</code>", htmlOptions())
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	existing, err := h.store.GetPlaylists(ctx, userID)
	if err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	if len(existing) >= 5 {
		_, _ = m.Reply("You can have at most 5 playlists.", htmlOptions())
		return nil
	}
	for _, playlist := range existing {
		if strings.EqualFold(playlist.Name, name) {
			_, _ = m.Reply("A playlist with that name already exists.", htmlOptions())
			return nil
		}
	}
	if _, err := h.store.CreatePlaylist(ctx, userID, name); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply(fmt.Sprintf("✅ Playlist <code>%s</code> created.", html.EscapeString(name)), htmlOptions())
	return nil
}

func (h *Handlers) playlistDelete(m *telegram.NewMessage, userID int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		_, _ = m.Reply("Usage: <code>/playlist del name</code>", htmlOptions())
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	playlist, err := h.store.GetPlaylistByName(ctx, userID, name)
	if err != nil || playlist == nil {
		_, _ = m.Reply("❌ Playlist not found.", htmlOptions())
		return nil
	}
	if err := h.store.DeletePlaylist(ctx, userID, playlist.ID); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply("🗑️ Playlist deleted.", htmlOptions())
	return nil
}

func (h *Handlers) playlistShow(m *telegram.NewMessage, userID int64, name string) error {
	name = strings.TrimSpace(name)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	playlist, err := h.store.GetPlaylistByName(ctx, userID, name)
	if err != nil || playlist == nil {
		_, _ = m.Reply("❌ Playlist not found.", htmlOptions())
		return nil
	}
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("🎼 <b>%s</b> — %d tracks\n", html.EscapeString(playlist.Name), len(playlist.Tracks)))
	for index, track := range playlist.Tracks {
		if index >= 20 {
			builder.WriteString(fmt.Sprintf("\n…and %d more", len(playlist.Tracks)-index))
			break
		}
		title := track.Title
		if title == "" {
			title = track.Query
		}
		builder.WriteString(fmt.Sprintf("%d. %s\n", index+1, html.EscapeString(title)))
	}
	_, _ = m.Reply(strings.TrimRight(builder.String(), "\n"), htmlOptions())
	return nil
}

func (h *Handlers) playlistAdd(m *telegram.NewMessage, userID int64, name, query string) error {
	name = strings.TrimSpace(name)
	if name == "" || strings.TrimSpace(query) == "" {
		_, _ = m.Reply("Usage: <code>/playlist add name song-name-or-URL</code>", htmlOptions())
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	playlist, err := h.store.GetPlaylistByName(ctx, userID, name)
	if err != nil || playlist == nil {
		_, _ = m.Reply("❌ Playlist not found.", htmlOptions())
		return nil
	}
	if len(playlist.Tracks) >= 50 {
		_, _ = m.Reply("This playlist already has 50 tracks.", htmlOptions())
		return nil
	}
	track := storage.PlaylistTrack{Query: strings.TrimSpace(query), Title: strings.TrimSpace(query)}
	if err := h.store.AddPlaylistTrack(ctx, userID, playlist.ID, track); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply(fmt.Sprintf("➕ Added to <code>%s</code>.", html.EscapeString(playlist.Name)), htmlOptions())
	return nil
}

func (h *Handlers) playlistRemove(m *telegram.NewMessage, userID int64, name, indexArg string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	playlist, err := h.store.GetPlaylistByName(ctx, userID, name)
	if err != nil || playlist == nil {
		_, _ = m.Reply("❌ Playlist not found.", htmlOptions())
		return nil
	}
	index, ok := parseIndex(indexArg)
	if !ok || index < 1 || index > len(playlist.Tracks) {
		_, _ = m.Reply(fmt.Sprintf("Index must be between 1 and %d.", len(playlist.Tracks)), htmlOptions())
		return nil
	}
	if err := h.store.RemovePlaylistTrack(ctx, userID, playlist.ID, index-1); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply("🗑️ Track removed.", htmlOptions())
	return nil
}

func parseIndex(raw string) (int, bool) {
	var value int
	_, err := fmt.Sscanf(raw, "%d", &value)
	return value, err == nil
}

func (h *Handlers) playPlaylist(m *telegram.NewMessage) error {
	return h.playPlaylistNamed(m, m.Args())
}

func (h *Handlers) playPlaylistNamed(m *telegram.NewMessage, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		_, _ = m.Reply("Usage: <code>/pl playlist-name</code>", htmlOptions())
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	playlist, err := h.store.GetPlaylistByName(ctx, m.SenderID(), name)
	if err != nil || playlist == nil {
		_, _ = m.Reply("❌ Playlist not found.", htmlOptions())
		return nil
	}
	if len(playlist.Tracks) == 0 {
		_, _ = m.Reply("That playlist is empty.", htmlOptions())
		return nil
	}
	entries := make([]media.SourceEntry, 0, len(playlist.Tracks))
	for _, track := range playlist.Tracks {
		entries = append(entries, media.SourceEntry{Query: track.Query, Title: track.Title, Duration: time.Duration(track.Duration) * time.Second})
	}
	return h.playEntries(m, false, entries, m.ChannelID(), false)
}
