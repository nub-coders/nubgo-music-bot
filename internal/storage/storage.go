package storage

import (
	"context"
	"errors"
)

var ErrUnavailable = errors.New("persistent storage is unavailable")

// ChatStat is a per-chat playback statistic derived from chat_playback.
type ChatStat struct {
	ChatID     int64
	PlayCount  int64
	LastPlayed int64
}

// PlaylistTrack is one entry in a user playlist. Query is what gets enqueued
// (a URL or a search string); Title/Duration are optional display metadata.
type PlaylistTrack struct {
	Query    string `bson:"query"`
	Title    string `bson:"title,omitempty"`
	Duration int64  `bson:"duration,omitempty"` // seconds
}

// Playlist is a named, ordered set of tracks owned by one Telegram user.
type Playlist struct {
	ID     string          `bson:"id"`
	Name   string          `bson:"name"`
	Tracks []PlaylistTrack `bson:"tracks"`
}

type Access interface {
	IsSudo(ctx context.Context, botID, userID int64) (bool, error)
	IsBotAdmin(ctx context.Context, botID, userID int64) (bool, error)
	IsAuthorized(ctx context.Context, botID, chatID, userID int64) (bool, error)
	IsBlocked(ctx context.Context, botID, userID int64) (bool, error)
	AddAuthorized(ctx context.Context, botID, chatID, userID int64) error
	RemoveAuthorized(ctx context.Context, botID, chatID, userID int64) error
	AddSudo(ctx context.Context, botID, userID int64) error
	RemoveSudo(ctx context.Context, botID, userID int64) error
	ListSudo(ctx context.Context, botID int64) ([]int64, error)
	ListAuthorized(ctx context.Context, botID, chatID int64) ([]int64, error)
	AddBlocked(ctx context.Context, botID, userID int64) error
	RemoveBlocked(ctx context.Context, botID, userID int64) error
	ListBlocked(ctx context.Context, botID int64) ([]int64, error)
	SeedAdmins(ctx context.Context, botID int64, userIDs []int64) error
	RecordPlay(ctx context.Context, chatID int64) error
	TopChats(ctx context.Context, limit int) ([]ChatStat, error)
	ChatIDs(ctx context.Context) ([]int64, error)
	SetWelcome(ctx context.Context, chatID int64, text string) error
	GetWelcome(ctx context.Context, chatID int64) (string, error)
	CreatePlaylist(ctx context.Context, userID int64, name string) (Playlist, error)
	GetPlaylists(ctx context.Context, userID int64) ([]Playlist, error)
	GetPlaylistByName(ctx context.Context, userID int64, name string) (*Playlist, error)
	RenamePlaylist(ctx context.Context, userID int64, playlistID, name string) error
	DeletePlaylist(ctx context.Context, userID int64, playlistID string) error
	AddPlaylistTrack(ctx context.Context, userID int64, playlistID string, track PlaylistTrack) error
	RemovePlaylistTrack(ctx context.Context, userID int64, playlistID string, index int) error
	Close(ctx context.Context) error
}

type Noop struct{}

func (Noop) IsSudo(context.Context, int64, int64) (bool, error)              { return false, nil }
func (Noop) IsBotAdmin(context.Context, int64, int64) (bool, error)          { return false, nil }
func (Noop) IsAuthorized(context.Context, int64, int64, int64) (bool, error) { return false, nil }
func (Noop) IsBlocked(context.Context, int64, int64) (bool, error)           { return false, nil }
func (Noop) AddAuthorized(context.Context, int64, int64, int64) error        { return ErrUnavailable }
func (Noop) RemoveAuthorized(context.Context, int64, int64, int64) error     { return ErrUnavailable }
func (Noop) AddSudo(context.Context, int64, int64) error                     { return ErrUnavailable }
func (Noop) RemoveSudo(context.Context, int64, int64) error                  { return ErrUnavailable }
func (Noop) ListSudo(context.Context, int64) ([]int64, error)                { return nil, ErrUnavailable }
func (Noop) ListAuthorized(context.Context, int64, int64) ([]int64, error) {
	return nil, ErrUnavailable
}
func (Noop) AddBlocked(context.Context, int64, int64) error      { return ErrUnavailable }
func (Noop) RemoveBlocked(context.Context, int64, int64) error   { return ErrUnavailable }
func (Noop) ListBlocked(context.Context, int64) ([]int64, error) { return nil, ErrUnavailable }
func (Noop) SeedAdmins(context.Context, int64, []int64) error    { return ErrUnavailable }
func (Noop) RecordPlay(context.Context, int64) error             { return nil }
func (Noop) TopChats(context.Context, int) ([]ChatStat, error)   { return nil, ErrUnavailable }
func (Noop) ChatIDs(context.Context) ([]int64, error)            { return nil, ErrUnavailable }
func (Noop) SetWelcome(context.Context, int64, string) error     { return ErrUnavailable }
func (Noop) GetWelcome(context.Context, int64) (string, error)   { return "", ErrUnavailable }
func (Noop) CreatePlaylist(context.Context, int64, string) (Playlist, error) {
	return Playlist{}, ErrUnavailable
}
func (Noop) GetPlaylists(context.Context, int64) ([]Playlist, error) { return nil, ErrUnavailable }
func (Noop) GetPlaylistByName(context.Context, int64, string) (*Playlist, error) {
	return nil, ErrUnavailable
}
func (Noop) RenamePlaylist(context.Context, int64, string, string) error { return ErrUnavailable }
func (Noop) DeletePlaylist(context.Context, int64, string) error         { return ErrUnavailable }
func (Noop) AddPlaylistTrack(context.Context, int64, string, PlaylistTrack) error {
	return ErrUnavailable
}
func (Noop) RemovePlaylistTrack(context.Context, int64, string, int) error { return ErrUnavailable }
func (Noop) Close(context.Context) error                                   { return nil }
