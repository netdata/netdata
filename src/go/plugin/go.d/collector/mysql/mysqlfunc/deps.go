// SPDX-License-Identifier: GPL-3.0-or-later

package mysqlfunc

import (
	"context"
	"database/sql"
)

// Queryer is the minimal query-only surface used by function handlers.
// Collector retains ownership of connection lifecycle.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Deps defines the dependency surface required by mysql function handlers.
type Deps interface {
	DB() (Queryer, error)
}
