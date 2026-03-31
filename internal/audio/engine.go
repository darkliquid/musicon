package audio

import (
	"errors"
	"fmt"
	"strings"
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
	visual   *visualizationState

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

// NewEngine constructs a playback engine with normalized defaults for buffering and backend selection.
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
		visual:        newVisualizationState(rate),
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

// QueueService returns a UI-facing queue adapter backed by the engine.
func (e *Engine) QueueService() teaui.QueueService {
	return queueService{engine: e}
}

// PlaybackService returns a UI-facing playback adapter backed by the engine.
func (e *Engine) PlaybackService() teaui.PlaybackService {
	return playbackService{engine: e}
}

// VisualizationService returns a UI-facing visualization adapter backed by the engine.
func (e *Engine) VisualizationService() teaui.VisualizationProvider {
	return visualizationService{engine: e}
}

// QueueSnapshot returns a copy of the current queue state.
func (e *Engine) QueueSnapshot() []teaui.QueueEntry {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]teaui.QueueEntry(nil), e.queue...)
}

// AddToQueue appends a search result to the playback queue.
func (e *Engine) AddToQueue(result teaui.SearchResult) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("audio runtime is closed")
	}

	if isGroupedCollectionResult(result) {
		return e.addCollectionLocked(result)
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

func (e *Engine) addCollectionLocked(result teaui.SearchResult) error {
	groupID := result.ID
	if groupID == "" {
		groupID = fmt.Sprintf("queue-group-%d", len(e.queue)+1)
	}

	children, err := groupedCollectionEntries(result)
	if err != nil {
		return err
	}
	e.queue = append(e.queue, children...)
	if e.currentIndex < 0 && len(e.queue) > 0 {
		e.currentIndex = 0
	}
	return nil
}

// RemoveFromQueue removes the queued entry with the supplied ID.
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

// RemoveGroupFromQueue removes every queued child that belongs to the supplied collection group.
func (e *Engine) RemoveGroupFromQueue(groupID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("audio runtime is closed")
	}
	if strings.TrimSpace(groupID) == "" {
		return errors.New("queue group id is required")
	}

	indices := make([]int, 0, len(e.queue))
	for i, entry := range e.queue {
		if entry.GroupID == groupID {
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		return fmt.Errorf("queue group %q not found", groupID)
	}

	removingCurrent := false
	for _, index := range indices {
		if e.current != nil && index == e.currentIndex {
			removingCurrent = true
			break
		}
	}
	if removingCurrent {
		e.stopCurrentLocked(false)
	}

	filtered := e.queue[:0]
	for _, entry := range e.queue {
		if entry.GroupID != groupID {
			filtered = append(filtered, entry)
		}
	}
	e.queue = append([]teaui.QueueEntry(nil), filtered...)
	e.normalizeQueueGroupsLocked()

	if len(e.queue) == 0 {
		e.currentIndex = -1
		return nil
	}

	if removingCurrent {
		if e.currentIndex >= len(e.queue) {
			e.currentIndex = len(e.queue) - 1
		}
		return e.startCurrentLocked(false)
	}

	if e.currentIndex >= len(e.queue) {
		e.currentIndex = len(e.queue) - 1
	}
	return nil
}

// MoveQueueEntry reorders a queued entry by the requested relative delta.
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

// ClearQueue stops playback and removes every queued entry.
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

// PlaybackSnapshot returns a UI-safe snapshot of the current playback state.
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

// TogglePause starts playback if needed or toggles the active track between paused and playing.
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

// Previous switches playback to the previous queue entry.
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

// Next switches playback to the next queue entry.
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

// AdjustVolume changes playback volume by delta percentage points.
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

// SeekTo moves the active track to the requested absolute position.
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

// SetRepeat updates repeat mode for queue playback.
func (e *Engine) SetRepeat(repeat bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("audio runtime is closed")
	}
	e.repeat = repeat
	return nil
}

// SetStream updates stream-continuation mode for playback.
func (e *Engine) SetStream(stream bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("audio runtime is closed")
	}
	e.stream = stream
	return nil
}

// Close releases playback resources and prevents further use of the engine.
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
	if e.visual != nil {
		e.visual.Reset(e.speakerRate)
	}
	index := e.currentIndex
	sequence := beep.Seq(newAnalysisTap(volumeFx, e.visual), beep.Callback(func() { go e.onTrackFinished(index) }))
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
	if e.visual != nil {
		e.visual.Clear()
	}
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

// Snapshot returns a copy of the queue state exposed through the UI adapter.
func (s queueService) Snapshot() []teaui.QueueEntry { return s.engine.QueueSnapshot() }

// Add appends a search result to the queue through the UI adapter.
func (s queueService) Add(result teaui.SearchResult) error { return s.engine.AddToQueue(result) }

// Move reorders a queued entry by delta positions through the UI adapter.
func (s queueService) Move(id string, delta int) error { return s.engine.MoveQueueEntry(id, delta) }

// Remove deletes a queued entry through the UI adapter.
func (s queueService) Remove(id string) error { return s.engine.RemoveFromQueue(id) }

// RemoveGroup deletes a grouped collection through the UI adapter.
func (s queueService) RemoveGroup(groupID string) error {
	return s.engine.RemoveGroupFromQueue(groupID)
}

// Clear removes all queued entries through the UI adapter.
func (s queueService) Clear() error { return s.engine.ClearQueue() }

func isGroupedCollectionResult(result teaui.SearchResult) bool {
	return (result.Kind == teaui.MediaAlbum || result.Kind == teaui.MediaPlaylist) && len(result.CollectionItems) > 0
}

func groupedCollectionEntries(result teaui.SearchResult) ([]teaui.QueueEntry, error) {
	entries := make([]teaui.QueueEntry, 0, len(result.CollectionItems))
	for _, child := range result.CollectionItems {
		entry := child
		entry.GroupID = result.ID
		entry.GroupTitle = result.Title
		entry.GroupKind = result.Kind
		if entry.Source == "" {
			entry.Source = result.Source
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("collection %q has no queueable children", result.Title)
	}
	for index := range entries {
		entries[index].GroupIndex = index
		entries[index].GroupSize = len(entries)
	}
	return entries, nil
}

func (e *Engine) normalizeQueueGroupsLocked() {
	groupCounts := make(map[string]int)
	for _, entry := range e.queue {
		if entry.GroupID != "" {
			groupCounts[entry.GroupID]++
		}
	}

	groupOffsets := make(map[string]int)
	for index := range e.queue {
		entry := &e.queue[index]
		if entry.GroupID == "" {
			continue
		}
		size := groupCounts[entry.GroupID]
		if size <= 1 {
			entry.GroupID = ""
			entry.GroupTitle = ""
			entry.GroupKind = ""
			entry.GroupIndex = 0
			entry.GroupSize = 0
			continue
		}
		entry.GroupIndex = groupOffsets[entry.GroupID]
		entry.GroupSize = size
		groupOffsets[entry.GroupID]++
	}
}

// Snapshot returns the current playback state exposed through the UI adapter.
func (s playbackService) Snapshot() teaui.PlaybackSnapshot { return s.engine.PlaybackSnapshot() }

// TogglePause toggles paused playback through the UI adapter.
func (s playbackService) TogglePause() error { return s.engine.TogglePause() }

// Previous switches to the previous queued track through the UI adapter.
func (s playbackService) Previous() error { return s.engine.Previous() }

// Next switches to the next queued track through the UI adapter.
func (s playbackService) Next() error { return s.engine.Next() }

// SeekTo performs an absolute seek through the UI adapter.
func (s playbackService) SeekTo(target time.Duration) error { return s.engine.SeekTo(target) }

// AdjustVolume changes playback volume through the UI adapter.
func (s playbackService) AdjustVolume(delta int) error { return s.engine.AdjustVolume(delta) }

// SetRepeat updates repeat mode through the UI adapter.
func (s playbackService) SetRepeat(repeat bool) error { return s.engine.SetRepeat(repeat) }

// SetStream updates stream-continuation mode through the UI adapter.
func (s playbackService) SetStream(stream bool) error { return s.engine.SetStream(stream) }
