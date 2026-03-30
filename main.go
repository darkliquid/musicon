package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/darkliquid/musicon/internal/audio"
	"github.com/darkliquid/musicon/internal/config"
	"github.com/darkliquid/musicon/internal/mpris"
	"github.com/darkliquid/musicon/internal/sources/local"
	"github.com/darkliquid/musicon/internal/ui"
	"github.com/darkliquid/musicon/pkg/coverart"
)

func main() {
	listBackends := flag.Bool("list-backends", false, "list usable audio backends in config-compatible form and exit")
	flag.Parse()

	loaded, err := config.LoadDefault()
	if err != nil {
		fmt.Fprintf(os.Stderr, "musicon: load config: %v\n", err)
		os.Exit(1)
	}

	if *listBackends {
		_ = os.Stderr.Close()
		if err := printUsableBackends(os.Stdout, audio.CanonicalBackendName(loaded.Config.Audio.Backend), audio.ListUsableBackends); err != nil {
			os.Exit(1)
		}
		return
	}

	library := local.NewLibrary(local.Options{Roots: loaded.Config.ResolvedLocalDirs()})
	engine := audio.NewEngine(audio.Options{
		Resolver: library,
		Backend:  loaded.Config.Audio.Backend,
	})
	defer engine.Close()

	playback := engine.PlaybackService()
	bridge := mpris.NewBridge(playback)
	if err := bridge.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "musicon: mpris unavailable: %v\n", err)
	} else {
		defer bridge.Close()
	}

	app := ui.NewApp(ui.Services{
		Search:   library,
		Queue:    engine.QueueService(),
		Playback: playback,
		Artwork:  buildArtworkProvider(),
	}, ui.Options{
		StartMode:      modeFromConfig(loaded.Config.UI.StartMode),
		Theme:          loaded.Config.UI.Theme,
		CellWidthRatio: loaded.Config.UI.CellWidthRatio,
		AlbumArt: ui.AlbumArtOptions{
			FillMode: loaded.Config.UI.AlbumArt.FillMode,
			Protocol: loaded.Config.UI.AlbumArt.Protocol,
		},
	})
	if err := ui.Run(app); err != nil {
		fmt.Fprintf(os.Stderr, "musicon: %v\n", err)
		os.Exit(1)
	}
}

func buildArtworkProvider() ui.ArtworkProvider {
	const userAgent = "musicon/0.1 (+https://github.com/darkliquid/musicon)"

	var cache coverart.Cache
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		cache = coverart.NewDiskCache(filepath.Join(dir, "musicon", "coverart"))
	}

	mb := coverart.NewMusicBrainzProvider(userAgent)
	spotify := &coverart.SpotifyProvider{
		ClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		ClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		Market:       getenv("SPOTIFY_MARKET", "us"),
	}
	apple := &coverart.AppleMusicProvider{
		DeveloperToken: os.Getenv("APPLE_MUSIC_DEVELOPER_TOKEN"),
		Storefront:     getenv("APPLE_MUSIC_STOREFRONT", "us"),
	}
	lastfm := &coverart.LastFMProvider{
		APIKey: os.Getenv("LASTFM_API_KEY"),
	}

	var providers []coverart.Provider
	providers = append(providers,
		coverart.NewLocalFilesProvider(),
		coverart.EmbeddedProvider{},
		withCache(mb, cache),
		withCache(spotify, cache),
		withCache(apple, cache),
		withCache(lastfm, cache),
	)

	return ui.NewCoverArtProvider(coverart.NewChain(providers...))
}

func withCache(provider coverart.Provider, cache coverart.Cache) coverart.Provider {
	if cache == nil {
		return provider
	}
	return coverart.NewCachedProvider(provider, cache)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func modeFromConfig(raw string) ui.Mode {
	if raw == "playback" {
		return ui.ModePlayback
	}
	return ui.ModeQueue
}

func printUsableBackends(out io.Writer, selected string, list func() ([]string, error)) error {
	backends, err := list()
	if err != nil {
		return err
	}
	for _, backend := range backends {
		if backend == selected {
			backend += " [selected]"
		}
		if _, err := fmt.Fprintln(out, backend); err != nil {
			return err
		}
	}
	return nil
}
