package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	bubblekey "github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/darkliquid/musicon/pkg/components"
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

	lyricsTrackID string
	lyricsLines   []string
	lyricsErr     error

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

func (p *playbackScreen) SetSize(width, height int) {
	p.width = max(1, width)
	p.height = max(1, height)
}

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

func (p *playbackScreen) View() string {
	if p.width <= 0 || p.height <= 0 {
		return ""
	}

	body := lipgloss.NewStyle().Width(p.width).Height(p.height).Render(p.centerView(p.width, p.height))
	top := p.paneOverlay()
	body = bottomOverlay(body, p.controlsOverlay(), p.width, p.height)
	if p.showInfo {
		top = lipgloss.JoinVertical(lipgloss.Center, top, "", p.infoOverlay())
	}
	body = topOverlay(body, top, p.width, p.height)
	return body
}

func (p *playbackScreen) HelpView() string {
	width := min(p.width, 68)
	height := min(p.height, 13)
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
	trackID := ""
	var trackInfo *TrackInfo
	if p.snapshot.Track != nil {
		trackInfo = p.snapshot.Track
		trackID = trackInfo.ID
	}

	switch p.pane {
	case PaneLyrics:
		p.refreshLyrics(trackID)
		if len(p.lyricsLines) > 0 {
			content := strings.Join(p.lyricsLines, "\n")
			return lipgloss.NewStyle().Width(width).Height(height).Render(content)
		}
		return components.RenderEmptyState(width, height, "Lyrics unavailable", "Hook up a lyrics provider to replace this placeholder with scrollable lyric content.")
	case PaneEQ, PaneVisualizer:
		p.refreshVisualization(width, height)
		if strings.TrimSpace(p.visualContent) != "" {
			return lipgloss.NewStyle().Width(width).Height(height).Render(p.visualContent)
		}
		return components.RenderEmptyState(width, height, p.pane.String()+" placeholder", "Attach a visualization provider to render live analysis inside this pane.")
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
		body := "Artwork will appear here when an artwork provider is connected. Until then this pane defines the layout and focus of playback mode."
		if strings.TrimSpace(p.artStatus) != "" {
			body = p.artStatus
		}
		return components.RenderEmptyState(width, height, "Album art", body)
	}
}

func (p *playbackScreen) refreshArtwork(trackInfo *TrackInfo) {
	p.artStatus = ""
	if p.services.Artwork == nil || trackInfo == nil {
		p.clearArtworkCache()
		return
	}
	key := artworkCacheKey(trackInfo)
	if key != p.artworkTrackKey {
		source, err := p.services.Artwork.Artwork(trackInfo.CoverArtMetadata())
		p.artworkTrackKey = key
		p.artworkSource = source
		p.artworkErr = err
		p.artwork.SetSource(source)
		return
	}
}

func (p *playbackScreen) clearArtworkCache() {
	p.artworkTrackKey = ""
	p.artworkSource = nil
	p.artworkErr = nil
	p.artwork.SetSource(nil)
}

func artworkCacheKey(track *TrackInfo) string {
	if track == nil {
		return ""
	}
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s",
		track.ID,
		track.Source,
		track.Title,
		track.Artist,
		track.Album,
	)
}

func (p *playbackScreen) refreshLyrics(trackID string) {
	if p.services.Lyrics == nil || trackID == "" {
		p.lyricsTrackID = ""
		p.lyricsLines = nil
		p.lyricsErr = nil
		return
	}
	if trackID == p.lyricsTrackID {
		return
	}
	lines, err := p.services.Lyrics.Lyrics(trackID)
	p.lyricsTrackID = trackID
	p.lyricsLines = append([]string(nil), lines...)
	p.lyricsErr = err
}

func (p *playbackScreen) refreshVisualization(width, height int) {
	if p.services.Visualization == nil {
		p.visualKey = ""
		p.visualContent = ""
		p.visualErr = nil
		return
	}
	key := fmt.Sprintf("%s:%dx%d", p.pane.String(), width, height)
	if key == p.visualKey {
		return
	}
	content, err := p.services.Visualization.Placeholder(p.pane, width, height)
	p.visualKey = key
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
		return "scrollable when lyrics exist"
	case PaneEQ:
		return "analysis-ready empty state"
	case PaneVisualizer:
		return "visualizer-ready empty state"
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
