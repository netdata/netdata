// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redis/redisfunc"
)

type funcDepsAdapter struct {
	collector *Collector
}

func (a funcDepsAdapter) Client() (redisfunc.Client, error) {
	return a.collector.currentClient()
}

func (c *Collector) currentClient() (redisClient, error) {
	c.rdbMu.RLock()
	rdb := c.rdb
	c.rdbMu.RUnlock()
	if rdb == nil {
		return nil, errors.New("redis client is not ready")
	}
	return rdb, nil
}

func (c *Collector) setClient(rdb redisClient) {
	c.rdbMu.Lock()
	c.rdb = rdb
	c.rdbMu.Unlock()
}

func (c *Collector) closeClient() error {
	c.rdbMu.Lock()
	rdb := c.rdb
	c.rdb = nil
	c.rdbMu.Unlock()
	if rdb == nil {
		return nil
	}
	return rdb.Close()
}
