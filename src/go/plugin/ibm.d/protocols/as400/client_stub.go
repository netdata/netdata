//go:build !cgo

package as400

import (
	"context"
	"database/sql"
	"errors"
)

type Config struct{}

type Client struct{}

func NewClient(Config) *Client { return &Client{} }

func (c *Client) Connect(context.Context) error { return errors.New("as400 protocol requires cgo") }
func (c *Client) Ping(context.Context) error    { return errors.New("as400 protocol requires cgo") }
func (c *Client) Close() error                  { return nil }
func (c *Client) DoQuery(context.Context, string, func(string, string, bool)) error {
	return errors.New("as400 protocol requires cgo")
}
func (c *Client) DoQueryRow(context.Context, string, func(string, string)) error {
	return errors.New("as400 protocol requires cgo")
}
func (c *Client) QueryRows(context.Context, string) (*sql.Rows, context.CancelFunc, error) {
	return nil, nil, errors.New("as400 protocol requires cgo")
}
func (c *Client) DB() *sql.DB { return nil }
