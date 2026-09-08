package telegrambot

import (
	"context"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/nub-coders/nub-go-music-bot/internal/media"
	"github.com/nub-coders/nub-go-music-bot/internal/playback"
)

func itoa(value int) string { return strconv.Itoa(value) }

// nowPlayingText renders the inline now-playing card.
func nowPlayingText(snapshot playback.Snapshot) string {
	current := snapshot.Current
	if current == nil {
		return ""
	}
	title := current.Title
	if title == "" {
		title = current.OriginalInput
	}
	var builder strings.Builder
	if snapshot.Paused {
		builder.WriteString("⏸️ <b>Paused</b>\n")
	} else {
		builder.WriteString("▶️ <b>Now playing</b>\n")
	}
	builder.WriteString("🎵 <a href=\"" + html.EscapeString(mediaWwwLink(*current)) + "\">" + html.EscapeString(title) + "</a>")
	if current.Author != "" {
		builder.WriteString("\n👤 " + html.EscapeString(current.Author))
	}
	if current.RequesterName != "" {
		builder.WriteString("\n🗣 Requested by <code>" + html.EscapeString(current.RequesterName) + "</code>")
	}
	if current.Duration > 0 {
		bar := progressBar(snapshot.Position, current.Duration)
		if bar != "" {
			builder.WriteString("\n" + bar)
		}
		builder.WriteString(" <code>" + formatDuration(snapshot.Position) + " / " + formatDuration(current.Duration) + "</code>")
	}
	if snapshot.Loop != playback.LoopOff {
		builder.WriteString("\n🔁 <code>" + []string{"off", "track", "queue"}[snapshot.Loop] + "</code>")
	}
	if len(snapshot.Queue) > 0 {
		builder.WriteString("\n\n<b>Up next</b>")
		for index, track := range snapshot.Queue {
			if index >= 3 {
				builder.WriteString("\n…and " + itoa(len(snapshot.Queue)-index) + " more")
				break
			}
			builder.WriteString("\n" + itoa(index+1) + ". " + formatTrack(track))
		}
	}
	return builder.String()
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
	keyboard := telegram.NewKeyboard()
	toggleText, toggleData := "⏸️ Pause", "np:pause"
	if snapshot.Paused {
		toggleText, toggleData = "▶️ Resume", "np:resume"
	}
	loopLabel := "🔁 Off"
	switch snapshot.Loop {
	case playback.LoopTrack:
		loopLabel = "🔂 Track"
	case playback.LoopQueue:
		loopLabel = "🔁 Queue"
	}
	keyboard.AddRow(
		telegram.Button.Data(toggleText, toggleData),
		telegram.Button.Data("⏭️ Skip", "np:skip"),
		telegram.Button.Data(loopLabel, "np:loop"),
	)
	keyboard.AddRow(
		telegram.Button.Data("⏹️ Stop", "np:stop"),
		telegram.Button.Data("📃 Queue", "np:queue"),
	)
	return keyboard.Build()
}

// postNowPlaying sends or updates the inline control card for a chat.
func (h *Handlers) postNowPlaying(chatID int64) {
	snapshot, err := h.player.Snapshot(context.Background(), chatID)
	if err != nil || snapshot.Current == nil {
		return
	}
	h.mu.Lock()
	messageID, exists := h.npMessages[chatID]
	h.mu.Unlock()

	text := nowPlayingText(snapshot)
	if exists {
		if _, editErr := h.bot.EditMessage(chatID, messageID, text, &telegram.SendOptions{ParseMode: "html", ReplyMarkup: npButtons(snapshot)}); editErr == nil {
			return
		}
	}
	sent, sendErr := h.bot.SendMessage(chatID, text, &telegram.SendOptions{ParseMode: "html", ReplyMarkup: npButtons(snapshot)})
	if sendErr != nil {
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
	if _, editErr := h.bot.EditMessage(chatID, messageID, nowPlayingText(snapshot), &telegram.SendOptions{ParseMode: "html", ReplyMarkup: npButtons(snapshot)}); editErr != nil {
		_ = editErr
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
	h.mu.Unlock()
	if exists {
		_, _ = h.bot.DeleteMessages(chatID, []int32{messageID}, true)
	}
}

// nowPlaying is the /np command handler.
func (h *Handlers) nowPlaying(m *telegram.NewMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snapshot, err := h.player.Snapshot(ctx, m.ChannelID())
	if err != nil || snapshot.Current == nil {
		_, _ = m.Reply("Nothing is playing right now.", htmlOptions())
		return nil
	}
	if !h.requireControl(ctx, m) {
		return nil
	}
	h.postNowPlaying(m.ChannelID())
	return nil
}

// onNPButton routes inline control presses from the now-playing card.
func (h *Handlers) onNPButton(callback *telegram.CallbackQuery) error {
	action := strings.TrimPrefix(callback.DataString(), "np:")
	chatID := callback.ChannelID()
	userID := callback.GetSenderID()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	allowed, err := h.auth.CanControl(ctx, chatID, userID)
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
	case "stop":
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
		_, _ = h.bot.SendMessage(chatID, queueText(snapshot), htmlOptions())
	default:
		_, _ = callback.Answer("", nil)
	}
	return nil
}
