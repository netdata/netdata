// SPDX-License-Identifier: GPL-3.0-or-later
package history

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/stretchr/testify/require"
)

func TestWriterRoundTripsMeasurementIdentity(t *testing.T) {
	store, _ := newTestStore(t)
	writer := NewWriter("shop", store, newFakeCounters())
	now := time.Now().UnixMicro()
	records := []aggregate.HistoryEvent{
		{Site: "shop", SessionID: "session", TSUnixUS: now, Type: "pageview", Page: "/entry", ExperienceID: "document", View: "checkout", ViewID: "view-one", Revision: 1},
		{Site: "shop", SessionID: "session", TSUnixUS: now + 1, Type: "view", Page: "/entry", ExperienceID: "document", View: "payment", ViewID: "view-two", Revision: 2},
		{Site: "shop", SessionID: "session", TSUnixUS: now + 2, Type: "vital", Page: "/entry", ExperienceID: "document", View: "payment", ViewID: "view-two", MetricID: "metric", Revision: 3},
	}
	for _, record := range records {
		writer.Event(record)
	}
	drain(writer)
	events, err := store.QuerySessionEvents(context.Background(), "shop", "session")
	require.NoError(t, err)
	require.Equal(t, []SessionEventRecord{
		{Site: "shop", SessionID: "session", TSUnixUS: now, Type: "pageview", Page: "/entry", ExperienceID: "document", View: "checkout", ViewID: "view-one", Revision: 1},
		{Site: "shop", SessionID: "session", TSUnixUS: now + 1, Type: "view", Page: "/entry", ExperienceID: "document", View: "payment", ViewID: "view-two", Revision: 2},
		{Site: "shop", SessionID: "session", TSUnixUS: now + 2, Type: "vital", Page: "/entry", ExperienceID: "document", View: "payment", ViewID: "view-two", MetricID: "metric", Revision: 3},
	}, events)
	sessions, err := store.QuerySessions(context.Background(), "shop", "", 0, time.Now().Unix()+1, 10)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.EqualValues(t, 1, sessions[0].Pageviews)
	require.EqualValues(t, 1, sessions[0].ApplicationViews)
}
