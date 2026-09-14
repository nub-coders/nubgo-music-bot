package telegrambot

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/nub-coders/gogram/telegram"
	"github.com/nub-coders/nub-go-music-bot/internal/media"
)

// autoplayCountdown is how long the suggestion card waits before it auto-plays
// the top related track, matching the Python bot's 10-second countdown.
const autoplayCountdown = 10 * time.Second

// autoplayHistoryLimit bounds the per-chat recent-played ring used to keep
// autoplay from recommending the same handful of tracks over and over.
const autoplayHistoryLimit = 50

// -- recent-history bookkeeping ----------------------------------------------

// recordRecentPlay appends a video ID to the chat's autoplay exclusion ring. IDs
// already present are ignored (no duplicates), and the ring is trimmed once it
// exceeds autoplayHistoryLimit.
func (h *Handlers) recordRecentPlay(chatID int64, videoID string) {
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	history := h.recentPlayed[chatID]
	for _, existing := range history {
		if existing == videoID {
			return
		}
	}
	history = append(history, videoID)
	if len(history) > autoplayHistoryLimit {
		history = history[len(history)-autoplayHistoryLimit:]
	}
	h.recentPlayed[chatID] = history
}

// recentPlaySet copies the chat's history into an exclusion set.
func (h *Handlers) recentPlaySet(chatID int64) map[string]struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	exclude := make(map[string]struct{}, len(h.recentPlayed[chatID]))
	for _, id := range h.recentPlayed[chatID] {
		exclude[id] = struct{}{}
	}
	return exclude
}

// -- suggestion card state ----------------------------------------------------

func (h *Handlers) setAutoplayCard(chatID int64, messageID int32) {
	h.mu.Lock()
	if messageID == 0 {
		delete(h.autoplayCards, chatID)
	} else {
		h.autoplayCards[chatID] = messageID
	}
	h.mu.Unlock()
}

func (h *Handlers) autoplayCard(chatID int64) (int32, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	messageID, ok := h.autoplayCards[chatID]
	return messageID, ok
}

// setAutoplayCancel installs (or clears) the pending countdown cancel for a
// chat, cancelling any previous one first.
func (h *Handlers) setAutoplayCancel(chatID int64, cancel context.CancelFunc) {
	h.mu.Lock()
	previous, had := h.autoplayCancels[chatID]
	if cancel == nil {
		delete(h.autoplayCancels, chatID)
	} else {
		h.autoplayCancels[chatID] = cancel
	}
	h.mu.Unlock()
	if had && previous != nil {
		previous()
	}
}

// cancelAutoplay aborts a pending countdown for the chat, if any.
func (h *Handlers) cancelAutoplay(chatID int64) {
	h.setAutoplayCancel(chatID, nil)
}

func (h *Handlers) setAutoplaySuggestions(chatID int64, suggestions []media.Suggestion) {
	h.mu.Lock()
	if len(suggestions) == 0 {
		delete(h.autoplaySuggestions, chatID)
	} else {
		h.autoplaySuggestions[chatID] = suggestions
	}
	h.mu.Unlock()
}

func (h *Handlers) autoplayModel(chatID int64) ([]media.Suggestion, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	suggestions, ok := h.autoplaySuggestions[chatID]
	return suggestions, ok
}

// suggestionPrefix returns the channel-mode callback prefix ("c") when cards for
// this playback chat live in a different chat.
func (h *Handlers) suggestionPrefix(chatID int64) string {
	if h.uiChatFor(chatID) != chatID {
		return "c"
	}
	return ""
}

// -- the engine ---------------------------------------------------------------

// handleAutoplay resolves related tracks for a drained queue, posts the
// suggestion card, and after the countdown enqueues the top result. It runs on
// its own goroutine, never on the playback session's goroutine.
func (h *Handlers) handleAutoplay(chatID int64, lastTrack media.Track) {
	h.logger.Info("handleAutoplay triggered", "chat_id", chatID, "track_title", lastTrack.Title, "track_id", lastTrack.ID)
	// A chat that disabled autoplay between the drain and this call should leave
	// the voice call instead of lingering connected with nothing playing.
	if !h.player.AutoplayEnabled(chatID) {
		h.logger.Info("autoplay disabled for chat, leaving call", "chat_id", chatID)
		h.leaveDrainedCall(chatID)
		return
	}

	seedID := strings.TrimSpace(lastTrack.ID)
	if seedID == "" {
		seedID = media.ExtractYouTubeVideoID(lastTrack.OriginalInput)
	}
	if seedID == "" {
		seedID = media.ExtractYouTubeVideoID(lastTrack.StreamURL)
	}
	if seedID != "" {
		lastTrack.ID = seedID
	}
	if seedID == "" && strings.TrimSpace(lastTrack.Title) == "" {
		h.logger.Warn("autoplay: no seed video id or title available", "chat_id", chatID)
		h.leaveDrainedCall(chatID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	exclude := h.recentPlaySet(chatID)
	if lastTrack.ID != "" {
		exclude[lastTrack.ID] = struct{}{}
	}

	suggestions, err := h.related.Related(ctx, lastTrack, exclude, 5)
	if err != nil || len(suggestions) == 0 {
		h.logger.Warn("autoplay: no related suggestions", "chat_id", chatID, "error", err)
		h.leaveDrainedCall(chatID)
		return
	}

	// Remember the seed and every candidate so the next drain avoids them all.
	if lastTrack.ID != "" {
		h.recordRecentPlay(chatID, lastTrack.ID)
	}
	for _, suggestion := range suggestions {
		if suggestion.VideoID != "" {
			h.recordRecentPlay(chatID, suggestion.VideoID)
		}
	}

	uiChatID := h.uiChatFor(chatID)
	prefix := h.suggestionPrefix(chatID)
	if !h.player.AutoplayEnabled(chatID) {
		// Autoplay was switched off while the related tracks were resolving;
		// nobody is waiting for a card, so just drop the call.
		h.leaveDrainedCall(chatID)
		return
	}

	markup := suggestionButtons(suggestions, true, prefix)
	sent, err := sendRich(h.bot, uiChatID, suggestionCardText(suggestions, true), &telegram.SendOptions{ReplyMarkup: markup})
	if err != nil || sent == nil {
		h.logger.Warn("autoplay: failed to post suggestion card", "chat_id", chatID, "ui_chat_id", uiChatID, "error", err)
		h.leaveDrainedCall(chatID)
		return
	}
	h.logger.Info("autoplay suggestion card posted", "chat_id", chatID, "message_id", sent.ID, "suggestions_count", len(suggestions))
	h.setAutoplayCard(chatID, sent.ID)
	h.setAutoplaySuggestions(chatID, suggestions)

	countdownCtx, countdownCancel := context.WithCancel(context.Background())
	h.setAutoplayCancel(chatID, countdownCancel)

	timer := time.NewTimer(autoplayCountdown)
	defer timer.Stop()
	select {
	case <-countdownCtx.Done():
		// Cancelled by /autoplay off, sgtoggle, sgplay or sgstop. The call stays
		// connected so the remaining song buttons still work; whichever handler
		// cancelled owns the follow-up edit. When autoplay was just switched off,
		// re-render the card as the choose-a-song variant so no stale countdown
		// remains on screen.
		if !h.player.AutoplayEnabled(chatID) {
			if messageID, ok := h.autoplayCard(chatID); ok {
				_, _ = editRichPeer(h.bot, uiChatID, messageID, suggestionCardText(suggestions, false), &telegram.SendOptions{ReplyMarkup: suggestionButtons(suggestions, false, prefix)})
			}
		}
		return
	case <-timer.C:
	}

	h.setAutoplayCancel(chatID, nil)

	// Someone started a track while the countdown ran, or autoplay was switched
	// off without cancelling the countdown.
	if h.playingNow(chatID) {
		h.closeAutoplayCard(chatID)
		return
	}
	if !h.player.AutoplayEnabled(chatID) {
		h.closeAutoplayCard(chatID)
		h.leaveDrainedCall(chatID)
		return
	}

	top := suggestions[0]
	startCtx, startCancel := context.WithTimeout(context.Background(), h.timeout)
	defer startCancel()
	if _, err := h.player.Enqueue(startCtx, chatID, top.WatchURL(), false, 0, "Autoplay"); err != nil {
		h.logger.Warn("autoplay: failed to start suggested track", "chat_id", chatID, "error", err)
		h.closeAutoplayCard(chatID)
		h.leaveDrainedCall(chatID)
		return
	}

	if messageID, ok := h.autoplayCard(chatID); ok {
		_, _ = editRichPeer(h.bot, uiChatID, messageID, autoplayNoticeText(suggestionDisplayTitle(top)), &telegram.SendOptions{})
	}
	h.setAutoplayCard(chatID, 0)
	h.setAutoplaySuggestions(chatID, nil)
}

// leaveDrainedCall removes the now-playing card and disconnects the assistant
// from the drained chat's voice call.
func (h *Handlers) leaveDrainedCall(chatID int64) {
	h.closeNowPlaying(chatID)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := h.player.Stop(ctx, chatID); err != nil {
		h.logger.Debug("autoplay: leaving drained call failed", "chat_id", chatID, "error", err)
	}
}

// closeAutoplayCard deletes the suggestion card for a chat and clears its state.
func (h *Handlers) closeAutoplayCard(chatID int64) {
	messageID, ok := h.autoplayCard(chatID)
	h.setAutoplayCard(chatID, 0)
	h.setAutoplaySuggestions(chatID, nil)
	if ok {
		_, _ = h.bot.DeleteMessages(h.uiChatFor(chatID), []int32{messageID}, true)
	}
}

// -- rendering ----------------------------------------------------------------

func suggestionCardText(suggestions []media.Suggestion, autoplayEnabled bool) string {
	var builder strings.Builder
	builder.WriteString(richHeading(emoji(emojiMusicNotes, "🎶")+" ǫᴜᴇᴜᴇ ᴇɴᴅᴇᴅ • ᴀᴜᴛᴏᴘʟᴀʏ sᴜɢɢᴇsᴛɪᴏɴs", 2))
	if autoplayEnabled {
		builder.WriteString("<p>" + emoji(emojiLoading, "⏳") + " <i>ᴀᴜᴛᴏᴘʟᴀʏɪɴɢ #1 ɪɴ <b>" + strconv.Itoa(int(autoplayCountdown.Seconds())) + "</b>s…</i></p>")
	} else {
		builder.WriteString("<p><i>ᴄʜᴏᴏsᴇ ᴀ sᴏɴɢ ᴛᴏ ᴘʟᴀʏ ɴᴇxᴛ:</i></p>")
	}
	builder.WriteString(suggestionTable(suggestions))
	builder.WriteString("<p>" + emoji(emojiInfo, "💡") + " <i>ᴛᴀᴘ ᴀɴʏ sᴏɴɢ ᴛɪᴛʟᴇ ᴛᴏ ᴘʟᴀʏ ɪᴍᴍᴇᴅɪᴀᴛᴇʟʏ.</i></p>")
	return builder.String()
}

func suggestionTable(suggestions []media.Suggestion) string {
	rows := make([][]string, 0, len(suggestions))
	for index, suggestion := range suggestions {
		length := ""
		if suggestion.Duration > 0 {
			length = richCode(formatDuration(suggestion.Duration))
		}
		artist := ""
		if strings.TrimSpace(suggestion.Author) != "" {
			artist = "<i>" + escape(suggestion.Author) + "</i>"
		}
		title := escape(suggestionDisplayTitle(suggestion))
		titleCell := "<b>" + emoji(emojiPlay, "▶️") + " " + title + "</b>"
		rows = append(rows, []string{
			keycap(index + 1),
			titleCell,
			artist,
			length,
		})
	}
	return richTable([]string{"#", "ᴛɪᴛʟᴇ", "ᴀʀᴛɪsᴛ", "ʟᴇɴɢᴛʜ"}, rows)
}

// suggestionButtons mirrors Buttons.suggestion_markup from nub-music-bot:
// controls only (Stop, Autoplay Toggle, Close), omitting song candidates from normal buttons.
func suggestionButtons(suggestions []media.Suggestion, autoplayEnabled bool, prefix string) *telegram.ReplyInlineMarkup {
	_ = suggestions
	keyboard := telegram.NewKeyboard()
	autoplayText := "ᴀᴜᴛᴏᴘʟᴀʏ: ᴏꜰꜰ"
	if autoplayEnabled {
		autoplayText = "ᴀᴜᴛᴏᴘʟᴀʏ: ᴏɴ"
	}
	keyboard.AddRow(
		styledData("sᴛᴏᴘ", prefix+"sgstop", buttonStyle(false, true, false, emojiStop)),
		styledData(autoplayText, prefix+"sgtoggle", buttonStyle(false, false, false, emojiSettings)),
	)
	keyboard.AddRow(styledData("ᴄʟᴏsᴇ", prefix+"sgclose", buttonStyle(false, true, false, emojiClose)))
	return keyboard.Build()
}

func autoplayNoticeText(title string) string {
	return richNote(emoji(emojiPlay, "▶️") + " <b>ᴀᴜᴛᴏᴘʟᴀʏɪɴɢ:</b> <b>" + escape(title) + "</b>…")
}

func suggestionDisplayTitle(suggestion media.Suggestion) string {
	if title := strings.TrimSpace(suggestion.Title); title != "" {
		return title
	}
	return suggestion.VideoID
}

// trimButtonText keeps inline button labels within Telegram's length limit.
func trimButtonText(title string) string {
	runes := []rune(strings.TrimSpace(title))
	if len(runes) == 0 {
		return "Play"
	}
	if len(runes) > 48 {
		return string(runes[:47]) + "…"
	}
	return string(runes)
}

// keycap renders a plain keycap digit sequence ("1️⃣"), matching the Python
// bot's keycaps fallback for suggestion numbering.
func keycap(number int) string {
	var builder strings.Builder
	for _, digit := range strconv.Itoa(number) {
		builder.WriteRune(digit)
		builder.WriteRune('\uFE0F')
		builder.WriteRune('\u20E3')
	}
	return builder.String()
}

// -- callback handlers --------------------------------------------------------

// playingNow reports whether a track is playing in the chat. The read is
// bounded so a busy session goroutine can never hang the autoplay or callback
// path; an unreadable snapshot is treated as "nothing playing".
func (h *Handlers) playingNow(chatID int64) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return h.player.Current(ctx, chatID) != nil
}

// callbackPlaybackChat maps the chat a suggestion-card button was pressed in to
// the chat its playback actually belongs to (channel mode routes through the
// linked chat recorded by setUIChat).
func (h *Handlers) callbackPlaybackChat(callback *telegram.CallbackQuery) int64 {
	uiChatID := callback.ChannelID()
	h.mu.Lock()
	linked, ok := h.playbackChats[uiChatID]
	h.mu.Unlock()
	if ok && linked != uiChatID && !h.playingNow(uiChatID) {
		return linked
	}
	return uiChatID
}

func (h *Handlers) onSuggestionPlay(callback *telegram.CallbackQuery) error {
	data := callback.DataString()
	index := strings.Index(data, "sgplay_")
	if index < 0 {
		_, _ = callback.Answer("")
		return nil
	}
	videoID := strings.TrimSpace(data[index+len("sgplay_"):])
	if videoID == "" {
		_, _ = callback.Answer("")
		return nil
	}
	uiChatID := callback.ChannelID()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	allowed, err := h.auth.CanControl(ctx, uiChatID, callback.GetSenderID())
	if err != nil || !allowed {
		_, _ = callback.Answer("⛔ Only admins or authorized users can control playback.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	chatID := h.callbackPlaybackChat(callback)
	h.cancelAutoplay(chatID)

	_, _ = callback.Answer("▶️ sᴛᴀʀᴛɪɴɢ ᴘʟᴀʏʙᴀᴄᴋ…")

	if messageID, ok := h.autoplayCard(chatID); ok {
		_, _ = editRichPeer(h.bot, uiChatID, messageID, richNote(emoji(emojiPlay, "▶️")+" <b>ᴘʟᴀʏɪɴɢ sᴜɢɢᴇsᴛɪᴏɴ:</b> "+richCode(videoID)+"…"), &telegram.SendOptions{})
	}
	h.setAutoplayCard(chatID, 0)
	h.setAutoplaySuggestions(chatID, nil)

	h.recordRecentPlay(chatID, videoID)
	playCtx, playCancel := context.WithTimeout(context.Background(), h.timeout)
	defer playCancel()
	if _, err := h.player.Enqueue(playCtx, chatID, "https://www.youtube.com/watch?v="+videoID, false, 0, "Suggestion"); err != nil {
		h.logger.Warn("autoplay: failed to play suggested track", "chat_id", chatID, "video_id", videoID, "error", err)
	}
	return nil
}

func (h *Handlers) onSuggestionStop(callback *telegram.CallbackQuery) error {
	uiChatID := callback.ChannelID()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	allowed, err := h.auth.CanControl(ctx, uiChatID, callback.GetSenderID())
	if err != nil || !allowed {
		_, _ = callback.Answer("⛔ Only admins or authorized users can end the session.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	chatID := h.callbackPlaybackChat(callback)
	h.cancelAutoplay(chatID)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
	err = h.player.Stop(stopCtx, chatID)
	stopCancel()
	if err != nil {
		h.logger.Debug("autoplay: stopping drained call failed", "chat_id", chatID, "error", err)
	}

	if messageID, ok := h.autoplayCard(chatID); ok {
		_, _ = editRichPeer(h.bot, uiChatID, messageID, richNote(emoji(emojiSuccess, "✅")+" <b>sᴛʀᴇᴀᴍ ᴇɴᴅᴇᴅ sᴜᴄᴄᴇsꜰᴜʟʟʏ.</b>"), &telegram.SendOptions{})
	}
	h.setAutoplayCard(chatID, 0)
	h.setAutoplaySuggestions(chatID, nil)

	_, _ = callback.Answer("✅ sᴛʀᴇᴀᴍ ᴇɴᴅᴇᴅ sᴜᴄᴄᴇsꜰᴜʟʟʏ.")
	return nil
}

func (h *Handlers) onSuggestionToggle(callback *telegram.CallbackQuery) error {
	uiChatID := callback.ChannelID()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	allowed, err := h.auth.CanControl(ctx, uiChatID, callback.GetSenderID())
	if err != nil || !allowed {
		_, _ = callback.Answer("⛔ Only admins or authorized users can switch autoplay.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	chatID := h.callbackPlaybackChat(callback)
	newState := !h.player.AutoplayEnabled(chatID)
	h.player.SetAutoplay(chatID, newState)
	if !newState {
		h.cancelAutoplay(chatID)
	}

	if newState {
		_, _ = callback.Answer("ᴀᴜᴛᴏᴘʟᴀʏ: ᴏɴ")
	} else {
		_, _ = callback.Answer("ᴀᴜᴛᴏᴘʟᴀʏ: ᴏꜰꜰ")
	}

	// Re-render the card in place so the toggle button reflects the new state.
	suggestions, ok := h.autoplayModel(chatID)
	if !ok {
		return nil
	}
	messageID, ok := h.autoplayCard(chatID)
	if !ok {
		return nil
	}
	markup := suggestionButtons(suggestions, newState, h.suggestionPrefix(chatID))
	if _, err := editRichPeer(h.bot, uiChatID, messageID, suggestionCardText(suggestions, newState), &telegram.SendOptions{ReplyMarkup: markup}); err != nil {
		h.logger.Debug("autoplay: failed to refresh suggestion card", "chat_id", chatID, "error", err)
	}
	return nil
}

// onSuggestionClose dismisses the suggestion card without touching playback;
// the countdown (if any) keeps running, matching the Python bot's close button.
func (h *Handlers) onSuggestionClose(callback *telegram.CallbackQuery) error {
	_, _ = callback.Answer("")
	chatID := h.callbackPlaybackChat(callback)
	h.closeAutoplayCard(chatID)
	return nil
}

// -- /autoplay command --------------------------------------------------------

// autoplay implements /autoplay, /cautoplay, /suggest and /csuggest. Members may
// view the status; only admins and authorized users may switch it.
func (h *Handlers) autoplay(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	chatID := h.controlChatID(ctx, m)
	current := h.player.AutoplayEnabled(chatID)
	status := autoplayStatusText(current)

	isAdmin := false
	if allowed, err := h.auth.CanControl(ctx, chatID, m.SenderID()); err == nil && allowed {
		isAdmin = true
	}

	parts := strings.Fields(strings.TrimSpace(m.Args()))
	if len(parts) > 0 {
		switch strings.ToLower(parts[0]) {
		case "status", "check", "info":
			_, err := replyRich(m, autoplayPanel(status), htmlOptions())
			return err
		}
		if !isAdmin {
			_, err := replyRich(m, autoplayAdminOnly(status), htmlOptions())
			return err
		}
		switch strings.ToLower(parts[0]) {
		case "on", "enable", "true", "1":
			h.player.SetAutoplay(chatID, true)
			_, err := replyRich(m, autoplayEnabledText(), htmlOptions())
			return err
		case "off", "disable", "false", "0":
			h.player.SetAutoplay(chatID, false)
			h.cancelAutoplay(chatID)
			_, err := replyRich(m, autoplayDisabledText(), htmlOptions())
			return err
		default:
			_, err := replyRich(m, autoplayUsageText(status)+autoplayPanel(status), htmlOptions())
			return err
		}
	}

	// No argument: members see the status, admins get a toggle.
	if !isAdmin {
		_, err := replyRich(m, autoplayAdminOnly(status), htmlOptions())
		return err
	}
	if current {
		h.player.SetAutoplay(chatID, false)
		h.cancelAutoplay(chatID)
		_, err := replyRich(m, autoplayDisabledText(), htmlOptions())
		return err
	}
	h.player.SetAutoplay(chatID, true)
	_, err := replyRich(m, autoplayEnabledText(), htmlOptions())
	return err
}

func autoplayStatusText(enabled bool) string {
	if enabled {
		return "<b>ᴇɴᴀʙʟᴇᴅ</b>"
	}
	return "<b>ᴅɪsᴀʙʟᴇᴅ</b>"
}

func autoplayEnabledText() string {
	return richNote(emoji(emojiSuccess, "✅") + " <b>ᴀᴜᴛᴏᴘʟᴀʏ ᴇɴᴀʙʟᴇᴅ ꜰᴏʀ ᴛʜɪs ᴄʜᴀᴛ.</b>")
}

func autoplayDisabledText() string {
	return richNote(emoji(emojiWarning, "⚠️") + " <b>ᴀᴜᴛᴏᴘʟᴀʏ ᴅɪsᴀʙʟᴇᴅ ꜰᴏʀ ᴛʜɪs ᴄʜᴀᴛ.</b>")
}

func autoplayAdminOnly(status string) string {
	return richNote(emoji(emojiInfo, "ℹ️") + " <b>ᴀᴜᴛᴏᴘʟᴀʏ sᴛᴀᴛᴜs:</b> " + status + "\n<i>(Only admins &amp; auth users can switch this setting)</i>")
}

func autoplayUsageText(status string) string {
	return richNote(emoji(emojiInfo, "ℹ️") + " <b>ᴜsᴀɢᴇ:</b> " + richCode("/autoplay [on|off]") + "\n‣ <b>ᴄᴜʀʀᴇɴᴛ sᴛᴀᴛᴜs:</b> " + status)
}

// autoplayPanel mirrors the Python bot's _autoplay_panel status card.
func autoplayPanel(status string) string {
	return richHeading(emoji(emojiSettings, "⚙️")+" ᴀᴜᴛᴏᴘʟᴀʏ", 1) +
		richKVTable([][2]string{
			{emoji(emojiInfo, "ℹ️") + " sᴛᴀᴛᴜs", status},
			{emoji(emojiShield, "🛡️") + " ᴄᴀɴ sᴡɪᴛᴄʜ", "<i>ᴀᴅᴍɪɴs &amp; ᴀᴜᴛʜ ᴜsᴇʀs</i>"},
		}) +
		richDetails(
			emoji(emojiInfo, "ℹ️")+" ᴏᴘᴛɪᴏɴs",
			richTable([]string{"ᴄᴏᴍᴍᴀɴᴅ", "ᴇꜰꜰᴇᴄᴛ"}, [][]string{
				{richCode("/autoplay"), "ᴛᴏɢɢʟᴇ ᴏɴ ⇄ ᴏꜰꜰ"},
				{richCode("/autoplay on"), "ᴇɴᴀʙʟᴇ sᴜɢɢᴇsᴛᴇᴅ ᴛʀᴀᴄᴋs"},
				{richCode("/autoplay off"), "sᴛᴏᴘ ᴀꜰᴛᴇʀ ᴛʜᴇ ǫᴜᴇᴜᴇ ᴇɴᴅs"},
				{richCode("/autoplay status"), "sʜᴏᴡ ᴛʜɪs ᴄᴀʀᴅ ᴏɴʟʏ"},
			}),
			false,
		)
}
