// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

func (c *Collector) validateConfig() error {
	if c.DSN == "" {
		return errors.New("DSN not set")
	}
	return nil
}

func (c *Collector) initDBSelector() (matcher.Matcher, error) {
	if c.DBSelector == "" {
		return nil, nil
	}

	return matcher.NewSimplePatternsMatcher(c.DBSelector)
}
