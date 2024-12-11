// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"errors"
	"net/url"
	"strings"

	goora "github.com/sijms/go-ora/v2"
)

func (c *Collector) validateDSN() (string, error) {
	if c.DSN == "" {
		return "", errors.New("dsn required but not set")
	}
	if _, err := goora.ParseConfig(c.DSN); err != nil {
		return "", err
	}

	u, err := url.Parse(c.DSN)
	if err != nil {
		return "", err
	}

	if u.User == nil {
		return u.String(), nil
	}

	var user, pass string
	if user = u.User.Username(); user != "" {
		user = strings.Repeat("x", len(user))
	}
	if pass, _ = u.User.Password(); pass != "" {
		pass = strings.Repeat("x", len(pass))
	}

	u.User = url.UserPassword(user, pass)

	return u.String(), nil
}
