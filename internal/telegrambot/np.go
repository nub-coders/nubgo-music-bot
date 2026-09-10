package telegrambot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/nub-coders/nub-go-music-bot/internal/media"
	"github.com/nub-coders/nub-go-music-bot/internal/playback"
)

func itoa(value int) string { return strconv.Itoa(value) }

// progressInterval matches the Python bot's 9-second in-place refresh cadence.
const progressInterval = 9 * time.Second

// nowPlayingText renders the inline now-playing card using rich helpers and
// Python-matching custom emoji HTML tags. Progress is deliberately absent here:
// it lives in a disabled inline button, matching the Python bot.
func nowPlayingText(snapshot playback.Snapshot) string {
	current := snapshot.Current
	if current == nil {
		return ""
	}
	title := current.Title
	if title == "" {
		title = current.OriginalInput
	}
	var b strings.Builder
	if snapshot.Paused {
		b.WriteString(emoji(emojiPause, "⏸️") + " <b>ᴘᴀᥙsᴇᴅ</b>\n")
	} else {
		b.WriteString(emoji(emojiNowPlaying, "▶️") + " <b>ɴᴏᴡ ᴘʟᴀʏɪɴɢ</b>\n")
	}
	b.WriteString(emoji(emojiMusicNote, "🎵") + " <a href=\"" + escape(mediaWwwLink(*current)) + "\">" + escape(title) + "</a>")
	if current.Author != "" {
		b.WriteString("\n" + emoji(emojiUser, "👤") + " " + escape(current.Author))
	}
	if current.Duration > 0 {
		b.WriteString("\n" + emoji(emojiInfo, "ℹ️") + " " + richCode(formatDuration(current.Duration)))
	}
	if current.RequesterName != "" {
		b.WriteString("\n" + emoji(emojiMic, "🗣") + " Requested by " + richCode(current.RequesterName))
	}
	if snapshot.Loop != playback.LoopOff {
		b.WriteString("\n" + emoji(emojiLoop, "🔁") + " " + richCode([]string{"off", "track", "queue"}[snapshot.Loop]))
	}
	if len(snapshot.Queue) > 0 {
		b.WriteString("\n\n<b>Up next</b>")
		for index, track := range snapshot.Queue {
			if index >= 3 {
				b.WriteString(fmt.Sprintf("\n…and %d more", len(snapshot.Queue)-index))
				break
			}
			b.WriteString(fmt.Sprintf("\n%d. %s", index+1, formatTrack(track)))
		}
	}
	return b.String()
}

func mediaWwwLink(track media.Track) string {
	if track.Kind == media.SourceYouTube && track.ID != "" {
		return "https://www.youtube.com/watch?v=" + track.ID
	}
	if track.OriginalInput != "" {
		return track.OriginalInput
	}
	return track.StreamURL
}

// npButtons builds the control markup, embedding the progress bar as a disabled
// button when the track has a known duration.
func npButtons(snapshot playback.Snapshot) *telegram.ReplyInlineMarkup {
	progress := ""
	if current := snapshot.Current; current != nil && current.Duration > 0 && !current.Live {
		progress = progressLabel(snapshot.Position, current.Duration)
	}
	return playbackButtons(snapshot.Paused, progress)
}

// postNowPlaying sends or updates the inline control card for a chat using
// rich-first helpers with graceful fallback. Channel playback renders the card
// in the chat the command came from, not the (often unreadable) linked chat.
//
// The per-chat lock is held across the send so that a concurrent caller (the
// TrackStarted observer racing the /play handler) cannot both observe "no card
// yet" and each post one, which produced duplicate cards.
func (h *Handlers) postNowPlaying(chatID int64) {
	lock := h.npLock(chatID)
	lock.Lock()
	defer lock.Unlock()
	h.postNowPlayingLocked(chatID)
}

func (h *Handlers) postNowPlayingLocked(chatID int64) {
	snapshot, err := h.player.Snapshot(context.Background(), chatID)
	if err != nil || snapshot.Current == nil {
		return
	}
	uiChatID := h.uiChatFor(chatID)
	h.mu.Lock()
	messageID, exists := h.npMessages[chatID]
	h.mu.Unlock()

	text := nowPlayingText(snapshot)
	markup := npButtons(snapshot)
	if exists {
		_, err := editRichPeer(h.bot, uiChatID, messageID, text, &telegram.SendOptions{ReplyMarkup: markup})
		if err == nil {
			return
		}
	}
	sent, err := sendRich(h.bot, uiChatID, text, &telegram.SendOptions{ReplyMarkup: markup})
	if err != nil {
		return
	}
	h.mu.Lock()
	previous, hadPrevious := h.npMessages[chatID]
	h.npMessages[chatID] = sent.ID
	h.mu.Unlock()
	// A stale card can survive if an edit failed above; drop it so only one
	// control card is ever live per chat.
	if hadPrevious && previous != sent.ID {
		_, _ = h.bot.DeleteMessages(uiChatID, []int32{previous}, true)
	}
}

// refreshNowPlaying re-renders an existing card (used after control changes).
func (h *Handlers) refreshNowPlaying(chatID int64) {
	lock := h.npLock(chatID)
	lock.Lock()
	defer lock.Unlock()

	snapshot, err := h.player.Snapshot(context.Background(), chatID)
	if err != nil || snapshot.Current == nil {
		return
	}
	h.mu.Lock()
	messageID, exists := h.npMessages[chatID]
	h.mu.Unlock()
	if !exists {
		h.postNowPlayingLocked(chatID)
		return
	}
	_, err = editRichPeer(h.bot, h.uiChatFor(chatID), messageID, nowPlayingText(snapshot), &telegram.SendOptions{ReplyMarkup: npButtons(snapshot)})
	if err != nil {
		h.mu.Lock()
		delete(h.npMessages, chatID)
		h.mu.Unlock()
	}
}

// closeNowPlaying removes the card (queue ended / stopped / error).
func (h *Handlers) closeNowPlaying(chatID int64) {
	h.stopProgress(chatID)

	lock := h.npLock(chatID)
	lock.Lock()
	defer lock.Unlock()

	h.mu.Lock()
	messageID, exists := h.npMessages[chatID]
	delete(h.npMessages, chatID)
	uiChatID, hasUI := h.uiChats[chatID]
	h.mu.Unlock()
	if !hasUI {
		uiChatID = chatID
	}
	if exists {
		_, _ = h.bot.DeleteMessages(uiChatID, []int32{messageID}, true)
	}
}

// startProgress runs an in-place progress-bar refresh for the chat's card,
// mirroring the Python bot's update_progress_button loop. Only the keyboard
// changes, so the edit is cheap; it stops when the track changes or the card
// disappears.
func (h *Handlers) startProgress(chatID int64, track media.Track) {
	if track.Duration <= 0 || track.Live {
		h.stopProgress(chatID)
		return
	}
	h.stopProgress(chatID)

	ctx, cancel := context.WithCancel(context.Background())
	h.mu.Lock()
	h.npProgress[chatID] = cancel
	h.mu.Unlock()

	go func() {
		defer cancel()
		ticker := time.NewTicker(progressInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			snapCtx, snapCancel := context.WithTimeout(ctx, 10*time.Second)
			snapshot, err := h.player.Snapshot(snapCtx, chatID)
			snapCancel()
			if err != nil || snapshot.Current == nil {
				return
			}
			// A new track owns its own loop.
			if snapshot.Current.ID != track.ID || snapshot.Current.StreamURL != track.StreamURL {
				return
			}
			h.mu.Lock()
			messageID, exists := h.npMessages[chatID]
			h.mu.Unlock()
			if !exists {
				return
			}
			if _, err := editRichPeer(h.bot, h.uiChatFor(chatID), messageID, nowPlayingText(snapshot), &telegram.SendOptions{ReplyMarkup: npButtons(snapshot)}); err != nil {
				return
			}
		}
	}()
}

func (h *Handlers) stopProgress(chatID int64) {
	h.mu.Lock()
	cancel, ok := h.npProgress[chatID]
	delete(h.npProgress, chatID)
	h.mu.Unlock()
	if ok {
		cancel()
	}
}

// nowPlaying is the /np command handler.
func (h *Handlers) nowPlaying(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	targetChatID := h.controlChatID(ctx, m)
	snapshot, err := h.player.Snapshot(ctx, targetChatID)
	if err != nil || snapshot.Current == nil {
		_, _ = replyRich(m, emoji(emojiWarning, "⚠️")+" <b>Nothing is playing right now.</b>", htmlOptions())
		return nil
	}
	if !h.requireControl(ctx, m) {
		return nil
	}
	h.postNowPlaying(targetChatID)
	return nil
}

// onNPButton routes inline control presses from the now-playing card.
func (h *Handlers) onNPButton(callback *telegram.CallbackQuery) error {
	action := strings.TrimPrefix(callback.DataString(), "np:")
	uiChatID := callback.ChannelID()
	userID := callback.GetSenderID()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// The card may control playback in a linked chat (/cplay).
	chatID := uiChatID
	h.mu.Lock()
	linked, hasLinked := h.playbackChats[uiChatID]
	h.mu.Unlock()
	if hasLinked && linked != uiChatID && h.player.Current(ctx, uiChatID) == nil {
		chatID = linked
	}

	allowed, err := h.auth.CanControl(ctx, uiChatID, userID)
	if err != nil || !allowed {
		_, _ = callback.Answer("⛔ Only admins or authorized users can control playback.")
		return nil
	}
	switch action {
	case "pause", "resume":
		paused := action == "pause"
		if err := h.player.Pause(ctx, chatID, paused); err != nil {
			_, _ = callback.Answer("❌ " + err.Error())
			return nil
		}
		_, _ = callback.Answer("")
		h.refreshNowPlaying(chatID)
	case "skip":
		track, err := h.player.Skip(ctx, chatID)
		if err != nil {
			_, _ = callback.Answer("❌ " + err.Error())
			return nil
		}
		_, _ = callback.Answer("")
		if track == nil {
			h.closeNowPlaying(chatID)
			return nil
		}
		h.refreshNowPlaying(chatID)
	case "loop":
		snapshot, err := h.player.Snapshot(ctx, chatID)
		if err != nil {
			_, _ = callback.Answer("❌ " + err.Error())
			return nil
		}
		next := (snapshot.Loop + 1) % 3
		if err := h.player.SetLoop(ctx, chatID, next); err != nil {
			_, _ = callback.Answer("❌ " + err.Error())
			return nil
		}
		_, _ = callback.Answer("")
		h.refreshNowPlaying(chatID)
	case "stop", "close":
		if err := h.player.Stop(ctx, chatID); err != nil {
			_, _ = callback.Answer("❌ " + err.Error())
			return nil
		}
		_, _ = callback.Answer("Stopped.")
		h.closeNowPlaying(chatID)
	case "queue":
		snapshot, err := h.player.Snapshot(ctx, chatID)
		if err != nil || snapshot.Current == nil {
			_, _ = callback.Answer("Queue is empty.")
			return nil
		}
		_, _ = callback.Answer("")
		_, _ = h.bot.SendMessage(uiChatID, queueText(snapshot), htmlOptions())
	default:
		_, _ = callback.Answer("")
	}
	return nil
}
