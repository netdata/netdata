// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux
// +build linux

package systemdunits

import (
	"errors"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"
)

func (s *SystemdUnits) validateConfig() error {
	if len(s.Include) == 0 {
		return errors.New("'include' option not set")
	}
	return nil
}

func (s *SystemdUnits) initUnitSelector() (matcher.Matcher, error) {
	if len(s.Include) == 0 {
		return matcher.TRUE(), nil
	}

	expr := strings.Join(s.Include, " ")
	return matcher.NewSimplePatternsMatcher(expr)
}
