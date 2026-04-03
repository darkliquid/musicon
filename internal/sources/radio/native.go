package radio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/asticode/go-astits"
	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	hlsplaylist "github.com/bluenviron/gohlslib/v2/pkg/playlist"
	mpeg4audio "github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/gopxl/beep"
	opusdecoder "github.com/kazzmir/opus-go/opus"
	goadts "github.com/skrashevich/go-aac/pkg/adts"
	aacdecoder "github.com/skrashevich/go-aac/pkg/decoder"
)

// This file holds the native live-stream playback paths used by the radio
// source when plain MP3/Ogg/WAV decoding is not enough. In practice that mostly
// means HLS and AAC variants, plus the buffering needed to make live audio fit
// beep's streaming interfaces.

const (
	initialBufferDuration = 250 * time.Millisecond
	initialPCMBufferBytes = 1 << 20
	livePCMBufferBytes    = 4 << 20
	opusFrameSize         = 5760
)

func (s *Source) openNativeStream(ctx context.Context, streamURL string) (beep.StreamSeekCloser, beep.Format, error) {
	if looksLikeHLS("", streamURL, nil) {
		return s.openHLSStream(ctx, streamURL)
	}

	resp, err := s.openStreamRequest(ctx, streamURL)
	if err != nil {
		return nil, beep.Format{}, err
	}

	reader := bufio.NewReader(resp.Body)
	preview, _ := reader.Peek(32)
	resolvedURL := streamURL
	if resp.Request != nil && resp.Request.URL != nil {
		resolvedURL = resp.Request.URL.String()
	}

	switch {
	case looksLikeHLS(resp.Header.Get("Content-Type"), resolvedURL, preview):
		_ = resp.Body.Close()
		return s.openHLSStream(ctx, resolvedURL)

	case looksLikeADTSAAC(resp.Header.Get("Content-Type"), resolvedURL, preview):
		return openADTSAACStream(ctx, reader, func() error { return resp.Body.Close() })

	default:
		_ = resp.Body.Close()
		return nil, beep.Format{}, fmt.Errorf("%w: native playback currently supports HLS, ADTS AAC, MP3, Ogg/Vorbis, and WAV", errUnsupportedCodec)
	}
}

func (s *Source) openHLSStream(ctx context.Context, streamURL string) (beep.StreamSeekCloser, beep.Format, error) {
	if stream, format, err := s.openHLSTSAACStream(ctx, streamURL); err == nil {
		return stream, format, nil
	} else if err != nil && !errors.Is(err, errUnsupportedCodec) {
		s.debugf("radio: pure-go TS/AAC HLS path unavailable for %s: %v", streamURL, err)
	}

	type readyResult struct {
		stream beep.StreamSeekCloser
		format beep.Format
		err    error
	}

	client := &gohlslib.Client{
		URI:        streamURL,
		HTTPClient: s.httpClient,
		OnRequest: func(req *http.Request) {
			req.Header.Set("User-Agent", s.userAgent)
		},
		OnDownloadPrimaryPlaylist: func(u string) {
			s.debugf("radio: downloading primary playlist %s", u)
		},
		OnDownloadStreamPlaylist: func(u string) {
			s.debugf("radio: downloading stream playlist %s", u)
		},
		OnDownloadSegment: func(u string) {
			s.debugf("radio: downloading segment %s", u)
		},
		OnDownloadPart: func(u string) {
			s.debugf("radio: downloading part %s", u)
		},
		OnDecodeError: func(err error) {
			s.debugf("radio: hls decode error: %v", err)
		},
	}

	readyCh := make(chan readyResult, 1)
	var readyOnce sync.Once
	var streamer *bufferedLivePCMStreamer

	sendReady := func(result readyResult) {
		readyOnce.Do(func() {
			readyCh <- result
		})
	}

	client.OnTracks = func(tracks []*gohlslib.Track) error {
		stream, format, err := newHLSNativeStream(client, tracks)
		if err != nil {
			sendReady(readyResult{err: err})
			return err
		}
		native, ok := stream.(*bufferedLivePCMStreamer)
		if !ok {
			err = errors.New("internal radio native stream type mismatch")
			sendReady(readyResult{err: err})
			return err
		}
		streamer = native
		sendReady(readyResult{stream: stream, format: format})
		return nil
	}

	if err := client.Start(); err != nil {
		return nil, beep.Format{}, fmt.Errorf("start HLS client: %w", err)
	}

	go func() {
		err := normalizeClientCloseErr(client.Wait2())
		if streamer != nil {
			streamer.finish(err)
			return
		}
		if err != nil {
			sendReady(readyResult{err: err})
		}
	}()

	select {
	case result := <-readyCh:
		if result.err != nil {
			client.Close()
			return nil, beep.Format{}, result.err
		}
		buffered, _ := result.stream.(*bufferedLivePCMStreamer)
		if err := buffered.waitForBuffer(ctx, result.format.SampleRate.N(initialBufferDuration)); err != nil {
			_ = buffered.Close()
			return nil, beep.Format{}, err
		}
		return result.stream, result.format, nil

	case <-ctx.Done():
		client.Close()
		return nil, beep.Format{}, ctx.Err()
	}
}

type hlsTSAACState struct {
	dec         *aacdecoder.Decoder
	selectedPID uint16
	carry       map[uint16][]byte
}

func (s *Source) openHLSTSAACStream(ctx context.Context, streamURL string) (beep.StreamSeekCloser, beep.Format, error) {
	playlistURL, media, err := s.fetchHLSMediaPlaylist(ctx, streamURL)
	if err != nil {
		return nil, beep.Format{}, err
	}
	if len(media.Segments) == 0 {
		return nil, beep.Format{}, fmt.Errorf("hls playlist contains no segments")
	}
	lastSegment := media.Segments[len(media.Segments)-1]
	if lastSegment == nil || strings.TrimSpace(lastSegment.URI) == "" || path.Ext(lastSegment.URI) != ".ts" {
		return nil, beep.Format{}, errUnsupportedCodec
	}

	state := &hlsTSAACState{
		dec:   aacdecoder.New(),
		carry: make(map[uint16][]byte),
	}
	lifetimeCtx, cancel := context.WithCancel(ctx)
	streamer := newBufferedLivePCMStreamer(func() error {
		cancel()
		return nil
	})
	seen := make(map[int]struct{})

	start := max(len(media.Segments)-2, 0)
	if err := s.processHLSTSAACSegments(lifetimeCtx, playlistURL, media, start, seen, state, streamer); err != nil {
		cancel()
		return nil, beep.Format{}, err
	}

	sampleRate := state.dec.Config.SampleRate
	if sampleRate <= 0 {
		cancel()
		return nil, beep.Format{}, fmt.Errorf("hls ts/aac stream did not expose a sample rate")
	}
	format := beep.Format{
		SampleRate:  beep.SampleRate(sampleRate),
		NumChannels: 2,
		Precision:   2,
	}

	go s.runHLSTSAACLoop(lifetimeCtx, playlistURL, media.TargetDuration, seen, state, streamer)

	if err := streamer.waitForBuffer(ctx, format.SampleRate.N(initialBufferDuration)); err != nil {
		_ = streamer.Close()
		return nil, beep.Format{}, err
	}
	return streamer, format, nil
}

func (s *Source) runHLSTSAACLoop(
	ctx context.Context,
	playlistURL *url.URL,
	targetDuration int,
	seen map[int]struct{},
	state *hlsTSAACState,
	streamer *bufferedLivePCMStreamer,
) {
	delay := time.Second
	if targetDuration > 0 {
		delay = max(time.Duration(targetDuration)*time.Second/2, time.Second)
	}
	ticker := time.NewTicker(delay)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			streamer.finish(nil)
			return
		default:
		}

		media, err := s.refreshHLSMediaPlaylist(ctx, playlistURL)
		if err != nil {
			streamer.finish(err)
			return
		}
		if err := s.processHLSTSAACSegments(ctx, playlistURL, media, 0, seen, state, streamer); err != nil {
			streamer.finish(err)
			return
		}

		select {
		case <-ctx.Done():
			streamer.finish(nil)
			return
		case <-ticker.C:
		}
	}
}

func (s *Source) fetchHLSMediaPlaylist(ctx context.Context, streamURL string) (*url.URL, *hlsplaylist.Media, error) {
	playlistURL, err := url.Parse(streamURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse hls playlist url: %w", err)
	}
	media, err := s.refreshHLSMediaPlaylist(ctx, playlistURL)
	if err != nil {
		return nil, nil, err
	}
	return playlistURL, media, nil
}

func (s *Source) refreshHLSMediaPlaylist(ctx context.Context, playlistURL *url.URL) (*hlsplaylist.Media, error) {
	s.debugf("radio: downloading primary playlist %s", playlistURL.String())
	resp, err := s.openStreamRequest(ctx, playlistURL.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read hls playlist: %w", err)
	}
	decoded, err := hlsplaylist.Unmarshal(body)
	if err != nil {
		return nil, fmt.Errorf("decode hls playlist: %w", err)
	}
	media, ok := decoded.(*hlsplaylist.Media)
	if !ok {
		return nil, errUnsupportedCodec
	}
	return media, nil
}

func (s *Source) processHLSTSAACSegments(
	ctx context.Context,
	playlistURL *url.URL,
	media *hlsplaylist.Media,
	start int,
	seen map[int]struct{},
	state *hlsTSAACState,
	streamer *bufferedLivePCMStreamer,
) error {
	if start < 0 {
		start = 0
	}
	for idx := start; idx < len(media.Segments); idx++ {
		segment := media.Segments[idx]
		if segment == nil || segment.Gap || strings.TrimSpace(segment.URI) == "" {
			continue
		}
		sequence := media.MediaSequence + idx
		if _, ok := seen[sequence]; ok {
			continue
		}
		segmentRef, err := url.Parse(segment.URI)
		if err != nil {
			return fmt.Errorf("parse hls segment url %q: %w", segment.URI, err)
		}
		segmentURL := playlistURL.ResolveReference(segmentRef)
		if err := s.processHLSTSAACSegment(ctx, segmentURL.String(), state, streamer); err != nil {
			return err
		}
		seen[sequence] = struct{}{}
	}
	return nil
}

func (s *Source) processHLSTSAACSegment(
	ctx context.Context,
	segmentURL string,
	state *hlsTSAACState,
	streamer *bufferedLivePCMStreamer,
) error {
	s.debugf("radio: downloading segment %s", segmentURL)
	resp, err := s.openStreamRequest(ctx, segmentURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read hls segment: %w", err)
	}
	if len(data) == 0 {
		return nil
	}

	dmx := astits.NewDemuxer(context.Background(), bytes.NewReader(data))
	buffers := make(map[uint16][]byte)
	decodedAny := false
	flush := func(pid uint16) error {
		pes := buffers[pid]
		buffers[pid] = nil
		if len(pes) == 0 {
			return nil
		}
		ok, err := s.decodeHLSTSAACPES(pid, pes, state, streamer)
		if err != nil {
			return err
		}
		if ok {
			decodedAny = true
		}
		return nil
	}

	for {
		packet, err := dmx.NextPacket()
		if err == astits.ErrNoMorePackets {
			break
		}
		if err != nil {
			return fmt.Errorf("decode hls ts packet: %w", err)
		}
		if packet == nil || !packet.Header.HasPayload || packet.Header.PID == astits.PIDNull {
			continue
		}
		pid := packet.Header.PID
		if packet.Header.PayloadUnitStartIndicator {
			if err := flush(pid); err != nil {
				return err
			}
		}
		buffers[pid] = append(buffers[pid], packet.Payload...)
	}
	for pid := range buffers {
		if err := flush(pid); err != nil {
			return err
		}
	}
	if !decodedAny && state.selectedPID == 0 {
		return errUnsupportedCodec
	}
	return nil
}

func (s *Source) decodeHLSTSAACPES(
	pid uint16,
	pes []byte,
	state *hlsTSAACState,
	streamer *bufferedLivePCMStreamer,
) (bool, error) {
	payload, err := parsePESPayload(pes)
	if err != nil {
		return false, nil
	}
	payload = append(state.carry[pid], payload...)
	frames, carry := extractADTSFrames(payload)
	state.carry[pid] = carry
	if len(frames) == 0 {
		return false, nil
	}
	if state.selectedPID == 0 {
		state.selectedPID = pid
	}
	if pid != state.selectedPID {
		return false, nil
	}

	decodedAny := false
	for _, frame := range frames {
		samples, err := state.dec.DecodeFrame(frame)
		if err != nil {
			if shouldIgnoreHLSAACDecodeErr(err) {
				continue
			}
			return decodedAny, fmt.Errorf("decode hls ts/aac frame: %w", err)
		}
		pcm, err := float32ToStereoPCM(samples, aacChannelCount(state.dec.Config.ChanConfig))
		if err != nil {
			return decodedAny, fmt.Errorf("convert hls ts/aac frame to pcm: %w", err)
		}
		streamer.append(pcm)
		decodedAny = true
	}
	return decodedAny, nil
}

func parsePESPayload(pes []byte) ([]byte, error) {
	if len(pes) < 6 || pes[0] != 0 || pes[1] != 0 || pes[2] != 1 {
		return nil, errors.New("invalid pes start code")
	}
	streamID := pes[3]
	packetLength := int(binary.BigEndian.Uint16(pes[4:6]))
	dataStart := 6
	if hasPESOptionalHeader(streamID) {
		if len(pes) < 9 {
			return nil, errors.New("short pes optional header")
		}
		dataStart = 9 + int(pes[8])
	}
	if dataStart > len(pes) {
		return nil, errors.New("pes header exceeds payload")
	}
	dataEnd := len(pes)
	if packetLength > 0 && 6+packetLength < dataEnd {
		dataEnd = 6 + packetLength
	}
	if dataEnd < dataStart {
		return nil, errors.New("invalid pes payload bounds")
	}
	return pes[dataStart:dataEnd], nil
}

func hasPESOptionalHeader(streamID byte) bool {
	return streamID != astits.StreamIDPaddingStream && streamID != astits.StreamIDPrivateStream2
}

func extractADTSFrames(data []byte) ([][]byte, []byte) {
	var frames [][]byte
	i := 0
	for i+7 <= len(data) {
		if !goadts.Probe(data[i:]) {
			i++
			continue
		}
		headerBytes := data[i:]
		if len(headerBytes) > 9 {
			headerBytes = headerBytes[:9]
		}
		header, err := goadts.ReadHeaderFromBytes(headerBytes)
		if err != nil || header.FrameLength < 7 {
			i++
			continue
		}
		if i+header.FrameLength > len(data) {
			break
		}
		frames = append(frames, append([]byte(nil), data[i:i+header.FrameLength]...))
		i += header.FrameLength
	}
	if i < len(data) {
		return frames, append([]byte(nil), data[i:]...)
	}
	return frames, nil
}

func newHLSNativeStream(client *gohlslib.Client, tracks []*gohlslib.Track) (beep.StreamSeekCloser, beep.Format, error) {
	var opusTrack *gohlslib.Track
	var opusCodec *codecs.Opus

	for _, track := range tracks {
		switch codec := track.Codec.(type) {
		case *codecs.MPEG4Audio:
			return newHLSAACStream(client, track, codec)
		case *codecs.Opus:
			if opusTrack == nil {
				opusTrack = track
				opusCodec = codec
			}
		}
	}

	if opusTrack != nil {
		return newHLSOpusStream(client, opusTrack, opusCodec)
	}

	return nil, beep.Format{}, errors.New("HLS station exposes no supported audio track")
}

func newHLSAACStream(client *gohlslib.Client, track *gohlslib.Track, codec *codecs.MPEG4Audio) (beep.StreamSeekCloser, beep.Format, error) {
	sampleRate := codec.Config.SampleRate
	if sampleRate <= 0 {
		sampleRate = track.ClockRate
	}
	if sampleRate <= 0 {
		sampleRate = 48_000
	}

	streamer := newBufferedLivePCMStreamer(func() error {
		client.Close()
		return nil
	})
	dec := aacdecoder.New()

	client.OnDataMPEG4Audio(track, func(_ int64, aus [][]byte) {
		for _, au := range aus {
			frame, err := marshalADTSAU(codec.Config, au)
			if err != nil {
				streamer.finish(fmt.Errorf("encode HLS AAC access unit: %w", err))
				client.Close()
				return
			}
			samples, err := dec.DecodeFrame(frame)
			if err != nil {
				if shouldIgnoreHLSAACDecodeErr(err) {
					continue
				}
				streamer.finish(fmt.Errorf("decode HLS AAC frame: %w", err))
				client.Close()
				return
			}
			channels := aacChannelCount(dec.Config.ChanConfig)
			pcm, err := float32ToStereoPCM(samples, channels)
			if err != nil {
				streamer.finish(fmt.Errorf("convert HLS AAC frame to PCM: %w", err))
				client.Close()
				return
			}
			streamer.append(pcm)
		}
	})

	return streamer, beep.Format{
		SampleRate:  beep.SampleRate(sampleRate),
		NumChannels: 2,
		Precision:   2,
	}, nil
}

func newHLSOpusStream(client *gohlslib.Client, track *gohlslib.Track, codec *codecs.Opus) (beep.StreamSeekCloser, beep.Format, error) {
	sampleRate := track.ClockRate
	if sampleRate <= 0 {
		sampleRate = 48_000
	}
	channels := codec.ChannelCount
	if channels <= 0 {
		channels = 2
	}

	dec, err := opusdecoder.NewDecoder(sampleRate, channels)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("create HLS Opus decoder: %w", err)
	}

	streamer := newBufferedLivePCMStreamer(func() error {
		client.Close()
		return nil
	})
	client.OnDataOpus(track, func(_ int64, packets [][]byte) {
		for _, packet := range packets {
			pcmBuffer := make([]int16, opusFrameSize*channels)
			samplesPerChannel, err := dec.Decode(packet, pcmBuffer, opusFrameSize, false)
			if err != nil {
				streamer.finish(fmt.Errorf("decode HLS Opus packet: %w", err))
				client.Close()
				return
			}
			pcm, err := int16ToStereoPCM(pcmBuffer[:samplesPerChannel*channels], channels)
			if err != nil {
				streamer.finish(fmt.Errorf("convert HLS Opus packet to PCM: %w", err))
				client.Close()
				return
			}
			streamer.append(pcm)
		}
	})

	return streamer, beep.Format{
		SampleRate:  beep.SampleRate(sampleRate),
		NumChannels: 2,
		Precision:   2,
	}, nil
}

func openADTSAACStream(ctx context.Context, reader *bufio.Reader, closeFn func() error) (beep.StreamSeekCloser, beep.Format, error) {
	dec := aacdecoder.New()

	firstFrame, err := readADTSFrame(reader)
	if err != nil {
		if closeFn != nil {
			_ = closeFn()
		}
		return nil, beep.Format{}, fmt.Errorf("read AAC frame: %w", err)
	}

	firstSamples, err := dec.DecodeFrame(firstFrame)
	if err != nil {
		if closeFn != nil {
			_ = closeFn()
		}
		return nil, beep.Format{}, fmt.Errorf("decode AAC frame: %w", err)
	}

	channels := aacChannelCount(dec.Config.ChanConfig)
	pcm, err := float32ToStereoPCM(firstSamples, channels)
	if err != nil {
		if closeFn != nil {
			_ = closeFn()
		}
		return nil, beep.Format{}, fmt.Errorf("convert AAC frame to PCM: %w", err)
	}

	sampleRate := dec.Config.SampleRate
	if sampleRate <= 0 {
		if closeFn != nil {
			_ = closeFn()
		}
		return nil, beep.Format{}, errors.New("AAC stream did not advertise a sample rate")
	}

	streamer := newBufferedLivePCMStreamer(closeFn)
	streamer.append(pcm)

	go func() {
		for {
			frame, err := readADTSFrame(reader)
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					streamer.finish(nil)
				} else {
					streamer.finish(fmt.Errorf("read AAC frame: %w", err))
				}
				return
			}

			samples, err := dec.DecodeFrame(frame)
			if err != nil {
				streamer.finish(fmt.Errorf("decode AAC frame: %w", err))
				return
			}

			pcm, err := float32ToStereoPCM(samples, aacChannelCount(dec.Config.ChanConfig))
			if err != nil {
				streamer.finish(fmt.Errorf("convert AAC frame to PCM: %w", err))
				return
			}
			streamer.append(pcm)
		}
	}()

	if err := streamer.waitForBuffer(ctx, beep.SampleRate(sampleRate).N(initialBufferDuration)); err != nil {
		_ = streamer.Close()
		return nil, beep.Format{}, err
	}

	return streamer, beep.Format{
		SampleRate:  beep.SampleRate(sampleRate),
		NumChannels: 2,
		Precision:   2,
	}, nil
}

func readADTSFrame(reader *bufio.Reader) ([]byte, error) {
	headerBytes, err := reader.Peek(9)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return nil, err
	}
	if len(headerBytes) < 7 {
		if err != nil {
			return nil, err
		}
		return nil, io.ErrUnexpectedEOF
	}

	header, err := goadts.ReadHeaderFromBytes(headerBytes)
	if err != nil {
		return nil, err
	}
	if header.FrameLength < 7 {
		return nil, fmt.Errorf("invalid AAC frame length %d", header.FrameLength)
	}

	frame := make([]byte, header.FrameLength)
	if _, err := io.ReadFull(reader, frame); err != nil {
		return nil, err
	}
	return frame, nil
}

func marshalADTSAU(config mpeg4audio.AudioSpecificConfig, au []byte) ([]byte, error) {
	return mpeg4audio.ADTSPackets{{
		Type:          config.Type,
		SampleRate:    config.SampleRate,
		ChannelConfig: config.ChannelConfig,
		ChannelCount:  config.ChannelCount,
		AU:            au,
	}}.Marshal()
}

func looksLikeHLS(contentType, rawURL string, preview []byte) bool {
	joined := strings.ToLower(strings.Join([]string{contentType, path.Ext(rawURL)}, " "))
	switch {
	case strings.Contains(joined, ".m3u8"),
		strings.Contains(joined, "mpegurl"),
		strings.Contains(joined, "application/x-mpegurl"),
		strings.Contains(joined, "application/vnd.apple.mpegurl"):
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(string(preview)), "#EXTM3U")
}

func looksLikeADTSAAC(contentType, rawURL string, preview []byte) bool {
	joined := strings.ToLower(strings.Join([]string{contentType, path.Ext(rawURL)}, " "))
	if strings.Contains(joined, "audio/aac") ||
		strings.Contains(joined, "audio/aacp") ||
		strings.Contains(joined, "audio/x-aac") ||
		strings.Contains(joined, ".aac") ||
		strings.Contains(joined, ".adts") {
		return true
	}
	return len(preview) >= 7 && goadts.Probe(preview)
}

func aacChannelCount(chanConfig int) int {
	switch chanConfig {
	case 1:
		return 1
	case 2:
		return 2
	case 3:
		return 3
	case 4:
		return 4
	case 5:
		return 5
	case 6:
		return 6
	case 7:
		return 8
	default:
		return 2
	}
}

func float32ToStereoPCM(samples []float32, channels int) ([]int16, error) {
	if channels <= 0 {
		return nil, errors.New("invalid channel count")
	}
	if len(samples)%channels != 0 {
		return nil, fmt.Errorf("AAC frame sample count %d is not divisible by %d channels", len(samples), channels)
	}

	frames := len(samples) / channels
	pcm := make([]int16, frames*2)
	for frame := range frames {
		base := frame * channels
		left := samples[base]
		right := left
		if channels > 1 {
			right = samples[base+1]
		}
		pcm[frame*2] = clampPCMFloat(left)
		pcm[frame*2+1] = clampPCMFloat(right)
	}
	return pcm, nil
}

func int16ToStereoPCM(samples []int16, channels int) ([]int16, error) {
	if channels <= 0 {
		return nil, errors.New("invalid channel count")
	}
	if len(samples)%channels != 0 {
		return nil, fmt.Errorf("Opus frame sample count %d is not divisible by %d channels", len(samples), channels)
	}

	frames := len(samples) / channels
	pcm := make([]int16, frames*2)
	for frame := range frames {
		base := frame * channels
		pcm[frame*2] = samples[base]
		pcm[frame*2+1] = samples[base]
		if channels > 1 {
			pcm[frame*2+1] = samples[base+1]
		}
	}
	return pcm, nil
}

func clampPCMFloat(sample float32) int16 {
	switch {
	case sample > 1:
		sample = 1
	case sample < -1:
		sample = -1
	}
	value := max(int(sample*32767), -32768)
	if value > 32767 {
		value = 32767
	}
	return int16(value)
}

func normalizeClientCloseErr(err error) error {
	if err == nil || strings.Contains(err.Error(), "terminated") {
		return nil
	}
	return err
}

type bufferedLivePCMStreamer struct {
	mu      sync.Mutex
	cond    *sync.Cond
	samples []int16
	pos     int
	closed  bool
	eof     bool
	err     error
	closeFn func() error
}

func newBufferedLivePCMStreamer(closeFn func() error) *bufferedLivePCMStreamer {
	streamer := &bufferedLivePCMStreamer{
		closeFn: closeFn,
		samples: make([]int16, 0, initialPCMBufferBytes/2),
	}
	streamer.cond = sync.NewCond(&streamer.mu)
	return streamer
}

func (s *bufferedLivePCMStreamer) append(samples []int16) {
	if len(samples) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.compactLocked()
	s.samples = append(s.samples, samples...)
	s.cond.Broadcast()
}

func (s *bufferedLivePCMStreamer) finish(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed && shouldIgnoreLiveStreamErr(err) {
		err = nil
	}
	if err != nil && !shouldIgnoreLiveStreamErr(err) && s.err == nil {
		s.err = err
	}
	s.eof = true
	s.cond.Broadcast()
}

func (s *bufferedLivePCMStreamer) waitForBuffer(ctx context.Context, frames int) error {
	if frames <= 0 {
		return nil
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		s.mu.Lock()
		decoded := len(s.samples)/2 - s.pos
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
			return errors.New("live radio stream decoded no audio")
		case closed:
			return errors.New("live radio stream closed")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *bufferedLivePCMStreamer) Stream(samples [][2]float64) (int, bool) {
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
			s.compactLocked()
			return frames, !s.eof || len(s.samples)/2-s.pos > 0
		}

		if s.eof {
			return 0, false
		}

		s.cond.Wait()
	}
}

func (s *bufferedLivePCMStreamer) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *bufferedLivePCMStreamer) Len() int { return 0 }

func (s *bufferedLivePCMStreamer) Position() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pos
}

func (s *bufferedLivePCMStreamer) Seek(int) error {
	return errors.New("radio streams do not support seeking")
}

func (s *bufferedLivePCMStreamer) Close() error {
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

func (s *bufferedLivePCMStreamer) compactLocked() {
	if s.pos <= 0 {
		return
	}

	dropFrames := 0
	maxFrames := livePCMBufferBytes / 4
	available := len(s.samples)/2 - s.pos

	if s.pos >= maxFrames/2 {
		dropFrames = s.pos
	}
	if available > maxFrames {
		dropFrames = s.pos
	}
	if dropFrames <= 0 {
		return
	}

	dropSamples := dropFrames * 2
	copy(s.samples, s.samples[dropSamples:])
	s.samples = s.samples[:len(s.samples)-dropSamples]
	s.pos -= dropFrames
	if s.pos < 0 {
		s.pos = 0
	}
}

func shouldIgnoreLiveStreamErr(err error) bool {
	return err == nil ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrClosedPipe)
}

func shouldIgnoreHLSAACDecodeErr(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "invalid syncword") ||
		strings.Contains(message, "invalid frame length")
}
