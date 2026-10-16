// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// Exercise discovery, the real command lanes, candidate preparation, activation,
// and public DynCfg output. Only collector I/O and source observations are faked.
func TestCandidateContinuationGetPreservesRawReference(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			configs := continuationConfigs(false)[:1]
			configs[0]["option_str"] = "${env:NETDATA_CANDIDATE_REFERENCE}"
			state := &continuationCollectorState{}
			fixture := newContinuationFixture(t, version, false, state, configs)
			fixture.waitStatus(dyncfg.StatusAccepted)
			fixture.assertExposed("raw-reference", configs[0])
			require.Empty(t, state.checked())
			require.Zero(t, state.inits.Load(), "GET must preserve the raw reference without initializing a collector")
			require.NoError(t, fixture.generation.run.DirtyCause())
		})
	}
}

func TestCandidateContinuationFirstSuccessControl(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		for mode, auto := range map[string]bool{"daemon-enable": false, "automatic": true} {
			t.Run(version+"/"+mode, func(t *testing.T) {
				configs := continuationConfigs(true)
				state := &continuationCollectorState{}
				fixture := newContinuationFixture(t, version, auto, state, configs)
				fixture.enableIfPassive(auto)
				fixture.waitStatus(dyncfg.StatusRunning)
				require.Equal(t, []string{configs[0]["option_str"].(string)}, state.checked())
				fixture.assertExposed("first-success", configs[0])
				fixture.assertLastMetadata(configs[0])
				require.NoError(t, fixture.generation.run.DirtyCause())
			})
		}
	}
}

func TestCandidateContinuationReachesRunning(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		for mode, auto := range map[string]bool{"daemon-enable": false, "automatic": true} {
			for slots, separate := range map[string]bool{"same-source": false, "separate-sources": true} {
				t.Run(version+"/"+mode+"/"+slots, func(t *testing.T) {
					configs := continuationConfigs(separate)
					state := &continuationCollectorState{check: func(_ context.Context, option string) error {
						if option == configs[0]["option_str"] {
							return errors.New("endpoint unavailable")
						}
						return nil
					}}
					fixture := newContinuationFixture(t, version, auto, state, configs)
					fixture.enableIfPassive(auto)
					fixture.waitStatus(dyncfg.StatusRunning)
					require.Equal(t, []string{configs[0]["option_str"].(string), configs[1]["option_str"].(string)}, state.checked())
					fixture.assertExposed("selected", configs[1])
					fixture.assertLastMetadata(configs[1])
					if version == "v2" {
						require.EqualValues(t, 1, state.active.Load())
						require.EqualValues(t, 1, state.maxActive.Load(), "one runtime may own the same job")
					}

					require.NoError(t, fixture.generation.run.DirtyCause())
				})
			}
		}
	}
}

func TestCandidateContinuationSuccessfulSiblingSurvivesPeriodicObservations(t *testing.T) {
	for mode, auto := range map[string]bool{"daemon-enable": false, "automatic": true} {
		t.Run(mode, func(t *testing.T) {
			configs := continuationConfigs(true)
			state := &continuationCollectorState{check: func(_ context.Context, option string) error {
				if option == configs[0]["option_str"] {
					return errors.New("endpoint unavailable")
				}
				return nil
			}}
			fixture := newContinuationFixture(t, "v2", auto, state, configs)
			fixture.enableIfPassive(auto)
			fixture.waitStatus(dyncfg.StatusRunning)
			for range 3 {
				fixture.observe(configs)
			}
			// The real discovery pipeline batches subsequent observations every two
			// seconds. Keep this oracle alive through that boundary.
			require.Never(t, func() bool { return len(state.checked()) != 2 }, 3*time.Second, time.Millisecond)
			fixture.assertExposed("after-identical", configs[1])
			require.EqualValues(t, 1, state.maxActive.Load())
			require.NoError(t, fixture.generation.run.DirtyCause())
		})
	}
}

func TestCandidateContinuationFailedBindingFollowsLastCandidate(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		for mode, auto := range map[string]bool{"daemon-enable": false, "automatic": true} {
			t.Run(version+"/"+mode, func(t *testing.T) {
				configs := continuationConfigs(true)
				state := &continuationCollectorState{check: func(context.Context, string) error {
					return errors.New("endpoint unavailable")
				}}
				fixture := newContinuationFixture(t, version, auto, state, configs)
				fixture.enableIfPassive(auto)
				require.Eventually(t, func() bool { return len(state.checked()) == 2 }, time.Second, time.Millisecond)
				fixture.waitStatus(dyncfg.StatusFailed)
				fixture.assertExposed("failed-binding", configs[1])
				fixture.assertLastMetadata(configs[1])
				require.Equal(t, []string{configs[0]["option_str"].(string), configs[1]["option_str"].(string)}, state.checked())
				require.Zero(t, state.active.Load())

				// Stored-config commands must target the exposed candidate, even
				// though another failed candidate shares its public module/name.
				fixture.command("restart-last", "restart", 422)
				require.Equal(t, []string{configs[0]["option_str"].(string), configs[1]["option_str"].(string), configs[1]["option_str"].(string)}, state.checked())
				fixture.assertExposed("after-restart", configs[1])
				require.NoError(t, fixture.generation.run.DirtyCause())
			})
		}
	}
}

func TestCandidateContinuationDisableRevokesDeferredSibling(t *testing.T) {
	for mode, auto := range map[string]bool{"daemon-enable": false, "automatic": true} {
		t.Run(mode, func(t *testing.T) {
			configs := continuationConfigs(true)
			entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			state := &continuationCollectorState{check: func(_ context.Context, option string) error {
				if option == configs[0]["option_str"] {
					return errors.New("endpoint unavailable")
				}
				close(entered)
				// Model collector I/O which physically returns after cancellation.
				<-release
				close(returned)
				return nil
			}}
			fixture := newContinuationFixture(t, "v2", auto, state, configs)
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			fixture.enableIfPassive(auto)
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("the sibling did not enter deferred Check")
			}
			fixture.command("disable-deferred", "disable", 200)
			fixture.waitStatus(dyncfg.StatusDisabled)
			releaseOnce.Do(func() { close(release) })
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Fatal("deferred Check did not physically return")
			}
			fixture.observe(configs)
			require.Never(t, func() bool {
				record, ok := fixture.generation.vnodes.graph.Lookup(configs[0].FullName())
				return !ok || record.Status != dyncfg.StatusDisabled.String() || state.maxActive.Load() != 0 || len(state.checked()) != 2
			}, 100*time.Millisecond, time.Millisecond, "late completion or identical inventory cannot enable a sibling")
			require.NoError(t, fixture.generation.run.DirtyCause())
		})
	}
}

func TestCandidateContinuationReadsAndRejectedEditsKeepAdmittedBinding(t *testing.T) {
	for mode, auto := range map[string]bool{"daemon-enable": false, "automatic": true} {
		t.Run(mode, func(t *testing.T) {
			configs := continuationConfigs(true)
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			var siblingChecks atomic.Int32
			state := &continuationCollectorState{check: func(_ context.Context, option string) error {
				switch option {
				case configs[0]["option_str"]:
					return errors.New("endpoint unavailable")
				case "rejected-edit":
					return collectorapi.PermanentError(errors.New("invalid edit"))
				case configs[1]["option_str"]:
					if siblingChecks.Add(1) == 1 {
						close(entered)
						<-release
					}
				}
				return nil
			}}
			fixture := newContinuationFixture(t, "v2", auto, state, configs)
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			fixture.enableIfPassive(auto)
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("sibling Check did not begin")
			}
			// A still owns the admitted binding until B's preflight is accepted.
			fixture.assertExposed("before-adoption", configs[0])
			fixture.assertLastMetadata(configs[0])
			releaseOnce.Do(func() { close(release) })
			fixture.waitStatus(dyncfg.StatusRunning)
			fixture.assertExposed("after-adoption", configs[1])
			fixture.assertLastMetadata(configs[1])

			for _, command := range []string{"test", "update"} {
				fixture.commandPayload("rejected-"+command, command, 422, `{"option_str":"rejected-edit"}`)
				fixture.assertExposed("after-rejected-"+command, configs[1])
				fixture.assertLastMetadata(configs[1])
			}
			fixture.command("restart-admitted", "restart", 200)
			fixture.assertExposed("after-exact-restart", configs[1])
			require.Equal(t, []string{
				configs[0]["option_str"].(string), configs[1]["option_str"].(string),
				"rejected-edit", "rejected-edit", configs[1]["option_str"].(string),
			}, state.checked())
			require.EqualValues(t, 1, state.maxActive.Load(), "restart must retire the prior runtime before starting its successor")

			before := state.checked()
			require.JSONEq(t, collectorapi.MockConfigSchema, fixture.command("module-schema", "schema", 200))
			rendered := fixture.commandPayload("caller-userconfig", "userconfig", 200, `{"option_str":"caller-config","option_int":17}`)
			var userconfig struct {
				Jobs []collectorapi.MockConfiguration `yaml:"jobs"`
			}
			require.NoError(t, yaml.Unmarshal([]byte(rendered), &userconfig))
			require.Equal(t, []collectorapi.MockConfiguration{{OptionStr: "caller-config", OptionInt: 17}}, userconfig.Jobs)
			require.Equal(t, before, state.checked(), "SCHEMA and USERCONFIG cannot probe or replace the admitted config")
			fixture.assertExposed("after-read-commands", configs[1])
			require.NoError(t, fixture.generation.run.DirtyCause())
		})
	}
}

func TestCandidateContinuationHiddenStockReappearsWithNewBinding(t *testing.T) {
	configs := continuationConfigs(true)
	for _, config := range configs {
		config.SetSourceType(confgroup.TypeStock)
	}
	sort.Slice(configs, func(i, j int) bool { return configs[i].UID() < configs[j].UID() })
	state := &continuationCollectorState{check: func(_ context.Context, option string) error {
		if option == "new-stock" {
			return nil
		}
		return errors.New("endpoint unavailable")
	}}
	fixture := newContinuationFixture(t, "v2", true, state, configs)
	require.Eventually(t, func() bool { return len(state.checked()) == 2 }, time.Second, time.Millisecond)
	require.Eventually(t, func() bool {
		_, exists := fixture.generation.vnodes.graph.Lookup(configs[0].FullName())
		return !exists
	}, time.Second, time.Millisecond)
	fixture.command("hidden-stock-get", "get", 404)
	replacement := confgroup.Config{"module": "module", "name": "local2", "update_every": 1, "option_str": "new-stock"}.
		SetSourceType(confgroup.TypeStock).SetProvider("test").SetSource("file=stock-new")
	fixture.observe(append(configs, replacement))
	fixture.waitStatus(dyncfg.StatusRunning)
	fixture.assertExposed("new-stock-get", replacement)
	fixture.assertLastMetadata(replacement)
	require.Equal(t, []string{configs[0]["option_str"].(string), configs[1]["option_str"].(string), "new-stock"}, state.checked())
	require.NoError(t, fixture.generation.run.DirtyCause())
}

func TestCandidateContinuationFailedUserTierMasksStock(t *testing.T) {
	for mode, auto := range map[string]bool{"daemon-enable": false, "automatic": true} {
		t.Run(mode, func(t *testing.T) {
			configs := continuationConfigs(true)
			lower := confgroup.Config{"module": "module", "name": "local2", "update_every": 1, "option_str": "stock"}.
				SetSourceType(confgroup.TypeStock).SetProvider("test").SetSource("file=stock")
			state := &continuationCollectorState{check: func(_ context.Context, option string) error {
				if option == "stock" {
					return nil
				}
				return errors.New("endpoint unavailable")
			}}
			fixture := newContinuationFixture(t, "v2", auto, state, append(configs, lower))
			fixture.enableIfPassive(auto)
			require.Eventually(t, func() bool { return len(state.checked()) >= 2 }, time.Second, time.Millisecond)
			fixture.waitStatus(dyncfg.StatusFailed)
			require.Never(t, func() bool { return len(state.checked()) != 2 || state.maxActive.Load() != 0 }, 100*time.Millisecond, time.Millisecond)
			require.Equal(t, []string{configs[0]["option_str"].(string), configs[1]["option_str"].(string)}, state.checked())
			fixture.assertExposed("masked-stock", configs[1])
			require.NoError(t, fixture.generation.run.DirtyCause())
		})
	}
}

// Choose the failure from the production UID order, not from YAML slice order.
func continuationConfigs(separate bool) []confgroup.Config {
	configs := []confgroup.Config{
		{"module": "module", "name": "local2", "update_every": 1, "option_str": "one"},
		{"module": "module", "name": "local2", "update_every": 1, "option_str": "two"},
	}
	for i, config := range configs {
		source := "file=candidates"
		if separate {
			source = fmt.Sprintf("file=candidate-%d", i)
		}
		config.SetSourceType(confgroup.TypeUser).SetProvider("test").SetSource(source)
		config.ApplyDefaults(confgroup.Default{})
	}
	sort.Slice(configs, func(i, j int) bool { return configs[i].UID() < configs[j].UID() })
	return configs
}

type continuationCollectorState struct {
	mu        sync.Mutex
	checks    []string
	inits     atomic.Int32
	check     func(context.Context, string) error
	active    atomic.Int32
	maxActive atomic.Int32
}

func (s *continuationCollectorState) checkConfig(ctx context.Context, option string) error {
	s.mu.Lock()
	s.checks = append(s.checks, option)
	s.mu.Unlock()
	if s.check != nil {
		return s.check(ctx, option)
	}
	return nil
}

func (s *continuationCollectorState) checked() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.checks...)
}

type continuationV2Collector struct {
	readinessIntegrationCollector
	Config collectorapi.MockConfiguration `yaml:",inline"`
	state  *continuationCollectorState
}

func (c *continuationV2Collector) Configuration() any         { return c.Config }
func (c *continuationV2Collector) Init(context.Context) error { c.state.inits.Add(1); return nil }
func (c *continuationV2Collector) Check(ctx context.Context) error {
	return c.state.checkConfig(ctx, c.Config.OptionStr)
}
func (c *continuationV2Collector) Run(ctx context.Context, ready func()) error {
	active := c.state.active.Add(1)
	defer c.state.active.Add(-1)
	for previous := c.state.maxActive.Load(); active > previous; previous = c.state.maxActive.Load() {
		if c.state.maxActive.CompareAndSwap(previous, active) {
			break
		}
	}
	ready()
	<-ctx.Done()
	return ctx.Err()
}

type continuationV1Collector struct {
	collectorapi.Base
	Config collectorapi.MockConfiguration `yaml:",inline"`
	state  *continuationCollectorState
}

func (c *continuationV1Collector) Configuration() any         { return c.Config }
func (c *continuationV1Collector) Init(context.Context) error { c.state.inits.Add(1); return nil }
func (*continuationV1Collector) Cleanup(context.Context)      {}
func (c *continuationV1Collector) Check(ctx context.Context) error {
	return c.state.checkConfig(ctx, c.Config.OptionStr)
}

func (c *continuationV1Collector) Charts() *collectorapi.Charts {
	return &collectorapi.Charts{&collectorapi.Chart{
		ID: "value", Title: "Value", Units: "value",
		Dims: collectorapi.Dims{&collectorapi.Dim{ID: "value"}},
	}}
}

func (*continuationV1Collector) Collect(context.Context) map[string]int64 {
	return map[string]int64{"value": 1}
}

type continuationDiscoverer struct {
	batches <-chan []*confgroup.Group
}

func (d continuationDiscoverer) Run(ctx context.Context, out chan<- []*confgroup.Group) {
	for {
		select {
		case <-ctx.Done():
			return
		case batch := <-d.batches:
			select {
			case out <- batch:
			case <-ctx.Done():
				return
			}
		}
	}
}

type continuationFixture struct {
	t          *testing.T
	generation *runGeneration
	output     *processSynchronizedBuffer
	batches    chan []*confgroup.Group
	state      *continuationCollectorState
}

func newContinuationFixture(t *testing.T, version string, auto bool, state *continuationCollectorState, configs []confgroup.Config) *continuationFixture {
	t.Helper()
	fixture := &continuationFixture{t: t, output: newProcessSynchronizedBuffer(), batches: make(chan []*confgroup.Group, 4), state: state}
	creator := collectorapi.Creator{JobConfigSchema: collectorapi.MockConfigSchema, Config: func() any { return &collectorapi.MockConfiguration{} }}
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
	catalog, err := agentdiscovery.NewProviderCatalog([]agentdiscovery.ProviderFactory{
		agentdiscovery.NewProviderFactory("test", func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
			return continuationDiscoverer{batches: fixture.batches}, true, nil
		}),
	})
	require.NoError(t, err)
	frames, err := lifecycle.NewFrameOwner(fixture.output)
	require.NoError(t, err)
	uids := lifecycle.NewUIDLedger()
	fixture.generation, err = newTestRunGeneration(t, runGenerationConfig{
		Generation: 1, ShutdownTimeout: time.Second, UIDs: uids, Frames: frames,
		Modules: collectorapi.Registry{"module": creator}, Jobs: testRunJobServices(t),
		Discovery: runDiscoveryServices{
			BuildContext: agentdiscovery.BuildContext{Registry: confgroup.Registry{"module": {}}},
			Providers:    catalog, AutoEnable: auto,
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fixture.generation.Stop()
		require.NoError(t, fixture.generation.Wait(context.Background()))
		closeRunTestUIDs(t, uids)
	})
	fixture.observe(configs)
	require.NoError(t, fixture.generation.start(context.Background()))
	return fixture
}

func (f *continuationFixture) observe(configs []confgroup.Config) {
	f.t.Helper()
	bySource := make(map[string][]confgroup.Config)
	for _, config := range configs {
		bySource[config.Source()] = append(bySource[config.Source()], config)
	}
	var batch []*confgroup.Group
	for source, entries := range bySource {
		batch = append(batch, &confgroup.Group{Source: source, Configs: entries})
	}
	select {
	case f.batches <- batch:
	case <-time.After(time.Second):
		f.t.Fatal("discovery did not consume source observation")
	}
}

func (f *continuationFixture) enableIfPassive(auto bool) {
	f.t.Helper()
	if !auto {
		f.waitStatus(dyncfg.StatusAccepted)
		require.Empty(f.t, f.state.checked(), "daemon registration must remain passive")
		f.command("enable-candidates", "enable", 202)
	}
}

func (f *continuationFixture) waitStatus(status dyncfg.Status) {
	f.t.Helper()
	require.Eventually(f.t, func() bool {
		record, ok := f.generation.vnodes.graph.Lookup("module_local2")
		return ok && record.Status == status.String()
	}, 3*time.Second, time.Millisecond, "expected %s; output=%s", status, f.output)
}

func (f *continuationFixture) command(uid, command string, code int) string {
	f.t.Helper()
	return f.commandPayload(uid, command, code, "")
}

func (f *continuationFixture) commandPayload(uid, command string, code int, payload string) string {
	f.t.Helper()
	require.NoError(f.t, f.generation.kernel.Submit(f.t.Context(), jobmgr.Request{
		UID: uid, Source: lifecycle.SourceFunction, Route: "config",
		Args: []string{"go.d:collector:module:local2", command}, Deadline: time.Now().Add(3 * time.Second),
		HasPayload: payload != "", Payload: []byte(payload), ContentType: "application/json", CallerSource: "user=test",
	}))
	f.output.waitContains(f.t, fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d ", uid, code))
	require.Eventually(f.t, func() bool {
		_, response, ok := strings.Cut(f.output.String(), "FUNCTION_RESULT_BEGIN "+uid+" ")
		return ok && strings.Contains(response, "FUNCTION_RESULT_END")
	}, time.Second, time.Millisecond)
	_, response, _ := strings.Cut(f.output.String(), "FUNCTION_RESULT_BEGIN "+uid+" ")
	_, body, _ := strings.Cut(response, "\n")
	body, _, _ = strings.Cut(body, "FUNCTION_RESULT_END")
	return strings.TrimSpace(body)
}

func (f *continuationFixture) assertExposed(uid string, config confgroup.Config) {
	f.t.Helper()
	require.Eventually(f.t, func() bool {
		record, ok := f.generation.vnodes.graph.Lookup(config.FullName())
		if !ok {
			return false
		}
		var raw confgroup.Config
		return yaml.Unmarshal([]byte(record.Payload()), &raw) == nil && raw["option_str"] == config["option_str"]
	}, time.Second, time.Millisecond, "the exposed graph must advance to the admitted candidate")
	before := f.state.checked()
	beforeInits := f.state.inits.Load()
	body := f.command(uid, "get", 200)
	var raw collectorapi.MockConfiguration
	require.NoError(f.t, json.Unmarshal([]byte(body), &raw))
	expected := collectorapi.MockConfiguration{OptionStr: config["option_str"].(string)}
	if value, ok := config["option_int"].(int); ok {
		expected.OptionInt = value
	}
	require.Equal(f.t, expected, raw, "GET must describe the admitted candidate")
	require.Equal(f.t, before, f.state.checked(), "GET must not execute Check")
	require.Equal(f.t, beforeInits, f.state.inits.Load(), "GET must not execute Init")
}

func (f *continuationFixture) assertLastMetadata(config confgroup.Config) {
	f.t.Helper()
	var last string
	require.Eventually(f.t, func() bool {
		for _, line := range strings.Split(f.output.String(), "\n") {
			if strings.HasPrefix(line, "CONFIG go.d:collector:module:local2 create ") {
				last = line
			}
		}
		return strings.Contains(last, " "+config.SourceType()+" '"+config.Source()+"'")
	}, time.Second, time.Millisecond, "CONFIG metadata must identify %s; output=%s", config.Source(), f.output)
}
