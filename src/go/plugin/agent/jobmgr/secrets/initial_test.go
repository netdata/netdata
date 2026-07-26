// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"bytes"
	"io"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestTemplatePublicationAcceptsReplayBeforeFrameBecomesVisible(t *testing.T) {
	resourceID := secretResourceID("vault:replay")
	successor := lifecycle.ResourceIdentity{ID: resourceID, Generation: 1}
	var controller *Controller
	var permit lifecycle.LongLivedPermit
	var owned lifecycle.ReadyResource
	status := 0
	writer := secretTestWriteFunc(func(payload []byte) (int, error) {
		if bytes.Contains(payload, []byte("CONFIG go.d:secretstore:vault create accepted template")) {
			transaction, err := controller.Prepare(
				t.Context(),
				CommandInput{
					Args:        []string{"go.d:secretstore:vault", "add", "replay"},
					Payload:     []byte(`{"value":"restored"}`),
					ContentType: "application/json",
					HasPayload:  true,
				},
				nil,
				lifecycle.ResourceTransactionScope{
					ID:        resourceID,
					Successor: successor,
				},
				permit,
			)
			require.NoError(t, err)
			applied, err := transaction.Apply(t.Context())
			require.NoError(t, err)
			_, _, owned = applied.Ownership()
			status = applied.ResultStatus()
		}
		return len(payload), nil
	})
	var store *secretstore.SecretStore
	var supervisor *lifecycle.TaskSupervisor
	controller, store, supervisor = newSecretControllerTestHarnessWithWriter(t, nil, writer)
	require.NoError(t, controller.Bind(restartTestJobs{}))
	issuedPermit, err := supervisor.IssueLongLivedPermit(successor, lifecycle.NewSecretStoreLongLivedPlan())
	require.NoError(t, err)
	permit = issuedPermit

	require.NoError(t, controller.templateCleanup()())
	if owned != nil {
		require.NoError(t, owned.Finalize())
	}
	require.NoError(t, store.Close(t.Context()))
	require.Equal(t, 200, status)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, supervisor.LongLivedCensus())
}

func TestConfigPublicationPreservesWindowsSourcePath(t *testing.T) {
	const source = `file=C:\Program Files\Netdata\etc\netdata\ss\vault.conf`
	var output bytes.Buffer
	controller, store, _ := newSecretControllerTestHarnessWithWriter(t, nil, &output)
	config := secretstore.Config{
		"name":            "main",
		"kind":            string(secretstore.KindVault),
		"__source__":      source,
		"__source_type__": confgroup.TypeUser,
	}

	require.NoError(t, controller.configCreateCleanup(secretEntry{
		config: config,
		status: dyncfg.StatusRunning,
	})())
	require.Contains(t, output.String(), `'`+source+`'`)
	require.NoError(t, store.Close(t.Context()))
}

func TestSecretAddCollisionIsReplayUpsert(t *testing.T) {
	controller, store, supervisor := newSecretControllerTestHarness(t, nil)
	require.NoError(t, controller.Bind(restartTestJobs{}))
	controller.setCommandsReady(true)

	existing := secretTestConfig(confgroup.TypeUser, "original")
	carrier := &transactionTestCarrier{}
	mutation, err := store.PrepareMutation(t.Context(), controller.creators, carrier, existing, 0)
	require.NoError(t, err)
	result, err := mutation.Commit(t.Context())
	require.NoError(t, err)
	require.True(t, result.Applied)
	controller.commitEntry(existing.ExposedKey(), &secretEntry{
		config: existing,
		status: dyncfg.StatusRunning,
	})

	resourceID := secretResourceID(existing.ExposedKey())
	currentIdentity := lifecycle.ResourceIdentity{ID: resourceID, Generation: 1}
	current, err := newStoreGenerationResource(
		currentIdentity,
		store,
		existing.ExposedKey(),
		result.Generation,
	)
	require.NoError(t, err)
	successorIdentity := lifecycle.ResourceIdentity{ID: resourceID, Generation: 2}
	permit, err := supervisor.IssueLongLivedPermit(
		successorIdentity,
		lifecycle.NewSecretStoreLongLivedPlan(),
	)
	require.NoError(t, err)

	add := CommandInput{
		Args:        []string{"go.d:secretstore:vault", "add", "main"},
		Payload:     []byte(`{"value":"replacement"}`),
		ContentType: "application/json",
		HasPayload:  true,
	}
	transaction, err := controller.Prepare(
		t.Context(),
		add,
		current,
		lifecycle.ResourceTransactionScope{
			ID:        resourceID,
			Current:   currentIdentity,
			Successor: successorIdentity,
		},
		permit,
	)
	require.NoError(t, err)
	applied, err := transaction.Apply(t.Context())
	require.NoError(t, err)
	_, disposition, owned := applied.Ownership()
	active, ok := store.Config(existing.ExposedKey())
	require.True(t, ok)
	require.NotNil(t, owned)
	require.Equal(t, 200, applied.ResultStatus())
	require.Equal(t, lifecycle.ResourceTransactionReplaced, disposition)
	require.Equal(t, confgroup.TypeDyncfg, active.SourceType())
	require.Equal(t, "replacement", active["value"])

	repeatSuccessor := lifecycle.ResourceIdentity{ID: resourceID, Generation: 3}
	repeatPermit, err := supervisor.IssueLongLivedPermit(
		repeatSuccessor,
		lifecycle.NewSecretStoreLongLivedPlan(),
	)
	require.NoError(t, err)
	repeat, err := controller.Prepare(
		t.Context(),
		add,
		owned,
		lifecycle.ResourceTransactionScope{
			ID:        resourceID,
			Current:   successorIdentity,
			Successor: repeatSuccessor,
		},
		repeatPermit,
	)
	require.NoError(t, err)
	repeated, err := repeat.Apply(t.Context())
	require.NoError(t, err)
	_, repeatDisposition, repeatOwned := repeated.Ownership()
	require.Equal(t, 200, repeated.ResultStatus())
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, repeatDisposition)
	require.Same(t, owned, repeatOwned)

	require.NoError(t, repeatOwned.Finalize())
	require.NoError(t, store.Close(t.Context()))
	require.EqualValues(t, lifecycle.LongLivedCensus{}, supervisor.LongLivedCensus())
}

func newSecretControllerTestHarness(
	t *testing.T,
	initial []secretstore.Config,
) (*Controller, *secretstore.SecretStore, *lifecycle.TaskSupervisor) {
	return newSecretControllerTestHarnessWithWriter(t, initial, io.Discard)
}

func newSecretControllerTestHarnessWithWriter(
	t *testing.T,
	initial []secretstore.Config,
	writer io.Writer,
) (*Controller, *secretstore.SecretStore, *lifecycle.TaskSupervisor) {
	t.Helper()
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	store, err := secretstore.NewSecretStore(resolver)
	require.NoError(t, err)
	catalog, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &transactionTestStore{}
		},
	}})
	require.NoError(t, err)
	frames, err := lifecycle.NewFrameOwner(writer)
	require.NoError(t, err)
	supervisor, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
	controller, err := NewController(ControllerConfig{
		Epoch:        1,
		PluginName:   "go.d",
		Frames:       frames,
		Store:        store,
		Creators:     catalog,
		Dependencies: NewSecretDependencyIndex(),
		Initial:      initial,
	})
	require.NoError(t, err)
	return controller, store, supervisor
}

type secretTestWriteFunc func([]byte) (int, error)

func (fn secretTestWriteFunc) Write(payload []byte) (int, error) {
	return fn(payload)
}

func secretTestConfig(sourceType, value string) secretstore.Config {
	return secretstore.Config{
		"name":            "main",
		"kind":            string(secretstore.KindVault),
		"value":           value,
		"__source__":      sourceType + "=test",
		"__source_type__": sourceType,
	}
}
