// SPDX-License-Identifier: GPL-3.0-or-later

package activemq

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("url not set")
	}
	if c.Webadmin == "" {
		return errors.New("webadmin root path set")
	}
	return nil
}

func (c *Collector) initQueuesFiler() (matcher.Matcher, error) {
	if c.QueuesFilter == "" {
		return matcher.TRUE(), nil
	}
	return matcher.NewSimplePatternsMatcher(c.QueuesFilter)
}

func (c *Collector) initTopicsFilter() (matcher.Matcher, error) {
	if c.TopicsFilter == "" {
		return matcher.TRUE(), nil
	}
	return matcher.NewSimplePatternsMatcher(c.TopicsFilter)
}
