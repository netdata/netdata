// Package as400 provides IBM i (AS/400) database access helpers for the ibm.d framework.
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo

package as400

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/pkg/dbdriver"
)

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
		return nil, nil, fmt.Errorf("as400 protocol: query failed: %w", err)
	}

	return rows, cancel, nil
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
