// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/require"
)

type streamingTestPipeline struct {
	updates  <-chan []*confgroup.Group
	panicNow <-chan struct{}
}

func (*streamingTestPipeline) Test(context.Context) (bool, error) { return false, nil }
func (p *streamingTestPipeline) Run(ctx context.Context, out chan<- []*confgroup.Group) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.panicNow:
			panic("runtime failed")
		case groups := <-p.updates:
			select {
			case <-ctx.Done():
				return
			case out <- groups:
			}
		}
	}
}

func TestSourceRemovalSurvivesRetirementWithoutRetainingMembership(t *testing.T) {
	updates := make(chan []*confgroup.Group)
	d, configs, output, _ := newActorDiscovery(t, Config{}, func(pipeline.Config) (sdPipeline, error) {
		return &streamingTestPipeline{
			updates: updates,
		}, nil
	})
	configs <- prepareConfigFile("origin", "job")
	send := func(group *confgroup.Group) {
		t.Helper()
		select {
		case updates <- []*confgroup.Group{group}:
		case <-time.After(time.Second):
			t.Fatal("pipeline did not accept update")
		}
	}
	send(&confgroup.Group{
		Source:  "gone",
		Configs: []confgroup.Config{{"name": "old"}},
	})
	select {
	case groups := <-output:
		require.NotEmpty(t, groups[0].Configs)
	case <-time.After(time.Second):
		t.Fatal("initial source was not published")
	}
	send(&confgroup.Group{
		Source: "gone",
	})
	// Output stays blocked while the actor must forget active membership, retain
	// the pending removal, and continue processing commands.
	require.Eventually(t, func() bool {
		d.mgr.mux.Lock()
		defer d.mgr.mux.Unlock()
		return len(d.mgr.pipelines["origin"].sources) == 0 && len(d.mgr.output["gone"]) == 1
	}, time.Second, time.Millisecond)
	require.Equal(t, 200, applyTestCommand(t, d, actorFunction(d, t.Context(), "job", "disable")).Result.Code)
	select {
	case groups := <-output:
		require.Equal(t, "gone", groups[0].Source)
		require.Empty(t, groups[0].Configs)
	case <-time.After(time.Second):
		t.Fatal("retirement lost the queued removal")
	}
	// Recreating the source after the removal must still reach consumers.
	require.Equal(t, 202, applyTestCommand(t, d, actorFunction(d, t.Context(), "job", "enable")).Result.Code)
	send(&confgroup.Group{
		Source:  "gone",
		Configs: []confgroup.Config{{"name": "new"}},
	})
	select {
	case groups := <-output:
		require.Equal(t, "new", groups[0].Configs[0]["name"])
	case <-time.After(time.Second):
		t.Fatal("recreated source was not published")
	}
}

func TestRenamedIncumbentFailureRemovesItsSources(t *testing.T) {
	updates, panicNow := make(chan []*confgroup.Group), make(chan struct{})
	d, configs, output, _ := newActorDiscovery(t, Config{}, func(cfg pipeline.Config) (sdPipeline, error) {
		if cfg.Name == "new" {
			return nil, errors.New("rename rejected")
		}
		return &streamingTestPipeline{
			updates:  updates,
			panicNow: panicNow,
		}, nil
	})
	configs <- prepareConfigFile("origin", "old")
	select {
	case updates <- []*confgroup.Group{{Source: "old-source", Configs: []confgroup.Config{{"name": "old"}}}}:
	case <-time.After(time.Second):
		t.Fatal("incumbent did not start")
	}
	select {
	case <-output:
	case <-time.After(time.Second):
		t.Fatal("incumbent did not publish")
	}
	configs <- prepareConfigFile("origin", "new")
	require.Eventually(t, func() bool {
		e, ok := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":new")
		return ok && e.Status == dyncfg.StatusFailed
	}, time.Second, time.Millisecond)
	close(panicNow)
	select {
	case groups := <-output:
		require.Equal(t, "old-source", groups[0].Source)
		require.Empty(t, groups[0].Configs)
	case <-time.After(time.Second):
		t.Fatal("failed incumbent retained published sources after rename")
	}
	require.False(t, d.mgr.IsRunning("origin"))
	e, ok := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":new")
	require.True(t, ok)
	require.Equal(t, dyncfg.StatusFailed, e.Status)
}

// Failed rename preserves the incumbent until another desired owner takes its name.
func TestFailedRenameAllowsFileTakeoverOfIncumbentName(t *testing.T) {
	incumbentStarted := make(chan struct{})
	incumbentCanceled := make(chan struct{})
	replacementStarted := make(chan struct{})
	var olds atomic.Int32
	d, configs, _, _ := newActorDiscovery(t, Config{}, func(cfg pipeline.Config) (sdPipeline, error) {
		switch cfg.Name {
		case "new":
			return nil, errors.New("rename construction rejected")
		case "old":
			if olds.Add(1) == 1 {
				return &controlledPipeline{
					started:  incumbentStarted,
					canceled: incumbentCanceled,
				}, nil
			}
			return &controlledPipeline{
				started: replacementStarted,
			}, nil
		}
		return nil, errors.New("unexpected")
	})
	configs <- prepareConfigFile("origin", "old")
	waitSignal(t, incumbentStarted)
	configs <- prepareConfigFile("origin", "new")
	require.Eventually(t, func() bool {
		e, ok := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":new")
		return ok && e.Status == dyncfg.StatusFailed
	}, time.Second, time.Millisecond)
	require.True(t, d.mgr.IsRunning("origin"), "incumbent preserved")

	// Another file now provides the old name.
	configs <- prepareConfigFile("other", "old")
	require.Eventually(t, func() bool {
		e, ok := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":old")
		return ok && e.Cfg.Source() == "other"
	}, time.Second, time.Millisecond)

	select {
	case <-replacementStarted:
	case <-time.After(2 * time.Second):
		d.mgr.mux.Lock()
		slot := d.mgr.pipelines["other"]
		waiting := slot != nil && slot.waiting
		d.mgr.mux.Unlock()
		e, _ := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":old")
		select {
		case <-incumbentCanceled:
			t.Logf("incumbent was canceled")
		default:
			t.Logf("incumbent still running")
		}
		t.Fatalf("new owner of 'old' never started: slot waiting=%v status=%s origin running=%v",
			waiting, e.Status, d.mgr.IsRunning("origin"))
	}
}

func TestFailedRenameAllowsDyncfgTakeoverOfIncumbentName(t *testing.T) {
	incumbentStarted, replacementStarted := make(chan struct{}), make(chan struct{})
	var olds atomic.Int32
	d, configs, _, _ := newActorDiscovery(t, Config{}, func(cfg pipeline.Config) (sdPipeline, error) {
		switch cfg.Name {
		case "new":
			return nil, errors.New("rename construction rejected")
		case "old":
			if olds.Add(1) == 1 {
				return &controlledPipeline{
					started: incumbentStarted,
				}, nil
			}
			return &controlledPipeline{
				started: replacementStarted,
			}, nil
		}
		return nil, errors.New("unexpected")
	})
	configs <- prepareConfigFile("origin", "old")
	waitSignal(t, incumbentStarted)
	configs <- prepareConfigFile("origin", "new")
	require.Eventually(t, func() bool {
		e, ok := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":new")
		return ok && e.Status == dyncfg.StatusFailed
	}, time.Second, time.Millisecond)
	payload, _ := json.Marshal(newTestNetListenersConfig("old", 0, 0, defaultTestServices()))
	add := dyncfg.NewFunction(t.Context(), functions.Function{
		UID:     "add",
		Args:    []string{d.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "old"},
		Source:  "user=test",
		Payload: payload,
	})
	require.Equal(t, 202, applyTestCommand(t, d, add).Result.Code)
	require.Equal(t, 202, applyTestCommand(t, d, actorFunction(d, t.Context(), "old", "enable")).Result.Code)
	select {
	case <-replacementStarted:
	case <-time.After(2 * time.Second):
		e, _ := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":old")
		t.Fatalf("DynCfg 'old' never started: status=%s origin running=%v", e.Status, d.mgr.IsRunning("origin"))
	}
}

func TestIncumbentNameTakeoverPreservesPendingRename(t *testing.T) {
	incumbentStarted, incumbentCanceled, exit := make(chan struct{}), make(chan struct{}), make(chan struct{})
	renameEntered, prepare, renameStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
	replacementStarted := make(chan struct{})
	defer func() {
		select {
		case <-exit:
		default:
			close(exit)
		}
		select {
		case <-prepare:
		default:
			close(prepare)
		}
	}()
	var oldConstructions atomic.Int32
	d, configs, _, _ := newActorDiscovery(t, Config{}, func(cfg pipeline.Config) (sdPipeline, error) {
		if cfg.Name == "new" {
			close(renameEntered)
			<-prepare
			return &controlledPipeline{
				started: renameStarted,
			}, nil
		}
		if oldConstructions.Add(1) == 1 {
			return &controlledPipeline{
				started:  incumbentStarted,
				canceled: incumbentCanceled,
				release:  exit,
			}, nil
		}
		return &controlledPipeline{
			started: replacementStarted,
		}, nil
	})
	configs <- prepareConfigFile("origin", "old")
	waitSignal(t, incumbentStarted)
	configs <- prepareConfigFile("origin", "new")
	waitSignal(t, renameEntered)
	configs <- prepareConfigFile("other", "old")
	waitSignal(t, incumbentCanceled)
	select {
	case <-replacementStarted:
		t.Fatal("takeover overlapped physical incumbent")
	default:
	}
	close(exit)
	waitSignal(t, replacementStarted)
	close(prepare)
	waitSignal(t, renameStarted)
	require.Eventually(t, func() bool {
		entry, ok := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":new")
		return ok && entry.Status == dyncfg.StatusRunning
	}, time.Second, time.Millisecond)
}

type sdDiagnosticRecorder struct {
	mu        sync.Mutex
	events    []jobmgr.DiagnosticEvent
	contained chan jobmgr.DiagnosticEvent
}

func (o *sdDiagnosticRecorder) ObserveDiagnostic(e jobmgr.DiagnosticEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, e)
	if e.Name == "job manager attempt contained" {
		select {
		case o.contained <- e:
		default:
		}
	}
}
func (o *sdDiagnosticRecorder) errorsNamed(name string) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := 0
	for _, e := range o.events {
		if e.Name == name && e.Level == jobmgr.DiagnosticError {
			n++
		}
	}
	return n
}

// Routine logical retirement is not an exceptional containment event.
func TestRoutineDisableDoesNotReportContainmentError(t *testing.T) {
	obs := &sdDiagnosticRecorder{}
	authority, err := containment.NewAuthority(obs)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = authority.Shutdown(ctx)
	})
	started, canceled := make(chan struct{}), make(chan struct{})
	d, configs, _, _ := newActorDiscovery(t, Config{
		Attempts: authority,
	}, func(pipeline.Config) (sdPipeline, error) {
		return &controlledPipeline{
			started:  started,
			canceled: canceled,
		}, nil
	})
	configs <- prepareConfigFile("origin", "job")
	waitSignal(t, started)
	require.Eventually(t, func() bool {
		e, ok := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":job")
		return ok && e.Status == dyncfg.StatusRunning
	}, time.Second, time.Millisecond)
	before := obs.errorsNamed("job manager attempt contained")
	applied := applyTestCommand(t, d, actorFunction(d, t.Context(), "job", "disable"))
	require.Equal(t, 200, applied.Result.Code)
	waitSignal(t, canceled)
	require.Eventually(
		t,
		func() bool { return authority.Census() == (containment.Census{}) },
		time.Second,
		time.Millisecond,
	)
	after := obs.errorsNamed("job manager attempt contained")
	t.Logf("error-level containment diagnostics on routine disable: %d", after-before)
	require.Zero(t, after-before, "routine disable produced error-level containment diagnostics")
}

// A valid replacement can prepare without revoking or waiting for the incumbent constructor.
func TestUpdateSupersedesConstructingRevision(t *testing.T) {
	entered, release, nextStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer close(release)
	var replacementConstructions atomic.Int32
	d, _, _, _ := newActorDiscovery(t, Config{}, func(cfg pipeline.Config) (sdPipeline, error) {
		if cfg.Services[0].ID == "initial" {
			close(entered)
			<-release
			return &controlledPipeline{}, nil
		}
		replacementConstructions.Add(1)
		return &controlledPipeline{
			started: nextStarted,
		}, nil
	})
	add := dyncfg.NewFunction(t.Context(), functions.Function{
		UID:    "add",
		Args:   []string{d.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "job"},
		Source: "user=test",
		Payload: []byte(
			`{"name":"job","discoverer":{"net_listeners":{}},"services":[{"id":"initial","match":"true"}]}`,
		),
	})
	require.Equal(t, 202, applyTestCommand(t, d, add).Result.Code)
	require.Equal(t, 202, applyTestCommand(t, d, actorFunction(d, t.Context(), "job", "enable")).Result.Code)
	waitSignal(t, entered)
	update := dyncfg.NewFunction(t.Context(), functions.Function{
		UID:    "update",
		Args:   []string{d.dyncfgJobID(testDiscovererTypeNetListeners, "job"), "update"},
		Source: "user=test",
		Payload: []byte(
			`{"name":"job","discoverer":{"net_listeners":{}},"services":[{"id":"replacement","match":"true"}]}`,
		),
	})
	applied := applyTestCommand(t, d, update)
	t.Logf(
		"UPDATE status=%d; replacement constructors entered=%d",
		applied.Result.Code,
		replacementConstructions.Load(),
	)
	require.Equal(
		t,
		202,
		applied.Result.Code,
		"valid replacement cannot be blocked solely by the incumbent revision's construction",
	)
	waitSignal(t, nextStarted)
}

func TestRoutineConstructionRetirementPreservesPhysicalOwnership(t *testing.T) {
	for _, action := range []string{"disable", "supersede", "shutdown"} {
		t.Run(action, func(t *testing.T) {
			obs := &sdDiagnosticRecorder{
				contained: make(chan jobmgr.DiagnosticEvent, 8),
			}
			authority, err := containment.NewAuthority(obs)
			require.NoError(t, err)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_ = authority.Shutdown(ctx)
			})
			entered, release := make(chan struct{}), make(chan struct{})
			var released bool
			t.Cleanup(func() {
				if !released {
					close(release)
				}
			})
			staleStarted := make(chan struct{}, 1)
			d, _, _, shutdown := newActorDiscovery(
				t,
				Config{
					Attempts: authority,
				},
				func(cfg pipeline.Config) (sdPipeline, error) {
					if cfg.Services[0].ID == "initial" {
						close(entered)
						<-release
						return &recordingStartPipeline{
							started: staleStarted,
						}, nil
					}
					return &controlledPipeline{}, nil
				},
			)
			add := dyncfg.NewFunction(t.Context(), functions.Function{
				UID:    "add",
				Args:   []string{d.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "job"},
				Source: "user=test",
				Payload: []byte(
					`{"name":"job","discoverer":{"net_listeners":{}},"services":[{"id":"initial","match":"true"}]}`,
				),
			})
			require.Equal(t, 202, applyTestCommand(t, d, add).Result.Code)
			require.Equal(t, 202, applyTestCommand(t, d, actorFunction(d, t.Context(), "job", "enable")).Result.Code)
			waitSignal(t, entered)
			entry, ok := d.exposed.LookupByKey(testDiscovererTypeNetListeners + ":job")
			require.True(t, ok)
			physicalDone, ok := authority.ProcessAttemptReleased(pipelineMaterializationIdentity(entry.Cfg))
			require.True(t, ok)
			switch action {
			case "disable":
				require.Equal(
					t,
					200,
					applyTestCommand(t, d, actorFunction(d, t.Context(), "job", "disable")).Result.Code,
				)
			case "supersede":
				update := dyncfg.NewFunction(t.Context(), functions.Function{
					UID:    "update",
					Args:   []string{d.dyncfgJobID(testDiscovererTypeNetListeners, "job"), "update"},
					Source: "user=test",
					Payload: []byte(
						`{"name":"job","discoverer":{"net_listeners":{}},"services":[{"id":"replacement","match":"true"}]}`,
					),
				})
				require.Equal(t, 202, applyTestCommand(t, d, update).Result.Code)
			case "shutdown":
				shutdown()
			}
			select {
			case event := <-obs.contained:
				require.Equal(t, jobmgr.DiagnosticInfo, event.Level, "routine construction retirement")
				require.ErrorIs(t, event.Err, jobmgr.ErrProcessAttemptRetired)
			case <-time.After(time.Second):
				t.Fatal("construction was not logically retired")
			}
			select {
			case <-physicalDone:
				t.Fatal("physical ownership released before constructor returned")
			default:
			}
			close(release)
			released = true
			waitSignal(t, physicalDone)
			require.Empty(t, staleStarted, "retired constructor must never activate")
		})
	}
}
