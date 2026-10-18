// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
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
					configs := candidateEchoConfigs(t, separate, 10)
					state := &continuationCollectorState{check: func(context.Context, string) error {
						return errors.New("endpoint unavailable")
					}}
					fixture := newCandidateEchoFixture(t, version, auto, state, configs)
					require.Eventually(t, func() bool { return len(state.checked()) >= len(configs) }, 3*time.Second, time.Millisecond)
					fixture.settle(t, len(configs))
					fixture.assertExposed(t, configs[0])
					fixture.assertRegistrations(t, 1)
					fixture.command(t, "daemon-reconcile", "enable", "", 200)
					require.Eventually(t, func() bool {
						_, response, _ := strings.Cut(fixture.output.String(), "FUNCTION_RESULT_BEGIN daemon-reconcile ")
						_, afterReply, complete := strings.Cut(response, "FUNCTION_RESULT_END\n\n")
						return complete && strings.Contains(afterReply, "CONFIG go.d:collector:module:local2 status failed")
					}, time.Second, time.Millisecond, "Failed health must follow the reply that updates the daemon's status")

					fixture.command(t, "explicit-enable", "enable", "user=test", 200)
					fixture.settle(t, len(configs))

					fixture.command(t, "explicit-disable", "disable", "user=test", 200)
					fixture.command(t, "daemon-reenable", "enable", "", 202)
					require.Eventually(t, func() bool { return len(state.checked()) >= 2*len(configs) }, 3*time.Second, time.Millisecond)
					fixture.settle(t, 2*len(configs))
					fixture.assertExposed(t, configs[0])
					fixture.assertRegistrations(t, 1)
				})
			}
		}
	}
}

func candidateEchoConfigs(t *testing.T, separate bool, count int) []confgroup.Config {
	t.Helper()
	base := continuationConfigs(false)[0]
	configs := make([]confgroup.Config, count)
	for i := range configs {
		config, err := base.Clone()
		require.NoError(t, err)
		config["option_str"] = fmt.Sprintf("endpoint-%d", i)
		if separate {
			config.SetSource(fmt.Sprintf("file=candidate-%d", i))
		}
		configs[i] = config
	}
	sort.Slice(configs, func(i, j int) bool { return configs[i].UID() < configs[j].UID() })
	return configs
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
				registrations := 1
				if separate {
					registrations++
				}
				fixture.assertRegistrations(t, registrations)
			})
		}
	}
}

func TestProcessCandidateDaemonEnablePreservesRetry(t *testing.T) {
	configs := continuationConfigs(true)
	for _, config := range configs {
		config["autodetection_retry"] = 1
	}
	sort.Slice(configs, func(i, j int) bool { return configs[i].UID() < configs[j].UID() })
	state := &continuationCollectorState{check: func(context.Context, string) error {
		return errors.New("endpoint unavailable")
	}}
	fixture := newCandidateEchoFixture(t, "v2", true, state, configs)
	require.Eventually(t, func() bool { return len(state.checked()) >= 2 }, 3*time.Second, time.Millisecond)
	fixture.settle(t, 2)
	fixture.command(t, "retry-reconcile", "enable", "", 200)
	require.Eventually(t, func() bool { return len(state.checked()) >= 3 }, 3*time.Second, time.Millisecond,
		"reconciliation must retain the scheduled retry")
	require.Equal(t, []string{
		configs[0]["option_str"].(string),
		configs[1]["option_str"].(string),
		configs[0]["option_str"].(string),
	}, state.checked()[:3])
}

func TestProcessCandidateDaemonPolicySameSourceReplacement(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		for _, initial := range []string{"running", "failed", "disabled"} {
			t.Run(version+"/"+initial, func(t *testing.T) {
				configs := continuationConfigs(false)[:1]
				state := &continuationCollectorState{check: func(_ context.Context, option string) error {
					if initial == "failed" && option == configs[0]["option_str"] {
						return errors.New("original endpoint unavailable")
					}
					return nil
				}}
				fixture := newCandidateEchoFixture(t, version, false, state, configs)
				health := initial
				if initial == "disabled" {
					fixture.output.waitContains(t, "CONFIG go.d:collector:module:local2 status running")
					fixture.policyDisabled.Store(true)
					fixture.command(t, "disable-before-reload", "disable", "user=test", 200)
				}
				fixture.output.waitContains(t, "CONFIG go.d:collector:module:local2 status "+health)
				fixture.assertRegistrations(t, 1)
				checks, code := 2, 202
				if initial == "disabled" {
					checks, code = 1, 200
				} else {
					health = "running"
				}
				statuses := strings.Count(fixture.output.String(), "CONFIG go.d:collector:module:local2 status "+health)
				replacement, err := configs[0].Clone()
				require.NoError(t, err)
				replacement["option_str"] = "replacement-endpoint"
				(&continuationFixture{t: t, batches: fixture.batches}).observe([]confgroup.Config{replacement})
				fixture.output.waitContains(t, fmt.Sprintf("FUNCTION_RESULT_BEGIN daemon-echo-2 %d ", code))
				require.Eventually(t, func() bool {
					return len(state.checked()) == checks && strings.Count(fixture.output.String(),
						"CONFIG go.d:collector:module:local2 status "+health) > statuses
				}, 3*time.Second, time.Millisecond, "a genuine replacement must receive current daemon policy")
				fixture.assertRegistrations(t, 2)
				fixture.assertExposedStatus(t, replacement, health)
				require.Never(t, func() bool { return len(state.checked()) > checks }, 100*time.Millisecond, time.Millisecond)
			})
		}
	}
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
	batches        chan []*confgroup.Group
	output         *candidateEchoOutput
	input          *io.PipeWriter
	state          *continuationCollectorState
	wire           sync.Mutex
	echoes         atomic.Int32
	policyDisabled atomic.Bool
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
	f.batches = batches
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
			command := "enable"
			if f.policyDisabled.Load() {
				command = "disable"
			}
			if err := f.write(uid, command, ""); err != nil {
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
	var last, health string
	for _, line := range strings.Split(f.output.String(), "\n") {
		if strings.HasPrefix(line, "CONFIG go.d:collector:module:local2 create ") {
			last = line
			health = strings.Fields(line)[3]
		} else if strings.HasPrefix(line, "CONFIG go.d:collector:module:local2 status ") {
			health = strings.Fields(line)[3]
		}
	}
	require.Equal(t, status, health)
	require.Contains(t, last, " "+config.SourceType()+" '"+config.Source()+"'")
}

func (f *candidateEchoFixture) assertRegistrations(t *testing.T, expected int) {
	t.Helper()
	count := 0
	for _, line := range strings.Split(f.output.String(), "\n") {
		if strings.HasPrefix(line, "CONFIG go.d:collector:module:local2 create ") {
			count++
		}
	}
	require.Equal(t, expected, count, "private probes must not register endpoint configurations")
}
