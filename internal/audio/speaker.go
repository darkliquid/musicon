package audio

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/darkliquid/mago"
	"github.com/gopxl/beep"
)

const speakerChannelCount = 2

type runtimeSpeaker struct {
	mu             sync.Mutex
	mixer          beep.Mixer
	current        *speakerState
	bufferDuration time.Duration
	backends       []mago.Backend
}

type speakerState struct {
	owner  *runtimeSpeaker
	lib    *mago.Library
	ctx    *mago.Context
	device *mago.Device

	scratch [][2]float64
}

func newRuntimeSpeaker(backends []mago.Backend) *runtimeSpeaker {
	return &runtimeSpeaker{backends: append([]mago.Backend(nil), backends...)}
}

func (s *runtimeSpeaker) Init(sampleRate beep.SampleRate, bufferSize int) error {
	if sampleRate <= 0 {
		return errors.New("speaker: sample rate must be positive")
	}
	if bufferSize <= 0 {
		return errors.New("speaker: buffer size must be positive")
	}

	s.mu.Lock()
	if s.current != nil {
		s.mu.Unlock()
		return errors.New("speaker cannot be initialized more than once")
	}
	s.mu.Unlock()

	lib, err := mago.Open()
	if err != nil {
		return err
	}

	var ctx *mago.Context
	if len(s.backends) == 0 {
		ctx, err = lib.NewContext()
	} else {
		ctx, err = lib.NewContext(s.backends...)
	}
	if err != nil {
		_ = lib.Close()
		return err
	}

	state := &speakerState{
		owner: s,
		lib:   lib,
		ctx:   ctx,
	}

	config := mago.DefaultPlaybackDeviceConfig()
	config.Format = mago.FormatF32
	config.Channels = speakerChannelCount
	if sampleRate > beep.SampleRate(math.MaxUint32) {
		_ = ctx.Close()
		_ = lib.Close()
		return fmt.Errorf("speaker: sample rate must be %d or less", uint32(math.MaxUint32))
	}
	if bufferSize > math.MaxUint32 {
		_ = ctx.Close()
		_ = lib.Close()
		return fmt.Errorf("speaker: buffer size must be %d or less", uint32(math.MaxUint32))
	}
	config.SampleRate = uint32(sampleRate)
	config.PeriodSizeInFrames = uint32(bufferSize)
	config.DataCallback = state.onDeviceData

	device, err := ctx.NewPlaybackDevice(config)
	if err != nil {
		_ = ctx.Close()
		_ = lib.Close()
		return err
	}
	state.device = device

	if err := device.Start(); err != nil {
		_ = device.Close()
		_ = ctx.Close()
		_ = lib.Close()
		return err
	}

	s.mu.Lock()
	if s.current != nil {
		s.mu.Unlock()
		_ = state.close()
		return errors.New("speaker cannot be initialized more than once")
	}
	s.mixer = beep.Mixer{}
	s.current = state
	s.bufferDuration = sampleRate.D(bufferSize)
	s.mu.Unlock()
	return nil
}

func (s *runtimeSpeaker) Close() {
	s.mu.Lock()
	state := s.current
	s.current = nil
	s.bufferDuration = 0
	s.mixer.Clear()
	s.mu.Unlock()

	if state != nil {
		_ = state.close()
	}
}

func (s *runtimeSpeaker) Lock() {
	s.mu.Lock()
}

func (s *runtimeSpeaker) Unlock() {
	s.mu.Unlock()
}

func (s *runtimeSpeaker) Play(streamers ...beep.Streamer) {
	s.mu.Lock()
	s.mixer.Add(streamers...)
	s.mu.Unlock()
}

func (s *runtimeSpeaker) Clear() {
	s.mu.Lock()
	s.mixer.Clear()
	s.mu.Unlock()
}

func (s *speakerState) close() error {
	var firstErr error
	if s.device != nil {
		if err := s.device.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.ctx != nil {
		if err := s.ctx.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.lib != nil {
		if err := s.lib.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *speakerState) onDeviceData(device *mago.Device, output unsafe.Pointer, input unsafe.Pointer, frameCount uint32) {
	_ = device
	_ = input
	if output == nil {
		return
	}

	out := unsafe.Slice((*float32)(output), int(frameCount)*speakerChannelCount)

	s.owner.mu.Lock()
	defer s.owner.mu.Unlock()
	streamToFloat32(&s.owner.mixer, &s.scratch, out)
}

func streamToFloat32(streamer beep.Streamer, scratch *[][2]float64, out []float32) {
	for i := range out {
		out[i] = 0
	}
	if streamer == nil || len(out) == 0 {
		return
	}

	frames := len(out) / speakerChannelCount
	if frames == 0 {
		return
	}
	if cap(*scratch) < frames {
		*scratch = make([][2]float64, frames)
	}
	samples := (*scratch)[:frames]
	n, _ := streamer.Stream(samples)
	for i := n; i < frames; i++ {
		samples[i] = [2]float64{}
	}
	for i := 0; i < frames; i++ {
		out[i*speakerChannelCount] = clampSample(samples[i][0])
		out[i*speakerChannelCount+1] = clampSample(samples[i][1])
	}
}

func clampSample(v float64) float32 {
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return float32(v)
}

func selectSpeakerBackends(raw string) ([]mago.Backend, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" || raw == "auto" {
		return nil, nil
	}

	backend, ok := parseSpeakerBackend(raw)
	if !ok {
		return nil, fmt.Errorf("unsupported audio backend %q", raw)
	}
	return []mago.Backend{backend}, nil
}

func parseSpeakerBackend(raw string) (mago.Backend, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "wasapi":
		return mago.BackendWASAPI, true
	case "dsound", "directsound":
		return mago.BackendDSound, true
	case "winmm":
		return mago.BackendWinMM, true
	case "coreaudio":
		return mago.BackendCoreAudio, true
	case "sndio":
		return mago.BackendSndIO, true
	case "audio4", "audio(4)":
		return mago.BackendAudio4, true
	case "oss":
		return mago.BackendOSS, true
	case "pulse", "pulseaudio":
		return mago.BackendPulseAudio, true
	case "alsa":
		return mago.BackendALSA, true
	case "jack":
		return mago.BackendJACK, true
	case "aaudio":
		return mago.BackendAAudio, true
	case "opensl":
		return mago.BackendOpenSL, true
	case "webaudio":
		return mago.BackendWebAudio, true
	case "null":
		return mago.BackendNull, true
	default:
		return 0, false
	}
}
