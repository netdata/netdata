// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestStoreDependencyNotificationFollowsSuccessfulCommit(t *testing.T) {
	for _, action := range []string{"commit", "dispose", "provider failure"} {
		t.Run(action, func(t *testing.T) {
			controller, store := newSecretControllerTestHarness(t, nil)
			require.NoError(t, controller.Bind(restartTestJobs{}))
			controller.setCommandsReady(true)
			t.Cleanup(func() { require.NoError(t, store.Close(context.Background())) })
			var notified []string
			controller.dependencyChanged = func(key string) {
				require.True(t, controller.mu.TryLock(), "notification must run outside the entry lock")
				controller.mu.Unlock()
				scope, err := store.AcquireScope([]string{key})
				require.NoError(t, err, "notification must observe the published Store generation")
				require.NoError(t, scope.Release(t.Context()))
				notified = append(notified, key)
			}
			value := "published"
			if action == "provider failure" {
				value = "provider-failure-one"
			}
			stage, err := controller.operations.prepare(storeOperationSpec{
				target: secretTarget{
					key: "vault:main",
				},
				config: secretTestConfig("dyncfg", value),
				mode:   storeOperationMutation,
			})
			require.NoError(t, err)
			defer stage.Release()
			stage.Start()
			requireStoreOperationReady(t, stage)
			id := secretResourceID("vault:main")
			operation, err := takeStoreOperation(stage)
			require.NoError(t, err)
			transaction, err := controller.prepareStoreMutation(
				lifecycle.ResourceTransactionScope{
					ID: id,
					Successor: lifecycle.ResourceIdentity{
						ID:         id,
						Generation: 1,
					},
				},
				nil,
				operation,
				true,
			)
			require.NoError(t, err)
			require.Empty(t, notified, "preparation must not notify dependents")
			if action == "dispose" {
				_, err = transaction.Dispose(t.Context())
				require.NoError(t, err)
			} else {
				applied, err := transaction.Apply(t.Context())
				require.NoError(t, err)
				_, _, resource := applied.Ownership()
				if resource != nil {
					require.NoError(t, resource.Finalize())
				}
			}
			if action == "commit" {
				require.Equal(t, []string{"vault:main"}, notified)
			} else {
				require.Empty(t, notified)
			}
		})
	}
}
