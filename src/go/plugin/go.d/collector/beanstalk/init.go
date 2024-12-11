// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

func (c *Collector) validateConfig() error {
	if c.Address == "" {
		return errors.New("beanstalk address is required")
	}
	return nil
}

func (c *Collector) initTubeSelector() (matcher.Matcher, error) {
	if c.TubeSelector == "" {
		return matcher.TRUE(), nil
	}

	m, err := matcher.NewSimplePatternsMatcher(c.TubeSelector)
	if err != nil {
		return nil, err
	}

	return m, nil
}
