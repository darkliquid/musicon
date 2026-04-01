package youtube

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	webmreader "github.com/ebml-go/webm"
	"github.com/gopxl/beep"
	opusogg "github.com/kazzmir/opus-go/ogg"
	opusdecoder "github.com/kazzmir/opus-go/opus"
	mkvparse "github.com/remko/go-mkvparse"
)

// This file holds the actual streaming and decode machinery.
//
// The central idea is:
//   - use ebml-go/webm for cue-aware packet access
//   - decode Opus packets in-process
//   - keep decoded PCM in a bounded sliding window
//   - satisfy beep's `StreamSeekCloser` contract on top of that window

// pcmBufferStreamer is the simplest fully in-memory streamer used by the older
// full-decode path.
type pcmBufferStreamer struct {
	samples []int16
	pos     int
	closed  bool
}

// bufferedPCMStreamer incrementally fills a growing PCM buffer from a background
// decoder goroutine.
type bufferedPCMStreamer struct {
	mu          sync.Mutex
	cond        *sync.Cond
	samples     []int16
	pos         int
	totalFrames int
	closed      bool
	eof         bool
	err         error
	closeFn     func() error
}

// webmOpusDecoder is an mkvparse handler that extracts the first Opus audio
// track and converts Matroska blocks into stereo PCM.
type webmOpusDecoder struct {
	mkvparse.DefaultHandler

	inTrackEntry  bool
	inAudio       bool
	currentTrack  trackInfo
	targetTrack   trackInfo
	clusterTime   int64
	decoder       *opusdecoder.Decoder
	preSkipFrames int
	stereoPCM     []int16
	emit          func([]int16) error
}

type trackInfo struct {
	number   int
	codecID  string
	channels int
	private  []byte
}

// cueSeekableOpusStream is the package's main playback streamer.
//
// It keeps a fixed-size PCM window around the current playhead so playback can
// start quickly without decoding the entire track up front.
type cueSeekableOpusStream struct {
	mu sync.Mutex

	cond         *sync.Cond
	buffer       []int16
	windowFrames int
	backFrames   int
	aheadFrames  int
	totalFrames  int
	sampleRate   beep.SampleRate

	windowStart int
	windowEnd   int
	pos         int

	closed  bool
	eof     bool
	seeking bool
	err     error

	reader *webmreader.Reader
	head   opusogg.OpusHead

	onClose func() error
}

type seekRequest struct {
	target int
	done   chan error
}

// newSeekableWebMOpusStream wires the range reader, WebM parser, and bounded
// PCM window together into a `beep.StreamSeekCloser`.
func newSeekableWebMOpusStream(ctx context.Context, media io.ReadSeeker, duration time.Duration, startSample int) (*cueSeekableOpusStream, beep.Format, error) {
	var parsed webmreader.WebM
	reader, err := webmreader.Parse(media, &parsed)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("parse webm stream: %w", err)
	}
	audioTrack := parsed.FindFirstAudioTrack()
	if audioTrack == nil {
		return nil, beep.Format{}, errors.New("webm stream has no audio track")
	}
	head, err := parseOpusHead(audioTrack.CodecPrivate, int(audioTrack.Channels))
	if err != nil {
		return nil, beep.Format{}, err
	}
	sampleRate := beep.SampleRate(int(audioTrack.SamplingFrequency + 0.5))
	if sampleRate <= 0 {
		sampleRate = beep.SampleRate(opusogg.OpusSampleRateHz)
	}
	windowFrames := max(initialPCMBufferBytes/4, sampleRate.N(initialBufferDuration)*2)
	stream := &cueSeekableOpusStream{
		buffer:       make([]int16, windowFrames*2),
		windowFrames: windowFrames,
		backFrames:   windowFrames / 2,
		aheadFrames:  windowFrames / 2,
		totalFrames:  sampleRate.N(duration),
		sampleRate:   sampleRate,
		reader:       reader,
		head:         head,
		pos:          max(startSample, 0),
	}
	stream.cond = sync.NewCond(&stream.mu)
	go stream.decodeLoop(ctx)
	bufferTarget := max(startSample, 0)
	if err := stream.waitForBuffered(ctx, bufferTarget, sampleRate.N(initialBufferDuration)); err != nil {
		_ = stream.Close()
		return nil, beep.Format{}, err
	}
	return stream, beep.Format{SampleRate: sampleRate, NumChannels: 2, Precision: 2}, nil
}

// decodeLoop is the long-lived worker that:
//   - waits until the playback window needs more data
//   - consumes packets from the WebM reader
//   - decodes Opus packets into stereo PCM
//   - appends the PCM into the bounded playback window
//
// It also handles the package's remaining internal seek path used for startup
// positioning.
func (s *cueSeekableOpusStream) decodeLoop(ctx context.Context) {
	decoder, decoderErr := opusdecoder.NewDecoderFromHead(s.head)
	if decoderErr != nil {
		s.finish(decoderErr)
		return
	}
	defer decoder.Close()

	currentSample := 0
	dropUntil := max(s.Position(), 0)

	for {
		if err := s.waitForDecodeDemand(ctx); err != nil {
			s.finish(err)
			return
		}
		select {
		case <-ctx.Done():
			s.finish(ctx.Err())
			return
		case packet, ok := <-s.reader.Chan:
			if !ok {
				s.finish(nil)
				return
			}
			if len(packet.Data) == 0 {
				if packet.Timecode == webmreader.BadTC {
					s.finish(nil)
					return
				}
				currentSample = s.sampleRate.N(packet.Timecode)
				dropUntil = currentSample
				continue
			}
			if packet.Timecode != webmreader.BadTC {
				currentSample = s.sampleRate.N(packet.Timecode)
			}
			pcmBuffer := make([]int16, opusFrameSize*int(s.head.Channels))
			samplesPerChannel, err := decoder.Decode(packet.Data, pcmBuffer, opusFrameSize, false)
			if err != nil {
				s.finish(fmt.Errorf("decode opus packet: %w", err))
				return
			}
			decoded := pcmBuffer[:samplesPerChannel*int(s.head.Channels)]
			startSample := currentSample
			if startSample == 0 && s.head.PreSkip > 0 {
				skip := min(int(s.head.PreSkip), samplesPerChannel)
				decoded = decoded[skip*int(s.head.Channels):]
				startSample += skip
			}
			currentSample += samplesPerChannel
			if currentSample <= dropUntil {
				continue
			}
			if startSample < dropUntil {
				trim := dropUntil - startSample
				if trim < samplesPerChannel {
					decoded = decoded[trim*int(s.head.Channels):]
					startSample = dropUntil
				}
			}
			// The rest of Musicon assumes stereo float samples, so all decoded
			// packets are normalized into stereo PCM here regardless of the
			// original Opus channel count.
			stereo := interleavedToStereo(decoded, int(s.head.Channels))
			s.appendFrames(startSample, stereo)
		}
	}
}

// waitForDecodeDemand blocks the decoder until playback has consumed enough of
// the forward buffer that more PCM is needed.
func (s *cueSeekableOpusStream) waitForDecodeDemand(ctx context.Context) error {
	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return nil
		}
		if s.err != nil {
			err := s.err
			s.mu.Unlock()
			return err
		}
		if s.eof {
			s.mu.Unlock()
			return nil
		}
		if s.seeking {
			s.mu.Unlock()
			return nil
		}
		if s.windowEnd-s.pos < s.aheadFrames {
			s.mu.Unlock()
			return nil
		}
		s.cond.Wait()
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

// appendFrames writes decoded PCM into the circular playback window.
//
// The write position is tracked in absolute sample space; the modulo operation
// only decides where those absolute samples live in the backing buffer.
func (s *cueSeekableOpusStream) appendFrames(startSample int, stereo []int16) {
	if len(stereo) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	frames := len(stereo) / 2
	if frames > s.windowFrames {
		stereo = stereo[len(stereo)-s.windowFrames*2:]
		startSample = startSample + frames - s.windowFrames
		frames = s.windowFrames
	}
	endSample := startSample + frames
	minStart := max(endSample-s.windowFrames, 0)
	if s.windowStart < minStart {
		s.windowStart = minStart
	}
	if startSample > s.windowEnd {
		// If decode resumes after a gap, zero-fill the skipped region so stale
		// samples are never replayed out of the reused ring slots.
		gap := min(startSample-s.windowEnd, s.windowFrames)
		for i := 0; i < gap; i++ {
			dst := ((s.windowEnd + i) % s.windowFrames) * 2
			s.buffer[dst] = 0
			s.buffer[dst+1] = 0
		}
	}
	for i := 0; i < frames; i++ {
		dst := ((startSample + i) % s.windowFrames) * 2
		src := i * 2
		copy(s.buffer[dst:dst+2], stereo[src:src+2])
	}
	if s.windowStart == 0 && s.windowEnd == 0 {
		s.windowStart = startSample
	}
	if endSample > s.windowEnd {
		s.windowEnd = endSample
	}
	if s.windowEnd-s.windowStart > s.windowFrames {
		s.windowStart = s.windowEnd - s.windowFrames
	}
	if s.pos < s.windowStart {
		s.pos = s.windowStart
	}
	s.cond.Broadcast()
}

// waitForBuffered is a startup helper that waits until a minimum amount of PCM
// exists ahead of the requested target sample.
func (s *cueSeekableOpusStream) waitForBuffered(ctx context.Context, target, minFrames int) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		s.mu.Lock()
		windowEnd := s.windowEnd
		err := s.err
		eof := s.eof
		closed := s.closed
		s.mu.Unlock()
		switch {
		case windowEnd-target >= minFrames:
			return nil
		case err != nil:
			return err
		case eof:
			if windowEnd > target {
				return nil
			}
			return io.EOF
		case closed:
			return errors.New("youtube stream closed")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *cueSeekableOpusStream) hasBuffered(target, minFrames int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.windowEnd-target >= minFrames
}

func (s *cueSeekableOpusStream) finish(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed && shouldIgnoreStreamErr(err) {
		err = nil
	}
	if err != nil && !shouldIgnoreStreamErr(err) {
		s.err = err
	}
	s.eof = true
	s.seeking = false
	s.cond.Broadcast()
}

// Stream satisfies beep.Streamer by decoding or replaying buffered Opus samples.
func (s *cueSeekableOpusStream) Stream(samples [][2]float64) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		if s.closed {
			return 0, false
		}
		if s.err != nil {
			return 0, false
		}
		if s.pos < s.windowStart || s.pos > s.windowEnd {
			if s.seeking {
				zeroStereo(samples)
				return len(samples), true
			}
			return 0, false
		}
		available := s.windowEnd - s.pos
		if available > 0 {
			frames := min(available, len(samples))
			for i := 0; i < frames; i++ {
				src := ((s.pos + i) % s.windowFrames) * 2
				samples[i][0] = float64(s.buffer[src]) / 32768
				samples[i][1] = float64(s.buffer[src+1]) / 32768
			}
			s.pos += frames
			if s.pos-s.windowStart > s.backFrames {
				s.windowStart = s.pos - s.backFrames
			}
			s.cond.Broadcast()
			return frames, true
		}
		if s.eof {
			return 0, false
		}
		if s.seeking {
			zeroStereo(samples)
			return len(samples), true
		}
		s.cond.Wait()
	}
}

// Err reports the first decode or transport error encountered by the stream.
func (s *cueSeekableOpusStream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Len reports the total sample count when the stream duration is known.
func (s *cueSeekableOpusStream) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.totalFrames > 0 {
		return s.totalFrames
	}
	return s.windowEnd
}

// Position reports the current sample position within the stream.
func (s *cueSeekableOpusStream) Position() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pos
}

// Seek moves playback to the requested absolute sample position when possible.
func (s *cueSeekableOpusStream) Seek(p int) error {
	s.mu.Lock()
	if p < 0 {
		p = 0
	}
	if s.totalFrames > 0 && p > s.totalFrames {
		p = s.totalFrames
	}
	if p >= s.windowStart && p <= s.windowEnd {
		s.pos = p
		s.cond.Broadcast()
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	return fmt.Errorf("youtube seek outside buffered window is temporarily disabled")
}

func (s *cueSeekableOpusStream) canSeekWithinWindow(target int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return target >= s.windowStart && target <= s.windowEnd
}

func zeroStereo(samples [][2]float64) {
	for i := range samples {
		samples[i][0] = 0
		samples[i][1] = 0
	}
}

// Close releases the cue-seekable stream and its decode resources.
func (s *cueSeekableOpusStream) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.cond.Broadcast()
	s.mu.Unlock()
	if s.reader != nil {
		s.reader.Shutdown()
	}
	if s.onClose != nil {
		return s.onClose()
	}
	return nil
}

func decodeWebMOpus(reader io.Reader) (beep.StreamSeekCloser, beep.Format, error) {
	decoder := &webmOpusDecoder{}
	if err := mkvparse.Parse(reader, decoder); err != nil {
		return nil, beep.Format{}, fmt.Errorf("parse webm opus stream: %w", err)
	}
	if decoder.decoder != nil {
		_ = decoder.decoder.Close()
	}
	if len(decoder.stereoPCM) == 0 {
		return nil, beep.Format{}, errors.New("youtube stream decoded no audio")
	}
	return &pcmBufferStreamer{samples: decoder.stereoPCM}, beep.Format{SampleRate: beep.SampleRate(opusogg.OpusSampleRateHz), NumChannels: 2, Precision: 2}, nil
}

// streamWebMOpus powers the incremental stdout-based decode path by feeding a
// growing PCM buffer from a background parser goroutine.
func streamWebMOpus(ctx context.Context, reader io.ReadCloser, wait func() error, totalFrames int) (beep.StreamSeekCloser, beep.Format, error) {
	streamer := newBufferedPCMStreamer(totalFrames, func() error {
		var closeErr error
		if reader != nil {
			closeErr = reader.Close()
		}
		waitErr := wait()
		if closeErr != nil {
			return closeErr
		}
		return waitErr
	})
	decoder := &webmOpusDecoder{
		emit: func(samples []int16) error {
			streamer.append(samples)
			return nil
		},
	}
	go func() {
		err := mkvparse.Parse(reader, decoder)
		if decoder.decoder != nil {
			_ = decoder.decoder.Close()
		}
		waitErr := wait()
		if err == nil {
			err = waitErr
		} else if waitErr != nil {
			err = fmt.Errorf("%v; yt-dlp failed: %w", err, waitErr)
		}
		streamer.finish(err)
	}()
	bufferFrames := beep.SampleRate(opusogg.OpusSampleRateHz).N(initialBufferDuration)
	if err := streamer.waitForBuffer(ctx, bufferFrames); err != nil {
		_ = streamer.Close()
		return nil, beep.Format{}, err
	}
	return streamer, beep.Format{SampleRate: beep.SampleRate(opusogg.OpusSampleRateHz), NumChannels: 2, Precision: 2}, nil
}

func newBufferedPCMStreamer(totalFrames int, closeFn func() error) *bufferedPCMStreamer {
	streamer := &bufferedPCMStreamer{
		totalFrames: totalFrames,
		closeFn:     closeFn,
		samples:     make([]int16, 0, initialPCMBufferBytes/2),
	}
	streamer.cond = sync.NewCond(&streamer.mu)
	return streamer
}

func (s *bufferedPCMStreamer) append(samples []int16) {
	if len(samples) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.samples = append(s.samples, samples...)
	s.cond.Broadcast()
}

func (s *bufferedPCMStreamer) finish(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed && shouldIgnoreStreamErr(err) {
		err = nil
	}
	if err != nil && !shouldIgnoreStreamErr(err) {
		s.err = err
	}
	s.eof = true
	s.cond.Broadcast()
}

func (s *bufferedPCMStreamer) waitForBuffer(ctx context.Context, frames int) error {
	if frames <= 0 {
		return nil
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		s.mu.Lock()
		decoded := len(s.samples) / 2
		eof := s.eof
		err := s.err
		closed := s.closed
		s.mu.Unlock()

		switch {
		case decoded >= frames:
			return nil
		case err != nil:
			return err
		case eof:
			if decoded > 0 {
				return nil
			}
			return errors.New("youtube stream decoded no audio")
		case closed:
			return errors.New("youtube stream closed")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Stream satisfies beep.Streamer by draining buffered PCM samples.
func (s *bufferedPCMStreamer) Stream(samples [][2]float64) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		if s.closed {
			return 0, false
		}
		available := len(s.samples)/2 - s.pos
		if available > 0 {
			frames := min(available, len(samples))
			for i := 0; i < frames; i++ {
				base := (s.pos + i) * 2
				samples[i][0] = float64(s.samples[base]) / 32768
				samples[i][1] = float64(s.samples[base+1]) / 32768
			}
			s.pos += frames
			return frames, s.pos < len(s.samples)/2 || !s.eof
		}
		if s.eof {
			return 0, false
		}
		s.cond.Wait()
	}
}

// Err reports any buffered decode error.
func (s *bufferedPCMStreamer) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Len reports the total sample length advertised by the buffered stream.
func (s *bufferedPCMStreamer) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.totalFrames > 0 {
		return s.totalFrames
	}
	return len(s.samples) / 2
}

// Position reports the current sample offset within the buffered stream.
func (s *bufferedPCMStreamer) Position() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pos
}

// Seek repositions the buffered stream to the requested absolute sample offset.
func (s *bufferedPCMStreamer) Seek(p int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p < 0 {
		p = 0
	}
	if s.totalFrames > 0 && p > s.totalFrames {
		p = s.totalFrames
	}
	s.pos = p
	s.cond.Broadcast()
	return nil
}

// Close releases buffered streamer resources.
func (s *bufferedPCMStreamer) Close() error {
	s.mu.Lock()
	alreadyClosed := s.closed
	s.closed = true
	s.cond.Broadcast()
	closeFn := s.closeFn
	s.mu.Unlock()
	if alreadyClosed {
		return nil
	}
	if closeFn != nil {
		return closeFn()
	}
	return nil
}

func shouldIgnoreStreamErr(err error) bool {
	return err == nil ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, os.ErrClosed)
}

// HandleMasterBegin implements mkvparse.Handler for the start of master elements.
func (d *webmOpusDecoder) HandleMasterBegin(id mkvparse.ElementID, info mkvparse.ElementInfo) (bool, error) {
	switch id {
	case mkvparse.TrackEntryElement:
		d.inTrackEntry = true
		d.currentTrack = trackInfo{}
	case mkvparse.AudioElement:
		d.inAudio = true
	case mkvparse.ClusterElement:
		d.clusterTime = 0
	}
	return true, nil
}

// HandleMasterEnd implements mkvparse.Handler for the end of master elements.
func (d *webmOpusDecoder) HandleMasterEnd(id mkvparse.ElementID, info mkvparse.ElementInfo) error {
	switch id {
	case mkvparse.TrackEntryElement:
		if d.currentTrack.codecID == "A_OPUS" && d.currentTrack.number > 0 && d.decoder == nil {
			head, err := parseOpusHead(d.currentTrack.private, d.currentTrack.channels)
			if err != nil {
				return err
			}
			decoder, err := opusdecoder.NewDecoderFromHead(head)
			if err != nil {
				return fmt.Errorf("create opus decoder: %w", err)
			}
			d.decoder = decoder
			d.targetTrack = d.currentTrack
			d.preSkipFrames = int(head.PreSkip)
		}
		d.inTrackEntry = false
		d.currentTrack = trackInfo{}
	case mkvparse.AudioElement:
		d.inAudio = false
	}
	return nil
}

// HandleString implements mkvparse.Handler for string-valued elements.
func (d *webmOpusDecoder) HandleString(id mkvparse.ElementID, value string, info mkvparse.ElementInfo) error {
	if d.inTrackEntry && id == mkvparse.CodecIDElement {
		d.currentTrack.codecID = strings.TrimSpace(value)
	}
	return nil
}

// HandleInteger implements mkvparse.Handler for integer-valued elements.
func (d *webmOpusDecoder) HandleInteger(id mkvparse.ElementID, value int64, info mkvparse.ElementInfo) error {
	switch {
	case d.inTrackEntry && id == mkvparse.TrackNumberElement:
		d.currentTrack.number = int(value)
	case id == mkvparse.TimecodeElement:
		d.clusterTime = value
	case d.inAudio && id == mkvparse.ChannelsElement:
		d.currentTrack.channels = int(value)
	}
	return nil
}

// HandleFloat implements mkvparse.Handler for float-valued elements.
func (d *webmOpusDecoder) HandleFloat(id mkvparse.ElementID, value float64, info mkvparse.ElementInfo) error {
	return nil
}

// HandleDate implements mkvparse.Handler for date-valued elements.
func (d *webmOpusDecoder) HandleDate(id mkvparse.ElementID, value time.Time, info mkvparse.ElementInfo) error {
	return nil
}

// HandleBinary implements mkvparse.Handler for binary-valued elements.
func (d *webmOpusDecoder) HandleBinary(id mkvparse.ElementID, value []byte, info mkvparse.ElementInfo) error {
	if d.inTrackEntry && id == mkvparse.CodecPrivateElement {
		d.currentTrack.private = append([]byte(nil), value...)
		return nil
	}
	if id != mkvparse.SimpleBlockElement && id != mkvparse.BlockElement {
		return nil
	}
	if d.decoder == nil || d.targetTrack.number == 0 {
		return nil
	}
	trackNumber, frames, err := parseMatroskaBlock(value)
	if err != nil {
		return err
	}
	if trackNumber != d.targetTrack.number {
		return nil
	}
	for _, frame := range frames {
		pcm, err := d.decodePacket(frame)
		if err != nil {
			return err
		}
		if d.emit != nil {
			if err := d.emit(pcm); err != nil {
				return err
			}
			continue
		}
		d.stereoPCM = append(d.stereoPCM, pcm...)
	}
	return nil
}

func (d *webmOpusDecoder) decodePacket(packet []byte) ([]int16, error) {
	pcmBuffer := make([]int16, opusFrameSize*d.targetTrack.channels)
	samplesPerChannel, err := d.decoder.Decode(packet, pcmBuffer, opusFrameSize, false)
	if err != nil {
		return nil, fmt.Errorf("decode opus packet: %w", err)
	}
	decoded := pcmBuffer[:samplesPerChannel*d.targetTrack.channels]
	if d.preSkipFrames > 0 {
		frames := min(d.preSkipFrames, samplesPerChannel)
		decoded = decoded[frames*d.targetTrack.channels:]
		d.preSkipFrames -= frames
	}
	return interleavedToStereo(decoded, d.targetTrack.channels), nil
}

func interleavedToStereo(samples []int16, channels int) []int16 {
	if channels <= 0 || len(samples) == 0 {
		return nil
	}
	frames := len(samples) / channels
	stereo := make([]int16, 0, frames*2)
	for frame := range frames {
		base := frame * channels
		left := samples[base]
		right := left
		if channels > 1 && base+1 < len(samples) {
			right = samples[base+1]
		}
		stereo = append(stereo, left, right)
	}
	return stereo
}

func parseOpusHead(data []byte, fallbackChannels int) (opusogg.OpusHead, error) {
	if len(data) < 19 || string(data[:8]) != "OpusHead" {
		return opusogg.OpusHead{}, errors.New("invalid opus codec private header")
	}
	head := opusogg.OpusHead{}
	head.Version = data[8]
	head.Channels = data[9]
	head.PreSkip = binary.LittleEndian.Uint16(data[10:12])
	head.InputSampleRate = binary.LittleEndian.Uint32(data[12:16])
	head.OutputGainQ8 = int16(binary.LittleEndian.Uint16(data[16:18]))
	head.ChannelMappingFamily = data[18]
	if head.Channels == 0 && fallbackChannels > 0 {
		head.Channels = uint8(fallbackChannels)
	}
	if head.ChannelMappingFamily != 0 {
		if len(data) < 21+int(head.Channels) {
			return opusogg.OpusHead{}, errors.New("invalid opus channel mapping")
		}
		head.StreamCount = data[19]
		head.CoupledStreamCount = data[20]
		head.ChannelMapping = append([]uint8(nil), data[21:21+int(head.Channels)]...)
	}
	return head, nil
}

// parseMatroskaBlock understands the different lacing modes Matroska can use to
// pack one or more Opus frames into a block.
func parseMatroskaBlock(data []byte) (int, [][]byte, error) {
	trackNumber, offset, err := readMatroskaVint(data)
	if err != nil {
		return 0, nil, err
	}
	if len(data) < offset+3 {
		return 0, nil, errors.New("matroska block too short")
	}
	flags := data[offset+2]
	payload := data[offset+3:]
	switch (flags >> 1) & 0x03 {
	case 0:
		return int(trackNumber), [][]byte{payload}, nil
	case 1:
		frames, err := parseXiphLacing(payload)
		return int(trackNumber), frames, err
	case 2:
		frames, err := parseFixedLacing(payload)
		return int(trackNumber), frames, err
	case 3:
		frames, err := parseEBMLLacing(payload)
		return int(trackNumber), frames, err
	default:
		return 0, nil, errors.New("unsupported matroska lacing")
	}
}

func readMatroskaVint(data []byte) (uint64, int, error) {
	if len(data) == 0 {
		return 0, 0, io.ErrUnexpectedEOF
	}
	mask := byte(0x80)
	length := 1
	for length <= 8 && data[0]&mask == 0 {
		mask >>= 1
		length++
	}
	if length > 8 || len(data) < length {
		return 0, 0, errors.New("invalid matroska vint")
	}
	value := uint64(data[0] & ^mask)
	for i := 1; i < length; i++ {
		value = (value << 8) | uint64(data[i])
	}
	return value, length, nil
}

func parseXiphLacing(payload []byte) ([][]byte, error) {
	if len(payload) == 0 {
		return nil, errors.New("xiph lacing missing frame count")
	}
	frameCount := int(payload[0]) + 1
	offset := 1
	sizes := make([]int, frameCount)
	remaining := len(payload) - offset
	for i := 0; i < frameCount-1; i++ {
		size := 0
		for {
			if offset >= len(payload) {
				return nil, io.ErrUnexpectedEOF
			}
			b := int(payload[offset])
			offset++
			size += b
			if b != 255 {
				break
			}
		}
		sizes[i] = size
		remaining -= size
	}
	remaining -= (offset - 1)
	if remaining < 0 {
		return nil, errors.New("invalid xiph lacing sizes")
	}
	sizes[frameCount-1] = remaining
	return sliceFrames(payload[offset:], sizes)
}

func parseFixedLacing(payload []byte) ([][]byte, error) {
	if len(payload) == 0 {
		return nil, errors.New("fixed lacing missing frame count")
	}
	frameCount := int(payload[0]) + 1
	data := payload[1:]
	if frameCount <= 0 || len(data)%frameCount != 0 {
		return nil, errors.New("invalid fixed lacing payload")
	}
	size := len(data) / frameCount
	sizes := make([]int, frameCount)
	for i := range sizes {
		sizes[i] = size
	}
	return sliceFrames(data, sizes)
}

func parseEBMLLacing(payload []byte) ([][]byte, error) {
	if len(payload) == 0 {
		return nil, errors.New("ebml lacing missing frame count")
	}
	frameCount := int(payload[0]) + 1
	offset := 1
	sizes := make([]int, frameCount)
	firstSize, n, err := readMatroskaVint(payload[offset:])
	if err != nil {
		return nil, err
	}
	sizes[0] = int(firstSize)
	offset += n
	total := sizes[0]
	prev := sizes[0]
	for i := 1; i < frameCount-1; i++ {
		deltaRaw, n, err := readMatroskaVint(payload[offset:])
		if err != nil {
			return nil, err
		}
		bits := 7 * n
		bias := (1 << (bits - 1)) - 1
		delta := int(deltaRaw) - bias
		sizes[i] = prev + delta
		if sizes[i] < 0 {
			return nil, errors.New("invalid ebml lacing size")
		}
		prev = sizes[i]
		total += sizes[i]
		offset += n
	}
	remaining := len(payload[offset:]) - total
	if remaining < 0 {
		return nil, errors.New("invalid ebml lacing payload")
	}
	sizes[frameCount-1] = remaining
	return sliceFrames(payload[offset:], sizes)
}

func sliceFrames(data []byte, sizes []int) ([][]byte, error) {
	frames := make([][]byte, 0, len(sizes))
	offset := 0
	for _, size := range sizes {
		if size < 0 || offset+size > len(data) {
			return nil, errors.New("invalid laced frame size")
		}
		frames = append(frames, append([]byte(nil), data[offset:offset+size]...))
		offset += size
	}
	return frames, nil
}

// Stream copies buffered PCM samples into the caller's output slice.
func (s *pcmBufferStreamer) Stream(samples [][2]float64) (int, bool) {
	if s.closed {
		return 0, false
	}
	frames := s.Len() - s.pos
	if frames <= 0 {
		return 0, false
	}
	if frames > len(samples) {
		frames = len(samples)
	}
	for i := 0; i < frames; i++ {
		base := (s.pos + i) * 2
		samples[i][0] = float64(s.samples[base]) / 32768
		samples[i][1] = float64(s.samples[base+1]) / 32768
	}
	s.pos += frames
	return frames, true
}

// Err reports the buffered PCM streamer error state.
func (s *pcmBufferStreamer) Err() error { return nil }

// Len reports the total number of stereo samples held in the PCM buffer.
func (s *pcmBufferStreamer) Len() int { return len(s.samples) / 2 }

// Position reports the current sample offset within the PCM buffer.
func (s *pcmBufferStreamer) Position() int { return s.pos }

// Seek moves the PCM buffer streamer to the requested sample offset.
func (s *pcmBufferStreamer) Seek(p int) error {
	if p < 0 {
		p = 0
	}
	if p > s.Len() {
		p = s.Len()
	}
	s.pos = p
	return nil
}

// Close marks the PCM buffer streamer as closed.
func (s *pcmBufferStreamer) Close() error { s.closed = true; return nil }
