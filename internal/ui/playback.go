package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	bubblekey "github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/darkliquid/musicon/pkg/components"
	"github.com/darkliquid/musicon/pkg/lyrics"
)

type playbackScreen struct {
	services  Services
	width     int
	height    int
	keymap    PlaybackKeyMap
	pane      PlaybackPane
	showInfo  bool
	pending   bool
	snapshot  PlaybackSnapshot
	status    string
	artStatus string
	artwork   *components.TerminalImage

	artworkTrackKey string
	artworkSource   *components.ImageSource
	artworkErr      error
	artworkLoading  bool
	artworkSpinner  int
	artworkAttempts []ArtworkAttempt
	artworkLookup   *artworkLookupState

	lyricsTrackKey string
	lyricsDoc      *lyrics.Document
	lyricsErr      error
	lyricsLoading  bool
	lyricsLookup   *lyricsLookupState
	lyricsScroll   int

	visualKey     string
	visualContent string
	visualErr     error

	seekAdjustment time.Duration
	seekDeadline   time.Time
}

type playbackActionResult struct {
	snapshot PlaybackSnapshot
	status   string
	err      error
}

type seekedMsg struct {
	snapshot PlaybackSnapshot
	err      error
}

type artworkLookupState struct {
	mu       sync.Mutex
	attempts []ArtworkAttempt
	source   *components.ImageSource
	err      error
	done     bool
}

type lyricsLookupState struct {
	mu       sync.Mutex
	document *lyrics.Document
	err      error
	done     bool
}

const (
	playbackSeekStep     = 5 * time.Second
	playbackSeekDebounce = 120 * time.Millisecond
)

func newPlaybackScreen(services Services, options AlbumArtOptions) *playbackScreen {
	return newPlaybackScreenWithKeyMap(services, options, normalizedKeyMap(KeybindOptions{}).Playback)
}

func newPlaybackScreenWithKeyMap(services Services, options AlbumArtOptions, keymap PlaybackKeyMap) *playbackScreen {
	screen := &playbackScreen{
		services: services,
		keymap:   keymap,
		pane:     PaneArtwork,
		snapshot: PlaybackSnapshot{Volume: 60},
		status:   "Playback mode ready. Connect a playback backend to drive live state.",
		artwork: components.NewTerminalImageWithSettings(components.TerminalImageSettings{
			Protocol:  options.Protocol,
			ScaleMode: options.FillMode,
		}),
	}
	screen.refreshSnapshot()
	return screen
}

// SetSize updates the playback screen's render bounds.
func (p *playbackScreen) SetSize(width, height int) {
	p.width = max(1, width)
	p.height = max(1, height)
}

// Update handles playback-screen input, ticks, and async playback results.
func (p *playbackScreen) Update(msg tea.Msg) (string, tea.Cmd) {
	switch typed := msg.(type) {
	case playbackActionResult:
		p.pending = false
		if typed.err != nil {
			return typed.err.Error(), nil
		}
		typed.snapshot.Volume = clamp(typed.snapshot.Volume, 0, 100)
		p.snapshot = typed.snapshot
		return typed.status, nil
	case seekedMsg:
		p.pending = false
		if typed.err != nil {
			return typed.err.Error(), nil
		}
		typed.snapshot.Volume = clamp(typed.snapshot.Volume, 0, 100)
		p.snapshot = typed.snapshot
		return "", nil
	case tickMsg:
		p.artworkSpinner = (p.artworkSpinner + 1) % len(artworkSpinnerFrames)
		p.consumeArtworkLookup()
		p.consumeLyricsLookup()
		if p.services.Playback == nil || p.pending || p.seekAdjustment == 0 || time.Now().Before(p.seekDeadline) {
			return "", nil
		}
		target := p.snapshot.Position + p.seekAdjustment
		p.seekAdjustment = 0
		p.seekDeadline = time.Time{}
		p.pending = true
		playback := p.services.Playback
		return "", func() tea.Msg {
			if err := playback.SeekTo(target); err != nil {
				return seekedMsg{err: err}
			}
			return seekedMsg{snapshot: playback.Snapshot()}
		}
	}

	keypress, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return "", nil
	}
	if p.pane == PaneLyrics {
		if status, handled := p.handleLyricsScrollKey(keypress); handled {
			return status, nil
		}
	}

	switch {
	case bubblekey.Matches(keypress, p.keymap.CyclePane):
		p.pane = (p.pane + 1) % 4
		return fmt.Sprintf("Playback pane: %s", p.pane.String()), nil
	case bubblekey.Matches(keypress, p.keymap.ToggleInfo):
		p.showInfo = !p.showInfo
		if p.showInfo {
			return "Track information shown.", nil
		}
		return "Track information hidden.", nil
	case bubblekey.Matches(keypress, p.keymap.ToggleRepeat):
		if p.services.Playback != nil {
			next := !p.snapshot.Repeat
			return "", p.runPlaybackAction(func(service PlaybackService) error {
				return service.SetRepeat(next)
			}, func(snapshot PlaybackSnapshot) string {
				return fmt.Sprintf("Repeat %s.", onOff(snapshot.Repeat))
			})
		} else {
			p.snapshot.Repeat = !p.snapshot.Repeat
		}
		return fmt.Sprintf("Repeat %s.", onOff(p.snapshot.Repeat)), nil
	case bubblekey.Matches(keypress, p.keymap.ToggleStream):
		if p.services.Playback != nil {
			next := !p.snapshot.Stream
			return "", p.runPlaybackAction(func(service PlaybackService) error {
				return service.SetStream(next)
			}, func(snapshot PlaybackSnapshot) string {
				return fmt.Sprintf("Stream continuation %s.", onOff(snapshot.Stream))
			})
		} else {
			p.snapshot.Stream = !p.snapshot.Stream
		}
		return fmt.Sprintf("Stream continuation %s.", onOff(p.snapshot.Stream)), nil
	case bubblekey.Matches(keypress, p.keymap.TogglePause):
		if p.services.Playback != nil {
			return "", p.runPlaybackAction(func(service PlaybackService) error {
				return service.TogglePause()
			}, func(snapshot PlaybackSnapshot) string {
				if snapshot.Paused {
					return "Playback paused."
				}
				return "Playback resumed."
			})
		} else {
			p.snapshot.Paused = !p.snapshot.Paused
		}
		if p.snapshot.Paused {
			return "Playback paused.", nil
		}
		return "Playback resumed.", nil
	case bubblekey.Matches(keypress, p.keymap.PreviousTrack):
		if p.services.Playback != nil {
			return "", p.runPlaybackAction(func(service PlaybackService) error {
				return service.Previous()
			}, func(PlaybackSnapshot) string {
				return "Moved to the previous track."
			})
		}
		return "Previous track requires a playback backend.", nil
	case bubblekey.Matches(keypress, p.keymap.NextTrack):
		if p.services.Playback != nil {
			return "", p.runPlaybackAction(func(service PlaybackService) error {
				return service.Next()
			}, func(PlaybackSnapshot) string {
				return "Moved to the next track."
			})
		}
		return "Next track requires a playback backend.", nil
	case bubblekey.Matches(keypress, p.keymap.SeekBackward):
		return p.accumulateSeek(-playbackSeekStep), nil
	case bubblekey.Matches(keypress, p.keymap.SeekForward):
		return p.accumulateSeek(playbackSeekStep), nil
	case bubblekey.Matches(keypress, p.keymap.VolumeDown):
		if p.services.Playback != nil {
			return "", p.runPlaybackAction(func(service PlaybackService) error {
				return service.AdjustVolume(-5)
			}, func(snapshot PlaybackSnapshot) string {
				return fmt.Sprintf("Volume set to %d%%.", snapshot.Volume)
			})
		}
		return p.adjustVolume(-5), nil
	case bubblekey.Matches(keypress, p.keymap.VolumeUp):
		if p.services.Playback != nil {
			return "", p.runPlaybackAction(func(service PlaybackService) error {
				return service.AdjustVolume(5)
			}, func(snapshot PlaybackSnapshot) string {
				return fmt.Sprintf("Volume set to %d%%.", snapshot.Volume)
			})
		}
		return p.adjustVolume(5), nil
	default:
		return "", nil
	}
}

// View renders the active playback pane plus its overlays.
func (p *playbackScreen) View() string {
	if p.width <= 0 || p.height <= 0 {
		return ""
	}

	body := lipgloss.NewStyle().Width(p.width).Height(p.height).Render(p.centerView(p.width, p.height))
	if overlay := p.artworkActivityOverlay(p.width, p.height); overlay != "" {
		body = centeredOverlay(body, overlay, p.width, p.height)
	}
	top := p.paneOverlay()
	body = bottomOverlay(body, p.controlsOverlay(), p.width, p.height)
	if p.showInfo {
		top = lipgloss.JoinVertical(lipgloss.Center, top, "", p.infoOverlay())
	}
	body = topOverlay(body, top, p.width, p.height)
	return body
}

// HelpView renders the playback-screen help overlay.
func (p *playbackScreen) HelpView() string {
	width := min(p.width, 68)
	height := min(p.height, 15)
	return components.RenderPanel(components.PanelOptions{
		Title:    "Playback help",
		Subtitle: "controls and info overlay the active pane",
		Width:    width,
		Height:   height,
		Focused:  true,
	}, strings.Join([]string{
		helpLine(p.keymap.TogglePause, "toggle play / pause state"),
		helpLinePair(p.keymap.PreviousTrack, p.keymap.NextTrack, "previous / next track request"),
		helpLinePair(p.keymap.SeekBackward, p.keymap.SeekForward, "seek backward / forward"),
		helpLinePair(p.keymap.VolumeDown, p.keymap.VolumeUp, "lower / raise volume"),
		helpLine(p.keymap.CyclePane, "cycle artwork, lyrics, eq, and visualizer panes"),
		helpLine(p.keymap.ToggleInfo, "show or hide track information"),
		helpLinePair(lyricsScrollUpKey, lyricsScrollDownKey, "scroll lyrics when Lyrics pane is active"),
		helpLinePair(lyricsPageUpKey, lyricsPageDownKey, "page lyrics viewport"),
		helpLinePair(lyricsHomeKey, lyricsEndKey, "jump to top / bottom of lyrics"),
		helpLinePair(p.keymap.ToggleRepeat, p.keymap.ToggleStream, "toggle repeat and stream continuation flags"),
	}, "\n"))
}

func (p *playbackScreen) paneOverlay() string {
	content := lipgloss.JoinHorizontal(
		lipgloss.Left,
		pill(p.pane.String(), true),
		"  ",
		lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(paneHint(p.pane)),
	)
	return lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Width(min(p.width, max(24, lipgloss.Width(content)+2))).
		Render(content)
}

func (p *playbackScreen) infoOverlay() string {
	width := min(p.width, 44)
	return components.RenderPanel(components.PanelOptions{
		Title:    "Track info",
		Subtitle: "toggle with i",
		Width:    width,
		Height:   8,
		Focused:  false,
	}, p.infoView(width-4, 5))
}

func (p *playbackScreen) controlsOverlay() string {
	width := min(p.width, max(36, p.width-4))
	return components.RenderPanel(components.PanelOptions{
		Title: "Playback",
		Subtitle: fmt.Sprintf("%s pause · %s / %s next/prev",
			bindingLabel(p.keymap.TogglePause),
			bindingLabel(p.keymap.PreviousTrack),
			bindingLabel(p.keymap.NextTrack),
		),
		Width:   width,
		Height:  6,
		Focused: false,
	}, p.controlsView(width-4))
}

func (p *playbackScreen) controlsView(width int) string {
	position := p.snapshot.Position
	duration := p.snapshot.Duration
	if duration <= 0 {
		duration = 0
	}
	ratio := 0.0
	if duration > 0 {
		ratio = float64(position) / float64(duration)
	}

	statusBits := []string{
		fmt.Sprintf("state: %s", playState(p.snapshot.Paused)),
		fmt.Sprintf("repeat: %s", onOff(p.snapshot.Repeat)),
		fmt.Sprintf("stream: %s", onOff(p.snapshot.Stream)),
		fmt.Sprintf("volume: %d%%", p.snapshot.Volume),
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		components.RenderProgress(width, ratio, fmt.Sprintf("%s / %s", formatDuration(position), formatDuration(duration))),
		lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(strings.Join(statusBits, " · ")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(fmt.Sprintf("visual mode: %s · info: %s", bindingLabel(p.keymap.CyclePane), bindingLabel(p.keymap.ToggleInfo))),
	)
}

func (p *playbackScreen) infoView(width, height int) string {
	track := p.snapshot.Track
	if track == nil {
		return components.RenderEmptyState(width, height, "No active track", "Connect a playback backend or load a track to populate metadata.")
	}

	lines := []string{
		fmt.Sprintf("Title:  %s", track.Title),
		fmt.Sprintf("Artist: %s", track.Artist),
		fmt.Sprintf("Album:  %s", track.Album),
		fmt.Sprintf("Source: %s", track.Source),
		fmt.Sprintf("Queue:  %d / %d", p.snapshot.QueueIndex+1, max(1, p.snapshot.QueueLength)),
	}
	return lipgloss.NewStyle().Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func (p *playbackScreen) centerView(width, height int) string {
	var trackInfo *TrackInfo
	if p.snapshot.Track != nil {
		trackInfo = p.snapshot.Track
	}

	switch p.pane {
	case PaneLyrics:
		p.refreshLyrics(trackInfo)
		if p.lyricsDoc != nil {
			if content := p.renderLyricsView(width, height); strings.TrimSpace(content) != "" {
				return content
			}
		}
		if p.lyricsLoading {
			return components.RenderEmptyState(width, height, "Loading lyrics", "Fetching lyrics for the active track without blocking playback mode.")
		}
		if p.lyricsErr != nil {
			return components.RenderEmptyState(width, height, "Lyrics unavailable", p.lyricsErr.Error())
		}
		if p.lyricsDoc != nil && p.lyricsDoc.Instrumental {
			return lipgloss.NewStyle().Width(width).Height(height).Render(strings.Join(p.lyricsDoc.DisplayLines(), "\n"))
		}
		return components.RenderEmptyState(width, height, "Lyrics unavailable", "No lyrics matched the active track.")
	case PaneEQ, PaneVisualizer:
		p.refreshVisualization(width, height)
		if strings.TrimSpace(p.visualContent) != "" {
			return lipgloss.NewStyle().Width(width).Height(height).Render(p.visualContent)
		}
		if p.visualErr != nil {
			return components.RenderEmptyState(width, height, p.pane.String()+" unavailable", p.visualErr.Error())
		}
		return neutralPlaybackPane(width, height)
	default:
		p.artwork.SetSize(width, height)
		p.refreshArtwork(trackInfo)
		if artwork := strings.TrimSpace(p.artwork.View()); artwork != "" {
			return lipgloss.Place(
				width,
				height,
				lipgloss.Center,
				lipgloss.Center,
				artwork,
				lipgloss.WithWhitespaceChars("·"),
				lipgloss.WithWhitespaceForeground(lipgloss.Color("238")),
			)
		}
		if renderErr := p.artwork.Error(); renderErr != nil {
			p.artStatus = fmt.Sprintf("Artwork render failed: %v", renderErr)
		} else if p.artworkErr != nil {
			p.artStatus = p.artworkErr.Error()
		} else if p.artworkSource != nil && strings.TrimSpace(p.artworkSource.Description) != "" {
			p.artStatus = p.artworkSource.Description
		}
		if strings.TrimSpace(p.artStatus) != "" {
			return components.RenderEmptyState(width, height, "Album art", p.artStatus)
		}
		return neutralPlaybackPane(width, height)
	}
}

func neutralPlaybackPane(width, height int) string {
	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		"",
		lipgloss.WithWhitespaceChars("·"),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("238")),
	)
}

func (p *playbackScreen) refreshArtwork(trackInfo *TrackInfo) {
	p.consumeArtworkLookup()
	p.artStatus = ""
	if p.services.Artwork == nil || trackInfo == nil {
		p.clearArtworkCache()
		return
	}
	key := artworkCacheKey(trackInfo)
	if key != p.artworkTrackKey {
		p.artworkTrackKey = key
		p.artworkSource = nil
		p.artworkErr = nil
		p.artworkLoading = true
		p.artworkSpinner = 0
		p.artworkAttempts = nil
		p.artwork.SetSource(nil)
		lookup := &artworkLookupState{}
		p.artworkLookup = lookup
		metadata := trackInfo.CoverArtMetadata()
		go func() {
			source, err := p.services.Artwork.ArtworkObserved(metadata, lookup.appendAttempt)
			lookup.complete(source, err)
		}()
		return
	}
}

func (p *playbackScreen) clearArtworkCache() {
	p.artworkTrackKey = ""
	p.artworkSource = nil
	p.artworkErr = nil
	p.artworkLoading = false
	p.artworkSpinner = 0
	p.artworkAttempts = nil
	p.artworkLookup = nil
	p.artwork.SetSource(nil)
}

func artworkCacheKey(track *TrackInfo) string {
	if track == nil {
		return ""
	}
	payload := struct {
		ID       string            `json:"id"`
		Source   string            `json:"source"`
		Title    string            `json:"title"`
		Artist   string            `json:"artist"`
		Album    string            `json:"album"`
		Metadata map[string]string `json:"metadata"`
	}{
		ID:     track.ID,
		Source: track.Source,
		Title:  track.Title,
		Artist: track.Artist,
		Album:  track.Album,
	}
	metadata := track.CoverArtMetadata().Normalize()
	payload.Metadata = map[string]string{
		"title":                      metadata.Title,
		"album":                      metadata.Album,
		"artist":                     metadata.Artist,
		"mb_release":                 metadata.IDs.MusicBrainzReleaseID,
		"mb_release_group":           metadata.IDs.MusicBrainzReleaseGroupID,
		"mb_recording":               metadata.IDs.MusicBrainzRecordingID,
		"spotify_album":              metadata.IDs.SpotifyAlbumID,
		"spotify_track":              metadata.IDs.SpotifyTrackID,
		"apple_music_album":          metadata.IDs.AppleMusicAlbumID,
		"apple_music_song":           metadata.IDs.AppleMusicSongID,
		"local_audio_path":           "",
		"local_cover_file_path":      "",
		"local_embedded_description": "",
	}
	if metadata.Local != nil {
		payload.Metadata["local_audio_path"] = metadata.Local.AudioPath
		payload.Metadata["local_cover_file_path"] = metadata.Local.CoverFilePath
		if metadata.Local.Embedded != nil {
			payload.Metadata["local_embedded_description"] = metadata.Local.Embedded.Description
		}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s", track.ID, track.Source, track.Title, track.Artist, track.Album)
	}
	return string(data)
}

var artworkSpinnerFrames = []string{"-", "\\", "|", "/"}

var (
	lyricsScrollUpKey   = bubblekey.NewBinding(bubblekey.WithKeys("up"), bubblekey.WithHelp("up", "scroll up"))
	lyricsScrollDownKey = bubblekey.NewBinding(bubblekey.WithKeys("down"), bubblekey.WithHelp("down", "scroll down"))
	lyricsPageUpKey     = bubblekey.NewBinding(bubblekey.WithKeys("pgup"), bubblekey.WithHelp("pgup", "page up"))
	lyricsPageDownKey   = bubblekey.NewBinding(bubblekey.WithKeys("pgdown"), bubblekey.WithHelp("pgdn", "page down"))
	lyricsHomeKey       = bubblekey.NewBinding(bubblekey.WithKeys("home"), bubblekey.WithHelp("home", "jump to top"))
	lyricsEndKey        = bubblekey.NewBinding(bubblekey.WithKeys("end"), bubblekey.WithHelp("end", "jump to bottom"))
)

func (p *playbackScreen) artworkActivityOverlay(width, height int) string {
	if p.pane != PaneArtwork || width <= 0 || height <= 0 {
		return ""
	}
	p.consumeArtworkLookup()
	if !p.artworkLoading {
		return ""
	}
	lines := make([]string, 0, 1+min(4, len(p.artworkAttempts)))
	lines = append(lines, fmt.Sprintf("%s artwork lookup running", artworkSpinnerFrames[p.artworkSpinner%len(artworkSpinnerFrames)]))
	for _, attempt := range p.recentArtworkAttempts(4) {
		message := attempt.Message
		if strings.TrimSpace(message) == "" {
			message = attempt.Status
		}
		lines = append(lines, fmt.Sprintf("%s: %s", attempt.Provider, message))
	}
	width = min(width-4, 44)
	if width < 18 {
		width = 18
	}
	return components.RenderPanel(components.PanelOptions{
		Title:    "Artwork lookup",
		Subtitle: "recent provider activity",
		Width:    width,
		Height:   min(height-2, max(4, len(lines)+2)),
		Focused:  false,
	}, strings.Join(lines, "\n"))
}

func (p *playbackScreen) recentArtworkAttempts(limit int) []ArtworkAttempt {
	if limit <= 0 || len(p.artworkAttempts) == 0 {
		return nil
	}
	start := max(0, len(p.artworkAttempts)-limit)
	return append([]ArtworkAttempt(nil), p.artworkAttempts[start:]...)
}

func (p *playbackScreen) consumeArtworkLookup() {
	if p.artworkLookup == nil {
		return
	}
	attempts, source, err, done := p.artworkLookup.snapshot()
	p.artworkAttempts = attempts
	p.artworkLoading = !done
	if !done {
		return
	}
	p.artworkSource = source
	p.artworkErr = err
	p.artwork.SetSource(source)
	p.artworkLookup = nil
}

func (s *artworkLookupState) appendAttempt(attempt ArtworkAttempt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts = append(s.attempts, attempt)
}

func (s *artworkLookupState) complete(source *components.ImageSource, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.source = source
	s.err = err
	s.done = true
}

func (s *artworkLookupState) snapshot() ([]ArtworkAttempt, *components.ImageSource, error, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ArtworkAttempt(nil), s.attempts...), s.source, s.err, s.done
}

func (p *playbackScreen) refreshLyrics(track *TrackInfo) {
	p.consumeLyricsLookup()
	if p.services.Lyrics == nil || track == nil {
		p.clearLyricsCache()
		return
	}
	key := lyricsCacheKey(track)
	if key == p.lyricsTrackKey {
		return
	}
	p.lyricsTrackKey = key
	p.lyricsDoc = nil
	p.lyricsErr = nil
	p.lyricsLoading = true
	p.lyricsScroll = 0
	lookup := &lyricsLookupState{}
	p.lyricsLookup = lookup
	request := track.LyricsRequest()
	go func() {
		document, err := p.services.Lyrics.Lyrics(request)
		lookup.complete(document, err)
	}()
}

func (p *playbackScreen) clearLyricsCache() {
	p.lyricsTrackKey = ""
	p.lyricsDoc = nil
	p.lyricsErr = nil
	p.lyricsLoading = false
	p.lyricsLookup = nil
	p.lyricsScroll = 0
}

func lyricsCacheKey(track *TrackInfo) string {
	if track == nil {
		return ""
	}
	payload := struct {
		ID             string        `json:"id"`
		Source         string        `json:"source"`
		Title          string        `json:"title"`
		Artist         string        `json:"artist"`
		Album          string        `json:"album"`
		Duration       time.Duration `json:"duration"`
		LocalAudioPath string        `json:"local_audio_path"`
	}{
		ID:       track.ID,
		Source:   track.Source,
		Title:    track.Title,
		Artist:   track.Artist,
		Album:    track.Album,
		Duration: track.Duration,
	}
	if local := track.CoverArtMetadata().Local; local != nil {
		payload.LocalAudioPath = local.AudioPath
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s", track.ID, track.Source, track.Title, track.Artist, track.Album)
	}
	return string(data)
}

func (p *playbackScreen) consumeLyricsLookup() {
	if p.lyricsLookup == nil {
		return
	}
	document, err, done := p.lyricsLookup.snapshot()
	p.lyricsLoading = !done
	if !done {
		return
	}
	p.lyricsDoc = document
	p.lyricsErr = err
	p.lyricsLookup = nil
}

func (p *playbackScreen) handleLyricsScrollKey(keypress tea.KeyPressMsg) (string, bool) {
	if p.lyricsHasTimedSync() {
		switch {
		case bubblekey.Matches(keypress, lyricsScrollUpKey),
			bubblekey.Matches(keypress, lyricsScrollDownKey),
			bubblekey.Matches(keypress, lyricsPageUpKey),
			bubblekey.Matches(keypress, lyricsPageDownKey),
			bubblekey.Matches(keypress, lyricsHomeKey),
			bubblekey.Matches(keypress, lyricsEndKey):
			return "Synced lyrics follow playback automatically.", true
		}
	}

	lines := p.lyricsDisplayLines()
	if len(lines) == 0 {
		return "", false
	}

	page := p.lyricsViewportHeight()
	if page <= 0 {
		page = 1
	}

	next := p.lyricsScroll
	switch {
	case bubblekey.Matches(keypress, lyricsScrollUpKey):
		next--
	case bubblekey.Matches(keypress, lyricsScrollDownKey):
		next++
	case bubblekey.Matches(keypress, lyricsPageUpKey):
		next -= page
	case bubblekey.Matches(keypress, lyricsPageDownKey):
		next += page
	case bubblekey.Matches(keypress, lyricsHomeKey):
		next = 0
	case bubblekey.Matches(keypress, lyricsEndKey):
		next = p.maxLyricsScroll()
	default:
		return "", false
	}

	next = clamp(next, 0, p.maxLyricsScroll())
	if next == p.lyricsScroll {
		return "", true
	}
	p.lyricsScroll = next
	start, end := p.lyricsWindow(len(lines))
	return fmt.Sprintf("Lyrics lines %d-%d of %d.", start+1, end, len(lines)), true
}

func (p *playbackScreen) renderLyricsView(width, height int) string {
	lines := p.renderedLyricsViewportLines()
	if len(lines) == 0 {
		return ""
	}

	topInset, bottomInset := p.lyricsInsets()
	canvas := make([]string, 0, height)
	for i := 0; i < topInset; i++ {
		canvas = append(canvas, "")
	}
	canvas = append(canvas, lines...)
	for len(canvas)+bottomInset < height {
		canvas = append(canvas, "")
	}
	for i := 0; i < bottomInset; i++ {
		canvas = append(canvas, "")
	}
	if len(canvas) > height {
		canvas = canvas[:height]
	}
	return lipgloss.NewStyle().Width(width).Height(height).Render(strings.Join(canvas, "\n"))
}

func (p *playbackScreen) lyricsDisplayLines() []string {
	if p.lyricsDoc == nil {
		return nil
	}
	return p.lyricsDoc.DisplayLines()
}

func (p *playbackScreen) lyricsHasTimedSync() bool {
	return p.lyricsDoc != nil && p.lyricsDoc.HasTimedLines()
}

func (p *playbackScreen) activeTimedLyricsLine() int {
	if !p.lyricsHasTimedSync() {
		return -1
	}
	return p.lyricsDoc.ActiveTimedLineIndex(p.snapshot.Position)
}

func (p *playbackScreen) renderedLyricsViewportLines() []string {
	lines := p.lyricsViewportLines()
	if len(lines) == 0 || !p.lyricsHasTimedSync() {
		return lines
	}

	active := p.activeTimedLyricsLine()
	start, _ := p.lyricsWindow(len(p.lyricsDisplayLines()))
	rendered := make([]string, 0, len(lines))
	pastStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	futureStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	activeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("57"))

	for offset, line := range lines {
		index := start + offset
		switch {
		case active >= 0 && index == active:
			rendered = append(rendered, activeStyle.Render("> "+line))
		case active >= 0 && index < active:
			rendered = append(rendered, pastStyle.Render("  "+line))
		default:
			rendered = append(rendered, futureStyle.Render("  "+line))
		}
	}
	return rendered
}

func (p *playbackScreen) lyricsViewportLines() []string {
	lines := p.lyricsDisplayLines()
	if len(lines) == 0 {
		return nil
	}
	start, end := p.lyricsWindow(len(lines))
	visible := append([]string(nil), lines[start:end]...)
	for len(visible) < p.lyricsViewportHeight() {
		visible = append(visible, "")
	}
	return visible
}

func (p *playbackScreen) lyricsWindow(total int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if p.lyricsHasTimedSync() {
		start := 0
		if active := p.activeTimedLyricsLine(); active >= 0 {
			start = clamp(active-(p.lyricsViewportHeight()/2), 0, max(0, total-p.lyricsViewportHeight()))
		}
		end := min(total, start+p.lyricsViewportHeight())
		return start, end
	}
	start := clamp(p.lyricsScroll, 0, p.maxLyricsScroll())
	p.lyricsScroll = start
	end := min(total, start+p.lyricsViewportHeight())
	return start, end
}

func (p *playbackScreen) maxLyricsScroll() int {
	return max(0, len(p.lyricsDisplayLines())-p.lyricsViewportHeight())
}

func (p *playbackScreen) lyricsViewportHeight() int {
	topInset, bottomInset := p.lyricsInsets()
	return max(1, p.height-topInset-bottomInset)
}

func (p *playbackScreen) lyricsInsets() (int, int) {
	topInset := lipgloss.Height(p.paneOverlay())
	if p.showInfo {
		topInset += 1 + lipgloss.Height(p.infoOverlay())
	}
	bottomInset := lipgloss.Height(p.controlsOverlay())
	return topInset, bottomInset
}

func (s *lyricsLookupState) complete(document *lyrics.Document, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.document = document
	s.err = err
	s.done = true
}

func (s *lyricsLookupState) snapshot() (*lyrics.Document, error, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.document, s.err, s.done
}

func (p *playbackScreen) refreshVisualization(width, height int) {
	if p.services.Visualization == nil {
		p.visualContent = ""
		p.visualErr = nil
		return
	}
	content, err := p.services.Visualization.Placeholder(p.pane, width, height)
	p.visualContent = content
	p.visualErr = err
}

func (p *playbackScreen) refreshSnapshot() {
	if p.services.Playback == nil {
		p.snapshot.Volume = clamp(p.snapshot.Volume, 0, 100)
		return
	}
	snapshot := p.services.Playback.Snapshot()
	snapshot.Volume = clamp(snapshot.Volume, 0, 100)
	p.snapshot = snapshot
}

func (p *playbackScreen) adjustVolume(delta int) string {
	if p.services.Playback != nil {
		return ""
	} else {
		p.snapshot.Volume = clamp(p.snapshot.Volume+delta, 0, 100)
	}
	return fmt.Sprintf("Volume set to %d%%.", p.snapshot.Volume)
}

func (p *playbackScreen) accumulateSeek(delta time.Duration) string {
	if p.services.Playback == nil || p.snapshot.Track == nil || p.pending {
		return ""
	}
	p.seekAdjustment += delta
	p.seekDeadline = time.Now().Add(playbackSeekDebounce)
	seconds := int(p.seekAdjustment / time.Second)
	if seconds >= 0 {
		return fmt.Sprintf("Seek +%ds queued.", seconds)
	}
	return fmt.Sprintf("Seek %ds queued.", seconds)
}

func (p *playbackScreen) runPlaybackAction(run func(PlaybackService) error, status func(PlaybackSnapshot) string) tea.Cmd {
	if p.services.Playback == nil || p.pending {
		return nil
	}
	p.seekAdjustment = 0
	p.seekDeadline = time.Time{}
	p.pending = true
	playback := p.services.Playback
	return func() tea.Msg {
		if err := run(playback); err != nil {
			return playbackActionResult{err: err}
		}
		snapshot := playback.Snapshot()
		return playbackActionResult{
			snapshot: snapshot,
			status:   status(snapshot),
		}
	}
}

func paneHint(pane PlaybackPane) string {
	switch pane {
	case PaneLyrics:
		return "auto-follow synced LRC; scroll plain lyrics with up/down/pgup/pgdn"
	case PaneEQ:
		return "live spectrum bars driven by the audio runtime"
	case PaneVisualizer:
		return "mirrored live spectrum driven by the audio runtime"
	default:
		return "album-art-first playback layout"
	}
}

func playState(paused bool) string {
	if paused {
		return "paused"
	}
	return "playing"
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
