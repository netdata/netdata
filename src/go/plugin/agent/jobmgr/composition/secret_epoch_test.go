// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func (es *processSecretEpochs) testLookup(
	generation uint64,
) (*processSecretEpoch, bool) {
	if es == nil || generation == 0 {
		return nil, false
	}
	es.mu.Lock()
	defer es.mu.Unlock()
	epoch, ok := es.epochs[generation]
	return epoch, ok
}

func TestProcessSecretEpochShutdownHonorsBudgetWithRetainedScope(t *testing.T) {
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	diagnostics := &recordingCompositionDiagnosticObserver{}
	epochs, err := newProcessSecretEpochs(resolver, diagnostics)
	require.NoError(t, err)
	epoch, err := epochs.create(1)
	require.NoError(t, err)
	catalog, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &processSecretStore{}
		},
	}})
	require.NoError(t, err)
	config := secretstore.Config{
		"name":            "main",
		"kind":            string(secretstore.KindVault),
		"value":           "retained",
		"__source__":      confgroup.TypeUser,
		"__source_type__": confgroup.TypeUser,
	}
	mutation, err := epoch.store.PrepareMutation(t.Context(), catalog, config, 0)
	require.NoError(t, err)
	result, err := mutation.Commit(t.Context())
	require.NoError(t, err)
	key := config.ExposedKey()
	scope, err := epoch.acquireScope([]string{key})
	require.NoError(t, err)
	require.NoError(t, epoch.store.Retire(t.Context(), key, result.Generation))

	shutdownCtx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = epochs.shutdown(shutdownCtx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), time.Second)

	events := diagnostics.snapshot()
	require.NotEmpty(t, events)
	require.Equal(t, "secret Store epoch retained at process shutdown", events[len(events)-1].Name)
	require.EqualValues(t, 1, events[len(events)-1].Generation)
	require.ErrorIs(t, events[len(events)-1].Err, context.DeadlineExceeded)

	require.NoError(t, scope.Release(t.Context()))
	select {
	case <-epoch.done():
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "retained Store epoch did not close after its scope drained")
	}
}

func TestProcessSecretEpochShutdownReportsBoundedDeterministicRetainedSample(t *testing.T) {
	const (
		total       = 10
		sampleLimit = 8
	)
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	diagnostics := &recordingCompositionDiagnosticObserver{}
	epochs, err := newProcessSecretEpochs(resolver, diagnostics)
	require.NoError(t, err)
	catalog, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &processSecretStore{}
		},
	}})
	require.NoError(t, err)
	retainedEpochs := make([]*processSecretEpoch, 0, total)
	retainedScopes := make([]secretresolver.AtomicScope, 0, total)
	for generation := uint64(1); generation <= total; generation++ {
		epoch, err := epochs.create(generation)
		require.NoError(t, err)
		config := secretstore.Config{
			"name":            fmt.Sprintf("main-%d", generation),
			"kind":            string(secretstore.KindVault),
			"value":           "retained",
			"__source__":      confgroup.TypeUser,
			"__source_type__": confgroup.TypeUser,
		}
		mutation, err := epoch.store.PrepareMutation(t.Context(), catalog, config, 0)
		require.NoError(t, err)
		result, err := mutation.Commit(t.Context())
		require.NoError(t, err)
		key := config.ExposedKey()
		scope, err := epoch.acquireScope([]string{key})
		require.NoError(t, err)
		require.NoError(t, epoch.store.Retire(t.Context(), key, result.Generation))
		retainedEpochs = append(retainedEpochs, epoch)
		retainedScopes = append(retainedScopes, scope)
	}
	shutdownCtx, cancel := context.WithCancel(t.Context())
	cancel()

	err = epochs.shutdown(shutdownCtx)

	require.ErrorIs(t, err, context.Canceled)
	events := diagnostics.snapshot()
	var aggregate *jobmgr.DiagnosticEvent
	var sampled []uint64
	for index := range events {
		event := &events[index]
		switch event.Name {
		case "secret Store epochs retained at process shutdown":
			aggregate = event
		case "secret Store epoch retained at process shutdown":
			sampled = append(sampled, event.Generation)
		}
	}
	require.NotNil(t, aggregate)
	require.Equal(t, total, aggregate.Count)
	require.Equal(t, []uint64{1, 2, 3, 4, 5, 6, 7, 8}, sampled)
	require.Len(t, sampled, sampleLimit)

	for _, scope := range retainedScopes {
		require.NoError(t, scope.Release(t.Context()))
	}
	for _, epoch := range retainedEpochs {
		select {
		case <-epoch.done():
		case <-time.After(time.Second):
			require.FailNow(t, "test failed", "retained Store epoch did not close after its scope drained")
		}
	}
}
