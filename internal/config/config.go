package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/darkliquid/musicon/pkg/components"
)

// This file defines the config schema plus the normalization pipeline that turns
// user TOML into startup-ready options for the rest of the application.

const (
	// DefaultFileName is the conventional base name for Musicon configuration files.
	DefaultFileName     = "musicon.toml"
	defaultStartMode    = "queue"
	defaultBackend      = "auto"
	defaultFillMode     = "fill"
	defaultProtocol     = "halfblocks"
	defaultSourceName   = "local"
	defaultYTResults    = 20
	defaultRadioURL     = "https://all.api.radio-browser.info"
	defaultRadioResults = 20
)

// Config describes the full TOML-backed Musicon configuration surface.
type Config struct {
	Audio    AudioConfig    `toml:"audio"`
	UI       UIConfig       `toml:"ui"`
	Keybinds KeybindsConfig `toml:"keybinds"`
	Sources  SourcesConfig  `toml:"sources"`
}

// KeybindsConfig groups configurable shortcuts for every screen.
type KeybindsConfig struct {
	Global   GlobalKeybindsConfig   `toml:"global"`
	Queue    QueueKeybindsConfig    `toml:"queue"`
	Playback PlaybackKeybindsConfig `toml:"playback"`
}

// GlobalKeybindsConfig declares bindings that are active in every UI mode.
type GlobalKeybindsConfig struct {
	Quit          []string `toml:"quit"`
	ToggleMode    []string `toml:"toggle_mode"`
	ToggleHelp    []string `toml:"toggle_help"`
	ToggleCompact []string `toml:"toggle_compact"`
}

// QueueKeybindsConfig declares shortcuts specific to queue mode.
type QueueKeybindsConfig struct {
	ToggleSearchFocus []string `toml:"toggle_search_focus"`
	SourcePrev        []string `toml:"source_prev"`
	SourceNext        []string `toml:"source_next"`
	CycleSearchMode   []string `toml:"cycle_search_mode"`
	ModeSongs         []string `toml:"mode_songs"`
	ModeArtists       []string `toml:"mode_artists"`
	ModeAlbums        []string `toml:"mode_albums"`
	ModePlaylists     []string `toml:"mode_playlists"`
	ExpandSelected    []string `toml:"expand_selected"`
	ActivateSelected  []string `toml:"activate_selected"`
	MoveSelectedUp    []string `toml:"move_selected_up"`
	MoveSelectedDown  []string `toml:"move_selected_down"`
	ClearQueue        []string `toml:"clear_queue"`
	RemoveSelected    []string `toml:"remove_selected"`
	BrowserUp         []string `toml:"browser_up"`
	BrowserDown       []string `toml:"browser_down"`
	BrowserHome       []string `toml:"browser_home"`
	BrowserEnd        []string `toml:"browser_end"`
	BrowserPageUp     []string `toml:"browser_page_up"`
	BrowserPageDown   []string `toml:"browser_page_down"`
}

// PlaybackKeybindsConfig declares shortcuts specific to playback mode.
type PlaybackKeybindsConfig struct {
	CyclePane     []string `toml:"cycle_pane"`
	ToggleInfo    []string `toml:"toggle_info"`
	ToggleRepeat  []string `toml:"toggle_repeat"`
	ToggleStream  []string `toml:"toggle_stream"`
	TogglePause   []string `toml:"toggle_pause"`
	PreviousTrack []string `toml:"previous_track"`
	NextTrack     []string `toml:"next_track"`
	SeekBackward  []string `toml:"seek_backward"`
	SeekForward   []string `toml:"seek_forward"`
	VolumeDown    []string `toml:"volume_down"`
	VolumeUp      []string `toml:"volume_up"`
}

// AudioConfig holds playback-runtime startup settings.
type AudioConfig struct {
	Backend string `toml:"backend"`
}

// UIConfig holds terminal UI startup settings.
type UIConfig struct {
	Theme          ThemeConfig    `toml:"theme"`
	StartMode      string         `toml:"start_mode"`
	CompactMode    bool           `toml:"compact_mode"`
	CellWidthRatio float64        `toml:"cell_width_ratio"`
	AlbumArt       AlbumArtConfig `toml:"album_art"`
}

// ThemeConfig holds semantic UI color roles and an optional external theme file.
type ThemeConfig struct {
	File           string `toml:"file"`
	Background     string `toml:"background"`
	Surface        string `toml:"surface"`
	SurfaceVariant string `toml:"surface_variant"`
	Primary        string `toml:"primary"`
	OnPrimary      string `toml:"on_primary"`
	Text           string `toml:"text"`
	TextMuted      string `toml:"text_muted"`
	TextSubtle     string `toml:"text_subtle"`
	Border         string `toml:"border"`
	Warning        string `toml:"warning"`
	OnWarning      string `toml:"on_warning"`
}

// UnmarshalTOML accepts both the new `[ui.theme]` table and the legacy
// `theme = "preset"` string form.
func (t *ThemeConfig) UnmarshalTOML(value any) error {
	switch typed := value.(type) {
	case string:
		*t = defaultThemeConfig()
		return nil
	case map[string]any:
		var decoded ThemeConfig
		if raw, ok := typed["file"]; ok {
			value, err := themeStringField(raw, "file")
			if err != nil {
				return err
			}
			decoded.File = value
		}
		if raw, ok := typed["background"]; ok {
			value, err := themeStringField(raw, "background")
			if err != nil {
				return err
			}
			decoded.Background = value
		}
		if raw, ok := typed["surface"]; ok {
			value, err := themeStringField(raw, "surface")
			if err != nil {
				return err
			}
			decoded.Surface = value
		}
		if raw, ok := typed["surface_variant"]; ok {
			value, err := themeStringField(raw, "surface_variant")
			if err != nil {
				return err
			}
			decoded.SurfaceVariant = value
		}
		if raw, ok := typed["primary"]; ok {
			value, err := themeStringField(raw, "primary")
			if err != nil {
				return err
			}
			decoded.Primary = value
		}
		if raw, ok := typed["on_primary"]; ok {
			value, err := themeStringField(raw, "on_primary")
			if err != nil {
				return err
			}
			decoded.OnPrimary = value
		}
		if raw, ok := typed["text"]; ok {
			value, err := themeStringField(raw, "text")
			if err != nil {
				return err
			}
			decoded.Text = value
		}
		if raw, ok := typed["text_muted"]; ok {
			value, err := themeStringField(raw, "text_muted")
			if err != nil {
				return err
			}
			decoded.TextMuted = value
		}
		if raw, ok := typed["text_subtle"]; ok {
			value, err := themeStringField(raw, "text_subtle")
			if err != nil {
				return err
			}
			decoded.TextSubtle = value
		}
		if raw, ok := typed["border"]; ok {
			value, err := themeStringField(raw, "border")
			if err != nil {
				return err
			}
			decoded.Border = value
		}
		if raw, ok := typed["warning"]; ok {
			value, err := themeStringField(raw, "warning")
			if err != nil {
				return err
			}
			decoded.Warning = value
		}
		if raw, ok := typed["on_warning"]; ok {
			value, err := themeStringField(raw, "on_warning")
			if err != nil {
				return err
			}
			decoded.OnWarning = value
		}
		*t = decoded
		return nil
	default:
		return fmt.Errorf("ui.theme must be a string or table, got %T", value)
	}
}

// AlbumArtConfig configures album-art rendering defaults.
type AlbumArtConfig struct {
	FillMode string `toml:"fill_mode"`
	Backend  string `toml:"backend"`
	Protocol string `toml:"protocol"`
}

// SourcesConfig groups source-specific startup settings.
type SourcesConfig struct {
	Local   LocalSourceConfig   `toml:"local"`
	YouTube YouTubeSourceConfig `toml:"youtube"`
	Radio   RadioSourceConfig   `toml:"radio"`
}

// LocalSourceConfig configures local-library discovery roots.
type LocalSourceConfig struct {
	Dirs []string `toml:"dirs"`
}

// YouTubeSourceConfig configures YouTube Music search and playback integration.
type YouTubeSourceConfig struct {
	Enabled            bool     `toml:"enabled"`
	MaxResults         int      `toml:"max_results"`
	CookiesFile        string   `toml:"cookies_file"`
	CookiesFromBrowser string   `toml:"cookies_from_browser"`
	ExtraArgs          []string `toml:"extra_args"`
	CacheDir           string   `toml:"cache_dir"`
}

// RadioSourceConfig configures Radio Browser search and internet radio playback integration.
type RadioSourceConfig struct {
	Enabled    bool   `toml:"enabled"`
	MaxResults int    `toml:"max_results"`
	BaseURL    string `toml:"base_url"`
}

// LoadResult reports the loaded config plus the path or paths that produced it.
type LoadResult struct {
	Path   string
	Config Config
}

func defaultThemeConfig() ThemeConfig {
	cfg := ThemeConfig{}
	cfg.setPalette(components.DefaultTheme())
	return cfg
}

func themeStringField(value any, key string) (string, error) {
	typed, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("ui.theme.%s must be a string, got %T", key, value)
	}
	return typed, nil
}

func (t ThemeConfig) Palette() components.Theme {
	return components.Theme{
		Background:     t.Background,
		Surface:        t.Surface,
		SurfaceVariant: t.SurfaceVariant,
		Primary:        t.Primary,
		OnPrimary:      t.OnPrimary,
		Text:           t.Text,
		TextMuted:      t.TextMuted,
		TextSubtle:     t.TextSubtle,
		Border:         t.Border,
		Warning:        t.Warning,
		OnWarning:      t.OnWarning,
	}
}

func (t *ThemeConfig) setPalette(theme components.Theme) {
	t.Background = theme.Background
	t.Surface = theme.Surface
	t.SurfaceVariant = theme.SurfaceVariant
	t.Primary = theme.Primary
	t.OnPrimary = theme.OnPrimary
	t.Text = theme.Text
	t.TextMuted = theme.TextMuted
	t.TextSubtle = theme.TextSubtle
	t.Border = theme.Border
	t.Warning = theme.Warning
	t.OnWarning = theme.OnWarning
}

func (t *ThemeConfig) normalize() error {
	t.File = normalizePath(t.File)
	palette := t.Palette().Normalize()
	if err := palette.Validate(); err != nil {
		return err
	}
	t.setPalette(palette)
	return nil
}

// Default returns the built-in Musicon configuration defaults.
func Default() Config {
	return Config{
		Audio: AudioConfig{
			Backend: defaultBackend,
		},
		UI: UIConfig{
			Theme:          defaultThemeConfig(),
			StartMode:      defaultStartMode,
			CellWidthRatio: components.TerminalCellWidthRatio(),
			AlbumArt: AlbumArtConfig{
				FillMode: defaultFillMode,
				Protocol: defaultProtocol,
			},
		},
		Keybinds: defaultKeybinds(),
		Sources: SourcesConfig{
			Local: LocalSourceConfig{
				Dirs: defaultLocalDirs(),
			},
			YouTube: YouTubeSourceConfig{
				Enabled:    true,
				MaxResults: defaultYTResults,
				CacheDir:   defaultYouTubeCacheDir(),
			},
			Radio: RadioSourceConfig{
				Enabled:    true,
				MaxResults: defaultRadioResults,
				BaseURL:    defaultRadioURL,
			},
		},
	}
}

// Load reads configuration from an explicit TOML file path.
func Load(path string) (Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Config{}, errors.New("config path is empty")
	}

	cfg := Default()
	metadata, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return Config{}, err
	}
	if err := applyThemeConfig(&cfg.UI.Theme, metadata, path); err != nil {
		return Config{}, err
	}
	if err := cfg.normalize(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// LoadDefault loads the default user config path and falls back to built-in defaults when it is absent.
func LoadDefault() (LoadResult, error) {
	path, explicit, err := explicitPath()
	if err != nil {
		return LoadResult{}, err
	}
	if explicit {
		cfg, loadErr := Load(path)
		if loadErr != nil {
			return LoadResult{}, loadErr
		}
		return LoadResult{Path: path, Config: cfg}, nil
	}

	path, err = defaultPath()
	if err != nil {
		return LoadResult{}, err
	}
	ok, err := isRegularFile(path)
	if err != nil {
		return LoadResult{}, fmt.Errorf("stat config %q: %w", path, err)
	}
	if !ok {
		return LoadResult{
			Path:   path,
			Config: Default(),
		}, nil
	}
	cfg, err := Load(path)
	if err != nil {
		return LoadResult{}, err
	}
	return LoadResult{Path: path, Config: cfg}, nil
}

// ResolvedLocalDirs returns deduplicated, expanded local-library directories.
func (c Config) ResolvedLocalDirs() []string {
	dirs := make([]string, 0, len(c.Sources.Local.Dirs))
	seen := make(map[string]struct{}, len(c.Sources.Local.Dirs))
	for _, raw := range c.Sources.Local.Dirs {
		expanded, ok := expandPath(raw)
		if !ok {
			continue
		}
		cleaned := filepath.Clean(expanded)
		if _, exists := seen[cleaned]; exists {
			continue
		}
		seen[cleaned] = struct{}{}
		dirs = append(dirs, cleaned)
	}
	if len(dirs) == 0 {
		return defaultLocalDirs()
	}
	return dirs
}

func (c *Config) normalize() error {
	c.Audio.Backend = normalizeString(c.Audio.Backend, defaultBackend)
	if err := c.UI.Theme.normalize(); err != nil {
		return fmt.Errorf("normalize ui theme: %w", err)
	}
	c.UI.StartMode = normalizeStartMode(c.UI.StartMode)
	if c.UI.CellWidthRatio <= 0 {
		c.UI.CellWidthRatio = components.TerminalCellWidthRatio()
	}
	c.UI.AlbumArt.FillMode = normalizeFillMode(c.UI.AlbumArt.FillMode)
	c.UI.AlbumArt.Protocol = normalizeProtocol(c.UI.AlbumArt.Backend, c.UI.AlbumArt.Protocol)
	c.UI.AlbumArt.Backend = c.UI.AlbumArt.Protocol
	c.Keybinds.normalize()
	if len(c.Sources.Local.Dirs) == 0 {
		c.Sources.Local.Dirs = defaultLocalDirs()
	} else {
		dirs := make([]string, 0, len(c.Sources.Local.Dirs))
		for _, dir := range c.Sources.Local.Dirs {
			dir = strings.TrimSpace(dir)
			if dir == "" {
				continue
			}
			dirs = append(dirs, dir)
		}
		if len(dirs) == 0 {
			dirs = defaultLocalDirs()
		}
		c.Sources.Local.Dirs = dirs
	}

	c.Sources.YouTube.CookiesFile = normalizePath(c.Sources.YouTube.CookiesFile)
	c.Sources.YouTube.CookiesFromBrowser = strings.TrimSpace(c.Sources.YouTube.CookiesFromBrowser)
	c.Sources.YouTube.CacheDir = normalizePathWithFallback(c.Sources.YouTube.CacheDir, defaultYouTubeCacheDir())
	if c.Sources.YouTube.MaxResults <= 0 {
		c.Sources.YouTube.MaxResults = defaultYTResults
	}
	extraArgs := make([]string, 0, len(c.Sources.YouTube.ExtraArgs))
	for _, arg := range c.Sources.YouTube.ExtraArgs {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			extraArgs = append(extraArgs, arg)
		}
	}
	c.Sources.YouTube.ExtraArgs = extraArgs

	c.Sources.Radio.BaseURL = strings.TrimRight(strings.TrimSpace(c.Sources.Radio.BaseURL), "/")
	if c.Sources.Radio.BaseURL == "" {
		c.Sources.Radio.BaseURL = defaultRadioURL
	}
	if c.Sources.Radio.MaxResults <= 0 {
		c.Sources.Radio.MaxResults = defaultRadioResults
	}
	return nil
}

func defaultKeybinds() KeybindsConfig {
	return KeybindsConfig{
		Global: GlobalKeybindsConfig{
			Quit:          []string{"ctrl+c"},
			ToggleMode:    []string{"tab"},
			ToggleHelp:    []string{"?"},
			ToggleCompact: []string{"ctrl+g"},
		},
		Queue: QueueKeybindsConfig{
			ToggleSearchFocus: []string{"ctrl+f"},
			SourcePrev:        []string{"["},
			SourceNext:        []string{"]"},
			CycleSearchMode:   []string{"m"},
			ModeSongs:         []string{"1"},
			ModeArtists:       []string{"2"},
			ModeAlbums:        []string{"3"},
			ModePlaylists:     []string{"4"},
			ExpandSelected:    []string{"e"},
			ActivateSelected:  []string{"enter"},
			MoveSelectedUp:    []string{"ctrl+k"},
			MoveSelectedDown:  []string{"ctrl+j"},
			ClearQueue:        []string{"ctrl+x"},
			RemoveSelected:    []string{"x"},
			BrowserUp:         []string{"up", "k"},
			BrowserDown:       []string{"down", "j"},
			BrowserHome:       []string{"home"},
			BrowserEnd:        []string{"end"},
			BrowserPageUp:     []string{"pgup"},
			BrowserPageDown:   []string{"pgdown"},
		},
		Playback: PlaybackKeybindsConfig{
			CyclePane:     []string{"v"},
			ToggleInfo:    []string{"i"},
			ToggleRepeat:  []string{"r"},
			ToggleStream:  []string{"s"},
			TogglePause:   []string{"space"},
			PreviousTrack: []string{"["},
			NextTrack:     []string{"]"},
			SeekBackward:  []string{"left"},
			SeekForward:   []string{"right"},
			VolumeDown:    []string{"-"},
			VolumeUp:      []string{"=", "+"},
		},
	}
}

func (k *KeybindsConfig) normalize() {
	defaults := defaultKeybinds()
	k.Global.Quit = normalizeKeyList(k.Global.Quit, defaults.Global.Quit)
	k.Global.ToggleMode = normalizeKeyList(k.Global.ToggleMode, defaults.Global.ToggleMode)
	k.Global.ToggleHelp = normalizeKeyList(k.Global.ToggleHelp, defaults.Global.ToggleHelp)
	k.Global.ToggleCompact = normalizeKeyList(k.Global.ToggleCompact, defaults.Global.ToggleCompact)

	k.Queue.ToggleSearchFocus = normalizeKeyList(k.Queue.ToggleSearchFocus, defaults.Queue.ToggleSearchFocus)
	k.Queue.SourcePrev = normalizeKeyList(k.Queue.SourcePrev, defaults.Queue.SourcePrev)
	k.Queue.SourceNext = normalizeKeyList(k.Queue.SourceNext, defaults.Queue.SourceNext)
	k.Queue.CycleSearchMode = normalizeKeyList(k.Queue.CycleSearchMode, defaults.Queue.CycleSearchMode)
	k.Queue.ModeSongs = normalizeKeyList(k.Queue.ModeSongs, defaults.Queue.ModeSongs)
	k.Queue.ModeArtists = normalizeKeyList(k.Queue.ModeArtists, defaults.Queue.ModeArtists)
	k.Queue.ModeAlbums = normalizeKeyList(k.Queue.ModeAlbums, defaults.Queue.ModeAlbums)
	k.Queue.ModePlaylists = normalizeKeyList(k.Queue.ModePlaylists, defaults.Queue.ModePlaylists)
	k.Queue.ExpandSelected = normalizeKeyList(k.Queue.ExpandSelected, defaults.Queue.ExpandSelected)
	k.Queue.ActivateSelected = normalizeKeyList(k.Queue.ActivateSelected, defaults.Queue.ActivateSelected)
	k.Queue.MoveSelectedUp = normalizeKeyList(k.Queue.MoveSelectedUp, defaults.Queue.MoveSelectedUp)
	k.Queue.MoveSelectedDown = normalizeKeyList(k.Queue.MoveSelectedDown, defaults.Queue.MoveSelectedDown)
	k.Queue.ClearQueue = normalizeKeyList(k.Queue.ClearQueue, defaults.Queue.ClearQueue)
	k.Queue.RemoveSelected = normalizeKeyList(k.Queue.RemoveSelected, defaults.Queue.RemoveSelected)
	k.Queue.BrowserUp = normalizeKeyList(k.Queue.BrowserUp, defaults.Queue.BrowserUp)
	k.Queue.BrowserDown = normalizeKeyList(k.Queue.BrowserDown, defaults.Queue.BrowserDown)
	k.Queue.BrowserHome = normalizeKeyList(k.Queue.BrowserHome, defaults.Queue.BrowserHome)
	k.Queue.BrowserEnd = normalizeKeyList(k.Queue.BrowserEnd, defaults.Queue.BrowserEnd)
	k.Queue.BrowserPageUp = normalizeKeyList(k.Queue.BrowserPageUp, defaults.Queue.BrowserPageUp)
	k.Queue.BrowserPageDown = normalizeKeyList(k.Queue.BrowserPageDown, defaults.Queue.BrowserPageDown)

	k.Playback.CyclePane = normalizeKeyList(k.Playback.CyclePane, defaults.Playback.CyclePane)
	k.Playback.ToggleInfo = normalizeKeyList(k.Playback.ToggleInfo, defaults.Playback.ToggleInfo)
	k.Playback.ToggleRepeat = normalizeKeyList(k.Playback.ToggleRepeat, defaults.Playback.ToggleRepeat)
	k.Playback.ToggleStream = normalizeKeyList(k.Playback.ToggleStream, defaults.Playback.ToggleStream)
	k.Playback.TogglePause = normalizeKeyList(k.Playback.TogglePause, defaults.Playback.TogglePause)
	k.Playback.PreviousTrack = normalizeKeyList(k.Playback.PreviousTrack, defaults.Playback.PreviousTrack)
	k.Playback.NextTrack = normalizeKeyList(k.Playback.NextTrack, defaults.Playback.NextTrack)
	k.Playback.SeekBackward = normalizeKeyList(k.Playback.SeekBackward, defaults.Playback.SeekBackward)
	k.Playback.SeekForward = normalizeKeyList(k.Playback.SeekForward, defaults.Playback.SeekForward)
	k.Playback.VolumeDown = normalizeKeyList(k.Playback.VolumeDown, defaults.Playback.VolumeDown)
	k.Playback.VolumeUp = normalizeKeyList(k.Playback.VolumeUp, defaults.Playback.VolumeUp)
}

func normalizeKeyList(values, fallback []string) []string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalizeString(value, "")
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return append([]string(nil), fallback...)
	}
	return normalized
}

func normalizeString(raw, fallback string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return fallback
	}
	return raw
}

func normalizeStartMode(raw string) string {
	switch normalizeString(raw, defaultStartMode) {
	case "playback", "player":
		return "playback"
	default:
		return "queue"
	}
}

func normalizeFillMode(raw string) string {
	switch normalizeString(raw, defaultFillMode) {
	case "stretch", "fit", "auto", "none":
		return normalizeString(raw, defaultFillMode)
	default:
		return defaultFillMode
	}
}

func normalizeProtocol(values ...string) string {
	for _, raw := range values {
		switch normalizeString(raw, "") {
		case "auto":
			return "auto"
		case "kitty":
			return "kitty"
		case "sixel":
			return "sixel"
		case "iterm2", "iterm":
			return "iterm2"
		case "unicode", "halfblock", "halfblocks":
			return "halfblocks"
		}
	}
	return defaultProtocol
}

func applyThemeConfig(theme *ThemeConfig, metadata toml.MetaData, configPath string) error {
	if theme == nil || !metadata.IsDefined("ui", "theme") {
		return nil
	}

	palette := theme.Palette().Normalize()
	if metadata.IsDefined("ui", "theme", "file") {
		theme.File = normalizePathRelative(theme.File, filepath.Dir(configPath))
		if theme.File == "" {
			palette = components.DefaultTheme()
		} else {
			loaded, err := loadThemeFile(theme.File)
			if err != nil {
				return fmt.Errorf("load ui theme file %q: %w", theme.File, err)
			}
			palette = loaded
		}
	}

	if metadata.IsDefined("ui", "theme", "background") {
		palette.Background = theme.Background
	}
	if metadata.IsDefined("ui", "theme", "surface") {
		palette.Surface = theme.Surface
	}
	if metadata.IsDefined("ui", "theme", "surface_variant") {
		palette.SurfaceVariant = theme.SurfaceVariant
	}
	if metadata.IsDefined("ui", "theme", "primary") {
		palette.Primary = theme.Primary
	}
	if metadata.IsDefined("ui", "theme", "on_primary") {
		palette.OnPrimary = theme.OnPrimary
	}
	if metadata.IsDefined("ui", "theme", "text") {
		palette.Text = theme.Text
	}
	if metadata.IsDefined("ui", "theme", "text_muted") {
		palette.TextMuted = theme.TextMuted
	}
	if metadata.IsDefined("ui", "theme", "text_subtle") {
		palette.TextSubtle = theme.TextSubtle
	}
	if metadata.IsDefined("ui", "theme", "border") {
		palette.Border = theme.Border
	}
	if metadata.IsDefined("ui", "theme", "warning") {
		palette.Warning = theme.Warning
	}
	if metadata.IsDefined("ui", "theme", "on_warning") {
		palette.OnWarning = theme.OnWarning
	}

	palette = palette.Normalize()
	if err := palette.Validate(); err != nil {
		return fmt.Errorf("invalid ui theme: %w", err)
	}
	theme.setPalette(palette)
	return nil
}

func loadThemeFile(path string) (components.Theme, error) {
	theme := defaultThemeConfig()
	if _, err := toml.DecodeFile(path, &theme); err != nil {
		return components.Theme{}, err
	}
	palette := theme.Palette().Normalize()
	if err := palette.Validate(); err != nil {
		return components.Theme{}, err
	}
	return palette, nil
}

func explicitPath() (path string, explicit bool, err error) {
	if raw := strings.TrimSpace(os.Getenv("MUSICON_CONFIG")); raw != "" {
		return raw, true, nil
	}
	return "", false, nil
}

func defaultPath() (string, error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if configRoot == "" {
		return "", errors.New("user config dir is empty")
	}
	return filepath.Join(configRoot, "musicon", "config.toml"), nil
}

func defaultPaths() ([]string, error) {
	candidates := make([]string, 0, 2)
	for _, root := range globalConfigRoots() {
		candidate := filepath.Join(root, "musicon", "config.toml")
		ok, err := isRegularFile(candidate)
		if err != nil {
			return nil, fmt.Errorf("stat config %q: %w", candidate, err)
		}
		if ok {
			candidates = append(candidates, candidate)
		}
	}

	if configRoot, configErr := os.UserConfigDir(); configErr == nil && configRoot != "" {
		candidate := filepath.Join(configRoot, "musicon", "config.toml")
		ok, err := isRegularFile(candidate)
		if err != nil {
			return nil, fmt.Errorf("stat config %q: %w", candidate, err)
		}
		if ok {
			candidates = append(candidates, candidate)
		}
	}

	return candidates, nil
}

func defaultLocalDirs() []string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return []string{filepath.Join(home, "Music")}
	}
	return []string{"."}
}

func defaultYouTubeCacheDir() string {
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		return filepath.Join(dir, "musicon", "youtube")
	}
	return filepath.Join(os.TempDir(), "musicon-youtube")
}

func expandPath(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if raw == "~" || strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", false
		}
		if raw == "~" {
			return home, true
		}
		return filepath.Join(home, strings.TrimPrefix(raw, "~/")), true
	}
	return raw, true
}

func normalizePath(raw string) string {
	expanded, ok := expandPath(raw)
	if !ok {
		return ""
	}
	return filepath.Clean(expanded)
}

func normalizePathRelative(raw, baseDir string) string {
	expanded, ok := expandPath(raw)
	if !ok {
		return ""
	}
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded)
	}
	if baseDir == "" {
		return filepath.Clean(expanded)
	}
	return filepath.Clean(filepath.Join(baseDir, expanded))
}

func normalizePathWithFallback(raw, fallback string) string {
	if normalized := normalizePath(raw); normalized != "" {
		return normalized
	}
	return normalizePath(fallback)
}

func globalConfigRoots() []string {
	raw := strings.TrimSpace(os.Getenv("XDG_CONFIG_DIRS"))
	if raw == "" {
		return []string{"/etc/xdg"}
	}

	roots := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, entry := range filepath.SplitList(raw) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, exists := seen[entry]; exists {
			continue
		}
		seen[entry] = struct{}{}
		roots = append(roots, entry)
	}
	if len(roots) == 0 {
		return []string{"/etc/xdg"}
	}
	return roots
}

func isRegularFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return !info.IsDir(), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
