// SPDX-License-Identifier: GPL-3.0-or-later

package mysqlfunc

import (
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

type testDeps struct {
	mu sync.RWMutex
	db *sql.DB

	cfg FunctionsConfig
}

func newTestDeps() *testDeps {
	return &testDeps{
		cfg: FunctionsConfig{
			Timeout: confopt.Duration(time.Second),
			TopQueries: TopQueriesConfig{
				Timeout: confopt.Duration(time.Second),
				Limit:   500,
			},
			DeadlockInfo: DeadlockInfoConfig{
				Timeout: confopt.Duration(time.Second),
			},
			ErrorInfo: ErrorInfoConfig{
				Timeout: confopt.Duration(time.Second),
			},
		},
	}
}

func (d *testDeps) setDB(db *sql.DB) {
	d.mu.Lock()
	d.db = db
	d.mu.Unlock()
}

func (d *testDeps) cleanup() {
	d.mu.Lock()
	d.db = nil
	d.mu.Unlock()
}

func (d *testDeps) DB() (Queryer, error) {
	d.mu.RLock()
	db := d.db
	d.mu.RUnlock()
	if db == nil {
		return nil, errors.New("db unavailable")
	}
	return db, nil
}
