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
	snapshot  PlaybackSnapshot
	status    string
	artStatus string
	artwork   *components.TerminalImage
}

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

func (p *playbackScreen) Update(msg tea.Msg) string {
	keypress, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return ""
	}

	switch {
	case bubblekey.Matches(keypress, p.keymap.CyclePane):
		p.pane = (p.pane + 1) % 4
		return fmt.Sprintf("Playback pane: %s", p.pane.String())
	case bubblekey.Matches(keypress, p.keymap.ToggleInfo):
		p.showInfo = !p.showInfo
		if p.showInfo {
			return "Track information shown."
		}
		return "Track information hidden."
	case bubblekey.Matches(keypress, p.keymap.ToggleRepeat):
		if p.services.Playback != nil {
			next := !p.snapshot.Repeat
			if err := p.services.Playback.SetRepeat(next); err != nil {
				return err.Error()
			}
			p.refreshSnapshot()
		} else {
			p.snapshot.Repeat = !p.snapshot.Repeat
		}
		return fmt.Sprintf("Repeat %s.", onOff(p.snapshot.Repeat))
	case bubblekey.Matches(keypress, p.keymap.ToggleStream):
		if p.services.Playback != nil {
			next := !p.snapshot.Stream
			if err := p.services.Playback.SetStream(next); err != nil {
				return err.Error()
			}
			p.refreshSnapshot()
		} else {
			p.snapshot.Stream = !p.snapshot.Stream
		}
		return fmt.Sprintf("Stream continuation %s.", onOff(p.snapshot.Stream))
	case bubblekey.Matches(keypress, p.keymap.TogglePause):
		if p.services.Playback != nil {
			if err := p.services.Playback.TogglePause(); err != nil {
				return err.Error()
			}
			p.refreshSnapshot()
		} else {
			p.snapshot.Paused = !p.snapshot.Paused
		}
		if p.snapshot.Paused {
			return "Playback paused."
		}
		return "Playback resumed."
	case bubblekey.Matches(keypress, p.keymap.PreviousTrack):
		if p.services.Playback != nil {
			if err := p.services.Playback.Previous(); err != nil {
				return err.Error()
			}
			p.refreshSnapshot()
			return "Moved to the previous track."
		}
		return "Previous track requires a playback backend."
	case bubblekey.Matches(keypress, p.keymap.NextTrack):
		if p.services.Playback != nil {
			if err := p.services.Playback.Next(); err != nil {
				return err.Error()
			}
			p.refreshSnapshot()
			return "Moved to the next track."
		}
		return "Next track requires a playback backend."
	case bubblekey.Matches(keypress, p.keymap.VolumeDown):
		return p.adjustVolume(-5)
	case bubblekey.Matches(keypress, p.keymap.VolumeUp):
		return p.adjustVolume(5)
	case bubblekey.Matches(keypress, p.keymap.SeekBackward):
		return p.seek(-5 * time.Second)
	case bubblekey.Matches(keypress, p.keymap.SeekForward):
		return p.seek(5 * time.Second)
	default:
		return ""
	}
}

func (p *playbackScreen) View() string {
	if p.width <= 0 || p.height <= 0 {
		return ""
	}

	p.refreshSnapshot()
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
		helpLinePair(p.keymap.SeekBackward, p.keymap.SeekForward, "scrub backward / forward by five seconds"),
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
		Subtitle: fmt.Sprintf("%s pause · %s / %s next/prev · %s / %s seek",
			bindingLabel(p.keymap.TogglePause),
			bindingLabel(p.keymap.PreviousTrack),
			bindingLabel(p.keymap.NextTrack),
			bindingLabel(p.keymap.SeekBackward),
			bindingLabel(p.keymap.SeekForward),
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
		if p.services.Lyrics != nil && trackID != "" {
			lines, err := p.services.Lyrics.Lyrics(trackID)
			if err == nil && len(lines) > 0 {
				content := strings.Join(lines, "\n")
				return lipgloss.NewStyle().Width(width).Height(height).Render(content)
			}
		}
		return components.RenderEmptyState(width, height, "Lyrics unavailable", "Hook up a lyrics provider to replace this placeholder with scrollable lyric content.")
	case PaneEQ, PaneVisualizer:
		if p.services.Visualization != nil {
			if content, err := p.services.Visualization.Placeholder(p.pane, width, height); err == nil && strings.TrimSpace(content) != "" {
				return lipgloss.NewStyle().Width(width).Height(height).Render(content)
			}
		}
		return components.RenderEmptyState(width, height, p.pane.String()+" placeholder", "Attach a visualization provider to render live analysis inside this pane.")
	default:
		p.artStatus = ""
		p.artwork.SetSize(width, height)
		if p.services.Artwork != nil && trackInfo != nil {
			source, err := p.services.Artwork.Artwork(trackInfo.CoverArtMetadata())
			if err != nil {
				p.artStatus = err.Error()
				p.artwork.SetSource(nil)
			} else {
				p.artwork.SetSource(source)
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
				} else if source != nil && strings.TrimSpace(source.Description) != "" {
					p.artStatus = source.Description
				}
			}
		} else {
			p.artwork.SetSource(nil)
		}
		body := "Artwork will appear here when an artwork provider is connected. Until then this pane defines the layout and focus of playback mode."
		if strings.TrimSpace(p.artStatus) != "" {
			body = p.artStatus
		}
		return components.RenderEmptyState(width, height, "Album art", body)
	}
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
		if err := p.services.Playback.AdjustVolume(delta); err != nil {
			return err.Error()
		}
		p.refreshSnapshot()
	} else {
		p.snapshot.Volume = clamp(p.snapshot.Volume+delta, 0, 100)
	}
	return fmt.Sprintf("Volume set to %d%%.", p.snapshot.Volume)
}

func (p *playbackScreen) seek(delta time.Duration) string {
	if p.services.Playback != nil {
		if err := p.services.Playback.Seek(delta); err != nil {
			return err.Error()
		}
		p.refreshSnapshot()
	} else {
		p.snapshot.Position += delta
		if p.snapshot.Position < 0 {
			p.snapshot.Position = 0
		}
		if p.snapshot.Duration > 0 && p.snapshot.Position > p.snapshot.Duration {
			p.snapshot.Position = p.snapshot.Duration
		}
	}
	return fmt.Sprintf("Scrubber moved to %s.", formatDuration(p.snapshot.Position))
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
