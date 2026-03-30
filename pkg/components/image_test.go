package components

import (
	"errors"
	"fmt"
	"testing"
)

func TestTerminalImageCachesRepeatedRender(t *testing.T) {
	calls := 0
	image := NewTerminalImageWithRenderer(ImageRendererFunc(func(source ImageSource, width, height int) (string, error) {
		calls++
		return fmt.Sprintf("%dx%d:%s", width, height, source.Description), nil
	}))
	image.SetSize(20, 10)
	image.SetSource(&ImageSource{Data: []byte("abc"), Description: "cover"})

	first := image.View()
	second := image.View()

	if first != "20x10:cover" {
		t.Fatalf("expected rendered image, got %q", first)
	}
	if second != first {
		t.Fatalf("expected cached render %q, got %q", first, second)
	}
	if calls != 1 {
		t.Fatalf("expected renderer to be called once, got %d", calls)
	}
}

func TestTerminalImageRerendersOnResize(t *testing.T) {
	calls := 0
	image := NewTerminalImageWithRenderer(ImageRendererFunc(func(source ImageSource, width, height int) (string, error) {
		calls++
		return fmt.Sprintf("%dx%d", width, height), nil
	}))
	image.SetSource(&ImageSource{Data: []byte("abc")})
	image.SetSize(20, 10)
	if got := image.View(); got != "20x10" {
		t.Fatalf("expected first render, got %q", got)
	}

	image.SetSize(25, 12)
	if got := image.View(); got != "25x12" {
		t.Fatalf("expected rerender after resize, got %q", got)
	}
	if calls != 2 {
		t.Fatalf("expected renderer to be called twice, got %d", calls)
	}
}

func TestTerminalImageSurfacedError(t *testing.T) {
	want := errors.New("boom")
	image := NewTerminalImageWithRenderer(ImageRendererFunc(func(source ImageSource, width, height int) (string, error) {
		return "", want
	}))
	image.SetSource(&ImageSource{Data: []byte("abc")})
	image.SetSize(20, 10)

	if got := image.View(); got != "" {
		t.Fatalf("expected empty render on error, got %q", got)
	}
	if !errors.Is(image.Error(), want) {
		t.Fatalf("expected error %v, got %v", want, image.Error())
	}
}

func TestTerminalImageClearsWhenSourceRemoved(t *testing.T) {
	image := NewTerminalImageWithRenderer(ImageRendererFunc(func(source ImageSource, width, height int) (string, error) {
		return "rendered", nil
	}))
	image.SetSource(&ImageSource{Data: []byte("abc")})
	image.SetSize(20, 10)
	if got := image.View(); got != "rendered" {
		t.Fatalf("expected rendered output, got %q", got)
	}

	image.SetSource(nil)
	if got := image.View(); got != "" {
		t.Fatalf("expected cleared output, got %q", got)
	}
}
