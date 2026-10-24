// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import (
	"context"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	probing "github.com/prometheus-community/pro-bing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeResult struct {
	stats *probing.Statistics
	err   error
	sleep time.Duration
}

type fakeRunner struct {
	mu      sync.Mutex
	byHost  map[string][]fakeResult
	called  []string
	lastCfg ProbeConfig
	lastCtx context.Context
}

func (r *fakeRunner) probe(ctx context.Context, host string, cfg ProbeConfig) (*probing.Statistics, error) {
	r.mu.Lock()
	r.called = append(r.called, host)
	r.lastCfg = cfg
	r.lastCtx = ctx

	queue := r.byHost[host]
	if len(queue) == 0 {
		r.mu.Unlock()
		return nil, syscall.ENOENT
	}

	res := queue[0]
	r.byHost[host] = queue[1:]
	r.mu.Unlock()

	if res.sleep > 0 {
		timer := time.NewTimer(res.sleep)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return res.stats, res.err
}

func TestClient_ProbeDoesNotMutateState(t *testing.T) {
	runner := &fakeRunner{
		byHost: map[string][]fakeResult{
			"host": {{stats: testStats("host")}},
		},
	}

	c, err := newClient(testConfig(), logger.NewWithWriter(nil), runner)
	require.NoError(t, err)

	sample, err := c.Probe(context.Background(), "host")
	require.NoError(t, err)

	assert.True(t, sample.Jitter.InstantValid)
	assert.False(t, sample.Jitter.SmoothedValid)
	assert.Empty(t, c.state.byHost)
}

func TestClient_ProbeAndTrackMutatesState(t *testing.T) {
	runner := &fakeRunner{
		byHost: map[string][]fakeResult{
			"host": {
				{stats: testStats("host")},
				{stats: testStats("host")},
			},
		},
	}

	c, err := newClient(testConfig(), logger.NewWithWriter(nil), runner)
	require.NoError(t, err)

	sample, err := c.ProbeAndTrack(context.Background(), "host")
	require.NoError(t, err)
	assert.True(t, sample.Jitter.SmoothedValid)
	assert.Equal(t, time.Duration(156250), sample.Jitter.EWMA)
	assert.Equal(t, 2500*time.Microsecond, sample.Jitter.SMA)

	sample, err = c.ProbeAndTrack(context.Background(), "host")
	require.NoError(t, err)
	assert.Equal(t, time.Duration(302734), sample.Jitter.EWMA)
	assert.Equal(t, 2500*time.Microsecond, sample.Jitter.SMA)
}

func TestClient_ProbeNoReplyReturnsCountsAndLoss(t *testing.T) {
	runner := &fakeRunner{
		byHost: map[string][]fakeResult{
			"host": {{
				stats: &probing.Statistics{
					PacketsSent: 5,
					PacketsRecv: 0,
					PacketLoss:  100,
				},
			}},
		},
	}

	c, err := newClient(testConfig(), logger.NewWithWriter(nil), runner)
	require.NoError(t, err)

	sample, err := c.ProbeAndTrack(context.Background(), "host")
	require.NoError(t, err)

	assert.Equal(t, int64(5), sample.PacketsSent)
	assert.Equal(t, int64(0), sample.PacketsRecv)
	assert.Equal(t, 100.0, sample.PacketLossPct)
	assert.False(t, sample.RTT.Valid)
	assert.False(t, sample.Jitter.InstantValid)
	assert.False(t, sample.Jitter.SmoothedValid)
	assert.Empty(t, c.state.byHost)
}

func TestClient_ProbeWithSingleRTTDoesNotUpdateJitterState(t *testing.T) {
	runner := &fakeRunner{
		byHost: map[string][]fakeResult{
			"host": {{
				stats: &probing.Statistics{
					PacketsSent: 1,
					PacketsRecv: 1,
					PacketLoss:  0,
					Rtts:        []time.Duration{10 * time.Millisecond},
					MinRtt:      10 * time.Millisecond,
					MaxRtt:      10 * time.Millisecond,
					AvgRtt:      10 * time.Millisecond,
				},
			}},
		},
	}

	c, err := newClient(testConfig(), logger.NewWithWriter(nil), runner)
	require.NoError(t, err)

	sample, err := c.ProbeAndTrack(context.Background(), "host")
	require.NoError(t, err)

	assert.True(t, sample.RTT.Valid)
	assert.False(t, sample.Jitter.InstantValid)
	assert.False(t, sample.Jitter.SmoothedValid)
	assert.Empty(t, c.state.byHost)
}

func TestClient_ProbeConcurrentDifferentHosts(t *testing.T) {
	runner := &fakeRunner{
		byHost: map[string][]fakeResult{
			"host1": {{stats: testStats("host1"), sleep: 10 * time.Millisecond}},
			"host2": {{stats: testStats("host2"), sleep: 10 * time.Millisecond}},
		},
	}

	c, err := newClient(testConfig(), logger.NewWithWriter(nil), runner)
	require.NoError(t, err)

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	for _, host := range []string{"host1", "host2"} {
		wg.Go(func() {
			_, err := c.ProbeAndTrack(context.Background(), host)
			errCh <- err
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
}

func TestClient_ProbeUsesFakeRunnerSeam(t *testing.T) {
	runner := &fakeRunner{
		byHost: map[string][]fakeResult{
			"host": {{stats: testStats("host")}},
		},
	}

	c, err := newClient(testConfig(), logger.NewWithWriter(nil), runner)
	require.NoError(t, err)

	_, err = c.Probe(context.Background(), "host")
	require.NoError(t, err)
	assert.Equal(t, []string{"host"}, runner.called)
	assert.Equal(t, testConfig().Probe.Timeout, runner.lastCfg.Timeout)
}

func TestClient_ProbePassesContextToRunner(t *testing.T) {
	type ctxKey struct{}

	runner := &fakeRunner{
		byHost: map[string][]fakeResult{
			"host": {{stats: testStats("host")}},
		},
	}

	c, err := newClient(testConfig(), logger.NewWithWriter(nil), runner)
	require.NoError(t, err)

	ctx := context.WithValue(context.Background(), ctxKey{}, "probe")

	_, err = c.Probe(ctx, "host")
	require.NoError(t, err)
	require.NotNil(t, runner.lastCtx)
	assert.Equal(t, "probe", runner.lastCtx.Value(ctxKey{}))
}

func TestClient_ProbeContextCancellation(t *testing.T) {
	runner := &fakeRunner{
		byHost: map[string][]fakeResult{
			"host": {{stats: testStats("host"), sleep: 100 * time.Millisecond}},
		},
	}

	c, err := newClient(testConfig(), logger.NewWithWriter(nil), runner)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = c.Probe(ctx, "host")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func testConfig() Config {
	return Config{
		Probe: ProbeConfig{
			Packets:  5,
			Interval: confopt.Duration(100 * time.Millisecond),
			Timeout:  time.Second,
		},
	}
}

func testStats(host string) *probing.Statistics {
	return &probing.Statistics{
		Addr:        host,
		PacketsRecv: 5,
		PacketsSent: 5,
		PacketLoss:  0,
		Rtts: []time.Duration{
			10 * time.Millisecond,
			12 * time.Millisecond,
			15 * time.Millisecond,
			18 * time.Millisecond,
			20 * time.Millisecond,
		},
		MinRtt:    10 * time.Millisecond,
		MaxRtt:    20 * time.Millisecond,
		AvgRtt:    15 * time.Millisecond,
		StdDevRtt: 5 * time.Millisecond,
	}
}
