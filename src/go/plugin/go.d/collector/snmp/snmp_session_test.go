// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

type forgetCountingClient struct {
	snmputils.Session
	err     error
	forgets int
}

func (c *forgetCountingClient) Get(context.Context, []string) (*gosnmp.Message, error) {
	return nil, c.err
}

func (c *forgetCountingClient) WalkAll(context.Context, string) ([]gosnmp.VarBind, gosnmp.WalkEnd, error) {
	return nil, gosnmp.WalkEnd{}, c.err
}

func (c *forgetCountingClient) BulkWalkAll(context.Context, string, uint32) ([]gosnmp.VarBind, gosnmp.WalkEnd, error) {
	return nil, gosnmp.WalkEnd{}, c.err
}

func (c *forgetCountingClient) ForgetEngine() { c.forgets++ }

func TestEngineForgettingClient(t *testing.T) {
	unknownEngine := fmt.Errorf("%w (%w)", gosnmp.ErrUnknownEngineID, gosnmp.ErrAnswered)
	for name, tc := range map[string]struct {
		err         error
		wantForgets int
	}{
		"success":            {},
		"unknown engine ID":  {err: unknownEngine, wantForgets: 1},
		"other report":       {err: fmt.Errorf("%w (%w)", gosnmp.ErrNotInTimeWindow, gosnmp.ErrAnswered)},
		"timeout":            {err: context.DeadlineExceeded},
		"error-status":       {err: &gosnmp.StatusError{Status: gosnmp.GenErr}},
		"wrapped unknown ID": {err: fmt.Errorf("walk: %w", unknownEngine), wantForgets: 1},
	} {
		t.Run(name, func(t *testing.T) {
			for method, call := range map[string]func(*engineForgettingClient) error{
				"get": func(c *engineForgettingClient) error {
					_, err := c.Get(context.Background(), []string{"1.3.6.1.2.1.1.3.0"})
					return err
				},
				"walk": func(c *engineForgettingClient) error {
					_, _, err := c.WalkAll(context.Background(), "1.3.6.1.2.1.2")
					return err
				},
				"bulk walk": func(c *engineForgettingClient) error {
					_, _, err := c.BulkWalkAll(context.Background(), "1.3.6.1.2.1.2", 10)
					return err
				},
			} {
				fake := &forgetCountingClient{err: tc.err}
				client := &engineForgettingClient{engineClient: fake, log: logger.New()}
				err := call(client)
				assert.True(t, errors.Is(err, tc.err) || err == tc.err, "%s returns the client's error", method)
				assert.Equal(t, tc.wantForgets, fake.forgets, method)
			}
		})
	}
}
