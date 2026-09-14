package telegrambot

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nub-coders/gogram/telegram"
	"github.com/nub-coders/nub-go-music-bot/internal/media"
	"github.com/nub-coders/nub-go-music-bot/internal/playback"
	"github.com/nub-coders/nub-go-music-bot/internal/storage"
)

func itoa(value int) string { return strconv.Itoa(value) }

// progressInterval matches the Python bot's 9-second in-place refresh cadence.
const progressInterval = 9 * time.Second

var anyTagRE = regexp.MustCompile(`<[^>]+>`)

func stripHTML(s string) string {
	return anyTagRE.ReplaceAllString(s, "")
}

func nowPlayingText(snapshot playback.Snapshot) string {
	return nowPlayingTextFull(snapshot, "", false)
}

// nowPlayingTextFull renders the inline now-playing card matching Messages.PLAY
// from nub-music-bot in root.
func nowPlayingTextFull(snapshot playback.Snapshot, botUsername string, isChannel bool) string {
	current := snapshot.Current
	if current == nil {
		return ""
	}
	title := current.Title
	if title == "" {
		title = current.OriginalInput
	}
	titleFormatted := escape(title)
	videoID := ""
	if current.Kind == media.SourceYouTube && current.ID != "" {
		videoID = current.ID
	}

	displayTitle := "<b>" + titleFormatted + "</b>"
	if videoID != "" && botUsername != "" {
		displayTitle = fmt.Sprintf(`<a href="https://t.me/%s?start=vidid_%s"><b>%s</b></a>`, botUsername, videoID, titleFormatted)
	} else if link := mediaWwwLink(*current); strings.HasPrefix(link, "http") {
		displayTitle = fmt.Sprintf(`<a href="%s"><b>%s</b></a>`, escape(link), titleFormatted)
	}

	duration := "-"
	if current.Duration > 0 {
		duration = formatDuration(current.Duration)
	}

	requester := "ᴜsᴇʀ"
	if current.RequesterID > 0 {
		reqName := current.RequesterName
		if reqName == "" {
			reqName = "ᴜsᴇʀ"
		}
		requester = fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, current.RequesterID, escape(reqName))
	} else if current.RequesterName != "" {
		requester = escape(current.RequesterName)
	}

	mode := "Audio"
	if current.Video {
		mode = "Video"
	}

	imgTag := ""
	if videoID != "" {
		imgTag = fmt.Sprintf("<img src=\"https://img.youtube.com/vi/%s/hqdefault.jpg\" />\n\n", videoID)
	} else if current.ThumbnailURL != "" {
		imgTag = fmt.Sprintf("<img src=\"%s\" />\n\n", escape(current.ThumbnailURL))
	}

	prefix := ""
	if isChannel {
		prefix = "c"
	}
	plQuote := richNote(fmt.Sprintf("<tg-button callback_data=\"%sadd_to_pl\">➕ ᴀᴅᴅ ᴛᴏ ᴘʟᴀʏʟɪsᴛ</tg-button>", prefix))

	return fmt.Sprintf("%s <b>ɴᴏᴡ ᴘʟᴀʏɪɴɢ</b>\n%s<p>\n<b>‣ ᴛɪᴛʟᴇ:</b> %s<br/>\n<b>‣ ᴅᴜʀᴀᴛɪᴏɴ:</b> <code>%s</code><br/>\n<b>‣ ʀᴇǫᴜᴇsᴛᴇᴅ ʙʏ:</b> %s<br/>\n<b>‣ ᴍᴏᴅᴇ:</b> <code>%s</code>\n</p>\n\n%s",
		emoji(emojiPlay, "🎞"),
		imgTag,
		displayTitle,
		duration,
		requester,
		mode,
		plQuote,
	)
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
	return npButtonsPrefixed(snapshot, "")
}

func npButtonsPrefixed(snapshot playback.Snapshot, prefix string) *telegram.ReplyInlineMarkup {
	progress := ""
	if current := snapshot.Current; current != nil && current.Duration > 0 && !current.Live {
		progress = progressLabel(snapshot.Position, current.Duration)
	}
	return playbackButtonsPrefixed(progress, prefix)
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
	isChannel := uiChatID != chatID
	botUser := ""
	if h.bot != nil && h.bot.Me() != nil {
		botUser = h.bot.Me().Username
	}
	prefix := ""
	if isChannel {
		prefix = "c"
	}
	h.mu.Lock()
	previous, hadPrevious := h.npMessages[chatID]
	h.mu.Unlock()

	markup := npButtonsPrefixed(snapshot, prefix)

	// Send via Bot API 10.3 sendRichMessage if botToken is available
	if h.botToken != "" {
		thumbPath := ""
		if h.cacheDir != "" {
			if path, err := media.GenerateThumbnail(context.Background(), *snapshot.Current, h.cacheDir); err == nil {
				thumbPath = path
			} else if h.logger != nil {
				h.logger.Warn("generate thumbnail failed", "error", err)
			}
		}

		mode := "Audio"
		if snapshot.Current.Video {
			mode = "Video"
		}
		reqName := snapshot.Current.RequesterName
		if reqName == "" {
			reqName = "ᴜsᴇʀ"
		}

		sentID, err := sendNowPlayingRichAPI(h.botToken, uiChatID, *snapshot.Current, reqName, mode, prefix, thumbPath, markup)
		if err == nil && sentID > 0 {
			h.mu.Lock()
			h.npMessages[chatID] = sentID
			h.mu.Unlock()
			if hadPrevious && previous != sentID {
				_, _ = h.bot.DeleteMessages(uiChatID, []int32{previous}, true)
			}
			h.startProgress(chatID, *snapshot.Current)
			return
		}
		if h.logger != nil {
			h.logger.Warn("sendNowPlayingRichAPI failed, falling back to MTProto", "error", err)
		}
	}

	// Fallback to MTProto rich text card
	text := nowPlayingTextFull(snapshot, botUser, isChannel)
	if hadPrevious {
		_, err := editRichPeer(h.bot, uiChatID, previous, text, &telegram.SendOptions{ReplyMarkup: markup})
		if err == nil {
			h.startProgress(chatID, *snapshot.Current)
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
	h.startProgress(chatID, *snapshot.Current)
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
	uiChatID := h.uiChatFor(chatID)
	isChannel := uiChatID != chatID
	botUser := ""
	if h.bot != nil && h.bot.Me() != nil {
		botUser = h.bot.Me().Username
	}
	prefix := ""
	if isChannel {
		prefix = "c"
	}
	h.mu.Lock()
	messageID, exists := h.npMessages[chatID]
	h.mu.Unlock()
	if !exists {
		h.postNowPlayingLocked(chatID)
		return
	}

	// Update reply markup via Bot API if available to keep buttons smooth and intact
	if h.botToken != "" {
		if err := editBotAPIReplyMarkup(h.botToken, uiChatID, messageID, npButtonsPrefixed(snapshot, prefix)); err == nil {
			return
		}
	}

	_, err = editRichPeer(h.bot, uiChatID, messageID, nowPlayingTextFull(snapshot, botUser, isChannel), &telegram.SendOptions{ReplyMarkup: npButtonsPrefixed(snapshot, prefix)})
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

// startProgress launches the 9-second progress refresh loop until the track
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
				h.logger.Info("progress loop: no active track playing, exiting", "chat_id", chatID)
				return
			}
			if snapshot.Current.Duration <= 0 || snapshot.Current.Live {
				continue
			}

			h.mu.Lock()
			messageID, exists := h.npMessages[chatID]
			uiChatID := h.uiChatFor(chatID)
			isChannel := uiChatID != chatID
			botUser := ""
			if h.bot != nil && h.bot.Me() != nil {
				botUser = h.bot.Me().Username
			}
			h.mu.Unlock()
			if !exists || messageID <= 0 {
				h.logger.Info("progress loop: message ID not recorded yet, waiting next tick", "chat_id", chatID)
				continue
			}
			prefix := ""
			if isChannel {
				prefix = "c"
			}

			markup := npButtonsPrefixed(snapshot, prefix)
			var editErr error
			if h.botToken != "" {
				editErr = editBotAPIReplyMarkup(h.botToken, uiChatID, messageID, markup)
				if editErr != nil {
					h.logger.Warn("editBotAPIReplyMarkup failed, attempting editRichPeer", "chat_id", chatID, "message_id", messageID, "error", editErr)
					_, editErr = editRichPeer(h.bot, uiChatID, messageID, nowPlayingTextFull(snapshot, botUser, isChannel), &telegram.SendOptions{ReplyMarkup: markup})
				}
			} else {
				_, editErr = editRichPeer(h.bot, uiChatID, messageID, nowPlayingTextFull(snapshot, botUser, isChannel), &telegram.SendOptions{ReplyMarkup: markup})
			}

			if editErr != nil {
				h.logger.Warn("progress update failed on this tick", "chat_id", chatID, "message_id", messageID, "error", editErr)
			} else {
				h.logger.Info("progress updated successfully", "chat_id", chatID, "message_id", messageID, "position", snapshot.Position.String())
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
	data := callback.DataString()
	action := strings.TrimPrefix(data, "np:")
	if strings.HasPrefix(action, "c") && len(action) > 1 && (strings.HasPrefix(action, "cresume") || strings.HasPrefix(action, "cpause") || strings.HasPrefix(action, "cskip") || strings.HasPrefix(action, "cend") || strings.HasPrefix(action, "cstop") || strings.HasPrefix(action, "cadd_to_pl") || strings.HasPrefix(action, "cplaynow_") || strings.HasPrefix(action, "cclose") || strings.HasPrefix(action, "cnoop")) {
		action = strings.TrimPrefix(action, "c")
	}
	uiChatID := callback.ChannelID()
	userID := callback.GetSenderID()

	if action == "noop" {
		_, _ = callback.Answer("")
		return nil
	}

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

	if action == "close" {
		_, _ = callback.Delete()
		h.closeNowPlaying(chatID)
		_, _ = callback.Answer("")
		return nil
	}

	if action == "add_to_pl" {
		return h.handleAddToPlaylist(callback, chatID, uiChatID, userID)
	}

	allowed, err := h.auth.CanControl(ctx, uiChatID, userID)
	if err != nil || !allowed {
		_, _ = callback.Answer("⛔ Only admins or authorized users can control playback.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	reqName := "User"
	if sender, err := callback.GetSender(); err == nil && sender != nil && strings.TrimSpace(sender.FirstName) != "" {
		reqName = strings.TrimSpace(sender.FirstName)
	}

	switch {
	case action == "pause" || action == "resume":
		paused := action == "pause"
		if err := h.player.Pause(ctx, chatID, paused); err != nil {
			_, _ = callback.Answer("❌ " + err.Error())
			return nil
		}
		if paused {
			_, _ = callback.Answer(stripHTML(msgPaused(reqName)))
		} else {
			_, _ = callback.Answer(stripHTML(msgResumed(reqName)))
		}
		h.refreshNowPlaying(chatID)
	case action == "skip":
		track, err := h.player.Skip(ctx, chatID)
		if err != nil {
			_, _ = callback.Answer("❌ " + err.Error())
			return nil
		}
		if track == nil {
			_, _ = callback.Answer(stripHTML(msgSkippedEmpty(reqName)))
			h.closeNowPlaying(chatID)
			return nil
		}
		_, _ = callback.Answer(stripHTML(msgSkipping(reqName)))
		h.refreshNowPlaying(chatID)
	case action == "loop":
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
		_, _ = callback.Answer(stripHTML(msgLooped([]string{"off", "track", "queue"}[next], reqName)))
		h.refreshNowPlaying(chatID)
	case action == "stop" || action == "end":
		if err := h.player.Stop(ctx, chatID); err != nil {
			_, _ = callback.Answer("❌ " + err.Error())
			return nil
		}
		_, _ = callback.Answer(stripHTML(msgStopped(reqName)))
		h.closeNowPlaying(chatID)
	case strings.HasPrefix(action, "playnow_"):
		trackID := strings.TrimPrefix(action, "playnow_")
		track, err := h.player.PlayNow(ctx, chatID, trackID)
		if err != nil {
			_, _ = callback.Answer("❌ " + err.Error())
			return nil
		}
		if track != nil {
			_, _ = callback.Answer("▶️ Playing now: " + track.Title)
		} else {
			_, _ = callback.Answer("")
		}
		h.refreshNowPlaying(chatID)
	case action == "queue":
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

func (h *Handlers) handleAddToPlaylist(callback *telegram.CallbackQuery, chatID, uiChatID, userID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	snapshot, err := h.player.Snapshot(ctx, chatID)
	if err != nil || snapshot.Current == nil {
		_, _ = callback.Answer("No track currently playing.", &telegram.CallbackOptions{Alert: true})
		return nil
	}
	trackTitle := snapshot.Current.Title
	if trackTitle == "" {
		trackTitle = "Current Track"
	}

	pls, err := h.store.GetPlaylists(ctx, userID)
	if err != nil {
		_, _ = callback.Answer("❌ Error reading playlists: "+err.Error(), &telegram.CallbackOptions{Alert: true})
		return nil
	}

	botUsername := h.botUsername()

	var text string
	keyboard := telegram.NewKeyboard()

	if len(pls) == 0 {
		text = fmt.Sprintf("%s <b>ᴀᴅᴅ ᴛᴏ ᴘʟᴀʏʟɪsᴛ</b>\n\n<b>‣ ᴛʀᴀᴄᴋ:</b> <b>%s</b>\n\n%s <i>ʏᴏᴜ ᴅᴏɴ'ᴛ ʜᴀᴠᴇ ᴀɴʏ ᴘʟᴀʏʟɪsᴛs ʏᴇᴛ. ᴛᴀᴘ ʙᴇʟᴏᴡ ᴛᴏ ᴄʀᴇᴀᴛᴇ ᴏɴᴇ:</i>",
			emoji(emojiMusicNotes, "🎶"),
			escape(trackTitle),
			emoji(emojiInfo, "💡"),
		)
		keyboard.AddRow(styledData(
			"⭐ ǫᴜɪᴄᴋ-ᴄʀᴇᴀᴛᴇ 'ꜰᴀᴠᴏʀɪᴛᴇs' & ᴀᴅᴅ",
			fmt.Sprintf("pl_quick_fav_%d", chatID),
			buttonStyle(true, false, false, emojiAdd),
		))
		if botUsername != "" {
			keyboard.AddRow(styledURL(
				"➕ ᴄʀᴇᴀᴛᴇ ɴᴇᴡ ᴘʟᴀʏʟɪsᴛ",
				fmt.Sprintf("https://t.me/%s?start=newpl_%d", botUsername, chatID),
				buttonStyle(false, false, false, emojiAdd),
			))
		}
	} else {
		text = fmt.Sprintf("%s <b>ᴀᴅᴅ ᴛᴏ ᴘʟᴀʏʟɪsᴛ</b>\n\n<b>‣ ᴛʀᴀᴄᴋ:</b> <code>%s</code>\n<i>Tap any playlist button below to save this song:</i>",
			emoji(emojiQueueIcon, "📁"),
			escape(trackTitle),
		)
		for _, pl := range pls {
			name := pl.Name
			if len([]rune(name)) > 24 {
				name = string([]rune(name)[:23]) + "…"
			}
			keyboard.AddRow(styledData(
				fmt.Sprintf("%s (%d/50)", name, len(pl.Tracks)),
				fmt.Sprintf("pl_add_%s_%d", pl.ID, chatID),
				buttonStyle(false, false, false, emojiMusicNote),
			))
		}
		if len(pls) < 5 && botUsername != "" {
			keyboard.AddRow(styledURL(
				"➕ ᴄʀᴇᴀᴛᴇ ɴᴇᴡ ᴘʟᴀʏʟɪsᴛ",
				fmt.Sprintf("https://t.me/%s?start=newpl_%d", botUsername, chatID),
				buttonStyle(true, false, false, emojiAdd),
			))
		}
	}

	keyboard.AddRow(styledData("✖ ᴄʟᴏsᴇ", "close_pl_selector", buttonStyle(false, false, true, emojiClose)))

	_, _ = callback.Answer("")
	_, _ = sendRich(h.bot, uiChatID, text, &telegram.SendOptions{ReplyMarkup: keyboard.Build()})
	return nil
}

func (h *Handlers) onPlaylistAdd(callback *telegram.CallbackQuery) error {
	data := callback.DataString()
	parts := strings.Split(strings.TrimPrefix(data, "pl_add_"), "_")
	if len(parts) < 2 {
		_, _ = callback.Answer("Invalid request.", &telegram.CallbackOptions{Alert: true})
		return nil
	}
	playlistID := parts[0]
	chatID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		_, _ = callback.Answer("Invalid chat.", &telegram.CallbackOptions{Alert: true})
		return nil
	}
	userID := callback.GetSenderID()
	if userID <= 0 {
		_, _ = callback.Answer("Please run this from your own account.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	snapshot, err := h.player.Snapshot(ctx, chatID)
	if err != nil || snapshot.Current == nil {
		_, _ = callback.Answer("No track currently playing.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	pls, err := h.store.GetPlaylists(ctx, userID)
	if err != nil {
		_, _ = callback.Answer("❌ Error: "+err.Error(), &telegram.CallbackOptions{Alert: true})
		return nil
	}

	var targetPL *storage.Playlist
	for i := range pls {
		if pls[i].ID == playlistID {
			targetPL = &pls[i]
			break
		}
	}
	if targetPL == nil {
		_, _ = callback.Answer("Playlist not found.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	current := snapshot.Current
	for _, t := range targetPL.Tracks {
		if strings.EqualFold(t.Title, current.Title) || (t.Query != "" && (t.Query == current.OriginalInput || t.Query == current.StreamURL)) {
			_, _ = callback.Answer(fmt.Sprintf("⚠️ This song is already in '%s'!", targetPL.Name), &telegram.CallbackOptions{Alert: true})
			return nil
		}
	}

	if len(targetPL.Tracks) >= 50 {
		_, _ = callback.Answer(fmt.Sprintf("❌ '%s' has reached the limit of 50 tracks!", targetPL.Name), &telegram.CallbackOptions{Alert: true})
		return nil
	}

	t := storage.PlaylistTrack{
		Query:    current.OriginalInput,
		Title:    current.Title,
		Duration: int64(current.Duration / time.Second),
	}
	if t.Query == "" {
		t.Query = current.StreamURL
	}
	if t.Query == "" {
		t.Query = current.Title
	}

	if err := h.store.AddPlaylistTrack(ctx, userID, playlistID, t); err != nil {
		_, _ = callback.Answer("⚠️ "+err.Error(), &telegram.CallbackOptions{Alert: true})
		return nil
	}

	_, _ = callback.Answer(fmt.Sprintf("✅ Added '%s' to '%s'!", current.Title, targetPL.Name), &telegram.CallbackOptions{Alert: false})

	successText := fmt.Sprintf("%s <b>ᴀᴅᴅᴇᴅ ᴛᴏ ᴘʟᴀʏʟɪsᴛ</b>\n\n%s <b>ᴛʀᴀᴄᴋ:</b> <code>%s</code>\n📁 <b>ᴘʟᴀʏʟɪsᴛ:</b> <code>%s</code>",
		emoji(emojiSuccess, "✅"),
		emoji(emojiMusicNote, "🎵"),
		escape(current.Title),
		escape(targetPL.Name),
	)
	kb := telegram.NewKeyboard()
	kb.AddRow(styledData("✖ ᴄʟᴏsᴇ", "close_pl_selector", buttonStyle(false, false, true, emojiClose)))
	if msg, err := callback.GetMessage(); err == nil && msg != nil {
		_, _ = editCard(callback.Client, msg, successText, &telegram.SendOptions{ReplyMarkup: kb.Build()})
	}
	return nil
}

func (h *Handlers) onPlaylistQuickFav(callback *telegram.CallbackQuery) error {
	data := callback.DataString()
	chatIDStr := strings.TrimPrefix(data, "pl_quick_fav_")
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		_, _ = callback.Answer("Invalid chat.", &telegram.CallbackOptions{Alert: true})
		return nil
	}
	userID := callback.GetSenderID()
	if userID <= 0 {
		_, _ = callback.Answer("Please run this from your own account.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	snapshot, err := h.player.Snapshot(ctx, chatID)
	if err != nil || snapshot.Current == nil {
		_, _ = callback.Answer("No track currently playing.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	pls, err := h.store.GetPlaylists(ctx, userID)
	if err != nil {
		_, _ = callback.Answer("❌ Error: "+err.Error(), &telegram.CallbackOptions{Alert: true})
		return nil
	}

	var favPL *storage.Playlist
	for i := range pls {
		if strings.EqualFold(pls[i].Name, "Favorites") {
			favPL = &pls[i]
			break
		}
	}

	if favPL == nil {
		if len(pls) >= 5 {
			_, _ = callback.Answer("You have reached the maximum of 5 playlists.", &telegram.CallbackOptions{Alert: true})
			return nil
		}
		newPL, err := h.store.CreatePlaylist(ctx, userID, "Favorites")
		if err != nil {
			_, _ = callback.Answer("❌ Failed to create playlist: "+err.Error(), &telegram.CallbackOptions{Alert: true})
			return nil
		}
		favPL = &newPL
	}

	current := snapshot.Current
	for _, t := range favPL.Tracks {
		if strings.EqualFold(t.Title, current.Title) || (t.Query != "" && (t.Query == current.OriginalInput || t.Query == current.StreamURL)) {
			_, _ = callback.Answer("⚠️ This song is already in Favorites!", &telegram.CallbackOptions{Alert: true})
			return nil
		}
	}

	if len(favPL.Tracks) >= 50 {
		_, _ = callback.Answer("❌ Favorites has reached the limit of 50 tracks!", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	t := storage.PlaylistTrack{
		Query:    current.OriginalInput,
		Title:    current.Title,
		Duration: int64(current.Duration / time.Second),
	}
	if t.Query == "" {
		t.Query = current.StreamURL
	}
	if t.Query == "" {
		t.Query = current.Title
	}

	if err := h.store.AddPlaylistTrack(ctx, userID, favPL.ID, t); err != nil {
		_, _ = callback.Answer("⚠️ "+err.Error(), &telegram.CallbackOptions{Alert: true})
		return nil
	}

	_, _ = callback.Answer(fmt.Sprintf("✅ Added '%s' to Favorites!", current.Title), &telegram.CallbackOptions{Alert: false})

	successText := fmt.Sprintf("%s <b>ᴀᴅᴅᴇᴅ ᴛᴏ ᴘʟᴀʏʟɪsᴛ</b>\n\n%s <b>ᴛʀᴀᴄᴋ:</b> <code>%s</code>\n📁 <b>ᴘʟᴀʏʟɪsᴛ:</b> <code>Favorites</code>",
		emoji(emojiSuccess, "✅"),
		emoji(emojiMusicNote, "🎵"),
		escape(current.Title),
	)
	kb := telegram.NewKeyboard()
	kb.AddRow(styledData("✖ ᴄʟᴏsᴇ", "close_pl_selector", buttonStyle(false, false, true, emojiClose)))
	if msg, err := callback.GetMessage(); err == nil && msg != nil {
		_, _ = editCard(callback.Client, msg, successText, &telegram.SendOptions{ReplyMarkup: kb.Build()})
	}
	return nil
}

func (h *Handlers) onPlaylistCloseSelector(callback *telegram.CallbackQuery) error {
	_, _ = callback.Delete()
	_, _ = callback.Answer("")
	return nil
}
