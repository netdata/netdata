// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package systemdunits

import (
	"errors"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

func (c *Collector) validateConfig() error {
	if len(c.Include) == 0 {
		return errors.New("'include' option not set")
	}
	return nil
}

func (c *Collector) initUnitSelector() (matcher.Matcher, error) {
	if len(c.Include) == 0 {
		return matcher.TRUE(), nil
	}

	expr := strings.Join(c.Include, " ")
	return matcher.NewSimplePatternsMatcher(expr)
}
