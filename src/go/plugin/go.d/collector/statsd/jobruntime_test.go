// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestJobRuntimeEndToEnd drives the real framework job: autodetection, Run
// readiness, scheduled collection, template capture and protocol emission.
func TestJobRuntimeEndToEnd(t *testing.T) {
	addr := freeAddress(t)
	c := New()
	c.Listeners = []ListenerConfig{{Protocol: protocolUDP, Address: addr}, {Protocol: protocolTCP, Address: addr}}
	c.profileDirs = writeProfiles(t, testProfiles)
	c.Profiles = []string{"pools", "app"}
	out := &syncBuffer{}
	job := jobruntime.NewJobV2(jobruntime.JobV2Config{
		PluginName:  "go.d",
		Name:        "local",
		ModuleName:  "statsd",
		FullName:    "statsd_local",
		Module:      c,
		Out:         out,
		UpdateEvery: 1,
		StoreFirst:  true,
	})
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	requireFree(t, protocolUDP, addr) // Autodetection prepares without binding.

	run := jobruntime.NewManagedRun(context.Background(), nil)
	stopped := make(chan struct{})
	go func() { job.StartManaged(run); close(stopped) }()
	var stop sync.Once
	stopJob := func() {
		stop.Do(func() {
			job.Stop()
			select {
			case <-stopped:
			case <-time.After(waitFor):
				t.Error("job did not stop")
			}
			job.Cleanup()
		})
	}
	t.Cleanup(stopJob)
	select {
	case <-run.StartupDone():
	case <-time.After(waitFor):
		t.Fatal("job did not start")
	}
	require.NoError(t, run.StartupErr())
	require.True(t, run.Running())

	udp, err := net.Dial(protocolUDP, addr)
	require.NoError(t, err)
	defer udp.Close()
	datagram := "requests:2|c|@.5|#zone:a\nlevel:10|g\nlatency:10|ms\ndifference:-5|h\nmembers:a|s\nsvc.a.size:7|g"
	_, err = udp.Write([]byte(datagram))
	require.NoError(t, err)
	tcp, err := net.Dial(protocolTCP, addr)
	require.NoError(t, err)
	defer tcp.Close()
	_, err = tcp.Write([]byte("members:b|s\nlatency:20|ms\n"))
	require.NoError(t, err)
	f := &coreFixture{
		c: c,
	}
	f.waitCounts(t, receiverCounts{
		accepted: 8,
	})

	want := []string{
		"'statsd.c.total.requests'", "'statsd.g.value.level'", "'statsd.ms.values.latency'",
		"'statsd.h.values.difference'", "'statsd.s.cardinality.members'", "'statsd.pools.size'",
		"'statsd.app.pool_size'", "CLABEL 'zone' 'a'", "CLABEL 'pool' 'a'",
		"SET 'size' = 7", "SET 'level' = 10", "SET 'requests' = 4",
	}
	for clock := 1; ; clock++ {
		job.Tick(clock)
		output := out.String()
		missing := 0
		for _, s := range want {
			if !strings.Contains(output, s) {
				missing++
			}
		}
		if missing == 0 {
			break
		}
		if clock > 500 {
			t.Fatalf("missing %d expected fragments in output:\n%s", missing, output)
		}
		time.Sleep(10 * time.Millisecond)
	}

	stopJob()
	require.Nil(t, run.Failure(), "a requested stop is a normal runner return")
	requireFree(t, protocolUDP, addr)
	requireFree(t, protocolTCP, addr)
}

// TestConcurrentSocketInputAndCollection runs socket readers, profile replace
// and admission alongside Collect, capture and publication.
func TestConcurrentSocketInputAndCollection(t *testing.T) {
	f := startRuntime(t, func(c *Collector) {
		c.profileDirs = writeProfiles(t, testProfiles)
		c.Profiles = []string{"pools", "app"}
		c.MetricIdleTimeout = 0
	})
	const clients, updates = 4, 2000
	var wg sync.WaitGroup
	for i := range clients {
		conn := f.dialTCP(t)
		wg.Add(1)
		go func() {
			defer wg.Done()
			var batch strings.Builder
			for range updates {
				fmt.Fprintf(&batch, "svc.w%d.size:+1|g\nrequests:1|c|#worker:%d\n", i, i)
			}
			// Establish each baseline first; deltas without one are rejected.
			if _, err := fmt.Fprintf(conn, "svc.w%d.size:0|g\n", i); err != nil {
				t.Error(err)
				return
			}
			if _, err := conn.Write([]byte(batch.String())); err != nil {
				t.Error(err)
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	for collecting := true; collecting; {
		select {
		case <-done:
			collecting = false
		default:
		}
		f.collect(t, false, false)
	}
	f.waitCounts(t, receiverCounts{
		accepted: clients * (2*updates + 1),
	})
	f.collect(t, false, false)
	for i := range clients {
		value(t, f.c, "g.value.svc.pool.size", updates, map[string]string{"pool": fmt.Sprintf("w%d", i)})
		value(t, f.c, "c.total.requests", updates, map[string]string{"worker": fmt.Sprint(i)})
	}
	assert.Equal(t, []string{diagnosticsEntryID, "pools", "app"}, entryIDs(f.c))
}
