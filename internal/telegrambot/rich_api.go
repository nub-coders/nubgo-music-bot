package telegrambot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nub-coders/gogram/telegram"
	"github.com/nub-coders/nub-go-music-bot/internal/media"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

func marshalReplyMarkup(markup *telegram.ReplyInlineMarkup) map[string]any {
	if markup == nil || len(markup.Rows) == 0 {
		return nil
	}
	var inlineKeyboard [][]map[string]any
	for _, row := range markup.Rows {
		if row == nil {
			continue
		}
		var rowBtns []map[string]any
		for _, btn := range row.Buttons {
			if btn == nil {
				continue
			}
			b := map[string]any{
				"text": btn.Text,
			}
			switch t := btn.Type.(type) {
			case *telegram.InlineButtonTypeCallback:
				if len(t.Data) > 0 {
					b["callback_data"] = string(t.Data)
				} else {
					b["callback_data"] = "noop"
				}
			case *telegram.InlineButtonTypeURL:
				if t.URL != "" {
					b["url"] = t.URL
				} else {
					b["callback_data"] = "noop"
				}
			case *telegram.InlineButtonTypeCopy:
				if t.CopyText != "" {
					b["copy_text"] = t.CopyText
				} else {
					b["callback_data"] = "noop"
				}
			default:
				b["callback_data"] = "noop"
			}
			if b["callback_data"] == nil && b["url"] == nil && b["copy_text"] == nil {
				b["callback_data"] = "noop"
			}

			if btn.Style != nil {
				if btn.Style.Icon != 0 {
					b["icon_custom_emoji_id"] = fmt.Sprint(btn.Style.Icon)
				}
				if btn.Style.BgPrimary {
					b["style"] = "primary"
				} else if btn.Style.BgDanger {
					b["style"] = "danger"
				} else if btn.Style.BgSuccess {
					b["style"] = "success"
				}
			}
			rowBtns = append(rowBtns, b)
		}
		inlineKeyboard = append(inlineKeyboard, rowBtns)
	}
	return map[string]any{
		"inline_keyboard": inlineKeyboard,
	}
}

func sendNowPlayingRichAPI(
	botToken string,
	chatID int64,
	track media.Track,
	requester string,
	mode string,
	prefix string,
	thumbPath string,
	markup *telegram.ReplyInlineMarkup,
) (int32, error) {
	title := track.Title
	if title == "" {
		title = track.OriginalInput
	}
	durStr := "-"
	if track.Duration > 0 {
		durStr = formatDuration(track.Duration)
	}

	titleItem := map[string]any{
		"type": "bold",
		"text": title,
	}

	var reqItem any = requester
	if track.RequesterID > 0 {
		reqName := track.RequesterName
		if reqName == "" {
			reqName = "ᴜsᴇʀ"
		}
		reqItem = map[string]any{
			"type": "text_mention",
			"text": reqName,
			"user": map[string]any{
				"id":         track.RequesterID,
				"first_name": reqName,
				"is_bot":     false,
			},
		}
	} else if track.RequesterName != "" {
		reqItem = track.RequesterName
	}

	blocks := []any{
		map[string]any{
			"type": "heading",
			"text": []any{
				map[string]any{"type": "custom_emoji", "custom_emoji_id": fmt.Sprint(emojiNowPlaying), "alternative_text": "🎵"},
				" ɴᴏᴡ ᴘʟᴀʏɪɴɢ",
			},
			"size": 2,
		},
	}

	hasLocalThumb := false
	if thumbPath != "" {
		if _, err := os.Stat(thumbPath); err == nil {
			hasLocalThumb = true
		}
	}
	imgURL := ""
	if track.Kind == media.SourceYouTube && track.ID != "" {
		imgURL = fmt.Sprintf("https://img.youtube.com/vi/%s/hqdefault.jpg", track.ID)
	} else if track.ThumbnailURL != "" {
		imgURL = track.ThumbnailURL
	}

	if hasLocalThumb {
		blocks = append(blocks, map[string]any{
			"type": "photo",
			"photo": map[string]any{
				"type":  "photo",
				"media": "attach://thumb",
			},
		})
	} else if imgURL != "" {
		blocks = append(blocks, map[string]any{
			"type": "photo",
			"photo": map[string]any{
				"type":  "photo",
				"media": imgURL,
			},
		})
	}

	blocks = append(blocks,
		map[string]any{
			"type": "paragraph",
			"text": []any{
				map[string]any{"type": "custom_emoji", "custom_emoji_id": fmt.Sprint(emojiPlay), "alternative_text": "🎞"},
				" ",
				map[string]any{"type": "bold", "text": "ᴛɪᴛʟᴇ: "},
				titleItem,
				"\n",
				map[string]any{"type": "custom_emoji", "custom_emoji_id": fmt.Sprint(emojiBolt), "alternative_text": "⚡️"},
				" ",
				map[string]any{"type": "bold", "text": "ᴅᴜʀᴀᴛɪᴏɴ: "},
				map[string]any{"type": "code", "text": durStr},
				"\n",
				map[string]any{"type": "custom_emoji", "custom_emoji_id": fmt.Sprint(emojiUser), "alternative_text": "👤"},
				" ",
				map[string]any{"type": "bold", "text": "ʀᴇǫᴜᴇsᴛᴇᴅ ʙʏ: "},
				reqItem,
				"\n",
				map[string]any{"type": "custom_emoji", "custom_emoji_id": fmt.Sprint(emojiHeadphones), "alternative_text": "🎧"},
				" ",
				map[string]any{"type": "bold", "text": "ᴍᴏᴅᴇ: "},
				map[string]any{"type": "code", "text": mode},
			},
		},
		map[string]any{
			"type": "blockquote",
			"blocks": []any{
				map[string]any{
					"type": "paragraph",
					"text": []any{
						map[string]any{
							"type": "button",
							"button": map[string]any{
								"text": []any{
									map[string]any{"type": "custom_emoji", "custom_emoji_id": fmt.Sprint(emojiAdd), "alternative_text": "➕"},
									" ᴀᴅᴅ ᴛᴏ ᴘʟᴀʏʟɪsᴛ",
								},
								"callback_data": prefix + "add_to_pl",
							},
						},
					},
				},
			},
		},
	)

	sendOnce := func(currentBlocks []any, includeThumb bool) (int32, error) {
		bodyBuf := &bytes.Buffer{}
		writer := multipart.NewWriter(bodyBuf)
		_ = writer.WriteField("chat_id", fmt.Sprint(chatID))

		richJSON, err := json.Marshal(map[string]any{"blocks": currentBlocks})
		if err != nil {
			return 0, fmt.Errorf("marshal rich blocks: %w", err)
		}
		_ = writer.WriteField("rich_message", string(richJSON))

		if km := marshalReplyMarkup(markup); km != nil {
			markupJSON, _ := json.Marshal(km)
			_ = writer.WriteField("reply_markup", string(markupJSON))
		}

		if includeThumb && hasLocalThumb {
			if fileData, err := os.ReadFile(thumbPath); err == nil {
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="thumb"; filename="%s"`, filepath.Base(thumbPath)))
				ext := strings.ToLower(filepath.Ext(thumbPath))
				if ext == ".jpg" || ext == ".jpeg" {
					h.Set("Content-Type", "image/jpeg")
				} else {
					h.Set("Content-Type", "image/png")
				}
				if part, err := writer.CreatePart(h); err == nil {
					_, _ = part.Write(fileData)
				}
			}
		}

		_ = writer.Close()

		url := fmt.Sprintf("https://api.telegram.org/bot%s/sendRichMessage", botToken)
		req, err := http.NewRequest("POST", url, bodyBuf)
		if err != nil {
			return 0, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())

		resp, err := httpClient.Do(req)
		if err != nil {
			return 0, fmt.Errorf("do sendRichMessage: %w", err)
		}
		defer resp.Body.Close()

		respBytes, _ := io.ReadAll(resp.Body)
		var res struct {
			OK          bool   `json:"ok"`
			Description string `json:"description"`
			Result      struct {
				MessageID int32 `json:"message_id"`
			} `json:"result"`
		}
		if err := json.Unmarshal(respBytes, &res); err != nil {
			return 0, fmt.Errorf("parse response %s: %w", string(respBytes), err)
		}
		if !res.OK {
			return 0, fmt.Errorf("api error: %s", res.Description)
		}
		return res.Result.MessageID, nil
	}

	msgID, err := sendOnce(blocks, true)
	if err != nil && (hasLocalThumb || imgURL != "") {
		// If sending with photo failed, retry with text-only blocks as fallback
		var cleanBlocks []any
		for _, b := range blocks {
			if bm, ok := b.(map[string]any); ok && bm["type"] == "photo" {
				continue
			}
			cleanBlocks = append(cleanBlocks, b)
		}
		if retryID, retryErr := sendOnce(cleanBlocks, false); retryErr == nil {
			return retryID, nil
		}
	}
	return msgID, err
}

func editBotAPIReplyMarkup(botToken string, chatID int64, messageID int32, markup *telegram.ReplyInlineMarkup) error {
	km := marshalReplyMarkup(markup)
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
	}
	if km != nil {
		payload["reply_markup"] = km
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/editMessageReplyMarkup", botToken)
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)
	var res struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(respBytes, &res)
	if !res.OK {
		if strings.Contains(strings.ToLower(res.Description), "message is not modified") {
			return nil
		}
		return fmt.Errorf("bot api %d: %s", resp.StatusCode, res.Description)
	}
	return nil
}
