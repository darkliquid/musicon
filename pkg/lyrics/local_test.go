package lyrics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalFileProviderReadsSidecarLRC(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "song.mp3")
	lrcPath := filepath.Join(dir, "song.lrc")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write audio file: %v", err)
	}
	if err := os.WriteFile(lrcPath, []byte("[00:01.00]Hello"), 0o644); err != nil {
		t.Fatalf("write lrc file: %v", err)
	}
	document, err := (LocalFileProvider{}).Lookup(context.Background(), Request{LocalAudioPath: audioPath})
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if document.Provider != "local-lrc" || len(document.TimedLines) != 1 || document.TimedLines[0].Text != "Hello" {
		t.Fatalf("unexpected local lyrics document: %#v", document)
	}
}
