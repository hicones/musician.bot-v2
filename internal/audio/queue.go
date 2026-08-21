package audio

import (
	"math/rand"
	"sync"
	"time"

	"musician-bot-v2/internal/ui"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type TrackQueue struct {
	mu         sync.RWMutex
	current    *ui.TrackItem
	tracks     []ui.TrackItem
	history    []ui.TrackItem
	repeatMode int
	paused     bool
	radioMode  bool
}

func NewTrackQueue() *TrackQueue {
	return &TrackQueue{
		tracks:  make([]ui.TrackItem, 0),
		history: make([]ui.TrackItem, 0),
	}
}

func (q *TrackQueue) Push(items ...ui.TrackItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tracks = append(q.tracks, items...)
}

func (q *TrackQueue) Pop() *ui.TrackItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.current != nil {
		// If repeat track mode is active, replay current
		if q.repeatMode == ui.RepeatModeTrack {
			return q.current
		}

		// Add current to history
		q.addToHistory(*q.current)

		// If repeat queue mode is active, re-append current to end of queue
		if q.repeatMode == ui.RepeatModeQueue {
			q.tracks = append(q.tracks, *q.current)
		}
	}

	if len(q.tracks) == 0 {
		q.current = nil
		return nil
	}

	item := q.tracks[0]
	q.tracks = q.tracks[1:]
	q.current = &item
	return q.current
}

func (q *TrackQueue) Previous() *ui.TrackItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.history) == 0 {
		return nil
	}

	lastIdx := len(q.history) - 1
	prev := q.history[lastIdx]
	q.history = q.history[:lastIdx]

	if q.current != nil {
		q.tracks = append([]ui.TrackItem{*q.current}, q.tracks...)
	}

	q.current = &prev
	return q.current
}

func (q *TrackQueue) addToHistory(item ui.TrackItem) {
	// Avoid immediate consecutive duplicate in history
	if len(q.history) > 0 && q.history[len(q.history)-1].Track.Info.Identifier == item.Track.Info.Identifier {
		return
	}
	q.history = append(q.history, item)
}

func (q *TrackQueue) Current() *ui.TrackItem {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.current
}

func (q *TrackQueue) SetCurrent(item *ui.TrackItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.current = item
}

func (q *TrackQueue) Tracks() []ui.TrackItem {
	q.mu.RLock()
	defer q.mu.RUnlock()
	cp := make([]ui.TrackItem, len(q.tracks))
	copy(cp, q.tracks)
	return cp
}

func (q *TrackQueue) History() []ui.TrackItem {
	q.mu.RLock()
	defer q.mu.RUnlock()
	cp := make([]ui.TrackItem, len(q.history))
	copy(cp, q.history)
	return cp
}

func (q *TrackQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tracks = make([]ui.TrackItem, 0)
	q.current = nil
	q.radioMode = false
}

func (q *TrackQueue) ClearHistory() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.history = make([]ui.TrackItem, 0)
}

func (q *TrackQueue) Shuffle() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := len(q.tracks) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		q.tracks[i], q.tracks[j] = q.tracks[j], q.tracks[i]
	}
}

func (q *TrackQueue) SetRepeatMode(mode int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.repeatMode = mode
}

func (q *TrackQueue) CycleRepeatMode() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.repeatMode = (q.repeatMode + 1) % 3
	return q.repeatMode
}

func (q *TrackQueue) RepeatMode() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.repeatMode
}

func (q *TrackQueue) SetPaused(paused bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.paused = paused
}

func (q *TrackQueue) IsPaused() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.paused
}

func (q *TrackQueue) SetRadioMode(enabled bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.radioMode = enabled
}

func (q *TrackQueue) IsRadioMode() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.radioMode
}

func (q *TrackQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.tracks)
}
