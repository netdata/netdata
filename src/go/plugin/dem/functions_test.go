// SPDX-License-Identifier: GPL-3.0-or-later
package dem

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	rumfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoryFunctionsRemainAvailableWithoutActiveSites(t *testing.T) {
	journalStore, err := journal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	st := history.NewStore(journalStore)
	now := time.Now().Unix()
	_, err = st.AppendEvent(context.Background(), history.EventRecord{
		Site:      "retired",
		SessionID: "session",
		TSUnixUS:  now * 1e6,
		Type:      "pageview",
		Page:      "/checkout",
	})
	require.NoError(t, err)
	components := New(Dependencies{
		History: journalStore,
	}, DefaultConfig())
	handler := components.Functions[0].NewHandler().(*rumfunctions.Handler)
	response := handler.HandleRaw(
		context.Background(),
		funcapi.RawMethodRequest{
			Method: "rum-sessions",
			Args:   []string{"site:retired"},
		},
	)
	require.NotNil(t, response.RawResponse)
	rows := response.RawResponse["data"].([][]any)
	require.Len(t, rows, 1)
	assert.Equal(t, "retired", rows[0][0])
	response = handler.HandleRaw(
		context.Background(),
		funcapi.RawMethodRequest{
			Method: "rum-session-events",
			Args:   []string{"site:retired", "session_id:session"},
		},
	)
	require.Len(t, response.RawResponse["data"].([][]any), 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (query.New(rumregistry.New(), st)).Sessions(ctx, "", 0, now)
	assert.ErrorIs(t, err, context.Canceled)
}
