package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/darkliquid/musicon/pkg/components"
)

var minimumViewportRequirements = components.SizeRequirements{
	MinWidth:  20,
	MinHeight: 20,
	MinSquare: 20,
}

type App struct {
	program *tea.Program
}

type tickMsg time.Time

type rootModel struct {
	services Services
	width    int
	height   int
	mode     Mode
	showHelp bool
	status   string
	queue    *queueScreen
	playback *playbackScreen
	viewport components.SquareViewport
}

// NewApp constructs the Bubble Tea application shell with injected UI-facing services.
func NewApp(services Services) *App {
	model := &rootModel{
		services: services,
		mode:     ModeQueue,
		status:   "Ready. tab switches modes, ? toggles help, ctrl+c exits.",
	}
	model.queue = newQueueScreen(services)
	model.playback = newPlaybackScreen(services)

	return &App{
		program: tea.NewProgram(model),
	}
}

// Run starts the terminal UI and returns any startup or runtime error.
func Run(app *App) error {
	if app == nil || app.program == nil {
		return errors.New("ui app is not initialized")
	}
	_, err := app.program.Run()
	return err
}

func (m *rootModel) Init() tea.Cmd {
	return tickCmd()
}

func (m *rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tickMsg:
		return m, tickCmd()
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		m.viewport = components.ClampSquare(m.width, m.height)
		m.updateSizeStatus()
		m.resizeScreens()
		return m, nil
	case tea.KeyPressMsg:
		switch typed.String() {
		case "ctrl+c":
			return m, tea.Quit
		}

		if !m.layoutCheck().Fits() {
			return m, nil
		}

		switch typed.String() {
		case "tab":
			m.toggleMode()
			return m, nil
		case "?":
			m.showHelp = !m.showHelp
			if m.showHelp {
				m.status = fmt.Sprintf("%s help shown.", m.mode.String())
			} else {
				m.status = fmt.Sprintf("%s help hidden.", m.mode.String())
			}
			return m, nil
		}
	}

	if m.mode == ModeQueue {
		if status := m.queue.Update(msg); status != "" {
			m.status = status
		}
	} else {
		if status := m.playback.Update(msg); status != "" {
			m.status = status
		}
	}

	return m, nil
}

func (m *rootModel) View() tea.View {
	if m.width <= 0 || m.height <= 0 {
		return m.makeView("loading terminal dimensions...", "Musicon - Starting")
	}

	check := m.layoutCheck()
	m.viewport = check.Viewport
	if !check.Fits() {
		message := lipgloss.JoinVertical(
			lipgloss.Center,
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Render("terminal too small"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render("Musicon needs a minimum 20×20 terminal to preserve the square UI."),
			lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(m.sizeFailureDetail(check)),
			lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("Resize the terminal. Only ctrl+c is active until the minimum is met."),
		)
		message = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("52")).
			Padding(1, 2).
			Render(message)
		title := fmt.Sprintf("Musicon - Resize Required (%dx%d)", check.Viewport.TerminalWidth, check.Viewport.TerminalHeight)
		return m.makeView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, message), title)
	}

	frameWidth, frameHeight := m.viewport.Inner(1)
	frameWidth = max(1, frameWidth)
	frameHeight = max(1, frameHeight)

	headerHeight := 3
	footerHeight := 2
	bodyHeight := frameHeight - headerHeight - footerHeight
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	m.queue.SetSize(frameWidth, bodyHeight)
	m.playback.SetSize(frameWidth, bodyHeight)

	body := m.activeBody()
	frame := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(lipgloss.Color("63")).
		Width(frameWidth).
		Height(frameHeight).
		Render(lipgloss.JoinVertical(
			lipgloss.Left,
			lipgloss.NewStyle().Width(frameWidth).Height(headerHeight).Render(m.renderHeader(frameWidth)),
			lipgloss.NewStyle().Width(frameWidth).Height(bodyHeight).Render(body),
			lipgloss.NewStyle().Width(frameWidth).Height(footerHeight).Render(m.renderFooter(frameWidth)),
		))

	return m.makeView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, frame), m.terminalTitle())
}

func (m *rootModel) makeView(content, title string) tea.View {
	view := tea.NewView(titleSequence(title) + content)
	view.AltScreen = true
	return view
}

func (m *rootModel) activeBody() string {
	if m.showHelp {
		if m.mode == ModeQueue {
			return m.queue.HelpView()
		}
		return m.playback.HelpView()
	}
	if m.mode == ModeQueue {
		return m.queue.View()
	}
	return m.playback.View()
}

func (m *rootModel) resizeScreens() {
	frameWidth, frameHeight := m.viewport.Inner(1)
	headerHeight := 3
	footerHeight := 2
	bodyHeight := max(1, frameHeight-headerHeight-footerHeight)
	m.queue.SetSize(max(1, frameWidth), bodyHeight)
	m.playback.SetSize(max(1, frameWidth), bodyHeight)
}

func (m *rootModel) toggleMode() {
	if m.mode == ModeQueue {
		m.mode = ModePlayback
	} else {
		m.mode = ModeQueue
	}
	m.status = fmt.Sprintf("Switched to %s mode.", m.mode.String())
}

func (m *rootModel) layoutCheck() components.SizeCheck {
	return minimumViewportRequirements.Check(m.width, m.height)
}

func (m *rootModel) updateSizeStatus() {
	check := m.layoutCheck()
	if !check.Fits() {
		m.status = "Resize terminal to at least 20×20 to continue using Musicon."
		return
	}

	if m.status == "" || strings.HasPrefix(m.status, "Resize terminal to at least 20×20") {
		m.status = "Terminal size accepted. Ready. tab switches modes, ? toggles help, ctrl+c exits."
	}
}

func (m *rootModel) sizeFailureDetail(check components.SizeCheck) string {
	return fmt.Sprintf(
		"Current: %d×%d, square viewport %d. Missing: +%d cols, +%d rows, +%d square cells.",
		check.Viewport.TerminalWidth,
		check.Viewport.TerminalHeight,
		check.Viewport.Size,
		check.MissingWidth(),
		check.MissingHeight(),
		check.MissingSquare(),
	)
}

func (m *rootModel) renderHeader(width int) string {
	left := lipgloss.JoinHorizontal(lipgloss.Left,
		pill("tab Queue", m.mode == ModeQueue),
		pill("tab Playback", m.mode == ModePlayback),
	)
	right := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Align(lipgloss.Right).Render("musicon · square viewport")
	line := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().Width(max(1, width-lipgloss.Width(right)-1)).Render(left),
		" ",
		right,
	)
	modeSummary := "Queue: search, filter, inspect, and curate the active queue."
	if m.mode == ModePlayback {
		modeSummary = "Playback: artwork-first controls, scrubber, help, and alternate visual panes."
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		line,
		lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Width(width).Render(truncate(modeSummary, width)),
	)
}

func (m *rootModel) renderFooter(width int) string {
	hint := "ctrl+c exit · ? help"
	statusWidth := width - lipgloss.Width(hint) - 1
	if statusWidth < 8 {
		statusWidth = width
		hint = ""
	}
	line := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Width(statusWidth).Render(truncate(m.status, statusWidth))
	if hint == "" {
		return line
	}
	return lipgloss.JoinHorizontal(lipgloss.Left,
		line,
		" ",
		lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(hint),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *rootModel) terminalTitle() string {
	segments := []string{"Musicon", m.mode.String()}
	if m.showHelp {
		segments = append(segments, "Help")
	}

	snapshot := m.playbackSnapshot()
	if snapshot.Track == nil {
		segments = append(segments, "Idle")
		return strings.Join(segments, " - ")
	}

	track := snapshot.Track.Title
	if artist := strings.TrimSpace(snapshot.Track.Artist); artist != "" {
		track = artist + " - " + track
	}
	state := "Playing"
	if snapshot.Paused {
		state = "Paused"
	}

	segments = append(segments, sanitizeTitle(track), state)
	return strings.Join(segments, " - ")
}

func (m *rootModel) playbackSnapshot() PlaybackSnapshot {
	if m.playback == nil {
		return PlaybackSnapshot{}
	}
	m.playback.refreshSnapshot()
	return m.playback.snapshot
}

func titleSequence(title string) string {
	return "\x1b]2;" + sanitizeTitle(title) + "\x07"
}

func sanitizeTitle(title string) string {
	replacer := strings.NewReplacer(
		"\x1b", "",
		"\x07", "",
		"\n", " ",
		"\r", " ",
		"\t", " ",
	)
	return strings.TrimSpace(replacer.Replace(title))
}
