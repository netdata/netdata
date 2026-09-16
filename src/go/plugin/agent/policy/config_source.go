// SPDX-License-Identifier: GPL-3.0-or-later

package policy

import "github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"

// SecretReferencesAllowed reports whether a collector configuration source is
// trusted to exercise secret-reference providers.
func SecretReferencesAllowed(sourceType string) bool {
	switch sourceType {
	case confgroup.TypeStock, confgroup.TypeUser, confgroup.TypeDyncfg:
		return true
	default:
		return false
	}
}
