// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func cycleController(t *testing.T, s CollectorStore) CycleController {
	t.Helper()
	managed, ok := AsCycleManagedStore(s)
	require.True(t, ok, "store does not expose cycle control")
	return managed.CycleController()
}

func expectPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		require.NotNil(t, recover(), "expected panic")
	}()
	fn()
}

func mustValue(t *testing.T, r Reader, name string, labels Labels, want SampleValue) {
	t.Helper()
	got, ok := r.Value(name, labels)
	require.True(t, ok, "expected value for %s", name)
	require.Equal(t, want, got, "unexpected value for %s", name)
}

func mustDelta(t *testing.T, r Reader, name string, labels Labels, want SampleValue) {
	t.Helper()
	got, ok := r.Delta(name, labels)
	require.True(t, ok, "expected delta for %s", name)
	require.Equal(t, want, got, "unexpected delta for %s", name)
}

func mustNoDelta(t *testing.T, r Reader, name string, labels Labels) {
	t.Helper()
	_, ok := r.Delta(name, labels)
	require.False(t, ok, "expected no delta for %s", name)
}
