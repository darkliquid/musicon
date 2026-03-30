package components

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"slices"
	"strings"

	termimg "github.com/blacktop/go-termimg"
)

var queryTerminalFeatures = termimg.QueryTerminalFeatures

// ImageSource describes encoded image data that can be rendered in a terminal.
type ImageSource struct {
	Data        []byte
	Description string
}

// ImageRenderer renders image data for a bounded terminal area.
type ImageRenderer interface {
	Render(source ImageSource, width, height int) (string, error)
}

// ImageRendererFunc adapts a function to ImageRenderer.
type ImageRendererFunc func(source ImageSource, width, height int) (string, error)

// Render implements ImageRenderer.
func (f ImageRendererFunc) Render(source ImageSource, width, height int) (string, error) {
	return f(source, width, height)
}

// TerminalImage is a reusable, cached terminal image component.
type TerminalImage struct {
	width    int
	height   int
	source   *ImageSource
	renderer ImageRenderer
	settings TerminalImageSettings

	renderKey string
	rendered  string
	err       error
}

type TerminalImageSettings struct {
	Protocol  string
	ScaleMode string
}

// NewTerminalImage constructs a terminal image component backed by go-termimg.
func NewTerminalImage() *TerminalImage {
	return NewTerminalImageWithRenderer(ImageRendererFunc(renderWithTermimg))
}

// NewTerminalImageWithSettings constructs a terminal image component with
// explicit rendering settings.
func NewTerminalImageWithSettings(settings TerminalImageSettings) *TerminalImage {
	image := NewTerminalImageWithRenderer(ImageRendererFunc(renderWithTermimg))
	image.settings = settings
	return image
}

// NewTerminalImageWithRenderer constructs a terminal image component with a
// caller-supplied renderer. This is primarily useful for tests.
func NewTerminalImageWithRenderer(renderer ImageRenderer) *TerminalImage {
	if renderer == nil {
		renderer = ImageRendererFunc(renderWithTermimg)
	}
	return &TerminalImage{renderer: renderer}
}

// SetSize updates the render bounds for the image component.
func (i *TerminalImage) SetSize(width, height int) {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	if i.width == width && i.height == height {
		return
	}
	i.width = width
	i.height = height
	i.renderKey = ""
}

// SetSource replaces the current image source.
func (i *TerminalImage) SetSource(source *ImageSource) {
	next := cloneImageSource(source)
	if equalImageSource(i.source, next) {
		return
	}
	i.source = next
	i.renderKey = ""
}

// Error reports the most recent rendering error.
func (i *TerminalImage) Error() error {
	i.ensureRendered()
	return i.err
}

// Ready reports whether the component currently has rendered image output.
func (i *TerminalImage) Ready() bool {
	return i.View() != "" && i.err == nil
}

// View renders the current image source within the configured bounds.
func (i *TerminalImage) View() string {
	i.ensureRendered()
	return i.rendered
}

func (i *TerminalImage) ensureRendered() {
	key := i.cacheKey()
	if key == i.renderKey {
		return
	}
	i.renderKey = key
	i.rendered = ""
	i.err = nil

	if i.width <= 0 || i.height <= 0 || i.source == nil || len(i.source.Data) == 0 {
		return
	}

	rendered, err := i.render(*i.source, i.width, i.height)
	if err != nil {
		i.err = err
		return
	}
	i.rendered = rendered
}

func (i *TerminalImage) cacheKey() string {
	if i.width <= 0 || i.height <= 0 || i.source == nil || len(i.source.Data) == 0 {
		return fmt.Sprintf("%dx%d:empty", i.width, i.height)
	}
	sum := sha256.Sum256(i.source.Data)
	return fmt.Sprintf("%dx%d:%x:%s", i.width, i.height, sum, i.source.Description)
}

func cloneImageSource(source *ImageSource) *ImageSource {
	if source == nil {
		return nil
	}
	data := make([]byte, len(source.Data))
	copy(data, source.Data)
	return &ImageSource{
		Data:        data,
		Description: source.Description,
	}
}

func equalImageSource(left, right *ImageSource) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.Description != right.Description {
		return false
	}
	return bytes.Equal(left.Data, right.Data)
}

func (i *TerminalImage) render(source ImageSource, width, height int) (string, error) {
	if i.settings.Protocol != "" || i.settings.ScaleMode != "" {
		return renderWithTermimgSettings(source, width, height, i.settings)
	}
	return i.renderer.Render(source, width, height)
}

func renderWithTermimg(source ImageSource, width, height int) (string, error) {
	return renderWithTermimgSettings(source, width, height, TerminalImageSettings{})
}

func renderWithTermimgSettings(source ImageSource, width, height int, settings TerminalImageSettings) (string, error) {
	decoded, _, err := image.Decode(bytes.NewReader(source.Data))
	if err != nil {
		return "", err
	}

	protocol := configuredImageProtocolWithOverride(settings.Protocol)
	scaleMode := configuredImageScaleModeWithOverride(settings.ScaleMode)
	rendered, err := termimg.New(decoded).
		Protocol(protocol).
		Scale(scaleMode).
		Size(width, height).
		Render()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(rendered) != "" {
		return rendered, nil
	}
	if protocol == termimg.Halfblocks {
		return rendered, nil
	}

	return termimg.New(decoded).
		Protocol(termimg.Halfblocks).
		Scale(scaleMode).
		Size(width, height).
		Render()
}

func configuredImageProtocol() termimg.Protocol {
	return configuredImageProtocolWithOverride(os.Getenv("MUSICON_IMAGE_PROTOCOL"))
}

func CanonicalImageRenderer(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "halfblocks", "halfblock", "unicode":
		return "halfblocks"
	case "auto":
		return "auto"
	case "kitty":
		return "kitty"
	case "sixel":
		return "sixel"
	case "iterm2", "iterm":
		return "iterm2"
	default:
		return "halfblocks"
	}
}

func ListUsableImageRenderers() []string {
	renderers := []string{"auto"}
	features := queryTerminalFeatures()
	if features != nil {
		if features.KittyGraphics {
			renderers = append(renderers, "kitty")
		}
		if features.ITerm2Graphics {
			renderers = append(renderers, "iterm2")
		}
		if features.SixelGraphics {
			renderers = append(renderers, "sixel")
		}
	}
	renderers = append(renderers, "halfblocks")
	return slices.Compact(renderers)
}

func TerminalCellWidthRatio() float64 {
	features := queryTerminalFeatures()
	if features == nil || features.FontWidth <= 0 || features.FontHeight <= 0 {
		return 0.5
	}
	return float64(features.FontWidth) / float64(features.FontHeight)
}

func configuredImageProtocolWithOverride(raw string) termimg.Protocol {
	switch CanonicalImageRenderer(raw) {
	case "halfblocks":
		return termimg.Halfblocks
	case "auto":
		return termimg.Auto
	case "kitty":
		return termimg.Kitty
	case "sixel":
		return termimg.Sixel
	case "iterm2", "iterm":
		return termimg.ITerm2
	default:
		return termimg.Halfblocks
	}
}

func configuredImageScaleMode() termimg.ScaleMode {
	return configuredImageScaleModeWithOverride(os.Getenv("MUSICON_IMAGE_SCALE"))
}

func configuredImageScaleModeWithOverride(raw string) termimg.ScaleMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "fill":
		return termimg.ScaleFill
	case "stretch":
		return termimg.ScaleStretch
	case "fit":
		return termimg.ScaleFit
	case "auto":
		return termimg.ScaleAuto
	case "none":
		return termimg.ScaleNone
	default:
		return termimg.ScaleFill
	}
}
