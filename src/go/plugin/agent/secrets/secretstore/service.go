// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import "context"

type Service interface {
	Capture() *Snapshot
	Resolve(ctx context.Context, snapshot *Snapshot, ref, original string) (string, error)

	Kinds() []StoreKind
	DisplayName(kind StoreKind) (string, bool)
	Schema(kind StoreKind) (string, bool)
	New(kind StoreKind) (Store, bool)

	GetStatus(key string) (StoreStatus, bool)

	Validate(ctx context.Context, cfg Config) error
	ValidateStored(ctx context.Context, key string) error
	Add(ctx context.Context, cfg Config) error
	Update(ctx context.Context, key string, cfg Config) error
	Remove(key string) error
}

type Creator struct {
	Kind        StoreKind
	DisplayName string
	Schema      string
	Create      func() Store
}

type Store interface {
	Configuration() any
	Init(context.Context) error
	Publish() PublishedStore
}

type PublishedStore interface {
	Resolve(ctx context.Context, req ResolveRequest) (string, error)
}
