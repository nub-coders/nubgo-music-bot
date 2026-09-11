package telegrambot

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/nub-coders/nub-go-music-bot/internal/media"
	"github.com/nub-coders/nub-go-music-bot/internal/playback"
	"github.com/nub-coders/nub-go-music-bot/internal/storage"
	"github.com/nub-coders/nub-go-music-bot/internal/voice"
)

type Handlers struct {
	bot          *telegram.Client
	player       *playback.Service
	auth         *Authorizer
	store        storage.Access
	sources      *media.Sources
	related      *media.RelatedResolver
	botID        int64
	ownerID      int64
	supportGroup string
	logger       *slog.Logger
	timeout      time.Duration
	started      time.Time

	mu         sync.Mutex
	npMessages map[int64]int32              // playback chatID -> now-playing message ID
	npLocks    map[int64]*sync.Mutex        // playback chatID -> card serialization lock
	npProgress map[int64]context.CancelFunc // playback chatID -> progress loop cancel

	// Autoplay suggestion state. Cards live in the UI chat; the countdown and
	// the candidate list are keyed by the playback chat.
	autoplayCards       map[int64]int32              // playback chatID -> suggestion card message ID
	autoplayCancels     map[int64]context.CancelFunc // playback chatID -> countdown cancel
	autoplaySuggestions map[int64][]media.Suggestion // playback chatID -> last suggestions
	// Channel playback (/cplay) streams into a chat linked to the one the
	// command was sent in, so cards and buttons stay where the user is.
	uiChats       map[int64]int64 // playback chatID -> chat the cards live in
	playbackChats map[int64]int64 // command chatID -> playback chatID

	// recentPlayed tracks video IDs played in each chat to avoid autoplay loops.
	recentPlayed map[int64][]string

	voice *voice.Manager
}

func NewHandlers(bot *telegram.Client, player *playback.Service, auth *Authorizer, store storage.Access, sources *media.Sources, botID, ownerID int64, supportGroup string, timeout time.Duration, logger *slog.Logger, voice *voice.Manager) *Handlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handlers{
		bot: bot, player: player, auth: auth, store: store, sources: sources,
		related: &media.RelatedResolver{Client: &http.Client{Timeout: 15 * time.Second}},
		botID:   botID, ownerID: ownerID, supportGroup: supportGroup, timeout: timeout, logger: logger,
		started: time.Now(), npMessages: make(map[int64]int32), voice: voice,
		npLocks:    make(map[int64]*sync.Mutex),
		npProgress: make(map[int64]context.CancelFunc),
		uiChats:    make(map[int64]int64), playbackChats: make(map[int64]int64),
		recentPlayed:        make(map[int64][]string),
		autoplayCards:       make(map[int64]int32),
		autoplayCancels:     make(map[int64]context.CancelFunc),
		autoplaySuggestions: make(map[int64][]media.Suggestion),
	}
}

// npLock returns the per-chat mutex that serializes now-playing card writes.
func (h *Handlers) npLock(chatID int64) *sync.Mutex {
	h.mu.Lock()
	defer h.mu.Unlock()
	lock, ok := h.npLocks[chatID]
	if !ok {
		lock = &sync.Mutex{}
		h.npLocks[chatID] = lock
	}
	return lock
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
	h.bot.OnCommand("playforce", func(m *telegram.NewMessage) error { return h.playWithMode(m, false, true, false) }, telegram.IsGroup)
	h.bot.OnCommand("vplayforce", func(m *telegram.NewMessage) error { return h.playWithMode(m, true, true, false) }, telegram.IsGroup)
	h.bot.OnCommand("cplay", func(m *telegram.NewMessage) error { return h.playWithMode(m, false, false, true) }, telegram.IsGroup)
	h.bot.OnCommand("cvplay", func(m *telegram.NewMessage) error { return h.playWithMode(m, true, false, true) }, telegram.IsGroup)
	h.bot.OnCommand("cplayforce", func(m *telegram.NewMessage) error { return h.playWithMode(m, false, true, true) }, telegram.IsGroup)
	h.bot.OnCommand("cvplayforce", func(m *telegram.NewMessage) error { return h.playWithMode(m, true, true, true) }, telegram.IsGroup)
	h.bot.OnCommand("pause", func(m *telegram.NewMessage) error { return h.pause(m, true) }, telegram.IsGroup)
	h.bot.OnCommand("resume", func(m *telegram.NewMessage) error { return h.pause(m, false) }, telegram.IsGroup)
	h.bot.OnCommand("skip", h.skip, telegram.IsGroup)
	h.bot.OnCommand("cskip", h.skip, telegram.IsGroup)
	h.bot.OnCommand("stop", h.stop, telegram.IsGroup)
	h.bot.OnCommand("end", h.stop, telegram.IsGroup)
	h.bot.OnCommand("cstop", h.stop, telegram.IsGroup)
	h.bot.OnCommand("cend", h.stop, telegram.IsGroup)
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

	// Autoplay / related-suggestion commands. The channel (linked-chat) variants
	// map through controlChatID the same way the play commands do.
	h.bot.OnCommand("autoplay", h.autoplay, telegram.IsGroup)
	h.bot.OnCommand("cautoplay", h.autoplay, telegram.IsGroup)
	h.bot.OnCommand("suggest", h.autoplay, telegram.IsGroup)
	h.bot.OnCommand("csuggest", h.autoplay, telegram.IsGroup)

	// Inline control buttons on the now-playing card and rich help cards.
	h.bot.AddCallbackHandler("np:", h.onNPButton, telegram.IsGroup)
	h.bot.OnCallback("commands_", h.commandsCallback)

	// Suggestion-card buttons. Patterns are anchored regexes, and the optional
	// leading "c" selects the channel-mode variants.
	h.bot.OnCallback("^c?sgplay_", h.onSuggestionPlay)
	h.bot.OnCallback("^c?sgstop$", h.onSuggestionStop)
	h.bot.OnCallback("^c?sgtoggle$", h.onSuggestionToggle)
	h.bot.OnCallback("^c?sgclose$", h.onSuggestionClose)
}

// -- /play, /vplay and their force / channel variants ------------------------

func (h *Handlers) play(m *telegram.NewMessage, video bool) error {
	return h.playWithMode(m, video, false, false)
}

// playWithMode backs every play command. force replaces whatever is playing
// right now; channelMode streams into the chat linked to this one.
func (h *Handlers) playWithMode(m *telegram.NewMessage, video, force, channelMode bool) error {
	input := strings.TrimSpace(m.Args())
	if input == "" {
		_, _ = m.Reply("Usage: <code>/play song name or URL</code>", htmlOptions())
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()

	targetChatID := m.ChannelID()
	if channelMode {
		linked, err := h.linkedChatID(m.ChannelID())
		if err != nil {
			h.logger.Warn("resolve linked chat", "chat_id", m.ChannelID(), "error", err)
			_, _ = replyRich(m, emoji(emojiError, "❌")+" <b>This chat has no linked channel to stream into.</b>", htmlOptions())
			return nil
		}
		targetChatID = linked
	}

	if !h.canPlay(ctx, m, targetChatID, force) {
		return nil
	}

	// A playlist / album link expands into many queries; a search or a single
	// video URL stays a one-element list.
	entries := []media.SourceEntry{{Query: input}}
	if h.sources != nil {
		if expanded := h.sources.Expand(ctx, input); len(expanded) > 0 {
			entries = expanded
		}
	}
	if channelMode {
		h.setUIChat(targetChatID, m.ChannelID())
	}
	return h.playEntries(m, video, entries, targetChatID, force)
}

// linkedChatID returns the chat linked to chatID — a discussion group's
// broadcast channel, or a channel's discussion group.
func (h *Handlers) linkedChatID(chatID int64) (int64, error) {
	peer, err := h.bot.ResolvePeer(chatID)
	if err != nil {
		return 0, err
	}
	channel, ok := peer.(*telegram.InputPeerChannel)
	if !ok {
		return 0, errors.New("basic groups cannot have a linked channel")
	}
	full, err := h.bot.ChannelsGetFullChannel(&telegram.InputChannelObj{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash})
	if err != nil {
		return 0, err
	}
	info, ok := full.FullChat.(*telegram.ChannelFull)
	if !ok || info.LinkedChatID == 0 {
		return 0, errors.New("no linked chat")
	}
	return -1_000_000_000_000 - info.LinkedChatID, nil
}

// canPlay authorizes a play command. Force-play interrupts the running track,
// so besides the usual admin / sudo / authorized tiers it also lets the user
// who requested the current track replace it.
func (h *Handlers) canPlay(ctx context.Context, m *telegram.NewMessage, targetChatID int64, force bool) bool {
	allowed, err := h.auth.CanControl(ctx, m.ChannelID(), m.SenderID())
	if err != nil {
		h.logger.Error("authorization check failed", "error", err)
		_, _ = m.Reply("❌ Could not verify your permissions.", htmlOptions())
		return false
	}
	if allowed {
		return true
	}
	if force && m.SenderID() > 0 {
		if current := h.player.Current(ctx, targetChatID); current != nil && current.RequesterID == m.SenderID() {
			return true
		}
	}
	_, _ = m.Reply("⛔ You must be a chat administrator or an authorized playback user.", htmlOptions())
	return false
}

func (h *Handlers) playEntries(m *telegram.NewMessage, video bool, entries []media.SourceEntry, targetChatID int64, force bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()
	sender, _ := m.GetSender()
	requesterName, requesterID := "User", m.SenderID()
	if sender != nil {
		requesterName = strings.TrimSpace(sender.FirstName + " " + sender.LastName)
	}

	status, err := replyRich(m, fmt.Sprintf("%s <b>Resolving %d source(s)…</b>", emoji(emojiLoading, "🔎"), len(entries)), htmlOptions())
	if err != nil {
		return err
	}

	var started *playback.EnqueueResult
	var queued []playback.EnqueueResult
	for index, entry := range entries {
		itemCtx, itemCancel := context.WithTimeout(ctx, h.timeout)
		var result playback.EnqueueResult
		var err error
		if force && index == 0 {
			// Resolves first, then interrupts: a query that cannot be played
			// leaves the current track running.
			result, err = h.player.ForcePlay(itemCtx, targetChatID, entry.Query, video, requesterID, requesterName)
		} else {
			result, err = h.player.Enqueue(itemCtx, targetChatID, entry.Query, video, requesterID, requesterName)
		}
		itemCancel()
		if err != nil {
			if index == 0 && len(entries) == 1 {
				_, _ = editRich(status, emoji(emojiError, "❌")+" "+escape(err.Error()), htmlOptions())
				return nil
			}
			h.logger.Warn("skipping source that failed to resolve", "chat_id", targetChatID, "query", entry.Query, "error", err)
			continue
		}
		if result.Started {
			started = &result
		} else {
			queued = append(queued, result)
		}
		if len(entries) > 1 && (index+1)%5 == 0 {
			_, _ = editRich(status, fmt.Sprintf("%s Queued <b>%d</b>/<b>%d</b> sources…", emoji(emojiLoading, "🔎"), index+1, len(entries)), htmlOptions())
		}
	}

	var builder strings.Builder
	for index, result := range queued {
		if index >= 10 {
			builder.WriteString(fmt.Sprintf("\n…and %d more queued", len(queued)-index))
			break
		}
		builder.WriteString(fmt.Sprintf("%s <b>Queued #%d:</b> %s", emoji(emojiAdd, "➕"), result.Position, formatTrack(result.Track)))
		builder.WriteString("\n")
	}
	if started == nil && len(queued) == 0 {
		_, _ = editRich(status, emoji(emojiError, "❌")+" None of the sources could be resolved.", htmlOptions())
		return nil
	}
	// The now-playing card already announces the started track, so the status
	// message is only kept when it still carries queue information. Otherwise it
	// is deleted to avoid posting the same track twice (matches the Python bot).
	if summary := strings.TrimRight(builder.String(), "\n"); summary != "" {
		_, _ = editRich(status, summary, htmlOptions())
	} else {
		_, _ = status.Delete()
	}
	h.refreshNowPlaying(targetChatID)
	return nil
}

// -- playback controls -------------------------------------------------------

// setUIChat records that playback in playbackChatID was started from uiChatID,
// so cards and later control commands follow the user's chat.
func (h *Handlers) setUIChat(playbackChatID, uiChatID int64) {
	h.mu.Lock()
	h.uiChats[playbackChatID] = uiChatID
	h.playbackChats[uiChatID] = playbackChatID
	h.mu.Unlock()
}

// uiChatFor returns the chat that a playback chat's cards belong in.
func (h *Handlers) uiChatFor(playbackChatID int64) int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ui, ok := h.uiChats[playbackChatID]; ok {
		return ui
	}
	return playbackChatID
}

// controlChatID maps a control command to the chat playback actually runs in.
// Only used when this chat has no playback of its own, so a group that plays
// locally is never redirected to its linked channel.
func (h *Handlers) controlChatID(ctx context.Context, m *telegram.NewMessage) int64 {
	chatID := m.ChannelID()
	h.mu.Lock()
	linked, ok := h.playbackChats[chatID]
	h.mu.Unlock()
	if !ok || linked == chatID {
		return chatID
	}
	if h.player.Current(ctx, chatID) != nil {
		return chatID
	}
	if h.player.Current(ctx, linked) != nil {
		return linked
	}
	return chatID
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

func (h *Handlers) pause(m *telegram.NewMessage, paused bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if !h.requireControl(ctx, m) {
		return nil
	}
	targetChatID := h.controlChatID(ctx, m)
	if err := h.player.Pause(ctx, targetChatID, paused); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	message := emoji(emojiPlay, "▶️") + " <b>Playback resumed.</b>"
	if paused {
		message = emoji(emojiPause, "⏸️") + " <b>Playback paused.</b>"
	}
	_, _ = replyRich(m, message, htmlOptions())
	h.refreshNowPlaying(targetChatID)
	return nil
}

func (h *Handlers) skip(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if !h.requireControl(ctx, m) {
		return nil
	}
	targetChatID := h.controlChatID(ctx, m)
	track, err := h.player.Skip(ctx, targetChatID)
	if err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	if track == nil {
		if h.player.AutoplayEnabled(targetChatID) {
			_, _ = replyRich(m, richNote(emoji(emojiSkip, "⏭️")+" <b>ǫᴜᴇᴜᴇ ɪs ᴇᴍᴘᴛʏ — ᴀᴜᴛᴏᴘʟᴀʏɪɴɢ ᴀ ʀᴇʟᴀᴛᴇᴅ ᴛʀᴀᴄᴋ…</b>"), htmlOptions())
			return nil
		}
		_, _ = replyRich(m, emoji(emojiStop, "⏹️")+" <b>Queue ended.</b>", htmlOptions())
		h.closeNowPlaying(targetChatID)
		return nil
	}
	_, _ = replyRich(m, emoji(emojiSkip, "⏭️")+" <b>Now playing:</b> "+formatTrack(*track), htmlOptions())
	h.refreshNowPlaying(targetChatID)
	return nil
}

func (h *Handlers) stop(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if !h.requireControl(ctx, m) {
		return nil
	}
	targetChatID := h.controlChatID(ctx, m)
	if err := h.player.Stop(ctx, targetChatID); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = replyRich(m, emoji(emojiStop, "⏹️")+" <b>Playback stopped and queue cleared.</b>", htmlOptions())
	h.closeNowPlaying(targetChatID)
	return nil
}

func (h *Handlers) shuffle(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if !h.requireControl(ctx, m) {
		return nil
	}
	targetChatID := h.controlChatID(ctx, m)
	if err := h.player.Shuffle(ctx, targetChatID); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = replyRich(m, emoji(emojiRefresh, "🔀")+" <b>Queue shuffled.</b>", htmlOptions())
	h.refreshNowPlaying(targetChatID)
	return nil
}

func (h *Handlers) seekForward(m *telegram.NewMessage) error { return h.seek(m, true) }
func (h *Handlers) seekBack(m *telegram.NewMessage) error    { return h.seek(m, false) }

func (h *Handlers) seek(m *telegram.NewMessage, forward bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if !h.requireControl(ctx, m) {
		return nil
	}
	targetChatID := h.controlChatID(ctx, m)
	offset, ok := parseSeekArg(m.Args())
	if !ok {
		_, _ = m.Reply("Usage: <code>/seek 90</code>, <code>/seek 1:30</code> or <code>/seekback 15</code>", htmlOptions())
		return nil
	}
	if forward {
		snapshot, err := h.player.Snapshot(ctx, targetChatID)
		if err != nil {
			_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
			return nil
		}
		// "/seek" is an absolute position only when the argument includes a
		// colon (1:30); plain seconds move forward from the current position.
		target := offset
		if !strings.Contains(m.Args(), ":") {
			target = snapshot.Position + offset
		}
		offset = target
	} else {
		snapshot, err := h.player.Snapshot(ctx, targetChatID)
		if err != nil {
			_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
			return nil
		}
		offset = snapshot.Position - offset
	}
	if err := h.player.Seek(ctx, targetChatID, offset); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = m.Reply(fmt.Sprintf("🎚️ Seeking to <code>%s</code>", formatDuration(offset)), htmlOptions())
	h.refreshNowPlaying(targetChatID)
	return nil
}

func (h *Handlers) queue(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snapshot, err := h.player.Snapshot(ctx, h.controlChatID(ctx, m))
	if err != nil || snapshot.Current == nil {
		_, _ = replyRich(m, emoji(emojiQueueIcon, "🗃")+" <b>The playback queue is empty.</b>", htmlOptions())
		return nil
	}
	_, _ = replyRich(m, queueText(snapshot), htmlOptions())
	return nil
}

func (h *Handlers) loop(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if !h.requireControl(ctx, m) {
		return nil
	}
	targetChatID := h.controlChatID(ctx, m)
	var mode playback.LoopMode
	switch strings.ToLower(strings.TrimSpace(m.Args())) {
	case "", "off", "0":
		mode = playback.LoopOff
	case "track", "one", "1":
		mode = playback.LoopTrack
	case "queue", "all":
		mode = playback.LoopQueue
	default:
		_, _ = replyRich(m, emoji(emojiInfo, "ℹ️")+" Usage: <code>/loop off|track|queue</code>", htmlOptions())
		return nil
	}
	if err := h.player.SetLoop(ctx, targetChatID, mode); err != nil {
		_, _ = m.Reply("❌ "+html.EscapeString(err.Error()), htmlOptions())
		return nil
	}
	_, _ = replyRich(m, fmt.Sprintf("%s Loop mode set to <code>%s</code>.", emoji(emojiLoop, "🔁"), []string{"off", "track", "queue"}[mode]), htmlOptions())
	h.refreshNowPlaying(targetChatID)
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

// progressLabel renders the playback progress shown inside a disabled inline
// button, matching the Python bot's 8-segment marker style:
//
//	01:23 ─ ─ ▷ ─ ─ ─ ─ 3:35
func progressLabel(position, duration time.Duration) string {
	if duration <= 0 {
		return ""
	}
	const segments = 8
	if position < 0 {
		position = 0
	}
	if position > duration {
		position = duration
	}
	marker := int((position.Seconds() / duration.Seconds()) * segments)
	if marker > segments-1 {
		marker = segments - 1
	}
	var bar strings.Builder
	for i := 0; i < segments; i++ {
		if i > 0 {
			bar.WriteString(" ")
		}
		if i == marker {
			bar.WriteString("▷")
			continue
		}
		bar.WriteString("─")
	}
	return formatDuration(position) + " " + bar.String() + " " + formatDuration(duration)
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
		builder.WriteString("\n<code>" + escape(progressLabel(snapshot.Position, snapshot.Current.Duration)) + "</code>")
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
	if h.voice != nil {
		h.voice.RecordPlay(chatID)
	}
	h.recordRecentPlay(chatID, track.ID)
	h.postNowPlaying(chatID)
	h.startProgress(chatID, track)
}
func (h *Handlers) QueueEnded(chatID int64) {
	h.closeNowPlaying(chatID)
}
func (h *Handlers) QueueDrained(chatID int64, lastTrack media.Track) {
	// The last track has finished, so its control card is stale. Drop it before
	// the suggestion card appears so the chat never shows two competing cards.
	h.closeNowPlaying(chatID)
	go h.handleAutoplay(chatID, lastTrack)
}
func (h *Handlers) PlaybackError(chatID int64, err error) {
	h.logger.Error("playback error", "chat_id", chatID, "error", err)
	h.closeNowPlaying(chatID)
}
