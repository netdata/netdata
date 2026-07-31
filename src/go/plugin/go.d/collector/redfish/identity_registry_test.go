// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResourceKeyUsesStructuralFields(t *testing.T) {
	require.NotEqual(
		t,
		resourceKey("origin", "kind\x00locator", "tail"),
		resourceKey("origin\x00kind", "locator", "tail"),
	)
}

func TestIdentityRegistryRegistersAtomicallyAndRemembersBindings(t *testing.T) {
	var registry identityRegistry
	require.NoError(t, registry.register([]identityBinding{
		{Domain: "resource", Key: "key-a", Preimage: "preimage-a"},
	}))
	require.NoError(t, registry.register([]identityBinding{
		{Domain: "resource", Key: "key-a", Preimage: "preimage-a"},
	}))

	err := registry.register([]identityBinding{
		{Domain: "resource", Key: "key-b", Preimage: "preimage-b"},
		{Domain: "resource", Key: "key-a", Preimage: "different"},
	})
	require.ErrorContains(t, err, "resource key collision")
	require.True(t, identityIntegrityError(err))

	registry.mu.Lock()
	_, partialCommit := registry.bindings["resource\x00key-b"]
	registry.mu.Unlock()
	require.False(t, partialCommit)
}
