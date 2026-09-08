package playback

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nub-coders/nub-go-music-bot/internal/media"
)

type fakeResolver struct{}

func (fakeResolver) Resolve(_ context.Context, input string, _ bool) (media.Track, error) {
	return media.Track{Title: input, OriginalInput: input, StreamURL: "https://example.com/" + input, ResolvedAt: time.Now()}, nil
}

type fakeVoice struct {
	mu     sync.Mutex
	played []string
	stops  int
}

func (f *fakeVoice) Play(_ context.Context, _ int64, track media.Track) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.played = append(f.played, track.Title)
	return nil
}
func (*fakeVoice) Pause(context.Context, int64) error  { return nil }
func (*fakeVoice) Resume(context.Context, int64) error { return nil }
func (*fakeVoice) Seek(context.Context, int64, time.Duration) error {
	return nil
}
func (f *fakeVoice) Stop(context.Context, int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	return nil
}

func TestQueueTransitionsAreSerialized(t *testing.T) {
	voice := &fakeVoice{}
	service := New(fakeResolver{}, voice, nil)
	ctx := context.Background()
	first, err := service.Enqueue(ctx, -1001, "first", false, 1, "one")
	if err != nil || !first.Started {
		t.Fatalf("first enqueue: %+v, %v", first, err)
	}
	second, err := service.Enqueue(ctx, -1001, "second", false, 2, "two")
	if err != nil || second.Position != 1 {
		t.Fatalf("second enqueue: %+v, %v", second, err)
	}
	next, err := service.Skip(ctx, -1001)
	if err != nil || next == nil || next.Title != "second" {
		t.Fatalf("skip: %+v, %v", next, err)
	}
	service.NotifyStreamEnd(-1001)
	time.Sleep(20 * time.Millisecond)
	snapshot, err := service.Snapshot(ctx, -1001)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Current == nil || snapshot.Current.Title != "second" {
		t.Fatalf("stale EOF advanced replacement: %+v", snapshot)
	}
}

// resolverWithDuration returns tracks with a fixed duration so seek clamping
// tests have a known upper bound.
type resolverWithDuration struct{ duration time.Duration }

func (r resolverWithDuration) Resolve(_ context.Context, input string, _ bool) (media.Track, error) {
	return media.Track{Title: input, OriginalInput: input, Duration: r.duration, StreamURL: "https://example.com/" + input, ResolvedAt: time.Now()}, nil
}

// trackingVoice records Seek calls so tests can assert offset clamping.
type trackingVoice struct {
	*fakeVoice
	seeks    []time.Duration
	duration time.Duration
}

func (v *trackingVoice) Seek(_ context.Context, _ int64, offset time.Duration) error {
	v.seeks = append(v.seeks, offset)
	return nil
}

func TestShufflePermutesQueue(t *testing.T) {
	service := New(fakeResolver{}, &fakeVoice{}, nil)
	ctx := context.Background()
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		if _, err := service.Enqueue(ctx, -2001, name, false, 1, "u"); err != nil {
			t.Fatalf("enqueue %s: %v", name, err)
		}
	}
	if err := service.Shuffle(ctx, -2001); err != nil {
		t.Fatalf("shuffle: %v", err)
	}
	snapshot, err := service.Snapshot(ctx, -2001)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Queue) != 4 {
		t.Fatalf("queue length = %d, want 4", len(snapshot.Queue))
	}
	seen := map[string]bool{snapshot.Current.Title: true}
	for _, track := range snapshot.Queue {
		seen[track.Title] = true
	}
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		if !seen[name] {
			t.Fatalf("shuffle lost track %q", name)
		}
	}
}

func TestSeekClampsToDuration(t *testing.T) {
	voice := &trackingVoice{fakeVoice: &fakeVoice{}, duration: 100 * time.Second}
	service := New(resolverWithDuration{100 * time.Second}, voice, nil)
	ctx := context.Background()
	if _, err := service.Enqueue(ctx, -2002, "song", false, 1, "u"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := service.Seek(ctx, -2002, 50*time.Second); err != nil {
		t.Fatalf("seek 50s: %v", err)
	}
	if err := service.Seek(ctx, -2002, 999*time.Second); err != nil {
		t.Fatalf("seek beyond duration: %v", err)
	}
	if len(voice.seeks) != 2 {
		t.Fatalf("expected 2 seeks, got %d", len(voice.seeks))
	}
	if voice.seeks[0] != 50*time.Second {
		t.Errorf("seek 50s recorded as %v", voice.seeks[0])
	}
	if voice.seeks[1] != 100*time.Second {
		t.Errorf("seek beyond duration not clamped: got %v, want 1m40s", voice.seeks[1])
	}
	snapshot, err := service.Snapshot(ctx, -2002)
	if err != nil {
		t.Fatal(err)
	}
	// elapsed() adds wall-clock time since the seek, so allow minor drift.
	if snapshot.Position < 100*time.Second || snapshot.Position > 101*time.Second {
		t.Errorf("snapshot position = %v, want ~1m40s", snapshot.Position)
	}
}

func TestSeekRequiresActivePlayback(t *testing.T) {
	service := New(fakeResolver{}, &fakeVoice{}, nil)
	if err := service.Seek(context.Background(), -2003, 10*time.Second); err == nil {
		t.Fatal("seek without playback should fail")
	}
}
