package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/musicon/internal/audio"
	"github.com/darkliquid/musicon/internal/mpris"
	"github.com/darkliquid/musicon/internal/sources/local"
	"github.com/darkliquid/musicon/internal/ui"
	"github.com/darkliquid/musicon/pkg/coverart"
)

func main() {
	library := local.NewLibrary(defaultLocalRoot())
	engine := audio.NewEngine(audio.Options{Resolver: library})
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

func defaultLocalRoot() string {
	if root := strings.TrimSpace(os.Getenv("MUSICON_LOCAL_ROOT")); root != "" {
		return root
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidate := filepath.Join(home, "Music")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return "."
}
