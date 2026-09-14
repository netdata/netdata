// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestStoreAttemptIdentityIsBoundedAndStructurallyDistinct(t *testing.T) {
	operations := &StoreOperations{epoch: 7}
	same := operations.identity("vault:main", false, nil)
	require.Equal(t, same, operations.identity("vault:main", false, nil))
	require.Len(t, same.Key, sha256.Size)
	require.Equal(t, "vault:main", same.Resource)

	differentEpoch := (&StoreOperations{epoch: 8}).identity("vault:main", false, nil)
	require.NotEqual(t, same.Key, differentEpoch.Key)

	testA := operations.identity("vault:main", true, []byte("a"))
	testB := operations.identity("vault:main", true, []byte("b"))
	require.NotEqual(t, same.Namespace, testA.Namespace)
	require.NotEqual(t, testA.Key, testB.Key)

	longKey := "vault:" + strings.Repeat("a", 251)
	long := operations.identity(longKey, false, nil)
	require.Len(t, long.Key, sha256.Size)
	require.Equal(t, "secret Store", long.Resource)
}

func TestInitialStoreAttemptAcceptsLongValidName(t *testing.T) {
	config := longNameSecretTestConfig(confgroup.TypeUser, "initial")
	require.NoError(t, config.Validate())
	require.Greater(t, len(config.ExposedKey()), 256)

	controller, store := newSecretControllerTestHarness(t, []secretstore.Config{config})
	require.NoError(t, controller.Bind(restartTestJobs{}))
	commands := &applyingInitialStoreTestCommands{
		publishTemplates: controller.templateCleanup(),
	}
	t.Cleanup(func() {
		require.NoError(t, controller.CloseProjection())
		require.NoError(t, commands.Finalize())
		require.NoError(t, store.Close(context.Background()))
	})

	require.NoError(t, controller.PublishInitial(t.Context(), commands))
	require.Equal(t, []int{200}, commands.Statuses())
}

func TestDynCfgStoreMutationAttemptsAcceptLongValidName(t *testing.T) {
	for _, command := range []dyncfg.Command{dyncfg.CommandAdd, dyncfg.CommandUpdate} {
		t.Run(string(command), func(t *testing.T) {
			controller, store := newSecretControllerTestHarness(t, nil)
			require.NoError(t, controller.Bind(restartTestJobs{}))
			controller.setCommandsReady(true)
			t.Cleanup(func() {
				require.NoError(t, store.Close(context.Background()))
			})

			config := longNameSecretTestConfig(confgroup.TypeDyncfg, "current")
			name := config.Name()
			if command == dyncfg.CommandUpdate {
				controller.commitEntry(config.ExposedKey(), &secretEntry{
					config: config,
					status: dyncfg.StatusRunning,
				})
			}
			input := CommandInput{
				Args: []string{
					"go.d:secretstore:vault",
					string(command),
					name,
				},
				Payload:     []byte(`{"value":"replacement"}`),
				ContentType: "application/json",
				HasPayload:  true,
			}
			if command == dyncfg.CommandUpdate {
				input.Args = []string{
					"go.d:secretstore:vault:" + name,
					string(command),
				}
			}

			stage, err := controller.Stage(input)
			require.NoError(t, err)
			t.Cleanup(stage.Release)
			stage.Start()
			requireStoreOperationReady(t, stage)

			result, err := stage.take()
			require.NoError(t, err)
			require.NoError(t, result.err)
			require.NotNil(t, result.mutation)
			require.NoError(t, result.mutation.Abort())
		})
	}
}

func TestDynCfgStoreTestAttemptAcceptsLongValidName(t *testing.T) {
	controller, store := newSecretControllerTestHarness(t, nil)
	require.NoError(t, controller.Bind(restartTestJobs{}))
	controller.setCommandsReady(true)
	t.Cleanup(func() {
		require.NoError(t, store.Close(context.Background()))
	})

	config := longNameSecretTestConfig(confgroup.TypeDyncfg, "current")
	controller.commitEntry(config.ExposedKey(), &secretEntry{
		config: config,
		status: dyncfg.StatusRunning,
	})
	input := CommandInput{
		Args: []string{
			"go.d:secretstore:vault:" + config.Name(),
			string(dyncfg.CommandTest),
		},
		Payload:     []byte(`{"value":"replacement"}`),
		ContentType: "application/json",
		HasPayload:  true,
	}

	stage, err := controller.Stage(input)
	require.NoError(t, err)
	t.Cleanup(stage.Release)
	stage.Start()
	requireStoreOperationReady(t, stage)

	result, err := stage.take()
	require.NoError(t, err)
	require.NoError(t, result.err)
}

func longNameSecretTestConfig(sourceType, value string) secretstore.Config {
	config := secretTestConfig(sourceType, value)
	config["name"] = strings.Repeat("a", 251)
	return config
}

func requireStoreOperationReady(t *testing.T, stage *PreparedStoreOperation) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-stage.Ready():
	case <-timer.C:
		require.FailNow(t, "test failed", "Store operation did not become ready")
	}
}

type applyingInitialStoreTestCommands struct {
	mu               sync.Mutex
	nextGeneration   uint64
	publishTemplates func() error
	statuses         []int
	owned            []lifecycle.ReadyResource
}

func (c *applyingInitialStoreTestCommands) SubmitPrepared(
	ctx context.Context,
	request jobmgr.Request,
	plan jobmgr.WorkPlan,
) error {
	return c.SubmitPreparedAndWait(ctx, request, plan)
}

func (c *applyingInitialStoreTestCommands) SubmitPreparedAndWait(
	ctx context.Context,
	request jobmgr.Request,
	plan jobmgr.WorkPlan,
) error {
	if request.LaneKey == secretBootResourceID {
		return c.publishTemplates()
	}
	c.mu.Lock()
	c.nextGeneration++
	generation := c.nextGeneration
	c.mu.Unlock()
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
	applied, err := prepared.Apply(ctx)
	if err != nil {
		return err
	}
	_, _, owned := applied.Ownership()
	c.mu.Lock()
	c.statuses = append(c.statuses, applied.ResultStatus())
	c.owned = append(c.owned, owned)
	c.mu.Unlock()
	return nil
}

func (c *applyingInitialStoreTestCommands) Statuses() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.statuses...)
}

func (c *applyingInitialStoreTestCommands) Finalize() error {
	c.mu.Lock()
	owned := append([]lifecycle.ReadyResource(nil), c.owned...)
	c.owned = nil
	c.mu.Unlock()
	var result error
	for _, resource := range owned {
		if resource != nil {
			result = errors.Join(result, resource.Finalize())
		}
	}
	return result
}
