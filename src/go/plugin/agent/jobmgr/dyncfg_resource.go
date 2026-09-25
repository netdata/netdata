// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

// DynCfgNamedResourceID is shared by wire routing and internal object commands.
func DynCfgNamedResourceID(scopePrefix, kind, name string) string {
	if name == "" {
		return scopePrefix + kind
	}
	return scopePrefix + kind + "_" + name
}
