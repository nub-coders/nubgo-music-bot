package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	innerTubeBaseURL = "https://youtubei.googleapis.com/youtubei/v1/"
	// This is YouTube's public Android client key, not a deployment secret.
	innerTubeAPIKey = "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"
)

var (
	youtubeVideoIDPattern = regexp.MustCompile(`(?:v=|/shorts/|youtu\.be/|/embed/|/live/)([A-Za-z0-9_-]{11})`)
	rawVideoIDPattern     = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
)

type InnerTubeResolver struct {
	Client *http.Client
	Guard  URLGuard
}

type innerTubeClient struct {
	Name        string
	Version     string
	Header      string
	UserAgent   string
	SDK         int
	DeviceMake  string
	DeviceModel string
	OSName      string
	OSVersion   string
}

var innerTubeClients = []innerTubeClient{
	{Name: "ANDROID", Version: "20.10.38", Header: "3", UserAgent: "com.google.android.youtube/20.10.38 (Linux; U; Android 11) gzip", SDK: 30},
	{Name: "ANDROID_VR", Version: "1.65.10", Header: "28", UserAgent: "com.google.android.apps.youtube.vr.oculus/1.65.10 (Linux; U; Android 12L) gzip", SDK: 32, DeviceMake: "Oculus", DeviceModel: "Quest 3", OSName: "Android", OSVersion: "12L"},
}

func (r *InnerTubeResolver) Resolve(ctx context.Context, input string, video bool) (Track, error) {
	videoID := extractYouTubeVideoID(input)
	if videoID == "" {
		var err error
		videoID, err = r.search(ctx, input)
		if err != nil {
			return Track{}, err
		}
	}
	var failures []error
	for _, client := range innerTubeClients {
		track, err := r.player(ctx, videoID, input, video, client)
		if err == nil {
			return track, nil
		}
		failures = append(failures, fmt.Errorf("%s: %w", client.Name, err))
	}
	return Track{}, fmt.Errorf("InnerTube resolution failed: %w", errors.Join(failures...))
}

func (r *InnerTubeResolver) search(ctx context.Context, query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", errors.New("empty InnerTube search query")
	}
	client := innerTubeClients[0]
	payload := map[string]any{"context": map[string]any{"client": innerTubeClientPayload(client)}, "query": query}
	var response map[string]any
	if err := r.post(ctx, "search", client, payload, &response); err != nil {
		return "", err
	}
	videoID := findVideoID(response)
	if videoID == "" {
		return "", errors.New("InnerTube search returned no videos")
	}
	return videoID, nil
}

func (r *InnerTubeResolver) player(ctx context.Context, videoID, original string, video bool, client innerTubeClient) (Track, error) {
	payload := map[string]any{"context": map[string]any{"client": innerTubeClientPayload(client)}, "videoId": videoID}
	var response struct {
		PlayabilityStatus struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"playabilityStatus"`
		VideoDetails struct {
			Title         string `json:"title"`
			Author        string `json:"author"`
			LengthSeconds string `json:"lengthSeconds"`
			IsLive        bool   `json:"isLive"`
			IsLiveContent bool   `json:"isLiveContent"`
			Thumbnail     struct {
				Thumbnails []struct {
					URL   string `json:"url"`
					Width int    `json:"width"`
				} `json:"thumbnails"`
			} `json:"thumbnail"`
		} `json:"videoDetails"`
		StreamingData struct {
			Formats []struct {
				URL      string `json:"url"`
				Bitrate  int64  `json:"bitrate"`
				Height   int    `json:"height"`
				MimeType string `json:"mimeType"`
			} `json:"formats"`
		} `json:"streamingData"`
	}
	if err := r.post(ctx, "player", client, payload, &response); err != nil {
		return Track{}, err
	}
	if response.PlayabilityStatus.Status != "OK" {
		return Track{}, fmt.Errorf("video is not playable (%s: %s)", response.PlayabilityStatus.Status, response.PlayabilityStatus.Reason)
	}
	var streamURL string
	var bestBitrate int64
	for _, format := range response.StreamingData.Formats {
		if format.URL == "" || format.Bitrate <= bestBitrate {
			continue
		}
		if video && format.Height > 720 {
			continue
		}
		// The formats array contains progressive/muxed streams. Both audio and
		// video readers can therefore consume the same URL.
		streamURL, bestBitrate = format.URL, format.Bitrate
	}
	if streamURL == "" {
		return Track{}, errors.New("InnerTube returned no progressive stream")
	}
	if _, err := r.Guard.Validate(ctx, streamURL); err != nil {
		return Track{}, fmt.Errorf("validate InnerTube stream: %w", err)
	}
	durationSeconds, _ := strconv.ParseInt(response.VideoDetails.LengthSeconds, 10, 64)
	thumbnail := ""
	for _, item := range response.VideoDetails.Thumbnail.Thumbnails {
		if item.URL != "" {
			thumbnail = item.URL
		}
	}
	return Track{
		ID: videoID, Title: response.VideoDetails.Title, Author: response.VideoDetails.Author,
		Duration: time.Duration(durationSeconds) * time.Second, ThumbnailURL: thumbnail,
		OriginalInput: original, StreamURL: streamURL, Kind: SourceYouTube, Video: video,
		Live:       response.VideoDetails.IsLive || response.VideoDetails.IsLiveContent,
		ResolvedAt: time.Now(),
	}, nil
}

func (r *InnerTubeResolver) post(ctx context.Context, endpoint string, client innerTubeClient, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	requestURL := innerTubeBaseURL + endpoint + "?key=" + url.QueryEscape(innerTubeAPIKey)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-YouTube-Client-Name", client.Header)
	request.Header.Set("X-YouTube-Client-Version", client.Version)
	request.Header.Set("User-Agent", client.UserAgent)
	httpClient := r.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("InnerTube %s returned HTTP %d", endpoint, response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func innerTubeClientPayload(client innerTubeClient) map[string]any {
	payload := map[string]any{"clientName": client.Name, "clientVersion": client.Version, "androidSdkVersion": client.SDK, "hl": "en", "gl": "US"}
	if client.DeviceMake != "" {
		payload["deviceMake"] = client.DeviceMake
		payload["deviceModel"] = client.DeviceModel
		payload["osName"] = client.OSName
		payload["osVersion"] = client.OSVersion
	}
	return payload
}

func extractYouTubeVideoID(value string) string {
	value = strings.TrimSpace(value)
	if rawVideoIDPattern.MatchString(value) {
		return value
	}
	match := youtubeVideoIDPattern.FindStringSubmatch(value)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func findVideoID(node any) string {
	switch value := node.(type) {
	case map[string]any:
		if id, ok := value["videoId"].(string); ok && rawVideoIDPattern.MatchString(id) {
			return id
		}
		for _, child := range value {
			if id := findVideoID(child); id != "" {
				return id
			}
		}
	case []any:
		for _, child := range value {
			if id := findVideoID(child); id != "" {
				return id
			}
		}
	}
	return ""
}
