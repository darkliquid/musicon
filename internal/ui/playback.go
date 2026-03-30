package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/darkliquid/musicon/pkg/components"
)

type playbackScreen struct {
	services  Services
	width     int
	height    int
	pane      PlaybackPane
	showInfo  bool
	snapshot  PlaybackSnapshot
	status    string
	artStatus string
}

func newPlaybackScreen(services Services) *playbackScreen {
	screen := &playbackScreen{
		services: services,
		pane:     PaneArtwork,
		snapshot: PlaybackSnapshot{Volume: 60},
		status:   "Playback mode ready. Connect a playback backend to drive live state.",
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

	switch keypress.String() {
	case "v":
		p.pane = (p.pane + 1) % 4
		return fmt.Sprintf("Playback pane: %s", p.pane.String())
	case "i":
		p.showInfo = !p.showInfo
		if p.showInfo {
			return "Track information shown."
		}
		return "Track information hidden."
	case "r":
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
	case "s":
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
	case "space":
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
	case "[":
		if p.services.Playback != nil {
			if err := p.services.Playback.Previous(); err != nil {
				return err.Error()
			}
			p.refreshSnapshot()
			return "Moved to the previous track."
		}
		return "Previous track requires a playback backend."
	case "]":
		if p.services.Playback != nil {
			if err := p.services.Playback.Next(); err != nil {
				return err.Error()
			}
			p.refreshSnapshot()
			return "Moved to the next track."
		}
		return "Next track requires a playback backend."
	case "-":
		return p.adjustVolume(-5)
	case "=", "+":
		return p.adjustVolume(5)
	case "left":
		return p.seek(-5 * time.Second)
	case "right":
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
	infoHeight := 0
	if p.showInfo {
		infoHeight = 7
	}
	controlsHeight := 7
	centerHeight := p.height - controlsHeight - infoHeight - 1
	if centerHeight < 6 {
		centerHeight = 6
	}

	center := components.RenderPanel(components.PanelOptions{
		Title:    fmt.Sprintf("Center pane · %s", p.pane.String()),
		Subtitle: paneHint(p.pane),
		Width:    p.width,
		Height:   centerHeight,
		Focused:  true,
	}, p.centerView(p.width-4, centerHeight-3))

	controls := components.RenderPanel(components.PanelOptions{
		Title:    "Playback controls",
		Subtitle: "space pause · [ ] skip · ← → seek · -/+ volume",
		Width:    p.width,
		Height:   controlsHeight,
		Focused:  false,
	}, p.controlsView(p.width-4))

	sections := []string{center, controls}
	if p.showInfo {
		sections = append(sections, components.RenderPanel(components.PanelOptions{
			Title:    "Track info",
			Subtitle: "toggle with i",
			Width:    p.width,
			Height:   infoHeight,
			Focused:  false,
		}, p.infoView(p.width-4, infoHeight-3)))
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (p *playbackScreen) HelpView() string {
	return components.RenderPanel(components.PanelOptions{
		Title:    "Playback help",
		Subtitle: "dedicated playback mode",
		Width:    p.width,
		Height:   p.height,
		Focused:  true,
	}, strings.Join([]string{
		"space             toggle play / pause state",
		"[ / ]             previous / next track request",
		"left / right      scrub backward / forward by five seconds",
		"- / +             lower / raise volume",
		"v                 cycle artwork, lyrics, eq, and visualizer panes",
		"i                 show or hide track information",
		"r / s             toggle repeat and stream continuation flags",
		"tab               switch back to queue mode",
		"?                 toggle this help view",
	}, "\n"))
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
		lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render("visual mode: v · info: i · help: ? · mode toggle: tab"),
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
	if p.snapshot.Track != nil {
		trackID = p.snapshot.Track.ID
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
		if p.services.Artwork != nil && trackID != "" {
			if artwork, err := p.services.Artwork.Artwork(trackID, width, height); err == nil && strings.TrimSpace(artwork) != "" {
				return lipgloss.NewStyle().Width(width).Height(height).Align(lipgloss.Center, lipgloss.Center).Render(artwork)
			}
		}
		return components.RenderEmptyState(width, height, "Album art", "Artwork will appear here when an artwork provider is connected. Until then this pane defines the layout and focus of playback mode.")
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
