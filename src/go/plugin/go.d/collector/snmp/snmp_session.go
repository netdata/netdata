// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// newSNMPSession makes the job's SNMP client from cfg.
func newSNMPSession(cfg gosnmp.Config, log *logger.Logger) (snmputils.Session, error) {
	client, err := gosnmp.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &engineForgettingClient{engineClient: client, log: log}, nil
}

// engineForgettingClient keeps a job-lived SNMPv3 client working across an
// agent's engine ID change (a replaced device, or a forged discovery answer):
// the client never leaves an engine it learned by itself, so a request that
// fails with an unknown engine ID makes it forget the engine, and the next
// request discovers it again. One goroutine uses the client, as ForgetEngine
// requires.
type engineForgettingClient struct {
	engineClient
	log *logger.Logger
}

// engineClient is the *gosnmp.Client methods the job uses.
type engineClient interface {
	snmputils.Session
	ForgetEngine()
}

func (c *engineForgettingClient) Get(ctx context.Context, oids []string) (*gosnmp.Message, error) {
	msg, err := c.engineClient.Get(ctx, oids)
	c.forgetOnUnknownEngine(err)
	return msg, err
}

func (c *engineForgettingClient) WalkAll(ctx context.Context, rootOid string) ([]gosnmp.VarBind, gosnmp.WalkEnd, error) {
	values, end, err := c.engineClient.WalkAll(ctx, rootOid)
	c.forgetOnUnknownEngine(err)
	return values, end, err
}

func (c *engineForgettingClient) BulkWalkAll(ctx context.Context, rootOid string, maxRepetitions uint32) ([]gosnmp.VarBind, gosnmp.WalkEnd, error) {
	values, end, err := c.engineClient.BulkWalkAll(ctx, rootOid, maxRepetitions)
	c.forgetOnUnknownEngine(err)
	return values, end, err
}

func (c *engineForgettingClient) forgetOnUnknownEngine(err error) {
	if !errors.Is(err, gosnmp.ErrUnknownEngineID) {
		return
	}
	c.ForgetEngine()
	if c.log != nil {
		c.log.Limit("snmp:forget_engine", 1, 5*time.Minute).
			Infof("the SNMPv3 agent does not know the engine ID the client holds; the next request discovers it again: %v", err)
	}
}
