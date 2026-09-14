package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type NUBAPIResolver struct {
	BaseURL string
	Token   string
	Client  *http.Client
	Guard   URLGuard

	mu            sync.Mutex
	failures      int
	cooldownUntil time.Time
}

type nubAPIResponse struct {
	Title       string          `json:"title"`
	VideoID     string          `json:"video_id"`
	Duration    json.RawMessage `json:"duration"`
	YouTubeLink string          `json:"youtube_link"`
	ChannelName string          `json:"channel_name"`
	StreamURL   string          `json:"stream_url"`
	Thumbnail   string          `json:"thumbnail"`
	IsLive      bool            `json:"is_live"`
}

func (r *NUBAPIResolver) Enabled() bool {
	return strings.TrimSpace(r.Token) != "" && strings.TrimSpace(r.BaseURL) != ""
}

func (r *NUBAPIResolver) Resolve(ctx context.Context, input string, video bool) (Track, error) {
	if !r.Enabled() {
		return Track{}, errors.New("NUB API is not configured")
	}
	if r.breakerOpen() {
		return Track{}, errors.New("NUB API circuit breaker is open")
	}
	base, err := url.Parse(strings.TrimRight(r.BaseURL, "/") + "/info")
	if err != nil {
		return Track{}, err
	}
	query := base.Query()
	query.Set("q", input)
	if video {
		query.Set("mode", "video")
	}
	base.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return Track{}, err
	}
	request.Header.Set("Authorization", "Bearer "+r.Token)
	request.Header.Set("Accept", "application/json")
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		r.recordFailure()
		return Track{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		r.recordFailure()
		return Track{}, fmt.Errorf("NUB API returned HTTP %d", response.StatusCode)
	}
	var payload nubAPIResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		r.recordFailure()
		return Track{}, err
	}
	if payload.StreamURL == "" || payload.Title == "" {
		r.recordFailure()
		return Track{}, errors.New("NUB API returned incomplete media data")
	}
	if _, err := r.Guard.Validate(ctx, payload.StreamURL); err != nil {
		r.recordFailure()
		return Track{}, fmt.Errorf("validate NUB API stream: %w", err)
	}

	// Probe stream URL to ensure it is actually accessible and not returning 403/5xx
	probeReq, probeErr := http.NewRequestWithContext(ctx, http.MethodGet, payload.StreamURL, nil)
	if probeErr == nil {
		probeReq.Header.Set("Range", "bytes=0-1024")
		probeClient := &http.Client{Timeout: 3 * time.Second}
		probeResp, probeErr := probeClient.Do(probeReq)
		if probeErr != nil || (probeResp.StatusCode >= 400 && probeResp.StatusCode != http.StatusRequestedRangeNotSatisfiable) {
			status := 0
			if probeResp != nil {
				status = probeResp.StatusCode
				probeResp.Body.Close()
			}
			r.recordFailure()
			return Track{}, fmt.Errorf("NUB API stream unreachable: status=%d err=%v", status, probeErr)
		}
		probeResp.Body.Close()
	}

	r.recordSuccess()
	return Track{
		ID: payload.VideoID, Title: payload.Title, Author: payload.ChannelName,
		Duration: parseNUBDuration(payload.Duration), ThumbnailURL: payload.Thumbnail,
		OriginalInput: input, StreamURL: payload.StreamURL, Kind: SourceYouTube,
		Video: video, Live: payload.IsLive, ResolvedAt: time.Now(),
	}, nil
}

func (r *NUBAPIResolver) breakerOpen() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return time.Now().Before(r.cooldownUntil)
}
func (r *NUBAPIResolver) recordSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures = 0
	r.cooldownUntil = time.Time{}
}
func (r *NUBAPIResolver) recordFailure() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures++
	if r.failures >= 3 {
		r.cooldownUntil = time.Now().Add(time.Minute)
	}
}

func parseNUBDuration(raw json.RawMessage) time.Duration {
	if len(raw) == 0 {
		return 0
	}
	var seconds float64
	if json.Unmarshal(raw, &seconds) == nil {
		return time.Duration(seconds * float64(time.Second))
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return 0
	}
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		return time.Duration(parsed * float64(time.Second))
	}
	parts := strings.Split(value, ":")
	var total int64
	for _, part := range parts {
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return time.Duration(total) * time.Second
}
