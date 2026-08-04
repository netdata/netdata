// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfileMetricTestCatalogTrapSemanticsMatchProduction(t *testing.T) {
	trap := func(oid, name string) *testTrapDef {
		return &testTrapDef{
			OID:      oid,
			Name:     name,
			Category: "state_change",
			Severity: "notice",
		}
	}

	t.Run("duplicate OID", func(t *testing.T) {
		idx := newTestCatalog(t)
		require.NoError(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::first")}))
		require.Error(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::second")}))
	})

	t.Run("duplicate name", func(t *testing.T) {
		idx := newTestCatalog(t)
		require.NoError(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::same")}))
		require.Error(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.2", "TEST-MIB::same")}))
	})

	t.Run("alternate OID", func(t *testing.T) {
		idx := newTestCatalog(t)
		require.NoError(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.0.1", "TEST-MIB::first")}))
		require.Error(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::second")}))
	})

	t.Run("failed batch is atomic", func(t *testing.T) {
		idx := newTestCatalog(t)
		candidate := trap("1.3.6.1.4.1.99999.1", "TEST-MIB::candidate")
		require.Error(t, idx.addTraps([]*testTrapDef{candidate, nil}))
		_, err := idx.ResolveTrap(candidate.OID)
		require.Error(t, err)
	})

	t.Run("references are trimmed", func(t *testing.T) {
		idx := newTestCatalog(t)
		candidate := trap("1.3.6.1.4.1.99999.1", "TEST-MIB::candidate")
		require.NoError(t, idx.addTraps([]*testTrapDef{candidate}))
		for _, ref := range []string{" 1.3.6.1.4.1.99999.1 ", " TEST-MIB::candidate "} {
			resolved, err := idx.ResolveTrap(ref)
			require.NoError(t, err)
			require.Equal(t, candidate.OID, resolved.OID)
		}
	})
}
