// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import "context"

// Handler is a Function callback with the request-scoped context managed by the
// Function manager.
type Handler func(context.Context, Function)

// Registry defines the interface for registering function handlers.
// Both jobmgr and service discovery use this to register dyncfg handlers.
type Registry interface {
	Register(name string, fn func(Function))
	Unregister(name string)
	RegisterPrefix(name, prefix string, fn func(Function))
	UnregisterPrefix(name string, prefix string)
}

// ContextRegistry is implemented by registries that can pass request
// cancellation directly to Function handlers.
type ContextRegistry interface {
	RegisterWithContext(name string, fn Handler)
	RegisterPrefixWithContext(name, prefix string, fn Handler)
}
