// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"context"
	"fmt"
	"maps"
	"time"
)

// QueryRowsCache maps query_id -> slice of row maps (col -> string value)
type QueryRowsCache map[string][]map[string]string

// execReusableQueries runs all queries declared in Config.Queries (new schema)
// and returns their full rowsets in memory. It also returns per-query durations (ms).
func (c *Collector) execReusableQueries(ctx context.Context) (QueryRowsCache, map[string]int64, error) {
	cache := make(QueryRowsCache, len(c.Queries))
	durations := make(map[string]int64, len(c.Queries))

	for i, q := range c.Queries {
		if q.ID == "" {
			return nil, nil, fmt.Errorf("queries[%d] missing id", i+1)
		}
		if q.Query == "" {
			return nil, nil, fmt.Errorf("queries[%d] missing query", i+1)
		}

		rows, dur, err := c.runSQL(ctx, q.Query)
		if err != nil {
			return nil, nil, fmt.Errorf("query %q failed: %w", q.ID, err)
		}
		cache[q.ID] = rows
		durations[q.ID] = dur
	}

	return cache, durations, nil
}

// execMetricQueries resolves and executes the query for each metric block.
// If a metric uses query_ref, it reuses rows from qcache (no re-query).
// If a metric has inline query, it executes it and stores the rows.
func (c *Collector) execMetricQueries(ctx context.Context, qcache QueryRowsCache) (QueryRowsCache, map[string]int64, error) {
	cache := make(QueryRowsCache, len(c.Metrics))
	durations := make(map[string]int64, len(c.Metrics))

	for i, m := range c.Metrics {
		if m.ID == "" {
			return nil, nil, fmt.Errorf("metrics[%d] missing id", i+1)
		}

		switch {
		case m.QueryRef != "":
			// reuse pre-fetched rows; duration is 0 because we didn't re-run
			rows, ok := qcache[m.QueryRef]
			if !ok {
				return nil, nil, fmt.Errorf("metrics[%d] query_ref %q not found in queries cache", i+1, m.QueryRef)
			}
			cache[m.ID] = rows
			durations[m.ID] = 0
		case m.Query != "":
			rows, dur, err := c.runSQL(ctx, m.Query)
			if err != nil {
				return nil, nil, fmt.Errorf("metrics[%d] (%q) query failed: %w", i+1, m.ID, err)
			}
			cache[m.ID] = rows
			durations[m.ID] = dur
		default:
			return nil, nil, fmt.Errorf("metrics[%d] must set one of query_ref or query", i+1)
		}
	}

	return cache, durations, nil
}

// runSQL executes a SQL statement with c.Timeout and returns the rowset as []map[col]value.
// Duration is milliseconds from QueryContext() start to first successful return.
func (c *Collector) runSQL(ctx context.Context, query string) ([]map[string]string, int64, error) {
	qctx := ctx
	cancel := func() {}
	if d := c.Timeout.Duration(); d > 0 {
		qctx, cancel = context.WithTimeout(ctx, d)
	}
	defer cancel()

	start := time.Now()
	rows, err := c.db.QueryContext(qctx, query)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	duration := time.Since(start).Milliseconds()

	columns, err := rows.Columns()
	if err != nil {
		return nil, duration, err
	}

	scan := makeRawBytesSlice(len(columns))
	out := make([]map[string]string, 0, 64)
	row := make(map[string]string, len(columns))

	for rows.Next() {
		if err := rows.Scan(scan...); err != nil {
			return nil, duration, err
		}
		clear(row)
		for i := range columns {
			row[columns[i]] = rawBytesToString(scan[i])
		}

		out = append(out, maps.Clone(row))
	}

	return out, duration, rows.Err()
}
