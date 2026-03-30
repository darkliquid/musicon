package components

import (
	"errors"
	"fmt"
	"testing"

	termimg "github.com/blacktop/go-termimg"
)

func withStubTerminalFeatures(t *testing.T, features *termimg.TerminalFeatures) {
	t.Helper()
	original := queryTerminalFeatures
	queryTerminalFeatures = func() *termimg.TerminalFeatures { return features }
	t.Cleanup(func() {
		queryTerminalFeatures = original
	})
}

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

func TestConfiguredImageProtocolDefaultsToHalfblocks(t *testing.T) {
	t.Setenv("MUSICON_IMAGE_PROTOCOL", "")

	if got := configuredImageProtocol(); got != termimg.Halfblocks {
		t.Fatalf("expected halfblocks default, got %v", got)
	}
}

func TestConfiguredImageProtocolFromEnv(t *testing.T) {
	tests := map[string]termimg.Protocol{
		"auto":      termimg.Auto,
		"kitty":     termimg.Kitty,
		"sixel":     termimg.Sixel,
		"iterm2":    termimg.ITerm2,
		"unicode":   termimg.Halfblocks,
		"something": termimg.Halfblocks,
	}

	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("MUSICON_IMAGE_PROTOCOL", raw)
			if got := configuredImageProtocol(); got != want {
				t.Fatalf("expected %v for %q, got %v", want, raw, got)
			}
		})
	}
}

func TestConfiguredImageScaleModeDefaultsToFill(t *testing.T) {
	t.Setenv("MUSICON_IMAGE_SCALE", "")

	if got := configuredImageScaleMode(); got != termimg.ScaleFill {
		t.Fatalf("expected fill default, got %v", got)
	}
}

func TestConfiguredImageScaleModeFromEnv(t *testing.T) {
	tests := map[string]termimg.ScaleMode{
		"fill":     termimg.ScaleFill,
		"stretch":  termimg.ScaleStretch,
		"fit":      termimg.ScaleFit,
		"auto":     termimg.ScaleAuto,
		"none":     termimg.ScaleNone,
		"surprise": termimg.ScaleFill,
	}

	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("MUSICON_IMAGE_SCALE", raw)
			if got := configuredImageScaleMode(); got != want {
				t.Fatalf("expected %v for %q, got %v", want, raw, got)
			}
		})
	}
}

func TestConfiguredImageProtocolWithOverride(t *testing.T) {
	if got := configuredImageProtocolWithOverride("kitty"); got != termimg.Kitty {
		t.Fatalf("expected kitty override, got %v", got)
	}
}

func TestCanonicalImageRendererNormalizesAliases(t *testing.T) {
	cases := map[string]string{
		"":          "halfblocks",
		"unicode":   "halfblocks",
		"halfblock": "halfblocks",
		"iterm":     "iterm2",
		"iterm2":    "iterm2",
		"kitty":     "kitty",
		"something": "halfblocks",
	}

	for raw, want := range cases {
		if got := CanonicalImageRenderer(raw); got != want {
			t.Fatalf("canonical renderer for %q: got %q want %q", raw, got, want)
		}
	}
}

func TestListUsableImageRenderersIncludesAutoAndHalfblocks(t *testing.T) {
	withStubTerminalFeatures(t, &termimg.TerminalFeatures{})

	renderers := ListUsableImageRenderers()
	if len(renderers) == 0 || renderers[0] != "auto" {
		t.Fatalf("expected auto first, got %#v", renderers)
	}
	foundHalfblocks := false
	for _, renderer := range renderers {
		if renderer == "halfblocks" {
			foundHalfblocks = true
			break
		}
	}
	if !foundHalfblocks {
		t.Fatalf("expected halfblocks fallback in %#v", renderers)
	}
}

func TestListUsableImageRenderersUsesDetectedFeatures(t *testing.T) {
	withStubTerminalFeatures(t, &termimg.TerminalFeatures{
		KittyGraphics:  true,
		ITerm2Graphics: true,
		SixelGraphics:  true,
	})

	renderers := ListUsableImageRenderers()
	want := []string{"auto", "kitty", "iterm2", "sixel", "halfblocks"}
	if len(renderers) != len(want) {
		t.Fatalf("unexpected renderers %#v", renderers)
	}
	for i := range want {
		if renderers[i] != want[i] {
			t.Fatalf("unexpected renderers %#v", renderers)
		}
	}
}

func TestTerminalCellWidthRatioUsesDetectedFontMetrics(t *testing.T) {
	withStubTerminalFeatures(t, &termimg.TerminalFeatures{
		FontWidth:  8,
		FontHeight: 16,
	})

	if got := TerminalCellWidthRatio(); got != 0.5 {
		t.Fatalf("expected derived ratio 0.5, got %v", got)
	}
}

func TestTerminalCellWidthRatioFallsBackWhenFeaturesMissing(t *testing.T) {
	withStubTerminalFeatures(t, &termimg.TerminalFeatures{})

	if got := TerminalCellWidthRatio(); got != 0.5 {
		t.Fatalf("expected fallback ratio 0.5, got %v", got)
	}
}

func TestConfiguredImageScaleModeWithOverride(t *testing.T) {
	if got := configuredImageScaleModeWithOverride("stretch"); got != termimg.ScaleStretch {
		t.Fatalf("expected stretch override, got %v", got)
	}
}
