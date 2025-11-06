// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	if c.db == nil {
		if err := c.openConnection(); err != nil {
			return nil, err
		}
	}

	mx := make(map[string]int64)

	for _, q := range c.Queries {
		if _, err := c.collectQuery(ctx, q, mx); err != nil {
			return nil, fmt.Errorf("query %q failed: %w", q.Name, err)
		}
	}

	return mx, nil
}

func (c *Collector) collectQuery(ctx context.Context, queryCfg QueryConfig, mx map[string]int64) (duration int64, err error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	start := time.Now()

	rows, err := c.db.QueryContext(ctx, queryCfg.Query)
	if err != nil {
		return 0, err
	}

	duration = time.Since(start).Milliseconds()
	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		return duration, err
	}

	values := makeRawBytesSlice(len(columns))
	row := make(map[string]string, len(columns))

	for rows.Next() {
		if err := rows.Scan(values...); err != nil {
			return duration, err
		}
		clear(row)
		for i, l := 0, len(values); i < l; i++ {
			row[columns[i]] = rawBytesToString(values[i])
		}
		c.collectRow(row, queryCfg, mx)
	}

	return duration, rows.Err()
}

func (c *Collector) collectRow(row map[string]string, queryCfg QueryConfig, mx map[string]int64) {
	var baseID strings.Builder
	baseID.Grow(128)

	baseID.WriteString(queryCfg.Name)
	for _, lbl := range queryCfg.Labels {
		if v, ok := row[lbl]; ok {
			baseID.WriteString("_" + v)
		}
	}

	base := strings.ToLower(baseID.String())

	for _, queryValue := range queryCfg.Values {
		valID := queryCfg.Name + "_" + queryValue
		if c.skipValues[valID] {
			continue
		}

		strVal, ok := row[queryValue]
		if !ok {
			continue
		}

		v, err := strconv.ParseFloat(strVal, 64)
		if err != nil {
			c.skipValues[valID] = true
			c.Warningf("invalid value %q for %s.%s: %v", strVal, queryCfg.Name, queryValue, err)
			continue
		}

		id := strings.ToLower(base + "_" + queryValue)

		if !c.seenCharts[id] {
			c.seenCharts[id] = true
			c.addQueryChart(row, queryCfg, id, queryValue)
		}

		mx[id] = int64(v)
	}
}

func (c *Collector) openConnection() error {
	db, err := sql.Open(c.Driver, c.DSN)
	if err != nil {
		return fmt.Errorf("open %s: %w (dsn=%s)", c.Driver, err, redactDSN(c.DSN))
	}

	if c.ConnMaxLifetime.Duration() > 0 {
		db.SetConnMaxLifetime(c.ConnMaxLifetime.Duration())
	}
	if c.MaxOpenConns > 0 {
		db.SetMaxOpenConns(c.MaxOpenConns)
	}
	if c.MaxIdleConns > 0 {
		db.SetMaxIdleConns(c.MaxIdleConns)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("ping %s: %w (dsn=%s)", c.Driver, err, redactDSN(c.DSN))
	}

	c.db = db

	return nil
}

func redactDSN(dsn string) string {
	if dsn == "" {
		return dsn
	}

	// Find where authority starts (right after "://", if present)
	authStart := 0
	if i := strings.Index(dsn, "://"); i != -1 {
		authStart = i + 3
	}

	// Find the *last* '@' after authority start
	rel := strings.LastIndex(dsn[authStart:], "@")
	if rel == -1 {
		// no userinfo
		return dsn
	}
	at := authStart + rel

	// userinfo is between authority start and '@'
	userinfo := dsn[authStart:at]
	if userinfo == "" {
		// malformed/empty userinfo; leave unchanged
		return dsn
	}

	// If there's a colon, treat text before first ':' as user and the rest as password.
	if colon := strings.IndexByte(userinfo, ':'); colon >= 0 {
		user := userinfo[:colon]
		// Keep user, redact password
		redacted := user + ":****"
		return dsn[:authStart] + redacted + dsn[at:]
	}

	// No password present -> redact entire username
	return dsn[:authStart] + "****" + dsn[at:]
}

func makeRawBytesSlice(size int) []any {
	values := make([]any, size)
	for i := range values {
		var b sql.RawBytes
		values[i] = &b
	}
	return values
}

func rawBytesToString(value any) string {
	if rb, ok := value.(*sql.RawBytes); ok && rb != nil {
		return string(*rb)
	}
	return ""
}
