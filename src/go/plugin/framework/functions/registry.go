// SPDX-License-Identifier: GPL-3.0-or-later

package functions

// Registry defines the interface for registering function handlers.
// Both jobmgr and service discovery use this to register dyncfg handlers.
type Registry interface {
	Register(name string, fn func(Function))
	Unregister(name string)
	RegisterPrefix(name, prefix string, fn func(Function))
	UnregisterPrefix(name string, prefix string)
}
