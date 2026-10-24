// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package streamexec

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/streamexec/streamexectest"
)

func TestMain(m *testing.M) {
	streamexectest.RunIfFake()
	os.Exit(m.Run())
}

// testTiming keeps records publishable and processes unstalled for the whole of a test that does not override it.
var testTiming = Timing{
	MaxSampleAge:    10 * time.Second,
	StallTimeout:    10 * time.Second,
	RestartDelayMin: 50 * time.Millisecond,
	RestartDelayMax: 200 * time.Millisecond,
}

// recordDecoder reports every line starting with "record " as a record of that line.
func recordDecoder() Decoder[string] {
	return DecoderFunc[string](func(line []byte) (string, bool) {
		s := string(line)
		return s, strings.HasPrefix(s, "record ")
	})
}

// newTestSource returns a source that starts the fake with the argument "--test".
func newTestSource(
	t *testing.T,
	fake *streamexectest.Fake,
	timing Timing,
	newDecoder func() Decoder[string],
) *Source[string] {
	t.Helper()
	s, err := New(Config[string]{
		Name:       "fake",
		Start:      startBinary(fake.Binary),
		NewDecoder: newDecoder,
		Timing:     timing,
	})
	require.NoError(t, err)
	return s
}

func startBinary(path string) StartFunc {
	return func(ctx context.Context, stdout *os.File) (*ndexec.Process, error) {
		return ndexec.StartUnprivilegedProcess(ctx, ndexec.ProcessOptions{
			Stdout: stdout,
		}, path, "--test")
	}
}

// startRun runs s until the test ends or the returned stop is called. It returns once Run reports a started instance.
// stop cancels Run and waits for it to return.
func startRun(t *testing.T, s *Source[string]) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	started, done := goRun(ctx, s)
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				assert.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Error("Run did not join its instance")
			}
		})
	}
	t.Cleanup(stop)
	select {
	case <-started:
	case err := <-done:
		once.Do(cancel)
		t.Fatalf("Run failed before starting: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not start")
	}
	return stop
}

func goRun(ctx context.Context, s *Source[string]) (started <-chan struct{}, done <-chan error) {
	startedCh := make(chan struct{})
	doneCh := make(chan error, 1)
	go func() { doneCh <- s.Run(ctx, func() { close(startedCh) }) }()
	return startedCh, doneCh
}

func waitLatest(t *testing.T, s *Source[string], want string) {
	t.Helper()
	require.Eventually(t, func() bool {
		got, ok := s.Latest()
		return ok && got == want
	}, 3*time.Second, 5*time.Millisecond)
}

func waitWithdrawn(t *testing.T, s *Source[string]) {
	t.Helper()
	require.Eventually(t, func() bool { return s.latest.Load() == nil }, 3*time.Second, 5*time.Millisecond)
}

func TestNew(t *testing.T) {
	valid := Config[string]{
		Name:       "fake",
		Start:      startBinary("/not-executed"),
		NewDecoder: recordDecoder,
		Timing:     testTiming,
	}
	for name, tc := range map[string]struct {
		edit    func(*Config[string])
		wantErr string
	}{
		"valid":            {edit: func(*Config[string]) {}},
		"empty name":       {edit: func(c *Config[string]) { c.Name = "" }, wantErr: "empty command name"},
		"nil start":        {edit: func(c *Config[string]) { c.Start = nil }, wantErr: "nil start function"},
		"nil decoder":      {edit: func(c *Config[string]) { c.NewDecoder = nil }, wantErr: "nil decoder constructor"},
		"zero sample age":  {edit: func(c *Config[string]) { c.Timing.MaxSampleAge = 0 }, wantErr: "must be positive"},
		"zero stall":       {edit: func(c *Config[string]) { c.Timing.StallTimeout = 0 }, wantErr: "must be positive"},
		"zero delay":       {edit: func(c *Config[string]) { c.Timing.RestartDelayMin = 0 }, wantErr: "must be positive"},
		"negative max":     {edit: func(c *Config[string]) { c.Timing.RestartDelayMax = -1 }, wantErr: "must be positive"},
		"min exceeds max":  {edit: func(c *Config[string]) { c.Timing.RestartDelayMin = time.Hour }, wantErr: "exceeds"},
		"min equal to max": {edit: func(c *Config[string]) { c.Timing.RestartDelayMin = c.Timing.RestartDelayMax }},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := valid
			tc.edit(&cfg)
			s, err := New(cfg)
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				assert.Nil(t, s)
				return
			}
			require.NoError(t, err)
			_, ok := s.Latest()
			assert.False(t, ok)
		})
	}
}

func TestRunPublishesFreshRecords(t *testing.T) {
	fake := streamexectest.NewFake(t)
	timing := testTiming
	timing.MaxSampleAge = 200 * time.Millisecond
	s := newTestSource(t, fake, timing, recordDecoder)
	startRun(t, s)
	conn := fake.Accept(t)
	assert.Equal(t, []string{"--test"}, conn.Args)
	_, ok := s.Latest()
	assert.False(t, ok, "a started instance has no record before its first one")

	conn.Line(t, "warning: not a record")
	conn.Line(t, "record 1")
	waitLatest(t, s, "record 1")
	conn.Line(t, "record 2")
	waitLatest(t, s, "record 2")

	// The instance keeps running, but its record ages out instead of being republished as current.
	require.Eventually(t, func() bool {
		_, ok := s.Latest()
		return !ok
	}, time.Second, 5*time.Millisecond)
	assert.NotNil(t, s.latest.Load(), "a stale record is not withdrawn while its instance runs")
}

func TestRunRecovers(t *testing.T) {
	for name, tc := range map[string]struct {
		fail func(*testing.T, *streamexectest.Conn)
	}{
		"nonzero exit": {fail: func(t *testing.T, conn *streamexectest.Conn) { conn.Exit(t, 7) }},
		"clean exit":   {fail: func(t *testing.T, conn *streamexectest.Conn) { conn.Exit(t, 0) }},
		"stall":        {fail: func(*testing.T, *streamexectest.Conn) {}},
		// Output that never completes a record does not reset the stall timer.
		"unrecognized output": {fail: func(t *testing.T, conn *streamexectest.Conn) { conn.Noise(t, "warning") }},
	} {
		t.Run(name, func(t *testing.T) {
			fake := streamexectest.NewFake(t)
			timing := testTiming
			// Long enough for a slow fake startup to deliver its first record before the stall timer fires.
			timing.StallTimeout = time.Second
			s := newTestSource(t, fake, timing, recordDecoder)
			startRun(t, s)
			conn := fake.Accept(t)
			conn.Line(t, "record before")
			waitLatest(t, s, "record before")

			tc.fail(t, conn)
			waitWithdrawn(t, s)
			replacement := fake.Accept(t)
			conn.RequireExited(t) // the failed instance exited before its replacement started

			replacement.Line(t, "record after")
			waitLatest(t, s, "record after")
		})
	}
}

func TestRunStartupFailure(t *testing.T) {
	fake := streamexectest.NewFake(t)
	s := newTestSource(t, fake, testTiming, recordDecoder)
	t.Cleanup(ndexec.SetRunnerPathsForTests(filepath.Join(t.TempDir(), "missing-wrapper"), ""))

	started, done := goRun(t.Context(), s)
	select {
	case err := <-done:
		require.ErrorContains(t, err, "start fake")
	case <-time.After(3 * time.Second):
		t.Fatal("startup failure not returned")
	}
	select {
	case <-started:
		t.Fatal("started without an instance")
	default:
	}
}

func TestRunCanceledBeforeStart(t *testing.T) {
	fake := streamexectest.NewFake(t)
	s := newTestSource(t, fake, testTiming, recordDecoder)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	started, done := goRun(ctx, s)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("canceled Run did not return")
	}
	select {
	case <-started:
		t.Fatal("started after cancellation")
	default:
	}
}

func TestRunSurvivesLateExecFailure(t *testing.T) {
	streamexectest.NewFake(t)
	s, err := New(Config[string]{
		Name:       "fake",
		Start:      startBinary(filepath.Join(t.TempDir(), "missing-command")),
		NewDecoder: recordDecoder,
		Timing:     testTiming,
	})
	require.NoError(t, err)

	// The helper starts, so Run reports a started instance before the missing command fails.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started, done := goRun(ctx, s)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("helper start not reported")
	}
	select {
	case err := <-done:
		t.Fatalf("late exec failure ended Run: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	_, ok := s.Latest()
	assert.False(t, ok)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("retry did not cancel")
	}
}

func TestCancelTerminatesDescendants(t *testing.T) {
	fake := streamexectest.NewFake(t)
	s := newTestSource(t, fake, testTiming, recordDecoder)
	stop := startRun(t, s)
	conn := fake.Accept(t)
	conn.Spawn(t)
	child := fake.Accept(t)
	require.True(t, child.Child)

	stop()
	conn.RequireExited(t) // the instance is gone once Run returns
	child.WaitExited(t)   // the descendant was terminated; Run does not wait for descendants
}

func TestFollowReportsFailure(t *testing.T) {
	// Exit cases wait longer than an exit can take, so only the stall cases stall. The stall cases publish a record
	// first, so its withdrawal is observable.
	for name, tc := range map[string]struct {
		fail         func(*testing.T, *streamexectest.Conn)
		stallTimeout time.Duration
		published    bool
		wantErr      string
	}{
		"nonzero exit": {
			fail:         func(t *testing.T, conn *streamexectest.Conn) { conn.Exit(t, 7) },
			stallTimeout: 10 * time.Second,
			wantErr:      "fake exited: exit status 7",
		},
		"clean exit": {
			fail:         func(t *testing.T, conn *streamexectest.Conn) { conn.Exit(t, 0) },
			stallTimeout: 10 * time.Second,
			wantErr:      "fake exited",
		},
		"stall": {
			fail:         func(*testing.T, *streamexectest.Conn) {},
			stallTimeout: 300 * time.Millisecond,
			published:    true,
			wantErr:      "fake stopped producing records",
		},
		"unrecognized output": {
			fail:         func(t *testing.T, conn *streamexectest.Conn) { conn.Noise(t, "warning") },
			stallTimeout: 300 * time.Millisecond,
			published:    true,
			wantErr:      "fake stopped producing records",
		},
	} {
		t.Run(name, func(t *testing.T) {
			fake := streamexectest.NewFake(t)
			timing := testTiming
			timing.StallTimeout = tc.stallTimeout
			s := newTestSource(t, fake, timing, recordDecoder)
			inst, err := s.start(t.Context())
			require.NoError(t, err)
			conn := fake.Accept(t)

			done := make(chan error, 1)
			go func() {
				_, err := s.follow(t.Context(), inst, observer{})
				done <- err
			}()
			if tc.published {
				conn.Line(t, "record before")
				require.Eventually(t, func() bool { return s.latest.Load() != nil }, time.Second, time.Millisecond)
			}
			tc.fail(t, conn)

			select {
			case err := <-done:
				assert.EqualError(t, err, tc.wantErr)
			case <-time.After(5 * time.Second):
				t.Fatal("follow did not end")
			}
			assert.Nil(t, s.latest.Load(), "follow withdraws the record when it ends")
		})
	}
}

func TestRestartBackoffGrowsForBriefOutput(t *testing.T) {
	fake := streamexectest.NewFake(t)
	s := newTestSource(t, fake, Timing{
		MaxSampleAge:    10 * time.Second,
		StallTimeout:    300 * time.Millisecond,
		RestartDelayMin: 50 * time.Millisecond,
		RestartDelayMax: 800 * time.Millisecond,
	}, recordDecoder)
	startRun(t, s)

	// Each instance prints one record and then stalls: it never runs healthy for a stall period, so every restart
	// waits twice as long as the last.
	var starts []time.Time
	for range 5 {
		conn := fake.Accept(t)
		starts = append(starts, time.Now())
		conn.Line(t, "record brief")
	}
	first := starts[1].Sub(starts[0])
	last := starts[4].Sub(starts[3])
	assert.GreaterOrEqual(t, last-first, 200*time.Millisecond, "restart intervals %v then %v", first, last)
}

func TestRestartBackoffResetsAfterHealthyInstance(t *testing.T) {
	const stallTimeout = 300 * time.Millisecond
	fake := streamexectest.NewFake(t)
	s := newTestSource(t, fake, Timing{
		MaxSampleAge:    10 * time.Second,
		StallTimeout:    stallTimeout,
		RestartDelayMin: 50 * time.Millisecond,
		RestartDelayMax: 3200 * time.Millisecond,
	}, recordDecoder)
	startRun(t, s)

	// Four brief instances grow the next delay to 800ms.
	for range 4 {
		fake.Accept(t).Line(t, "record brief")
	}
	// A healthy instance produces a record every 100ms for twice the stall timeout, then exits.
	healthy := fake.Accept(t)
	for i := range 6 {
		healthy.Line(t, "record healthy "+strconv.Itoa(i))
		time.Sleep(stallTimeout / 3)
	}
	exitAt := time.Now()
	healthy.Exit(t, 0)
	fake.Accept(t)
	assert.Less(t, time.Since(exitAt), 400*time.Millisecond, "a healthy instance restarted with accumulated backoff")
}

func TestReaderLines(t *testing.T) {
	fake := streamexectest.NewFake(t)
	var mu sync.Mutex
	var decoded []string
	s := newTestSource(t, fake, testTiming, func() Decoder[string] {
		return DecoderFunc[string](func(line []byte) (string, bool) {
			mu.Lock()
			defer mu.Unlock()
			decoded = append(decoded, string(line))
			return string(line), true
		})
	})
	startRun(t, s)
	conn := fake.Accept(t)

	conn.Raw(t, "crlf\r\n")
	conn.Raw(t, strings.Repeat("x", maxLineSize)+"\n")   // one byte past the limit with its terminator
	conn.Raw(t, strings.Repeat("z", 3*maxLineSize)+"\n") // spans several buffer fills
	conn.Raw(t, strings.Repeat("y", maxLineSize-1)+"\n") // the longest accepted line with its terminator
	conn.Line(t, "after")
	waitLatest(t, s, "after")
	conn.Raw(t, "unterminated")
	conn.Exit(t, 0)
	fake.Accept(t)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"crlf", strings.Repeat("y", maxLineSize-1), "after"}, decoded)
}

func TestDecoderPerInstance(t *testing.T) {
	fake := streamexectest.NewFake(t)
	// Each decoder numbers the lines it has seen, so a reused decoder would continue the count.
	s := newTestSource(t, fake, testTiming, func() Decoder[string] {
		var n int
		return DecoderFunc[string](func([]byte) (string, bool) {
			n++
			return strconv.Itoa(n), true
		})
	})
	startRun(t, s)

	conn := fake.Accept(t)
	conn.Line(t, "a")
	conn.Line(t, "b")
	waitLatest(t, s, "2")
	conn.Exit(t, 0)

	replacement := fake.Accept(t)
	replacement.Line(t, "c")
	waitLatest(t, s, "1")
}

func TestRunStartWithoutProcess(t *testing.T) {
	s, err := New(Config[string]{
		Name:       "fake",
		Start:      func(context.Context, *os.File) (*ndexec.Process, error) { return nil, nil },
		NewDecoder: recordDecoder,
		Timing:     testTiming,
	})
	require.NoError(t, err)

	started, done := goRun(t.Context(), s)
	select {
	case err := <-done:
		require.EqualError(t, err, "start fake: start function returned no process")
	case <-time.After(3 * time.Second):
		t.Fatal("missing process not reported")
	}
	select {
	case <-started:
		t.Fatal("started without a process")
	default:
	}
}

func TestRunAgainAfterReturn(t *testing.T) {
	fake := streamexectest.NewFake(t)
	s := newTestSource(t, fake, testTiming, recordDecoder)

	stop := startRun(t, s)
	first := fake.Accept(t)
	first.Line(t, "record first")
	waitLatest(t, s, "record first")
	stop()
	first.RequireExited(t)
	assert.Nil(t, s.latest.Load())

	startRun(t, s)
	second := fake.Accept(t)
	second.Line(t, "record second")
	waitLatest(t, s, "record second")
}

// goBackground calls Background in the background and returns its result channel.
func goBackground(ctx context.Context, s *Source[string]) <-chan backgroundResult {
	result := make(chan backgroundResult, 1)
	go func() {
		stop, err := s.Background(ctx)
		result <- backgroundResult{
			stop: stop,
			err:  err,
		}
	}()
	return result
}

type backgroundResult struct {
	stop func()
	err  error
}

func waitBackground(t *testing.T, result <-chan backgroundResult) backgroundResult {
	t.Helper()
	select {
	case r := <-result:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("Background did not return")
		return backgroundResult{}
	}
}

func TestBackgroundReturnsAfterFirstRecord(t *testing.T) {
	fake := streamexectest.NewFake(t)
	s := newTestSource(t, fake, testTiming, recordDecoder)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := goBackground(ctx, s)
	conn := fake.Accept(t)
	select {
	case r := <-result:
		t.Fatalf("Background returned before a record: %v", r.err)
	case <-time.After(100 * time.Millisecond):
	}

	conn.Line(t, "warning: not a record")
	conn.Line(t, "record 1")
	r := waitBackground(t, result)
	require.NoError(t, r.err)
	got, ok := s.Latest()
	assert.True(t, ok)
	assert.Equal(t, "record 1", got)

	// Supervision continues after Background returns, independently of its context.
	cancel()
	conn.Exit(t, 0)
	replacement := fake.Accept(t)
	conn.RequireExited(t)
	replacement.Line(t, "record 2")
	waitLatest(t, s, "record 2")

	r.stop()
	replacement.RequireExited(t) // the instance is gone once stop returns
	assert.Nil(t, s.latest.Load())
	r.stop() // idempotent
}

func TestBackgroundFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		prepare func(*testing.T)
		act     func(*testing.T, *streamexectest.Fake, context.CancelFunc)
		wantErr string
	}{
		"start failure": {
			prepare: func(t *testing.T) {
				t.Cleanup(ndexec.SetRunnerPathsForTests(filepath.Join(t.TempDir(), "missing-wrapper"), ""))
			},
			act:     func(*testing.T, *streamexectest.Fake, context.CancelFunc) {},
			wantErr: "start fake:",
		},
		"first instance exits before a record": {
			act: func(t *testing.T, fake *streamexectest.Fake, _ context.CancelFunc) {
				conn := fake.Accept(t)
				conn.Line(t, "warning: not a record")
				conn.Exit(t, 9)
			},
			wantErr: "fake exited: exit status 9",
		},
		"wait ends before a record": {
			act: func(t *testing.T, fake *streamexectest.Fake, cancel context.CancelFunc) {
				fake.Accept(t)
				cancel()
			},
			wantErr: "wait for the first fake record: context canceled",
		},
	} {
		t.Run(name, func(t *testing.T) {
			fake := streamexectest.NewFake(t)
			if tc.prepare != nil {
				tc.prepare(t)
			}
			s := newTestSource(t, fake, testTiming, recordDecoder)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			began := time.Now()
			result := goBackground(ctx, s)
			tc.act(t, fake, cancel)

			r := waitBackground(t, result)
			require.ErrorContains(t, r.err, tc.wantErr)
			assert.Nil(t, r.stop)
			assert.Less(t, time.Since(began), 3*time.Second, "a failure waits for nothing else")
			fake.RequireNoStart(t, 300*time.Millisecond) // nothing keeps running or restarting
		})
	}
}

func TestStopJoinsTheReader(t *testing.T) {
	fake := streamexectest.NewFake(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	s := newTestSource(t, fake, testTiming, func() Decoder[string] {
		return DecoderFunc[string](func(line []byte) (string, bool) {
			if string(line) == "block" {
				close(entered)
				<-release
				return "", false
			}
			return string(line), true
		})
	})
	result := goBackground(t.Context(), s)
	conn := fake.Accept(t)
	conn.Line(t, "record 1")
	r := waitBackground(t, result)
	require.NoError(t, r.err)

	conn.Line(t, "block")
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the decoder did not receive the line")
	}
	stopped := make(chan struct{})
	go func() {
		r.stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("stop returned while the reader was still decoding")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not return after the reader finished")
	}
}
