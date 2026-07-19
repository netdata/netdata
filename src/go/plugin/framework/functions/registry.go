// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import "context"

// Handler is a Function callback with its request-scoped context.
type Handler func(context.Context, Function)

// Registry defines the interface for registering function handlers.
// Job Manager and service discovery use it to publish handlers without owning
// ingress or invocation lifecycle.
type Registry interface {
	Register(name string, fn func(Function))
	Unregister(name string)
	RegisterPrefix(name, prefix string, fn func(Function))
	UnregisterPrefix(name string, prefix string)
}

// ContextRegistry is implemented by registries that pass request cancellation
// directly to Function handlers.
type ContextRegistry interface {
	RegisterWithContext(name string, fn Handler)
	RegisterPrefixWithContext(name, prefix string, fn Handler)
}
