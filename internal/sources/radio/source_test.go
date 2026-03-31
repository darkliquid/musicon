package radio

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	teaui "github.com/darkliquid/musicon/internal/ui"
	"github.com/gopxl/beep"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type stubStream struct {
	length   int
	position int
}

func (s *stubStream) Stream(samples [][2]float64) (int, bool) { return 0, false }
func (s *stubStream) Err() error                              { return nil }
func (s *stubStream) Len() int                                { return s.length }
func (s *stubStream) Position() int                           { return s.position }
func (s *stubStream) Seek(p int) error                        { s.position = p; return nil }
func (s *stubStream) Close() error                            { return nil }

type observingStream struct {
	stubStream
	onClose func() error
}

func (s *observingStream) Close() error {
	if s.onClose != nil {
		return s.onClose()
	}
	return nil
}

func TestSearchReturnsHealthyStationsIncludingFallbackCandidates(t *testing.T) {
	source := NewSource(Options{Enabled: true, MaxResults: 4, BaseURL: "https://radio.test"})
	source.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/json/stations/byname/jazz":
			return jsonResponse(`[
				{"stationuuid":"uuid-1","name":" Jazz FM ","favicon":"https://img.test/jazz.png","countrycode":"US","language":"english","codec":"MP3","bitrate":128,"lastcheckok":1,"hls":0},
				{"stationuuid":"uuid-2","name":"AAC Station","codec":"AAC","bitrate":64,"lastcheckok":1,"hls":0},
				{"stationuuid":"uuid-3","name":"Broken Station","codec":"MP3","bitrate":128,"lastcheckok":0,"hls":0}
			]`), nil
		case "/json/stations/bytag/jazz":
			return jsonResponse(`[
				{"stationuuid":"uuid-1","name":"Jazz FM","codec":"MP3","bitrate":128,"lastcheckok":1,"hls":0},
				{"stationuuid":"uuid-4","name":"Vorbis Station","country":"Germany","language":"german","codec":"VORBIS","bitrate":192,"lastcheckok":1,"hls":0},
				{"stationuuid":"uuid-5","name":"HLS Station","codec":"MP3","bitrate":128,"lastcheckok":1,"hls":1}
			]`), nil
		default:
			t.Fatalf("unexpected path: %s", req.URL.Path)
			return nil, nil
		}
	})}

	results, err := source.Search(context.Background(), teaui.SearchRequest{
		SourceID: sourceID,
		Query:    "jazz",
		Filters:  teaui.DefaultSearchFilters(),
		Mode:     teaui.SearchModeStreams,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 healthy stations, got %#v", results)
	}
	if results[0].ID != "radio:uuid-1:mp3:direct" || results[0].Artwork.RemoteURL != "https://img.test/jazz.png" {
		t.Fatalf("unexpected first result: %#v", results[0])
	}
	if results[1].ID != "radio:uuid-2:aac:fallback" || !strings.Contains(results[1].Subtitle, "AAC 64k via native stream") {
		t.Fatalf("unexpected second result: %#v", results[1])
	}
	if results[2].ID != "radio:uuid-4:vorbis:direct" || !strings.Contains(results[2].Subtitle, "VORBIS 192k") {
		t.Fatalf("unexpected third result: %#v", results[2])
	}
	if results[3].ID != "radio:uuid-5:mp3:fallback" || !strings.Contains(results[3].Subtitle, "via native stream") {
		t.Fatalf("unexpected fourth result: %#v", results[3])
	}
}

func TestResolveUsesClickEndpointAndMP3Decoder(t *testing.T) {
	source := NewSource(Options{Enabled: true, BaseURL: "https://radio.test"})
	source.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/json/url/station-1":
			return jsonResponse(`{"ok":"true","name":"Jazz FM","stationuuid":"station-1","url":"https://stream.test/live.mp3"}`), nil
		case "/live.mp3":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"audio/mpeg"}},
				Body:       io.NopCloser(strings.NewReader("not-real-mp3")),
				Request:    req,
			}, nil
		default:
			t.Fatalf("unexpected path: %s", req.URL.Path)
			return nil, nil
		}
	})}
	source.decodeMP3 = func(body io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) {
		data, err := io.ReadAll(body)
		if err != nil {
			return nil, beep.Format{}, err
		}
		if string(data) != "not-real-mp3" {
			t.Fatalf("unexpected stream body: %q", string(data))
		}
		return &stubStream{length: 48_000}, beep.Format{SampleRate: 48_000, NumChannels: 2, Precision: 2}, nil
	}

	resolved, err := source.Resolve(teaui.QueueEntry{
		ID:       "radio:station-1:mp3:direct",
		Title:    "Queued Jazz FM",
		Subtitle: "US · english · MP3 128k",
		Source:   sourceName,
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.Info.Title != "Jazz FM" || resolved.Info.Artist != "US · english · MP3 128k" {
		t.Fatalf("unexpected resolved track info: %#v", resolved.Info)
	}
	if resolved.Stream.Len() != 0 {
		t.Fatalf("expected live stream len to be open-ended, got %d", resolved.Stream.Len())
	}
	if err := resolved.Stream.Seek(10); err == nil {
		t.Fatal("expected seek to be rejected for live stream")
	}
}

func TestResolveAcceptsBooleanClickOKField(t *testing.T) {
	source := NewSource(Options{Enabled: true, BaseURL: "https://radio.test"})
	source.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/json/url/station-1":
			return jsonResponse(`{"ok":true,"name":"Jazz FM","stationuuid":"station-1","url":"https://stream.test/live.mp3"}`), nil
		case "/live.mp3":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"audio/mpeg"}},
				Body:       io.NopCloser(strings.NewReader("not-real-mp3")),
				Request:    req,
			}, nil
		default:
			t.Fatalf("unexpected path: %s", req.URL.Path)
			return nil, nil
		}
	})}
	source.decodeMP3 = func(body io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) {
		_, _ = io.ReadAll(body)
		return &stubStream{length: 48_000}, beep.Format{SampleRate: 48_000, NumChannels: 2, Precision: 2}, nil
	}

	resolved, err := source.Resolve(teaui.QueueEntry{
		ID:       "radio:station-1:mp3:direct",
		Title:    "Queued Jazz FM",
		Subtitle: "US · english · MP3 128k",
		Source:   sourceName,
	})
	if err != nil {
		t.Fatalf("resolve failed with boolean ok field: %v", err)
	}
	if resolved.Info.Title != "Jazz FM" {
		t.Fatalf("unexpected resolved track info: %#v", resolved.Info)
	}
}

func TestResolveUsesNativeStreamForUnsupportedCodec(t *testing.T) {
	source := NewSource(Options{Enabled: true, BaseURL: "https://radio.test"})
	source.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/json/url/station-1" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		return jsonResponse(`{"ok":"true","name":"AAC Station","stationuuid":"station-1","url":"https://stream.test/live.aac"}`), nil
	})}
	source.openFallback = func(_ context.Context, rawURL string) (beep.StreamSeekCloser, beep.Format, error) {
		if rawURL != "https://stream.test/live.aac" {
			t.Fatalf("unexpected fallback url: %q", rawURL)
		}
		return &stubStream{length: 48_000}, beep.Format{SampleRate: 48_000, NumChannels: 2, Precision: 2}, nil
	}

	resolved, err := source.Resolve(teaui.QueueEntry{ID: "radio:station-1:aac:fallback", Title: "AAC Station", Source: sourceName})
	if err != nil {
		t.Fatalf("expected native stream resolve to succeed, got %v", err)
	}
	if resolved.Info.Title != "AAC Station" {
		t.Fatalf("unexpected resolved info: %#v", resolved.Info)
	}
}

func TestResolveKeepsNativeStreamContextAliveUntilClose(t *testing.T) {
	source := NewSource(Options{Enabled: true, BaseURL: "https://radio.test"})
	source.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/json/url/station-1" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		return jsonResponse(`{"ok":"true","name":"AAC Station","stationuuid":"station-1","url":"https://stream.test/live.aac"}`), nil
	})}

	ctxState := make(chan error, 2)
	source.openFallback = func(ctx context.Context, rawURL string) (beep.StreamSeekCloser, beep.Format, error) {
		if rawURL != "https://stream.test/live.aac" {
			t.Fatalf("unexpected fallback url: %q", rawURL)
		}
		ctxState <- ctx.Err()
		return &observingStream{
			stubStream: stubStream{length: 48_000},
			onClose: func() error {
				ctxState <- ctx.Err()
				return nil
			},
		}, beep.Format{SampleRate: 48_000, NumChannels: 2, Precision: 2}, nil
	}

	resolved, err := source.Resolve(teaui.QueueEntry{ID: "radio:station-1:aac:fallback", Title: "AAC Station", Source: sourceName})
	if err != nil {
		t.Fatalf("expected native stream resolve to succeed, got %v", err)
	}

	if err := <-ctxState; err != nil {
		t.Fatalf("expected open context to stay alive after resolve, got %v", err)
	}

	if err := resolved.Stream.Close(); err != nil {
		t.Fatalf("close stream: %v", err)
	}
	if err := <-ctxState; err != context.Canceled {
		t.Fatalf("expected open context to be canceled on close, got %v", err)
	}
}

func TestResolveUsesNativeStreamWhenDirectDecodeFails(t *testing.T) {
	source := NewSource(Options{Enabled: true, BaseURL: "https://radio.test"})
	source.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/json/url/station-1":
			return jsonResponse(`{"ok":"true","name":"HLS Station","stationuuid":"station-1","url":"https://stream.test/live.m3u8"}`), nil
		case "/live.m3u8":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/vnd.apple.mpegurl"}},
				Body:       io.NopCloser(strings.NewReader("#EXTM3U")),
				Request:    req,
			}, nil
		default:
			t.Fatalf("unexpected path: %s", req.URL.Path)
			return nil, nil
		}
	})}
	source.openFallback = func(_ context.Context, rawURL string) (beep.StreamSeekCloser, beep.Format, error) {
		if rawURL != "https://stream.test/live.m3u8" {
			t.Fatalf("unexpected fallback url: %q", rawURL)
		}
		return &stubStream{length: 96_000}, beep.Format{SampleRate: 48_000, NumChannels: 2, Precision: 2}, nil
	}

	resolved, err := source.Resolve(teaui.QueueEntry{ID: "radio:station-1:mp3:fallback", Title: "HLS Station", Source: sourceName})
	if err != nil {
		t.Fatalf("expected native stream resolve to succeed, got %v", err)
	}
	if resolved.Stream.Len() != 0 {
		t.Fatalf("expected live stream len to stay open-ended, got %d", resolved.Stream.Len())
	}
}

func TestSearchSkipsWhenFiltersDisableStreams(t *testing.T) {
	source := NewSource(Options{Enabled: true})
	results, err := source.Search(context.Background(), teaui.SearchRequest{
		SourceID: sourceID,
		Query:    "news",
		Filters:  teaui.SearchFilters{Tracks: true},
	})
	if err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results when stream filter is disabled, got %#v", results)
	}
}

func TestShouldIgnoreHLSAACDecodeErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "syncword", err: errors.New("adts: invalid syncword 0x0"), want: true},
		{name: "frame length", err: errors.New("ics: invalid frame length 0"), want: true},
		{name: "other", err: errors.New("unexpected EOF"), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldIgnoreHLSAACDecodeErr(tc.err); got != tc.want {
				t.Fatalf("shouldIgnoreHLSAACDecodeErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
