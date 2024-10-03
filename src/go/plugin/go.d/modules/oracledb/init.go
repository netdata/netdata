// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"errors"
	"net/url"
	"strings"

	goora "github.com/sijms/go-ora/v2"
)

func (o *OracleDB) validateDSN() (string, error) {
	if o.DSN == "" {
		return "", errors.New("dsn required but not set")
	}
	if _, err := goora.ParseConfig(o.DSN); err != nil {
		return "", err
	}

	u, err := url.Parse(o.DSN)
	if err != nil {
		return "", err
	}

	if u.User == nil {
		return u.String(), nil
	}

	var user, pass string
	if user = u.User.Username(); user != "" {
		user = strings.Repeat("*", len(user))
	}
	if pass, _ = u.User.Password(); pass != "" {
		pass = strings.Repeat("*", len(pass))
	}

	u.User = url.UserPassword(user, pass)

	return u.String(), nil
}
