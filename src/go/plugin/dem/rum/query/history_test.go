// SPDX-License-Identifier: GPL-3.0-or-later

package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The history store reports cancellation before reading; the query service
// must return that error rather than an empty result.
func TestSessionsReturnHistoryCancellation(t *testing.T) {
	journalStore, err := journal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = query.New(rumregistry.New(), history.NewStore(journalStore)).Sessions(ctx, "", "", 0, time.Now().Unix(), "")
	assert.ErrorIs(t, err, context.Canceled)
}
