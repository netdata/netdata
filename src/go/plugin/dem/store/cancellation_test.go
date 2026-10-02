// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoryQueriesCancelWhileWaitingForDatabaseConnection(t *testing.T) {
	queries := map[string]func(context.Context, *Store) error{
		"sessions": func(ctx context.Context, st *Store) error {
			_, err := st.QueryRumSessions(ctx, "site", 0, 1000, 10)
			return err
		},
		"session events": func(ctx context.Context, st *Store) error {
			_, err := st.QueryRumSessionEvents(ctx, "site", "session")
			return err
		},
		"errors": func(ctx context.Context, st *Store) error {
			_, err := st.QueryRumErrors(ctx, "site", 0, 1000)
			return err
		},
	}
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			st, err := Open(context.Background(), "")
			require.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, st.Close()) })
			record := RumSessionRecord{
				Site:      "site",
				SessionID: "session",
				StartedAt: 100,
				LastAt:    200,
				Pageviews: 1,
			}
			_, _, err = st.WriteRumHistoryBatch(
				context.Background(),
				RumHistoryBatch{
					Sessions: []RumSessionRecord{record},
				},
			)
			require.NoError(t, err)

			// Occupy the actual sole connection. The query must wait in the
			// database pool rather than succeed against an empty fixture.
			conn, err := st.db.Conn(context.Background())
			require.NoError(t, err)
			defer conn.Close()
			waits := st.db.Stats().WaitCount
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- query(ctx, st) }()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.DeadlineExceeded)
			case <-time.After(time.Second):
				// Release this test's connection before failing so the query
				// cannot leave a blocked consumer behind.
				require.NoError(t, conn.Close())
				<-done
				t.Fatal("query did not honor its deadline while waiting for the connection")
			}
			assert.Greater(t, st.db.Stats().WaitCount, waits)
			require.NoError(t, conn.Close())

			require.NoError(t, query(context.Background(), st))
			rows, err := st.QueryRumSessions(context.Background(), "site", 0, 1000, 10)
			require.NoError(t, err)
			assert.Equal(t, []RumSessionRecord{record}, rows)
		})
	}
}
