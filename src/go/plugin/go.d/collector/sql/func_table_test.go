// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"context"
	"database/sql"
	"io"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
)

func TestNormalizeValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{"nil", nil, nil},
		{"string", "hello", "hello"},
		{"int64", int64(42), int64(42)},
		{"int", int(42), int64(42)},
		{"int32", int32(42), int64(42)},
		{"float64", float64(3.14), float64(3.14)},
		{"float32", float32(3.14), float64(float32(3.14))},
		{"bytes", []byte("hello"), "hello"},
		{"bool", true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := normalizeValue(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestNormalizeValue_Time(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	result := normalizeValue(ts)
	assert.Equal(t, ts.UnixMilli(), result)
}

func TestConfigFunction_Validate(t *testing.T) {
	tests := []struct {
		name        string
		cfg         ConfigFunction
		expectError bool
		errorCount  int
	}{
		{
			name:        "valid config",
			cfg:         ConfigFunction{ID: "test", Query: "SELECT 1"},
			expectError: false,
		},
		{
			name:        "valid config with limit",
			cfg:         ConfigFunction{ID: "test", Query: "SELECT 1", Limit: 500},
			expectError: false,
		},
		{
			name:        "missing id",
			cfg:         ConfigFunction{Query: "SELECT 1"},
			expectError: true,
			errorCount:  1,
		},
		{
			name:        "missing query",
			cfg:         ConfigFunction{ID: "test"},
			expectError: true,
			errorCount:  1,
		},
		{
			name:        "missing both",
			cfg:         ConfigFunction{},
			expectError: true,
			errorCount:  2,
		},
		{
			name:        "id contains colon",
			cfg:         ConfigFunction{ID: "test:query", Query: "SELECT 1"},
			expectError: true,
			errorCount:  1,
		},
		{
			name:        "negative limit",
			cfg:         ConfigFunction{ID: "test", Query: "SELECT 1", Limit: -1},
			expectError: true,
			errorCount:  1,
		},
		{
			name:        "limit exceeds maximum",
			cfg:         ConfigFunction{ID: "test", Query: "SELECT 1", Limit: 10001},
			expectError: true,
			errorCount:  1,
		},
		{
			name:        "negative timeout",
			cfg:         ConfigFunction{ID: "test", Query: "SELECT 1", Timeout: confopt.Duration(-time.Second)},
			expectError: true,
			errorCount:  1,
		},
		{
			name: "invalid column type",
			cfg: ConfigFunction{
				ID:      "test",
				Query:   "SELECT 1",
				Columns: map[string]ConfigFuncColumn{"col1": {Type: "invalid"}},
			},
			expectError: true,
			errorCount:  1,
		},
		{
			name: "valid column types",
			cfg: ConfigFunction{
				ID:    "test",
				Query: "SELECT 1",
				Columns: map[string]ConfigFuncColumn{
					"col1": {Type: "string"},
					"col2": {Type: "integer"},
					"col3": {Type: "float"},
					"col4": {Type: "boolean"},
					"col5": {Type: "duration"},
					"col6": {Type: "timestamp"},
				},
			},
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			seen := make(map[string]bool)
			errs := tc.cfg.validate(0, seen)
			if tc.expectError {
				assert.Len(t, errs, tc.errorCount)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestConfigFunction_Validate_DuplicateID(t *testing.T) {
	seen := map[string]bool{"existing": true}
	cfg := ConfigFunction{ID: "existing", Query: "SELECT 1"}

	errs := cfg.validate(0, seen)
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "duplicate id")
}

func TestFuncTable_FindFunction(t *testing.T) {
	c := &Collector{}
	c.Config.Functions = []ConfigFunction{
		{ID: "func1", Query: "SELECT 1"},
		{ID: "func2", Query: "SELECT 2"},
	}

	ft := &funcTable{collector: c}

	// Found
	f := ft.findFunction("func1")
	assert.NotNil(t, f)
	assert.Equal(t, "func1", f.ID)

	f = ft.findFunction("func2")
	assert.NotNil(t, f)
	assert.Equal(t, "func2", f.ID)

	// Not found
	f = ft.findFunction("nonexistent")
	assert.Nil(t, f)
}

func TestSqlJobMethods(t *testing.T) {
	tests := map[string]struct {
		setupCollector func() *Collector
		jobName        string
		wantLen        int
		wantMethodIDs  []string
	}{
		"collector with single function": {
			setupCollector: func() *Collector {
				c := New()
				c.Config.Functions = []ConfigFunction{
					{ID: "test-query", Query: "SELECT 1", Description: "Test query"},
				}
				return c
			},
			jobName:       "postgres_test",
			wantLen:       1,
			wantMethodIDs: []string{"postgres_test:test-query"},
		},
		"collector with multiple functions": {
			setupCollector: func() *Collector {
				c := New()
				c.Config.Functions = []ConfigFunction{
					{ID: "active-queries", Query: "SELECT 1"},
					{ID: "databases", Query: "SELECT 2"},
					{ID: "roles", Query: "SELECT 3"},
				}
				return c
			},
			jobName: "pg_main",
			wantLen: 3,
			wantMethodIDs: []string{
				"pg_main:active-queries",
				"pg_main:databases",
				"pg_main:roles",
			},
		},
		"collector without functions": {
			setupCollector: func() *Collector {
				return New()
			},
			jobName: "empty_job",
			wantLen: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := tc.setupCollector()
			job := jobruntime.NewJob(jobruntime.JobConfig{
				Name:       tc.jobName,
				ModuleName: "sql",
				FullName:   "sql_" + tc.jobName,
				Module:     c,
				Out:        io.Discard,
			})
			methods := sqlJobMethods(job)

			assert.Len(t, methods, tc.wantLen)
			for i, wantID := range tc.wantMethodIDs {
				assert.Equal(t, wantID, methods[i].ID)
				assert.Equal(t, 10, methods[i].UpdateEvery)
			}
		})
	}
}

func TestFuncTable_MethodParams(t *testing.T) {
	// MethodParams now always returns nil since each function is a separate endpoint
	c := New()
	c.db, _, _ = sqlmock.New()
	c.Config.Functions = []ConfigFunction{
		{ID: "func1", Query: "SELECT 1"},
		{ID: "func2", Query: "SELECT 2"},
	}
	defer func() { _ = c.db.Close() }()

	ft := &funcTable{collector: c}
	params, err := ft.MethodParams(context.Background(), "postgres_test:func1")

	require.NoError(t, err)
	assert.Nil(t, params)
}

func TestFuncTable_Handle(t *testing.T) {
	tests := map[string]struct {
		functions   []ConfigFunction
		functionID  string
		dbNil       bool
		prepareMock func(sqlmock.Sqlmock)
		checkResp   func(*testing.T, *funcapi.FunctionResponse)
	}{
		"db not initialized": {
			functions:  []ConfigFunction{{ID: "test", Query: "SELECT 1"}},
			functionID: "test",
			dbNil:      true,
			checkResp: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 503, resp.Status)
				assert.Contains(t, resp.Message, "not initialized")
			},
		},
		"unknown function": {
			functions:  []ConfigFunction{{ID: "known", Query: "SELECT 1"}},
			functionID: "unknown",
			checkResp: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 404, resp.Status)
				assert.Contains(t, resp.Message, "unknown function")
			},
		},
		"successful query": {
			functions: []ConfigFunction{
				{ID: "test", Query: "SELECT id, name FROM users", Description: "Test query"},
			},
			functionID: "test",
			prepareMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name"}).
					AddRow(1, "Alice").
					AddRow(2, "Bob")
				m.ExpectQuery("SELECT id, name FROM users").WillReturnRows(rows)
			},
			checkResp: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				assert.Equal(t, "Test query", resp.Help)
				assert.Len(t, resp.Data, 2)
			},
		},
		"query error": {
			functions:  []ConfigFunction{{ID: "test", Query: "SELECT 1"}},
			functionID: "test",
			prepareMock: func(m sqlmock.Sqlmock) {
				m.ExpectQuery("SELECT 1").WillReturnError(assert.AnError)
			},
			checkResp: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 500, resp.Status)
				assert.Contains(t, resp.Message, "query failed")
			},
		},
		"limit applied": {
			functions: []ConfigFunction{
				{ID: "test", Query: "SELECT n", Limit: 2},
			},
			functionID: "test",
			prepareMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"n"}).
					AddRow(1).AddRow(2).AddRow(3).AddRow(4).AddRow(5)
				m.ExpectQuery("SELECT n").WillReturnRows(rows)
			},
			checkResp: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				assert.Len(t, resp.Data, 2) // limited to 2
			},
		},
		"default limit applied": {
			functions: []ConfigFunction{
				{ID: "test", Query: "SELECT n"}, // no limit set
			},
			functionID: "test",
			prepareMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"n"})
				for i := 0; i < 150; i++ {
					rows.AddRow(i)
				}
				m.ExpectQuery("SELECT n").WillReturnRows(rows)
			},
			checkResp: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				assert.Len(t, resp.Data, defaultFunctionLimit) // 100
			},
		},
		"default_sort valid column": {
			functions: []ConfigFunction{
				{ID: "test", Query: "SELECT id, name", DefaultSort: "id"},
			},
			functionID: "test",
			prepareMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "test")
				m.ExpectQuery("SELECT id, name").WillReturnRows(rows)
			},
			checkResp: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				assert.Equal(t, "id", resp.DefaultSortColumn)
			},
		},
		"default_sort invalid column": {
			functions: []ConfigFunction{
				{ID: "test", Query: "SELECT id, name", DefaultSort: "nonexistent"},
			},
			functionID: "test",
			prepareMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "test")
				m.ExpectQuery("SELECT id, name").WillReturnRows(rows)
			},
			checkResp: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				assert.Equal(t, "", resp.DefaultSortColumn) // cleared because invalid
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Config.Functions = tc.functions

			var mock sqlmock.Sqlmock
			if !tc.dbNil {
				var db *sql.DB
				var err error
				db, mock, err = sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
				require.NoError(t, err)
				defer func() { _ = db.Close() }()
				c.db = db

				if tc.prepareMock != nil {
					tc.prepareMock(mock)
				}
			}

			ft := &funcTable{collector: c}

			// Method format is "jobName:functionID" - function ID is extracted from method
			method := "test_job:" + tc.functionID
			resp := ft.Handle(context.Background(), method, nil)

			tc.checkResp(t, resp)
			if mock != nil {
				assert.NoError(t, mock.ExpectationsWereMet())
			}
		})
	}
}

func TestInferType(t *testing.T) {
	tests := []struct {
		dbType   string
		expected funcapi.FieldType
	}{
		// MySQL (uppercase)
		{"INT", funcapi.FieldTypeInteger},
		{"BIGINT", funcapi.FieldTypeInteger},
		{"VARCHAR", funcapi.FieldTypeString},
		{"DATETIME", funcapi.FieldTypeTimestamp},
		{"TIME", funcapi.FieldTypeDuration},
		{"FLOAT", funcapi.FieldTypeFloat},
		{"DOUBLE", funcapi.FieldTypeFloat},
		{"TEXT", funcapi.FieldTypeString},

		// PostgreSQL (lowercase)
		{"int4", funcapi.FieldTypeInteger},
		{"int8", funcapi.FieldTypeInteger},
		{"varchar", funcapi.FieldTypeString},
		{"text", funcapi.FieldTypeString},
		{"timestamp", funcapi.FieldTypeTimestamp},
		{"timestamptz", funcapi.FieldTypeTimestamp},
		{"bool", funcapi.FieldTypeBoolean},
		{"boolean", funcapi.FieldTypeBoolean},
		{"float8", funcapi.FieldTypeFloat},
		{"numeric", funcapi.FieldTypeFloat},
		{"interval", funcapi.FieldTypeDuration},

		// SQL Server
		{"NVARCHAR", funcapi.FieldTypeString},
		{"DATETIME2", funcapi.FieldTypeTimestamp},
		{"BIT", funcapi.FieldTypeBoolean},
		{"REAL", funcapi.FieldTypeFloat},

		// Oracle
		{"VARCHAR2", funcapi.FieldTypeString},
		{"NUMBER", funcapi.FieldTypeFloat},
		{"BINARY_DOUBLE", funcapi.FieldTypeFloat},

		// Case insensitive
		{"int", funcapi.FieldTypeInteger},
		{"Int", funcapi.FieldTypeInteger},
		{"VARCHAR", funcapi.FieldTypeString},
		{"Varchar", funcapi.FieldTypeString},

		// Unknown -> string
		{"UNKNOWN_TYPE", funcapi.FieldTypeString},
		{"custom", funcapi.FieldTypeString},
		{"", funcapi.FieldTypeString},
	}

	for _, tc := range tests {
		t.Run(tc.dbType, func(t *testing.T) {
			result := inferType(tc.dbType)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestParseFieldType(t *testing.T) {
	tests := []struct {
		input    string
		expected funcapi.FieldType
	}{
		{"string", funcapi.FieldTypeString},
		{"STRING", funcapi.FieldTypeString},
		{"integer", funcapi.FieldTypeInteger},
		{"INTEGER", funcapi.FieldTypeInteger},
		{"float", funcapi.FieldTypeFloat},
		{"FLOAT", funcapi.FieldTypeFloat},
		{"boolean", funcapi.FieldTypeBoolean},
		{"BOOLEAN", funcapi.FieldTypeBoolean},
		{"duration", funcapi.FieldTypeDuration},
		{"DURATION", funcapi.FieldTypeDuration},
		{"timestamp", funcapi.FieldTypeTimestamp},
		{"TIMESTAMP", funcapi.FieldTypeTimestamp},
		{"unknown", funcapi.FieldTypeString},
		{"", funcapi.FieldTypeString},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := parseFieldType(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestDeriveTransform(t *testing.T) {
	tests := []struct {
		fieldType funcapi.FieldType
		expected  funcapi.FieldTransform
	}{
		{funcapi.FieldTypeInteger, funcapi.FieldTransformNumber},
		{funcapi.FieldTypeFloat, funcapi.FieldTransformNumber},
		{funcapi.FieldTypeDuration, funcapi.FieldTransformDuration},
		{funcapi.FieldTypeTimestamp, funcapi.FieldTransformDatetime},
		{funcapi.FieldTypeString, funcapi.FieldTransformNone},
		{funcapi.FieldTypeBoolean, funcapi.FieldTransformNone},
	}

	for _, tc := range tests {
		t.Run(tc.fieldType.String(), func(t *testing.T) {
			result := deriveTransform(tc.fieldType)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestDeriveFilterSummary(t *testing.T) {
	tests := []struct {
		fieldType       funcapi.FieldType
		expectedFilter  funcapi.FieldFilter
		expectedSummary funcapi.FieldSummary
	}{
		{funcapi.FieldTypeInteger, funcapi.FieldFilterRange, funcapi.FieldSummarySum},
		{funcapi.FieldTypeFloat, funcapi.FieldFilterRange, funcapi.FieldSummarySum},
		{funcapi.FieldTypeDuration, funcapi.FieldFilterRange, funcapi.FieldSummarySum},
		{funcapi.FieldTypeTimestamp, funcapi.FieldFilterRange, funcapi.FieldSummaryMax},
		{funcapi.FieldTypeBoolean, funcapi.FieldFilterMultiselect, funcapi.FieldSummaryCount},
		{funcapi.FieldTypeString, funcapi.FieldFilterMultiselect, funcapi.FieldSummaryCount},
	}

	for _, tc := range tests {
		t.Run(tc.fieldType.String(), func(t *testing.T) {
			filter, summary := deriveFilterSummary(tc.fieldType)
			assert.Equal(t, tc.expectedFilter, filter)
			assert.Equal(t, tc.expectedSummary, summary)
		})
	}
}

func TestDeriveNameFromID(t *testing.T) {
	tests := []struct {
		id       string
		expected string
	}{
		{"slow-queries", "Slow Queries"},
		{"top_connections", "Top Connections"},
		{"active-sessions", "Active Sessions"},
		{"simple", "Simple"},
		{"multi-word-id", "Multi Word Id"},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			result := deriveNameFromID(tc.id)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestConfigFunction_derivedName(t *testing.T) {
	tests := []struct {
		name     string
		cfg      ConfigFunction
		expected string
	}{
		{
			name:     "uses explicit name",
			cfg:      ConfigFunction{ID: "slow-queries", Name: "My Custom Name"},
			expected: "My Custom Name",
		},
		{
			name:     "derives from ID",
			cfg:      ConfigFunction{ID: "slow-queries"},
			expected: "Slow Queries",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.cfg.derivedName()
			assert.Equal(t, tc.expected, result)
		})
	}
}
