// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	sdk "github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObservedKindIgnoresRejectedAppend(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Append(t.Context(), []sdk.Field{sdk.StringField("DEM_KIND", "rum")})
	require.Error(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = s.Append(ctx, testFields())
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, s.HasObservedKind("rum"))
	assert.False(t, s.HasObservedKind("synthetic"))
	assert.False(t, s.HasObservedKind("unknown"))
}

func TestObservedKindAfterRetentionAndRestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		root := t.TempDir()
		s, err := Open(t.Context(), root)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, s.Close()) })
		appendEntry(t, s)
		require.True(t, s.HasObservedKind("rum"))
		time.Sleep(48 * time.Hour)
		require.NoError(t, s.EnforceHistoryRetention(t.Context(), 1, 1<<30))
		require.Zero(t, countEntries(t, s))
		assert.True(t, s.HasObservedKind("rum"))
		require.NoError(t, s.Close())
		s, err = Open(t.Context(), root)
		require.NoError(t, err)
		assert.False(t, s.HasObservedKind("rum"))
		assert.False(t, s.HasObservedKind("synthetic"))
	})
}
