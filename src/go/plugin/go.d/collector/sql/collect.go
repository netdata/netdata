// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	if c.db == nil {
		if err := c.openConnection(); err != nil {
			return nil, err
		}
	}

	qcache, _, err := c.execReusableQueries(ctx)
	if err != nil {
		return nil, err
	}

	mcache, _, err := c.execMetricQueries(ctx, qcache)
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)
	if err := c.collectMetrics(mx, mcache); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectMetrics(mx map[string]int64, mcache QueryRowsCache) error {
	for i, m := range c.Metrics {
		rows, ok := mcache[m.ID]
		if !ok {
			continue
		}

		switch strings.ToLower(m.Mode) {
		case "columns", "":
			if err := c.collectMetricsModeColumns(mx, m, rows); err != nil {
				return fmt.Errorf("metric %q (index %d) columns: %w", m.ID, i, err)
			}
		case "kv":
			if err := c.collectMetricsModeKV(mx, m, rows); err != nil {
				return fmt.Errorf("metric %q (index %d) kv: %w", m.ID, i, err)
			}
		default:
			return fmt.Errorf("metric %q (index %d) unknown mode %q", m.ID, i, m.Mode)
		}
	}
	return nil
}

func (c *Collector) collectMetricsModeColumns(mx map[string]int64, m ConfigMetricBlock, rows []map[string]string) error {
	for _, ch := range m.Charts {
		for _, row := range rows {
			chartID := buildChartID(m, ch, row)
			if chartID == "" {
				continue
			}
			c.createChart(chartID, m, ch, row)

			for _, d := range ch.Dims {
				raw, ok := row[d.Source]
				if !ok {
					continue
				}
				id := buildDimID(chartID, d.Name)

				if d.StatusWhen == nil {
					mx[id] += toInt64(raw)
				} else if v, ok := mx[id]; !ok || v == 0 {
					mx[id] = metrix.Bool(c.evalStatusWhen(d.StatusWhen, raw))
				}
			}
		}
	}

	return nil
}

func (c *Collector) collectMetricsModeKV(mx map[string]int64, m ConfigMetricBlock, rows []map[string]string) error {
	if m.KVMode == nil {
		return nil
	}

	nameCol := m.KVMode.NameCol
	valCol := m.KVMode.ValueCol

	for _, ch := range m.Charts {
		for _, row := range rows {
			chartID := buildChartID(m, ch, row)
			if chartID == "" {
				continue
			}
			c.createChart(chartID, m, ch, row)

			k, ok1 := row[nameCol]
			vraw, ok2 := row[valCol]
			if !ok1 || !ok2 {
				continue
			}

			for _, d := range ch.Dims {
				if d.Source != k {
					continue
				}
				id := buildDimID(chartID, d.Name)

				if d.StatusWhen == nil {
					mx[id] += toInt64(vraw)
				} else if v, ok := mx[id]; !ok || v == 0 {
					mx[id] = metrix.Bool(c.evalStatusWhen(d.StatusWhen, vraw))
				}
			}
		}
	}

	return nil
}

func (c *Collector) evalStatusWhen(sw *ConfigStatusWhen, value string) bool {
	switch {
	case sw.Equals != "":
		return value == sw.Equals
	case len(sw.In) > 0:
		return slices.Contains(sw.In, value)
	case sw.re != nil:
		return sw.re.MatchString(value)
	default:
		return false
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

func buildChartID(m ConfigMetricBlock, ch ConfigChartConfig, row map[string]string) string {
	var b strings.Builder
	b.Grow(128)

	b.WriteString(m.ID)
	b.WriteString("_")
	b.WriteString(ch.Context)

	for _, lf := range m.LabelsFromRow {
		v, ok := row[lf.Source]
		if !ok {
			return ""
		}
		b.WriteString("_" + v)
	}

	return normalizeID(b.String())
}

func buildDimID(chartID, dimName string) string {
	return normalizeID(chartID + "." + dimName)
}

func normalizeID(id string) string {
	r := strings.NewReplacer(" ", "_", ".", "_")
	return strings.ToLower(r.Replace(id))
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

// local parser: int -> float -> bool -> 0
func toInt64(s string) int64 {
	if s == "" {
		return 0
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f)
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "t", "yes", "y", "on", "up":
		return 1
	case "false", "f", "no", "n", "off", "down":
		return 0
	}
	return 0
}
