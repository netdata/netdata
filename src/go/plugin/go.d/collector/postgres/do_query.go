// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"context"
	"database/sql"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/sqlquery"
)

func (c *Collector) doQueryRow(query string, v any) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	return c.db.QueryRowContext(ctx, query).Scan(v)
}

func (c *Collector) doDBQueryRow(db *sql.DB, query string, v any) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	return db.QueryRowContext(ctx, query).Scan(v)
}

func (c *Collector) doQuery(query string, assign func(column, value string, rowEnd bool)) error {
	return c.doDBQuery(c.db, query, assign)
}

func (c *Collector) doDBQuery(db *sql.DB, query string, assign func(column, value string, rowEnd bool)) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	_, err := sqlquery.QueryRows(ctx, db, query, assign)
	return err
}
