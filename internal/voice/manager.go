package voice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/nub-coders/nub-go-music-bot/internal/media"
	"github.com/nub-coders/nub-go-music-bot/internal/voice/ntg"
)

type Assistant struct {
	Index  int
	Client *telegram.Client
	Calls  *ntg.Client
	User   *telegram.UserObj
}

type callSession struct {
	assistant   *Assistant
	groupCall   *telegram.InputGroupCall
	description ntg.MediaDescription
	trackURL    string
	video       bool
	live        bool
	joined      bool
}

type Manager struct {
	bot         *telegram.Client
	logger      *slog.Logger
	mu          sync.RWMutex
	assistants  map[int]*Assistant
	sessions    map[int64]*callSession
	lastPlayed  map[int64]time.Time
	onStreamEnd func(chatID int64)
	onFailure   func(chatID int64, err error)
}

func NewManager(bot *telegram.Client, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		bot: bot, logger: logger,
		assistants: make(map[int]*Assistant),
		sessions:   make(map[int64]*callSession),
		lastPlayed: make(map[int64]time.Time),
	}
}

func (m *Manager) SetHandlers(onStreamEnd func(int64), onFailure func(int64, error)) {
	m.onStreamEnd = onStreamEnd
	m.onFailure = onFailure
}

func (m *Manager) RegisterAssistant(assistant *Assistant) {
	m.mu.Lock()
	m.assistants[assistant.Index] = assistant
	m.mu.Unlock()

	assistant.Calls.OnStreamEnd(func(chatID int64, streamType ntg.StreamType, device ntg.StreamDevice) {
		if chatID == 0 || streamType != ntg.AudioStream || device != ntg.MicrophoneStream {
			return
		}
		m.mu.RLock()
		active := m.sessions[chatID] != nil
		handler := m.onStreamEnd
		m.mu.RUnlock()
		if active && handler != nil {
			handler(chatID)
		}
	})
	assistant.Calls.OnConnectionChange(func(chatID int64, info ntg.NetworkInfo) {
		m.logger.Info("voice connection changed", "chat_id", chatID, "assistant", assistant.Index, "state", info.State)
		if info.State != ntg.Failed && info.State != ntg.Timeout && info.State != ntg.Closed {
			return
		}
		m.mu.RLock()
		active := m.sessions[chatID] != nil
		handler := m.onFailure
		m.mu.RUnlock()
		if active && handler != nil {
			handler(chatID, fmt.Errorf("voice connection entered terminal state %d", info.State))
		}
	})
}

// RecordPlay marks the chat as recently active so the auto-leave sweep
// does not evict it.
func (m *Manager) RecordPlay(chatID int64) {
	m.mu.Lock()
	m.lastPlayed[chatID] = time.Now()
	m.mu.Unlock()
}

// AutoLeaveOptions configures the idle-chat sweep.
type AutoLeaveOptions struct {
	Enabled bool
	// IdleTimeout is how long a chat must go without playback before an
	// assistant leaves it.
	IdleTimeout time.Duration
	// MaxPerSweep caps how many chats one assistant may leave per sweep.
	// Zero means unlimited. The cap bounds the blast radius of a bug in the
	// idle heuristic: rejoining hundreds of groups needs fresh invite links.
	MaxPerSweep int
	// DryRun logs every would-be leave without leaving.
	DryRun bool
	// LoggerID is never left.
	LoggerID int64
	// BotID and Store are used to keep chats with authorized users.
	BotID int64
	Store AuthorizedLister
}

// AuthorizedLister reports a chat's authorized playback users. Implemented by
// storage.Access; kept narrow so the voice package does not depend on storage.
type AuthorizedLister interface {
	ListAuthorized(ctx context.Context, botID, chatID int64) ([]int64, error)
}

const (
	autoLeaveStartupGrace = 90 * time.Second
	autoLeaveInterval     = time.Hour
	autoLeaveSpacing      = 2 * time.Second
)

// AutoLeaveLoop scans each assistant's dialogs and leaves groups that have
// been idle longer than the configured threshold, keeping assistants safely
// under Telegram's 500-group membership limit. It returns when ctx is done.
func (m *Manager) AutoLeaveLoop(ctx context.Context, opts AutoLeaveOptions) {
	if !opts.Enabled {
		m.logger.Info("[auto_leave] Disabled (AUTO_LEAVING_ASSISTANT is off); no groups will be left")
		return
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = 90 * time.Minute
	}
	sweepCap := "unlimited"
	if opts.MaxPerSweep > 0 {
		sweepCap = strconv.Itoa(opts.MaxPerSweep)
	}
	m.logger.Info("[auto_leave] Enabled",
		"idle_threshold", opts.IdleTimeout, "max_per_sweep", sweepCap, "dry_run", opts.DryRun)

	if !sleepCtx(ctx, autoLeaveStartupGrace) { // grace period on startup
		return
	}
	for {
		m.autoLeaveSweep(ctx, opts)
		if !sleepCtx(ctx, autoLeaveInterval) {
			return
		}
	}
}

func (m *Manager) autoLeaveSweep(ctx context.Context, opts AutoLeaveOptions) {
	m.mu.RLock()
	assistants := make([]*Assistant, 0, len(m.assistants))
	for _, assistant := range m.assistants {
		assistants = append(assistants, assistant)
	}
	m.mu.RUnlock()

	for _, assistant := range assistants {
		if ctx.Err() != nil {
			return
		}
		left := m.autoLeaveAssistant(ctx, assistant, opts)
		if left > 0 {
			verb := "left"
			if opts.DryRun {
				verb = "would leave"
			}
			m.logger.Info("[auto_leave] sweep finished", "assistant", assistant.Index, "action", verb, "chats", left)
		}
	}
}

// autoLeaveAssistant sweeps one assistant's dialogs and returns how many chats
// it left (or would have left, in dry-run mode).
func (m *Manager) autoLeaveAssistant(ctx context.Context, assistant *Assistant, opts AutoLeaveOptions) int {
	if assistant == nil || assistant.Client == nil {
		return 0
	}
	dialogs, err := assistant.Client.GetDialogs(&telegram.DialogOptions{Limit: 0, Context: ctx})
	if err != nil {
		m.logger.Warn("[auto_leave] Error scanning dialogs", "assistant", assistant.Index, "error", err)
		return 0
	}

	left := 0
	for index := range dialogs {
		if ctx.Err() != nil {
			return left
		}
		if opts.MaxPerSweep > 0 && left >= opts.MaxPerSweep {
			m.logger.Warn("[auto_leave] Per-sweep cap reached; deferring the rest to the next sweep",
				"assistant", assistant.Index, "cap", opts.MaxPerSweep)
			return left
		}
		dialog := dialogs[index]
		if !dialog.IsChat() && !dialog.IsChannel() {
			continue
		}
		chatID := dialog.GetChannelID()
		if chatID == 0 {
			continue
		}
		idle, ok := m.autoLeaveIdleFor(chatID, opts)
		if !ok {
			continue
		}
		if opts.DryRun {
			m.logger.Info("[auto_leave][DRY RUN] would leave idle chat — no action taken",
				"assistant", assistant.Index, "chat_id", chatID, "idle", idle)
			left++
			continue
		}
		m.logger.Info("[auto_leave] Leaving idle chat", "assistant", assistant.Index, "chat_id", chatID, "idle", idle)
		if err := assistant.Client.LeaveChannel(chatID); err != nil {
			m.logger.Warn("[auto_leave] Failed to leave chat", "assistant", assistant.Index, "chat_id", chatID, "error", err)
			continue
		}
		left++
		// Membership is gone: drop the playback record so a later /play
		// redoes the join handshake instead of trusting a stale entry.
		m.forgetChat(chatID)
		if !sleepCtx(ctx, autoLeaveSpacing) { // rate-limit safety spacing
			return left
		}
	}
	return left
}

// autoLeaveIdleFor decides whether a chat may be left, returning how long it
// has been idle. Every uncertain case keeps the chat: leaving is unrecoverable
// without a fresh invite link.
func (m *Manager) autoLeaveIdleFor(chatID int64, opts AutoLeaveOptions) (time.Duration, bool) {
	if opts.LoggerID != 0 && chatID == opts.LoggerID {
		return 0, false
	}

	m.mu.RLock()
	active := m.sessions[chatID] != nil
	lastPlayed, played := m.lastPlayed[chatID]
	m.mu.RUnlock()

	if active {
		return 0, false
	}
	// No observed playback means "unknown", not "idle" — never mass-leave
	// chats this process has simply never played in.
	if !played {
		return 0, false
	}
	idle := time.Since(lastPlayed)
	if idle < opts.IdleTimeout {
		return 0, false
	}
	if opts.Store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		authorized, err := opts.Store.ListAuthorized(ctx, opts.BotID, chatID)
		cancel()
		if err != nil {
			// Fail safe: an unreadable auth list must not turn into a leave.
			m.logger.Debug("[auto_leave] Keeping chat; authorization lookup failed", "chat_id", chatID, "error", err)
			return 0, false
		}
		if len(authorized) > 0 {
			return 0, false
		}
	}
	return idle, true
}

// forgetChat drops the playback record for a chat the assistant has left.
func (m *Manager) forgetChat(chatID int64) {
	m.mu.Lock()
	delete(m.lastPlayed, chatID)
	m.mu.Unlock()
}

// sleepCtx waits for d, returning false if ctx is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (m *Manager) AssistantCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.assistants)
}

func (m *Manager) Play(ctx context.Context, chatID int64, track media.Track) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	description := mediaDescription(track.StreamURL, track.Video, track.Live, 0)

	m.mu.RLock()
	existing := m.sessions[chatID]
	m.mu.RUnlock()
	if existing != nil && existing.joined {
		previous := existing.description
		if err := existing.assistant.Calls.SetStreamSources(chatID, ntg.CaptureStream, description); err != nil {
			if restoreErr := existing.assistant.Calls.SetStreamSources(chatID, ntg.CaptureStream, previous); restoreErr != nil {
				m.logger.Error("failed to restore previous media after source swap", "chat_id", chatID, "error", restoreErr)
			}
			return fmt.Errorf("replace voice media source: %w", err)
		}
		existing.description = description
		existing.trackURL = track.StreamURL
		existing.video = track.Video
		existing.live = track.Live
		return nil
	}

	assistant, err := m.selectAssistant(chatID)
	if err != nil {
		return err
	}
	if err := m.ensureAssistantInChat(assistant, chatID); err != nil {
		return err
	}
	groupCall, err := m.getGroupCall(assistant, chatID)
	if err != nil {
		return err
	}
	joinParams, err := assistant.Calls.CreateCall(chatID)
	if err != nil {
		return fmt.Errorf("create NTgCalls session: %w", err)
	}
	if err := assistant.Calls.SetStreamSources(chatID, ntg.CaptureStream, description); err != nil {
		_ = assistant.Calls.Stop(chatID)
		return fmt.Errorf("configure voice media source: %w", err)
	}
	updates, err := assistant.Client.PhoneJoinGroupCall(&telegram.PhoneJoinGroupCallParams{
		Call: *groupCall, JoinAs: &telegram.InputPeerSelf{}, Params: &telegram.DataJson{Data: joinParams},
	})
	if err != nil {
		_ = assistant.Calls.Stop(chatID)
		return fmt.Errorf("join Telegram group call: %w", err)
	}
	connectionJSON := extractConnectionJSON(updates)
	if connectionJSON == "" {
		_ = assistant.Calls.Stop(chatID)
		_ = assistant.Client.LeaveGroupCall(*groupCall, 0)
		return errors.New("Telegram join response did not contain voice connection data")
	}
	if err := assistant.Calls.Connect(chatID, connectionJSON, false); err != nil {
		_ = assistant.Calls.Stop(chatID)
		_ = assistant.Client.LeaveGroupCall(*groupCall, 0)
		return fmt.Errorf("connect NTgCalls transport: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = assistant.Calls.Stop(chatID)
		_ = assistant.Client.LeaveGroupCall(*groupCall, 0)
		return err
	}

	m.mu.Lock()
	m.sessions[chatID] = &callSession{assistant: assistant, groupCall: groupCall, description: description, trackURL: track.StreamURL, video: track.Video, live: track.Live, joined: true}
	m.mu.Unlock()
	return nil
}

// Seek restarts the current media source at an absolute offset (seconds). It
// reuses the live NTgCalls session when possible so the group call is not
// torn down.
func (m *Manager) Seek(ctx context.Context, chatID int64, offset time.Duration) error {
	m.mu.RLock()
	session := m.sessions[chatID]
	m.mu.RUnlock()
	if session == nil || !session.joined {
		return errors.New("voice chat is not connected")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	description := mediaDescription(session.trackURL, session.video, session.live, offset)
	previous := session.description
	if err := session.assistant.Calls.SetStreamSources(chatID, ntg.CaptureStream, description); err != nil {
		if restoreErr := session.assistant.Calls.SetStreamSources(chatID, ntg.CaptureStream, previous); restoreErr != nil {
			m.logger.Error("failed to restore media after seek", "chat_id", chatID, "error", restoreErr)
		}
		return fmt.Errorf("seek media source: %w", err)
	}
	session.description = description
	return nil
}

func (m *Manager) Pause(ctx context.Context, chatID int64) error {
	session, err := m.session(chatID)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = session.assistant.Calls.Pause(chatID)
	return err
}

func (m *Manager) Resume(ctx context.Context, chatID int64) error {
	session, err := m.session(chatID)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = session.assistant.Calls.Resume(chatID)
	return err
}

func (m *Manager) Stop(ctx context.Context, chatID int64) error {
	m.mu.Lock()
	session := m.sessions[chatID]
	delete(m.sessions, chatID)
	m.mu.Unlock()
	if session == nil {
		return nil
	}
	var result error
	if err := session.assistant.Calls.Stop(chatID); err != nil {
		result = errors.Join(result, fmt.Errorf("stop native call: %w", err))
	}
	if session.groupCall != nil {
		if err := session.assistant.Client.LeaveGroupCall(*session.groupCall, 0); err != nil {
			result = errors.Join(result, fmt.Errorf("leave Telegram group call: %w", err))
		}
	}
	return errors.Join(result, ctx.Err())
}

func (m *Manager) Close(ctx context.Context) error {
	m.mu.RLock()
	chatIDs := make([]int64, 0, len(m.sessions))
	for chatID := range m.sessions {
		chatIDs = append(chatIDs, chatID)
	}
	assistants := make([]*Assistant, 0, len(m.assistants))
	for _, assistant := range m.assistants {
		assistants = append(assistants, assistant)
	}
	m.mu.RUnlock()
	var result error
	for _, chatID := range chatIDs {
		result = errors.Join(result, m.Stop(ctx, chatID))
	}
	for _, assistant := range assistants {
		assistant.Calls.Free()
		assistant.Client.Stop()
	}
	return result
}

// LeaveChat stops any active call in the given chat and tells the assistant
// to leave. The call is always torn down; the assistant leave is best-effort.
func (m *Manager) LeaveChat(assistantIdx int, chatID int64) error {
	_ = m.Stop(context.Background(), chatID)
	m.mu.RLock()
	assistant, ok := m.assistants[assistantIdx]
	m.mu.RUnlock()
	if !ok || assistant == nil || assistant.Client == nil {
		return errors.New("assistant is not available")
	}
	if err := assistant.Client.LeaveChannel(chatID); err != nil {
		return fmt.Errorf("leave chat: %w", err)
	}
	m.forgetChat(chatID)
	return nil
}

func (m *Manager) session(chatID int64) (*callSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session := m.sessions[chatID]
	if session == nil {
		return nil, errors.New("voice chat is not connected")
	}
	return session, nil
}

func (m *Manager) selectAssistant(chatID int64) (*Assistant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.assistants) == 0 {
		return nil, errors.New("no assistant accounts are available")
	}
	loads := make(map[int]int, len(m.assistants))
	for index := range m.assistants {
		loads[index] = 0
	}
	for _, session := range m.sessions {
		loads[session.assistant.Index]++
	}
	bestIndex, bestLoad := 0, int(^uint(0)>>1)
	for index, load := range loads {
		if load < bestLoad || (load == bestLoad && index < bestIndex) {
			bestIndex, bestLoad = index, load
		}
	}
	return m.assistants[bestIndex], nil
}

func (m *Manager) ensureAssistantInChat(assistant *Assistant, chatID int64) error {
	normalized := normalizeChatID(chatID)
	if _, err := assistant.Client.ResolvePeer(normalized); err == nil {
		return nil
	}
	botPeer, err := m.bot.ResolvePeer(normalized)
	if err != nil {
		return fmt.Errorf("resolve chat as bot: %w", err)
	}
	user := &telegram.InputUserObj{UserID: assistant.User.ID, AccessHash: assistant.User.AccessHash}
	switch peer := botPeer.(type) {
	case *telegram.InputPeerChannel:
		_, err = m.bot.ChannelsInviteToChannel(&telegram.InputChannelObj{ChannelID: peer.ChannelID, AccessHash: peer.AccessHash}, []telegram.InputUser{user})
	case *telegram.InputPeerChat:
		_, err = m.bot.MessagesAddChatUser(peer.ChatID, user, 0)
	}
	if err == nil {
		return nil
	}

	exported, exportErr := m.bot.GetChatInviteLink(botPeer, &telegram.InviteLinkOptions{Expire: int32(time.Now().Add(time.Hour).Unix()), Limit: 1})
	if exportErr != nil {
		return fmt.Errorf("invite assistant: %v; export invite: %w", err, exportErr)
	}
	invite, ok := exported.(*telegram.ChatInviteExported)
	if !ok || invite.Link == "" {
		return errors.New("Telegram returned an invalid assistant invite link")
	}
	if _, joinErr := assistant.Client.JoinChannel(invite.Link); joinErr != nil && !telegram.MatchError(joinErr, "USER_ALREADY_PARTICIPANT") {
		return fmt.Errorf("assistant join chat: %w", joinErr)
	}
	return nil
}

func (m *Manager) getGroupCall(assistant *Assistant, chatID int64) (*telegram.InputGroupCall, error) {
	peer, err := assistant.Client.ResolvePeer(normalizeChatID(chatID))
	if err != nil {
		return nil, fmt.Errorf("resolve chat as assistant: %w", err)
	}
	switch peer := peer.(type) {
	case *telegram.InputPeerChannel:
		full, err := assistant.Client.ChannelsGetFullChannel(&telegram.InputChannelObj{ChannelID: peer.ChannelID, AccessHash: peer.AccessHash})
		if err != nil {
			return nil, err
		}
		if channel, ok := full.FullChat.(*telegram.ChannelFull); ok && channel.Call != nil {
			return &channel.Call, nil
		}
	case *telegram.InputPeerChat:
		full, err := assistant.Client.MessagesGetFullChat(peer.ChatID)
		if err != nil {
			return nil, err
		}
		if chat, ok := full.FullChat.(*telegram.ChatFullObj); ok && chat.Call != nil {
			return &chat.Call, nil
		}
	}
	return nil, errors.New("no active voice chat; start a group voice chat first")
}

func mediaDescription(input string, video, live bool, seekOffset time.Duration) ntg.MediaDescription {
	reconnect := ""
	if live || strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		reconnect = "-reconnect 1 -reconnect_at_eof 1 -reconnect_streamed 1 -reconnect_delay_max 2 "
	}
	quoted := shellQuote(input)
	seekPrefix := ""
	if seekOffset > 0 {
		seekPrefix = "-ss " + shellQuote(strconv.FormatFloat(seekOffset.Seconds(), 'f', 3, 64)) + " "
	}
	audio := "ffmpeg -nostdin -hide_banner -loglevel error " + reconnect + seekPrefix + "-i " + quoted + " -f s16le -ac 2 -ar 48000 pipe:1"
	description := ntg.MediaDescription{Microphone: &ntg.AudioDescription{MediaSource: ntg.MediaSourceShell, Input: audio, SampleRate: 48000, ChannelCount: 2}}
	if video {
		videoCommand := "ffmpeg -nostdin -hide_banner -loglevel error " + reconnect + seekPrefix + "-i " + quoted + " -f rawvideo -pix_fmt yuv420p -r 30 -vf scale=1280:720 pipe:1"
		description.Camera = &ntg.VideoDescription{MediaSource: ntg.MediaSourceShell, Input: videoCommand, Width: 1280, Height: 720, Fps: 30}
	}
	return description
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
func normalizeChatID(id int64) int64 {
	if id > 0 {
		return -1_000_000_000_000 - id
	}
	return id
}

func extractConnectionJSON(updates telegram.Updates) string {
	switch value := updates.(type) {
	case *telegram.UpdatesObj:
		for _, update := range value.Updates {
			if connection, ok := update.(*telegram.UpdateGroupCallConnection); ok && connection.Params != nil {
				return connection.Params.Data
			}
		}
	case *telegram.UpdatesCombined:
		for _, update := range value.Updates {
			if connection, ok := update.(*telegram.UpdateGroupCallConnection); ok && connection.Params != nil {
				return connection.Params.Data
			}
		}
	}
	return ""
}
