package audio

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	teaui "github.com/darkliquid/musicon/internal/ui"
	"github.com/gopxl/beep"
)

// This file holds the analysis side-channel used by the UI's EQ and visualizer
// panes. Playback remains the source of truth; visualization simply taps the
// outgoing audio stream and maintains a decaying spectral summary for rendering.

const (
	analysisFFTSize        = 1024
	analysisBandCount      = 24
	analysisMinFrequency   = 32.0
	analysisMaxFrequency   = 16_000.0
	analysisUpdateInterval = 40 * time.Millisecond
	analysisDecayInterval  = 180 * time.Millisecond
)

var eqGradientStops = []string{
	"#1d4ed8",
	"#06b6d4",
	"#22c55e",
	"#eab308",
	"#f97316",
	"#ef4444",
}

var brailleDotMask = [4][2]int{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

type visualizationState struct {
	mu         sync.RWMutex
	sampleRate beep.SampleRate
	buffer     []float64
	levels     []float64
	lastUpdate time.Time
	active     bool
}

type visualizationService struct {
	engine *Engine
}

type analysisTap struct {
	source beep.Streamer
	state  *visualizationState
}

func newVisualizationState(sampleRate beep.SampleRate) *visualizationState {
	return &visualizationState{
		sampleRate: sampleRate,
		levels:     make([]float64, analysisBandCount),
	}
}

func newAnalysisTap(source beep.Streamer, state *visualizationState) beep.Streamer {
	if state == nil {
		return source
	}
	return &analysisTap{source: source, state: state}
}

func (t *analysisTap) Stream(samples [][2]float64) (int, bool) {
	n, ok := t.source.Stream(samples)
	if n > 0 {
		t.state.Ingest(samples[:n])
	}
	return n, ok
}

func (t *analysisTap) Err() error {
	if streamer, ok := t.source.(interface{ Err() error }); ok {
		return streamer.Err()
	}
	return nil
}

func (s visualizationService) Placeholder(mode teaui.PlaybackPane, width, height int) (string, error) {
	if s.engine == nil || s.engine.visual == nil || width <= 0 || height <= 0 {
		return "", nil
	}
	levels, active := s.engine.visual.Snapshot()
	if !active {
		return "", nil
	}
	switch mode {
	case teaui.PaneEQ:
		return renderEQBars(levels, width, height), nil
	case teaui.PaneVisualizer:
		return renderMirrorBars(levels, width, height), nil
	default:
		return "", nil
	}
}

func (s *visualizationState) Reset(sampleRate beep.SampleRate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sampleRate > 0 {
		s.sampleRate = sampleRate
	}
	clear(s.levels)
	s.buffer = s.buffer[:0]
	s.lastUpdate = time.Time{}
	s.active = true
}

func (s *visualizationState) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	clear(s.levels)
	s.buffer = s.buffer[:0]
	s.lastUpdate = time.Time{}
	s.active = false
}

func (s *visualizationState) Ingest(samples [][2]float64) {
	if len(samples) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active || s.sampleRate <= 0 {
		return
	}

	for _, sample := range samples {
		mono := (sample[0] + sample[1]) * 0.5
		s.buffer = append(s.buffer, mono)
	}
	if len(s.buffer) > analysisFFTSize*2 {
		s.buffer = append(s.buffer[:0], s.buffer[len(s.buffer)-analysisFFTSize*2:]...)
	}
	if len(s.buffer) < analysisFFTSize {
		return
	}

	now := time.Now()
	if !s.lastUpdate.IsZero() && now.Sub(s.lastUpdate) < analysisUpdateInterval {
		return
	}

	frame := append([]float64(nil), s.buffer[len(s.buffer)-analysisFFTSize:]...)
	nextLevels := spectrumLevels(frame, float64(s.sampleRate), len(s.levels))
	for i, level := range nextLevels {
		current := s.levels[i]
		if level > current {
			s.levels[i] = current + (level-current)*0.65
			continue
		}
		s.levels[i] = max(level, current*0.82)
	}
	s.lastUpdate = now
}

func (s *visualizationState) Snapshot() ([]float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.active {
		return nil, false
	}

	levels := append([]float64(nil), s.levels...)
	if s.lastUpdate.IsZero() {
		return levels, true
	}

	idle := time.Since(s.lastUpdate)
	if idle <= 0 {
		return levels, true
	}

	decay := math.Pow(0.78, float64(idle)/float64(analysisDecayInterval))
	for i := range levels {
		levels[i] *= decay
	}
	return levels, true
}

func spectrumLevels(frame []float64, sampleRate float64, bands int) []float64 {
	if len(frame) == 0 || sampleRate <= 0 || bands <= 0 {
		return nil
	}

	real := make([]float64, len(frame))
	imag := make([]float64, len(frame))
	last := max(1, len(frame)-1)
	for i, sample := range frame {
		window := 0.5 - 0.5*math.Cos((2*math.Pi*float64(i))/float64(last))
		real[i] = sample * window
	}
	fft(real, imag)

	half := len(real) / 2
	levels := make([]float64, bands)
	maxFrequency := min(analysisMaxFrequency, sampleRate/2)
	if maxFrequency <= analysisMinFrequency {
		maxFrequency = sampleRate / 2
	}

	for band := range bands {
		low := logarithmicFrequency(analysisMinFrequency, maxFrequency, float64(band)/float64(bands))
		high := logarithmicFrequency(analysisMinFrequency, maxFrequency, float64(band+1)/float64(bands))
		lowBin := max(1, int(math.Floor((low/sampleRate)*float64(len(real)))))
		highBin := min(half, max(lowBin+1, int(math.Ceil((high/sampleRate)*float64(len(real))))))

		power := 0.0
		count := 0
		for bin := lowBin; bin <= highBin; bin++ {
			magnitude := math.Hypot(real[bin], imag[bin]) / float64(len(real))
			power += magnitude * magnitude
			count++
		}
		if count == 0 {
			continue
		}

		rms := math.Sqrt(power / float64(count))
		decibels := 20 * math.Log10(rms+1e-9)
		level := clampFloat((decibels+60)/60, 0, 1)
		levels[band] = level
	}

	return levels
}

func logarithmicFrequency(minFrequency, maxFrequency, ratio float64) float64 {
	if ratio <= 0 {
		return minFrequency
	}
	if ratio >= 1 {
		return maxFrequency
	}
	return minFrequency * math.Pow(maxFrequency/minFrequency, ratio)
}

func fft(real, imag []float64) {
	n := len(real)
	if n <= 1 {
		return
	}

	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for ; j >= bit; bit >>= 1 {
			j -= bit
		}
		j += bit
		if i < j {
			real[i], real[j] = real[j], real[i]
			imag[i], imag[j] = imag[j], imag[i]
		}
	}

	for length := 2; length <= n; length <<= 1 {
		angle := -2 * math.Pi / float64(length)
		wLenReal := math.Cos(angle)
		wLenImag := math.Sin(angle)
		for i := 0; i < n; i += length {
			wReal := 1.0
			wImag := 0.0
			half := length / 2
			for j := range half {
				uReal := real[i+j]
				uImag := imag[i+j]
				vIndex := i + j + half
				vReal := real[vIndex]*wReal - imag[vIndex]*wImag
				vImag := real[vIndex]*wImag + imag[vIndex]*wReal

				real[i+j] = uReal + vReal
				imag[i+j] = uImag + vImag
				real[vIndex] = uReal - vReal
				imag[vIndex] = uImag - vImag

				nextReal := wReal*wLenReal - wImag*wLenImag
				wImag = wReal*wLenImag + wImag*wLenReal
				wReal = nextReal
			}
		}
	}
}

func renderEQBars(levels []float64, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	pixelWidth := width * 2
	pixelHeight := height * 4
	columns := resampleLevels(levels, pixelWidth)
	return renderBrailleGrid(width, height,
		func(px, py int) bool {
			level := clampFloat(columns[px], 0, 1)
			filled := int(math.Round(level * float64(pixelHeight)))
			return filled > 0 && py >= pixelHeight-filled
		},
		func(cellRow, totalRows int) string {
			maxRow := max(1, totalRows-1)
			ratio := 1 - (float64(cellRow) / float64(maxRow))
			return gradientColorAt(eqGradientStops, ratio)
		},
	)
}

func renderMirrorBars(levels []float64, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	pixelWidth := width * 2
	pixelHeight := height * 4
	columns := resampleLevels(levels, pixelWidth)
	center := float64(pixelHeight) / 2
	return renderBrailleGrid(width, height,
		func(px, py int) bool {
			level := clampFloat(columns[px], 0, 1)
			radius := level * (float64(pixelHeight) / 2)
			return math.Abs((float64(py)+0.5)-center) <= radius
		},
		func(cellRow, totalRows int) string {
			center := float64(totalRows) / 2
			maxDistance := max(0.5, center-0.5)
			rowCenter := float64(cellRow) + 0.5
			ratio := math.Abs(rowCenter-center) / maxDistance
			return gradientColorAt(eqGradientStops, ratio)
		},
	)
}

func resampleLevels(levels []float64, count int) []float64 {
	if count <= 0 {
		return nil
	}
	if len(levels) == 0 {
		return make([]float64, count)
	}
	if count == 1 {
		return []float64{levels[0]}
	}

	out := make([]float64, count)
	if len(levels) == 1 {
		for i := range out {
			out[i] = levels[0]
		}
		return out
	}

	scale := float64(len(levels)-1) / float64(count-1)
	for i := range out {
		position := float64(i) * scale
		lower := int(math.Floor(position))
		upper := min(len(levels)-1, lower+1)
		mix := position - float64(lower)
		out[i] = levels[lower] + (levels[upper]-levels[lower])*mix
	}
	return out
}

func clampFloat(value, lower, upper float64) float64 {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}

func clampInt(value, lower, upper int) int {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}

func renderBrailleGrid(width, height int, active func(px, py int) bool, colorForRow func(cellRow, totalRows int) string) string {
	lines := make([]string, height)
	for cellRow := range height {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(colorForRow(cellRow, height)))
		var line strings.Builder
		line.Grow(width * 12)
		for cellCol := range width {
			mask := 0
			for localY := range 4 {
				for localX := range 2 {
					px := cellCol*2 + localX
					py := cellRow*4 + localY
					if active(px, py) {
						mask |= brailleDotMask[localY][localX]
					}
				}
			}
			if mask == 0 {
				line.WriteRune(' ')
				continue
			}
			line.WriteString(style.Render(string(brailleRune(mask))))
		}
		lines[cellRow] = line.String()
	}
	return strings.Join(lines, "\n")
}

func brailleRune(mask int) rune {
	return rune(0x2800 + clampInt(mask, 0, 0xFF))
}

func gradientColorAt(stops []string, ratio float64) string {
	if len(stops) == 0 {
		return "#ffffff"
	}
	if len(stops) == 1 {
		return stops[0]
	}
	ratio = clampFloat(ratio, 0, 1)
	scaled := ratio * float64(len(stops)-1)
	lower := int(math.Floor(scaled))
	upper := min(len(stops)-1, lower+1)
	if lower == upper {
		return normalizeHexColor(stops[lower])
	}
	mix := scaled - float64(lower)
	return interpolateHexColor(stops[lower], stops[upper], mix)
}

func interpolateHexColor(from, to string, mix float64) string {
	fromRGB, ok := parseHexColor(from)
	if !ok {
		fromRGB = [3]int{255, 255, 255}
	}
	toRGB, ok := parseHexColor(to)
	if !ok {
		toRGB = fromRGB
	}
	mix = clampFloat(mix, 0, 1)
	return fmt.Sprintf(
		"#%02x%02x%02x",
		int(math.Round(float64(fromRGB[0])+(float64(toRGB[0]-fromRGB[0])*mix))),
		int(math.Round(float64(fromRGB[1])+(float64(toRGB[1]-fromRGB[1])*mix))),
		int(math.Round(float64(fromRGB[2])+(float64(toRGB[2]-fromRGB[2])*mix))),
	)
}

func normalizeHexColor(value string) string {
	rgb, ok := parseHexColor(value)
	if !ok {
		return "#ffffff"
	}
	return fmt.Sprintf("#%02x%02x%02x", rgb[0], rgb[1], rgb[2])
}

func parseHexColor(value string) ([3]int, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 {
		return [3]int{}, false
	}
	r, err := strconv.ParseUint(value[0:2], 16, 8)
	if err != nil {
		return [3]int{}, false
	}
	g, err := strconv.ParseUint(value[2:4], 16, 8)
	if err != nil {
		return [3]int{}, false
	}
	b, err := strconv.ParseUint(value[4:6], 16, 8)
	if err != nil {
		return [3]int{}, false
	}
	return [3]int{int(r), int(g), int(b)}, true
}
