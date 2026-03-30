package components

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"slices"
	"strings"

	chafa "github.com/ploMP4/chafa-go"
)

const defaultTerminalCellWidthRatio = 0.5

type terminalImageCapabilities struct {
	Kitty  bool
	ITerm2 bool
	Sixel  bool
}

var (
	detectTerminalImageCapabilities = defaultDetectTerminalImageCapabilities
	detectChafaTermInfo             = func(protocol string) (*chafa.TermInfo, func()) {
		return defaultDetectChafaTermInfo(protocol)
	}
)

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

// NewTerminalImage constructs a terminal image component backed by chafa-go.
func NewTerminalImage() *TerminalImage {
	return NewTerminalImageWithRenderer(ImageRendererFunc(renderWithChafa))
}

// NewTerminalImageWithSettings constructs a terminal image component with
// explicit rendering settings.
func NewTerminalImageWithSettings(settings TerminalImageSettings) *TerminalImage {
	image := NewTerminalImageWithRenderer(ImageRendererFunc(renderWithChafa))
	image.settings = settings
	return image
}

// NewTerminalImageWithRenderer constructs a terminal image component with a
// caller-supplied renderer. This is primarily useful for tests.
func NewTerminalImageWithRenderer(renderer ImageRenderer) *TerminalImage {
	if renderer == nil {
		renderer = ImageRendererFunc(renderWithChafa)
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
		return renderWithChafaSettings(source, width, height, i.settings)
	}
	return i.renderer.Render(source, width, height)
}

func renderWithChafa(source ImageSource, width, height int) (string, error) {
	return renderWithChafaSettings(source, width, height, TerminalImageSettings{})
}

func renderWithChafaSettings(source ImageSource, width, height int, settings TerminalImageSettings) (string, error) {
	decoded, _, err := image.Decode(bytes.NewReader(source.Data))
	if err != nil {
		return "", err
	}

	scaleMode := effectiveImageScaleMode(settings.ScaleMode)
	prepared := prepareImageForScaleMode(decoded, width, height, scaleMode)

	renderer := effectiveImageProtocol(settings.Protocol)
	termInfo, release := detectChafaTermInfo(renderer)
	defer release()

	rendered, err := renderPreparedImage(prepared, width, height, renderer, scaleMode, termInfo)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(rendered) != "" || renderer == "halfblocks" {
		return rendered, nil
	}

	return renderPreparedImage(prepared, width, height, "halfblocks", scaleMode, termInfo)
}

func renderPreparedImage(decoded image.Image, width, height int, renderer, scaleMode string, termInfo *chafa.TermInfo) (string, error) {
	rgba := cloneToRGBA(decoded)
	srcWidth := rgba.Bounds().Dx()
	srcHeight := rgba.Bounds().Dy()
	destWidth, destHeight := chafaGeometry(srcWidth, srcHeight, width, height, scaleMode)

	config := chafa.CanvasConfigNew()
	defer chafa.CanvasConfigUnref(config)

	chafa.CanvasConfigSetGeometry(config, destWidth, destHeight)
	chafa.CanvasConfigSetCellGeometry(config, 1, 2)

	if termInfo != nil {
		chafa.CanvasConfigSetCanvasMode(config, chafa.TermInfoGetBestCanvasMode(termInfo))
	} else {
		chafa.CanvasConfigSetCanvasMode(config, chafa.CHAFA_CANVAS_MODE_TRUECOLOR)
	}

	pixelMode := chafaPixelModeForRenderer(termInfo, renderer)
	chafa.CanvasConfigSetPixelMode(config, pixelMode)
	if termInfo != nil && chafa.TermInfoGetIsPixelPassthroughNeeded(termInfo, pixelMode) {
		chafa.CanvasConfigSetPassthrough(config, chafa.TermInfoGetPassthroughType(termInfo))
	}
	if pixelMode == chafa.CHAFA_PIXEL_MODE_SYMBOLS {
		applyHalfblockSymbolMap(config, termInfo)
	}

	canvas := chafa.CanvasNew(config)
	defer chafa.CanvasUnRef(canvas)

	chafa.CanvasDrawAllPixels(
		canvas,
		chafa.CHAFA_PIXEL_RGBA8_UNASSOCIATED,
		rgba.Pix,
		int32(srcWidth),
		int32(srcHeight),
		int32(rgba.Stride),
	)

	gs := chafa.CanvasPrint(canvas, termInfo)
	if gs == nil {
		return "", fmt.Errorf("chafa renderer produced no output")
	}
	return gs.String(), nil
}

func configuredImageProtocol() string {
	return effectiveImageProtocol("")
}

func EffectiveImageRenderer(explicit string) string {
	return effectiveImageProtocol(explicit)
}

func CanonicalImageRenderer(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "halfblocks", "halfblock", "unicode", "symbols":
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
	capabilities := detectTerminalImageCapabilities()
	if capabilities.Kitty {
		renderers = append(renderers, "kitty")
	}
	if capabilities.ITerm2 {
		renderers = append(renderers, "iterm2")
	}
	if capabilities.Sixel {
		renderers = append(renderers, "sixel")
	}
	renderers = append(renderers, "halfblocks")
	return slices.Compact(renderers)
}

func TerminalCellWidthRatio() float64 {
	return defaultTerminalCellWidthRatio
}

func configuredImageProtocolWithOverride(raw string) string {
	return CanonicalImageRenderer(raw)
}

func configuredImageScaleMode() string {
	return effectiveImageScaleMode("")
}

func effectiveImageProtocol(explicit string) string {
	if env := strings.TrimSpace(os.Getenv("MUSICON_IMAGE_PROTOCOL")); env != "" {
		return configuredImageProtocolWithOverride(env)
	}
	return configuredImageProtocolWithOverride(explicit)
}

func effectiveImageScaleMode(explicit string) string {
	if env := strings.TrimSpace(os.Getenv("MUSICON_IMAGE_SCALE")); env != "" {
		return configuredImageScaleModeWithOverride(env)
	}
	return configuredImageScaleModeWithOverride(explicit)
}

func configuredImageScaleModeWithOverride(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "fill":
		return "fill"
	case "stretch":
		return "stretch"
	case "fit":
		return "fit"
	case "auto":
		return "auto"
	case "none":
		return "none"
	default:
		return "fill"
	}
}

func defaultDetectTerminalImageCapabilities() terminalImageCapabilities {
	termInfo, release := detectChafaTermInfo("auto")
	defer release()
	if termInfo == nil {
		return terminalImageCapabilities{}
	}
	return terminalImageCapabilities{
		Kitty:  chafa.TermInfoIsPixelModeSupported(termInfo, chafa.CHAFA_PIXEL_MODE_KITTY),
		ITerm2: chafa.TermInfoIsPixelModeSupported(termInfo, chafa.CHAFA_PIXEL_MODE_ITERM2),
		Sixel:  chafa.TermInfoIsPixelModeSupported(termInfo, chafa.CHAFA_PIXEL_MODE_SIXELS),
	}
}

func defaultDetectChafaTermInfo(protocol string) (*chafa.TermInfo, func()) {
	db := chafa.TermDbGetDefault()
	if db == nil {
		return nil, func() {}
	}

	// Determine the effective protocol to see if we really need detection.
	if protocol == "" {
		protocol = strings.ToLower(strings.TrimSpace(os.Getenv("MUSICON_IMAGE_PROTOCOL")))
	}

	// If we've specified a protocol that isn't 'auto', we skip detection entirely to avoid crashes
	// in Chafa's TermDbDetect, which can be unstable in some environments.
	// We treat empty as halfblocks (default) which doesn't need detection.
	if protocol != "" && protocol != "auto" {
		fallback := chafa.TermDbGetFallbackInfo(db)
		if fallback == nil {
			return nil, func() {}
		}
		return fallback, func() {
			chafa.TermInfoUnref(fallback)
		}
	}

	// For 'auto', we still try to detect, but pass a minimal environment to be safe.
	// If the user hasn't specified anything, we default to halfblocks and skip detection.
	if protocol == "" {
		fallback := chafa.TermDbGetFallbackInfo(db)
		if fallback == nil {
			return nil, func() {}
		}
		return fallback, func() {
			chafa.TermInfoUnref(fallback)
		}
	}

	// For 'auto', we still try to detect, but pass a minimal environment to be safe.
	var env []string
	if term := os.Getenv("TERM"); term != "" {
		env = append(env, "TERM="+term)
	}
	if colorterm := os.Getenv("COLORTERM"); colorterm != "" {
		env = append(env, "COLORTERM="+colorterm)
	}

	termInfo := chafa.TermDbDetect(db, env)
	if termInfo == nil {
		fallback := chafa.TermDbGetFallbackInfo(db)
		if fallback == nil {
			return nil, func() {}
		}
		return fallback, func() {
			chafa.TermInfoUnref(fallback)
		}
	}

	fallback := chafa.TermDbGetFallbackInfo(db)
	if fallback != nil {
		chafa.TermInfoSupplement(termInfo, fallback)
		chafa.TermInfoUnref(fallback)
	}

	return termInfo, func() {
		chafa.TermInfoUnref(termInfo)
	}
}

func chafaPixelModeForRenderer(termInfo *chafa.TermInfo, renderer string) chafa.PixelMode {
	// If a specific renderer was forced, we respect it even if TermInfo doesn't explicitly support it,
	// as we may have skipped detection to avoid a crash.
	forced := strings.ToLower(strings.TrimSpace(os.Getenv("MUSICON_IMAGE_PROTOCOL")))

	switch renderer {
	case "kitty":
		if forced == "kitty" || (termInfo != nil && chafa.TermInfoIsPixelModeSupported(termInfo, chafa.CHAFA_PIXEL_MODE_KITTY)) {
			return chafa.CHAFA_PIXEL_MODE_KITTY
		}
	case "sixel":
		if forced == "sixel" || (termInfo != nil && chafa.TermInfoIsPixelModeSupported(termInfo, chafa.CHAFA_PIXEL_MODE_SIXELS)) {
			return chafa.CHAFA_PIXEL_MODE_SIXELS
		}
	case "iterm2":
		if forced == "iterm2" || (termInfo != nil && chafa.TermInfoIsPixelModeSupported(termInfo, chafa.CHAFA_PIXEL_MODE_ITERM2)) {
			return chafa.CHAFA_PIXEL_MODE_ITERM2
		}
	case "auto":
		if termInfo != nil {
			return chafa.TermInfoGetBestPixelMode(termInfo)
		}
	}
	return chafa.CHAFA_PIXEL_MODE_SYMBOLS
}

func applyHalfblockSymbolMap(config *chafa.CanvasConfig, termInfo *chafa.TermInfo) {
	symbolMap := chafa.SymbolMapNew()
	defer chafa.SymbolMapUnref(symbolMap)

	tags := chafa.CHAFA_SYMBOL_TAG_SPACE | chafa.CHAFA_SYMBOL_TAG_SOLID | chafa.CHAFA_SYMBOL_TAG_HALF
	if termInfo != nil {
		safeTags := chafa.TermInfoGetSafeSymbolTags(termInfo)
		filtered := tags & safeTags
		if filtered != 0 {
			tags = filtered
		}
	}
	chafa.SymbolMapAddByTags(symbolMap, tags)
	chafa.CanvasConfigSetSymbolMap(config, symbolMap)
}

func cloneToRGBA(img image.Image) *image.RGBA {
	bounds := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, bounds.Min, draw.Src)
	return rgba
}

func prepareImageForScaleMode(img image.Image, width, height int, scaleMode string) image.Image {
	switch scaleMode {
	case "fill", "auto":
		targetAspect := (float64(maxInt(width, 1)) * defaultTerminalCellWidthRatio) / float64(maxInt(height, 1))
		return cropImageToAspect(img, targetAspect)
	default:
		return img
	}
}

func cropImageToAspect(img image.Image, targetAspect float64) image.Image {
	if targetAspect <= 0 {
		return img
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return img
	}

	currentAspect := float64(width) / float64(height)
	if almostEqual(currentAspect, targetAspect) {
		return img
	}

	if currentAspect > targetAspect {
		croppedWidth := int(float64(height) * targetAspect)
		if croppedWidth <= 0 || croppedWidth >= width {
			return img
		}
		left := bounds.Min.X + (width-croppedWidth)/2
		return cropImage(img, image.Rect(left, bounds.Min.Y, left+croppedWidth, bounds.Max.Y))
	}

	croppedHeight := int(float64(width) / targetAspect)
	if croppedHeight <= 0 || croppedHeight >= height {
		return img
	}
	top := bounds.Min.Y + (height-croppedHeight)/2
	return cropImage(img, image.Rect(bounds.Min.X, top, bounds.Max.X, top+croppedHeight))
}

func cropImage(img image.Image, rect image.Rectangle) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(dst, dst.Bounds(), img, rect.Min, draw.Src)
	return dst
}

func chafaGeometry(srcWidth, srcHeight, width, height int, scaleMode string) (int32, int32) {
	destWidth := int32(maxInt(width, 1))
	destHeight := int32(maxInt(height, 1))
	zoom, stretch := chafaGeometryPreferences(scaleMode)
	chafa.CalcCanvasGeometry(
		int32(maxInt(srcWidth, 1)),
		int32(maxInt(srcHeight, 1)),
		&destWidth,
		&destHeight,
		float32(defaultTerminalCellWidthRatio),
		zoom,
		stretch,
	)
	if destWidth < 1 {
		destWidth = 1
	}
	if destHeight < 1 {
		destHeight = 1
	}
	return destWidth, destHeight
}

func chafaGeometryPreferences(scaleMode string) (zoom, stretch bool) {
	switch scaleMode {
	case "none":
		return false, false
	case "stretch":
		return true, true
	default:
		return true, false
	}
}

func almostEqual(left, right float64) bool {
	diff := left - right
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-9
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
