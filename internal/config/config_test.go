package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultProvidesExpectedTunables(t *testing.T) {
	cfg := Default()

	if cfg.Audio.Backend != "auto" {
		t.Fatalf("expected auto backend, got %q", cfg.Audio.Backend)
	}
	if cfg.UI.Theme != "default" {
		t.Fatalf("expected default theme, got %q", cfg.UI.Theme)
	}
	if cfg.UI.StartMode != "queue" {
		t.Fatalf("expected queue start mode, got %q", cfg.UI.StartMode)
	}
	if cfg.UI.AlbumArt.FillMode != "fill" {
		t.Fatalf("expected fill mode, got %q", cfg.UI.AlbumArt.FillMode)
	}
	if len(cfg.Sources.Local.Dirs) == 0 {
		t.Fatal("expected default local dirs")
	}
}

func TestLoadOverlaysTOMLAndNormalizesValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "musicon.toml")
	if err := os.WriteFile(path, []byte(`
[audio]
backend = "ALSA"

[ui]
theme = "Midnight"
start_mode = "playback"
cell_width_ratio = 0.6

[ui.album_art]
fill_mode = "stretch"
protocol = "kitty"

[sources.local]
dirs = ["~/Music", " /tmp/library "]
`), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Audio.Backend != "alsa" {
		t.Fatalf("expected normalized backend, got %q", cfg.Audio.Backend)
	}
	if cfg.UI.Theme != "midnight" {
		t.Fatalf("expected normalized theme, got %q", cfg.UI.Theme)
	}
	if cfg.UI.StartMode != "playback" {
		t.Fatalf("expected playback start mode, got %q", cfg.UI.StartMode)
	}
	if cfg.UI.CellWidthRatio != 0.6 {
		t.Fatalf("expected cell width ratio 0.6, got %v", cfg.UI.CellWidthRatio)
	}
	if cfg.UI.AlbumArt.FillMode != "stretch" || cfg.UI.AlbumArt.Protocol != "kitty" {
		t.Fatalf("unexpected album art settings: %#v", cfg.UI.AlbumArt)
	}
	if got := cfg.Sources.Local.Dirs[1]; got != "/tmp/library" {
		t.Fatalf("expected trimmed dir, got %q", got)
	}
}

func TestResolvedLocalDirsExpandsHomeAndDeduplicates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := Config{
		Sources: SourcesConfig{
			Local: LocalSourceConfig{
				Dirs: []string{"~/Music", "~/Music", "/tmp/music"},
			},
		},
	}
	cfg.normalize()

	dirs := cfg.ResolvedLocalDirs()
	if len(dirs) != 2 {
		t.Fatalf("expected two resolved dirs, got %#v", dirs)
	}
	if dirs[0] != filepath.Join(os.Getenv("HOME"), "Music") {
		t.Fatalf("expected expanded home path, got %q", dirs[0])
	}
}

func TestLoadDefaultOverlaysGlobalWithUserXDGConfig(t *testing.T) {
	root := t.TempDir()
	globalDir := filepath.Join(root, "global")
	userDir := filepath.Join(root, "user")
	if err := os.MkdirAll(filepath.Join(globalDir, "musicon"), 0o755); err != nil {
		t.Fatalf("mkdir global failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(userDir, "musicon"), 0o755); err != nil {
		t.Fatalf("mkdir user failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(globalDir, "musicon", "config.toml"), []byte(`
[audio]
backend = "alsa"

[ui]
start_mode = "queue"
cell_width_ratio = 0.55
`), 0o644); err != nil {
		t.Fatalf("write global config failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(userDir, "musicon", "config.toml"), []byte(`
[ui]
start_mode = "playback"

[sources.local]
dirs = ["~/Music", "/tmp/user-library"]
`), 0o644); err != nil {
		t.Fatalf("write user config failed: %v", err)
	}

	t.Setenv("XDG_CONFIG_DIRS", globalDir)
	t.Setenv("XDG_CONFIG_HOME", userDir)
	t.Setenv("MUSICON_CONFIG", "")

	result, err := LoadDefault()
	if err != nil {
		t.Fatalf("load default failed: %v", err)
	}

	if result.Config.Audio.Backend != "alsa" {
		t.Fatalf("expected global backend, got %q", result.Config.Audio.Backend)
	}
	if result.Config.UI.StartMode != "playback" {
		t.Fatalf("expected user start mode overlay, got %q", result.Config.UI.StartMode)
	}
	if result.Config.UI.CellWidthRatio != 0.55 {
		t.Fatalf("expected global cell width ratio preserved, got %v", result.Config.UI.CellWidthRatio)
	}
	if len(result.Config.Sources.Local.Dirs) != 2 || result.Config.Sources.Local.Dirs[1] != "/tmp/user-library" {
		t.Fatalf("expected user local dirs overlay, got %#v", result.Config.Sources.Local.Dirs)
	}
}
