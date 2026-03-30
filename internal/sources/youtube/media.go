package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gopxl/beep"
)

// ytDLPStreamInfo captures the subset of `yt-dlp -j` output that playback
// needs: the final media URL plus any request headers required to access it.
type ytDLPStreamInfo struct {
	URL         string            `json:"url"`
	HTTPHeaders map[string]string `json:"http_headers"`
}

// openYTDLPMedia turns a YouTube URL into a seekable beep stream.
//
// The important detail is that yt-dlp is used only as an extractor here. Once
// we have the final media URL and headers, Musicon performs its own ranged HTTP
// reads and decode pipeline.
func (s *Source) openYTDLPMedia(ctx context.Context, rawURL string, duration time.Duration) (beep.StreamSeekCloser, beep.Format, error) {
	info, err := s.extractYTDLPStreamInfo(ctx, rawURL)
	if err != nil {
		return nil, beep.Format{}, err
	}
	return s.openResolvedMedia(info, duration)
}

func (s *Source) openResolvedMedia(info ytDLPStreamInfo, duration time.Duration) (beep.StreamSeekCloser, beep.Format, error) {
	streamCtx, streamCancel := context.WithCancel(context.Background())
	reader, err := newRangeReadSeeker(streamCtx, s.httpClient, info.URL, headerFromMap(info.HTTPHeaders), mediaRequestBlockSize)
	if err != nil {
		streamCancel()
		return nil, beep.Format{}, err
	}
	return openPreparedMediaStream(streamCtx, streamCancel, reader, duration, 0, func(ctx context.Context, media io.ReadSeeker, duration time.Duration, startSample int) (beep.StreamSeekCloser, beep.Format, error) {
		return newSeekableWebMOpusStream(ctx, media, duration, startSample)
	})
}

type seekableStreamFactory func(ctx context.Context, media io.ReadSeeker, duration time.Duration, startSample int) (beep.StreamSeekCloser, beep.Format, error)

func openPreparedMediaStream(streamCtx context.Context, streamCancel context.CancelFunc, reader *rangeReadSeeker, duration time.Duration, startSample int, factory seekableStreamFactory) (beep.StreamSeekCloser, beep.Format, error) {
	stream, format, err := factory(streamCtx, reader, duration, startSample)
	if err != nil {
		streamCancel()
		_ = reader.Close()
		return nil, beep.Format{}, err
	}
	return &mediaCloser{
		stream: stream,
		closer: func() error {
			streamCancel()
			return reader.Close()
		},
		prepareReplacement: func(target int) (beep.StreamSeekCloser, error) {
			replacementCtx, replacementCancel := context.WithCancel(context.Background())
			clone := reader.Clone(replacementCtx)
			offset := estimatedMediaOffset(target, format.SampleRate.N(duration), clone.KnownSize())
			if offset > 0 {
				if err := clone.Prime(offset); err != nil {
					replacementCancel()
					_ = clone.Close()
					return nil, err
				}
			}
			replacement, _, err := openPreparedMediaStream(replacementCtx, replacementCancel, clone, duration, target, factory)
			if err != nil {
				return nil, err
			}
			return replacement, nil
		},
	}, format, nil
}

// openYTDLPStream is the older "stream bytes from yt-dlp stdout" path. It is
// still useful in tests and as a fallback building block, even though the
// default playback path now prefers direct ranged reads.
func (s *Source) openYTDLPStream(ctx context.Context, rawURL string) (io.ReadCloser, func() error, error) {
	binaryPath, err := exec.LookPath("yt-dlp")
	if err != nil {
		return nil, nil, err
	}
	args := s.ytDLPArgs(rawURL)
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	wait := func() error {
		if err := cmd.Wait(); err != nil {
			message := strings.TrimSpace(stderr.String())
			if message != "" {
				return fmt.Errorf("%w: %s", err, message)
			}
			return err
		}
		return nil
	}
	return stdout, wait, nil
}

// ytDLPArgs builds the subprocess arguments for a playback-oriented yt-dlp
// invocation.
func (s *Source) ytDLPArgs(rawURL string) []string {
	args := []string{
		"--quiet",
		"--no-warnings",
		"--no-progress",
		"--no-playlist",
		"-f", "ba[ext=webm]/ba",
		"-o", "-",
	}
	if s != nil {
		if s.cookiesFile != "" {
			args = append(args, "--cookies", s.cookiesFile)
		}
		if s.cookiesBrowser != "" {
			args = append(args, "--cookies-from-browser", s.cookiesBrowser)
		}
		if s.cacheDir != "" {
			args = append(args, "--cache-dir", s.cacheDir)
		}
		if len(s.extraArgs) > 0 {
			args = append(args, s.extraArgs...)
		}
	}
	args = append(args, strings.TrimSpace(rawURL))
	return args
}

// extractYTDLPStreamInfo runs `yt-dlp -j` to ask yt-dlp for the final media URL
// and any required request headers without actually downloading the file.
func (s *Source) extractYTDLPStreamInfo(ctx context.Context, rawURL string) (ytDLPStreamInfo, error) {
	binaryPath, err := exec.LookPath("yt-dlp")
	if err != nil {
		return ytDLPStreamInfo{}, err
	}
	args := []string{"-j", "--no-warnings", "--no-playlist", "-f", "ba[ext=webm]/ba"}
	if s != nil {
		if s.cookiesFile != "" {
			args = append(args, "--cookies", s.cookiesFile)
		}
		if s.cookiesBrowser != "" {
			args = append(args, "--cookies-from-browser", s.cookiesBrowser)
		}
		if s.cacheDir != "" {
			args = append(args, "--cache-dir", s.cacheDir)
		}
		if len(s.extraArgs) > 0 {
			args = append(args, s.extraArgs...)
		}
	}
	args = append(args, strings.TrimSpace(rawURL))
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return ytDLPStreamInfo{}, fmt.Errorf("%w: %s", err, message)
		}
		return ytDLPStreamInfo{}, err
	}
	var info ytDLPStreamInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return ytDLPStreamInfo{}, fmt.Errorf("decode yt-dlp json: %w", err)
	}
	if strings.TrimSpace(info.URL) == "" {
		return ytDLPStreamInfo{}, errors.New("yt-dlp returned no media URL")
	}
	return info, nil
}

// headerFromMap converts the header map embedded in yt-dlp JSON into Go's
// canonical Header type.
func headerFromMap(values map[string]string) http.Header {
	headers := make(http.Header, len(values))
	for key, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			headers.Set(key, trimmed)
		}
	}
	return headers
}

// mediaCloser keeps the decoded stream and the underlying transport lifecycle
// tied together, so callers only have one thing to close.
type mediaCloser struct {
	stream             beep.StreamSeekCloser
	closer             func() error
	prepareReplacement func(target int) (beep.StreamSeekCloser, error)
}

func (m *mediaCloser) Stream(samples [][2]float64) (int, bool) { return m.stream.Stream(samples) }
func (m *mediaCloser) Err() error                              { return m.stream.Err() }
func (m *mediaCloser) Len() int                                { return m.stream.Len() }
func (m *mediaCloser) Position() int                           { return m.stream.Position() }
func (m *mediaCloser) Seek(p int) error                        { return m.stream.Seek(p) }
func (m *mediaCloser) PrepareReplacement(target int) (beep.StreamSeekCloser, error) {
	if m.prepareReplacement == nil {
		return nil, errors.New("replacement seek is not supported")
	}
	return m.prepareReplacement(target)
}
func (m *mediaCloser) Close() error {
	var streamErr error
	if m.stream != nil {
		streamErr = m.stream.Close()
	}
	if m.closer != nil {
		if closeErr := m.closer(); closeErr != nil && streamErr == nil {
			streamErr = closeErr
		}
	}
	return streamErr
}

func estimatedMediaOffset(target, totalFrames int, size int64) int64 {
	if target <= 0 || totalFrames <= 0 || size <= 0 {
		return 0
	}
	if target >= totalFrames {
		target = totalFrames
	}
	return (int64(target) * size) / int64(totalFrames)
}

// rangeReadSeeker is the core transport primitive that makes direct HTTP range
// reads look enough like an `io.ReadSeeker` for the WebM parser.
//
// Each fetched block is written to a per-stream temp directory keyed by aligned
// byte range. That means revisiting an older block can be served locally
// without evicting every other previously downloaded block or issuing another
// range request.
type rangeReadSeeker struct {
	ctx       context.Context
	client    *http.Client
	url       string
	headers   http.Header
	blockSize int64
	cache     *rangeReadCache

	mu         sync.Mutex
	pos        int64
	size       int64
	block      []byte
	blockStart int64
	blockLen   int
	closed     bool
}

type rangeReadCache struct {
	mu        sync.Mutex
	cacheDir  string
	blockLens map[int64]int
	refs      int
}

// newRangeReadSeeker initializes the fixed-size range reader used by the WebM
// parser.
func newRangeReadSeeker(ctx context.Context, client *http.Client, rawURL string, headers http.Header, blockSize int64) (*rangeReadSeeker, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if blockSize <= 0 {
		blockSize = mediaRequestBlockSize
	}
	cacheDir, err := os.MkdirTemp("", "musicon-youtube-range-*")
	if err != nil {
		return nil, fmt.Errorf("create media block cache: %w", err)
	}
	cache := &rangeReadCache{
		cacheDir:  cacheDir,
		blockLens: make(map[int64]int),
		refs:      1,
	}
	return &rangeReadSeeker{
		ctx:        ctx,
		client:     client,
		url:        rawURL,
		headers:    headers.Clone(),
		blockSize:  blockSize,
		cache:      cache,
		block:      make([]byte, blockSize),
		blockStart: -1,
	}, nil
}

func (r *rangeReadSeeker) Clone(ctx context.Context) *rangeReadSeeker {
	if ctx == nil {
		ctx = context.Background()
	}
	r.cache.mu.Lock()
	r.cache.refs++
	r.cache.mu.Unlock()
	return &rangeReadSeeker{
		ctx:        ctx,
		client:     r.client,
		url:        r.url,
		headers:    r.headers.Clone(),
		blockSize:  r.blockSize,
		cache:      r.cache,
		size:       r.size,
		block:      make([]byte, r.blockSize),
		blockStart: -1,
	}
}

// Read serves bytes out of the cached block and refills it on demand when the
// read cursor moves outside the current block.
func (r *rangeReadSeeker) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, os.ErrClosed
	}
	read := 0
	for len(p) > 0 {
		if r.size > 0 && r.pos >= r.size {
			if read > 0 {
				return read, nil
			}
			return 0, io.EOF
		}
		if !r.hasCurrentBlockLocked() {
			if err := r.fetchBlockLocked(r.pos); err != nil {
				if read > 0 && errors.Is(err, io.EOF) {
					return read, nil
				}
				return read, err
			}
		}
		offset := int(r.pos - r.blockStart)
		if offset < 0 || offset >= r.blockLen {
			return read, io.EOF
		}
		copied := copy(p, r.block[offset:r.blockLen])
		read += copied
		r.pos += int64(copied)
		p = p[copied:]
		if copied == 0 {
			break
		}
	}
	if read == 0 {
		return 0, io.EOF
	}
	return read, nil
}

// Seek only moves the logical cursor. The next Read decides whether a new block
// fetch is required.
func (r *rangeReadSeeker) Seek(offset int64, whence int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = r.pos + offset
	case io.SeekEnd:
		if r.size <= 0 {
			return 0, errors.New("seek from end unavailable without known media size")
		}
		next = r.size + offset
	default:
		return 0, errors.New("invalid seek whence")
	}
	if next < 0 {
		next = 0
	}
	r.pos = next
	return r.pos, nil
}

func (r *rangeReadSeeker) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return r.cache.release()
}

func (r *rangeReadSeeker) hasCurrentBlockLocked() bool {
	return r.blockStart >= 0 && r.pos >= r.blockStart && r.pos < r.blockStart+int64(r.blockLen)
}

// fetchBlockLocked requests a single byte range from the remote media URL and
// refreshes the local cache with that block.
func (r *rangeReadSeeker) fetchBlockLocked(offset int64) error {
	if r.size > 0 && offset >= r.size {
		return io.EOF
	}
	start := offset - (offset % r.blockSize)
	if cached, err := r.loadCachedBlockLocked(start); err != nil {
		return err
	} else if cached {
		return nil
	}
	end := start + int64(len(r.block)) - 1
	if r.size > 0 && end >= r.size {
		end = r.size - 1
	}
	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return err
	}
	for key, values := range r.headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("fetch ranged media block: %s", strings.TrimSpace(firstNonEmpty(string(body), resp.Status)))
	}
	n, readErr := io.ReadFull(resp.Body, r.block)
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return readErr
	}
	r.blockStart = start
	r.blockLen = n
	r.cache.storeBlockLen(start, n)
	if size, ok := mediaSizeFromResponse(resp, start, int64(n)); ok {
		r.size = size
	}
	if n == 0 {
		return io.EOF
	}
	if err := r.writeCachedBlockLocked(start, r.block[:n]); err != nil {
		return err
	}
	return nil
}

func (r *rangeReadSeeker) loadCachedBlockLocked(start int64) (bool, error) {
	cacheDir := r.cache.dir()
	if cacheDir == "" {
		return false, nil
	}
	data, err := os.ReadFile(r.cachePath(start))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read cached media block: %w", err)
	}
	r.blockStart = start
	r.blockLen = copy(r.block, data)
	r.cache.storeBlockLen(start, r.blockLen)
	if r.blockLen < len(r.block) && r.size < start+int64(r.blockLen) {
		r.size = start + int64(r.blockLen)
	}
	return true, nil
}

func (r *rangeReadSeeker) writeCachedBlockLocked(start int64, data []byte) error {
	if r.cache.dir() == "" {
		return nil
	}
	if err := os.WriteFile(r.cachePath(start), data, 0o600); err != nil {
		return fmt.Errorf("write cached media block: %w", err)
	}
	return nil
}

func (r *rangeReadSeeker) cachePath(start int64) string {
	return filepath.Join(r.cache.dir(), fmt.Sprintf("%020d.block", start))
}

func (r *rangeReadSeeker) Prime(offset int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return os.ErrClosed
	}
	current := r.pos
	err := r.fetchBlockLocked(offset)
	r.pos = current
	return err
}

func (r *rangeReadSeeker) KnownSize() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.size
}

func (c *rangeReadCache) dir() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cacheDir
}

func (c *rangeReadCache) storeBlockLen(start int64, length int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.blockLens != nil {
		c.blockLens[start] = length
	}
}

func (c *rangeReadCache) release() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.refs > 0 {
		c.refs--
	}
	if c.refs != 0 || c.cacheDir == "" {
		return nil
	}
	err := os.RemoveAll(c.cacheDir)
	c.cacheDir = ""
	c.blockLens = nil
	return err
}

// mediaSizeFromResponse infers total media size from the best available HTTP
// response metadata.
func mediaSizeFromResponse(resp *http.Response, start, bytesRead int64) (int64, bool) {
	if contentRange := strings.TrimSpace(resp.Header.Get("Content-Range")); contentRange != "" {
		parts := strings.Split(contentRange, "/")
		if len(parts) == 2 {
			if size, err := parseInt64(strings.TrimSpace(parts[1])); err == nil {
				return size, true
			}
		}
	}
	if resp.ContentLength > 0 {
		if resp.StatusCode == http.StatusPartialContent {
			return start + resp.ContentLength, true
		}
		return resp.ContentLength, true
	}
	if bytesRead > 0 {
		return start + bytesRead, false
	}
	return 0, false
}

func parseInt64(raw string) (int64, error) {
	var value int64
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid integer %q", raw)
		}
		value = value*10 + int64(ch-'0')
	}
	return value, nil
}

type managedReadCloser struct {
	io.ReadCloser
	once    sync.Once
	onClose func()
}

func (m *managedReadCloser) Close() error {
	var err error
	m.once.Do(func() {
		if m.onClose != nil {
			m.onClose()
		}
		if m.ReadCloser != nil {
			err = m.ReadCloser.Close()
		}
	})
	return err
}
