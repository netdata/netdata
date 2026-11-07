// SPDX-License-Identifier: GPL-3.0-or-later

// Package siteid validates optional DEM grouping identities, independently of job ownership.
package siteid

import (
	"errors"
	"regexp"
)

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Validate accepts an empty association for an unassigned job.
func Validate(id string) error {
	if id != "" && !identifier.MatchString(id) {
		return errors.New("site_id must be empty or 1-64 lowercase letters, digits, underscores or hyphens, starting with a letter or digit")
	}
	return nil
}
