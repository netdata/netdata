// SPDX-License-Identifier: GPL-3.0-or-later

package policy

import "github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"

// SecretReferencesAllowed checks collector source authority and the pipeline's trust stamp.
func SecretReferencesAllowed(config confgroup.Config) bool {
	switch config.SourceType() {
	case confgroup.TypeStock, confgroup.TypeUser, confgroup.TypeDyncfg:
		return true
	case confgroup.TypeDiscovered:
		return config.TrustDiscoveredTargets()
	default:
		return false
	}
}
