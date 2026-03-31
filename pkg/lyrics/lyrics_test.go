package lyrics

import (
	"context"
	"errors"
	"testing"
)

type stubProvider struct {
	name  string
	doc   Document
	err   error
	calls int
}

func (s *stubProvider) Name() string { return s.name }

func (s *stubProvider) Lookup(context.Context, Request) (Document, error) {
	s.calls++
	return s.doc, s.err
}

func TestRequestNormalize(t *testing.T) {
	request := Request{
		Title:          " Song ",
		Artist:         " Artist ",
		Album:          " Album ",
		Source:         " local ",
		LocalAudioPath: " /tmp/song.mp3 ",
	}
	got := request.Normalize()
	if got.Title != "Song" || got.Artist != "Artist" || got.Album != "Album" || got.Source != "local" || got.LocalAudioPath != "/tmp/song.mp3" {
		t.Fatalf("unexpected normalized request: %#v", got)
	}
}

func TestChainResolveFallsThroughNotFoundAndHardFailure(t *testing.T) {
	first := &stubProvider{name: "first", err: errors.New("boom")}
	second := &stubProvider{name: "second", err: ErrNotFound}
	third := &stubProvider{name: "third", doc: Document{PlainLyrics: "line"}}
	chain := NewChain(first, second, third)

	document, err := chain.Resolve(context.Background(), Request{Title: "Song", Artist: "Artist"})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if document.Provider != "third" {
		t.Fatalf("unexpected provider: %#v", document)
	}
	if first.calls != 1 || second.calls != 1 || third.calls != 1 {
		t.Fatalf("expected all providers to run, got %d %d %d", first.calls, second.calls, third.calls)
	}
}

func TestDocumentDisplayLinesPrefersTimedLines(t *testing.T) {
	document := Document{
		TimedLines:  []TimedLine{{Text: "line one"}, {Text: "line two"}},
		PlainLyrics: "plain one\nplain two",
	}
	lines := document.DisplayLines()
	if len(lines) != 2 || lines[0] != "line one" {
		t.Fatalf("unexpected display lines: %#v", lines)
	}
}
