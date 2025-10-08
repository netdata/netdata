//go:build !cgo

package jmx

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/jmxbridge"
)

func NewClient(Config, jmxbridge.Logger, ...Option) (*Client, error) {
	return nil, errors.New("websphere jmx protocol requires CGO support")
}

func (c *Client) Start(context.Context) error {
	return errors.New("websphere jmx protocol requires CGO support")
}

func (c *Client) Shutdown() {}

func (c *Client) FetchJVM(context.Context) (*JVMStats, error) {
	return nil, errors.New("websphere jmx protocol requires CGO support")
}

func (c *Client) FetchThreadPools(context.Context, int) ([]ThreadPool, error) {
	return nil, errors.New("websphere jmx protocol requires CGO support")
}
