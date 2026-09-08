package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type YTDLPResolver struct {
	Binary      string
	CookiesFile string
	Guard       URLGuard
}

type ytdlpResult struct {
	ID                 string          `json:"id"`
	Title              string          `json:"title"`
	Uploader           string          `json:"uploader"`
	Channel            string          `json:"channel"`
	Duration           float64         `json:"duration"`
	Thumbnail          string          `json:"thumbnail"`
	URL                string          `json:"url"`
	WebpageURL         string          `json:"webpage_url"`
	LiveStatus         string          `json:"live_status"`
	IsLive             bool            `json:"is_live"`
	RequestedDownloads []ytdlpDownload `json:"requested_downloads"`
	Entries            []ytdlpResult   `json:"entries"`
}

type ytdlpDownload struct {
	URL string `json:"url"`
}

func (r *YTDLPResolver) Resolve(ctx context.Context, input string, video bool) (Track, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Track{}, errors.New("empty media query")
	}

	if IsHTTPURL(input) {
		if _, err := r.Guard.Validate(ctx, input); err != nil {
			return Track{}, err
		}
		if !IsYouTubeURL(input) {
			return Track{
				Title:         input,
				OriginalInput: input,
				StreamURL:     input,
				Kind:          SourceDirect,
				Video:         video,
				Live:          true,
				ResolvedAt:    time.Now(),
			}, nil
		}
	}

	binary := r.Binary
	if binary == "" {
		binary = "yt-dlp"
	}
	format := "bestaudio/best"
	if video {
		// NTgCalls launches separate FFmpeg readers from one URL, so select a
		// progressive format containing both audio and video.
		format = "best[height<=720][vcodec!=none][acodec!=none]/best[height<=720]"
	}
	target := input
	if !IsHTTPURL(input) {
		target = "ytsearch1:" + input
	}
	args := []string{"--dump-single-json", "--no-warnings", "--no-playlist", "--format", format}
	if r.CookiesFile != "" {
		args = append(args, "--cookies", r.CookiesFile)
	}
	args = append(args, target)

	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Track{}, errors.New("media resolution timed out")
		}
		message := strings.TrimSpace(stderr.String())
		if len(message) > 500 {
			message = message[:500]
		}
		return Track{}, fmt.Errorf("yt-dlp failed: %w: %s", err, message)
	}

	var result ytdlpResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return Track{}, fmt.Errorf("decode yt-dlp response: %w", err)
	}
	if len(result.Entries) > 0 {
		result = result.Entries[0]
	}
	streamURL := result.URL
	if len(result.RequestedDownloads) > 0 && result.RequestedDownloads[0].URL != "" {
		streamURL = result.RequestedDownloads[0].URL
	}
	if streamURL == "" {
		return Track{}, errors.New("yt-dlp returned no playable stream URL")
	}
	if _, err := r.Guard.Validate(ctx, streamURL); err != nil {
		return Track{}, fmt.Errorf("validate extracted stream URL: %w", err)
	}
	author := result.Uploader
	if author == "" {
		author = result.Channel
	}
	return Track{
		ID:            result.ID,
		Title:         result.Title,
		Author:        author,
		Duration:      time.Duration(result.Duration * float64(time.Second)),
		ThumbnailURL:  result.Thumbnail,
		OriginalInput: input,
		StreamURL:     streamURL,
		Kind:          SourceYouTube,
		Video:         video,
		Live:          result.IsLive || result.LiveStatus == "is_live",
		ResolvedAt:    time.Now(),
	}, nil
}
