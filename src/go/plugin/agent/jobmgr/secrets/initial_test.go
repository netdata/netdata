// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
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
	var owned lifecycle.ReadyResource
	status := 0
	writer := secretTestWriteFunc(func(payload []byte) (int, error) {
		if bytes.Contains(payload, []byte("CONFIG go.d:secretstore:vault create accepted template")) {
			input := CommandInput{
				Args:        []string{"go.d:secretstore:vault", "add", "replay"},
				Payload:     []byte(`{"value":"restored"}`),
				ContentType: "application/json",
				HasPayload:  true,
			}
			stage, err := controller.Stage(input)
			require.NoError(t, err)
			stage.Start()
			<-stage.Ready()
			defer stage.Release()
			transaction, err := controller.PrepareStaged(
				t.Context(),
				input,
				nil,
				lifecycle.ResourceTransactionScope{
					ID:        resourceID,
					Successor: successor,
				},
				lifecycle.LongLivedPermit{},
				stage,
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
	controller, store = newSecretControllerTestHarnessWithWriter(t, nil, writer)
	require.NoError(t, controller.Bind(restartTestJobs{}))

	require.NoError(t, controller.templateCleanup()())
	if owned != nil {
		require.NoError(t, owned.Finalize())
	}
	require.NoError(t, store.Close(t.Context()))
	require.Equal(t, 200, status)
}

func TestConfigPublicationPreservesWindowsSourcePath(t *testing.T) {
	const source = `file=C:\Program Files\Netdata\etc\netdata\ss\vault.conf`
	var output bytes.Buffer
	controller, store := newSecretControllerTestHarnessWithWriter(t, nil, &output)
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

func TestInitialStoreMaterializationStartsDifferentIdentitiesConcurrently(t *testing.T) {
	gate := newInitialStoreMaterializationGate()
	t.Cleanup(gate.release)
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	store, err := secretstore.NewSecretStore(resolver)
	require.NoError(t, err)
	catalog, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &initialStoreMaterializationStore{gate: gate}
		},
	}})
	require.NoError(t, err)
	frames, err := lifecycle.NewFrameOwner(io.Discard)
	require.NoError(t, err)
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	operations, err := NewStoreOperations(StoreOperationsConfig{
		Epoch:    1,
		Attempts: attempts,
		Store:    store,
		Creators: catalog,
	})
	require.NoError(t, err)
	controller, err := NewController(ControllerConfig{
		Epoch:        1,
		PluginName:   "go.d",
		Frames:       frames,
		Store:        store,
		Operations:   operations,
		Creators:     catalog,
		Dependencies: NewSecretDependencyIndex(),
		Initial: []secretstore.Config{
			secretInitialMaterializationConfig("a-blocked", "blocked"),
			secretInitialMaterializationConfig("z-fast", "fast"),
		},
	})
	require.NoError(t, err)
	require.NoError(t, controller.Bind(restartTestJobs{}))
	commands := &initialStoreTestCommands{
		publishTemplates: controller.templateCleanup(),
	}
	published := make(chan error, 1)
	go func() {
		published <- controller.PublishInitial(context.Background(), commands)
	}()

	select {
	case <-gate.blocked:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "blocking initial Store did not enter Init")
	}
	fastBeforeRelease := false
	select {
	case <-gate.fast:
		fastBeforeRelease = true
	case <-time.After(300 * time.Millisecond):
	}

	gate.release()
	select {
	case err := <-published:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "initial Store publication did not complete")
	}
	require.True(t, fastBeforeRelease, "initial Store identities were materialized serially")
	require.NoError(t, controller.CloseProjection())
	attempts.BeginShutdown()
	require.NoError(t, attempts.Shutdown(t.Context()))
	require.NoError(t, store.Close(t.Context()))
}

func TestInitialStoreConfigsSelectHighestPriorityWinnerBeforeMaterialization(t *testing.T) {
	stock := secretTestConfig(confgroup.TypeStock, "stock")
	user := secretTestConfig(confgroup.TypeUser, "user")

	selected := selectInitialConfigs([]secretstore.Config{stock, user})

	require.Len(t, selected, 1)
	require.Equal(t, confgroup.TypeUser, selected[0].SourceType())
	require.Equal(t, "user", selected[0]["value"])
}

func TestPreparedStoreOperationRetainsIdentityUntilStageRelease(t *testing.T) {
	var initializations atomic.Int32
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	store, err := secretstore.NewSecretStore(resolver)
	require.NoError(t, err)
	catalog, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &countingStoreOperationProvider{initializations: &initializations}
		},
	}})
	require.NoError(t, err)
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	operations, err := NewStoreOperations(StoreOperationsConfig{
		Epoch:    1,
		Attempts: attempts,
		Store:    store,
		Creators: catalog,
	})
	require.NoError(t, err)

	target := secretTarget{
		command: dyncfg.CommandAdd,
		key:     secretstore.StoreKey(secretstore.KindVault, "main"),
		kind:    secretstore.KindVault,
		name:    "main",
	}
	first, err := operations.prepare(storeOperationSpec{
		target:    target,
		config:    secretTestConfig(confgroup.TypeUser, "first"),
		mode:      storeOperationMutation,
		supersede: true,
	})
	require.NoError(t, err)
	second, err := operations.prepare(storeOperationSpec{
		target:    target,
		config:    secretTestConfig(confgroup.TypeUser, "second"),
		mode:      storeOperationMutation,
		supersede: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		second.Cancel(context.Canceled)
		second.Release()
		first.Release()
		attempts.BeginShutdown()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, attempts.Shutdown(shutdownCtx))
		require.NoError(t, store.Close(context.Background()))
	})

	first.Start()
	select {
	case <-first.Ready():
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "first Store operation did not become ready")
	}
	require.EqualValues(t, 1, initializations.Load())

	second.Start()
	select {
	case <-second.Ready():
		require.FailNow(t, "test failed", "successor Store operation started before predecessor stage release")
	case <-time.After(200 * time.Millisecond):
	}
	require.EqualValues(t, 1, initializations.Load())
}

func TestInvalidStoreCommandClearsOlderPendingDesiredConfig(t *testing.T) {
	controller, store := newSecretControllerTestHarness(t, nil)
	require.NoError(t, controller.Bind(restartTestJobs{}))
	commands := &initialStoreTestCommands{
		publishTemplates: controller.templateCleanup(),
	}
	require.NoError(t, controller.PublishInitial(t.Context(), commands))
	t.Cleanup(func() {
		require.NoError(t, controller.CloseProjection())
		require.NoError(t, store.Close(context.Background()))
	})

	pending := secretTestConfig(confgroup.TypeDyncfg, "pending")
	version, err := controller.allocateDesiredVersion()
	require.NoError(t, err)
	release := make(chan struct{})
	controller.retainPending(pending, version, release)
	require.True(t, controller.pendingVersion(pending.ExposedKey(), version))

	input := CommandInput{
		Args:        []string{"go.d:secretstore:vault", "add", "main"},
		Payload:     []byte(`{`),
		ContentType: "application/json",
		HasPayload:  true,
	}
	stage, err := controller.Stage(input)
	require.NoError(t, err)
	stage.Start()
	select {
	case <-stage.Ready():
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "invalid Store operation did not settle")
	}
	defer stage.Release()

	resourceID := secretResourceID(pending.ExposedKey())
	prepared, err := controller.PrepareStaged(
		t.Context(),
		input,
		nil,
		lifecycle.ResourceTransactionScope{
			ID: resourceID,
			Successor: lifecycle.ResourceIdentity{
				ID:         resourceID,
				Generation: 1,
			},
		},
		lifecycle.LongLivedPermit{},
		stage,
	)
	require.NoError(t, err)
	applied, err := prepared.Apply(t.Context())
	require.NoError(t, err)
	require.Equal(t, 400, applied.ResultStatus())
	require.False(t, controller.pendingVersion(pending.ExposedKey(), version))
}

func TestRemoveFailedAbsentDynCfgStoreWithdrawsPendingDesiredState(t *testing.T) {
	controller, store := newSecretControllerTestHarness(t, nil)
	require.NoError(t, controller.Bind(restartTestJobs{}))
	commands := &initialStoreTestCommands{
		publishTemplates: controller.templateCleanup(),
	}
	require.NoError(t, controller.PublishInitial(t.Context(), commands))
	t.Cleanup(func() {
		require.NoError(t, controller.CloseProjection())
		require.NoError(t, store.Close(context.Background()))
	})

	config := secretTestConfig(confgroup.TypeDyncfg, "pending")
	key := config.ExposedKey()
	controller.commitEntry(key, &secretEntry{
		config: config,
		status: dyncfg.StatusFailed,
	})
	version, err := controller.allocateDesiredVersion()
	require.NoError(t, err)
	release := make(chan struct{})
	controller.retainPending(config, version, release)
	require.True(t, controller.pendingVersion(key, version))

	input := CommandInput{
		Args: []string{"go.d:secretstore:vault:main", "remove"},
	}
	stage, err := controller.Stage(input)
	require.NoError(t, err)
	stage.Start()
	select {
	case <-stage.Ready():
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "Store removal did not settle")
	}
	defer stage.Release()
	prepared, err := controller.PrepareStaged(
		t.Context(),
		input,
		nil,
		lifecycle.ResourceTransactionScope{ID: secretResourceID(key)},
		lifecycle.LongLivedPermit{},
		stage,
	)
	require.NoError(t, err)

	applied, err := prepared.Apply(t.Context())

	require.NoError(t, err)
	require.Equal(t, 200, applied.ResultStatus())
	_, disposition, owned := applied.Ownership()
	// The failed entry never installed a resource, so removing its desired
	// state leaves resource ownership unchanged.
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
	require.Nil(t, owned)
	_, exists := controller.entry(key)
	require.False(t, exists)
	require.False(t, controller.pendingVersion(key, version))
	close(release)
	require.Never(t, func() bool {
		_, ok := controller.entry(key)
		return ok
	}, 50*time.Millisecond, time.Millisecond)
}

func TestPendingStoreRestartsInactiveWorkerForLaterDesiredState(t *testing.T) {
	controller, store := newSecretControllerTestHarness(t, nil)
	diagnostics := &secretRecordingDiagnosticObserver{}
	controller.diagnostics = diagnostics
	controller.mu.Lock()
	controller.commands = failingPendingRetryCommands{
		err: errors.New("retry command failed"),
	}
	controller.mu.Unlock()
	t.Cleanup(func() {
		require.NoError(t, controller.CloseProjection())
		require.NoError(t, store.Close(context.Background()))
	})

	config := secretTestConfig(confgroup.TypeDyncfg, "first")
	firstVersion, err := controller.allocateDesiredVersion()
	require.NoError(t, err)
	firstRelease := make(chan struct{})
	close(firstRelease)
	controller.retainPending(config, firstVersion, firstRelease)
	require.Eventually(t, func() bool {
		return countSecretDiagnostics(
			diagnostics.snapshot(),
			"secret Store pending retry failed",
		) == 1
	}, time.Second, time.Millisecond)

	config = secretTestConfig(confgroup.TypeDyncfg, "second")
	secondVersion, err := controller.allocateDesiredVersion()
	require.NoError(t, err)
	secondRelease := make(chan struct{})
	close(secondRelease)
	controller.retainPending(config, secondVersion, secondRelease)
	require.Eventually(t, func() bool {
		return countSecretDiagnostics(
			diagnostics.snapshot(),
			"secret Store pending retry failed",
		) == 2
	}, time.Second, time.Millisecond)
	require.True(t, controller.pendingVersion(config.ExposedKey(), secondVersion))
}

type failingPendingRetryCommands struct {
	err error
}

func (commands failingPendingRetryCommands) SubmitPrepared(
	context.Context,
	jobmgr.Request,
	jobmgr.WorkPlan,
) error {
	return commands.err
}

func (commands failingPendingRetryCommands) SubmitPreparedAndWait(
	context.Context,
	jobmgr.Request,
	jobmgr.WorkPlan,
) error {
	return commands.err
}

func countSecretDiagnostics(events []jobmgr.DiagnosticEvent, name string) int {
	count := 0
	for _, event := range events {
		if event.Name == name {
			count++
		}
	}
	return count
}

func TestFailedStoreUpdateReplacesFailedProjectionWhenNoActiveGenerationExists(t *testing.T) {
	controller, store := newSecretControllerTestHarness(t, nil)
	require.NoError(t, controller.Bind(restartTestJobs{}))
	controller.setCommandsReady(true)
	resourceID := secretResourceID(secretstore.StoreKey(secretstore.KindVault, "main"))
	nextGeneration := uint64(0)
	apply := func(command dyncfg.Command, value string) int {
		nextGeneration++
		input := CommandInput{
			Args: []string{
				"go.d:secretstore:vault",
				string(command),
				"main",
			},
			Payload:     []byte(`{"value":"` + value + `"}`),
			ContentType: "application/json",
			HasPayload:  true,
		}
		if command == dyncfg.CommandUpdate {
			input.Args = input.Args[:2]
			input.Args[0] = "go.d:secretstore:vault:main"
		}
		stage, err := controller.Stage(input)
		require.NoError(t, err)
		stage.Start()
		select {
		case <-stage.Ready():
		case <-time.After(time.Second):
			require.FailNow(t, "test failed", "failed Store command did not settle")
		}
		prepared, err := controller.PrepareStaged(
			t.Context(),
			input,
			nil,
			lifecycle.ResourceTransactionScope{
				ID: resourceID,
				Successor: lifecycle.ResourceIdentity{
					ID:         resourceID,
					Generation: nextGeneration,
				},
			},
			lifecycle.LongLivedPermit{},
			stage,
		)
		require.NoError(t, err)
		applied, err := prepared.Apply(t.Context())
		require.NoError(t, err)
		stage.Release()
		return applied.ResultStatus()
	}

	require.Equal(t, 400, apply(dyncfg.CommandAdd, "provider-failure-one"))
	require.Equal(t, 400, apply(dyncfg.CommandUpdate, "provider-failure-two"))
	entry, ok := controller.entry(secretstore.StoreKey(secretstore.KindVault, "main"))
	require.True(t, ok)
	require.Equal(t, dyncfg.StatusFailed, entry.status)
	require.Equal(t, "provider-failure-two", entry.config["value"])
	require.NoError(t, store.Close(context.Background()))
}

func TestStoreTestIdentityDoesNotBlockMutationsOrDifferentTests(t *testing.T) {
	gate := make(chan struct{})
	entered := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(gate) })
	})
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	store, err := secretstore.NewSecretStore(resolver)
	require.NoError(t, err)
	catalog, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &testIdentityStoreOperationProvider{
				entered: entered,
				gate:    gate,
			}
		},
	}})
	require.NoError(t, err)
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	operations, err := NewStoreOperations(StoreOperationsConfig{
		Epoch:    1,
		Attempts: attempts,
		Store:    store,
		Creators: catalog,
	})
	require.NoError(t, err)
	target := secretTarget{
		command: dyncfg.CommandTest,
		key:     secretstore.StoreKey(secretstore.KindVault, "main"),
		kind:    secretstore.KindVault,
		name:    "main",
	}
	blockedConfig := secretTestConfig(confgroup.TypeDyncfg, "blocked")
	first, err := operations.prepare(storeOperationSpec{
		target:       target,
		config:       blockedConfig,
		mode:         storeOperationValidation,
		testIdentity: true,
	})
	require.NoError(t, err)
	var stages []*PreparedStoreOperation
	stages = append(stages, first)
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(gate) })
		for _, stage := range stages {
			stage.Release()
		}
		attempts.BeginShutdown()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, attempts.Shutdown(shutdownCtx))
		require.NoError(t, store.Close(context.Background()))
	})

	first.Start()
	select {
	case <-entered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "first Store test did not enter provider initialization")
	}
	first.Cancel(context.Canceled)
	select {
	case <-first.Ready():
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "contained Store test did not settle logically")
	}

	mutation, err := operations.prepare(storeOperationSpec{
		target: secretTarget{
			command: dyncfg.CommandUpdate,
			key:     target.key,
			kind:    target.kind,
			name:    target.name,
		},
		config:    secretTestConfig(confgroup.TypeDyncfg, "mutation"),
		mode:      storeOperationMutation,
		supersede: true,
	})
	require.NoError(t, err)
	stages = append(stages, mutation)
	mutation.Start()
	select {
	case <-mutation.Ready():
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "production Store mutation was blocked by contained test")
	}
	mutationResult, err := mutation.take()
	require.NoError(t, err)
	require.NoError(t, mutationResult.err)
	require.NotNil(t, mutationResult.mutation)
	require.NoError(t, mutationResult.mutation.Abort())

	duplicate, err := operations.prepare(storeOperationSpec{
		target:       target,
		config:       blockedConfig,
		mode:         storeOperationValidation,
		testIdentity: true,
	})
	require.NoError(t, err)
	stages = append(stages, duplicate)
	duplicate.Start()
	select {
	case <-duplicate.Ready():
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "identical Store test did not return busy")
	}
	duplicateResult, err := duplicate.take()
	require.NoError(t, err)
	require.ErrorIs(t, duplicateResult.err, jobmgr.ErrProcessAttemptBusy)
	require.True(t, duplicateResult.retryable)

	distinct, err := operations.prepare(storeOperationSpec{
		target:       target,
		config:       secretTestConfig(confgroup.TypeDyncfg, "distinct"),
		mode:         storeOperationValidation,
		testIdentity: true,
	})
	require.NoError(t, err)
	stages = append(stages, distinct)
	distinct.Start()
	select {
	case <-distinct.Ready():
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "distinct Store test was blocked")
	}
	distinctResult, err := distinct.take()
	require.NoError(t, err)
	require.NoError(t, distinctResult.err)

	releaseOnce.Do(func() { close(gate) })
}

func TestSecretAddCollisionIsReplayUpsert(t *testing.T) {
	controller, store := newSecretControllerTestHarness(t, nil)
	require.NoError(t, controller.Bind(restartTestJobs{}))
	controller.setCommandsReady(true)

	existing := secretTestConfig(confgroup.TypeUser, "original")
	mutation, err := store.PrepareMutation(t.Context(), controller.creators, existing, 0)
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

	add := CommandInput{
		Args:        []string{"go.d:secretstore:vault", "add", "main"},
		Payload:     []byte(`{"value":"replacement"}`),
		ContentType: "application/json",
		HasPayload:  true,
	}
	stage, err := controller.Stage(add)
	require.NoError(t, err)
	stage.Start()
	<-stage.Ready()
	transaction, err := controller.PrepareStaged(
		t.Context(),
		add,
		current,
		lifecycle.ResourceTransactionScope{
			ID:        resourceID,
			Current:   currentIdentity,
			Successor: successorIdentity,
		},
		lifecycle.LongLivedPermit{},
		stage,
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
	require.Nil(t, current.store)
	require.Empty(t, current.key)
	require.Zero(t, current.storeGen)
	stage.Release()

	repeatSuccessor := lifecycle.ResourceIdentity{ID: resourceID, Generation: 3}
	repeatStage, err := controller.Stage(add)
	require.NoError(t, err)
	repeatStage.Start()
	<-repeatStage.Ready()
	repeat, err := controller.PrepareStaged(
		t.Context(),
		add,
		owned,
		lifecycle.ResourceTransactionScope{
			ID:        resourceID,
			Current:   successorIdentity,
			Successor: repeatSuccessor,
		},
		lifecycle.LongLivedPermit{},
		repeatStage,
	)
	require.NoError(t, err)
	repeated, err := repeat.Apply(t.Context())
	require.NoError(t, err)
	_, repeatDisposition, repeatOwned := repeated.Ownership()
	require.Equal(t, 200, repeated.ResultStatus())
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, repeatDisposition)
	require.Same(t, owned, repeatOwned)
	repeatStage.Release()

	require.NoError(t, repeatOwned.Finalize())
	resource, ok := repeatOwned.(*storeGenerationResource)
	require.True(t, ok)
	require.Nil(t, resource.store)
	require.Empty(t, resource.key)
	require.Zero(t, resource.storeGen)
	require.NoError(t, store.Close(t.Context()))
}

func newSecretControllerTestHarness(
	t *testing.T,
	initial []secretstore.Config,
) (*Controller, *secretstore.SecretStore) {
	return newSecretControllerTestHarnessWithWriter(t, initial, io.Discard)
}

func newSecretControllerTestHarnessWithWriter(
	t *testing.T,
	initial []secretstore.Config,
	writer io.Writer,
) (*Controller, *secretstore.SecretStore) {
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
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	operations, err := NewStoreOperations(StoreOperationsConfig{
		Epoch:    1,
		Attempts: attempts,
		Store:    store,
		Creators: catalog,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		attempts.BeginShutdown()
		require.NoError(t, attempts.Shutdown(context.Background()))
	})
	controller, err := NewController(ControllerConfig{
		Epoch:        1,
		PluginName:   "go.d",
		Frames:       frames,
		Store:        store,
		Operations:   operations,
		Creators:     catalog,
		Dependencies: NewSecretDependencyIndex(),
		Initial:      initial,
	})
	require.NoError(t, err)
	return controller, store
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

type initialStoreTestCommands struct {
	mu               sync.Mutex
	nextGeneration   uint64
	publishTemplates func() error
}

func (commands *initialStoreTestCommands) SubmitPrepared(
	ctx context.Context,
	request jobmgr.Request,
	plan jobmgr.WorkPlan,
) error {
	return commands.SubmitPreparedAndWait(ctx, request, plan)
}

func (commands *initialStoreTestCommands) SubmitPreparedAndWait(
	ctx context.Context,
	request jobmgr.Request,
	plan jobmgr.WorkPlan,
) error {
	if request.LaneKey == secretBootResourceID {
		return commands.publishTemplates()
	}
	commands.mu.Lock()
	commands.nextGeneration++
	generation := commands.nextGeneration
	commands.mu.Unlock()
	scope := lifecycle.ResourceTransactionScope{
		ID: request.LaneKey,
		Successor: lifecycle.ResourceIdentity{
			ID:         request.LaneKey,
			Generation: generation,
		},
	}
	prepared, err := plan.Transaction.Prepare(
		ctx,
		nil,
		scope,
		lifecycle.LongLivedPermit{},
	)
	if err != nil {
		return err
	}
	_, err = prepared.Dispose(ctx)
	return err
}

type initialStoreMaterializationGate struct {
	blocked  chan struct{}
	fast     chan struct{}
	releaseC chan struct{}
	once     sync.Once
}

func newInitialStoreMaterializationGate() *initialStoreMaterializationGate {
	return &initialStoreMaterializationGate{
		blocked:  make(chan struct{}),
		fast:     make(chan struct{}),
		releaseC: make(chan struct{}),
	}
}

func (gate *initialStoreMaterializationGate) release() {
	gate.once.Do(func() {
		close(gate.releaseC)
	})
}

type initialStoreMaterializationStore struct {
	gate   *initialStoreMaterializationGate
	config struct {
		Value string `yaml:"value"`
	}
}

func (store *initialStoreMaterializationStore) Configuration() any {
	return &store.config
}

func (store *initialStoreMaterializationStore) Init(context.Context) error {
	switch store.config.Value {
	case "blocked":
		close(store.gate.blocked)
		<-store.gate.releaseC
	case "fast":
		close(store.gate.fast)
	}
	return nil
}

func (store *initialStoreMaterializationStore) Publish() secretstore.PublishedStore {
	return transactionTestPublished(store.config.Value)
}

type countingStoreOperationProvider struct {
	initializations *atomic.Int32
	config          struct {
		Value string `yaml:"value"`
	}
}

func (store *countingStoreOperationProvider) Configuration() any {
	return &store.config
}

func (store *countingStoreOperationProvider) Init(context.Context) error {
	store.initializations.Add(1)
	return nil
}

func (store *countingStoreOperationProvider) Publish() secretstore.PublishedStore {
	return transactionTestPublished(store.config.Value)
}

type testIdentityStoreOperationProvider struct {
	entered chan struct{}
	gate    <-chan struct{}
	config  struct {
		Value string `yaml:"value"`
	}
}

func (store *testIdentityStoreOperationProvider) Configuration() any {
	return &store.config
}

func (store *testIdentityStoreOperationProvider) Init(context.Context) error {
	if store.config.Value == "blocked" {
		select {
		case store.entered <- struct{}{}:
		default:
		}
		<-store.gate
	}
	return nil
}

func (store *testIdentityStoreOperationProvider) Publish() secretstore.PublishedStore {
	return transactionTestPublished(store.config.Value)
}

func secretInitialMaterializationConfig(name, value string) secretstore.Config {
	return secretstore.Config{
		"name":            name,
		"kind":            string(secretstore.KindVault),
		"value":           value,
		"__source__":      confgroup.TypeUser + "=test",
		"__source_type__": confgroup.TypeUser,
	}
}
