// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"fmt"
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
			input := CommandInput{
				Args:        []string{"go.d:secretstore:vault", "add", "main"},
				Payload:     []byte(fmt.Sprintf(`{"value":%q}`, value)),
				ContentType: "application/json", HasPayload: true,
			}
			stage, err := controller.Stage(input)
			require.NoError(t, err)
			defer stage.Release()
			stage.Start()
			requireStoreOperationReady(t, stage)
			id := secretResourceID("vault:main")
			transaction, err := controller.PrepareStaged(t.Context(), input, nil,
				lifecycle.ResourceTransactionScope{ID: id, Successor: lifecycle.ResourceIdentity{ID: id, Generation: 1}},
				lifecycle.LongLivedPermit{}, stage)
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
