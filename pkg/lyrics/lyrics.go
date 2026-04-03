package lyrics

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

// This file defines the reusable lyrics domain model: requests, documents,
// provider contracts, and provider-chain behavior. Concrete local/remote lookup
// implementations layer on top of these abstractions.

// ErrNotFound reports that no provider could find lyrics for the supplied request.
var ErrNotFound = errors.New("lyrics not found")

// Request describes the reusable metadata a lyrics provider can use to locate lyrics.
type Request struct {
	Title          string
	Artist         string
	Album          string
	Source         string
	Duration       time.Duration
	LocalAudioPath string
}

// Normalize trims request fields and removes unusable values.
func (r Request) Normalize() Request {
	r.Title = strings.TrimSpace(r.Title)
	r.Artist = strings.TrimSpace(r.Artist)
	r.Album = strings.TrimSpace(r.Album)
	r.Source = strings.TrimSpace(r.Source)
	r.LocalAudioPath = strings.TrimSpace(r.LocalAudioPath)
	if r.Duration < 0 {
		r.Duration = 0
	}
	return r
}

// Empty reports whether the request contains no useful lookup data.
func (r Request) Empty() bool {
	r = r.Normalize()
	return r.Title == "" && r.Artist == "" && r.Album == "" && r.Duration <= 0 && r.LocalAudioPath == ""
}

// TimedLine is one parsed synced-lyrics row.
type TimedLine struct {
	Start time.Duration `json:"start"`
	Text  string        `json:"text"`
}

// Document is a successful lyrics lookup result.
type Document struct {
	Provider     string        `json:"provider"`
	Source       string        `json:"source"`
	TrackName    string        `json:"track_name"`
	ArtistName   string        `json:"artist_name"`
	AlbumName    string        `json:"album_name"`
	Duration     time.Duration `json:"duration"`
	Instrumental bool          `json:"instrumental"`
	PlainLyrics  string        `json:"plain_lyrics"`
	SyncedLyrics string        `json:"synced_lyrics"`
	TimedLines   []TimedLine   `json:"timed_lines"`
}

// DisplayLines returns a plain text rendering suitable for basic lyrics panes.
func (d Document) DisplayLines() []string {
	switch {
	case d.Instrumental:
		return []string{"[Instrumental]"}
	case len(d.TimedLines) > 0:
		lines := make([]string, 0, len(d.TimedLines))
		for _, line := range d.TimedLines {
			lines = append(lines, line.Text)
		}
		return trimTrailingEmptyLines(lines)
	case strings.TrimSpace(d.PlainLyrics) != "":
		return trimTrailingEmptyLines(strings.Split(normalizeLineEndings(d.PlainLyrics), "\n"))
	default:
		return nil
	}
}

// HasTimedLines reports whether the document carries synced LRC timing data.
func (d Document) HasTimedLines() bool {
	return len(d.TimedLines) > 0
}

// ActiveTimedLineIndex returns the timed-line index active at the supplied playback position.
func (d Document) ActiveTimedLineIndex(position time.Duration) int {
	if len(d.TimedLines) == 0 {
		return -1
	}
	if position < 0 {
		position = 0
	}
	next := sort.Search(len(d.TimedLines), func(i int) bool {
		return d.TimedLines[i].Start > position
	})
	if next == 0 {
		return -1
	}
	return next - 1
}

// Empty reports whether the document contains no displayable lyrics.
func (d Document) Empty() bool {
	return len(d.DisplayLines()) == 0
}

// Provider resolves lyrics from one source.
type Provider interface {
	Name() string
	Lookup(context.Context, Request) (Document, error)
}

// Chain resolves lyrics through providers in priority order.
type Chain struct {
	providers []Provider
}

// NewChain creates a new priority-ordered lyrics chain.
func NewChain(providers ...Provider) *Chain {
	filtered := make([]Provider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			filtered = append(filtered, provider)
		}
	}
	return &Chain{providers: filtered}
}

// Providers returns a shallow copy of the configured provider list.
func (c *Chain) Providers() []Provider {
	if c == nil {
		return nil
	}
	return append([]Provider(nil), c.providers...)
}

// Resolve runs providers in order until one returns usable lyrics.
func (c *Chain) Resolve(ctx context.Context, request Request) (Document, error) {
	request = request.Normalize()
	if request.Empty() || c == nil || len(c.providers) == 0 {
		return Document{}, ErrNotFound
	}

	var firstHardErr error
	for _, provider := range c.providers {
		document, err := provider.Lookup(ctx, request)
		switch {
		case err == nil:
			document.Provider = firstNonEmpty(document.Provider, provider.Name())
			if document.Empty() {
				continue
			}
			return document, nil
		case errors.Is(err, ErrNotFound):
			continue
		default:
			if firstHardErr == nil {
				firstHardErr = err
			}
		}
	}
	if firstHardErr != nil {
		return Document{}, firstHardErr
	}
	return Document{}, ErrNotFound
}

// IsNotFound reports whether the error is a miss rather than a hard failure.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func normalizeLineEndings(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return value
}

func trimTrailingEmptyLines(lines []string) []string {
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return append([]string(nil), lines[:end]...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
