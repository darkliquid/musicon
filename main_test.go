package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/darkliquid/musicon/pkg/coverart"
)

func TestPrintSelectedOptionsWritesOnePerLine(t *testing.T) {
	var out bytes.Buffer
	err := printSelectedOptions(&out, "", func() ([]string, error) {
		return []string{"auto", "alsa", "jack"}, nil
	})
	if err != nil {
		t.Fatalf("print selected options failed: %v", err)
	}

	if got := out.String(); got != "auto\nalsa\njack\n" {
		t.Fatalf("unexpected backend listing: %q", got)
	}
}

func TestPrintSelectedOptionsMarksSelectedOption(t *testing.T) {
	var out bytes.Buffer
	err := printSelectedOptions(&out, "alsa", func() ([]string, error) {
		return []string{"auto", "alsa", "jack"}, nil
	})
	if err != nil {
		t.Fatalf("print selected options failed: %v", err)
	}

	if got := out.String(); got != "auto\nalsa [selected]\njack\n" {
		t.Fatalf("unexpected backend listing: %q", got)
	}
}

func TestPrintSelectedOptionsReturnsListingError(t *testing.T) {
	want := errors.New("boom")
	err := printSelectedOptions(&bytes.Buffer{}, "", func() ([]string, error) {
		return nil, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestArtworkDebugResolverLogsAttemptsAndOutcome(t *testing.T) {
	var log bytes.Buffer
	var reported []coverart.AttemptEvent
	resolver := artworkDebugResolver{
		next: stubArtworkResolver{
			result: coverart.Result{
				Provider: "musicbrainz",
				Image:    coverart.Image{Data: []byte("jpeg"), MIMEType: "image/jpeg", Description: "cover"},
			},
			events: []coverart.AttemptEvent{
				{Provider: "musicbrainz", Status: coverart.AttemptCacheMiss, Message: "cache miss"},
				{Provider: "musicbrainz", Status: coverart.AttemptSuccess, Message: "artwork found"},
			},
		},
		logf: func(format string, args ...interface{}) {
			fmt.Fprintf(&log, format+"\n", args...)
		},
	}

	result, err := resolver.ResolveObserved(context.Background(), coverart.Metadata{
		Title:  "Song",
		Artist: "Artist",
		Album:  "Album",
	}, func(event coverart.AttemptEvent) {
		reported = append(reported, event)
	})
	if err != nil {
		t.Fatalf("ResolveObserved returned error: %v", err)
	}
	if result.Provider != "musicbrainz" {
		t.Fatalf("unexpected provider: %q", result.Provider)
	}
	if len(reported) != 2 {
		t.Fatalf("expected 2 reported events, got %d", len(reported))
	}

	output := log.String()
	for _, want := range []string{
		`coverart: request started metadata=title="Song" artist="Artist" album="Album"`,
		`coverart: attempt=1 provider=musicbrainz status=cache-miss message=cache miss`,
		`coverart: attempt=2 provider=musicbrainz status=success message=artwork found`,
		`coverart: resolved provider=musicbrainz mime="image/jpeg" description="cover" bytes=4 elapsed=`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected log output to contain %q, got %q", want, output)
		}
	}
}

type stubArtworkResolver struct {
	result coverart.Result
	err    error
	events []coverart.AttemptEvent
}

func (s stubArtworkResolver) Resolve(ctx context.Context, metadata coverart.Metadata) (coverart.Result, error) {
	return s.ResolveObserved(ctx, metadata, nil)
}

func (s stubArtworkResolver) ResolveObserved(_ context.Context, _ coverart.Metadata, report func(coverart.AttemptEvent)) (coverart.Result, error) {
	for _, event := range s.events {
		if report != nil {
			report(event)
		}
	}
	return s.result, s.err
}
