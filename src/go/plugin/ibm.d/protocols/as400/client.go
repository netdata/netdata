// Package as400 provides IBM i (AS/400) database access helpers for the ibm.d framework.
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo

package as400

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/pkg/dbdriver"
)

var (
	// ErrFeatureUnavailable indicates that a requested SQL service is not available on this IBM i system.
	ErrFeatureUnavailable = errors.New("as400 protocol: feature unavailable")
	// ErrTemporaryFailure indicates a transient database error that may succeed on retry.
	ErrTemporaryFailure = errors.New("as400 protocol: temporary failure")
)

// RowScanner receives a row's column names and values.
// Implementations should treat the values slice as read-only for the duration of the callback.
type RowScanner func(columns []string, values []string) error

// Config represents the connection options required to talk to IBM i.
type Config struct {
	DSN          string
	Timeout      time.Duration
	MaxOpenConns int
	ConnMaxLife  time.Duration
}

// Client wraps the SQL connection and exposes typed query helpers used by the collector.
type Client struct {
	cfg Config
	db  *sql.DB
}

// NewClient creates a new IBM i client with the supplied configuration.
func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg}
}

// Connect ensures the underlying connection is ready.
func (c *Client) Connect(ctx context.Context) error {
	if c.db != nil {
		return nil
	}

	if c.cfg.DSN == "" {
		return errors.New("as400 protocol: DSN is required")
	}

	db, err := sql.Open("odbcbridge", c.cfg.DSN)
	if err != nil {
		return fmt.Errorf("as400 protocol: opening connection failed: %w", err)
	}

	if c.cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(c.cfg.MaxOpenConns)
	}
	if c.cfg.ConnMaxLife > 0 {
		db.SetConnMaxLifetime(c.cfg.ConnMaxLife)
	}

	pingCtx, cancel := context.WithTimeout(ctx, c.effectiveTimeout())
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return fmt.Errorf("as400 protocol: ping failed: %w", err)
	}

	c.db = db
	return nil
}

// Ping verifies connectivity on an existing connection.
func (c *Client) Ping(ctx context.Context) error {
	if c.db == nil {
		return errors.New("as400 protocol: ping called before connect")
	}
	pingCtx, cancel := context.WithTimeout(ctx, c.effectiveTimeout())
	defer cancel()
	return c.db.PingContext(pingCtx)
}

// Close terminates the connection.
func (c *Client) Close() error {
	if c.db == nil {
		return nil
	}
	err := c.db.Close()
	c.db = nil
	return err
}

// DoQuery executes a query and streams rows to the handler.
func (c *Client) DoQuery(ctx context.Context, query string, fn func(column, value string, lineEnd bool)) error {
	rows, cancel, err := c.QueryRows(ctx, query)
	if err != nil {
		return err
	}
	defer cancel()
	defer rows.Close()

	return c.readRows(rows, fn)
}

// DoQueryRow executes a query expected to return a single row.
func (c *Client) DoQueryRow(ctx context.Context, query string, fn func(column, value string)) error {
	rows, cancel, err := c.QueryRows(ctx, query)
	if err != nil {
		return err
	}
	defer cancel()
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("as400 protocol: fetching columns failed: %w", err)
	}

	scan := make([]sql.RawBytes, len(columns))
	pointers := make([]any, len(columns))
	for i := range scan {
		pointers[i] = &scan[i]
	}

	if rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return fmt.Errorf("as400 protocol: scanning row failed: %w", err)
		}
		for idx, col := range columns {
			fn(col, string(scan[idx]))
		}
	}

	return rows.Err()
}

// QueryRows runs the query and returns the raw rows.
func (c *Client) QueryRows(ctx context.Context, query string) (*sql.Rows, context.CancelFunc, error) {
	if err := c.Connect(ctx); err != nil {
		return nil, nil, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, c.effectiveTimeout())
	rows, err := c.db.QueryContext(queryCtx, query)
	if err != nil {
		cancel()
		return nil, nil, classifySQLError(err)
	}

	return rows, cancel, nil
}

// Query executes the query and streams rows to the provided scanner.
func (c *Client) Query(ctx context.Context, query string, scan RowScanner) error {
	rows, cancel, err := c.QueryRows(ctx, query)
	if err != nil {
		return err
	}
	defer cancel()
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("as400 protocol: reading columns failed: %w", err)
	}

	raw := make([]sql.RawBytes, len(columns))
	ptrs := make([]any, len(columns))
	for i := range raw {
		ptrs[i] = &raw[i]
	}

	values := make([]string, len(columns))

	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return fmt.Errorf("as400 protocol: scanning row failed: %w", err)
		}
		for i := range raw {
			if raw[i] == nil {
				values[i] = "NULL"
			} else {
				values[i] = string(raw[i])
			}
		}
		if err := scan(columns, values); err != nil {
			return err
		}
	}

	return rows.Err()
}

// QueryWithLimit executes the query and enforces a FETCH FIRST limit when limit > 0.
func (c *Client) QueryWithLimit(ctx context.Context, query string, limit int, scan RowScanner) error {
	return c.Query(ctx, applyFetchLimit(query, limit), scan)
}

// Exec runs a statement that does not return rows.
func (c *Client) Exec(ctx context.Context, statement string) error {
	if err := c.Connect(ctx); err != nil {
		return err
	}
	execCtx, cancel := context.WithTimeout(ctx, c.effectiveTimeout())
	defer cancel()
	if _, err := c.db.ExecContext(execCtx, statement); err != nil {
		return classifySQLError(err)
	}
	return nil
}

func applyFetchLimit(query string, limit int) string {
	if limit <= 0 {
		return query
	}
	trimmed := strings.TrimSpace(query)
	upper := strings.ToUpper(trimmed)
	if strings.Contains(upper, "FETCH FIRST") {
		return trimmed
	}
	return fmt.Sprintf("%s FETCH FIRST %d ROWS ONLY", trimmed, limit)
}

func (c *Client) readRows(rows *sql.Rows, fn func(column, value string, lineEnd bool)) error {
	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("as400 protocol: reading columns failed: %w", err)
	}

	scan := make([]sql.RawBytes, len(columns))
	pointers := make([]any, len(columns))
	for i := range scan {
		pointers[i] = &scan[i]
	}

	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return fmt.Errorf("as400 protocol: scanning row failed: %w", err)
		}
		for idx, col := range columns {
			fn(col, string(scan[idx]), idx == len(columns)-1)
		}
	}

	return rows.Err()
}

func (c *Client) effectiveTimeout() time.Duration {
	if c.cfg.Timeout > 0 {
		return c.cfg.Timeout
	}
	return 5 * time.Second
}

// DB exposes the underlying handle for operations that still rely on *sql.DB.
func (c *Client) DB() *sql.DB {
	return c.db
}

var featureErrorTokens = []string{
	"SQL0204",
	"SQL0206",
	"SQL0443",
	"SQL0551",
	"SQL7024",
	"SQL0707",
	"SQLCODE=-204",
	"SQLCODE=-206",
	"SQLCODE=-443",
	"SQLCODE=-551",
	"SQLCODE=-707",
}

var temporaryErrorTokens = []string{
	"SQL0519",
	"SQLCODE=-519",
}

// IsFeatureError reports whether err indicates an unavailable SQL service.
func IsFeatureError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrFeatureUnavailable) {
		return true
	}
	msg := strings.ToUpper(err.Error())
	for _, token := range featureErrorTokens {
		if strings.Contains(msg, token) {
			return true
		}
	}
	return false
}

// IsTemporaryError reports whether err is a transient database error that may succeed on retry.
func IsTemporaryError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrTemporaryFailure) {
		return true
	}
	msg := strings.ToUpper(err.Error())
	for _, token := range temporaryErrorTokens {
		if strings.Contains(msg, token) {
			return true
		}
	}
	return false
}

func classifySQLError(err error) error {
	if err == nil {
		return nil
	}
	if IsFeatureError(err) {
		return fmt.Errorf("%w: %s", ErrFeatureUnavailable, err)
	}
	if IsTemporaryError(err) {
		return fmt.Errorf("%w: %s", ErrTemporaryFailure, err)
	}
	return fmt.Errorf("as400 protocol: query failed: %w", err)
}
