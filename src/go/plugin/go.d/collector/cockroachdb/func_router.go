// SPDX-License-Identifier: GPL-3.0-or-later

package cockroachdb

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
	collector *Collector // for config (Functions.DSN, logger)

	// Shared SQL connection
	db   *sql.DB
	dbMu sync.Mutex

	handlers map[string]funcapi.MethodHandler
}

func newFuncRouter(c *Collector) *funcRouter {
	r := &funcRouter{
		collector: c,
		handlers:  make(map[string]funcapi.MethodHandler),
	}
	r.handlers[topQueriesMethodID] = newFuncTopQueries(r)
	r.handlers[runningQueriesMethodID] = newFuncRunningQueries(r)
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
	if r.collector.Functions.DSN == "" {
		return errSQLDSNNotSet
	}

	db, err := sql.Open("pgx", r.collector.Functions.DSN)
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

	setCtx, cancel := context.WithTimeout(ctx, timeout)
	if _, err := db.ExecContext(setCtx, "SET allow_unsafe_internals = on"); err != nil {
		r.collector.Debugf("unable to set allow_unsafe_internals: %v", err)
	}
	cancel()

	r.db = db
	return nil
}

func (r *funcRouter) sqlTimeout() time.Duration {
	if r.collector.Timeout.Duration() > 0 {
		return r.collector.Timeout.Duration()
	}
	return time.Second
}

func (r *funcRouter) topQueriesLimit() int {
	return r.collector.topQueriesLimit()
}

func cockroachMethods() []funcapi.MethodConfig {
	return []funcapi.MethodConfig{
		topQueriesMethodConfig(),
		runningQueriesMethodConfig(),
	}
}

func cockroachFunctionHandler(job module.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
