// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLookupFunction_UsesDirectSnapshot(t *testing.T) {
	mgr := NewManager()

	called := make(chan struct{}, 1)
	mgr.Register("fn", func(Function) { called <- struct{}{} })

	handler, ok := mgr.lookupFunction("fn")
	require.True(t, ok)

	mgr.Unregister("fn")
	handler(Function{Name: "fn"})

	select {
	case <-called:
	default:
		t.Fatal("snapshot handler should still invoke the originally resolved direct function")
	}
}

func TestLookupFunction_UsesPrefixSnapshot(t *testing.T) {
	mgr := NewManager()

	called := make(chan struct{}, 1)
	mgr.RegisterPrefix("config", "collector:", func(Function) { called <- struct{}{} })

	handler, ok := mgr.lookupFunction("config")
	require.True(t, ok)

	mgr.UnregisterPrefix("config", "collector:")
	handler(Function{Name: "config", Args: []string{"collector:job"}})

	select {
	case <-called:
	default:
		t.Fatal("snapshot handler should still route using the prefix set captured at lookup time")
	}
}
