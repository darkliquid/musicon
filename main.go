package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/darkliquid/musicon/internal/audio"
	"github.com/darkliquid/musicon/internal/config"
	"github.com/darkliquid/musicon/internal/mpris"
	"github.com/darkliquid/musicon/internal/sources/local"
	"github.com/darkliquid/musicon/internal/sources/youtube"
	"github.com/darkliquid/musicon/internal/ui"
	"github.com/darkliquid/musicon/pkg/components"
	"github.com/darkliquid/musicon/pkg/coverart"
)

func main() {
	listBackends := flag.Bool("list-backends", false, "list usable audio backends in config-compatible form and exit")
	listImageRenderers := flag.Bool("list-image-renderers", false, "list usable image renderers and exit")
	audioBackend := flag.String("audio-backend", "", "force a specific audio backend (e.g. alsa, pulse, jack)")
	imageBackend := flag.String("image-backend", "", "force a specific image renderer (e.g. kitty, sixel, iterm2, halfblocks)")
	flag.Parse()

	listingOnly := *listBackends || *listImageRenderers
	if listingOnly {
		_ = os.Stderr.Close()
	}

	debuglog("Loading Musicon Config...")
	loaded, err := config.LoadDefault()
	if err != nil {
		if listingOnly {
			os.Exit(1)
		}
		debuglog("musicon: load config: %v\n", err)
		os.Exit(1)
	}

	if *audioBackend != "" {
		loaded.Config.Audio.Backend = audio.CanonicalBackendName(*audioBackend)
	}
	if *imageBackend != "" {
		backend := components.CanonicalImageRenderer(*imageBackend)
		loaded.Config.UI.AlbumArt.Backend = backend
		loaded.Config.UI.AlbumArt.Protocol = backend
		os.Setenv("MUSICON_IMAGE_PROTOCOL", backend)
	}

	if *listBackends {
		if err := printSelectedOptions(os.Stdout, audio.CanonicalBackendName(loaded.Config.Audio.Backend), audio.ListUsableBackends); err != nil {
			os.Exit(1)
		}
		return
	}
	if *listImageRenderers {
		if err := printSelectedOptions(os.Stdout, components.EffectiveImageRenderer(loaded.Config.UI.AlbumArt.Backend), func() ([]string, error) {
			return components.ListUsableImageRenderers(), nil
		}); err != nil {
			os.Exit(1)
		}
		return
	}

	debuglog("Loading Local Library")
	library := local.NewLibrary(local.Options{Roots: loaded.Config.ResolvedLocalDirs()})
	ytmusic := youtube.NewSource(youtube.Options{
		Enabled:            loaded.Config.Sources.YouTube.Enabled,
		MaxResults:         loaded.Config.Sources.YouTube.MaxResults,
		CookiesFile:        loaded.Config.Sources.YouTube.CookiesFile,
		CookiesFromBrowser: loaded.Config.Sources.YouTube.CookiesFromBrowser,
		ExtraArgs:          loaded.Config.Sources.YouTube.ExtraArgs,
		CacheDir:           loaded.Config.Sources.YouTube.CacheDir,
	})
	search := combinedSearch{providers: []ui.SearchService{library, ytmusic}}
	resolver := combinedResolver{
		local:   library,
		youtube: ytmusic,
	}

	debuglog("Initializing Musicon Engine...")
	engine := audio.NewEngine(audio.Options{
		Resolver: resolver,
		Backend:  loaded.Config.Audio.Backend,
	})
	defer engine.Close()

	debuglog("Creating Playback Service...")
	playback := engine.PlaybackService()

	debuglog("Connecting MPRIS...")
	bridge := mpris.NewBridge(playback)
	if err := bridge.Start(); err != nil {
		debuglog("musicon: mpris unavailable: %v\n", err)
	} else {
		defer bridge.Close()
	}

	app := ui.NewApp(ui.Services{
		Search:   search,
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
		Keybinds: ui.KeybindOptions{
			Global: ui.GlobalKeybindOptions{
				Quit:       loaded.Config.Keybinds.Global.Quit,
				ToggleMode: loaded.Config.Keybinds.Global.ToggleMode,
				ToggleHelp: loaded.Config.Keybinds.Global.ToggleHelp,
			},
			Queue: ui.QueueKeybindOptions{
				ToggleSearchFocus: loaded.Config.Keybinds.Queue.ToggleSearchFocus,
				SourcePrev:        loaded.Config.Keybinds.Queue.SourcePrev,
				SourceNext:        loaded.Config.Keybinds.Queue.SourceNext,
				FilterTracks:      loaded.Config.Keybinds.Queue.FilterTracks,
				FilterStreams:     loaded.Config.Keybinds.Queue.FilterStreams,
				FilterPlaylists:   loaded.Config.Keybinds.Queue.FilterPlaylists,
				ActivateSelected:  loaded.Config.Keybinds.Queue.ActivateSelected,
				MoveSelectedUp:    loaded.Config.Keybinds.Queue.MoveSelectedUp,
				MoveSelectedDown:  loaded.Config.Keybinds.Queue.MoveSelectedDown,
				ClearQueue:        loaded.Config.Keybinds.Queue.ClearQueue,
				RemoveSelected:    loaded.Config.Keybinds.Queue.RemoveSelected,
				BrowserUp:         loaded.Config.Keybinds.Queue.BrowserUp,
				BrowserDown:       loaded.Config.Keybinds.Queue.BrowserDown,
				BrowserHome:       loaded.Config.Keybinds.Queue.BrowserHome,
				BrowserEnd:        loaded.Config.Keybinds.Queue.BrowserEnd,
				BrowserPageUp:     loaded.Config.Keybinds.Queue.BrowserPageUp,
				BrowserPageDown:   loaded.Config.Keybinds.Queue.BrowserPageDown,
			},
			Playback: ui.PlaybackKeybindOptions{
				CyclePane:     loaded.Config.Keybinds.Playback.CyclePane,
				ToggleInfo:    loaded.Config.Keybinds.Playback.ToggleInfo,
				ToggleRepeat:  loaded.Config.Keybinds.Playback.ToggleRepeat,
				ToggleStream:  loaded.Config.Keybinds.Playback.ToggleStream,
				TogglePause:   loaded.Config.Keybinds.Playback.TogglePause,
				PreviousTrack: loaded.Config.Keybinds.Playback.PreviousTrack,
				NextTrack:     loaded.Config.Keybinds.Playback.NextTrack,
				SeekBackward:  loaded.Config.Keybinds.Playback.SeekBackward,
				SeekForward:   loaded.Config.Keybinds.Playback.SeekForward,
				VolumeDown:    loaded.Config.Keybinds.Playback.VolumeDown,
				VolumeUp:      loaded.Config.Keybinds.Playback.VolumeUp,
			},
		},
	})

	debuglog("Booting Musicon...")
	if err := ui.Run(app); err != nil {
		debuglog("musicon: %v\n", err)
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

func printSelectedOptions(out io.Writer, selected string, list func() ([]string, error)) error {
	options, err := list()
	if err != nil {
		return err
	}
	for _, option := range options {
		if option == selected {
			option += " [selected]"
		}
		if _, err := fmt.Fprintln(out, option); err != nil {
			return err
		}
	}
	return nil
}

func debuglog(format string, args ...interface{}) {
	if *debug {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

var debug = flag.Bool("debug", false, "enable debug logging to stderr")

type combinedSearch struct {
	providers []ui.SearchService
}

// Sources returns the configured source descriptors, adding an aggregate option when multiple providers are available.
func (c combinedSearch) Sources() []ui.SourceDescriptor {
	descriptors := make([]ui.SourceDescriptor, 0, len(c.providers)+1)
	seen := make(map[string]struct{}, len(c.providers)+1)
	for _, provider := range c.providers {
		if provider == nil {
			continue
		}
		for _, descriptor := range provider.Sources() {
			if descriptor.ID == "" {
				continue
			}
			if _, exists := seen[descriptor.ID]; exists {
				continue
			}
			seen[descriptor.ID] = struct{}{}
			descriptors = append(descriptors, descriptor)
		}
	}
	if len(descriptors) > 1 {
		return append([]ui.SourceDescriptor{{
			ID:          "all",
			Name:        "All sources",
			Description: "Search across every configured music source.",
		}}, descriptors...)
	}
	return descriptors
}

// Search queries each configured provider in order and deduplicates results by ID.
func (c combinedSearch) Search(ctx context.Context, request ui.SearchRequest) ([]ui.SearchResult, error) {
	results := make([]ui.SearchResult, 0, 64)
	seen := make(map[string]struct{}, 64)
	for _, provider := range c.providers {
		if provider == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		matches, err := provider.Search(ctx, request)
		if err != nil {
			return nil, err
		}
		for _, result := range matches {
			if _, exists := seen[result.ID]; exists {
				continue
			}
			seen[result.ID] = struct{}{}
			results = append(results, result)
		}
	}
	return results, nil
}

type combinedResolver struct {
	local   audio.Resolver
	youtube audio.Resolver
}

// Resolve routes queue entries to the resolver that owns the entry namespace.
func (c combinedResolver) Resolve(entry ui.QueueEntry) (audio.ResolvedTrack, error) {
	if youtube.OwnsEntryID(entry.ID) {
		if c.youtube == nil {
			return audio.ResolvedTrack{}, fmt.Errorf("no resolver configured for %q", entry.Source)
		}
		return c.youtube.Resolve(entry)
	}
	if c.local == nil {
		return audio.ResolvedTrack{}, fmt.Errorf("no resolver configured for %q", entry.Source)
	}
	return c.local.Resolve(entry)
}
