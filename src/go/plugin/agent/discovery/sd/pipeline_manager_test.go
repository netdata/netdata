// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/require"
)

func applyTestCommand(t *testing.T, d *ServiceDiscovery, fn dyncfg.Function) dyncfg.AppliedCommand {
	t.Helper()
	p, err := d.prepareDyncfgCommand(fn)
	require.NoError(t, err)
	applied, err := p.Apply(fn.Context())
	require.NoError(t, err)
	if applied.Published != nil {
		applied.Published()
	}
	return applied
}

// Only legacy protocol fixtures need preloaded pipelines. Use the same actor
// Apply boundary as production; no manager API bypasses serialization.
type testActorPrepared struct{ apply func() }

func (p testActorPrepared) Apply(context.Context) (dyncfg.AppliedCommand, error) {
	p.apply()
	return dyncfg.AppliedCommand{}, nil
}
func (p testActorPrepared) Dispose(context.Context) error { return nil }
func publishTestPipeline(d *ServiceDiscovery, cfg sdConfig, pipeline sdPipeline) {
	request := sdActorCommand{
		ctx:    d.ctx,
		result: make(chan sdActorResult, 1),
		prepared: testActorPrepared{
			apply: func() {
				publish, err := d.mgr.enable(cfg, pipeline, nil)
				if err != nil {
					panic(err)
				}
				publish()
			},
		},
	}
	d.actorCommands <- request
	result := <-request.result
	result.applied.Published()
}

func newActorDiscovery(
	t *testing.T,
	attemptsOverride Config,
	factory func(pipeline.Config) (sdPipeline, error),
) (*ServiceDiscovery, chan confFile, chan []*confgroup.Group, context.CancelFunc) {
	t.Helper()
	if attemptsOverride.Attempts == nil {
		attemptsOverride.Attempts = newTestAttemptAuthority(t)
	}
	if attemptsOverride.Epoch == 0 {
		attemptsOverride.Epoch = 1
	}
	attemptsOverride.PluginName = testPluginName
	attemptsOverride.Discoverers = testDiscovererRegistry()
	attemptsOverride.RunModePolicy = policy.RunModePolicy{
		AutoEnableDiscovered: true,
	}
	d, err := NewServiceDiscovery(attemptsOverride)
	require.NoError(t, err)
	d.newPipeline = factory
	configs := make(chan confFile)
	d.confProv = &mockConfigProvider{
		ch: configs,
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.ctx = ctx
	d.output = make(chan []*confgroup.Group)
	output := make(chan []*confgroup.Group)
	d.output = output
	d.mgr = NewPipelineManager(d.Logger)
	d.mgr.bind(d)
	done := make(chan struct{})
	go func() { defer close(done); d.run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("actor did not stop")
		}
	})
	return d, configs, output, cancel
}
func actorFunction(d *ServiceDiscovery, ctx context.Context, name, command string) dyncfg.Function {
	return dyncfg.NewFunction(
		ctx,
		functions.Function{
			UID:  command,
			Args: []string{d.dyncfgJobID(testDiscovererTypeNetListeners, name), command},
		},
	)
}

type controlledPipeline struct {
	started  chan struct{}
	canceled chan struct{}
	release  <-chan struct{}
	groups   []*confgroup.Group
	finite   bool
}

func (p *controlledPipeline) Test(context.Context) (bool, error) { return false, nil }
func (p *controlledPipeline) Run(ctx context.Context, out chan<- []*confgroup.Group) {
	if p.started != nil {
		close(p.started)
	}
	if p.groups != nil {
		select {
		case out <- p.groups:
		case <-ctx.Done():
		}
	}
	if !p.finite {
		<-ctx.Done()
	}
	if p.canceled != nil {
		close(p.canceled)
	}
	if p.release != nil {
		<-p.release
	}
}
func waitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("signal did not arrive")
	}
}

func TestActorAcceptanceWaitsForPublicationBeforeActivation(t *testing.T) {
	started := make(chan struct{})
	d, configs, _, _ := newActorDiscovery(
		t,
		Config{},
		func(pipeline.Config) (sdPipeline, error) {
			return &controlledPipeline{
				started: started,
			}, nil
		},
	)
	// A manually exposed disabled entry isolates enable publication from auto-enable.
	cfg, err := newSDConfigFromYAML(prepareConfigFile("origin", "job").content, "origin", confgroup.TypeUser, "origin")
	require.NoError(t, err)
	_ = configs
	d.handler.AddDiscoveredConfig(cfg, dyncfg.StatusDisabled)
	fnctx, cancel := context.WithCancel(context.Background())
	p, err := d.prepareDyncfgCommand(actorFunction(d, fnctx, "job", "enable"))
	require.NoError(t, err)
	applied, err := p.Apply(fnctx)
	require.NoError(t, err)
	require.Equal(t, 202, applied.Result.Code)
	cancel()
	select {
	case <-started:
		t.Fatal("activation preceded publication")
	case <-time.After(20 * time.Millisecond):
	}
	applied.Published()
	applied.Published()
	waitSignal(t, started)
}

func TestActorDisableAcceptsBeforePhysicalExitAndFencesOutput(t *testing.T) {
	started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer close(release)
	d, configs, output, _ := newActorDiscovery(t, Config{}, func(pipeline.Config) (sdPipeline, error) {
		return &controlledPipeline{
			started:  started,
			canceled: canceled,
			release:  release,
			groups:   []*confgroup.Group{{Source: "source", Configs: []confgroup.Config{{"name": "old"}}}},
		}, nil
	})
	configs <- prepareConfigFile("origin", "job")
	waitSignal(t, started)
	require.Eventually(t, func() bool {
		entry, ok := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":job")
		return ok && entry.Status == dyncfg.StatusRunning
	}, time.Second, time.Millisecond)
	applied := applyTestCommand(t, d, actorFunction(d, t.Context(), "job", "disable"))
	require.Equal(t, 200, applied.Result.Code)
	waitSignal(t, canceled)
	select {
	case groups := <-output:
		require.Empty(t, groups[0].Configs)
	case <-time.After(30 * time.Millisecond):
	}
	require.False(t, d.mgr.IsRunning("origin"))
}

func TestFinitePipelineRetainsSuccessfulSnapshot(t *testing.T) {
	exited := make(chan struct{})
	d, configs, output, _ := newActorDiscovery(t, Config{}, func(pipeline.Config) (sdPipeline, error) {
		return &controlledPipeline{
			finite:   true,
			canceled: exited,
			groups:   []*confgroup.Group{{Source: "finite", Configs: []confgroup.Config{{"name": "retained"}}}},
		}, nil
	})
	configs <- prepareConfigFile("origin", "job")
	waitSignal(t, exited)
	select {
	case groups := <-output:
		require.Len(t, groups[0].Configs, 1)
	case <-time.After(time.Second):
		t.Fatal("finite snapshot missing")
	}
	require.Eventually(t, func() bool {
		entry, ok := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":job")
		return ok && entry.Status == dyncfg.StatusRunning
	}, time.Second, time.Millisecond)
	require.True(t, d.mgr.IsRunning("origin"))
}

func TestContainmentCutsBlockedOutputOnActorSettlement(t *testing.T) {
	started := make(chan struct{})
	d, configs, output, _ := newActorDiscovery(t, Config{}, func(pipeline.Config) (sdPipeline, error) {
		return &controlledPipeline{
			started: started,
			groups:  []*confgroup.Group{{Source: "source", Configs: []confgroup.Config{{"name": "stale"}}}},
		}, nil
	})
	configs <- prepareConfigFile("origin", "job")
	waitSignal(t, started)
	require.Eventually(
		t,
		func() bool { d.mgr.mux.Lock(); defer d.mgr.mux.Unlock(); return len(d.mgr.output) > 0 },
		time.Second,
		time.Millisecond,
	)
	require.True(
		t,
		d.attempts.CutProcessAttempt(
			d.mgr.runtimeIdentity("logical", testDiscovererTypeNetListeners+":job"),
			context.Canceled,
		),
	)
	require.Eventually(t, func() bool {
		entry, ok := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":job")
		return ok && entry.Status == dyncfg.StatusFailed
	}, time.Second, time.Millisecond)
	select {
	case groups := <-output:
		require.Empty(t, groups[0].Configs)
	case <-time.After(time.Second):
		t.Fatal("removal missing")
	}
}

func TestRenamedFileWaitsForActualIncumbentExit(t *testing.T) {
	oldStarted, oldCanceled, release, newStarted := make(
		chan struct{},
	), make(
		chan struct{},
	), make(
		chan struct{},
	), make(
		chan struct{},
	)
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	d, configs, _, _ := newActorDiscovery(t, Config{}, func(cfg pipeline.Config) (sdPipeline, error) {
		if cfg.Name == "old" {
			return &controlledPipeline{
				started:  oldStarted,
				canceled: oldCanceled,
				release:  release,
			}, nil
		}
		return &controlledPipeline{
			started: newStarted,
		}, nil
	})
	configs <- prepareConfigFile("origin", "old")
	waitSignal(t, oldStarted)
	configs <- prepareConfigFile("origin", "new")
	waitSignal(t, oldCanceled)
	select {
	case <-newStarted:
		t.Fatal("replacement overlapped old physical runtime")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	waitSignal(t, newStarted)
	require.Eventually(t, func() bool {
		entry, ok := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":new")
		return ok && entry.Status == dyncfg.StatusRunning
	}, time.Second, time.Millisecond)
}

func TestFailedFileRenamePreservesIncumbentUntilFileRemoval(t *testing.T) {
	for _, occupied := range []bool{false, true} {
		t.Run(fmt.Sprint("occupied=", occupied), func(t *testing.T) {
			oldStarted, oldCanceled := make(chan struct{}), make(chan struct{})
			d, configs, _, _ := newActorDiscovery(t, Config{}, func(cfg pipeline.Config) (sdPipeline, error) {
				if cfg.Name == "old" {
					return &controlledPipeline{
						started:  oldStarted,
						canceled: oldCanceled,
					}, nil
				}
				return nil, errors.New("rename construction rejected")
			})
			configs <- prepareConfigFile("origin", "old")
			waitSignal(t, oldStarted)
			if occupied {
				cfg, err := newSDConfigFromYAML(
					prepareConfigFile("other", "new").content,
					"other",
					confgroup.TypeDyncfg,
					"other",
				)
				require.NoError(t, err)
				d.handler.AddDiscoveredConfig(cfg, dyncfg.StatusRunning)
			}
			configs <- prepareConfigFile("origin", "new")
			if !occupied {
				require.Eventually(t, func() bool {
					entry, ok := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":new")
					return ok && entry.Status == dyncfg.StatusFailed
				}, time.Second, time.Millisecond)
			}
			require.True(t, d.mgr.IsRunning("origin"))
			select {
			case <-oldCanceled:
				t.Fatal("failed rename stopped incumbent")
			default:
			}
			configs <- confFile{
				source: "origin",
			}
			waitSignal(t, oldCanceled)
		})
	}
}

// Delay alias acquisition independently of the actor to expose publication-pause
// ordering: the primary physical reservation must release before its event sends.
type gatedAliasAuthority struct {
	jobmgr.ProcessAttemptAuthority
	alias   jobmgr.ProcessAttemptIdentity
	entered chan struct{}
	proceed <-chan struct{}
	once    sync.Once
}

func (a *gatedAliasAuthority) StartProcessAttempt(
	ctx context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	if plan.Identity == a.alias {
		a.once.Do(func() { close(a.entered); <-a.proceed })
	}
	return a.ProcessAttemptAuthority.StartProcessAttempt(ctx, plan)
}
func TestBusyFileAliasReleasesLogicalReservationDuringPublicationPause(t *testing.T) {
	authority := newTestAttemptAuthority(t)
	alias := jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptServiceDiscovery,
		Key:       jobmgr.ProcessAttemptIdentityKey("sd-runtime-file", testPluginName, "origin"),
		Resource:  "service discovery pipeline",
	}
	occupied, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	guard, err := authority.StartProcessAttempt(
		t.Context(),
		jobmgr.ProcessAttemptPlan{
			Identity: alias,
			Target:   1,
			Work: func(_ context.Context, a jobmgr.ProcessAttemptAdmission) error {
				if err := a.Admit(); err != nil {
					return err
				}
				close(occupied)
				<-release
				return nil
			},
		},
	)
	require.NoError(t, err)
	waitSignal(t, occupied)
	entered, proceed := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-proceed:
		default:
			close(proceed)
		}
	}()
	wrapped := &gatedAliasAuthority{
		ProcessAttemptAuthority: authority,
		alias:                   alias,
		entered:                 entered,
		proceed:                 proceed,
	}
	started := make(chan struct{})
	d, configs, _, _ := newActorDiscovery(
		t,
		Config{
			Attempts: wrapped,
		},
		func(pipeline.Config) (sdPipeline, error) {
			return &controlledPipeline{
				started: started,
			}, nil
		},
	)
	configs <- prepareConfigFile("origin", "job")
	waitSignal(t, entered)
	// Pause the actor at a real prepared read of an unknown mutation target.
	p, err := d.prepareDyncfgCommand(actorFunction(d, t.Context(), "missing", "disable"))
	require.NoError(t, err)
	applied, err := p.Apply(t.Context())
	require.NoError(t, err)
	require.Equal(t, 404, applied.Result.Code)
	logical, ok := authority.ProcessAttemptReleased(
		d.mgr.runtimeIdentity("logical", testDiscovererTypeNetListeners+":job"),
	)
	require.True(t, ok)
	close(proceed)
	waitSignal(t, logical)
	select {
	case <-started:
		t.Fatal("started despite busy alias")
	default:
	}
	close(release)
	waitSignal(t, guard.Released())
	applied.Published()
	waitSignal(t, started)
}

func TestCanceledQueuedCommandDoesNotAdopt(t *testing.T) {
	d, _, _, _ := newActorDiscovery(
		t,
		Config{},
		func(pipeline.Config) (sdPipeline, error) { return &controlledPipeline{}, nil },
	)
	cfg, err := newSDConfigFromYAML(prepareConfigFile("origin", "job").content, "origin", confgroup.TypeUser, "origin")
	require.NoError(t, err)
	d.handler.AddDiscoveredConfig(cfg, dyncfg.StatusDisabled)
	first, err := d.prepareDyncfgCommand(actorFunction(d, t.Context(), "missing", "disable"))
	require.NoError(t, err)
	paused, err := first.Apply(t.Context())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	next, err := d.prepareDyncfgCommand(actorFunction(d, ctx, "job", "enable"))
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { _, err := next.Apply(ctx); done <- err }()
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	paused.Published()
	entry, ok := d.exposed.LookupByKey(cfg.ExposedKey())
	require.True(t, ok)
	require.Equal(t, dyncfg.StatusDisabled, entry.Status)
	require.False(t, d.mgr.IsRunning("origin"))
}

func TestRunRotationWaitsForProcessOwnedPhysicalExit(t *testing.T) {
	authority := newTestAttemptAuthority(t)
	started, canceled, release, nextStarted := make(
		chan struct{},
	), make(
		chan struct{},
	), make(
		chan struct{},
	), make(
		chan struct{},
	)
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	_, firstConfigs, _, stopFirst := newActorDiscovery(
		t,
		Config{
			Epoch:    1,
			Attempts: authority,
		},
		func(pipeline.Config) (sdPipeline, error) {
			return &controlledPipeline{
				started:  started,
				canceled: canceled,
				release:  release,
			}, nil
		},
	)
	firstConfigs <- prepareConfigFile("origin", "job")
	waitSignal(t, started)
	stopFirst()
	waitSignal(t, canceled)
	second, secondConfigs, _, _ := newActorDiscovery(
		t,
		Config{
			Epoch:    2,
			Attempts: authority,
		},
		func(pipeline.Config) (sdPipeline, error) {
			return &controlledPipeline{
				started: nextStarted,
			}, nil
		},
	)
	secondConfigs <- prepareConfigFile("origin", "renamed")
	require.Eventually(t, func() bool {
		second.mgr.mux.Lock()
		defer second.mgr.mux.Unlock()
		slot := second.mgr.pipelines["origin"]
		return slot != nil && slot.waiting
	}, time.Second, time.Millisecond)
	select {
	case <-nextStarted:
		t.Fatal("new epoch overlapped file origin")
	default:
	}
	close(release)
	waitSignal(t, nextStarted)
}

func TestUpdateGraceRetainsSourcesAndRemovesOnlyMissingAfterExpiry(t *testing.T) {
	started := make(chan struct{})
	var version atomic.Int32
	d, configs, output, _ := newActorDiscovery(t, Config{}, func(pipeline.Config) (sdPipeline, error) {
		if version.Add(1) == 1 {
			return &controlledPipeline{
				started: started,
				groups: []*confgroup.Group{
					{Source: "kept", Configs: []confgroup.Config{{"name": "old"}}},
					{Source: "gone", Configs: []confgroup.Config{{"name": "old"}}},
				},
			}, nil
		}
		return &controlledPipeline{
			groups: []*confgroup.Group{{Source: "kept", Configs: []confgroup.Config{{"name": "new"}}}},
		}, nil
	})
	// Dynamic input uses restart grace; file conversion intentionally removes old sources.
	cfg, err := newSDConfigFromYAML(
		prepareConfigFile("origin", "job").content,
		"origin",
		confgroup.TypeDyncfg,
		pipelineKey(testDiscovererTypeNetListeners, "job"),
	)
	require.NoError(t, err)
	d.handler.AddDiscoveredConfig(cfg, dyncfg.StatusDisabled)
	_ = configs
	applyTestCommand(t, d, actorFunction(d, t.Context(), "job", "enable"))
	waitSignal(t, started)
	for range 2 {
		select {
		case groups := <-output:
			require.Len(t, groups[0].Configs, 1)
		case <-time.After(time.Second):
			t.Fatal("initial snapshot missing")
		}
	}
	require.Eventually(t, func() bool {
		entry, _ := d.exposed.LookupByKey(cfg.ExposedKey())
		return entry.Status == dyncfg.StatusRunning
	}, time.Second, time.Millisecond)
	payload := []byte(
		`{"name":"job","discoverer":{"net_listeners":{"interval":"9s"}},"services":[{"id":"test-rule","match":"true"}]}`,
	)
	fn := dyncfg.NewFunction(
		t.Context(),
		functions.Function{
			UID:     "update",
			Args:    []string{d.dyncfgJobID(testDiscovererTypeNetListeners, "job"), "update"},
			Source:  "test",
			Payload: payload,
		},
	)
	applied := applyTestCommand(t, d, fn)
	require.Equal(t, 202, applied.Result.Code)
	select {
	case groups := <-output:
		require.Equal(t, "kept", groups[0].Source)
		require.Equal(t, "new", groups[0].Configs[0]["name"])
	case <-time.After(time.Second):
		t.Fatal("replacement snapshot missing")
	}
	select {
	case groups := <-output:
		t.Fatalf("source removed before grace expiry: %+v", groups)
	case <-time.After(20 * time.Millisecond):
	}
	// Advance the existing grace clock on actor; this test does not sleep a minute.
	request := sdActorCommand{
		ctx:    d.ctx,
		result: make(chan sdActorResult, 1),
		prepared: testActorPrepared{
			apply: func() {
				d.mgr.mux.Lock()
				d.mgr.pipelines[cfg.PipelineKey()].expires = time.Now().Add(-time.Second)
				d.mgr.mux.Unlock()
				d.mgr.processGracePeriodRemovals(d.ctx)
			},
		},
	}
	d.actorCommands <- request
	settled := <-request.result
	settled.applied.Published()
	select {
	case groups := <-output:
		require.Equal(t, "gone", groups[0].Source)
		require.Empty(t, groups[0].Configs)
	case <-time.After(time.Second):
		t.Fatal("expired missing source not removed")
	}
}

func TestWaitingMaterializationRetainsOnlyLatestDesired(t *testing.T) {
	entered, intermediateEntered := make(chan struct{}), make(chan struct{})
	release, intermediateRelease := make(chan struct{}), make(chan struct{})
	started, staleStarted := make(chan struct{}), make(chan struct{}, 2)
	initialReturned, intermediateReturned := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
		select {
		case <-intermediateRelease:
		default:
			close(intermediateRelease)
		}
	}()
	d, configs, _, _ := newActorDiscovery(t, Config{}, func(cfg pipeline.Config) (sdPipeline, error) {
		switch cfg.Services[0].ID {
		case "test-rule":
			close(entered)
			<-release
			defer close(initialReturned)
			return &recordingStartPipeline{
				started: staleStarted,
			}, nil
		case "intermediate":
			close(intermediateEntered)
			<-intermediateRelease
			defer close(intermediateReturned)
			return &recordingStartPipeline{
				started: staleStarted,
			}, nil
		default:
			return &controlledPipeline{
				started: started,
			}, nil
		}
	})
	initial := prepareConfigFile("origin", "job")
	configs <- initial
	waitSignal(t, entered)
	next := initial
	next.content = bytes.ReplaceAll(initial.content, []byte("id: test-rule"), []byte("id: intermediate"))
	configs <- next
	waitSignal(t, intermediateEntered)
	latest := initial
	latest.content = bytes.ReplaceAll(initial.content, []byte("id: test-rule"), []byte("id: latest"))
	configs <- latest
	waitSignal(t, started)
	close(release)
	close(intermediateRelease)
	waitSignal(t, initialReturned)
	waitSignal(t, intermediateReturned)
	require.Eventually(t, func() bool {
		entry, ok := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":job")
		return ok && entry.Status == dyncfg.StatusRunning && bytes.Contains(entry.Cfg.DataJSON(), []byte("latest"))
	}, time.Second, time.Millisecond)
	require.Never(t, func() bool { return len(staleStarted) > 0 }, 50*time.Millisecond, time.Millisecond)
}

// Records any actual runtime entry, independently of the manager's desired state.
type recordingStartPipeline struct{ started chan<- struct{} }

func (*recordingStartPipeline) Test(context.Context) (bool, error) { return false, nil }
func (p *recordingStartPipeline) Run(ctx context.Context, _ chan<- []*confgroup.Group) {
	p.started <- struct{}{}
	<-ctx.Done()
}

func TestUnacceptedUpdatePreservesPendingEnableConstruction(t *testing.T) {
	for _, cancelUpdate := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel-before-apply=%t", cancelUpdate), func(t *testing.T) {
			entered, release, started := make(chan struct{}), make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			var constructions atomic.Int32
			candidateStarted := make(chan struct{})
			d, _, _, _ := newActorDiscovery(t, Config{}, func(cfg pipeline.Config) (sdPipeline, error) {
				constructions.Add(1)
				if cfg.Services[0].ID == "replacement" {
					if !cancelUpdate {
						return nil, errors.New("replacement construction rejected")
					}
					return &controlledPipeline{
						started: candidateStarted,
					}, nil
				}
				close(entered)
				<-release
				return &controlledPipeline{
					started: started,
				}, nil
			})
			cfg, err := newSDConfigFromYAML(
				prepareConfigFile("origin", "job").content,
				"origin",
				confgroup.TypeDyncfg,
				pipelineKey(testDiscovererTypeNetListeners, "job"),
			)
			require.NoError(t, err)
			d.handler.AddDiscoveredConfig(cfg, dyncfg.StatusDisabled)
			enabled := applyTestCommand(t, d, actorFunction(d, t.Context(), "job", "enable"))
			require.Equal(t, 202, enabled.Result.Code)
			waitSignal(t, entered)
			expected, ok := d.exposed.LookupByKey(cfg.ExposedKey())
			require.True(t, ok)
			require.Equal(t, dyncfg.StatusAccepted, expected.Status)
			require.True(t, expected.Enabled)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			update := dyncfg.NewFunction(
				ctx,
				functions.Function{
					UID:    "update",
					Args:   []string{d.dyncfgJobID(testDiscovererTypeNetListeners, "job"), "update"},
					Source: "test",
					Payload: []byte(
						`{"name":"job","discoverer":{"net_listeners":{"interval":"9s"}},"services":[{"id":"replacement","match":"true"}]}`,
					),
				},
			)
			prepared, err := d.prepareDyncfgCommand(update)
			require.NoError(t, err)
			if cancelUpdate {
				cancel()
			}
			applied, err := prepared.Apply(ctx)
			if cancelUpdate {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, err)
				require.Equal(t, 422, applied.Result.Code)
				applied.Published()
			}
			close(release)
			waitSignal(t, started)
			require.Eventually(t, func() bool {
				entry, ok := d.exposed.LookupByKey(cfg.ExposedKey())
				return ok && entry.Status == dyncfg.StatusRunning && entry.Cfg.Hash() == cfg.Hash()
			}, time.Second, time.Millisecond)
			require.Equal(t, int32(2), constructions.Load())
			select {
			case <-candidateStarted:
				t.Fatal("unaccepted candidate ran")
			default:
			}
		})
	}
}
