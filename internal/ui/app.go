package ui

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/darkliquid/musicon/pkg/components"
	"golang.org/x/term"
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
	services       Services
	options        Options
	width          int
	height         int
	cellWidthRatio float64
	mode           Mode
	showHelp       bool
	status         string
	queue          *queueScreen
	playback       *playbackScreen
	viewport       components.SquareViewport
}

// NewApp constructs the Bubble Tea application shell with injected UI-facing services.
func NewApp(services Services, options Options) *App {
	options = normalizedOptions(options)
	model := &rootModel{
		services:       services,
		options:        options,
		cellWidthRatio: options.CellWidthRatio,
		mode:           options.StartMode,
		status:         "Ready. tab switches modes, ? toggles help, ctrl+c exits.",
	}
	model.queue = newQueueScreen(services)
	model.playback = newPlaybackScreen(services, options.AlbumArt)
	width, height := initialTerminalSize()

	return &App{
		program: tea.NewProgram(model, tea.WithWindowSize(width, height)),
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
	return tea.Batch(
		requestWindowSizeCmd(),
		tickCmd(),
	)
}

func requestWindowSizeCmd() tea.Cmd {
	return func() tea.Msg { return tea.RequestWindowSize() }
}

func initialTerminalSize() (int, int) {
	if width, height, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 && height > 0 {
		return width, height
	}

	if width, height, ok := terminalSizeFromEnv(); ok {
		return width, height
	}

	return 80, 24
}

func terminalSizeFromEnv() (int, int, bool) {
	width, widthOK := parsePositiveEnvInt("COLUMNS")
	height, heightOK := parsePositiveEnvInt("LINES")
	if !widthOK || !heightOK {
		return 0, 0, false
	}
	return width, height, true
}

func parsePositiveEnvInt(key string) (int, bool) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, false
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, false
	}

	return value, true
}

func (m *rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tickMsg:
		return m, tickCmd()
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		m.viewport = components.ClampSquareWithCellWidthRatio(m.width, m.height, m.cellWidthRatio)
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

	bodyWidth := max(1, m.viewport.Width)
	bodyHeight := max(1, m.viewport.Height)
	m.queue.SetSize(bodyWidth, bodyHeight)
	m.playback.SetSize(bodyWidth, bodyHeight)

	body := lipgloss.NewStyle().Width(bodyWidth).Height(bodyHeight).Render(m.activeBody())
	if m.showHelp {
		body = centeredOverlay(body, m.activeHelpOverlay(), bodyWidth, bodyHeight)
	}

	return m.makeView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body), m.terminalTitle())
}

func (m *rootModel) makeView(content, title string) tea.View {
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = sanitizeTitle(title)
	return view
}

func (m *rootModel) activeBody() string {
	if m.mode == ModeQueue {
		return m.queue.View()
	}
	return m.playback.View()
}

func (m *rootModel) resizeScreens() {
	m.queue.SetSize(max(1, m.viewport.Width), max(1, m.viewport.Height))
	m.playback.SetSize(max(1, m.viewport.Width), max(1, m.viewport.Height))
}

func (m *rootModel) activeHelpOverlay() string {
	if m.mode == ModeQueue {
		return m.queue.HelpView()
	}
	return m.playback.HelpView()
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
	return minimumViewportRequirements.CheckWithCellWidthRatio(m.width, m.height, m.cellWidthRatio)
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

func normalizedOptions(options Options) Options {
	if options.CellWidthRatio <= 0 {
		options.CellWidthRatio = terminalCellWidthRatio()
	}
	if options.Theme == "" {
		options.Theme = "default"
	}
	switch options.StartMode {
	case ModePlayback:
	default:
		options.StartMode = ModeQueue
	}
	if options.AlbumArt.FillMode == "" {
		options.AlbumArt.FillMode = "fill"
	}
	if options.AlbumArt.Protocol == "" {
		options.AlbumArt.Protocol = "halfblocks"
	}
	return options
}

func terminalCellWidthRatio() float64 {
	if ratio, ok := parsePositiveEnvFloat("MUSICON_CELL_WIDTH_RATIO"); ok {
		return ratio
	}
	return components.TerminalCellWidthRatio()
}

func parsePositiveEnvFloat(key string) (float64, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}
