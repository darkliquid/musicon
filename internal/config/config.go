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

const (
	DefaultFileName   = "musicon.toml"
	defaultTheme      = "default"
	defaultStartMode  = "queue"
	defaultBackend    = "auto"
	defaultFillMode   = "fill"
	defaultProtocol   = "halfblocks"
	defaultSourceName = "local"
	defaultYTResults  = 20
)

type Config struct {
	Audio    AudioConfig    `toml:"audio"`
	UI       UIConfig       `toml:"ui"`
	Keybinds KeybindsConfig `toml:"keybinds"`
	Sources  SourcesConfig  `toml:"sources"`
}

type KeybindsConfig struct {
	Global   GlobalKeybindsConfig   `toml:"global"`
	Queue    QueueKeybindsConfig    `toml:"queue"`
	Playback PlaybackKeybindsConfig `toml:"playback"`
}

type GlobalKeybindsConfig struct {
	Quit       []string `toml:"quit"`
	ToggleMode []string `toml:"toggle_mode"`
	ToggleHelp []string `toml:"toggle_help"`
}

type QueueKeybindsConfig struct {
	ToggleSearchFocus []string `toml:"toggle_search_focus"`
	SourcePrev        []string `toml:"source_prev"`
	SourceNext        []string `toml:"source_next"`
	FilterTracks      []string `toml:"filter_tracks"`
	FilterStreams     []string `toml:"filter_streams"`
	FilterPlaylists   []string `toml:"filter_playlists"`
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

type PlaybackKeybindsConfig struct {
	CyclePane     []string `toml:"cycle_pane"`
	ToggleInfo    []string `toml:"toggle_info"`
	ToggleRepeat  []string `toml:"toggle_repeat"`
	ToggleStream  []string `toml:"toggle_stream"`
	TogglePause   []string `toml:"toggle_pause"`
	PreviousTrack []string `toml:"previous_track"`
	NextTrack     []string `toml:"next_track"`
	VolumeDown    []string `toml:"volume_down"`
	VolumeUp      []string `toml:"volume_up"`
	SeekBackward  []string `toml:"seek_backward"`
	SeekForward   []string `toml:"seek_forward"`
}

type AudioConfig struct {
	Backend string `toml:"backend"`
}

type UIConfig struct {
	Theme          string         `toml:"theme"`
	StartMode      string         `toml:"start_mode"`
	CellWidthRatio float64        `toml:"cell_width_ratio"`
	AlbumArt       AlbumArtConfig `toml:"album_art"`
}

type AlbumArtConfig struct {
	FillMode string `toml:"fill_mode"`
	Backend  string `toml:"backend"`
	Protocol string `toml:"protocol"`
}

type SourcesConfig struct {
	Local   LocalSourceConfig   `toml:"local"`
	YouTube YouTubeSourceConfig `toml:"youtube"`
}

type LocalSourceConfig struct {
	Dirs []string `toml:"dirs"`
}

type YouTubeSourceConfig struct {
	Enabled            bool     `toml:"enabled"`
	MaxResults         int      `toml:"max_results"`
	CookiesFile        string   `toml:"cookies_file"`
	CookiesFromBrowser string   `toml:"cookies_from_browser"`
	ExtraArgs          []string `toml:"extra_args"`
	CacheDir           string   `toml:"cache_dir"`
}

type LoadResult struct {
	Path   string
	Config Config
}

func Default() Config {
	return Config{
		Audio: AudioConfig{
			Backend: defaultBackend,
		},
		UI: UIConfig{
			Theme:          defaultTheme,
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
		},
	}
}

func Load(path string) (Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Config{}, errors.New("config path is empty")
	}

	cfg := Default()
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, err
	}
	cfg.normalize()
	return cfg, nil
}

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

	paths, err := defaultPaths()
	if err != nil {
		return LoadResult{}, err
	}

	cfg := Default()
	loaded := make([]string, 0, len(paths))
	for _, candidate := range paths {
		if _, err := toml.DecodeFile(candidate, &cfg); err != nil {
			return LoadResult{}, err
		}
		loaded = append(loaded, candidate)
	}
	cfg.normalize()
	return LoadResult{
		Path:   strings.Join(loaded, ":"),
		Config: cfg,
	}, nil
}

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

func (c *Config) normalize() {
	c.Audio.Backend = normalizeString(c.Audio.Backend, defaultBackend)
	c.UI.Theme = normalizeString(c.UI.Theme, defaultTheme)
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
		return
	}

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
}

func defaultKeybinds() KeybindsConfig {
	return KeybindsConfig{
		Global: GlobalKeybindsConfig{
			Quit:       []string{"ctrl+c"},
			ToggleMode: []string{"tab"},
			ToggleHelp: []string{"?"},
		},
		Queue: QueueKeybindsConfig{
			ToggleSearchFocus: []string{"ctrl+f"},
			SourcePrev:        []string{"["},
			SourceNext:        []string{"]"},
			FilterTracks:      []string{"1"},
			FilterStreams:     []string{"2"},
			FilterPlaylists:   []string{"3"},
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
			VolumeDown:    []string{"-"},
			VolumeUp:      []string{"=", "+"},
			SeekBackward:  []string{"left"},
			SeekForward:   []string{"right"},
		},
	}
}

func (k *KeybindsConfig) normalize() {
	defaults := defaultKeybinds()
	k.Global.Quit = normalizeKeyList(k.Global.Quit, defaults.Global.Quit)
	k.Global.ToggleMode = normalizeKeyList(k.Global.ToggleMode, defaults.Global.ToggleMode)
	k.Global.ToggleHelp = normalizeKeyList(k.Global.ToggleHelp, defaults.Global.ToggleHelp)

	k.Queue.ToggleSearchFocus = normalizeKeyList(k.Queue.ToggleSearchFocus, defaults.Queue.ToggleSearchFocus)
	k.Queue.SourcePrev = normalizeKeyList(k.Queue.SourcePrev, defaults.Queue.SourcePrev)
	k.Queue.SourceNext = normalizeKeyList(k.Queue.SourceNext, defaults.Queue.SourceNext)
	k.Queue.FilterTracks = normalizeKeyList(k.Queue.FilterTracks, defaults.Queue.FilterTracks)
	k.Queue.FilterStreams = normalizeKeyList(k.Queue.FilterStreams, defaults.Queue.FilterStreams)
	k.Queue.FilterPlaylists = normalizeKeyList(k.Queue.FilterPlaylists, defaults.Queue.FilterPlaylists)
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
	k.Playback.VolumeDown = normalizeKeyList(k.Playback.VolumeDown, defaults.Playback.VolumeDown)
	k.Playback.VolumeUp = normalizeKeyList(k.Playback.VolumeUp, defaults.Playback.VolumeUp)
	k.Playback.SeekBackward = normalizeKeyList(k.Playback.SeekBackward, defaults.Playback.SeekBackward)
	k.Playback.SeekForward = normalizeKeyList(k.Playback.SeekForward, defaults.Playback.SeekForward)
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

func explicitPath() (path string, explicit bool, err error) {
	if raw := strings.TrimSpace(os.Getenv("MUSICON_CONFIG")); raw != "" {
		return raw, true, nil
	}
	return "", false, nil
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
