package voice

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func testManager() *Manager {
	return &Manager{
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		assistants: make(map[int]*Assistant),
		sessions:   make(map[int64]*callSession),
		lastPlayed: make(map[int64]time.Time),
	}
}

type stubStore struct {
	authorized map[int64][]int64
	err        error
}

func (s stubStore) ListAuthorized(_ context.Context, _, chatID int64) ([]int64, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.authorized[chatID], nil
}

func TestAutoLeaveIdleForLeavesOnlyIdleChats(t *testing.T) {
	manager := testManager()
	manager.lastPlayed[-100] = time.Now().Add(-3 * time.Hour)
	manager.lastPlayed[-200] = time.Now().Add(-time.Minute)

	opts := AutoLeaveOptions{IdleTimeout: 90 * time.Minute}

	if _, ok := manager.autoLeaveIdleFor(-100, opts); !ok {
		t.Error("chat idle for 3h should be eligible to leave")
	}
	if _, ok := manager.autoLeaveIdleFor(-200, opts); ok {
		t.Error("chat that played a minute ago must not be left")
	}
}

func TestAutoLeaveKeepsChatsWithoutPlaybackHistory(t *testing.T) {
	manager := testManager()
	// Absence of a record means "unknown", not "idle".
	if _, ok := manager.autoLeaveIdleFor(-300, AutoLeaveOptions{IdleTimeout: time.Minute}); ok {
		t.Error("chat with no observed playback must be kept")
	}
}

func TestAutoLeaveKeepsActiveLoggerAndAuthorizedChats(t *testing.T) {
	manager := testManager()
	idle := time.Now().Add(-5 * time.Hour)
	for _, chatID := range []int64{-100, -400, -500} {
		manager.lastPlayed[chatID] = idle
	}
	manager.sessions[-100] = &callSession{}

	opts := AutoLeaveOptions{
		IdleTimeout: time.Minute,
		LoggerID:    -400,
		Store:       stubStore{authorized: map[int64][]int64{-500: {7}}},
	}

	if _, ok := manager.autoLeaveIdleFor(-100, opts); ok {
		t.Error("chat with an active call must not be left")
	}
	if _, ok := manager.autoLeaveIdleFor(-400, opts); ok {
		t.Error("logger chat must not be left")
	}
	if _, ok := manager.autoLeaveIdleFor(-500, opts); ok {
		t.Error("chat with authorized users must not be left")
	}
}

func TestAutoLeaveKeepsChatWhenAuthorizationLookupFails(t *testing.T) {
	manager := testManager()
	manager.lastPlayed[-600] = time.Now().Add(-5 * time.Hour)

	opts := AutoLeaveOptions{
		IdleTimeout: time.Minute,
		Store:       stubStore{err: errors.New("mongo is down")},
	}
	if _, ok := manager.autoLeaveIdleFor(-600, opts); ok {
		t.Error("an unreadable auth list must fail safe and keep the chat")
	}
}

func TestAutoLeaveLoopReturnsWhenDisabled(t *testing.T) {
	manager := testManager()
	done := make(chan struct{})
	go func() {
		manager.AutoLeaveLoop(context.Background(), AutoLeaveOptions{Enabled: false})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("disabled auto-leave loop did not return")
	}
}

func TestAutoLeaveLoopStopsOnContextCancel(t *testing.T) {
	manager := testManager()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		manager.AutoLeaveLoop(ctx, AutoLeaveOptions{Enabled: true, IdleTimeout: time.Hour})
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("auto-leave loop ignored context cancellation")
	}
}

func TestForgetChatDropsPlaybackRecord(t *testing.T) {
	manager := testManager()
	manager.RecordPlay(-700)
	manager.forgetChat(-700)

	manager.mu.RLock()
	_, exists := manager.lastPlayed[-700]
	manager.mu.RUnlock()
	if exists {
		t.Error("leaving a chat should drop its playback record")
	}
}
