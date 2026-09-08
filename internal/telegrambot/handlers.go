package telegrambot

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/nub-coders/nub-go-music-bot/internal/media"
	"github.com/nub-coders/nub-go-music-bot/internal/playback"
	"github.com/nub-coders/nub-go-music-bot/internal/storage"
)

type Handlers struct {
	bot     *telegram.Client
	player  *playback.Service
	auth    *Authorizer
	store   storage.Access
	sources *media.Sources
	botID   int64
	ownerID int64
	logger  *slog.Logger
	timeout time.Duration
	started time.Time

	mu         sync.Mutex
	npMessages map[int64]int32 // chatID -> now-playing message ID
}

func NewHandlers(bot *telegram.Client, player *playback.Service, auth *Authorizer, store storage.Access, sources *media.Sources, botID, ownerID int64, timeout time.Duration, logger *slog.Logger) *Handlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handlers{
		bot: bot, player: player, auth: auth, store: store, sources: sources,
		botID: botID, ownerID: ownerID, timeout: timeout, logger: logger,
		started: time.Now(), npMessages: make(map[int64]int32),
	}
}

func (h *Handlers) Register() {
	// Global commands.
	h.bot.OnCommand("start", h.start)
	h.bot.OnCommand("help", h.help)
	h.bot.OnCommand("ping", h.ping)
	h.bot.OnCommand("about", h.about)
	h.bot.OnCommand("sudo", func(m *telegram.NewMessage) error { return h.setSudo(m, true) })
	h.bot.OnCommand("delsudo", func(m *telegram.NewMessage) error { return h.setSudo(m, false) })
	h.bot.OnCommand("sudolist", h.sudoList)
	h.bot.OnCommand("stats", h.stats)
	h.bot.OnCommand("playlist", h.playlist)
	h.bot.OnCommand("myplaylist", h.myPlaylist)
	h.bot.OnCommand("pl", h.playPlaylist)
	h.bot.OnCommand("pplay", h.playPlaylist)

	// Group-only playback commands.
	h.bot.OnCommand("play", func(m *telegram.NewMessage) error { return h.play(m, false) }, telegram.IsGroup)
	h.bot.OnCommand("vplay", func(m *telegram.NewMessage) error { return h.play(m, true) }, telegram.IsGroup)
	h.bot.OnCommand("pause", func(m *telegram.NewMessage) error { return h.pause(m, true) }, telegram.IsGroup)
	h.bot.OnCommand("resume", func(m *telegram.NewMessage) error { return h.pause(m, false) }, telegram.IsGroup)
	h.bot.OnCommand("skip", h.skip, telegram.IsGroup)
	h.bot.OnCommand("stop", h.stop, telegram.IsGroup)
	h.bot.OnCommand("end", h.stop, telegram.IsGroup)
	h.bot.OnCommand("queue", h.queue, telegram.IsGroup)
	h.bot.OnCommand("q", h.queue, telegram.IsGroup)
	h.bot.OnCommand("loop", h.loop, telegram.IsGroup)
	h.bot.OnCommand("shuffle", h.shuffle, telegram.IsGroup)
	h.bot.OnCommand("seek", h.seekForward, telegram.IsGroup)
	h.bot.OnCommand("seekback", h.seekBack, telegram.IsGroup)
	h.bot.OnCommand("np", h.nowPlaying, telegram.IsGroup)
	h.bot.OnCommand("nowplaying", h.nowPlaying, telegram.IsGroup)

	// Auth / admin commands (usable in a group or in the bot's private chat).
	h.bot.OnCommand("auth", func(m *telegram.NewMessage) error { return h.setAuthorized(m, true) }, telegram.IsGroup)
	h.bot.OnCommand("unauth", func(m *telegram.NewMessage) error { return h.setAuthorized(m, false) }, telegram.IsGroup)
	h.bot.OnCommand("authlist", h.authList, telegram.IsGroup)
	h.bot.OnCommand("block", h.block, telegram.IsGroup)
	h.bot.OnCommand("unblock", h.unblock, telegram.IsGroup)
	h.bot.OnCommand("blocklist", h.blockList, telegram.IsGroup)
	h.bot.OnCommand("broadcast", h.broadcast)
	h.bot.OnCommand("fbroadcast", h.broadcastForce)
	h.bot.OnCommand("setwelcome", h.setWelcome, telegram.IsGroup)
	h.bot.OnCommand("welcome", h.welcome, telegram.IsGroup)

	// Inline control buttons on the now-playing card.
	h.bot.AddCallbackHandler("np:", h.onNPButton, telegram.IsGroup)
}

// -- /play & /vplay ----------------------------------------------------------

func (h *Handlers) play(m *telegram.NewMessage, video bool) error {
	input := strings.TrimSpace(m.Args())
	if input == "" {
		_, _ = m.Reply("Usage: <code>/play song name or URL</code>", htmlOptions())
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()
	entries := []media.SourceEntry{{Query: input}}
	if h.sources != nil {
		entries = h.sources.Expand(ctx, input)
	}
	if len(entries) == 0 {
		entries = []media.SourceEntry{{Query: input}}
	}
	return h.playEntries(m, video, entries)
}

func (h *Handlers) playEntries(m *telegram.NewMessage, video bool, entries []media.SourceEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()
	if !h.requireControl(ctx, m) {
		return nil
	}
	sender, _ := m.GetSender()
	requesterName, requesterID := "User", m.SenderID()
	if sender != nil {
		requesterName = strings.TrimSpace(sender.FirstName + " " + sender.LastName)
	}

	status, err := m.Reply(fmt.Sprintf("🔎 Resolving <b>%d</b> source(s)…", len(entries)), htmlOptions())
	if err != nil {
		return err
	}

	var started *playback.EnqueueResult
	var queued []playback.EnqueueResult
	for index, entry := range entries {
		itemCtx, itemCancel := context.WithTimeout(ctx, h.timeout)
		result, err := h.player.Enqueue(itemCtx, m.ChannelID(), entry.Query, video, requesterID, requesterName)
		itemCancel()
		if err != nil {
			if index == 0 && len(entries) == 1 {
				_, _ = status.Edit("❌ "+html.EscapeString(err.Error()), htmlOptions())
				return nil
			}
			h.logger.Warn("skipping source that failed to resolve", "chat_id", m.ChannelID(), "query", entry.Query, "error", err)
			continue
		}
		if result.Started {
			started = &result
		} else {
			queued = append(queued, result)
		}
		if len(entries) > 1 && (index+1)%5 == 0 {
			_, _ = status.Edit(fmt.Sprintf("🔎 Queued <b>%d</b>/<b>%d</b> sources…", index+1, len(entries)), htmlOptions())
		}
	}

	var builder strings.Builder
	if started != nil {
		builder.WriteString("▶️ <b>Started:</b> " + formatTrack(started.Track))
		builder.WriteString("\n")
	}
	for index, result := range queued {
		if index >= 10 {
			builder.WriteString(fmt.Sprintf("\n…and %d more queued", len(queued)-index))
			break
		}
		builder.WriteString(fmt.Sprintf("➕ <b>Queued #%d:</b> %s", result.Position, formatTrack(result.Track)))
		builder.WriteString("\n")
	}
	if started == nil && len(queued) == 0 {
		_, _ = status.Edit("❌ None of the sources could be resolved.", htmlOptions())
		return nil
	}
	_, _ = status.Edit(strings.TrimRight(builder.String(), "\n"), htmlOptions())
	h.refreshNowPlaying(m.ChannelID())
	return nil
}

// -- playback controls -------------------------------------------------------

func (h *Handlers) pause(m *telegram.NewMessage, paused bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if !h.requireControl(ctx, m) {
		return nil
	}
	if err := h.player.Pause(ctx, m.ChannelID(), paused); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	message := "▶️ Playback resumed."
	if paused {
		message = "⏸️ Playback paused."
	}
	_, _ = m.Reply(message, htmlOptions())
	h.refreshNowPlaying(m.ChannelID())
	return nil
}

func (h *Handlers) skip(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if !h.requireControl(ctx, m) {
		return nil
	}
	track, err := h.player.Skip(ctx, m.ChannelID())
	if err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	if track == nil {
		_, _ = m.Reply("⏹️ Queue ended.", htmlOptions())
		h.closeNowPlaying(m.ChannelID())
		return nil
	}
	_, _ = m.Reply("⏭️ <b>Now playing:</b> "+formatTrack(*track), htmlOptions())
	h.refreshNowPlaying(m.ChannelID())
	return nil
}

func (h *Handlers) stop(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if !h.requireControl(ctx, m) {
		return nil
	}
	if err := h.player.Stop(ctx, m.ChannelID()); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply("⏹️ Playback stopped and queue cleared.", htmlOptions())
	h.closeNowPlaying(m.ChannelID())
	return nil
}

func (h *Handlers) shuffle(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if !h.requireControl(ctx, m) {
		return nil
	}
	if err := h.player.Shuffle(ctx, m.ChannelID()); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply("🔀 Queue shuffled.", htmlOptions())
	h.refreshNowPlaying(m.ChannelID())
	return nil
}

func parseSeekArg(raw string) (time.Duration, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Duration(seconds) * time.Second, true
	}
	parts := strings.Split(raw, ":")
	if len(parts) > 3 {
		return 0, false
	}
	var total int64
	for _, part := range parts {
		value, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil {
			return 0, false
		}
		total = total*60 + value
	}
	return time.Duration(total) * time.Second, true
}

func (h *Handlers) seekForward(m *telegram.NewMessage) error { return h.seek(m, true) }
func (h *Handlers) seekBack(m *telegram.NewMessage) error    { return h.seek(m, false) }

func (h *Handlers) seek(m *telegram.NewMessage, forward bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if !h.requireControl(ctx, m) {
		return nil
	}
	offset, ok := parseSeekArg(m.Args())
	if !ok {
		_, _ = m.Reply("Usage: <code>/seek 90</code>, <code>/seek 1:30</code> or <code>/seekback 15</code>", htmlOptions())
		return nil
	}
	if forward {
		snapshot, err := h.player.Snapshot(ctx, m.ChannelID())
		if err != nil {
			_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
			return nil
		}
		// /seek N jumps to an absolute position; /seekback N rewinds by N.
		target := offset
		if snapshot.Current != nil && snapshot.Current.Duration > 0 {
			// "/seek" is treated as an absolute position only when the argument
			// includes a colon (1:30); plain seconds move forward from current.
			if strings.Contains(m.Args(), ":") {
				target = offset
			} else {
				target = snapshot.Position + offset
			}
		} else {
			target = snapshot.Position + offset
		}
		offset = target
	} else {
		snapshot, err := h.player.Snapshot(ctx, m.ChannelID())
		if err != nil {
			_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
			return nil
		}
		offset = snapshot.Position - offset
	}
	if err := h.player.Seek(ctx, m.ChannelID(), offset); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply(fmt.Sprintf("🎚️ Seeking to <code>%s</code>", formatDuration(offset)), htmlOptions())
	h.refreshNowPlaying(m.ChannelID())
	return nil
}

func (h *Handlers) queue(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snapshot, err := h.player.Snapshot(ctx, m.ChannelID())
	if err != nil || snapshot.Current == nil {
		_, _ = m.Reply("The playback queue is empty.", htmlOptions())
		return nil
	}
	_, _ = m.Reply(queueText(snapshot), htmlOptions())
	return nil
}

func (h *Handlers) loop(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if !h.requireControl(ctx, m) {
		return nil
	}
	var mode playback.LoopMode
	switch strings.ToLower(strings.TrimSpace(m.Args())) {
	case "", "off", "0":
		mode = playback.LoopOff
	case "track", "one", "1":
		mode = playback.LoopTrack
	case "queue", "all":
		mode = playback.LoopQueue
	default:
		_, _ = m.Reply("Usage: <code>/loop off|track|queue</code>", htmlOptions())
		return nil
	}
	if err := h.player.SetLoop(ctx, m.ChannelID(), mode); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply(fmt.Sprintf("🔁 Loop mode set to <code>%s</code>.", []string{"off", "track", "queue"}[mode]), htmlOptions())
	h.refreshNowPlaying(m.ChannelID())
	return nil
}

// -- auth / sudo -------------------------------------------------------------

func (h *Handlers) setAuthorized(m *telegram.NewMessage, add bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	allowed, err := h.auth.CanManage(ctx, m.ChannelID(), m.SenderID())
	if err != nil || !allowed {
		_, _ = m.Reply("⛔ Only chat administrators, sudo users, or the owner can do that.", htmlOptions())
		return nil
	}
	target := targetUserID(m)
	if target <= 0 {
		_, _ = m.Reply("Reply to a user or provide a numeric user ID.", htmlOptions())
		return nil
	}
	if add {
		err = h.store.AddAuthorized(ctx, h.botID, m.ChannelID(), target)
	} else {
		err = h.store.RemoveAuthorized(ctx, h.botID, m.ChannelID(), target)
	}
	if err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	action := "authorized"
	if !add {
		action = "unauthorized"
	}
	_, _ = m.Reply(fmt.Sprintf("✅ User <code>%d</code> %s.", target, action), htmlOptions())
	return nil
}

func (h *Handlers) setSudo(m *telegram.NewMessage, add bool) error {
	if h.ownerID <= 0 || m.SenderID() != h.ownerID {
		_, _ = m.Reply("⛔ Only the configured owner can manage sudo users.", htmlOptions())
		return nil
	}
	target := targetUserID(m)
	if target <= 0 {
		_, _ = m.Reply("Reply to a user or provide a numeric user ID.", htmlOptions())
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var err error
	if add {
		err = h.store.AddSudo(ctx, h.botID, target)
	} else {
		err = h.store.RemoveSudo(ctx, h.botID, target)
	}
	if err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply("✅ Sudo list updated.", htmlOptions())
	return nil
}

func (h *Handlers) requireControl(ctx context.Context, m *telegram.NewMessage) bool {
	allowed, err := h.auth.CanControl(ctx, m.ChannelID(), m.SenderID())
	if err != nil {
		h.logger.Error("authorization check failed", "error", err)
		_, _ = m.Reply("❌ Could not verify your permissions.", htmlOptions())
		return false
	}
	if !allowed {
		_, _ = m.Reply("⛔ You must be a chat administrator or an authorized playback user.", htmlOptions())
		return false
	}
	return true
}

func targetUserID(m *telegram.NewMessage) int64 {
	if reply, _ := m.GetReplyMessage(); reply != nil && reply.SenderID() > 0 {
		return reply.SenderID()
	}
	id, _ := strconv.ParseInt(strings.TrimSpace(m.Args()), 10, 64)
	return id
}

func formatTrack(track media.Track) string {
	title := track.Title
	if title == "" {
		title = track.OriginalInput
	}
	text := html.EscapeString(title)
	if track.Author != "" {
		text += " · " + html.EscapeString(track.Author)
	}
	if track.Duration > 0 {
		text += " <code>" + formatDuration(track.Duration) + "</code>"
	}
	return text
}
func formatDuration(duration time.Duration) string {
	seconds := int64(duration.Seconds())
	if seconds >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", seconds/3600, (seconds%3600)/60, seconds%60)
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}
func progressBar(position, duration time.Duration) string {
	if duration <= 0 {
		return ""
	}
	const width = 12
	filled := int((position.Seconds() / duration.Seconds()) * width)
	if filled > width {
		filled = width
	}
	return "▰" + strings.Repeat("▬", filled) + strings.Repeat("—", width-filled) + "▰"
}
func htmlOptions() *telegram.SendOptions { return &telegram.SendOptions{ParseMode: "html"} }

func queueText(snapshot playback.Snapshot) string {
	var builder strings.Builder
	builder.WriteString("<b>Now playing</b>\n")
	if snapshot.Current != nil {
		builder.WriteString(formatTrack(*snapshot.Current))
	}
	if snapshot.Paused {
		builder.WriteString(" — <i>paused</i>")
	}
	if snapshot.Current != nil && snapshot.Current.Duration > 0 {
		builder.WriteString("\n" + progressBar(snapshot.Position, snapshot.Current.Duration))
		builder.WriteString(fmt.Sprintf(" <code>%s / %s</code>", formatDuration(snapshot.Position), formatDuration(snapshot.Current.Duration)))
	}
	builder.WriteString(fmt.Sprintf("\n🔁 <code>%s</code>", []string{"off", "track", "queue"}[snapshot.Loop]))
	for index, track := range snapshot.Queue {
		if index == 0 {
			builder.WriteString("\n\n<b>Up next</b>")
		}
		if index >= 15 {
			builder.WriteString(fmt.Sprintf("\n…and %d more", len(snapshot.Queue)-index))
			break
		}
		builder.WriteString(fmt.Sprintf("\n%d. %s", index+1, formatTrack(track)))
	}
	return builder.String()
}

// -- observer callbacks from the playback service ----------------------------

func (h *Handlers) TrackStarted(chatID int64, track media.Track) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = h.store.RecordPlay(ctx, chatID)
	h.postNowPlaying(chatID)
}
func (h *Handlers) QueueEnded(chatID int64) {
	h.closeNowPlaying(chatID)
}
func (h *Handlers) PlaybackError(chatID int64, err error) {
	h.logger.Error("playback error", "chat_id", chatID, "error", err)
	h.closeNowPlaying(chatID)
}
