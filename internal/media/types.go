package media

import (
	"context"
	"time"
)

type SourceKind string

const (
	SourceYouTube  SourceKind = "youtube"
	SourceDirect   SourceKind = "direct"
	SourceTelegram SourceKind = "telegram"
)

type Track struct {
	ID            string
	Title         string
	Author        string
	Duration      time.Duration
	ThumbnailURL  string
	OriginalInput string
	StreamURL     string
	Kind          SourceKind
	Video         bool
	Live          bool
	RequesterID   int64
	RequesterName string
	ResolvedAt    time.Time
}

type Resolver interface {
	Resolve(ctx context.Context, input string, video bool) (Track, error)
}
