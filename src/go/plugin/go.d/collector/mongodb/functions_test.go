// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMongoMethods(t *testing.T) {
	methods := mongoMethods()

	assert.Len(t, methods, 1)
	assert.Equal(t, "top-queries", methods[0].ID)
	assert.Equal(t, "Top Queries", methods[0].Name)
	assert.Contains(t, methods[0].Help, "WARNING")
	assert.Contains(t, methods[0].Help, "PII")

	var sortParam *funcapi.ParamConfig
	for i := range methods[0].RequiredParams {
		if methods[0].RequiredParams[i].ID == "__sort" {
			sortParam = &methods[0].RequiredParams[i]
			break
		}
	}
	require.NotNil(t, sortParam, "should have __sort param")
	require.NotEmpty(t, sortParam.Options, "should have sort options")

	// Check default sort option
	defaultFound := false
	for _, opt := range sortParam.Options {
		if opt.Default {
			defaultFound = true
			assert.Equal(t, "execution_time", opt.ID)
			assert.Equal(t, "millis", opt.Column)
		}
	}
	assert.True(t, defaultFound, "should have a default sort option")
}

func TestMongoColumnMeta(t *testing.T) {
	// Check that mongoAllColumns has all expected columns
	assert.NotEmpty(t, mongoAllColumns, "should have column definitions")

	// Check required core columns exist
	coreColumns := []string{"timestamp", "namespace", "operation", "query", "execution_time", "docs_examined", "keys_examined", "docs_returned", "plan_summary"}
	for _, colID := range coreColumns {
		found := false
		for _, col := range mongoAllColumns {
			if col.id == colID {
				found = true
				break
			}
		}
		assert.True(t, found, "core column %s should exist", colID)
	}

	// Check that core columns are visible by default
	for _, col := range mongoAllColumns {
		switch col.id {
		case "timestamp", "namespace", "operation", "query", "execution_time", "docs_examined", "keys_examined", "docs_returned", "plan_summary":
			assert.True(t, col.visible, "column %s should be visible by default", col.id)
		}
	}
}

func TestBuildAvailableMongoColumns(t *testing.T) {
	tests := []struct {
		name      string
		available map[string]bool
		expectLen int
	}{
		{
			name: "core fields only",
			available: map[string]bool{
				"ts": true, "ns": true, "op": true, "command": true, "millis": true,
			},
			expectLen: 5, // timestamp, namespace, operation, query, execution_time
		},
		{
			name: "with extra fields",
			available: map[string]bool{
				"ts": true, "ns": true, "op": true, "command": true, "millis": true,
				"docsExamined": true, "keysExamined": true, "planSummary": true,
			},
			expectLen: 8,
		},
		{
			name: "all fields",
			available: func() map[string]bool {
				m := make(map[string]bool)
				for _, col := range mongoAllColumns {
					m[col.dbField] = true
				}
				return m
			}(),
			expectLen: len(mongoAllColumns),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cols := buildAvailableMongoColumns(tc.available)
			assert.Len(t, cols, tc.expectLen)
		})
	}
}

func TestBuildMongoColumnsFromMeta(t *testing.T) {
	// Use a subset of columns for testing
	testCols := []mongoColumnMeta{
		{id: "timestamp", dbField: "ts", name: "Timestamp", colType: ftTimestamp, visible: true, sortable: true, filter: filterRange, visualization: visValue, summary: summaryMax, transform: trDatetime, uniqueKey: true},
		{id: "execution_time", dbField: "millis", name: "Execution Time", colType: ftDuration, visible: true, sortable: true, filter: filterRange, visualization: visBar, summary: summarySum, transform: trDuration, units: "seconds", decimalPoints: 3},
		{id: "query", dbField: "command", name: "Query", colType: ftString, visible: true, sortable: false, fullWidth: true, wrap: true, filter: filterText, visualization: visValue, transform: trText},
	}

	cols := buildMongoColumnsFromMeta(testCols)

	// Check required columns exist
	assert.Contains(t, cols, "timestamp")
	assert.Contains(t, cols, "execution_time")
	assert.Contains(t, cols, "query")

	// Check execution_time column properties
	execTimeVal, ok := cols["execution_time"]
	require.True(t, ok, "execution_time column must exist")
	execTime, ok := execTimeVal.(map[string]any)
	require.True(t, ok, "execution_time must be map[string]any")
	assert.Equal(t, "duration", execTime["type"])
	assert.Equal(t, true, execTime["visible"])
	assert.Equal(t, "seconds", execTime["units"])

	// Check query column properties
	queryVal, ok := cols["query"]
	require.True(t, ok, "query column must exist")
	query, ok := queryVal.(map[string]any)
	require.True(t, ok, "query must be map[string]any")
	assert.Equal(t, true, query["full_width"])
	assert.Equal(t, false, query["sortable"])
}

func TestSortProfileDocuments(t *testing.T) {
	docs := []profileDocument{
		{Millis: 100, DocsExamined: 50, KeysExamined: 10, Nreturned: 5},
		{Millis: 500, DocsExamined: 200, KeysExamined: 30, Nreturned: 20},
		{Millis: 200, DocsExamined: 100, KeysExamined: 20, Nreturned: 10},
	}

	tests := []struct {
		name       string
		sortColumn string
		expected   []int64 // expected order of millis values after sort
	}{
		{
			name:       "sort by millis",
			sortColumn: "millis",
			expected:   []int64{500, 200, 100},
		},
		{
			name:       "sort by docsExamined",
			sortColumn: "docsExamined",
			expected:   []int64{500, 200, 100}, // 200, 100, 50 -> millis 500, 200, 100
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Make a copy to avoid modifying original
			docsCopy := make([]profileDocument, len(docs))
			copy(docsCopy, docs)

			sortProfileDocuments(docsCopy, tc.sortColumn)

			for i, expectedMillis := range tc.expected {
				assert.Equal(t, expectedMillis, docsCopy[i].Millis,
					"position %d should have millis=%d", i, expectedMillis)
			}
		})
	}
}

func TestSortProfileDocumentsByTimestamp(t *testing.T) {
	now := time.Now()
	docs := []profileDocument{
		{Timestamp: now.Add(-time.Hour), Millis: 100},
		{Timestamp: now, Millis: 200},
		{Timestamp: now.Add(-30 * time.Minute), Millis: 300},
	}

	sortProfileDocuments(docs, "ts")

	// Should be sorted by timestamp descending (most recent first)
	assert.Equal(t, int64(200), docs[0].Millis) // now
	assert.Equal(t, int64(300), docs[1].Millis) // -30min
	assert.Equal(t, int64(100), docs[2].Millis) // -1hr
}

func TestSortProfileDocumentsByNewColumns(t *testing.T) {
	docs := []profileDocument{
		{Millis: 100, Ndeleted: 5, Ninserted: 10, NModified: 15, ResponseLength: 100, NumYield: 1},
		{Millis: 200, Ndeleted: 15, Ninserted: 5, NModified: 10, ResponseLength: 300, NumYield: 3},
		{Millis: 300, Ndeleted: 10, Ninserted: 15, NModified: 5, ResponseLength: 200, NumYield: 2},
	}

	tests := []struct {
		name       string
		sortColumn string
		expected   []int64 // expected order of millis values after sort
	}{
		{
			name:       "sort by ndeleted",
			sortColumn: "ndeleted",
			expected:   []int64{200, 300, 100}, // 15, 10, 5
		},
		{
			name:       "sort by ninserted",
			sortColumn: "ninserted",
			expected:   []int64{300, 100, 200}, // 15, 10, 5
		},
		{
			name:       "sort by nModified",
			sortColumn: "nModified",
			expected:   []int64{100, 200, 300}, // 15, 10, 5
		},
		{
			name:       "sort by responseLength",
			sortColumn: "responseLength",
			expected:   []int64{200, 300, 100}, // 300, 200, 100
		},
		{
			name:       "sort by numYield",
			sortColumn: "numYield",
			expected:   []int64{200, 300, 100}, // 3, 2, 1
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			docsCopy := make([]profileDocument, len(docs))
			copy(docsCopy, docs)

			sortProfileDocuments(docsCopy, tc.sortColumn)

			for i, expectedMillis := range tc.expected {
				assert.Equal(t, expectedMillis, docsCopy[i].Millis,
					"position %d should have millis=%d", i, expectedMillis)
			}
		})
	}
}

func TestSortProfileDocumentsByOptionalColumns(t *testing.T) {
	pt1 := int64(100)
	pt2 := int64(300)
	pt3 := int64(200)
	cpu1 := int64(1000)
	cpu2 := int64(3000)
	cpu3 := int64(2000)

	docs := []profileDocument{
		{Millis: 100, PlanningTimeMicros: &pt1, CpuNanos: &cpu1},
		{Millis: 200, PlanningTimeMicros: &pt2, CpuNanos: &cpu2},
		{Millis: 300, PlanningTimeMicros: &pt3, CpuNanos: &cpu3},
	}

	t.Run("sort by planningTimeMicros", func(t *testing.T) {
		docsCopy := make([]profileDocument, len(docs))
		copy(docsCopy, docs)

		sortProfileDocuments(docsCopy, "planningTimeMicros")

		// Should be sorted by planningTimeMicros descending: 300, 200, 100
		assert.Equal(t, int64(200), docsCopy[0].Millis) // pt2=300
		assert.Equal(t, int64(300), docsCopy[1].Millis) // pt3=200
		assert.Equal(t, int64(100), docsCopy[2].Millis) // pt1=100
	})

	t.Run("sort by cpuNanos", func(t *testing.T) {
		docsCopy := make([]profileDocument, len(docs))
		copy(docsCopy, docs)

		sortProfileDocuments(docsCopy, "cpuNanos")

		// Should be sorted by cpuNanos descending: 3000, 2000, 1000
		assert.Equal(t, int64(200), docsCopy[0].Millis) // cpu2=3000
		assert.Equal(t, int64(300), docsCopy[1].Millis) // cpu3=2000
		assert.Equal(t, int64(100), docsCopy[2].Millis) // cpu1=1000
	})

	t.Run("sort by planningTimeMicros with nil values", func(t *testing.T) {
		pt := int64(100)
		docsWithNil := []profileDocument{
			{Millis: 100, PlanningTimeMicros: nil},
			{Millis: 200, PlanningTimeMicros: &pt},
			{Millis: 300, PlanningTimeMicros: nil},
		}

		sortProfileDocuments(docsWithNil, "planningTimeMicros")

		// Non-nil values should come first when sorted descending
		assert.Equal(t, int64(200), docsWithNil[0].Millis) // has pt=100
	})
}

func TestConfigGetTopQueriesFunctionEnabled(t *testing.T) {
	t.Run("nil returns true (default)", func(t *testing.T) {
		cfg := Config{TopQueriesFunctionEnabled: nil}
		assert.True(t, cfg.GetTopQueriesFunctionEnabled())
	})

	t.Run("explicit true returns true", func(t *testing.T) {
		enabled := true
		cfg := Config{TopQueriesFunctionEnabled: &enabled}
		assert.True(t, cfg.GetTopQueriesFunctionEnabled())
	})

	t.Run("explicit false returns false", func(t *testing.T) {
		disabled := false
		cfg := Config{TopQueriesFunctionEnabled: &disabled}
		assert.False(t, cfg.GetTopQueriesFunctionEnabled())
	})
}

func TestTopQueriesLimitDefault(t *testing.T) {
	// When TopQueriesLimit is 0 or negative, should use default
	assert.Equal(t, 500, defaultTopQueriesLimit)

	cfg := Config{TopQueriesLimit: 0}
	// collectTopQueries would use defaultTopQueriesLimit when config is 0
	limit := cfg.TopQueriesLimit
	if limit <= 0 {
		limit = defaultTopQueriesLimit
	}
	assert.Equal(t, 500, limit)

	cfg2 := Config{TopQueriesLimit: 100}
	limit2 := cfg2.TopQueriesLimit
	if limit2 <= 0 {
		limit2 = defaultTopQueriesLimit
	}
	assert.Equal(t, 100, limit2)
}

func TestOptionalBool(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		result := optionalBool(nil)
		assert.Nil(t, result)
	})

	t.Run("true returns Yes", func(t *testing.T) {
		v := true
		result := optionalBool(&v)
		assert.Equal(t, "Yes", result)
	})

	t.Run("false returns No", func(t *testing.T) {
		v := false
		result := optionalBool(&v)
		assert.Equal(t, "No", result)
	})
}

func TestOptionalDuration(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		result := optionalDuration(nil, 1000.0)
		assert.Nil(t, result)
	})

	t.Run("value converts correctly", func(t *testing.T) {
		v := int64(1000000)
		result := optionalDuration(&v, 1000000.0)
		assert.Equal(t, 1.0, result)
	})

	t.Run("microseconds to seconds", func(t *testing.T) {
		v := int64(500000) // 500,000 microseconds = 0.5 seconds
		result := optionalDuration(&v, 1000000.0)
		assert.Equal(t, 0.5, result)
	})

	t.Run("nanoseconds to seconds", func(t *testing.T) {
		v := int64(1500000000) // 1.5 billion nanoseconds = 1.5 seconds
		result := optionalDuration(&v, 1000000000.0)
		assert.Equal(t, 1.5, result)
	})
}
