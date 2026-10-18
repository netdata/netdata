// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestProcessCandidateDaemonEnableFinitePass(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		for mode, auto := range map[string]bool{"passive": false, "automatic": true} {
			for sources, separate := range map[string]bool{"same-source": false, "distinct-sources": true} {
				t.Run(version+"/"+mode+"/"+sources, func(t *testing.T) {
					configs := continuationConfigs(separate)
					state := &continuationCollectorState{check: func(context.Context, string) error {
						return errors.New("endpoint unavailable")
					}}
					fixture := newCandidateEchoFixture(t, version, auto, state, configs)
					require.Eventually(t, func() bool { return len(state.checked()) >= 2 }, 3*time.Second, time.Millisecond)
					fixture.settle(t, 2)
					fixture.assertExposed(t, configs[1])
					fixture.command(t, "daemon-reconcile", "enable", "", 200)
					require.Eventually(t, func() bool {
						_, response, _ := strings.Cut(fixture.output.String(), "FUNCTION_RESULT_BEGIN daemon-reconcile ")
						_, afterReply, complete := strings.Cut(response, "FUNCTION_RESULT_END\n\n")
						return complete && strings.Contains(afterReply, "CONFIG go.d:collector:module:local2 status failed")
					}, time.Second, time.Millisecond, "Failed health must follow the reply that updates the daemon's status")

					// Explicit callers retain the ability to start a fresh pass.
					fixture.command(t, "explicit-enable", "enable", "user=test", 202)
					require.Eventually(t, func() bool { return len(state.checked()) >= 4 }, 3*time.Second, time.Millisecond)
					fixture.settle(t, 4)

					fixture.command(t, "explicit-disable", "disable", "user=test", 200)
					fixture.command(t, "daemon-reenable", "enable", "", 202)
					require.Eventually(t, func() bool { return len(state.checked()) >= 6 }, 3*time.Second, time.Millisecond)
					fixture.settle(t, 6)
				})
			}
		}
	}
}

func TestProcessCandidateDaemonEnablePreservesFallback(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		for sources, separate := range map[string]bool{"same-source": false, "distinct-sources": true} {
			t.Run(version+"/"+sources, func(t *testing.T) {
				configs := continuationConfigs(separate)
				release := make(chan struct{})
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				defer unblock()
				state := &continuationCollectorState{check: func(ctx context.Context, option string) error {
					if option == configs[0]["option_str"] {
						return errors.New("first endpoint unavailable")
					}
					select {
					case <-release:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}}
				fixture := newCandidateEchoFixture(t, version, true, state, configs)
				require.Eventually(t, func() bool { return len(state.checked()) >= 2 }, 3*time.Second, time.Millisecond)
				fixture.settle(t, 2)
				unblock()
				fixture.output.waitContains(t, "CONFIG go.d:collector:module:local2 status running")
				fixture.settle(t, 2)
				fixture.assertExposedStatus(t, configs[1], "running")
			})
		}
	}
}

func TestProcessCandidateDaemonEnablePreservesRetry(t *testing.T) {
	configs := continuationConfigs(true)
	for _, config := range configs {
		config["autodetection_retry"] = 1
	}
	checks := make(chan time.Time, 4)
	state := &continuationCollectorState{check: func(ctx context.Context, _ string) error {
		select {
		case checks <- time.Now():
		case <-ctx.Done():
		}
		return errors.New("endpoint unavailable")
	}}
	fixture := newCandidateEchoFixture(t, "v2", true, state, configs)
	require.Eventually(t, func() bool { return len(state.checked()) >= 2 }, 3*time.Second, time.Millisecond)
	fixture.settle(t, 2)
	fixture.command(t, "retry-reconcile", "enable", "", 200)
	require.Eventually(t, func() bool { return len(state.checked()) >= 3 }, 3*time.Second, time.Millisecond,
		"reconciliation must retain the scheduled retry")
	first := <-checks
	<-checks
	retry := <-checks
	require.GreaterOrEqual(t, retry.Sub(first), time.Second, "daemon replies must not bypass the retry deadline")
}

// Observe emitted registrations independently of the writer: replying inline
// would deadlock on the frame owner's output lock.
type candidateEchoOutput struct {
	*processSynchronizedBuffer
	changed chan struct{}
}

func (o *candidateEchoOutput) Write(payload []byte) (int, error) {
	n, err := o.processSynchronizedBuffer.Write(payload)
	select {
	case o.changed <- struct{}{}:
	default:
	}
	return n, err
}

type candidateEchoFixture struct {
	output *candidateEchoOutput
	input  *io.PipeWriter
	state  *continuationCollectorState
	wire   sync.Mutex
	echoes atomic.Int32
}

func newCandidateEchoFixture(t *testing.T, version string, auto bool, state *continuationCollectorState, configs []confgroup.Config) *candidateEchoFixture {
	t.Helper()
	reader, writer := io.Pipe()
	f := &candidateEchoFixture{
		output: &candidateEchoOutput{
			processSynchronizedBuffer: newProcessSynchronizedBuffer(),
			changed:                   make(chan struct{}, 1),
		},
		input: writer,
		state: state,
	}
	creator := collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Config:          func() any { return &collectorapi.MockConfiguration{} },
	}
	if version == "v2" {
		creator.CreateV2 = func() collectorapi.CollectorV2 {
			return &continuationV2Collector{
				readinessIntegrationCollector: readinessIntegrationCollector{store: metrix.NewCollectorStore()},
				state:                         state,
			}
		}
	} else {
		creator.Create = func() collectorapi.CollectorV1 { return &continuationV1Collector{state: state} }
	}
	batches := make(chan []*confgroup.Group, 1)
	cfg := testProductionProcessConfig(reader, f.output)
	cfg.AutoEnable = auto
	cfg.Modules = collectorapi.Registry{"module": creator}
	cfg.Defaults = confgroup.Registry{"module": {}}
	cfg.DiscoveryProviders = []agentdiscovery.ProviderFactory{
		agentdiscovery.NewProviderFactory("test", func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
			return continuationDiscoverer{batches: batches}, true, nil
		}),
	}
	process, err := NewProcess(cfg)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- process.Run(ctx) }()
	echoCtx, stopEcho := context.WithCancel(ctx)
	echoDone := make(chan error, 1)
	go func() { echoDone <- f.echo(echoCtx) }()
	t.Cleanup(func() {
		stopEcho()
		shutdown, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		defer cancel()
		defer reader.Close()
		defer writer.Close()
		err := process.Terminate(shutdown)
		_ = writer.Close()
		require.NoError(t, err)
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-shutdown.Done():
			t.Error("candidate process did not join")
		}
		select {
		case err := <-echoDone:
			require.NoError(t, err)
		case <-shutdown.Done():
			t.Error("daemon echo observer did not join")
		}
	})
	(&continuationFixture{t: t, batches: batches}).observe(configs)
	return f
}

func (f *candidateEchoFixture) echo(ctx context.Context) error {
	var consumed int
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-f.output.changed:
		}
		pending := f.output.String()[consumed:]
		end := strings.LastIndexByte(pending, '\n')
		if end < 0 {
			continue
		}
		consumed += end + 1
		for _, line := range strings.Split(pending[:end], "\n") {
			if !strings.HasPrefix(line, "CONFIG go.d:collector:module:local2 create ") {
				continue
			}
			uid := fmt.Sprintf("daemon-echo-%d", f.echoes.Add(1))
			if err := f.write(uid, "enable", ""); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

func (f *candidateEchoFixture) write(uid, command, source string) error {
	f.wire.Lock()
	defer f.wire.Unlock()
	_, err := fmt.Fprintf(f.input, "FUNCTION %s 3 \"config go.d:collector:module:local2 %s\" 0xFFFF %q\n", uid, command, source)
	return err
}

func (f *candidateEchoFixture) command(t *testing.T, uid, command, source string, code int) string {
	t.Helper()
	require.NoError(t, f.write(uid, command, source))
	f.output.waitContains(t, fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d ", uid, code))
	var body string
	require.Eventually(t, func() bool {
		_, response, ok := strings.Cut(f.output.String(), "FUNCTION_RESULT_BEGIN "+uid+" ")
		if !ok {
			return false
		}
		_, response, _ = strings.Cut(response, "\n")
		var complete bool
		body, _, complete = strings.Cut(response, "FUNCTION_RESULT_END")
		return complete
	}, time.Second, time.Millisecond)
	return strings.TrimSpace(body)
}

func (f *candidateEchoFixture) settle(t *testing.T, checks int) {
	t.Helper()
	require.Never(t, func() bool { return len(f.state.checked()) > checks }, 100*time.Millisecond, time.Millisecond,
		"automatic ENABLE must not restart a failed candidate pass")
	require.Equal(t, checks, len(f.state.checked()))
	require.Positive(t, f.echoes.Load(), "the real ingress must receive daemon ENABLE replies")
	require.NotContains(t, f.output.String(), "attempt superseded")
}

func (f *candidateEchoFixture) assertExposed(t *testing.T, config confgroup.Config) {
	t.Helper()
	f.assertExposedStatus(t, config, "failed")
}

func (f *candidateEchoFixture) assertExposedStatus(t *testing.T, config confgroup.Config, status string) {
	t.Helper()
	body := f.command(t, "exposed-config", "get", "user=test", 200)
	var raw collectorapi.MockConfiguration
	require.NoError(t, json.Unmarshal([]byte(body), &raw))
	require.Equal(t, config["option_str"], raw.OptionStr)
	var last string
	for _, line := range strings.Split(f.output.String(), "\n") {
		if strings.HasPrefix(line, "CONFIG go.d:collector:module:local2 create ") {
			last = line
		}
	}
	// A successful candidate is admitted as Accepted before readiness is published.
	if status == "failed" {
		require.Contains(t, last, "create failed ")
	} else {
		require.Contains(t, f.output.String(), "CONFIG go.d:collector:module:local2 status "+status)
	}
	require.Contains(t, last, " "+config.SourceType()+" '"+config.Source()+"'")
}
