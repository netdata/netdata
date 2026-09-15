// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package credentialfile

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

const chunkMax = 32768

// Reader serializes requests to one helper. Close is terminal and interrupts
// active I/O. A stopped child must be reaped before another can be started.
type Reader struct {
	slot    chan struct{}
	closed  chan struct{}
	mu      sync.Mutex
	session *session
	command func() *exec.Cmd
}
type session struct {
	cmd           *exec.Cmd
	input, output *os.File
	done          chan struct{}
	stopped       bool // guarded by Reader.mu
	pathMax       uint32
}

// New constructs a reader without starting a process. Call Close when its owner ends.
func New() *Reader {
	return newReader(func() *exec.Cmd {
		return exec.Command(filepath.Join(buildinfo.NetdataBinDir, "nd-run"), "--file-reader")
	})
}
func newReader(command func() *exec.Cmd) *Reader {
	return &Reader{
		slot:    make(chan struct{}, 1),
		closed:  make(chan struct{}),
		command: command,
	}
}

func (r *Reader) stopLocked(s *session) {
	if s.stopped {
		return
	}
	s.stopped = true
	s.input.Close()
	s.output.Close()
	_ = s.cmd.Process.Kill()
}
func (r *Reader) retire(s *session) { r.mu.Lock(); defer r.mu.Unlock(); r.stopLocked(s) }

// Close stops active I/O without waiting for the request slot or an uninterruptible
// kernel operation. A dedicated goroutine remains responsible for reaping the child.
func (r *Reader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case <-r.closed:
		return nil
	default:
		close(r.closed)
	}
	if r.session != nil {
		r.stopLocked(r.session)
	}
	return nil
}

func (r *Reader) acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.closed:
		return transportError("closed")
	case r.slot <- struct{}{}:
	}
	if err := ctx.Err(); err != nil {
		<-r.slot
		return err
	}
	select {
	case <-r.closed:
		<-r.slot
		return transportError("closed")
	default:
		return nil
	}
}

// getSession is called only while holding the request slot.
func (r *Reader) getSession(ctx context.Context) (*session, bool, error) {
	for {
		r.mu.Lock()
		select {
		case <-r.closed:
			r.mu.Unlock()
			return nil, false, transportError("closed")
		default:
		}
		if s := r.session; s != nil {
			select {
			case <-s.done:
				r.stopLocked(s)
				r.session = nil
			default:
				if !s.stopped {
					r.mu.Unlock()
					return s, false, nil
				}
				r.mu.Unlock()
				select {
				case <-s.done:
					continue
				case <-ctx.Done():
					return nil, false, ctx.Err()
				case <-r.closed:
					return nil, false, transportError("closed")
				}
			}
		}
		inR, inW, err := os.Pipe()
		if err != nil {
			r.mu.Unlock()
			return nil, false, transportError("pipe failed")
		}
		outR, outW, err := os.Pipe()
		if err != nil {
			inR.Close()
			inW.Close()
			r.mu.Unlock()
			return nil, false, transportError("pipe failed")
		}
		cmd := r.command()
		cmd.Stdin = inR
		cmd.Stdout = outW
		cmd.Stderr = nil
		err = cmd.Start()
		inR.Close()
		outW.Close()
		if err != nil {
			inW.Close()
			outR.Close()
			r.mu.Unlock()
			return nil, false, startError(err)
		}
		s := &session{
			cmd:    cmd,
			input:  inW,
			output: outR,
			done:   make(chan struct{}),
		}
		r.session = s
		go func() { _ = cmd.Wait(); close(s.done) }()
		r.mu.Unlock()
		return s, true, nil
	}
}

type request struct {
	r        *Reader
	s        *session
	ctx      context.Context
	stop     func() bool
	canceled chan struct{}
	once     sync.Once
	path     string
	limit    uint64
}

func (q *request) finish() {
	q.once.Do(func() {
		if !q.stop() {
			<-q.canceled
		}
		<-q.r.slot
	})
}
func (q *request) failure() error {
	q.r.retire(q.s)
	if err := q.ctx.Err(); err != nil {
		return err
	}
	return transportError("protocol or I/O failed")
}
func (r *Reader) begin(ctx context.Context, path string, op, flags uint32, limit uint64) (*request, error) {
	if strings.IndexByte(path, 0) >= 0 {
		return nil, &fileError{"open", path, syscall.EINVAL}
	}
	if err := r.acquire(ctx); err != nil {
		return nil, err
	}
	s, fresh, err := r.getSession(ctx)
	if err != nil {
		<-r.slot
		return nil, err
	}
	q := &request{
		r:        r,
		s:        s,
		ctx:      ctx,
		path:     path,
		limit:    limit,
		canceled: make(chan struct{}),
	}
	q.stop = context.AfterFunc(ctx, func() { r.retire(s); close(q.canceled) })
	fail := func() (*request, error) { err := q.failure(); q.finish(); return nil, err }
	if fresh {
		var hello [16]byte
		if _, err = io.ReadFull(s.output, hello[:]); err != nil {
			return fail()
		}
		s.pathMax = binary.BigEndian.Uint32(hello[8:12])
		if string(hello[:8]) != "NDFILE01" || s.pathMax == 0 || binary.BigEndian.Uint32(hello[12:]) != chunkMax {
			return fail()
		}
	}
	if uint64(len(path)) >= uint64(s.pathMax) {
		q.finish()
		return nil, &fileError{"open", path, syscall.ENAMETOOLONG}
	}
	var header [24]byte
	binary.BigEndian.PutUint32(header[0:4], op)
	binary.BigEndian.PutUint32(header[4:8], flags)
	binary.BigEndian.PutUint64(header[8:16], limit)
	binary.BigEndian.PutUint32(header[16:20], uint32(len(path)))
	if _, err = s.input.Write(header[:]); err != nil {
		return fail()
	}
	if _, err = io.WriteString(s.input, path); err != nil {
		return fail()
	}
	return q, nil
}
func (q *request) frame() (kind uint32, payload []byte, err error) {
	var h [16]byte
	if _, e := io.ReadFull(q.s.output, h[:]); e != nil {
		return 0, nil, q.failure()
	}
	kind = binary.BigEndian.Uint32(h[:4])
	n := binary.BigEndian.Uint32(h[4:8])
	code := binary.BigEndian.Uint32(h[8:12])
	errno := binary.BigEndian.Uint32(h[12:])
	switch kind {
	case 1:
		if n == 0 || n > chunkMax || code != 0 || errno != 0 {
			return 0, nil, q.failure()
		}
	case 3:
		if n != 16 || code != 0 || errno != 0 {
			return 0, nil, q.failure()
		}
	case 2:
		if n != 0 || code > 6 || (code == 0 || code >= 5) && errno != 0 || code >= 1 && code <= 4 && errno == 0 {
			return 0, nil, q.failure()
		}
		if code == 0 {
			return kind, nil, nil
		}
		var cause error
		var op string
		switch code {
		case 1, 2, 3, 4:
			op = []string{"", "open", "stat", "read", "close"}[code]
			cause = syscall.Errno(errno)
		case 5:
			op = "read"
			cause = safefile.ErrNotRegular
		case 6:
			op = "read"
			cause = safefile.ErrTooLarge
		}
		return kind, nil, &fileError{op, q.path, cause}
	default:
		return 0, nil, q.failure()
	}
	payload = make([]byte, n)
	if _, e := io.ReadFull(q.s.output, payload); e != nil {
		return 0, nil, q.failure()
	}
	return kind, payload, nil
}

// Read requires a regular file of at most safefile.MaxSize bytes. It returns no
// partial contents when any file or transport operation fails.
func (r *Reader) Read(ctx context.Context, path string) ([]byte, error) {
	return r.read(ctx, path, 1, uint64(safefile.MaxSize))
}

// ReadAll retains unbounded, streaming file semantics for consumers that need them.
func (r *Reader) ReadAll(ctx context.Context, path string) ([]byte, error) {
	return r.read(ctx, path, 0, 0)
}
func (r *Reader) read(ctx context.Context, path string, flags uint32, limit uint64) ([]byte, error) {
	q, err := r.begin(ctx, path, 1, flags, limit)
	if err != nil {
		return nil, err
	}
	s := &stream{
		q: q,
	}
	defer s.Close()
	data, err := io.ReadAll(s)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Open returns a stream holding the reader's request slot until EOF or Close.
// Closing before EOF retires the session, allowing blocked reads to be interrupted.
func (r *Reader) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	q, err := r.begin(ctx, path, 1, 0, 0)
	if err != nil {
		return nil, err
	}
	return &stream{
		q: q,
	}, nil
}

// Stat returns the modification time observed by the unprivileged helper.
func (r *Reader) Stat(ctx context.Context, path string) (time.Time, error) {
	q, err := r.begin(ctx, path, 2, 0, 0)
	if err != nil {
		return time.Time{}, err
	}
	defer q.finish()
	kind, payload, err := q.frame()
	if err != nil {
		return time.Time{}, err
	}
	if kind != 3 {
		return time.Time{}, q.failure()
	}
	seconds := int64(binary.BigEndian.Uint64(payload[:8]))
	ns := int64(binary.BigEndian.Uint64(payload[8:]))
	if ns < 0 || ns >= 1e9 {
		return time.Time{}, q.failure()
	}
	return time.Unix(seconds, ns), nil
}

type stream struct {
	q       *request
	mu      sync.Mutex
	pending []byte
	done    bool
	ended   atomic.Bool
	total   uint64
}

func (s *stream) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	if len(s.pending) == 0 {
		kind, data, err := s.q.frame()
		if err != nil || kind == 2 {
			s.done = true
			s.ended.Store(true)
			s.q.finish()
			if err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
		if kind != 1 {
			s.done = true
			s.ended.Store(true)
			err = s.q.failure()
			s.q.finish()
			return 0, err
		}
		s.total += uint64(len(data))
		if s.q.limit != 0 && s.total > s.q.limit {
			s.done = true
			s.ended.Store(true)
			err = s.q.failure()
			s.q.finish()
			return 0, err
		}
		s.pending = data
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}
func (s *stream) Close() error {
	// Completion is published before releasing the slot, so closing an old stream
	// cannot retire a session already reused by the next request.
	s.q.r.mu.Lock()
	if !s.ended.Load() {
		s.q.r.stopLocked(s.q.s)
	}
	s.q.r.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.done {
		s.done = true
		s.ended.Store(true)
		s.q.finish()
	}
	return nil
}
