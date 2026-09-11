package playback

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nub-coders/nub-go-music-bot/internal/media"
)

// observerRecorder captures observer callbacks for autoplay drain assertions.
type observerRecorder struct {
	mu       sync.Mutex
	drained  []media.Track
	ended    []int64
	started  []media.Track
	failures []error
}

func (o *observerRecorder) TrackStarted(_ int64, track media.Track) {
	o.mu.Lock()
	o.started = append(o.started, track)
	o.mu.Unlock()
}

func (o *observerRecorder) QueueDrained(_ int64, track media.Track) {
	o.mu.Lock()
	o.drained = append(o.drained, track)
	o.mu.Unlock()
}

func (o *observerRecorder) QueueEnded(chatID int64) {
	o.mu.Lock()
	o.ended = append(o.ended, chatID)
	o.mu.Unlock()
}

func (o *observerRecorder) PlaybackError(_ int64, err error) {
	o.mu.Lock()
	o.failures = append(o.failures, err)
	o.mu.Unlock()
}

func (o *observerRecorder) counts() (int, int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.drained), len(o.ended)
}

// waitFor polls condition until it holds or the deadline passes.
func waitFor(t *testing.T, condition func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// TestAutoplayDrainKeepsVoiceCallConnected asserts that when the queue drains
// naturally with autoplay on, the observer is told to suggest (QueueDrained)
// rather than to end, and the voice call is left up so the next track can swap
// its stream source.
func TestAutoplayDrainKeepsVoiceCallConnected(t *testing.T) {
	voice := &fakeVoice{}
	observer := &observerRecorder{}
	service := New(fakeResolver{}, voice, nil)
	service.SetObserver(observer)
	ctx := context.Background()

	if _, err := service.Enqueue(ctx, -3001, "seed", false, 1, "u"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// Drain the only track naturally.
	service.NotifyStreamEnd(-3001)

	if !waitFor(t, func() bool {
		drained, _ := observer.counts()
		return drained == 1
	}) {
		t.Fatalf("expected QueueDrained, got drained=%v", observer.drained)
	}
	if _, ended := observer.counts(); ended != 0 {
		t.Fatalf("expected no QueueEnded on autoplay drain, got %v", observer.ended)
	}
	voice.mu.Lock()
	stops := voice.stops
	voice.mu.Unlock()
	if stops != 0 {
		t.Fatalf("autoplay drain must not stop the voice call, got %d stops", stops)
	}
	if snapshot, err := service.Snapshot(ctx, -3001); err != nil || snapshot.Current != nil {
		t.Fatalf("expected idle-but-connected session, got %+v (%v)", snapshot, err)
	}
}

// TestAutoplayDisabledDrainEndsCall asserts the disabled path still tears the
// call down and reports QueueEnded.
func TestAutoplayDisabledDrainEndsCall(t *testing.T) {
	voice := &fakeVoice{}
	observer := &observerRecorder{}
	service := New(fakeResolver{}, voice, nil)
	service.SetObserver(observer)
	service.SetAutoplay(-3002, false)
	ctx := context.Background()

	if _, err := service.Enqueue(ctx, -3002, "seed", false, 1, "u"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	service.NotifyStreamEnd(-3002)

	if !waitFor(t, func() bool {
		_, ended := observer.counts()
		return ended == 1
	}) {
		t.Fatalf("expected QueueEnded, got ended=%v", observer.ended)
	}
	voice.mu.Lock()
	stops := voice.stops
	voice.mu.Unlock()
	if stops == 0 {
		t.Fatalf("disabled autoplay must stop the voice call")
	}
}

// TestSkipOnLastTrackTriggersSuggestions asserts an explicit /skip of the final
// track behaves like a natural drain when autoplay is on.
func TestSkipOnLastTrackTriggersSuggestions(t *testing.T) {
	voice := &fakeVoice{}
	observer := &observerRecorder{}
	service := New(fakeResolver{}, voice, nil)
	service.SetObserver(observer)

	if _, err := service.Enqueue(context.Background(), -3003, "only", false, 1, "u"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	track, err := service.Skip(context.Background(), -3003)
	if err != nil {
		t.Fatalf("skip: %v", err)
	}
	if track != nil {
		t.Fatalf("expected drained skip to return no next track, got %+v", track)
	}
	if !waitFor(t, func() bool {
		drained, _ := observer.counts()
		return drained == 1
	}) {
		t.Fatalf("expected QueueDrained after skip, got drained=%v ended=%v", observer.drained, observer.ended)
	}
	voice.mu.Lock()
	stops := voice.stops
	voice.mu.Unlock()
	if stops != 0 {
		t.Fatalf("skip with autoplay must not stop the voice call, got %d stops", stops)
	}
}

// TestAutoplayEnabledDefaultsTrue documents the default-on preference.
func TestAutoplayEnabledDefaultsTrue(t *testing.T) {
	service := New(fakeResolver{}, &fakeVoice{}, nil)
	if !service.AutoplayEnabled(-3004) {
		t.Fatal("autoplay should default to enabled")
	}
	service.SetAutoplay(-3004, false)
	if service.AutoplayEnabled(-3004) {
		t.Fatal("SetAutoplay(false) should disable")
	}
}
