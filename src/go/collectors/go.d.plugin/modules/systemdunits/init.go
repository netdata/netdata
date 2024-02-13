// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux
// +build linux

package systemdunits

import (
	"errors"
	"strings"

	"github.com/netdata/go.d.plugin/pkg/matcher"
)

func (s *SystemdUnits) validateConfig() error {
	if len(s.Include) == 0 {
		return errors.New("'include' option not set")
	}
	return nil
}

func (s *SystemdUnits) initSelector() (matcher.Matcher, error) {
	if len(s.Include) == 0 {
		return matcher.TRUE(), nil
	}

	expr := strings.Join(s.Include, " ")
	return matcher.NewSimplePatternsMatcher(expr)
}
