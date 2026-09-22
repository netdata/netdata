// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package mssqlfunc

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Execute both production queries with controlled file rows. Prefix overlap must not let
// another session displace the selected session's latest event. No server objects are created.
func TestIntegration_XEventFileIsolation(t *testing.T) {
	db, err := sql.Open("sqlserver", getDSN(t))
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for name, tc := range map[string]struct {
		query     string
		eventName string
	}{
		"deadlock": {query: querySystemHealthLatestDeadlockEventFile, eventName: "xml_deadlock_report"},
		"error":    {query: queryMSSQLErrorInfoEventFile, eventName: "error_reported"},
	} {
		t.Run(name, func(t *testing.T) {
			for pathName, path := range map[string]string{
				"Windows":    `C:\Logs\audit.xel`,
				"Linux":      "/var/opt/mssql/log/audit.xel",
				"Azure blob": "https://storage.example/container/audit.xel",
			} {
				t.Run(pathName, func(t *testing.T) {
					target := eventFileReadTarget(path)
					prefix := strings.TrimSuffix(path, ".xel")
					args := append(target.fileQueryArgs(tc.eventName),
						sql.Named("limit", 1),
						sql.Named("ownFile", prefix+"_0_134000000000000000.xel"),
						sql.Named("otherFile", prefix+"_0_other_0_134000000000000001.xel"),
						sql.Named("numericPrefixFile", prefix+"_0_0_134000000000000002.xel"),
						sql.Named("expectedEvent", xeventIsolationEvent("expected", "2026-01-01T00:00:00Z")),
						sql.Named("foreignEvent", xeventIsolationEvent("foreign", "2026-01-02T00:00:00Z")),
					)
					query := strings.Replace(
						tc.query,
						"sys.fn_xe_file_target_read_file(@filePath, NULL, NULL, NULL)",
						"@fileEvents",
						1,
					)
					rows, err := db.QueryContext(ctx, xeventIsolationFiles+query, args...)
					require.NoError(t, err)
					defer rows.Close()
					require.True(t, rows.Next())
					var timestamp time.Time
					if tc.eventName == "xml_deadlock_report" {
						var graph string
						require.NoError(t, rows.Scan(&timestamp, &graph))
						assert.Contains(t, graph, `id="expected"`)
					} else {
						var number, state int
						var message, text, hash string
						require.NoError(t, rows.Scan(&timestamp, &number, &state, &message, &text, &hash))
						assert.Equal(t, "expected", message)
					}
					assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), timestamp.UTC())
					assert.False(t, rows.Next())
					require.NoError(t, rows.Err())
				})
			}
		})
	}
}

const xeventIsolationFiles = `
DECLARE @fileEvents TABLE (file_name nvarchar(2048), event_data nvarchar(max), object_name nvarchar(128));
INSERT INTO @fileEvents VALUES
  (@ownFile, @expectedEvent, @eventName),
  (@otherFile, @foreignEvent, @eventName),
  (@numericPrefixFile, @foreignEvent, @eventName),
  (@ownFile, @foreignEvent, 'unrelated_event');
`

func xeventIsolationEvent(value, timestamp string) string {
	return `<event timestamp="` + timestamp + `"><data name="xml_report"><value><deadlock><process-list><process id="` + value +
		`"/></process-list></deadlock></value></data><data name="error_number"><value>50000</value></data>` +
		`<data name="state"><value>1</value></data><data name="message"><value>` + value + `</value></data>` +
		`<action name="sql_text"><value>SELECT 1</value></action><action name="query_hash"><value>1</value></action></event>`
}
