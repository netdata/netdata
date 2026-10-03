// SPDX-License-Identifier: GPL-3.0-or-later

// rum_history.go persists sessions, session events and error occurrences
// for rum-sessions, rum-errors and rum-session-events queries beyond the
// live aggregation window. Per-site writers in plugin/dem/rum/history
// submit batches; this file owns the schema, SQL and retention operations.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// sessionEventCap is the per-session event cap: later
// events are dropped and counted once a session already has this many
// stored rows.
const sessionEventCap = 200

// migrateRumHistory idempotently creates tables during store open. Indexes
// support time-range scans, per-session event lookup and retention.
func (s *Store) migrateRumHistory(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS rum_sessions(
  site        TEXT NOT NULL,
  session_id  TEXT NOT NULL,
  started_at  INTEGER NOT NULL,
  last_at     INTEGER NOT NULL,
  pageviews   INTEGER NOT NULL DEFAULT 0,
  errors      INTEGER NOT NULL DEFAULT 0,
  browser     TEXT NOT NULL DEFAULT '',
  device      TEXT NOT NULL DEFAULT '',
  country     TEXT NOT NULL DEFAULT '',
  city        TEXT NOT NULL DEFAULT '',
  version     TEXT NOT NULL DEFAULT '',
  entry_page  TEXT NOT NULL DEFAULT '',
  last_page   TEXT NOT NULL DEFAULT '',
  user_id     TEXT NOT NULL DEFAULT '',
  frustrations INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (site, session_id)
);
CREATE INDEX IF NOT EXISTS idx_rum_sessions_last_at ON rum_sessions(last_at);
CREATE INDEX IF NOT EXISTS idx_rum_sessions_site_last_at ON rum_sessions(site, last_at);

CREATE TABLE IF NOT EXISTS rum_session_events(
  site       TEXT NOT NULL,
  session_id TEXT NOT NULL,
  ts_us      INTEGER NOT NULL,
  type       TEXT NOT NULL,
  page       TEXT NOT NULL DEFAULT '',
  text       TEXT NOT NULL DEFAULT '',
  trace_id   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_rum_session_events_session ON rum_session_events(site, session_id, ts_us);
CREATE INDEX IF NOT EXISTS idx_rum_session_events_ts ON rum_session_events(ts_us);

CREATE TABLE IF NOT EXISTS rum_error_groups(
  site         TEXT NOT NULL,
  fingerprint  TEXT NOT NULL,
  type         TEXT NOT NULL DEFAULT '',
  message      TEXT NOT NULL DEFAULT '',
  sample_stack TEXT NOT NULL DEFAULT '',
  first_seen   INTEGER NOT NULL,
  last_seen    INTEGER NOT NULL,
  PRIMARY KEY (site, fingerprint)
);
CREATE INDEX IF NOT EXISTS idx_rum_error_groups_last_seen ON rum_error_groups(last_seen);

CREATE TABLE IF NOT EXISTS rum_error_occurrences(
  site        TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  ts          INTEGER NOT NULL,
  session_id  TEXT NOT NULL DEFAULT '',
  page        TEXT NOT NULL DEFAULT '',
  browser     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_rum_error_occ_fp ON rum_error_occurrences(site, fingerprint, ts);
CREATE INDEX IF NOT EXISTS idx_rum_error_occ_ts ON rum_error_occurrences(ts);
CREATE INDEX IF NOT EXISTS idx_rum_error_occ_site_ts ON rum_error_occurrences(site, ts);
`)
	return err
}

// ---- record types (writer input / query output) ----

// RumSessionRecord is one rum_sessions row. Timestamps are
// unix seconds.
type RumSessionRecord struct {
	Site, SessionID                         string
	StartedAt, LastAt                       int64
	Pageviews, Errors                       int64
	Browser, Device, Country, City, Version string
	EntryPage, LastPage                     string
	UserID                                  string
	Frustrations                            int64
}

// RumSessionEventRecord is one rum_session_events row. TSUnixUS is
// microseconds (matching the in-memory ring's resolution and the
// rum-session-events wire column).
type RumSessionEventRecord struct {
	Site, SessionID  string
	TSUnixUS         int64
	Type, Page, Text string
	TraceID          string // hex, on traced requests
}

// RumErrorGroupRecord is one rum_error_groups row. Timestamps are unix
// seconds; FirstSeen is only honored on first insert (ON CONFLICT keeps
// the original).
type RumErrorGroupRecord struct {
	Site, Fingerprint, Type, Message, SampleStack string
	FirstSeen, LastSeen                           int64
}

// RumErrorOccurrenceRecord is one rum_error_occurrences row. TS is unix
// seconds.
type RumErrorOccurrenceRecord struct {
	Site, Fingerprint        string
	TS                       int64
	SessionID, Page, Browser string
}

// RumHistoryBatch bundles one flush of records for WriteRumHistoryBatch
// to insert in a transaction.
type RumHistoryBatch struct {
	Sessions  []RumSessionRecord
	Events    []RumSessionEventRecord
	ErrGroups []RumErrorGroupRecord
	ErrOccs   []RumErrorOccurrenceRecord
}

// WriteRumHistoryBatch upserts/inserts b in one transaction, returning
// per-site written/dropped counts. Drops here are events beyond the
// 200-event session cap; the caller counts channel-full drops before
// records reach this method.
func (s *Store) WriteRumHistoryBatch(
	ctx context.Context,
	b RumHistoryBatch,
) (written, dropped map[string]int, err error) {
	written, dropped = map[string]int{}, map[string]int{}
	if len(b.Sessions) == 0 && len(b.Events) == 0 && len(b.ErrGroups) == 0 && len(b.ErrOccs) == 0 {
		return written, dropped, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		// SQLite may report SQLITE_INTERRUPT for a cancelled BEGIN. No write
		// has started; expose caller cancellation consistently to the writer.
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return nil, nil, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	sessStmt, err := tx.PrepareContext(ctx, `
INSERT INTO rum_sessions(site, session_id, started_at, last_at, pageviews, errors, browser, device, country, city, version, entry_page, last_page, user_id, frustrations)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(site, session_id) DO UPDATE SET
  last_at=MAX(rum_sessions.last_at,excluded.last_at), pageviews=rum_sessions.pageviews+excluded.pageviews, errors=rum_sessions.errors+excluded.errors,
  browser=excluded.browser, device=excluded.device, country=excluded.country,
  city=excluded.city, version=excluded.version, last_page=excluded.last_page,
  user_id=excluded.user_id, frustrations=rum_sessions.frustrations+excluded.frustrations`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = sessStmt.Close() }()
	for _, r := range b.Sessions {
		if _, err := sessStmt.ExecContext(ctx, r.Site, r.SessionID, r.StartedAt, r.LastAt, r.Pageviews, r.Errors,
			r.Browser, r.Device, r.Country, r.City, r.Version, r.EntryPage, r.LastPage, r.UserID, r.Frustrations); err != nil {
			return nil, nil, err
		}
		written[r.Site]++
	}

	// Enforce the per-session cap in the same transaction as the inserts.
	evStmt, err := tx.PrepareContext(ctx, `
INSERT INTO rum_session_events(site, session_id, ts_us, type, page, text, trace_id)
SELECT ?,?,?,?,?,?,? WHERE (SELECT COUNT(*) FROM rum_session_events WHERE site=? AND session_id=?) < ?`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = evStmt.Close() }()
	for _, r := range b.Events {
		res, err := evStmt.ExecContext(
			ctx,
			r.Site,
			r.SessionID,
			r.TSUnixUS,
			r.Type,
			r.Page,
			r.Text,
			r.TraceID,
			r.Site,
			r.SessionID,
			sessionEventCap,
		)
		if err != nil {
			return nil, nil, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			written[r.Site]++
		} else {
			dropped[r.Site]++
		}
	}

	grpStmt, err := tx.PrepareContext(ctx, `
INSERT INTO rum_error_groups(site, fingerprint, type, message, sample_stack, first_seen, last_seen)
VALUES(?,?,?,?,?,?,?)
ON CONFLICT(site, fingerprint) DO UPDATE SET last_seen=excluded.last_seen`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = grpStmt.Close() }()
	for _, r := range b.ErrGroups {
		if _, err := grpStmt.ExecContext(ctx, r.Site, r.Fingerprint, r.Type, r.Message, r.SampleStack, r.FirstSeen, r.LastSeen); err != nil {
			return nil, nil, err
		}
		written[r.Site]++
	}

	occStmt, err := tx.PrepareContext(
		ctx,
		`INSERT INTO rum_error_occurrences(site, fingerprint, ts, session_id, page, browser) VALUES(?,?,?,?,?,?)`,
	)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = occStmt.Close() }()
	for _, r := range b.ErrOccs {
		if _, err := occStmt.ExecContext(ctx, r.Site, r.Fingerprint, r.TS, r.SessionID, r.Page, r.Browser); err != nil {
			return nil, nil, err
		}
		written[r.Site]++
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return written, dropped, nil
}

// QueryRumSessions returns sessions overlapping [after, before], ordered
// newest-last-seen first and capped at limit. site == "" matches every site.
func (s *Store) QueryRumSessions(
	ctx context.Context,
	site string,
	after, before int64,
	limit int,
) ([]RumSessionRecord, error) {
	if limit <= 0 || limit > 2000 {
		limit = 2000
	}
	q := `SELECT site, session_id, started_at, last_at, pageviews, errors, browser, device, country, city, version, entry_page, last_page, user_id, frustrations
	      FROM rum_sessions WHERE last_at >= ? AND started_at <= ?`
	args := []any{after, before}
	if site != "" {
		q += ` AND site = ?`
		args = append(args, site)
	}
	q += ` ORDER BY last_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []RumSessionRecord
	for rows.Next() {
		var r RumSessionRecord
		if err := rows.Scan(&r.Site, &r.SessionID, &r.StartedAt, &r.LastAt, &r.Pageviews, &r.Errors,
			&r.Browser, &r.Device, &r.Country, &r.City, &r.Version, &r.EntryPage, &r.LastPage, &r.UserID, &r.Frustrations); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// QueryRumSessionEvents returns the full stored session timeline, oldest
// first. site == "" matches the session id on any site, as in the live
// Aggregator.SessionEvents lookup.
func (s *Store) QueryRumSessionEvents(ctx context.Context, site, sessionID string) ([]RumSessionEventRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT site, session_id, ts_us, type, page, text, trace_id FROM rum_session_events WHERE session_id = ? AND (? = '' OR site = ?) ORDER BY ts_us ASC`,
		sessionID,
		site,
		site,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []RumSessionEventRecord
	for rows.Next() {
		var r RumSessionEventRecord
		if err := rows.Scan(&r.Site, &r.SessionID, &r.TSUnixUS, &r.Type, &r.Page, &r.Text, &r.TraceID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RumErrorAgg is one rum-errors row. Counts, affected sessions, first/last
// seen, top page and browsers are computed over the requested time range.
type RumErrorAgg struct {
	Site, Fingerprint, Type, Message, SampleStack string
	CountWindow, SessionsAffected                 int
	FirstSeen, LastSeen                           int64 // unix seconds, within the range
	TopPage                                       string
	Browsers                                      []string
}

// QueryRumErrors aggregates rum_error_occurrences in [after, before],
// joined with rum_error_groups for the fixed type/message/sample_stack.
// site == "" matches every site. Three queries in total, however many
// groups are in range.
func (s *Store) QueryRumErrors(ctx context.Context, site string, after, before int64) ([]RumErrorAgg, error) {
	where := `o.ts >= ? AND o.ts <= ?`
	args := []any{after, before}
	if site != "" {
		where += ` AND o.site = ?`
		args = append(args, site)
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT o.site, o.fingerprint, COUNT(*), COUNT(DISTINCT NULLIF(o.session_id, '')), MIN(o.ts), MAX(o.ts),
	      COALESCE(g.type, ''), COALESCE(g.message, ''), COALESCE(g.sample_stack, '')
	      FROM rum_error_occurrences o
	      LEFT JOIN rum_error_groups g ON g.site = o.site AND g.fingerprint = o.fingerprint
	      WHERE `+where+`
	      GROUP BY o.site, o.fingerprint ORDER BY o.site, o.fingerprint`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RumErrorAgg
	index := map[[2]string]int{}
	for rows.Next() {
		var r RumErrorAgg
		if err := rows.Scan(&r.Site, &r.Fingerprint, &r.CountWindow, &r.SessionsAffected, &r.FirstSeen, &r.LastSeen,
			&r.Type, &r.Message, &r.SampleStack); err != nil {
			_ = rows.Close()
			return nil, err
		}
		index[[2]string{r.Site, r.Fingerprint}] = len(out)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = rows.Close()
	if len(out) == 0 {
		return out, nil
	}

	pages, err := s.topRumErrorValues(ctx, "page", where, args, 1)
	if err != nil {
		return nil, err
	}
	browsers, err := s.topRumErrorValues(ctx, "browser", where, args, 3)
	if err != nil {
		return nil, err
	}
	for key, i := range index {
		if p := pages[key]; len(p) > 0 {
			out[i].TopPage = p[0]
		}
		out[i].Browsers = browsers[key]
	}
	return out, nil
}

// topRumErrorValues ranks non-empty values of col (page|browser) per error
// group over the same filter as QueryRumErrors, most frequent first
// (ties by value), keeping the top n per group.
func (s *Store) topRumErrorValues(
	ctx context.Context,
	col, where string,
	args []any,
	n int,
) (map[[2]string][]string, error) {
	rows, err := s.db.QueryContext(
		ctx,
		fmt.Sprintf(`SELECT o.site, o.fingerprint, o.%s, COUNT(*) c FROM rum_error_occurrences o
	      WHERE %s AND o.%s != ''
	      GROUP BY o.site, o.fingerprint, o.%s
	      ORDER BY o.site, o.fingerprint, c DESC, o.%s ASC`, col, where, col, col, col),
		args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[[2]string][]string{}
	for rows.Next() {
		var site, fp, v string
		var c int
		if err := rows.Scan(&site, &fp, &v, &c); err != nil {
			return nil, err
		}
		key := [2]string{site, fp}
		if len(out[key]) < n {
			out[key] = append(out[key], v)
		}
	}
	return out, rows.Err()
}

// ---- retention ----

// EnforceRumHistoryRetention deletes rows older than days (<=0 disables
// the age cutoff), then — while the RUM tables' estimated byte share
// exceeds maxBytes (<=0 disables the cap) — deletes the oldest day's
// worth of detail rows (occurrences/events). Either pass can leave a
// session/group with no detail rows left (e.g. a session's last_at kept
// fresh by beacons that updated no event, while its actual events all
// aged out): pruneOrphanRumParents runs unconditionally afterward so that
// case is cleaned up too, not just the one the size-cap loop causes on
// its own.
func (s *Store) EnforceRumHistoryRetention(ctx context.Context, days int, maxBytes int64) error {
	if days > 0 {
		if err := s.enforceRumHistoryAge(ctx, days); err != nil {
			return err
		}
	}
	if maxBytes > 0 {
		if err := s.enforceRumHistorySize(ctx, maxBytes); err != nil {
			return err
		}
	}
	return s.pruneOrphanRumParents(ctx)
}

// enforceRumHistoryAge deletes rows older than days from every RUM table,
// each keyed by its own timestamp column. This alone can still leave an
// orphaned session/group row (see EnforceRumHistoryRetention's comment);
// the caller prunes those afterward.
func (s *Store) enforceRumHistoryAge(ctx context.Context, days int) error {
	cutoffAt := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	stmts := []struct {
		sql string
		arg int64
	}{
		{`DELETE FROM rum_error_occurrences WHERE ts < ?`, cutoffAt},
		{`DELETE FROM rum_session_events WHERE ts_us < ?`, cutoffAt * 1_000_000},
		{`DELETE FROM rum_sessions WHERE last_at < ?`, cutoffAt},
		{`DELETE FROM rum_error_groups WHERE last_seen < ?`, cutoffAt},
	}
	for _, st := range stmts {
		if _, err := s.db.ExecContext(ctx, st.sql, st.arg); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) enforceRumHistorySize(ctx context.Context, maxBytes int64) error {
	// Measure once, then subtract what each evicted day held (indexed range
	// scans). Orphaned parent rows are not subtracted, so the estimate only
	// errs towards evicting a little more, never towards exceeding the cap.
	size, err := s.rumHistoryBytes(ctx)
	if err != nil || size <= maxBytes {
		return err
	}
	evicted := false
	for size > maxBytes {
		day, ok, err := s.oldestRumDay(ctx)
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		from, to := day*86400, (day+1)*86400
		freed, err := s.rumDayBytes(ctx, from, to)
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM rum_error_occurrences WHERE ts >= ? AND ts < ?`, from, to); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM rum_session_events WHERE ts_us >= ? AND ts_us < ?`, from*1e6, to*1e6); err != nil {
			return err
		}
		size -= freed
		evicted = true
	}
	if evicted {
		return s.pruneOrphanRumParents(ctx)
	}
	return nil
}

// rumDayBytes approximates the detail rows in [from, to) (unix seconds),
// with the same accounting as rumHistoryBytes.
func (s *Store) rumDayBytes(ctx context.Context, from, to int64) (int64, error) {
	var total int64
	err := s.db.QueryRowContext(ctx, `
SELECT
  COALESCE((SELECT SUM(LENGTH(site)+LENGTH(session_id)+LENGTH(type)+LENGTH(page)+LENGTH(text)+?) FROM rum_session_events WHERE ts_us >= ? AND ts_us < ?), 0) +
  COALESCE((SELECT SUM(LENGTH(site)+LENGTH(fingerprint)+LENGTH(session_id)+LENGTH(page)+LENGTH(browser)+?) FROM rum_error_occurrences WHERE ts >= ? AND ts < ?), 0)
`, rumRowOverheadBytes, from*1e6, to*1e6, rumRowOverheadBytes, from, to).Scan(&total)
	return total, err
}

// pruneOrphanRumParents deletes sessions/groups left with no detail rows
// after size-cap eviction.
func (s *Store) pruneOrphanRumParents(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
DELETE FROM rum_sessions
WHERE NOT EXISTS (SELECT 1 FROM rum_session_events e WHERE e.site = rum_sessions.site AND e.session_id = rum_sessions.session_id)`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
DELETE FROM rum_error_groups
WHERE NOT EXISTS (SELECT 1 FROM rum_error_occurrences o WHERE o.site = rum_error_groups.site AND o.fingerprint = rum_error_groups.fingerprint)`)
	return err
}

// oldestRumDay returns the earliest day bucket (unix seconds / 86400)
// present in either detail table, ok=false when both are empty.
func (s *Store) oldestRumDay(ctx context.Context) (int64, bool, error) {
	// MIN over each ts index, not a per-row expression: stays cheap at scale.
	var occ, ev sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MIN(ts) FROM rum_error_occurrences`).Scan(&occ); err != nil {
		return 0, false, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT MIN(ts_us) FROM rum_session_events`).Scan(&ev); err != nil {
		return 0, false, err
	}
	switch {
	case occ.Valid && ev.Valid:
		return min(occ.Int64, ev.Int64/1e6) / 86400, true, nil
	case occ.Valid:
		return occ.Int64 / 86400, true, nil
	case ev.Valid:
		return ev.Int64 / 1e6 / 86400, true, nil
	}
	return 0, false, nil
}

// rumHistoryBytes approximates the four RUM tables' on-disk share: the
// sum of their text column lengths plus a fixed per-row overhead for the
// integer columns and SQLite's own row/index bookkeeping. It is a budget
// signal for the size cap, not an exact byte count (an exact count would
// need PRAGMA dbstat, not available in every SQLite build).
const rumRowOverheadBytes = 48

func (s *Store) rumHistoryBytes(ctx context.Context) (int64, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT
  COALESCE((SELECT SUM(LENGTH(site)+LENGTH(session_id)+LENGTH(browser)+LENGTH(device)+LENGTH(country)+LENGTH(city)+LENGTH(version)+LENGTH(entry_page)+LENGTH(last_page)+?) FROM rum_sessions), 0) +
  COALESCE((SELECT SUM(LENGTH(site)+LENGTH(session_id)+LENGTH(type)+LENGTH(page)+LENGTH(text)+?) FROM rum_session_events), 0) +
  COALESCE((SELECT SUM(LENGTH(site)+LENGTH(fingerprint)+LENGTH(type)+LENGTH(message)+LENGTH(sample_stack)+?) FROM rum_error_groups), 0) +
  COALESCE((SELECT SUM(LENGTH(site)+LENGTH(fingerprint)+LENGTH(session_id)+LENGTH(page)+LENGTH(browser)+?) FROM rum_error_occurrences), 0)
`, rumRowOverheadBytes, rumRowOverheadBytes, rumRowOverheadBytes, rumRowOverheadBytes)
	var total int64
	if err := row.Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}
