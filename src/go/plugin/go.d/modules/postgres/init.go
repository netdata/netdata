// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"
)

func (p *Postgres) validateConfig() error {
	if p.DSN == "" {
		return errors.New("DSN not set")
	}
	return nil
}

func (p *Postgres) initDBSelector() (matcher.Matcher, error) {
	if p.DBSelector == "" {
		return nil, nil
	}

	return matcher.NewSimplePatternsMatcher(p.DBSelector)
}
