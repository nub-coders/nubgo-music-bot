package media

import (
	"context"
	"fmt"
	"log/slog"
)

// PriorityResolver mirrors the Python bot's resolver order:
// InnerTube -> optional NUB API -> optional YouTube Data API search -> yt-dlp.
type PriorityResolver struct {
	Inner   *InnerTubeResolver
	NUB     *NUBAPIResolver
	DataAPI *YouTubeDataSearch
	YTDLP   *YTDLPResolver
	Logger  *slog.Logger
}

func (r *PriorityResolver) Resolve(ctx context.Context, input string, video bool) (Track, error) {
	if IsHTTPURL(input) && !IsYouTubeURL(input) {
		return r.YTDLP.Resolve(ctx, input, video)
	}
	logger := r.Logger
	if logger == nil {
		logger = slog.Default()
	}

	if r.Inner != nil {
		track, err := r.Inner.Resolve(ctx, input, video)
		if err == nil {
			logger.Debug("media resolved", "resolver", "innertube", "video_id", track.ID)
			return track, nil
		}
		logger.Debug("InnerTube resolution failed", "error", err)
	}
	if r.NUB != nil && r.NUB.Enabled() {
		track, err := r.NUB.Resolve(ctx, input, video)
		if err == nil {
			logger.Debug("media resolved", "resolver", "nub_api", "video_id", track.ID)
			return track, nil
		}
		logger.Debug("NUB API resolution failed", "error", err)
	}

	// The Python implementation uses YouTube Data API only after InnerTube and
	// NUB API fail. It converts the search result into a stable watch URL, then
	// retries the stream resolvers before the final yt-dlp fallback.
	if r.DataAPI != nil && r.DataAPI.Enabled() && !IsYouTubeURL(input) && extractYouTubeVideoID(input) == "" {
		metadata, err := r.DataAPI.Search(ctx, input)
		if err == nil {
			if r.Inner != nil {
				if track, innerErr := r.Inner.Resolve(ctx, metadata.OriginalInput, video); innerErr == nil {
					return mergeMetadata(track, metadata), nil
				}
			}
			if r.NUB != nil && r.NUB.Enabled() {
				if track, nubErr := r.NUB.Resolve(ctx, metadata.OriginalInput, video); nubErr == nil {
					return mergeMetadata(track, metadata), nil
				}
			}
			if track, ytdlpErr := r.YTDLP.Resolve(ctx, metadata.OriginalInput, video); ytdlpErr == nil {
				return mergeMetadata(track, metadata), nil
			}
		}
		logger.Debug("YouTube Data API search failed", "error", err)
	}

	track, err := r.YTDLP.Resolve(ctx, input, video)
	if err != nil {
		return Track{}, fmt.Errorf("all media resolvers failed: %w", err)
	}
	logger.Debug("media resolved", "resolver", "yt-dlp", "video_id", track.ID)
	return track, nil
}

func mergeMetadata(track, metadata Track) Track {
	if track.ID == "" {
		track.ID = metadata.ID
	}
	if track.Title == "" {
		track.Title = metadata.Title
	}
	if track.Author == "" {
		track.Author = metadata.Author
	}
	if track.Duration == 0 {
		track.Duration = metadata.Duration
	}
	if track.ThumbnailURL == "" {
		track.ThumbnailURL = metadata.ThumbnailURL
	}
	return track
}
