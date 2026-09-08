package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type YouTubeDataSearch struct {
	Keys   []string
	Client *http.Client
	next   atomic.Uint64
}

type youtubeSearchResponse struct {
	Items []struct {
		ID struct {
			VideoID string `json:"videoId"`
		} `json:"id"`
		Snippet struct {
			Title        string `json:"title"`
			ChannelTitle string `json:"channelTitle"`
			Thumbnails   struct {
				High struct {
					URL string `json:"url"`
				} `json:"high"`
			} `json:"thumbnails"`
		} `json:"snippet"`
	} `json:"items"`
}

type youtubeDetailsResponse struct {
	Items []struct {
		ContentDetails struct {
			Duration string `json:"duration"`
		} `json:"contentDetails"`
	} `json:"items"`
}

var isoDurationPattern = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?$`)

func (s *YouTubeDataSearch) Enabled() bool { return len(s.Keys) > 0 }

func (s *YouTubeDataSearch) Search(ctx context.Context, query string) (Track, error) {
	if !s.Enabled() {
		return Track{}, errors.New("YouTube Data API is not configured")
	}
	key := s.Keys[(s.next.Add(1)-1)%uint64(len(s.Keys))]
	params := url.Values{"part": {"snippet"}, "q": {query}, "maxResults": {"1"}, "type": {"video"}, "key": {key}}
	var search youtubeSearchResponse
	if err := s.get(ctx, "https://www.googleapis.com/youtube/v3/search?"+params.Encode(), &search); err != nil {
		return Track{}, err
	}
	if len(search.Items) == 0 || search.Items[0].ID.VideoID == "" {
		return Track{}, errors.New("YouTube Data API returned no videos")
	}
	item := search.Items[0]
	duration := time.Duration(0)
	detailParams := url.Values{"part": {"contentDetails"}, "id": {item.ID.VideoID}, "key": {key}}
	var details youtubeDetailsResponse
	if err := s.get(ctx, "https://www.googleapis.com/youtube/v3/videos?"+detailParams.Encode(), &details); err == nil && len(details.Items) > 0 {
		duration = parseISODuration(details.Items[0].ContentDetails.Duration)
	}
	canonical := "https://www.youtube.com/watch?v=" + item.ID.VideoID
	return Track{ID: item.ID.VideoID, Title: item.Snippet.Title, Author: item.Snippet.ChannelTitle, Duration: duration, ThumbnailURL: item.Snippet.Thumbnails.High.URL, OriginalInput: canonical, Kind: SourceYouTube}, nil
}

func (s *YouTubeDataSearch) get(ctx context.Context, requestURL string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("YouTube Data API returned HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func parseISODuration(value string) time.Duration {
	match := isoDurationPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) == 0 {
		return 0
	}
	var seconds int64
	for index, multiplier := range []int64{3600, 60, 1} {
		if match[index+1] == "" {
			continue
		}
		part, _ := strconv.ParseInt(match[index+1], 10, 64)
		seconds += part * multiplier
	}
	return time.Duration(seconds) * time.Second
}
