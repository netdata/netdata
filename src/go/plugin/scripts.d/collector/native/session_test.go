// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func persistentCollector(t *testing.T, body string) (*Collector, string) {
	t.Helper()
	c, dir := fixtureCollector(t, "[[ $1 == serve ]]\n"+body)
	data, err := os.ReadFile(c.Manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(c.Manifest, append(data, []byte("mode: persistent\n")...), 0644))
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	return c, dir
}

type testRuntime struct {
	cancel context.CancelFunc
	ready  chan struct{}
	done   chan struct{}
	err    error // Read after done closes.
}

func startRuntime(t *testing.T, c *Collector) *testRuntime {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &testRuntime{
		cancel: cancel,
		ready:  make(chan struct{}),
		done:   make(chan struct{}),
	}
	go func() {
		defer close(r.done)
		r.err = c.Run(ctx, func() { close(r.ready) })
	}()
	t.Cleanup(func() { cancel(); r.wait(t) })
	return r
}

func (r *testRuntime) wait(t *testing.T) {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(3 * time.Second):
		t.Fatal("persistent runtime did not stop")
	}
}

func (r *testRuntime) waitReady(t *testing.T) {
	t.Helper()
	select {
	case <-r.ready:
	case <-r.done:
		t.Fatalf("runtime failed before ready: %v", r.err)
	case <-time.After(3 * time.Second):
		t.Fatal("persistent runtime did not become ready")
	}
}

func bashHelper(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs("../../lib/native.sh")
	require.NoError(t, err)
	return "source '" + path + "'\n"
}

func TestPersistentStateAndTransientFailure(t *testing.T) {
	setupRunner(t)
	c, dir := persistentCollector(t, bashHelper(t)+`
printf '%s' "$$" > "$(dirname "$0")/pid"
nd_ready
count=0
while nd_next; do
    count=$((count+1))
    if [[ $count == 2 ]]; then nd_fail; continue; fi
    nd_begin
    nd_metric processed_total "$count" queue mail
    nd_metric depth "$$" queue mail
    nd_check backlog critical queue mail
    nd_end
done
`)
	// Both candidate paths remain local-only.
	_, err := os.Stat(filepath.Join(dir, "pid"))
	require.True(t, os.IsNotExist(err))
	r := startRuntime(t, c)
	r.waitReady(t)
	first, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, float64(1), first[`processed_total{queue="mail"}`])
	assert.Equal(t, float64(1), first[`native.check.backlog{native.check.backlog="critical",queue="mail"}`])
	_, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.ErrorIs(t, err, errCollectionFailed)
	third, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, float64(3), third[`processed_total{queue="mail"}`])
	assert.Equal(t, first[`depth{queue="mail"}`], third[`depth{queue="mail"}`], "same process after transient failure")
	r.cancel()
	r.wait(t)
	require.ErrorIs(t, r.err, context.Canceled)
	// A new generation cannot reuse its predecessor's state or closed channels.
	r2 := startRuntime(t, c)
	r2.waitReady(t)
	restarted, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, float64(1), restarted[`processed_total{queue="mail"}`])
	assert.NotEqual(t, first[`depth{queue="mail"}`], restarted[`depth{queue="mail"}`])
}

func TestPersistentStartupFailure(t *testing.T) {
	setupRunner(t)
	for name, body := range map[string]string{
		"no ready":         "sleep 30\n",
		"partial ready":    `printf '{"version":"v1","ready":true}'; sleep 30`,
		"invalid ready":    `printf '%s\n' '{"version":"v1","ready":false}'; sleep 30`,
		"early exit":       "exit 0\n",
		"eof without exit": "exec 1>&-\nsleep 30\n",
		"oversize":         "printf '%1048576s\\n' x\nsleep 30\n",
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := persistentCollector(t, body)
			c.Timeout = confopt.Duration(time.Second)
			r := startRuntime(t, c)
			r.wait(t)
			require.Error(t, r.err)
			select {
			case <-r.ready:
				t.Fatal("invalid startup signaled readiness")
			default:
			}
		})
	}
}

func TestPersistentTerminalReply(t *testing.T) {
	setupRunner(t)
	for name, body := range map[string]string{
		"wrong id":      `printf '%s\n' '{"id":"2","result":{"version":"v1"}}'`,
		"invalid batch": `printf '%s\n' '{"id":"1","result":{"version":"v1","metrics":[{"name":"depth","value":17},{"name":"undeclared","value":0}]}}'`,
		"unknown error": `printf '%s\n' '{"id":"1","error":"secret diagnostic"}'`,
		"partial":       `printf '%s' '{"id":"1","result":{"version":"v1"}}'`,
		"timeout":       ":",
		"exit":          "exit 1",
		"oversize":      `printf '%1048576s\n' x`,
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := persistentCollector(
				t,
				"printf '%s\\n' '{\"version\":\"v1\",\"ready\":true}'\nread -r request\n"+body+"\nsleep 30\n",
			)
			c.Timeout = confopt.Duration(time.Second)
			r := startRuntime(t, c)
			r.waitReady(t)
			// Intentionally commit even on failure: no sample may have been staged.
			managed, ok := metrix.AsCycleManagedStore(c.store)
			require.True(t, ok)
			managed.CycleController().BeginCycle()
			err := c.Collect(context.Background())
			require.Error(t, err)
			managed.CycleController().CommitCycleSuccess()
			values := map[string]float64{}
			c.store.Read(metrix.ReadRaw()).
				ForEachSeries(func(name string, _ metrix.LabelView, value float64) { values[name] = value })
			assert.Empty(t, values)
			r.wait(t)
			require.Error(t, r.err)
			assert.NotContains(t, r.err.Error(), "secret diagnostic")
		})
	}
}

func TestPersistentDuplicateReplyIsTerminal(t *testing.T) {
	setupRunner(t)
	c, _ := persistentCollector(t, bashHelper(t)+`
nd_ready
nd_next
nd_begin
nd_metric depth 1
nd_end
printf '%s\n' '{"id":"1","result":{"version":"v1","metrics":[{"name":"depth","value":999}]}}'
sleep 30
`)
	r := startRuntime(t, c)
	r.waitReady(t)
	first, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	// Run may already have failed by the time the caller receives its first reply.
	if err == nil {
		assert.Equal(t, float64(1), first["depth"])
	}
	_, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.Error(t, err)
	r.wait(t)
	require.Error(t, r.err)
}

func TestPersistentCancelInFlight(t *testing.T) {
	setupRunner(t)
	c, dir := persistentCollector(t, bashHelper(t)+`
nd_ready
nd_next
printf started > "$(dirname "$0")/started"
sleep 30
`)
	r := startRuntime(t, c)
	r.waitReady(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collected := make(chan error, 1)
	go func() { collected <- c.Collect(ctx) }()
	require.Eventually(
		t,
		func() bool { _, err := os.Stat(filepath.Join(dir, "started")); return err == nil },
		time.Second,
		time.Millisecond,
	)
	cancel()
	select {
	case err := <-collected:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("Collect ignored cancellation")
	}
	r.wait(t)
	require.ErrorIs(t, r.err, context.Canceled)
}

func TestPersistentFrameBoundaries(t *testing.T) {
	for _, size := range []int{1, maxResponseBytes - 1, maxResponseBytes, maxResponseBytes + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(strings.Repeat(" ", size-1) + "\n"))
			scanner.Buffer(make([]byte, 4096), maxResponseBytes+1)
			scanner.Split(splitFrame)
			if size <= maxResponseBytes {
				require.True(t, scanner.Scan())
				assert.Len(t, scanner.Bytes(), size-1)
				assert.False(t, scanner.Scan())
				require.NoError(t, scanner.Err())
			} else {
				assert.False(t, scanner.Scan())
				require.ErrorIs(t, scanner.Err(), errResponseTooLarge)
			}
		})
	}
	_, _, err := splitFrame([]byte("{}"), true)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestDecodePersistentEnvelope(t *testing.T) {
	c, _ := fixtureCollector(t, "exit 0\n")
	require.NoError(t, decodeReady([]byte(`{"version":"v1","ready":true}`)))
	for _, data := range []string{
		`{"version":"v2","ready":true}`, `{"version":"v1","Ready":true}`,
		`{"version":"v1","ready":true,"ready":true}`, `{"version":"v1","ready":null}`,
		`{"version":"v1","ready":true,"extra":0}`, `[]`,
	} {
		require.Error(t, decodeReady([]byte(data)), data)
	}
	for _, data := range []string{
		`{"id":"1"}`, `{"id":1,"result":{"version":"v1"}}`,
		`{"id":"1","result":null}`, `{"id":"1","error":null}`,
		`{"id":"1","result":{"version":"v1"},"error":"collection_failed"}`,
		`{"id":"1","result":{"version":"v1","checks":null}}`,
		`{"id":"1","id":"1","result":{"version":"v1"}}`,
		`{"id":"1","Result":{"version":"v1"}}`,
		`{"id":"1","error":"arbitrary text"}`, `{"id":"2","error":"collection_failed"}`,
	} {
		_, err := c.definition.decodeReply([]byte(data), "1")
		require.Error(t, err, data)
		require.False(t, errors.Is(err, errCollectionFailed), data)
	}
	_, err := c.definition.decodeReply([]byte(`{"id":"1","error":"collection_failed"}`), "1")
	require.ErrorIs(t, err, errCollectionFailed)
	_, err = c.definition.decodeReply([]byte(`{"id":"1","result":`+validResponse("critical")+`}`), "1")
	require.NoError(t, err)
}

func TestPersistentDevelopmentExamples(t *testing.T) {
	setupRunner(t)
	for _, language := range []string{"bash", "python"} {
		t.Run(language, func(t *testing.T) {
			if language == "python" {
				if _, err := exec.LookPath("python3"); err != nil {
					t.Skip("Python 3 is not installed")
				}
			}
			c := New()
			var err error
			c.Manifest, err = filepath.Abs("../../development/persistent-" + language + "/manifest.yaml")
			require.NoError(t, err)
			c.validateExecutable = func(path string) (string, error) { _, err := os.Stat(path); return path, err }
			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))
			r := startRuntime(t, c)
			r.waitReady(t)
			first, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Equal(t, float64(1), first[`processed_total{queue="mail"}`])
			assert.Equal(t, float64(1), first[`native.check.backlog{native.check.backlog="critical",queue="mail"}`])
			_, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.ErrorIs(t, err, errCollectionFailed)
			third, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Equal(t, float64(3), third[`processed_total{queue="mail"}`])
			assert.Equal(t, float64(1), third[`native.check.backlog{native.check.backlog="ok",queue="mail"}`])
		})
	}
}

func TestPersistentBlockedWriteCancellation(t *testing.T) {
	// A real pipe with no consumer forces the same blocked Write path as a peer
	// that stops reading stdin. Cancellation must join the writer.
	input, output, err := os.Pipe()
	require.NoError(t, err)
	defer input.Close()
	defer output.Close()
	s := &scriptSession{
		ctx:    context.Background(),
		stdin:  output,
		exited: make(chan struct{}),
		frames: make(chan scriptFrame),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := s.exchange(ctx, strings.Repeat("1", 1<<20)); finished <- err }()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("blocked writer was not joined")
	}
}

func TestBashSnapshotShapes(t *testing.T) {
	setupRunner(t)
	for name, tc := range map[string]struct {
		body   string
		series int
	}{
		"empty":        {body: "", series: 0},
		"metrics only": {body: "nd_metric depth 1", series: 1},
		"checks only":  {body: "nd_check backlog critical queue mail", series: 4},
	} {
		for _, mode := range []string{modeOneshot, modePersistent} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				body := bashHelper(t) + "nd_begin\n" + tc.body + "\nnd_end\n"
				var c *Collector
				if mode == modePersistent {
					c, _ = persistentCollector(t, bashHelper(t)+"nd_ready\nwhile nd_next; do\n"+body+"done\n")
					startRuntime(t, c).waitReady(t)
				} else {
					c, _ = fixtureCollector(t, body)
				}
				values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				require.NoError(t, err)
				assert.Len(t, values, tc.series)
			})
		}
	}
}
