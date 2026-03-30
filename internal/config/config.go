package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	DefaultFileName   = "musicon.toml"
	defaultTheme      = "default"
	defaultStartMode  = "queue"
	defaultBackend    = "auto"
	defaultFillMode   = "fill"
	defaultProtocol   = "halfblocks"
	defaultCellRatio  = 0.5
	defaultSourceName = "local"
)

type Config struct {
	Audio   AudioConfig   `toml:"audio"`
	UI      UIConfig      `toml:"ui"`
	Sources SourcesConfig `toml:"sources"`
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
	Protocol string `toml:"protocol"`
}

type SourcesConfig struct {
	Local LocalSourceConfig `toml:"local"`
}

type LocalSourceConfig struct {
	Dirs []string `toml:"dirs"`
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
			CellWidthRatio: defaultCellRatio,
			AlbumArt: AlbumArtConfig{
				FillMode: defaultFillMode,
				Protocol: defaultProtocol,
			},
		},
		Sources: SourcesConfig{
			Local: LocalSourceConfig{
				Dirs: defaultLocalDirs(),
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
		c.UI.CellWidthRatio = defaultCellRatio
	}
	c.UI.AlbumArt.FillMode = normalizeFillMode(c.UI.AlbumArt.FillMode)
	c.UI.AlbumArt.Protocol = normalizeProtocol(c.UI.AlbumArt.Protocol)
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

func normalizeProtocol(raw string) string {
	switch normalizeString(raw, defaultProtocol) {
	case "auto", "kitty", "sixel", "iterm2", "iterm", "unicode", "halfblock":
		return normalizeString(raw, defaultProtocol)
	default:
		return defaultProtocol
	}
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
