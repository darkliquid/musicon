package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/musicon/internal/audio"
	"github.com/darkliquid/musicon/internal/config"
	"github.com/darkliquid/musicon/internal/mpris"
	"github.com/darkliquid/musicon/internal/sources/local"
	"github.com/darkliquid/musicon/internal/sources/youtube"
	"github.com/darkliquid/musicon/internal/ui"
	"github.com/darkliquid/musicon/pkg/components"
	"github.com/darkliquid/musicon/pkg/coverart"
	"github.com/darkliquid/musicon/pkg/lyrics"
)

func main() {
	listBackends := flag.Bool("list-backends", false, "list usable audio backends in config-compatible form and exit")
	listImageRenderers := flag.Bool("list-image-renderers", false, "list usable image renderers and exit")
	audioBackend := flag.String("audio-backend", "", "force a specific audio backend (e.g. alsa, pulse, jack)")
	imageBackend := flag.String("image-backend", "", "force a specific image renderer (e.g. kitty, sixel, iterm2, halfblocks)")
	flag.Parse()

	if err := configureDebugLogging(*debug, *debugLogPath); err != nil {
		fmt.Fprintf(os.Stderr, "musicon: configure debug logging: %v\n", err)
		os.Exit(1)
	}
	defer closeDebugLogging()

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
		Search:        search,
		Queue:         engine.QueueService(),
		Playback:      playback,
		Lyrics:        buildLyricsProvider(),
		Artwork:       buildArtworkProvider(),
		Visualization: engine.VisualizationService(),
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
				CycleSearchMode:   loaded.Config.Keybinds.Queue.CycleSearchMode,
				ModeSongs:         loaded.Config.Keybinds.Queue.ModeSongs,
				ModeArtists:       loaded.Config.Keybinds.Queue.ModeArtists,
				ModeAlbums:        loaded.Config.Keybinds.Queue.ModeAlbums,
				ModePlaylists:     loaded.Config.Keybinds.Queue.ModePlaylists,
				ExpandSelected:    loaded.Config.Keybinds.Queue.ExpandSelected,
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

	debuglog(
		"coverart: configured providers cache=%t spotify_credentials=%t apple_token=%t lastfm_key=%t",
		cache != nil,
		strings.TrimSpace(spotify.ClientID) != "" && strings.TrimSpace(spotify.ClientSecret) != "",
		strings.TrimSpace(apple.DeveloperToken) != "",
		strings.TrimSpace(lastfm.APIKey) != "",
	)

	var providers []coverart.Provider
	providers = append(providers,
		coverart.NewLocalFilesProvider(),
		coverart.EmbeddedProvider{},
		withCache(coverart.MetadataURLProvider{}, cache),
		withCache(mb, cache),
		withCache(spotify, cache),
		withCache(apple, cache),
		withCache(lastfm, cache),
	)

	resolver := coverart.NewChain(providers...)
	if debugLoggingEnabled() {
		return ui.NewCoverArtProvider(artworkDebugResolver{next: resolver, logf: debuglog})
	}
	return ui.NewCoverArtProvider(resolver)
}

func buildLyricsProvider() ui.LyricsProvider {
	var cache lyrics.Cache
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		cache = lyrics.NewDiskCache(filepath.Join(dir, "musicon", "lyrics"))
	}

	lrclib := &lyrics.LRCLibProvider{}
	debuglog("lyrics: configured providers cache=%t lrclib=%t", cache != nil, true)

	resolver := lyrics.NewChain(
		lyrics.LocalFileProvider{},
		lyrics.NewCachedProvider(lrclib, cache),
	)
	return ui.NewLyricsProvider(resolver)
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
	debugOutput.logf(format, args...)
}

var debug = flag.Bool("debug", false, "enable debug logging to stderr")
var debugLogPath = flag.String("debug-log", "", "write debug logging to the specified file")

type debugSink struct {
	mu   sync.Mutex
	out  io.Writer
	file io.Closer
}

func (s *debugSink) configure(toStderr bool, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
	s.out = nil

	var outputs []io.Writer
	if toStderr {
		outputs = append(outputs, os.Stderr)
	}
	if path = strings.TrimSpace(path); path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		file, err := os.Create(path)
		if err != nil {
			return err
		}
		s.file = file
		outputs = append(outputs, file)
	}
	if len(outputs) == 1 {
		s.out = outputs[0]
	} else if len(outputs) > 1 {
		s.out = io.MultiWriter(outputs...)
	}
	return nil
}

func (s *debugSink) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.out = nil
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}

func (s *debugSink) enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.out != nil
}

func (s *debugSink) logf(format string, args ...interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.out == nil {
		return
	}
	fmt.Fprintf(s.out, format+"\n", args...)
}

var debugOutput debugSink

func configureDebugLogging(toStderr bool, path string) error {
	return debugOutput.configure(toStderr, path)
}

func closeDebugLogging() {
	_ = debugOutput.close()
}

func debugLoggingEnabled() bool {
	return debugOutput.enabled()
}

type artworkDebugResolver struct {
	next interface {
		Resolve(context.Context, coverart.Metadata) (coverart.Result, error)
	}
	logf func(string, ...interface{})
}

func (r artworkDebugResolver) Resolve(ctx context.Context, metadata coverart.Metadata) (coverart.Result, error) {
	return r.ResolveObserved(ctx, metadata, nil)
}

func (r artworkDebugResolver) ResolveObserved(ctx context.Context, metadata coverart.Metadata, report func(coverart.AttemptEvent)) (coverart.Result, error) {
	metadata = metadata.Normalize()
	startedAt := time.Now()
	providers := resolverProviderNames(r.next)
	r.log("coverart: request started metadata=%s", summarizeArtworkMetadata(metadata))
	if len(providers) > 0 {
		r.log("coverart: provider-order=%s", strings.Join(providers, " -> "))
	}

	observed, ok := r.next.(interface {
		ResolveObserved(context.Context, coverart.Metadata, func(coverart.AttemptEvent)) (coverart.Result, error)
	})
	if !ok {
		result, err := r.next.Resolve(ctx, metadata)
		r.logOutcome(result, err, time.Since(startedAt))
		return result, err
	}

	attemptCounts := make(map[string]int)
	result, err := observed.ResolveObserved(ctx, metadata, func(event coverart.AttemptEvent) {
		attemptCounts[event.Provider]++
		r.log(
			"coverart: attempt=%d provider=%s status=%s message=%s",
			attemptCounts[event.Provider],
			event.Provider,
			event.Status,
			event.Message,
		)
		if report != nil {
			report(event)
		}
	})
	r.logOutcome(result, err, time.Since(startedAt))
	return result, err
}

func (r artworkDebugResolver) logOutcome(result coverart.Result, err error, elapsed time.Duration) {
	switch {
	case err == nil:
		r.log(
			"coverart: resolved provider=%s mime=%q description=%q bytes=%d elapsed=%s",
			result.Provider,
			result.Image.MIMEType,
			result.Image.Description,
			len(result.Image.Data),
			elapsed.Truncate(time.Millisecond),
		)
	case coverart.IsNotFound(err):
		r.log("coverart: no artwork found elapsed=%s", elapsed.Truncate(time.Millisecond))
	default:
		r.log("coverart: lookup failed elapsed=%s error=%v", elapsed.Truncate(time.Millisecond), err)
	}
}

func (r artworkDebugResolver) log(format string, args ...interface{}) {
	if r.logf == nil {
		return
	}
	r.logf(format, args...)
}

func summarizeArtworkMetadata(metadata coverart.Metadata) string {
	parts := make([]string, 0, 8)
	if metadata.Title != "" {
		parts = append(parts, fmt.Sprintf("title=%q", metadata.Title))
	}
	if metadata.Artist != "" {
		parts = append(parts, fmt.Sprintf("artist=%q", metadata.Artist))
	}
	if metadata.Album != "" {
		parts = append(parts, fmt.Sprintf("album=%q", metadata.Album))
	}
	if metadata.RemoteURL != "" {
		parts = append(parts, fmt.Sprintf("remote_url=%q", metadata.RemoteURL))
	}
	if metadata.IDs.MusicBrainzReleaseID != "" {
		parts = append(parts, fmt.Sprintf("mb_release=%q", metadata.IDs.MusicBrainzReleaseID))
	}
	if metadata.IDs.MusicBrainzReleaseGroupID != "" {
		parts = append(parts, fmt.Sprintf("mb_group=%q", metadata.IDs.MusicBrainzReleaseGroupID))
	}
	if metadata.IDs.MusicBrainzRecordingID != "" {
		parts = append(parts, fmt.Sprintf("mb_recording=%q", metadata.IDs.MusicBrainzRecordingID))
	}
	if metadata.IDs.SpotifyAlbumID != "" {
		parts = append(parts, fmt.Sprintf("spotify_album=%q", metadata.IDs.SpotifyAlbumID))
	}
	if metadata.IDs.SpotifyTrackID != "" {
		parts = append(parts, fmt.Sprintf("spotify_track=%q", metadata.IDs.SpotifyTrackID))
	}
	if metadata.IDs.AppleMusicAlbumID != "" {
		parts = append(parts, fmt.Sprintf("apple_album=%q", metadata.IDs.AppleMusicAlbumID))
	}
	if metadata.IDs.AppleMusicSongID != "" {
		parts = append(parts, fmt.Sprintf("apple_song=%q", metadata.IDs.AppleMusicSongID))
	}
	if metadata.Local != nil {
		if metadata.Local.AudioPath != "" {
			parts = append(parts, fmt.Sprintf("audio_path=%q", metadata.Local.AudioPath))
		}
		if metadata.Local.CoverFilePath != "" {
			parts = append(parts, fmt.Sprintf("cover_path=%q", metadata.Local.CoverFilePath))
		}
		if metadata.Local.Embedded != nil {
			parts = append(parts, fmt.Sprintf("embedded_bytes=%d", len(metadata.Local.Embedded.Data)))
		}
	}
	if len(parts) == 0 {
		return "<empty>"
	}
	return strings.Join(parts, " ")
}

func resolverProviderNames(resolver any) []string {
	type providerLister interface {
		Providers() []coverart.Provider
	}

	list, ok := resolver.(providerLister)
	if !ok {
		return nil
	}
	providers := list.Providers()
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		name := strings.TrimSpace(provider.Name())
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

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
			SearchModes: []ui.SearchModeDescriptor{
				{ID: ui.SearchModeAll, Name: ui.SearchModeAll.String()},
				{ID: ui.SearchModeTracks, Name: ui.SearchModeTracks.String()},
				{ID: ui.SearchModeStreams, Name: ui.SearchModeStreams.String()},
				{ID: ui.SearchModePlaylists, Name: ui.SearchModePlaylists.String()},
			},
			DefaultMode: ui.SearchModeAll,
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

// ExpandCollection asks the source that owns the collection row for its child tracks.
func (c combinedSearch) ExpandCollection(ctx context.Context, result ui.SearchResult) ([]ui.SearchResult, error) {
	for _, provider := range c.providers {
		if provider == nil {
			continue
		}
		matches, err := provider.ExpandCollection(ctx, result)
		if err != nil {
			return nil, err
		}
		if len(matches) > 0 {
			return matches, nil
		}
	}
	return nil, nil
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
