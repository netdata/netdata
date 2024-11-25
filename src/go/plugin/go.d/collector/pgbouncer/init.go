// SPDX-License-Identifier: GPL-3.0-or-later

package pgbouncer

import "errors"

func (p *PgBouncer) validateConfig() error {
	if p.DSN == "" {
		return errors.New("DSN not set")
	}
	return nil
}
