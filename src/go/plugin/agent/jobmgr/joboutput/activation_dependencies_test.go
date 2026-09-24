// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestActivationDependencyKeysPreserveSourceAuthority(t *testing.T) {
	config := confgroup.Config{
		"vnode": "router", "password": "${store:vault:credentials:key}",
		"duplicate": "${store:vault:credentials:other}", "env": "${env:IGNORED}",
		"__internal__": "${store:vault:internal:key}",
	}.SetSourceType(confgroup.TypeDyncfg)
	keys, err := dependencyKeys(config)
	require.NoError(t, err)
	require.Equal(t, []activationDependency{{"vnode", "router"}, {"secretstore", "vault:credentials"}}, keys)
	config.SetSourceType(confgroup.TypeDiscovered)
	keys, err = dependencyKeys(config)
	require.NoError(t, err)
	require.Equal(t, []activationDependency{{"vnode", "router"}}, keys)
	config.SetSourceType(confgroup.TypeDyncfg)
	config["password"] = "${store:invalid}"
	_, err = dependencyKeys(config)
	require.Error(t, err)
}

func TestActivationDependencyIndexRetainsLookupWaitNotification(t *testing.T) {
	var index activationDependencyIndex
	wake := make(chan struct{}, 1)
	index.replace("job", []activationDependency{{"vnode", "router"}}, wake)
	// The lookup has failed, but the waiter has not started receiving yet.
	index.notify("vnode", "router")
	index.notify("vnode", "router")
	require.Len(t, wake, 1, "changes coalesce without blocking the committing caller")
	<-wake
	index.notify("secretstore", "router")
	index.notify("vnode", "other")
	require.Empty(t, wake, "only the named dependency can wake this job")
}

func TestActivationDependencyIndexReplacesAndRemovesSubscriptions(t *testing.T) {
	var index activationDependencyIndex
	old, next, shared := make(chan struct{}, 1), make(chan struct{}, 1), make(chan struct{}, 1)
	keys := []activationDependency{{"vnode", "router"}}
	index.replace("job", keys, old)
	index.replace("shared", keys, shared)
	keys[0].name = "mutated"
	index.replace("job", []activationDependency{{"secretstore", "vault:credentials"}}, next)
	index.notify("vnode", "router")
	require.Empty(t, old)
	require.Len(t, shared, 1)
	require.Empty(t, next)
	index.notify("secretstore", "vault:credentials")
	require.Len(t, next, 1)
	<-next
	index.remove("job")
	index.remove("shared")
	index.notify("secretstore", "vault:credentials")
	require.Empty(t, next)
	require.Empty(t, index.watchers)
	require.Empty(t, index.subscriptions)
}

func TestActivationDependencyIndexConcurrentNotifications(t *testing.T) {
	var index activationDependencyIndex
	var workers sync.WaitGroup
	for n := 0; n < 8; n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			wake := make(chan struct{}, 1)
			id := fmt.Sprint(n)
			for i := 0; i < 100; i++ {
				index.replace(id, []activationDependency{{"vnode", "router"}}, wake)
				index.notify("vnode", "router")
				index.remove(id)
			}
		}()
	}
	workers.Wait()
	require.Empty(t, index.watchers)
}

func TestActivationDependencyWaitClassification(t *testing.T) {
	config := confgroup.Config{"vnode": "router", "password": "${store:vault:credentials:key}"}.SetSourceType(confgroup.TypeDyncfg)
	missing := &secretresolver.AtomicResolveError{Kind: secretresolver.AtomicErrorScope,
		Cause: fmt.Errorf("sensitive scope details: %w", secretstore.ErrStoreNotFound)}
	for name, test := range map[string]struct {
		err  error
		wait bool
	}{
		"missing Store":            {missing, true},
		"redacted missing Store":   {redactResolvedLifecycleError(missing), true},
		"provider unavailable":     {&secretresolver.AtomicResolveError{Kind: secretresolver.AtomicErrorProvider, Cause: errors.New("offline")}, false},
		"provider reports missing": {&secretresolver.AtomicResolveError{Kind: secretresolver.AtomicErrorProvider, Cause: secretstore.ErrStoreNotFound}, false},
		"scope infrastructure":     {&secretresolver.AtomicResolveError{Kind: secretresolver.AtomicErrorScope, Cause: errors.New("closed")}, false},
		"missing vnode":            {transientJobConstruction(withJobConfigFailure(errors.New("missing"), "vnode", "missing_vnode")), true},
		"pending vnode":            {transientJobConstruction(withJobConfigFailure(errors.New("pending"), "vnode", "pending_vnode")), true},
		"collector failure":        {errors.New("vnode is missing"), false},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.wait, activationWaitsForDependency(config, test.err))
		})
	}
	require.NotContains(t, redactResolvedLifecycleError(missing).Error(), "sensitive scope details")
	config.SetSourceType(confgroup.TypeDiscovered)
	require.False(t, activationWaitsForDependency(config, missing))
}
