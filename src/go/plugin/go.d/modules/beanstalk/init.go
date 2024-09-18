// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

func (b *Beanstalk) validateConfig() error {
	if b.Address == "" {
		return errors.New("beanstalk address is required")
	}
	return nil
}

func (b *Beanstalk) initTubeSelector() (matcher.Matcher, error) {
	if b.TubeSelector == "" {
		return matcher.TRUE(), nil
	}

	m, err := matcher.NewSimplePatternsMatcher(b.TubeSelector)
	if err != nil {
		return nil, err
	}

	return m, nil
}
