package audio

import (
	"errors"
	"fmt"
	"sync"
	"time"

	teaui "github.com/darkliquid/musicon/internal/ui"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/effects"
)

const (
	defaultSampleRate     = beep.SampleRate(48_000)
	defaultBufferDuration = 100 * time.Millisecond
)

// Resolver turns queue entries into playable beep streams without embedding
// source-specific loading behavior in the audio runtime.
type Resolver interface {
	Resolve(entry teaui.QueueEntry) (ResolvedTrack, error)
}

// ResolvedTrack contains a decoded/ready-to-play stream plus the metadata the
// UI needs to display the current item.
type ResolvedTrack struct {
	Info   teaui.TrackInfo
	Format beep.Format
	Stream beep.StreamSeekCloser
}

// Options configures the playback engine.
type Options struct {
	Resolver       Resolver
	OutputRate     beep.SampleRate
	BufferDuration time.Duration
	Backend        string
}

// Engine owns queue state, active playback state, and the mago/beep runtime.
type Engine struct {
	mu sync.Mutex

	resolver Resolver

	queue        []teaui.QueueEntry
	currentIndex int
	current      *activeTrack

	repeat bool
	stream bool
	volume int

	speakerRate   beep.SampleRate
	bufferSamples int
	speakerReady  bool
	closed        bool
	speaker       *runtimeSpeaker
	speakerErr    error
	lastSnapshot  teaui.PlaybackSnapshot
}

type activeTrack struct {
	entry      teaui.QueueEntry
	info       teaui.TrackInfo
	format     beep.Format
	stream     beep.StreamSeekCloser
	controller *beep.Ctrl
	volumeFx   *effects.Volume
}

type replacementStreamPreparer interface {
	PrepareReplacement(target int) (beep.StreamSeekCloser, error)
}

type queueService struct{ engine *Engine }
type playbackService struct{ engine *Engine }

func NewEngine(options Options) *Engine {
	rate := options.OutputRate
	if rate <= 0 {
		rate = defaultSampleRate
	}
	bufferDuration := options.BufferDuration
	if bufferDuration <= 0 {
		bufferDuration = defaultBufferDuration
	}
	backends, backendErr := selectSpeakerBackends(options.Backend)

	return &Engine{
		resolver:      options.Resolver,
		currentIndex:  -1,
		volume:        70,
		speakerRate:   rate,
		bufferSamples: rate.N(bufferDuration),
		speaker:       newRuntimeSpeaker(backends),
		speakerErr:    backendErr,
		lastSnapshot: teaui.PlaybackSnapshot{
			Volume:     70,
			QueueIndex: -1,
		},
	}
}

func (e *Engine) QueueService() teaui.QueueService {
	return queueService{engine: e}
}

func (e *Engine) PlaybackService() teaui.PlaybackService {
	return playbackService{engine: e}
}

func (e *Engine) QueueSnapshot() []teaui.QueueEntry {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]teaui.QueueEntry(nil), e.queue...)
}

func (e *Engine) AddToQueue(result teaui.SearchResult) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("audio runtime is closed")
	}

	entry := teaui.QueueEntry{
		ID:       result.ID,
		Title:    result.Title,
		Subtitle: result.Subtitle,
		Source:   result.Source,
		Kind:     result.Kind,
		Duration: result.Duration,
		Artwork:  result.Artwork,
	}
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("queue-%d", len(e.queue)+1)
	}
	if entry.Title == "" {
		entry.Title = entry.ID
	}

	e.queue = append(e.queue, entry)
	if e.currentIndex < 0 {
		e.currentIndex = 0
	}
	return nil
}

func (e *Engine) RemoveFromQueue(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("audio runtime is closed")
	}

	index := -1
	for i, entry := range e.queue {
		if entry.ID == id {
			index = i
			break
		}
	}
	if index == -1 {
		return fmt.Errorf("queue item %q not found", id)
	}

	removingCurrent := e.current != nil && index == e.currentIndex
	if removingCurrent {
		e.stopCurrentLocked(false)
	}

	e.queue = append(e.queue[:index], e.queue[index+1:]...)
	if len(e.queue) == 0 {
		e.currentIndex = -1
		return nil
	}
	if index < e.currentIndex {
		e.currentIndex--
	}
	if e.currentIndex >= len(e.queue) {
		e.currentIndex = len(e.queue) - 1
	}
	if removingCurrent {
		return e.startCurrentLocked(false)
	}
	return nil
}

func (e *Engine) MoveQueueEntry(id string, delta int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("audio runtime is closed")
	}
	if len(e.queue) == 0 {
		return errors.New("queue is empty")
	}
	if delta == 0 {
		return nil
	}

	index := -1
	for i, entry := range e.queue {
		if entry.ID == id {
			index = i
			break
		}
	}
	if index == -1 {
		return fmt.Errorf("queue item %q not found", id)
	}

	target := index + delta
	if target < 0 {
		target = 0
	}
	if target >= len(e.queue) {
		target = len(e.queue) - 1
	}
	if target == index {
		return nil
	}

	entry := e.queue[index]
	e.queue = append(e.queue[:index], e.queue[index+1:]...)
	head := append([]teaui.QueueEntry(nil), e.queue[:target]...)
	head = append(head, entry)
	e.queue = append(head, e.queue[target:]...)

	switch {
	case e.currentIndex == index:
		e.currentIndex = target
	case index < e.currentIndex && target >= e.currentIndex:
		e.currentIndex--
	case index > e.currentIndex && target <= e.currentIndex:
		e.currentIndex++
	}
	return nil
}

func (e *Engine) ClearQueue() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("audio runtime is closed")
	}
	e.stopCurrentLocked(false)
	e.queue = nil
	e.currentIndex = -1
	return nil
}

func (e *Engine) PlaybackSnapshot() teaui.PlaybackSnapshot {
	if !e.mu.TryLock() {
		return e.lastSnapshot
	}
	defer e.mu.Unlock()

	snapshot := e.playbackSnapshotLocked()
	e.lastSnapshot = snapshot
	return snapshot
}

func (e *Engine) playbackSnapshotLocked() teaui.PlaybackSnapshot {
	snapshot := teaui.PlaybackSnapshot{
		Repeat:      e.repeat,
		Stream:      e.stream,
		Volume:      e.volume,
		QueueLength: len(e.queue),
		QueueIndex:  e.currentIndex,
	}
	if len(e.queue) > 0 && snapshot.QueueIndex < 0 {
		snapshot.QueueIndex = 0
	}
	if e.current == nil {
		return snapshot
	}

	track := e.current.info
	snapshot.Track = &track
	snapshot.Duration = e.currentDurationLocked()
	snapshot.Position = e.currentPositionLocked()
	snapshot.Paused = e.current.controller != nil && e.current.controller.Paused
	return snapshot
}

func (e *Engine) TogglePause() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("audio runtime is closed")
	}
	if len(e.queue) == 0 {
		return errors.New("queue is empty")
	}
	if e.current == nil {
		if e.currentIndex < 0 {
			e.currentIndex = 0
		}
		return e.startCurrentLocked(false)
	}
	if e.current.controller == nil {
		return errors.New("current track is not controllable")
	}
	e.withSpeakerLock(func() {
		e.current.controller.Paused = !e.current.controller.Paused
	})
	return nil
}

func (e *Engine) Previous() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("audio runtime is closed")
	}
	if len(e.queue) == 0 {
		return errors.New("queue is empty")
	}
	if e.currentIndex <= 0 {
		if !e.repeat {
			return errors.New("already at the start of the queue")
		}
		e.currentIndex = len(e.queue) - 1
	} else {
		e.currentIndex--
	}
	return e.startCurrentLocked(false)
}

func (e *Engine) Next() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("audio runtime is closed")
	}
	if len(e.queue) == 0 {
		return errors.New("queue is empty")
	}
	if e.currentIndex >= len(e.queue)-1 {
		if !e.repeat {
			return errors.New("already at the end of the queue")
		}
		e.currentIndex = 0
	} else {
		e.currentIndex++
	}
	return e.startCurrentLocked(false)
}

func (e *Engine) AdjustVolume(delta int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("audio runtime is closed")
	}
	e.volume += delta
	if e.volume < 0 {
		e.volume = 0
	}
	if e.volume > 100 {
		e.volume = 100
	}
	if e.current == nil || e.current.volumeFx == nil {
		return nil
	}
	volume, silent := percentToLevel(e.volume)
	e.withSpeakerLock(func() {
		e.current.volumeFx.Volume = volume
		e.current.volumeFx.Silent = silent
	})
	return nil
}

func (e *Engine) SeekTo(target time.Duration) error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return errors.New("audio runtime is closed")
	}
	if e.current == nil || e.current.stream == nil {
		e.mu.Unlock()
		return errors.New("no active track")
	}
	current := e.current
	targetSample := current.format.SampleRate.N(target)
	if targetSample < 0 {
		targetSample = 0
	}
	if length := current.stream.Len(); length > 0 && targetSample > length {
		targetSample = length
	}
	e.mu.Unlock()

	var seekErr error
	e.withSpeakerLock(func() {
		seekErr = current.stream.Seek(targetSample)
	})
	if seekErr == nil {
		return nil
	}

	preparer, ok := current.stream.(replacementStreamPreparer)
	if !ok {
		return seekErr
	}
	replacement, err := preparer.PrepareReplacement(targetSample)
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		_ = replacement.Close()
		return errors.New("audio runtime is closed")
	}
	if e.current != current {
		_ = replacement.Close()
		return errors.New("active track changed during seek")
	}
	paused := current.controller != nil && current.controller.Paused
	if e.speakerReady {
		e.speaker.Clear()
	}
	if current.stream != nil {
		_ = current.stream.Close()
	}
	e.current = nil
	return e.activateTrackLocked(current.entry, current.info, current.format, replacement, paused)
}

func (e *Engine) SetRepeat(repeat bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("audio runtime is closed")
	}
	e.repeat = repeat
	return nil
}

func (e *Engine) SetStream(stream bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("audio runtime is closed")
	}
	e.stream = stream
	return nil
}

func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	e.stopCurrentLocked(true)
	e.queue = nil
	e.currentIndex = -1
	return nil
}

func (e *Engine) startCurrentLocked(paused bool) error {
	if len(e.queue) == 0 {
		return errors.New("queue is empty")
	}
	if e.currentIndex < 0 || e.currentIndex >= len(e.queue) {
		e.currentIndex = 0
	}
	if e.resolver == nil {
		return errors.New("no audio resolver configured")
	}

	entry := e.queue[e.currentIndex]
	resolved, err := e.resolver.Resolve(entry)
	if err != nil {
		return err
	}
	if resolved.Stream == nil {
		return fmt.Errorf("resolver returned no stream for %q", entry.Title)
	}
	if resolved.Format.SampleRate <= 0 {
		_ = resolved.Stream.Close()
		return errors.New("resolver returned an invalid sample rate")
	}
	if err := e.ensureSpeakerLocked(); err != nil {
		_ = resolved.Stream.Close()
		return err
	}

	e.stopCurrentLocked(false)

	info := resolved.Info
	info = prepareTrackInfo(entry, resolved)
	return e.activateTrackLocked(entry, info, resolved.Format, resolved.Stream, paused)
}

func (e *Engine) activateTrackLocked(entry teaui.QueueEntry, info teaui.TrackInfo, format beep.Format, stream beep.StreamSeekCloser, paused bool) error {
	streamer := beep.Streamer(stream)
	if format.SampleRate != e.speakerRate {
		streamer = beep.Resample(3, format.SampleRate, e.speakerRate, streamer)
	}

	controller := &beep.Ctrl{Streamer: streamer, Paused: paused}
	volumeLevel, silent := percentToLevel(e.volume)
	volumeFx := &effects.Volume{Streamer: controller, Base: 2, Volume: volumeLevel, Silent: silent}
	index := e.currentIndex
	sequence := beep.Seq(volumeFx, beep.Callback(func() { go e.onTrackFinished(index) }))
	e.speaker.Play(sequence)

	e.current = &activeTrack{
		entry:      entry,
		info:       info,
		format:     format,
		stream:     stream,
		controller: controller,
		volumeFx:   volumeFx,
	}
	return nil
}

func prepareTrackInfo(entry teaui.QueueEntry, resolved ResolvedTrack) teaui.TrackInfo {
	info := resolved.Info
	if info.ID == "" {
		info.ID = entry.ID
	}
	if info.Title == "" {
		info.Title = entry.Title
	}
	if info.Source == "" {
		info.Source = entry.Source
	}
	if info.Duration <= 0 && resolved.Stream != nil && resolved.Format.SampleRate > 0 {
		info.Duration = resolved.Format.SampleRate.D(resolved.Stream.Len())
	}
	info.Artwork = info.Artwork.Merge(entry.Artwork)
	return info
}

func (e *Engine) ensureSpeakerLocked() error {
	if e.speakerReady {
		return nil
	}
	if e.speakerErr != nil {
		return e.speakerErr
	}
	if err := e.speaker.Init(e.speakerRate, e.bufferSamples); err != nil {
		return err
	}
	e.speakerReady = true
	return nil
}

func (e *Engine) stopCurrentLocked(closeSpeaker bool) {
	if e.speakerReady {
		e.speaker.Clear()
	}
	if e.current != nil && e.current.stream != nil {
		_ = e.current.stream.Close()
	}
	e.current = nil
	if closeSpeaker && e.speakerReady {
		e.speaker.Close()
		e.speakerReady = false
	}
}

func (e *Engine) currentPositionLocked() time.Duration {
	if e.current == nil || e.current.stream == nil {
		return 0
	}
	return e.current.format.SampleRate.D(e.current.stream.Position())
}

func (e *Engine) currentDurationLocked() time.Duration {
	if e.current == nil || e.current.stream == nil {
		return 0
	}
	return e.current.format.SampleRate.D(e.current.stream.Len())
}

func (e *Engine) withSpeakerLock(fn func()) {
	if !e.speakerReady {
		fn()
		return
	}
	e.speaker.Lock()
	defer e.speaker.Unlock()
	fn()
}

func (e *Engine) onTrackFinished(index int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.current == nil || index != e.currentIndex {
		return
	}
	if e.current.stream != nil {
		_ = e.current.stream.Close()
	}
	e.current = nil
	if len(e.queue) == 0 {
		e.currentIndex = -1
		return
	}

	nextIndex := index + 1
	if nextIndex >= len(e.queue) {
		if e.repeat {
			nextIndex = 0
		} else {
			return
		}
	}
	e.currentIndex = nextIndex
	_ = e.startCurrentLocked(false)
}

func percentToLevel(percent int) (float64, bool) {
	if percent <= 0 {
		return 0, true
	}
	if percent > 100 {
		percent = 100
	}
	return float64(percent-100) / 50.0, false
}

func (s queueService) Snapshot() []teaui.QueueEntry         { return s.engine.QueueSnapshot() }
func (s queueService) Add(result teaui.SearchResult) error  { return s.engine.AddToQueue(result) }
func (s queueService) Move(id string, delta int) error      { return s.engine.MoveQueueEntry(id, delta) }
func (s queueService) Remove(id string) error               { return s.engine.RemoveFromQueue(id) }
func (s queueService) Clear() error                         { return s.engine.ClearQueue() }
func (s playbackService) Snapshot() teaui.PlaybackSnapshot  { return s.engine.PlaybackSnapshot() }
func (s playbackService) TogglePause() error                { return s.engine.TogglePause() }
func (s playbackService) Previous() error                   { return s.engine.Previous() }
func (s playbackService) Next() error                       { return s.engine.Next() }
func (s playbackService) SeekTo(target time.Duration) error { return s.engine.SeekTo(target) }
func (s playbackService) AdjustVolume(delta int) error      { return s.engine.AdjustVolume(delta) }
func (s playbackService) SetRepeat(repeat bool) error       { return s.engine.SetRepeat(repeat) }
func (s playbackService) SetStream(stream bool) error       { return s.engine.SetStream(stream) }
