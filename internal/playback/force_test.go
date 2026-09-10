package playback

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nub-coders/nub-go-music-bot/internal/media"
)

// failingResolver rejects one specific query so tests can assert that a
// force-play which cannot resolve leaves the running track alone.
type failingResolver struct{ bad string }

func (r failingResolver) Resolve(_ context.Context, input string, _ bool) (media.Track, error) {
	if input == r.bad {
		return media.Track{}, errors.New("cannot resolve")
	}
	return media.Track{Title: input, OriginalInput: input, StreamURL: "https://example.com/" + input, ResolvedAt: time.Now()}, nil
}

func TestForcePlayReplacesCurrentAndKeepsQueue(t *testing.T) {
	voice := &fakeVoice{}
	service := New(fakeResolver{}, voice, nil)
	ctx := context.Background()

	if _, err := service.Enqueue(ctx, -3001, "first", false, 1, "one"); err != nil {
		t.Fatalf("enqueue first: %v", err)
	}
	if _, err := service.Enqueue(ctx, -3001, "second", false, 2, "two"); err != nil {
		t.Fatalf("enqueue second: %v", err)
	}

	result, err := service.ForcePlay(ctx, -3001, "forced", false, 3, "three")
	if err != nil {
		t.Fatalf("force play: %v", err)
	}
	if !result.Started {
		t.Fatalf("force play should start immediately: %+v", result)
	}

	snapshot, err := service.Snapshot(ctx, -3001)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Current == nil || snapshot.Current.Title != "forced" {
		t.Fatalf("current track = %+v, want forced", snapshot.Current)
	}
	// The pending queue must survive, so playback continues after the
	// forced track ends.
	if len(snapshot.Queue) != 1 || snapshot.Queue[0].Title != "second" {
		t.Fatalf("queue = %+v, want [second]", snapshot.Queue)
	}
	if voice.stops != 0 {
		t.Errorf("force play tore down the call %d time(s); it should swap sources", voice.stops)
	}
}

func TestForcePlayKeepsCurrentTrackWhenResolutionFails(t *testing.T) {
	voice := &fakeVoice{}
	service := New(failingResolver{bad: "broken"}, voice, nil)
	ctx := context.Background()

	if _, err := service.Enqueue(ctx, -3002, "playing", false, 1, "one"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := service.ForcePlay(ctx, -3002, "broken", false, 2, "two"); err == nil {
		t.Fatal("force play with an unresolvable query should fail")
	}

	snapshot, err := service.Snapshot(ctx, -3002)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Current == nil || snapshot.Current.Title != "playing" {
		t.Fatalf("current track = %+v, want the original to keep playing", snapshot.Current)
	}
}

func TestForcePlayStartsPlaybackInIdleChat(t *testing.T) {
	service := New(fakeResolver{}, &fakeVoice{}, nil)
	ctx := context.Background()

	result, err := service.ForcePlay(ctx, -3003, "solo", false, 1, "one")
	if err != nil {
		t.Fatalf("force play in idle chat: %v", err)
	}
	if !result.Started {
		t.Fatalf("force play should start playback: %+v", result)
	}
}

func TestCurrentReportsRequester(t *testing.T) {
	service := New(fakeResolver{}, &fakeVoice{}, nil)
	ctx := context.Background()

	if current := service.Current(ctx, -3004); current != nil {
		t.Fatalf("idle chat reported a current track: %+v", current)
	}
	if _, err := service.Enqueue(ctx, -3004, "song", false, 42, "requester"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	current := service.Current(ctx, -3004)
	if current == nil || current.RequesterID != 42 {
		t.Fatalf("current = %+v, want requester 42", current)
	}
}
