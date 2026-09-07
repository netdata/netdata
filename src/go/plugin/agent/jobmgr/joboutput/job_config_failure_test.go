// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

type failureLifecycleHook struct {
	*recordingJobConfigLifecycle
	mode       string
	observed   collectorapi.JobConfigFailure
	before     collectorapi.JobConfigLifecycleSnapshot
	reconciled collectorapi.JobConfigLifecycleSnapshot
}

func (h *failureLifecycleHook) ProjectFailure(
	snapshot collectorapi.JobConfigLifecycleSnapshot,
	f collectorapi.JobConfigFailure,
) collectorapi.JobConfigLifecycleSnapshot {
	h.observed, h.before = f, snapshot
	switch h.mode {
	case "panic":
		panic("failure projection panic")
	case "nil":
		return nil
	case "wrong_identity":
		return &recordingJobConfigLifecycleSnapshot{identity: collectorapi.JobConfigIdentity{1}}
	}
	cloned := *snapshot.(*recordingJobConfigLifecycleSnapshot)
	return &cloned
}

func (h *failureLifecycleHook) Reconcile(
	previous collectorapi.JobConfigIdentity,
	snapshot collectorapi.JobConfigLifecycleSnapshot,
	runtime collectorapi.RuntimeJob,
) {
	h.reconciled = snapshot
	h.recordingJobConfigLifecycle.Reconcile(previous, snapshot, runtime)
}

func TestJobConfigFailureEnrichesOnlyCommittedSnapshot(t *testing.T) {
	tests := map[string]struct {
		mode               string
		beforeConstruction bool
	}{
		"failed check":     {},
		"missing vnode":    {beforeConstruction: true},
		"projection panic": {mode: "panic"},
		"nil projection":   {mode: "nil"},
		"wrong identity":   {mode: "wrong_identity"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			events := []string{}
			hook := &failureLifecycleHook{recordingJobConfigLifecycle: &recordingJobConfigLifecycle{events: &events}, mode: tc.mode}
			creator := controller.modules["module"]
			creator.JobConfigLifecycle = hook
			creator.Create = func() collectorapi.CollectorV1 {
				return &collectorapi.MockCollectorV1{
					CheckFunc: func(context.Context) error { return errors.New("private Check error SECRET") },
				}
			}
			controller.modules["module"] = creator
			config := factoryTestConfig(false)
			config.SetSourceType(confgroup.TypeDyncfg)
			config.SetSource("user=test")
			config.SetProvider("test")
			if tc.beforeConstruction {
				config.Set("vnode", "missing-vnode")
			}
			seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusRunning)
			currentID := lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 1}
			current := &transactionTestReadyResource{identity: currentID, prefix: "current", events: &events}
			permit, _ := issueTestJobPermit(t, config.FullName(), 2)
			scope := lifecycle.ResourceTransactionScope{
				ID:        config.FullName(),
				Current:   currentID,
				Successor: lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 2},
			}
			transaction, err := controller.prepareDiscovered(
				context.Background(),
				DiscoveredJobChange{Config: config, Status: dyncfg.StatusRunning, Restart: true},
				current,
				scope,
				permit,
			)
			require.NoError(t, err)
			require.Nil(t, hook.reconciled)
			require.Empty(t, hook.reconciliations)
			require.True(t, hook.observed.Valid())
			if tc.beforeConstruction {
				require.Equal(t, "vnode", hook.observed.Stage)
				require.Equal(t, "unavailable", hook.observed.Reason)
				require.NotContains(t, events, "capture")
			} else {
				require.Equal(t, "autodetection", hook.observed.Stage)
				require.Contains(t, events, "capture")
				require.NotContains(t, events, "project", "failed Check evidence must not be replaced by a baseline")
			}
			_, err = transaction.Apply(context.Background())
			require.NoError(t, err)
			require.Equal(t, jobConfigIdentity(config), hook.reconciled.Identity())
			if tc.mode != "" {
				require.Same(t, hook.before, hook.reconciled, "bad enrichment must preserve the valid captured snapshot")
			} else {
				require.NotSame(t, hook.before, hook.reconciled)
			}
			data, err := json.Marshal(hook.observed)
			require.NoError(t, err)
			require.NotContains(t, string(data), "SECRET")
		})
	}
}

func TestPreparationFailureSurvivesSecretRedactionWithoutRetainingError(t *testing.T) {
	original := withJobConfigFailure(
		&secretresolver.AtomicResolveError{Kind: secretresolver.AtomicErrorProvider, Cause: errors.New("credential SECRET")},
		"configuration",
		"",
	)
	redacted := redactResolvedLifecycleError(original)
	require.NotContains(t, redacted.Error(), "SECRET")
	var tagged *jobConfigPreparationError
	require.ErrorAs(t, redacted, &tagged)
	require.NotContains(t, tagged.cause.Error(), "SECRET")
	f := jobConfigFailure(redacted, "construction")
	require.Equal(t, "secret_resolution", f.Stage)
	require.Equal(t, "provider", f.Reason)
	require.True(t, f.Valid())
}
