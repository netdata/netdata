// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
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
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestCanceledStoreStageCannotCutSuccessorBeforeAttemptRegistration(t *testing.T) {
	controller, store := newSecretControllerTestHarness(t, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	releaseFirst := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() {
		releaseFirst()
		require.NoError(t, store.Close(context.Background()))
	})
	controller.operations.attempts = &delayedStoreAttemptRegistration{
		ProcessAttemptAuthority: controller.operations.attempts,
		entered:                 entered,
		release:                 release,
	}
	stage := func(value string) *PreparedStoreOperation {
		operation, err := controller.operations.prepare(storeOperationSpec{
			target: secretTarget{
				key: "vault:main",
			},
			mode:   storeOperationMutation,
			config: secretTestConfig(confgroup.TypeDyncfg, value),
		})
		require.NoError(t, err)
		t.Cleanup(operation.Release)
		operation.Start()
		return operation
	}
	old := stage("old")
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("old attempt did not reach registration")
	}
	old.Cancel(context.Canceled)
	current := stage("current")
	requireStoreOperationReady(t, current)
	releaseFirst()
	requireStoreOperationReady(t, old)
	result, err := current.take()
	require.NoError(t, err)
	require.NoError(t, result.err)
	require.NotNil(t, result.mutation)
	committed, err := result.mutation.Commit(t.Context())
	require.NoError(t, err)
	require.True(t, committed.Applied)
	config, ok := store.Config("vault:main")
	require.True(t, ok)
	require.Equal(t, "current", config["value"])
	require.NoError(t, store.Retire(t.Context(), "vault:main", committed.Generation))
}

type delayedStoreAttemptRegistration struct {
	jobmgr.ProcessAttemptAuthority
	entered chan struct{}
	release <-chan struct{}
	calls   atomic.Int32
}

func (a *delayedStoreAttemptRegistration) StartProcessAttempt(
	ctx context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	if a.calls.Add(1) == 1 {
		close(a.entered)
		<-a.release
	}
	return a.ProcessAttemptAuthority.StartProcessAttempt(ctx, plan)
}

func TestStoreCandidateCannotReplaceNewerAcceptedRevision(t *testing.T) {
	for _, command := range []string{"add", "update"} {
		t.Run(command, func(t *testing.T) {
			controller, store := newSecretControllerTestHarness(t, nil)
			require.NoError(t, controller.Bind(restartTestJobs{}))
			require.NoError(t, controller.PublishInitial(t.Context(), &initialStoreTestCommands{
				publishTemplates: controller.templateCleanup(),
			}))
			t.Cleanup(func() {
				require.NoError(t, controller.CloseProjection())
				require.NoError(t, store.Close(context.Background()))
			})
			first, target := stageAdoptionEdit(t, controller, "add", "first")
			applyAdoptionEdit(t, controller, first, target, 202)
			candidate, candidateTarget := stageAdoptionEdit(t, controller, command, "candidate")
			latest, latestTarget := stageAdoptionEdit(t, controller, "add", "latest")
			applyAdoptionEdit(t, controller, latest, latestTarget, 202)
			applyAdoptionEdit(t, controller, candidate, candidateTarget, 503)
			entry, ok := controller.entry("vault:main")
			require.True(t, ok)
			require.Equal(t, "latest", entry.config["value"])
			require.True(t, controller.pendingVersion("vault:main", entry.version))
			require.Zero(t, store.Generation("vault:main"))
		})
	}
}

func TestStoreAdmissionCannotReplaceGenerationInstalledAfterStaging(t *testing.T) {
	controller, store := newSecretControllerTestHarness(t, nil)
	require.NoError(t, controller.Bind(restartTestJobs{}))
	require.NoError(t, controller.PublishInitial(t.Context(), &initialStoreTestCommands{
		publishTemplates: controller.templateCleanup(),
	}))
	t.Cleanup(func() {
		require.NoError(t, controller.CloseProjection())
		require.NoError(t, store.Close(context.Background()))
	})
	first, target := stageAdoptionEdit(t, controller, "add", "first")
	applyAdoptionEdit(t, controller, first, target, 202)
	entry, ok := controller.entry(target.key)
	require.True(t, ok)
	candidate, candidateTarget := stageAdoptionEdit(t, controller, "add", "candidate")

	// Drive the actual accepted completion boundary while the raw ADD is staged.
	activation, err := controller.operations.prepare(storeOperationSpec{
		target:          target,
		config:          entry.config,
		mode:            storeOperationMutation,
		desiredVersion:  entry.version,
		acceptedVersion: entry.version,
	})
	require.NoError(t, err)
	defer activation.Release()
	activation.Start()
	requireStoreOperationReady(t, activation)
	id := secretResourceID(target.key)
	scope := lifecycle.ResourceTransactionScope{
		ID: id,
		Successor: lifecycle.ResourceIdentity{
			ID:         id,
			Generation: 1,
		},
	}
	prepared, err := controller.preparePendingAttempt(
		entry.config,
		entry.version,
		nil,
		scope,
		lifecycle.LongLivedPermit{},
		activation,
	)
	require.NoError(t, err)
	applied, err := prepared.Apply(t.Context())
	require.NoError(t, err)
	require.Equal(t, 200, applied.ResultStatus())
	_, _, owned := applied.Ownership()
	require.NotNil(t, owned)
	defer func() { require.NoError(t, owned.Finalize()) }()

	scope.Current = owned.Identity()
	scope.Successor.Generation = 2
	prepared, err = controller.prepareAdd(scope, owned, candidateTarget, candidate)
	require.NoError(t, err)
	applied, err = prepared.Apply(t.Context())
	require.NoError(t, err)
	require.Equal(t, 503, applied.ResultStatus())
	current, ok := controller.entry(target.key)
	require.True(t, ok)
	require.Equal(t, entry.version, current.version)
	require.Equal(t, "first", current.config["value"])
}

func TestIdenticalStoreUpdateRechecksAcquisitionCompletion(t *testing.T) {
	for _, value := range []string{"ready", "provider-failure-one", "replaced"} {
		t.Run(value, func(t *testing.T) {
			controller, store := newSecretControllerTestHarness(t, nil)
			require.NoError(t, controller.Bind(restartTestJobs{}))
			require.NoError(t, controller.PublishInitial(t.Context(), &initialStoreTestCommands{
				publishTemplates: controller.templateCleanup(),
			}))
			t.Cleanup(func() {
				require.NoError(t, controller.CloseProjection())
				require.NoError(t, store.Close(context.Background()))
			})
			first, target := stageAdoptionEdit(t, controller, "add", value)
			applyAdoptionEdit(t, controller, first, target, 202)
			entry, ok := controller.entry(target.key)
			require.True(t, ok)
			duplicate, duplicateTarget := stageAdoptionEdit(t, controller, "update", value)
			require.Zero(t, store.Census().Preparations, "coalesced UPDATE prepared another generation")
			if value == "replaced" {
				newer, newerTarget := stageAdoptionEdit(t, controller, "add", "newer")
				applyAdoptionEdit(t, controller, newer, newerTarget, 202)
				applyAdoptionEdit(t, controller, duplicate, duplicateTarget, 503)
				latest, ok := controller.entry(target.key)
				require.True(t, ok)
				require.Equal(t, "newer", latest.config["value"])
				return
			}

			activation, err := controller.operations.prepare(storeOperationSpec{
				target:          target,
				config:          entry.config,
				mode:            storeOperationMutation,
				desiredVersion:  entry.version,
				acceptedVersion: entry.version,
			})
			require.NoError(t, err)
			defer activation.Release()
			activation.Start()
			requireStoreOperationReady(t, activation)
			id := secretResourceID(target.key)
			scope := lifecycle.ResourceTransactionScope{
				ID: id,
				Successor: lifecycle.ResourceIdentity{
					ID:         id,
					Generation: 1,
				},
			}
			prepared, err := controller.preparePendingAttempt(
				entry.config,
				entry.version,
				nil,
				scope,
				lifecycle.LongLivedPermit{},
				activation,
			)
			require.NoError(t, err)
			applied, err := prepared.Apply(t.Context())
			require.NoError(t, err)
			_, _, current := applied.Ownership()
			want := 503 // A Failed completion must not apply an unprepared candidate.
			if current != nil {
				defer func() { require.NoError(t, current.Finalize()) }()
				scope.Current = current.Identity()
				want = 200
			}
			scope.Successor.Generation = 2
			prepared, err = controller.prepareUpdate(scope, current, duplicateTarget, duplicate)
			require.NoError(t, err)
			applied, err = prepared.Apply(t.Context())
			require.NoError(t, err)
			require.Equal(t, want, applied.ResultStatus())
			latest, ok := controller.entry(target.key)
			require.True(t, ok)
			require.Equal(t, entry.version, latest.version)
			require.Equal(t, value, latest.config["value"])
		})
	}
}

func TestIdenticalStoreUpdateAbortsPreflightOvertakenByAcceptance(t *testing.T) {
	controller, store := newSecretControllerTestHarness(t, nil)
	require.NoError(t, controller.Bind(restartTestJobs{}))
	require.NoError(t, controller.PublishInitial(t.Context(), &initialStoreTestCommands{
		publishTemplates: controller.templateCleanup(),
	}))
	t.Cleanup(func() {
		require.NoError(t, controller.CloseProjection())
		require.NoError(t, store.Close(context.Background()))
	})
	first, target := stageAdoptionEdit(t, controller, "add", "first")
	applyAdoptionEdit(t, controller, first, target, 202)
	candidate, candidateTarget := stageAdoptionEdit(t, controller, "update", "replacement")
	require.EqualValues(t, 1, store.Census().Preparations)
	latest, latestTarget := stageAdoptionEdit(t, controller, "add", "replacement")
	applyAdoptionEdit(t, controller, latest, latestTarget, 202)
	applyAdoptionEdit(t, controller, candidate, candidateTarget, 202)
	require.Zero(t, store.Census().Preparations, "coalescing leaked unused preflight ownership")
	entry, ok := controller.entry(target.key)
	require.True(t, ok)
	require.Equal(t, "replacement", entry.config["value"])
	require.True(t, controller.pendingAcceptedVersion(target.key, entry.version))
	require.Zero(t, store.Generation(target.key))
}

func TestStoreAcceptedOwnerRequiresSuccessfulCurrentPublication(t *testing.T) {
	for _, outcome := range []string{"blocked", "failed", "superseded"} {
		t.Run(outcome, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			writer := io.Writer(io.Discard)
			if outcome != "superseded" {
				writer = secretTestWriteFunc(func(payload []byte) (int, error) {
					close(entered)
					<-release
					if outcome == "failed" {
						return 0, errors.New("publication failed")
					}
					return len(payload), nil
				})
			}
			controller, store := newSecretControllerTestHarnessWithWriter(t, nil, writer)
			require.NoError(t, controller.Bind(restartTestJobs{}))
			controller.commands = failingPendingRetryCommands{
				err: errors.New("test completion boundary"),
			}
			controller.setCommandsReady(true)
			t.Cleanup(func() {
				require.NoError(t, controller.CloseProjection())
				attempts := controller.operations.attempts.(*containment.Authority)
				attempts.BeginShutdown()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				require.NoError(t, attempts.Shutdown(shutdownCtx))
				require.NoError(t, store.Close(context.Background()))
			})
			stage, target := stageAdoptionEdit(t, controller, "add", "first")
			id := secretResourceID(target.key)
			prepared, err := controller.prepareAdd(lifecycle.ResourceTransactionScope{
				ID: id,
			}, nil, target, stage)
			require.NoError(t, err)
			publish := prepared.(*preparedSecretTransaction).spec.cleanup
			_, err = prepared.Apply(t.Context())
			require.NoError(t, err)
			assertDormant := func() {
				controller.mu.Lock()
				defer controller.mu.Unlock()
				require.False(t, controller.pending[target.key].running)
			}
			assertDormant()
			if outcome == "superseded" {
				newer, newerTarget := stageAdoptionEdit(t, controller, "add", "newer")
				applyAdoptionEdit(t, controller, newer, newerTarget, 202)
				require.NoError(t, publish())
				assertDormant() // The old callback cannot open the new owner's gate.
				return
			}
			done := make(chan error, 1)
			go func() { done <- publish() }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("publication did not start")
			}
			assertDormant()
			close(release)
			if outcome == "failed" {
				require.Error(t, <-done)
				assertDormant()
			} else {
				require.NoError(t, <-done)
				controller.mu.Lock()
				require.True(t, controller.pending[target.key].running)
				controller.mu.Unlock()
			}
		})
	}
}

func stageAdoptionEdit(
	t *testing.T,
	controller *Controller,
	command, value string,
) (*PreparedStoreOperation, secretTarget) {
	t.Helper()
	input := CommandInput{
		Args:        []string{"go.d:secretstore:vault", command, "main"},
		Payload:     []byte(`{"value":"` + value + `"}`),
		ContentType: "application/json",
		HasPayload:  true,
	}
	if command == "update" {
		input.Args = []string{"go.d:secretstore:vault:main", command}
	}
	stage, err := controller.Stage(input)
	require.NoError(t, err)
	t.Cleanup(stage.Release)
	stage.Start()
	requireStoreOperationReady(t, stage)
	target, failure := controller.resolveTarget(input)
	require.Nil(t, failure)
	return stage, target
}

func applyAdoptionEdit(
	t *testing.T,
	controller *Controller,
	stage *PreparedStoreOperation,
	target secretTarget,
	status int,
) {
	t.Helper()
	id := secretResourceID(target.key)
	prepared, err := controller.prepareEdit(lifecycle.ResourceTransactionScope{
		ID: id,
		Successor: lifecycle.ResourceIdentity{
			ID:         id,
			Generation: 1,
		},
	}, nil, target, stage, target.command == dyncfg.CommandAdd)
	require.NoError(t, err)
	applied, err := prepared.Apply(t.Context())
	require.NoError(t, err)
	require.Equal(t, status, applied.ResultStatus())
	stage.Release()
}
