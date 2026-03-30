package audio

import (
	"testing"
	"time"

	teaui "github.com/darkliquid/musicon/internal/ui"
)

func TestEngineQueueSnapshot(t *testing.T) {
	engine := NewEngine(Options{})
	defer engine.Close()

	queue := engine.QueueService()
	if err := queue.Add(teaui.SearchResult{ID: "one", Title: "First", Duration: 3 * time.Second}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if err := queue.Add(teaui.SearchResult{ID: "two", Title: "Second", Duration: 4 * time.Second}); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	snapshot := queue.Snapshot()
	if got := len(snapshot); got != 2 {
		t.Fatalf("expected queue length 2, got %d", got)
	}
	if snapshot[0].Title != "First" || snapshot[1].Title != "Second" {
		t.Fatalf("unexpected queue snapshot: %#v", snapshot)
	}
}

func TestEngineToggleFlags(t *testing.T) {
	engine := NewEngine(Options{})
	defer engine.Close()

	playback := engine.PlaybackService()
	if err := playback.SetRepeat(true); err != nil {
		t.Fatalf("set repeat failed: %v", err)
	}
	if err := playback.SetStream(true); err != nil {
		t.Fatalf("set stream failed: %v", err)
	}
	if err := playback.AdjustVolume(-25); err != nil {
		t.Fatalf("adjust volume failed: %v", err)
	}

	snapshot := playback.Snapshot()
	if !snapshot.Repeat || !snapshot.Stream {
		t.Fatalf("expected repeat and stream enabled, got %#v", snapshot)
	}
	if snapshot.Volume != 45 {
		t.Fatalf("expected volume 45, got %d", snapshot.Volume)
	}
}

func TestEngineTogglePauseWithoutResolverFails(t *testing.T) {
	engine := NewEngine(Options{})
	defer engine.Close()

	queue := engine.QueueService()
	if err := queue.Add(teaui.SearchResult{ID: "one", Title: "First"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if err := engine.PlaybackService().TogglePause(); err == nil {
		t.Fatal("expected toggle pause to fail without resolver")
	}
}
