// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
)

type storeGenerationResource struct {
	mu sync.Mutex

	identity lifecycle.ResourceIdentity
	store    *secretstore.SecretStore
	key      string
	storeGen uint64
	retired  bool
}

func newStoreGenerationResource(
	identity lifecycle.ResourceIdentity,
	store *secretstore.SecretStore,
	key string,
	storeGeneration uint64,
) (*storeGenerationResource, error) {
	if !identity.Valid() ||
		store == nil ||
		key == "" ||
		storeGeneration == 0 {
		return nil, errors.New(
			"jobmgr secrets: invalid Store-generation resource",
		)
	}
	return &storeGenerationResource{
		identity: identity,
		store:    store,
		key:      key,
		storeGen: storeGeneration,
	}, nil
}

func (resource *storeGenerationResource) Identity() lifecycle.ResourceIdentity {
	if resource == nil {
		return lifecycle.ResourceIdentity{}
	}
	return resource.identity
}

func (*storeGenerationResource) Publish() error { return nil }

func (resource *storeGenerationResource) AbortReady(
	ctx context.Context,
) error {
	return resource.retire(ctx)
}

func (*storeGenerationResource) Stop(context.Context) error { return nil }

func (resource *storeGenerationResource) Finalize() error {
	return resource.retire(context.Background())
}

func (resource *storeGenerationResource) supersede() error {
	if resource == nil {
		return errors.New(
			"jobmgr secrets: nil superseded Store resource",
		)
	}
	resource.mu.Lock()
	defer resource.mu.Unlock()
	if resource.retired {
		return errors.New(
			"jobmgr secrets: Store resource already retired",
		)
	}
	resource.retired = true
	return nil
}

func (resource *storeGenerationResource) retire(
	ctx context.Context,
) error {
	if resource == nil || ctx == nil {
		return errors.New(
			"jobmgr secrets: invalid Store resource retirement",
		)
	}
	resource.mu.Lock()
	if resource.retired {
		resource.mu.Unlock()
		return errors.New(
			"jobmgr secrets: Store resource retired twice",
		)
	}
	resource.retired = true
	store, key, generation :=
		resource.store, resource.key, resource.storeGen
	resource.mu.Unlock()
	return store.Retire(ctx, key, generation)
}
