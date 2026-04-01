package ui

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/darkliquid/musicon/pkg/components"
	"golang.org/x/term"
)

var minimumViewportRequirements = components.SizeRequirements{
	MinWidth:  20,
	MinHeight: 20,
	MinSquare: 20,
}

// App owns the Bubble Tea program used to run Musicon's terminal UI.
type App struct {
	program *tea.Program
}

type tickMsg time.Time

type rootModel struct {
	services          Services
	options           Options
	sessionStore      SessionStore
	theme             components.Theme
	width             int
	height            int
	cellWidthRatio    float64
	mode              Mode
	showHelp          bool
	status            string
	keymap            KeyMap
	queue             *queueScreen
	playback          *playbackScreen
	viewport          components.SquareViewport
	lastPersistedJSON string
	lastPersistAt     time.Time
}

// NewApp constructs the Bubble Tea application shell with injected UI-facing services.
func NewApp(services Services, options Options) *App {
	options = normalizedOptions(options)
	model := &rootModel{
		services:       services,
		options:        options,
		sessionStore:   options.SessionStore,
		theme:          options.Theme,
		cellWidthRatio: options.CellWidthRatio,
		mode:           options.StartMode,
		keymap:         normalizedKeyMap(options.Keybinds),
	}
	model.status = readyStatus(model.keymap.Global)
	model.queue = newQueueScreenWithThemeAndKeyMap(services, model.theme, model.keymap.Queue)
	model.playback = newPlaybackScreenWithThemeAndKeyMap(services, model.theme, options.AlbumArt, model.keymap.Playback)
	model.applyRestoredSession(options.Restore)
	model.rememberRestoredSession()
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

// Init requests the initial window size and starts the UI redraw tick.
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

// Update routes Bubble Tea messages through the active screen and root shell state.
func (m *rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmdBatch []tea.Cmd
	allowPersistThrottle := false
	skipScreenUpdate := false

HandleMsg:
	switch typed := msg.(type) {
	case tickMsg:
		if m.playback != nil {
			m.playback.refreshSnapshot()
		}
		cmdBatch = append(cmdBatch, tickCmd())
		allowPersistThrottle = true
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		m.viewport = components.ClampSquareWithCellWidthRatio(m.width, m.height, m.cellWidthRatio)
		m.updateSizeStatus()
		m.resizeScreens()
	case tea.KeyPressMsg:
		switch {
		case key.Matches(typed, m.keymap.Global.Quit):
			if err := m.persistSession(true, false); err != nil {
				m.status = fmt.Sprintf("Failed to save session state: %v", err)
				return m, nil
			}
			skipScreenUpdate = true
			cmdBatch = append(cmdBatch, tea.Quit)
			break HandleMsg
		}

		if !m.layoutCheck().Fits() {
			break HandleMsg
		}

		switch {
		case key.Matches(typed, m.keymap.Global.ToggleMode):
			m.toggleMode()
			skipScreenUpdate = true
		case key.Matches(typed, m.keymap.Global.ToggleHelp):
			m.showHelp = !m.showHelp
			if m.showHelp {
				m.status = fmt.Sprintf("%s help shown.", m.mode.String())
			} else {
				m.status = fmt.Sprintf("%s help hidden.", m.mode.String())
			}
		}
	}

	if !skipScreenUpdate {
		switch m.mode {
		case ModeQueue:
			status, cmd := m.queue.Update(msg)
			if status != "" {
				m.status = status
			}
			cmdBatch = append(cmdBatch, cmd)
		case ModePlayback:
			status, cmd := m.playback.Update(msg)
			if status != "" {
				m.status = status
			}
			cmdBatch = append(cmdBatch, cmd)
		}
	}

	if err := m.persistSession(false, allowPersistThrottle); err != nil {
		m.status = fmt.Sprintf("Failed to save session state: %v", err)
	}

	return m, tea.Batch(cmdBatch...)
}

// View renders the active square viewport or the current resize/loading state.
func (m *rootModel) View() tea.View {
	if m.width <= 0 || m.height <= 0 {
		return m.makeView("loading terminal dimensions...", "Musicon - Starting")
	}

	check := m.layoutCheck()
	m.viewport = check.Viewport
	if !check.Fits() {
		theme := m.theme.Normalize()
		message := lipgloss.JoinVertical(
			lipgloss.Center,
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.OnWarning)).Render("terminal too small"),
			lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Text)).Render("Musicon needs a minimum 20×20 terminal to preserve the square UI."),
			lipgloss.NewStyle().Foreground(lipgloss.Color(theme.TextSubtle)).Render(m.sizeFailureDetail(check)),
			lipgloss.NewStyle().Foreground(lipgloss.Color(theme.TextSubtle)).Render("Resize the terminal. Only ctrl+c is active until the minimum is met."),
		)
		message = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.OnWarning)).
			Background(lipgloss.Color(theme.Warning)).
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
		m.status = "Terminal size accepted. " + readyStatus(m.keymap.Global)
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
		pill("tab Queue", m.mode == ModeQueue, m.theme),
		pill("tab Playback", m.mode == ModePlayback, m.theme),
	)
	right := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Normalize().TextSubtle)).Align(lipgloss.Right).Render("musicon · square viewport")
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
		lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Normalize().TextMuted)).Width(width).Render(truncate(modeSummary, width)),
	)
}

func (m *rootModel) renderFooter(width int) string {
	hint := "ctrl+c exit · ? help"
	statusWidth := width - lipgloss.Width(hint) - 1
	if statusWidth < 8 {
		statusWidth = width
		hint = ""
	}
	theme := m.theme.Normalize()
	line := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Text)).Width(statusWidth).Render(truncate(m.status, statusWidth))
	if hint == "" {
		return line
	}
	return lipgloss.JoinHorizontal(lipgloss.Left,
		line,
		" ",
		lipgloss.NewStyle().Foreground(lipgloss.Color(theme.TextSubtle)).Render(hint),
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
	providedKeybinds := options.Keybinds
	if options.CellWidthRatio <= 0 {
		options.CellWidthRatio = terminalCellWidthRatio()
	}
	options.Theme = options.Theme.Normalize()
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
	options.Keybinds = mergeKeybindOptions(defaultKeybindOptions(), providedKeybinds)
	return options
}

func readyStatus(keymap GlobalKeyMap) string {
	return fmt.Sprintf("Ready. %s switches modes, %s toggles help, %s exits.",
		bindingLabel(keymap.ToggleMode),
		bindingLabel(keymap.ToggleHelp),
		bindingLabel(keymap.Quit),
	)
}

func mergeKeybindOptions(defaults, provided KeybindOptions) KeybindOptions {
	merged := defaults

	if len(provided.Global.Quit) > 0 {
		merged.Global.Quit = append([]string(nil), provided.Global.Quit...)
	}
	if len(provided.Global.ToggleMode) > 0 {
		merged.Global.ToggleMode = append([]string(nil), provided.Global.ToggleMode...)
	}
	if len(provided.Global.ToggleHelp) > 0 {
		merged.Global.ToggleHelp = append([]string(nil), provided.Global.ToggleHelp...)
	}

	if len(provided.Queue.ToggleSearchFocus) > 0 {
		merged.Queue.ToggleSearchFocus = append([]string(nil), provided.Queue.ToggleSearchFocus...)
	}
	if len(provided.Queue.SourcePrev) > 0 {
		merged.Queue.SourcePrev = append([]string(nil), provided.Queue.SourcePrev...)
	}
	if len(provided.Queue.SourceNext) > 0 {
		merged.Queue.SourceNext = append([]string(nil), provided.Queue.SourceNext...)
	}
	if len(provided.Queue.CycleSearchMode) > 0 {
		merged.Queue.CycleSearchMode = append([]string(nil), provided.Queue.CycleSearchMode...)
	}
	if len(provided.Queue.ModeSongs) > 0 {
		merged.Queue.ModeSongs = append([]string(nil), provided.Queue.ModeSongs...)
	}
	if len(provided.Queue.ModeArtists) > 0 {
		merged.Queue.ModeArtists = append([]string(nil), provided.Queue.ModeArtists...)
	}
	if len(provided.Queue.ModeAlbums) > 0 {
		merged.Queue.ModeAlbums = append([]string(nil), provided.Queue.ModeAlbums...)
	}
	if len(provided.Queue.ModePlaylists) > 0 {
		merged.Queue.ModePlaylists = append([]string(nil), provided.Queue.ModePlaylists...)
	}
	if len(provided.Queue.ExpandSelected) > 0 {
		merged.Queue.ExpandSelected = append([]string(nil), provided.Queue.ExpandSelected...)
	}
	if len(provided.Queue.ActivateSelected) > 0 {
		merged.Queue.ActivateSelected = append([]string(nil), provided.Queue.ActivateSelected...)
	}
	if len(provided.Queue.MoveSelectedUp) > 0 {
		merged.Queue.MoveSelectedUp = append([]string(nil), provided.Queue.MoveSelectedUp...)
	}
	if len(provided.Queue.MoveSelectedDown) > 0 {
		merged.Queue.MoveSelectedDown = append([]string(nil), provided.Queue.MoveSelectedDown...)
	}
	if len(provided.Queue.ClearQueue) > 0 {
		merged.Queue.ClearQueue = append([]string(nil), provided.Queue.ClearQueue...)
	}
	if len(provided.Queue.RemoveSelected) > 0 {
		merged.Queue.RemoveSelected = append([]string(nil), provided.Queue.RemoveSelected...)
	}
	if len(provided.Queue.BrowserUp) > 0 {
		merged.Queue.BrowserUp = append([]string(nil), provided.Queue.BrowserUp...)
	}
	if len(provided.Queue.BrowserDown) > 0 {
		merged.Queue.BrowserDown = append([]string(nil), provided.Queue.BrowserDown...)
	}
	if len(provided.Queue.BrowserHome) > 0 {
		merged.Queue.BrowserHome = append([]string(nil), provided.Queue.BrowserHome...)
	}
	if len(provided.Queue.BrowserEnd) > 0 {
		merged.Queue.BrowserEnd = append([]string(nil), provided.Queue.BrowserEnd...)
	}
	if len(provided.Queue.BrowserPageUp) > 0 {
		merged.Queue.BrowserPageUp = append([]string(nil), provided.Queue.BrowserPageUp...)
	}
	if len(provided.Queue.BrowserPageDown) > 0 {
		merged.Queue.BrowserPageDown = append([]string(nil), provided.Queue.BrowserPageDown...)
	}

	if len(provided.Playback.CyclePane) > 0 {
		merged.Playback.CyclePane = append([]string(nil), provided.Playback.CyclePane...)
	}
	if len(provided.Playback.ToggleInfo) > 0 {
		merged.Playback.ToggleInfo = append([]string(nil), provided.Playback.ToggleInfo...)
	}
	if len(provided.Playback.ToggleRepeat) > 0 {
		merged.Playback.ToggleRepeat = append([]string(nil), provided.Playback.ToggleRepeat...)
	}
	if len(provided.Playback.ToggleStream) > 0 {
		merged.Playback.ToggleStream = append([]string(nil), provided.Playback.ToggleStream...)
	}
	if len(provided.Playback.TogglePause) > 0 {
		merged.Playback.TogglePause = append([]string(nil), provided.Playback.TogglePause...)
	}
	if len(provided.Playback.PreviousTrack) > 0 {
		merged.Playback.PreviousTrack = append([]string(nil), provided.Playback.PreviousTrack...)
	}
	if len(provided.Playback.NextTrack) > 0 {
		merged.Playback.NextTrack = append([]string(nil), provided.Playback.NextTrack...)
	}
	if len(provided.Playback.SeekBackward) > 0 {
		merged.Playback.SeekBackward = append([]string(nil), provided.Playback.SeekBackward...)
	}
	if len(provided.Playback.SeekForward) > 0 {
		merged.Playback.SeekForward = append([]string(nil), provided.Playback.SeekForward...)
	}
	if len(provided.Playback.VolumeDown) > 0 {
		merged.Playback.VolumeDown = append([]string(nil), provided.Playback.VolumeDown...)
	}
	if len(provided.Playback.VolumeUp) > 0 {
		merged.Playback.VolumeUp = append([]string(nil), provided.Playback.VolumeUp...)
	}

	return merged
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
