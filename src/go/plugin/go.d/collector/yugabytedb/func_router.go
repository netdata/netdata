// SPDX-License-Identifier: GPL-3.0-or-later

package yugabytedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var errSQLDSNNotSet = errors.New("SQL DSN is not set")

// funcRouter routes method calls to appropriate function handlers.
// Owns shared SQL connection used by all function handlers.
type funcRouter struct {
	collector *Collector // for config (DSN, SQLTimeout, TopQueriesLimit, logger)

	handlers map[string]funcapi.MethodHandler

	// Shared SQL connection
	db   *sql.DB
	dbMu sync.Mutex

	// Column detection cache for pg_stat_statements
	pgStatStatementsColumns   map[string]bool
	pgStatStatementsColumnsMu sync.RWMutex
}

func newFuncRouter(c *Collector) *funcRouter {
	r := &funcRouter{
		collector: c,
		handlers:  make(map[string]funcapi.MethodHandler),
	}
	r.handlers["top-queries"] = newFuncTopQueries(r)
	r.handlers["running-queries"] = newFuncRunningQueries(r)
	return r
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcRouter)(nil)

func (r *funcRouter) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if h, ok := r.handlers[method]; ok {
		return h.MethodParams(ctx, method)
	}
	return nil, fmt.Errorf("unknown method: %s", method)
}

func (r *funcRouter) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if h, ok := r.handlers[method]; ok {
		return h.Handle(ctx, method, params)
	}
	return funcapi.NotFoundResponse(method)
}

func (r *funcRouter) Cleanup(ctx context.Context) {
	for _, h := range r.handlers {
		h.Cleanup(ctx)
	}
	r.dbMu.Lock()
	defer r.dbMu.Unlock()
	if r.db != nil {
		_ = r.db.Close()
		r.db = nil
	}
}

// ensureDB lazily initializes the SQL connection.
func (r *funcRouter) ensureDB(ctx context.Context) error {
	r.dbMu.Lock()
	defer r.dbMu.Unlock()

	if r.db != nil {
		return nil
	}
	if r.collector.DSN == "" {
		return errSQLDSNNotSet
	}

	db, err := sql.Open("pgx", r.collector.DSN)
	if err != nil {
		return fmt.Errorf("error opening SQL connection: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(10 * time.Minute)

	timeout := r.sqlTimeout()
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return fmt.Errorf("error pinging SQL connection: %w", err)
	}

	r.db = db
	return nil
}

func (r *funcRouter) sqlTimeout() time.Duration {
	if r.collector.SQLTimeout.Duration() > 0 {
		return r.collector.SQLTimeout.Duration()
	}
	return time.Second
}

func (r *funcRouter) topQueriesLimit() int {
	if r.collector.TopQueriesLimit > 0 {
		return r.collector.TopQueriesLimit
	}
	return 500
}

func yugabyteMethods() []module.MethodConfig {
	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "top-queries",
			Name:         "Top Queries",
			Help:         "Top SQL queries from pg_stat_statements. WARNING: Query text may contain unmasked literals (potential PII).",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				buildSortParam(topQueriesColumns),
			},
		},
		{
			UpdateEvery:  10,
			ID:           "running-queries",
			Name:         "Running Queries",
			Help:         "Currently running SQL statements from pg_stat_activity. WARNING: Query text may contain unmasked literals (potential PII).",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				buildSortParam(runningQueriesColumns),
			},
		},
	}
}

func yugabyteFunctionHandler(job *module.Job) funcapi.MethodHandler {
	c, ok := job.Module().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}

// sortableColumn is an interface for columns that can be used in sort options.
type sortableColumn interface {
	isSortOption() bool
	sortLabel() string
	isDefaultSort() bool
	name() string
}

func (c topQueriesColumn) isSortOption() bool  { return c.IsSortOption }
func (c topQueriesColumn) sortLabel() string   { return c.SortLabel }
func (c topQueriesColumn) isDefaultSort() bool { return c.IsDefaultSort }
func (c topQueriesColumn) name() string        { return c.Name }

func (c runningQueriesColumn) isSortOption() bool  { return c.IsSortOption }
func (c runningQueriesColumn) sortLabel() string   { return c.SortLabel }
func (c runningQueriesColumn) isDefaultSort() bool { return c.IsDefaultSort }
func (c runningQueriesColumn) name() string        { return c.Name }

func buildSortParam[T sortableColumn](cols []T) funcapi.ParamConfig {
	var options []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range cols {
		if !col.isSortOption() {
			continue
		}
		opt := funcapi.ParamOption{
			ID:     col.name(),
			Column: col.name(),
			Name:   col.sortLabel(),
			Sort:   &sortDir,
		}
		if col.isDefaultSort() {
			opt.Default = true
		}
		options = append(options, opt)
	}
	return funcapi.ParamConfig{
		ID:         "__sort",
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    options,
		UniqueView: true,
	}
}
