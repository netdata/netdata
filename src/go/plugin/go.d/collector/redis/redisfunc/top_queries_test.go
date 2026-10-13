// SPDX-License-Identifier: GPL-3.0-or-later

package redisfunc

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

func TestMethods(t *testing.T) {
	methods := Methods()
	require.Len(t, methods, 1)

	method := methods[0]
	assert.Equal(t, topQueriesMethodID, method.ID)
	require.Len(t, method.RequiredParams, 1)

	sortParam := method.RequiredParams[0]
	assert.Equal(t, "__sort", sortParam.ID)

	var defaults []string
	for _, opt := range sortParam.Options {
		assert.Truef(t, topQueriesColumnSet().ContainsColumn(opt.Column), "sort option %q names no column", opt.ID)
		if opt.Default {
			defaults = append(defaults, opt.Column)
		}
	}
	assert.Equal(t, []string{topQueriesDefaultSort}, defaults, "exactly one default sort option, the response default")
}

func TestFuncTopQueries_Handle(t *testing.T) {
	base := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	entries := []redis.SlowLog{
		{ID: 1, Time: base.Add(3 * time.Second), Duration: 2 * time.Millisecond, Args: []string{"GET", "k"}},
		{ID: 2, Time: base.Add(1 * time.Second), Duration: 5 * time.Millisecond, Args: []string{"SET", "k", "v"}},
		{ID: 3, Time: base.Add(2 * time.Second), Duration: 1 * time.Millisecond, Args: []string{"DEL", "k"}},
	}
	// Every sort option yields a distinct order: duration [2 1 3], id [3 2 1], timestamp [1 3 2], command name [3 1 2].

	tests := map[string]struct {
		deps       Deps
		cfg        FunctionsConfig
		sort       string
		wantStatus int
		wantIDs    []int64
	}{
		"default sort is duration": {
			deps:       newTestDeps(entries, nil),
			wantStatus: 200,
			wantIDs:    []int64{2, 1, 3},
		},
		"sort by id": {
			deps:       newTestDeps(entries, nil),
			sort:       "id",
			wantStatus: 200,
			wantIDs:    []int64{3, 2, 1},
		},
		"sort by timestamp": {
			deps:       newTestDeps(entries, nil),
			sort:       "timestamp",
			wantStatus: 200,
			wantIDs:    []int64{1, 3, 2},
		},
		"sort by command name": {
			deps:       newTestDeps(entries, nil),
			sort:       "command_name",
			wantStatus: 200,
			wantIDs:    []int64{3, 1, 2},
		},
		"unknown sort falls back to duration": {
			deps:       newTestDeps(entries, nil),
			sort:       "duration;drop",
			wantStatus: 200,
			wantIDs:    []int64{2, 1, 3},
		},
		"limit keeps the top rows": {
			deps: newTestDeps(entries, nil),
			cfg: FunctionsConfig{
				TopQueries: TopQueriesConfig{
					Limit: 2,
				},
			},
			wantStatus: 200,
			wantIDs:    []int64{2, 1},
		},
		"empty slowlog": {
			deps:       newTestDeps(nil, nil),
			wantStatus: 200,
			wantIDs:    []int64{},
		},
		"disabled": {
			deps: newTestDeps(entries, nil),
			cfg: FunctionsConfig{
				TopQueries: TopQueriesConfig{
					Disabled: true,
				},
			},
			wantStatus: 503,
		},
		"client unavailable": {
			deps: testDeps{
				err: errors.New("no client"),
			},
			wantStatus: 503,
		},
		"slowlog error": {
			deps:       newTestDeps(nil, errors.New("ERR unknown command")),
			wantStatus: 500,
		},
		"zero limit uses the default": {
			deps:       newTestDeps(numberedEntries(DefaultTopQueriesLimit+1), nil),
			wantStatus: 200,
			wantIDs:    descendingIDs(DefaultTopQueriesLimit+1, DefaultTopQueriesLimit),
		},
		"zero timeout uses the collector timeout": {
			deps: testDeps{
				client: blockingClient{},
			},
			cfg: FunctionsConfig{
				Timeout: confopt.Duration(time.Millisecond),
			},
			wantStatus: 504,
		},
		"slowlog timeout": {
			deps: testDeps{
				client: blockingClient{},
			},
			cfg: FunctionsConfig{
				TopQueries: TopQueriesConfig{
					Timeout: confopt.Duration(time.Millisecond),
				},
			},
			wantStatus: 504,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.cfg.Timeout == 0 {
				test.cfg.Timeout = confopt.Duration(time.Second)
			}
			handler := NewRouter(test.deps, test.cfg)
			params := funcapi.ResolveParams(topQueriesParams(), map[string][]string{"__sort": {test.sort}})

			resp := handler.Handle(context.Background(), topQueriesMethodID, params)

			require.NotNil(t, resp)
			assert.Equal(t, test.wantStatus, resp.Status, resp.Message)
			if test.wantIDs != nil {
				assert.Equal(t, test.wantIDs, rowIDs(t, resp.Data))
			}
		})
	}
}

func TestFuncTopQueries_HandleRow(t *testing.T) {
	entry := redis.SlowLog{
		ID:         7,
		Time:       time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC),
		Duration:   1500 * time.Microsecond,
		Args:       []string{"HSET", "h", "f", "v"},
		ClientAddr: "127.0.0.1:50000",
		ClientName: "app",
	}
	handler := NewRouter(
		newTestDeps([]redis.SlowLog{entry}, nil),
		FunctionsConfig{
			Timeout: confopt.Duration(time.Second),
		},
	)

	resp := handler.Handle(context.Background(), topQueriesMethodID, funcapi.ResolveParams(topQueriesParams(), nil))

	require.Equal(t, 200, resp.Status)
	assert.Equal(t, [][]any{{
		int64(7),
		"2026-10-03T12:00:00Z",
		"HSET h f v",
		"HSET",
		1.5,
		"127.0.0.1:50000",
		"app",
	}}, resp.Data)
	assert.Equal(t, topQueriesDefaultSort, resp.DefaultSortColumn)
}

// numberedEntries returns n entries whose durations grow with their IDs (1..n).
func numberedEntries(n int) []redis.SlowLog {
	entries := make([]redis.SlowLog, 0, n)
	for id := range int64(n) {
		entries = append(entries, redis.SlowLog{
			ID:       id + 1,
			Duration: time.Duration(id+1) * time.Microsecond,
			Args:     []string{"GET", "k"},
		})
	}
	return entries
}

// descendingIDs returns the first n IDs counting down from top.
func descendingIDs(top, n int) []int64 {
	ids := make([]int64, 0, n)
	for id := range n {
		ids = append(ids, int64(top-id))
	}
	return ids
}

func rowIDs(t *testing.T, data any) []int64 {
	t.Helper()
	rows, ok := data.([][]any)
	require.True(t, ok, "unexpected response data type %T", data)
	idCol := slices.IndexFunc(topQueriesColumns, func(c topQueriesColumn) bool { return c.Name == "id" })
	require.NotEqual(t, -1, idCol)
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row[idCol].(int64))
	}
	return ids
}

type testDeps struct {
	client Client
	err    error
}

func newTestDeps(entries []redis.SlowLog, err error) testDeps {
	return testDeps{
		client: slowLogClient{
			entries: entries,
			err:     err,
		},
	}
}

func (d testDeps) Client() (Client, error) {
	return d.client, d.err
}

type slowLogClient struct {
	entries []redis.SlowLog
	err     error
}

func (c slowLogClient) SlowLogGet(ctx context.Context, num int64) *redis.SlowLogCmd {
	cmd := redis.NewSlowLogCmd(ctx, "slowlog", "get", num)
	if c.err != nil {
		cmd.SetErr(c.err)
		return cmd
	}
	cmd.SetVal(slices.Clone(c.entries))
	return cmd
}

// blockingClient answers only when the request context ends, like a server that does not reply in time.
type blockingClient struct{}

func (blockingClient) SlowLogGet(ctx context.Context, num int64) *redis.SlowLogCmd {
	cmd := redis.NewSlowLogCmd(ctx, "slowlog", "get", num)
	<-ctx.Done()
	cmd.SetErr(ctx.Err())
	return cmd
}
