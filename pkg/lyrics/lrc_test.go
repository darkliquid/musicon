package lyrics

import (
	"testing"
	"time"
)

func TestParseLRCParsesTimedLinesAndMetadata(t *testing.T) {
	document := ParseLRC("[ar:Artist]\n[ti:Song]\n[00:01.50]Hello\n[00:03.00]World\n")
	if document.ArtistName != "Artist" || document.TrackName != "Song" {
		t.Fatalf("unexpected metadata: %#v", document)
	}
	if len(document.TimedLines) != 2 {
		t.Fatalf("expected 2 timed lines, got %#v", document.TimedLines)
	}
	if document.TimedLines[0].Start != 1500*time.Millisecond || document.TimedLines[0].Text != "Hello" {
		t.Fatalf("unexpected first timed line: %#v", document.TimedLines[0])
	}
}
