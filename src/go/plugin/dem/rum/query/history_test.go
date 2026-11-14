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

func TestRetainedSessionsReportCancellation(t *testing.T) {
	journalStore, err := journal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	store := history.NewStore(journalStore)
	now := time.Now().Unix()
	_, err = store.AppendEvent(context.Background(), history.EventRecord{
		Site:       "retired",
		SessionID:  "session",
		ObservedUS: now * 1e6,
		Type:       "pageview",
		Page:       "/checkout",
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = query.New(rumregistry.New(), store).Sessions(ctx, "", "", 0, now, "")
	assert.ErrorIs(t, err, context.Canceled)
}
