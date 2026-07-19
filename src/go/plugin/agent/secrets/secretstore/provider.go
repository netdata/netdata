// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import "context"

// Creator describes one SecretStore provider implementation.
type Creator struct {
	Kind        StoreKind
	DisplayName string
	Schema      string
	Create      func() Store
}

// Store is a mutable provider instance used during configuration.
type Store interface {
	Configuration() any
	Init(context.Context) error
	Publish() PublishedStore
}

// PublishedStore is immutable provider state used for resolution.
type PublishedStore interface {
	Resolve(ctx context.Context, req ResolveRequest) (string, error)
}
