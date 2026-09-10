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

// nowPlayingText renders the inline now-playing card using rich helpers and
// Python-matching custom emoji HTML tags.
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
		b.WriteString(emoji(emojiPause, "⏸️") + " <b>Paused</b>\n")
	} else {
		b.WriteString(emoji(emojiNowPlaying, "▶️") + " <b>Now playing</b>\n")
	}
	b.WriteString(emoji(emojiMusicNote, "🎵") + " <a href=\"" + escape(mediaWwwLink(*current)) + "\">" + escape(title) + "</a>")
	if current.Author != "" {
		b.WriteString("\n" + emoji(emojiUser, "👤") + " " + escape(current.Author))
	}
	if current.RequesterName != "" {
		b.WriteString("\n" + emoji(emojiMic, "🗣") + " Requested by " + richCode(current.RequesterName))
	}
	if current.Duration > 0 {
		bar := progressBar(snapshot.Position, current.Duration)
		if bar != "" {
			b.WriteString("\n" + bar)
		}
		b.WriteString(" " + richCode(formatDuration(snapshot.Position)+" / "+formatDuration(current.Duration)))
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

func npButtons(snapshot playback.Snapshot) *telegram.ReplyInlineMarkup {
	return playbackButtons(snapshot.Paused)
}

// postNowPlaying sends or updates the inline control card for a chat using
// rich-first helpers with graceful fallback. Channel playback renders the card
// in the chat the command came from, not the (often unreadable) linked chat.
func (h *Handlers) postNowPlaying(chatID int64) {
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
	h.npMessages[chatID] = sent.ID
	h.mu.Unlock()
}

// refreshNowPlaying re-renders an existing card (used after control changes).
func (h *Handlers) refreshNowPlaying(chatID int64) {
	snapshot, err := h.player.Snapshot(context.Background(), chatID)
	if err != nil || snapshot.Current == nil {
		return
	}
	h.mu.Lock()
	messageID, exists := h.npMessages[chatID]
	h.mu.Unlock()
	if !exists {
		h.postNowPlaying(chatID)
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
		_, _ = callback.Answer("⛔ Only admins or authorized users can control playback.", nil)
		return nil
	}
	switch action {
	case "pause", "resume":
		paused := action == "pause"
		if err := h.player.Pause(ctx, chatID, paused); err != nil {
			_, _ = callback.Answer("❌ "+err.Error(), nil)
			return nil
		}
		_, _ = callback.Answer("", nil)
		h.refreshNowPlaying(chatID)
	case "skip":
		track, err := h.player.Skip(ctx, chatID)
		if err != nil {
			_, _ = callback.Answer("❌ "+err.Error(), nil)
			return nil
		}
		_, _ = callback.Answer("", nil)
		if track == nil {
			h.closeNowPlaying(chatID)
			return nil
		}
		h.refreshNowPlaying(chatID)
	case "loop":
		snapshot, err := h.player.Snapshot(ctx, chatID)
		if err != nil {
			_, _ = callback.Answer("❌ "+err.Error(), nil)
			return nil
		}
		next := (snapshot.Loop + 1) % 3
		if err := h.player.SetLoop(ctx, chatID, next); err != nil {
			_, _ = callback.Answer("❌ "+err.Error(), nil)
			return nil
		}
		_, _ = callback.Answer("", nil)
		h.refreshNowPlaying(chatID)
	case "stop", "close":
		if err := h.player.Stop(ctx, chatID); err != nil {
			_, _ = callback.Answer("❌ "+err.Error(), nil)
			return nil
		}
		_, _ = callback.Answer("Stopped.", nil)
		h.closeNowPlaying(chatID)
	case "queue":
		snapshot, err := h.player.Snapshot(ctx, chatID)
		if err != nil || snapshot.Current == nil {
			_, _ = callback.Answer("Queue is empty.", nil)
			return nil
		}
		_, _ = callback.Answer("", nil)
		_, _ = h.bot.SendMessage(uiChatID, queueText(snapshot), htmlOptions())
	default:
		_, _ = callback.Answer("", nil)
	}
	return nil
}
