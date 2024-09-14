// SPDX-License-Identifier: GPL-3.0-or-later

package activemq

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"
)

func (a *ActiveMQ) validateConfig() error {
	if a.URL == "" {
		return errors.New("url not set")
	}
	if a.Webadmin == "" {
		return errors.New("webadmin root path set")
	}
	return nil
}

func (a *ActiveMQ) initQueuesFiler() (matcher.Matcher, error) {
	if a.QueuesFilter == "" {
		return matcher.TRUE(), nil
	}
	return matcher.NewSimplePatternsMatcher(a.QueuesFilter)
}

func (a *ActiveMQ) initTopicsFilter() (matcher.Matcher, error) {
	if a.TopicsFilter == "" {
		return matcher.TRUE(), nil
	}
	return matcher.NewSimplePatternsMatcher(a.TopicsFilter)
}
