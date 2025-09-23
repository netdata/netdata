// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !cgo

package db2

import (
	"context"
	"database/sql"
	"errors"
)

type Config struct{}

type Client struct{}

func NewClient(Config) *Client { return &Client{} }

func (c *Client) Connect(context.Context) error { return errors.New("db2 protocol requires CGO") }

func (c *Client) Ping(context.Context) error { return errors.New("db2 protocol requires CGO") }

func (c *Client) Close() error { return nil }

func (c *Client) DoQuery(context.Context, string, func(string, string, bool)) error {
	return errors.New("db2 protocol requires CGO")
}

func (c *Client) DoQueryRow(context.Context, string, func(string, string)) error {
	return errors.New("db2 protocol requires CGO")
}

func (c *Client) QueryRows(context.Context, string) (*sql.Rows, error) {
	return nil, errors.New("db2 protocol requires CGO")
}

func (c *Client) DB() *sql.DB { return nil }
