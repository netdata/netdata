// SPDX-License-Identifier: GPL-3.0-or-later

// Package vnodeguid validates captured DEM vnode identities.
package vnodeguid

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

// Validate accepts an empty identity for observations without a configured vnode.
func Validate(value string) error {
	if value == "" {
		return nil
	}
	canonical, err := vnodes.ConfiguredGUIDKey(value)
	if err != nil || canonical != value {
		return errors.New("vnode_guid must be empty or a canonical UUID")
	}
	return nil
}
