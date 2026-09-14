package playback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/nub-coders/nub-go-music-bot/internal/media"
)

type LoopMode int

const (
	LoopOff LoopMode = iota
	LoopTrack
	LoopQueue
)

type Voice interface {
	Play(ctx context.Context, chatID int64, track media.Track) error
	Pause(ctx context.Context, chatID int64) error
	Resume(ctx context.Context, chatID int64) error
	Stop(ctx context.Context, chatID int64) error
	Seek(ctx context.Context, chatID int64, offset time.Duration) error
}

type Observer interface {
	TrackStarted(chatID int64, track media.Track)
	QueueDrained(chatID int64, track media.Track)
	QueueEnded(chatID int64)
	PlaybackError(chatID int64, err error)
}

type EnqueueResult struct {
	Track    media.Track
	Position int
	Started  bool
}

type Snapshot struct {
	Current  *media.Track
	Queue    []media.Track
	Paused   bool
	Loop     LoopMode
	Position time.Duration // elapsed playtime of the current track (approx)
}

type Service struct {
	resolver media.Resolver
	voice    Voice
	logger   *slog.Logger
	mu       sync.RWMutex
	sessions map[int64]*chatSession
	autoplay map[int64]bool
	observer Observer
}

type chatSession struct {
	chatID         int64
	service        *Service
	commands       chan any
	current        *media.Track
	queue          []media.Track
	paused         bool
	loop           LoopMode
	stopping       bool
	ignoreEOFUntil time.Time
	currentStarted time.Time
	position       time.Duration
}

type enqueueCommand struct {
	track media.Track
	resp  chan commandResult
}
type forceCommand struct {
	track media.Track
	resp  chan commandResult
}
type playNowCommand struct {
	trackID string
	resp    chan commandResult
}
type skipCommand struct{ resp chan commandResult }
type stopCommand struct{ resp chan error }
type pauseCommand struct {
	paused bool
	resp   chan error
}
type snapshotCommand struct{ resp chan Snapshot }
type loopCommand struct {
	mode LoopMode
	resp chan error
}
type shuffleCommand struct{ resp chan error }
type seekCommand struct {
	target time.Duration
	resp   chan error
}
type streamEndEvent struct{}
type failureEvent struct{ err error }
type shutdownCommand struct{ resp chan error }

type commandResult struct {
	enqueue EnqueueResult
	track   *media.Track
	err     error
}

func New(resolver media.Resolver, voice Voice, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		resolver: resolver,
		voice:    voice,
		logger:   logger,
		sessions: make(map[int64]*chatSession),
		autoplay: make(map[int64]bool),
	}
}

func (s *Service) SetObserver(observer Observer) { s.observer = observer }

// AutoplayEnabled reports whether a chat should continue with a related track
// when its queue drains. Autoplay is enabled by default for every chat.
func (s *Service) AutoplayEnabled(chatID int64) bool {
	s.mu.RLock()
	enabled, ok := s.autoplay[chatID]
	s.mu.RUnlock()
	return !ok || enabled
}

// SetAutoplay changes the per-chat autoplay preference. A disabled chat leaves
// the voice call as soon as its queue drains; an enabled chat keeps the call
// connected so the next track can replace the stream source seamlessly.
func (s *Service) SetAutoplay(chatID int64, enabled bool) {
	s.mu.Lock()
	s.autoplay[chatID] = enabled
	s.mu.Unlock()
}

func (s *Service) Enqueue(ctx context.Context, chatID int64, input string, video bool, requesterID int64, requesterName string) (EnqueueResult, error) {
	track, err := s.resolver.Resolve(ctx, input, video)
	if err != nil {
		return EnqueueResult{}, err
	}
	track.RequesterID = requesterID
	track.RequesterName = requesterName

	resp := make(chan commandResult, 1)
	if err := s.send(ctx, chatID, enqueueCommand{track: track, resp: resp}); err != nil {
		return EnqueueResult{}, err
	}
	select {
	case result := <-resp:
		return result.enqueue, result.err
	case <-ctx.Done():
		return EnqueueResult{}, ctx.Err()
	}
}

// ForcePlay interrupts the current track and starts input immediately, keeping
// the pending queue. The replacement is resolved *before* the running track is
// touched, so a query that cannot be resolved leaves playback untouched.
func (s *Service) ForcePlay(ctx context.Context, chatID int64, input string, video bool, requesterID int64, requesterName string) (EnqueueResult, error) {
	track, err := s.resolver.Resolve(ctx, input, video)
	if err != nil {
		return EnqueueResult{}, err
	}
	track.RequesterID = requesterID
	track.RequesterName = requesterName

	resp := make(chan commandResult, 1)
	if err := s.send(ctx, chatID, forceCommand{track: track, resp: resp}); err != nil {
		return EnqueueResult{}, err
	}
	select {
	case result := <-resp:
		return result.enqueue, result.err
	case <-ctx.Done():
		return EnqueueResult{}, ctx.Err()
	}
}

func (s *Service) PlayNow(ctx context.Context, chatID int64, trackID string) (*media.Track, error) {
	cmd := playNowCommand{trackID: trackID, resp: make(chan commandResult, 1)}
	if err := s.sendExisting(ctx, chatID, cmd); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-cmd.resp:
		if res.err != nil {
			return nil, res.err
		}
		return &res.enqueue.Track, nil
	}
}

func (s *Service) Skip(ctx context.Context, chatID int64) (*media.Track, error) {
	resp := make(chan commandResult, 1)
	if err := s.sendExisting(ctx, chatID, skipCommand{resp: resp}); err != nil {
		return nil, err
	}
	select {
	case result := <-resp:
		return result.track, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Service) Stop(ctx context.Context, chatID int64) error {
	resp := make(chan error, 1)
	if err := s.sendExisting(ctx, chatID, stopCommand{resp: resp}); err != nil {
		return err
	}
	select {
	case err := <-resp:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Pause(ctx context.Context, chatID int64, paused bool) error {
	resp := make(chan error, 1)
	if err := s.sendExisting(ctx, chatID, pauseCommand{paused: paused, resp: resp}); err != nil {
		return err
	}
	select {
	case err := <-resp:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) SetLoop(ctx context.Context, chatID int64, mode LoopMode) error {
	resp := make(chan error, 1)
	if err := s.sendExisting(ctx, chatID, loopCommand{mode: mode, resp: resp}); err != nil {
		return err
	}
	select {
	case err := <-resp:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Shuffle(ctx context.Context, chatID int64) error {
	resp := make(chan error, 1)
	if err := s.sendExisting(ctx, chatID, shuffleCommand{resp: resp}); err != nil {
		return err
	}
	select {
	case err := <-resp:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Seek moves playback to an absolute offset within the current track. It is
// clamped to the track duration when known; otherwise only backward movement
// is rejected.
func (s *Service) Seek(ctx context.Context, chatID int64, target time.Duration) error {
	if target < 0 {
		target = 0
	}
	resp := make(chan error, 1)
	if err := s.sendExisting(ctx, chatID, seekCommand{target: target, resp: resp}); err != nil {
		return err
	}
	select {
	case err := <-resp:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Snapshot(ctx context.Context, chatID int64) (Snapshot, error) {
	resp := make(chan Snapshot, 1)
	if err := s.sendExisting(ctx, chatID, snapshotCommand{resp: resp}); err != nil {
		return Snapshot{}, err
	}
	select {
	case snapshot := <-resp:
		return snapshot, nil
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	}
}

// Current returns a copy of the track playing in the chat, or nil when nothing
// is playing. The read goes through the session goroutine, so it is safe to
// call from Telegram handler goroutines while playback mutates.
func (s *Service) Current(ctx context.Context, chatID int64) *media.Track {
	snapshot, err := s.Snapshot(ctx, chatID)
	if err != nil {
		return nil
	}
	return snapshot.Current
}

func normalizeChatID(id int64) int64 {
	if id > 0 {
		return -1_000_000_000_000 - id
	}
	return id
}

func (s *Service) findSession(chatID int64) *chatSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sess, ok := s.sessions[chatID]; ok {
		return sess
	}
	normID := normalizeChatID(chatID)
	if sess, ok := s.sessions[normID]; ok {
		return sess
	}
	strID := fmt.Sprintf("%d", chatID)
	if strings.HasPrefix(strID, "-100") {
		var stripped int64
		if _, err := fmt.Sscanf(strID[4:], "%d", &stripped); err == nil {
			if sess, ok := s.sessions[stripped]; ok {
				return sess
			}
			if sess, ok := s.sessions[-stripped]; ok {
				return sess
			}
		}
	}
	return nil
}

// NotifyStreamEnd is safe to call from NTgCalls callback goroutines. Unknown and
// stopped chats are ignored rather than recreating playback state.
func (s *Service) NotifyStreamEnd(chatID int64) {
	s.logger.Info("playback NotifyStreamEnd received", "chat_id", chatID)
	session := s.findSession(chatID)
	if session == nil {
		s.logger.Warn("playback NotifyStreamEnd session not found", "chat_id", chatID)
		return
	}
	select {
	case session.commands <- streamEndEvent{}:
		s.logger.Info("queued streamEndEvent", "chat_id", session.chatID)
	default:
		s.logger.Warn("dropping duplicate stream-end event", "chat_id", session.chatID)
	}
}

func (s *Service) NotifyFailure(chatID int64, err error) {
	s.logger.Warn("playback NotifyFailure received", "chat_id", chatID, "error", err)
	session := s.findSession(chatID)
	if session == nil {
		return
	}
	select {
	case session.commands <- failureEvent{err: err}:
	default:
		s.logger.Warn("dropping duplicate voice failure event", "chat_id", session.chatID)
	}
}

func (s *Service) Close(ctx context.Context) error {
	s.mu.RLock()
	sessions := make([]*chatSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.mu.RUnlock()

	var joined error
	for _, session := range sessions {
		resp := make(chan error, 1)
		select {
		case session.commands <- shutdownCommand{resp: resp}:
		case <-ctx.Done():
			return errors.Join(joined, ctx.Err())
		}
		select {
		case err := <-resp:
			joined = errors.Join(joined, err)
		case <-ctx.Done():
			return errors.Join(joined, ctx.Err())
		}
	}
	return joined
}

func (s *Service) send(ctx context.Context, chatID int64, command any) error {
	session := s.getOrCreate(chatID)
	select {
	case session.commands <- command:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) sendExisting(ctx context.Context, chatID int64, command any) error {
	s.mu.RLock()
	session := s.sessions[chatID]
	s.mu.RUnlock()
	if session == nil {
		return errors.New("no active playback in this chat")
	}
	select {
	case session.commands <- command:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) getOrCreate(chatID int64) *chatSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.sessions[chatID]; existing != nil {
		return existing
	}
	session := &chatSession{chatID: chatID, service: s, commands: make(chan any, 32)}
	s.sessions[chatID] = session
	go session.run()
	return session
}

func (s *Service) remove(chatID int64, expected *chatSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[chatID] == expected {
		delete(s.sessions, chatID)
	}
}

func (c *chatSession) run() {
	for command := range c.commands {
		switch command := command.(type) {
		case enqueueCommand:
			command.resp <- c.enqueue(command.track)
		case forceCommand:
			command.resp <- c.forcePlay(command.track)
		case playNowCommand:
			var found *media.Track
			for i, t := range c.queue {
				if t.ID == command.trackID || strings.Contains(t.OriginalInput, command.trackID) || strings.Contains(t.StreamURL, command.trackID) {
					target := t
					found = &target
					c.queue = append(c.queue[:i], c.queue[i+1:]...)
					break
				}
			}
			if found != nil {
				command.resp <- c.forcePlay(*found)
			} else {
				command.resp <- commandResult{err: errors.New("track not found in queue")}
			}
		case skipCommand:
			track, err := c.advance(false)
			command.resp <- commandResult{track: track, err: err}
		case stopCommand:
			command.resp <- c.stop()
		case pauseCommand:
			command.resp <- c.setPaused(command.paused)
		case snapshotCommand:
			command.resp <- c.snapshot()
		case loopCommand:
			if command.mode < LoopOff || command.mode > LoopQueue {
				command.resp <- errors.New("invalid loop mode")
			} else {
				c.loop = command.mode
				command.resp <- nil
			}
		case shuffleCommand:
			c.shuffle()
			command.resp <- nil
		case seekCommand:
			command.resp <- c.seek(command.target)
		case streamEndEvent:
			c.service.logger.Info("session processing streamEndEvent",
				"chat_id", c.chatID,
				"stopping", c.stopping,
				"has_current", c.current != nil,
				"ignore_eof_until", c.ignoreEOFUntil,
				"is_before_ignore", time.Now().Before(c.ignoreEOFUntil),
			)
			if c.stopping || c.current == nil || time.Now().Before(c.ignoreEOFUntil) {
				continue
			}
			if _, err := c.advance(true); err != nil {
				_ = c.stop()
				c.notifyError(err)
			}
		case failureEvent:
			if c.stopping || c.current == nil {
				continue
			}
			_ = c.stop()
			c.notifyError(command.err)
		case shutdownCommand:
			command.resp <- c.stop()
			c.service.remove(c.chatID, c)
			return
		}
	}
}

func (c *chatSession) enqueue(track media.Track) commandResult {
	if c.current != nil {
		c.queue = append(c.queue, track)
		return commandResult{enqueue: EnqueueResult{Track: track, Position: len(c.queue), Started: false}}
	}
	if err := c.start(track, false); err != nil {
		return commandResult{err: err}
	}
	return commandResult{enqueue: EnqueueResult{Track: track, Started: true}}
}

// forcePlay swaps the active track for the given one without disturbing the
// pending queue, mirroring the Python bot's force-play semantics (the queue
// keeps playing after the forced track finishes).
func (c *chatSession) forcePlay(track media.Track) commandResult {
	replacing := c.current != nil
	if err := c.start(track, replacing); err != nil {
		return commandResult{err: err}
	}
	return commandResult{enqueue: EnqueueResult{Track: track, Started: true}}
}

func (c *chatSession) shuffle() {
	// Fisher-Yates shuffle over the pending queue.
	for index := len(c.queue) - 1; index > 0; index-- {
		other := rand.Intn(index + 1)
		c.queue[index], c.queue[other] = c.queue[other], c.queue[index]
	}
}

func (c *chatSession) seek(target time.Duration) error {
	if c.current == nil {
		return errors.New("no active playback in this chat")
	}
	if c.current.Duration > 0 && target > c.current.Duration {
		target = c.current.Duration
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := c.service.voice.Seek(ctx, c.chatID, target); err != nil {
		return err
	}
	c.currentStarted = time.Now()
	c.position = target
	c.ignoreEOFUntil = time.Now().Add(500 * time.Millisecond)
	return nil
}

func (c *chatSession) advance(natural bool) (*media.Track, error) {
	if c.current == nil {
		return nil, errors.New("no active playback in this chat")
	}
	previous := *c.current
	var next *media.Track
	switch c.loop {
	case LoopTrack:
		next = &previous
	case LoopQueue:
		c.queue = append(c.queue, previous)
	}
	if next == nil && len(c.queue) > 0 {
		value := c.queue[0]
		c.queue = c.queue[1:]
		next = &value
	}
	if next == nil {
		// Both a natural track end and an explicit /skip land here (an explicit
		// /stop or /end tears the call down directly via stopCommand). When
		// autoplay is on the call stays connected and the observer is asked for a
		// related track, so the handler can swap stream sources without rejoining.
		if c.service.AutoplayEnabled(c.chatID) {
			c.current = nil
			c.queue = nil
			c.paused = false
			c.position = 0
			c.notifyQueueDrained(previous)
			return nil, nil
		}
		if err := c.stop(); err != nil {
			return nil, err
		}
		c.notifyQueueEnded()
		return nil, nil
	}
	if err := c.start(*next, true); err != nil {
		return nil, err
	}
	if !natural {
		c.ignoreEOFUntil = time.Now().Add(750 * time.Millisecond)
	}
	return next, nil
}

func (c *chatSession) start(track media.Track, replacing bool) error {
	if time.Since(track.ResolvedAt) > 10*time.Minute && track.Kind == media.SourceYouTube {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		refreshed, err := c.service.resolver.Resolve(ctx, track.OriginalInput, track.Video)
		cancel()
		if err != nil {
			return fmt.Errorf("refresh queued media URL: %w", err)
		}
		refreshed.RequesterID = track.RequesterID
		refreshed.RequesterName = track.RequesterName
		track = refreshed
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.service.voice.Play(ctx, c.chatID, track); err != nil {
		return fmt.Errorf("start track: %w", err)
	}
	c.current = &track
	c.paused = false
	c.position = 0
	c.currentStarted = time.Now()
	if replacing {
		c.ignoreEOFUntil = time.Now().Add(750 * time.Millisecond)
	}
	c.notifyTrackStarted(track)
	return nil
}

func (c *chatSession) setPaused(paused bool) error {
	if c.current == nil {
		return errors.New("no active playback in this chat")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var err error
	if paused {
		err = c.service.voice.Pause(ctx, c.chatID)
		if err == nil {
			c.position += time.Since(c.currentStarted)
		}
	} else {
		err = c.service.voice.Resume(ctx, c.chatID)
		if err == nil {
			c.currentStarted = time.Now()
		}
	}
	if err == nil {
		c.paused = paused
	}
	return err
}

// elapsed returns the approximate playhead within the current track.
func (c *chatSession) elapsed() time.Duration {
	if c.current == nil {
		return 0
	}
	if c.paused {
		return c.position
	}
	return c.position + time.Since(c.currentStarted)
}

func (c *chatSession) stop() error {
	c.stopping = true
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	err := c.service.voice.Stop(ctx, c.chatID)
	cancel()
	c.current = nil
	c.queue = nil
	c.paused = false
	c.position = 0
	c.stopping = false
	return err
}

func (c *chatSession) snapshot() Snapshot {
	var current *media.Track
	if c.current != nil {
		copy := *c.current
		current = &copy
	}
	queue := append([]media.Track(nil), c.queue...)
	return Snapshot{Current: current, Queue: queue, Paused: c.paused, Loop: c.loop, Position: c.elapsed()}
}

func (c *chatSession) notifyTrackStarted(track media.Track) {
	if observer := c.service.observer; observer != nil {
		go observer.TrackStarted(c.chatID, track)
	}
}
func (c *chatSession) notifyQueueEnded() {
	if observer := c.service.observer; observer != nil {
		go observer.QueueEnded(c.chatID)
	}
}

// notifyQueueDrained fires when autoplay is enabled and the queue drains (a
// natural track end or an explicit /skip of the last track). The caller
// (advance) already cleared current/queue; this callback lets the handler
// resolve and enqueue a related track without re-joining the voice call.
func (c *chatSession) notifyQueueDrained(previous media.Track) {
	c.service.logger.Info("queue drained, notifying observer for autoplay", "chat_id", c.chatID, "track_title", previous.Title, "track_id", previous.ID)
	if observer := c.service.observer; observer != nil {
		go observer.QueueDrained(c.chatID, previous)
	}
}
func (c *chatSession) notifyError(err error) {
	c.service.logger.Error("playback session failed", "chat_id", c.chatID, "error", err)
	if observer := c.service.observer; observer != nil {
		go observer.PlaybackError(c.chatID, err)
	}
}
