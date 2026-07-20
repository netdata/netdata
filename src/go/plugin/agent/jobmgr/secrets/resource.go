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
	mu sync.Mutex // guards retired

	identity lifecycle.ResourceIdentity // resource identity of the store generation
	store    *secretstore.SecretStore   // the secret store
	key      string                     // store key (kind_name)
	storeGen uint64                     // the store generation number this resource retires
	retired  bool                       // the generation has been retired
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

func (sgr *storeGenerationResource) Identity() lifecycle.ResourceIdentity {
	if sgr == nil {
		return lifecycle.ResourceIdentity{}
	}
	return sgr.identity
}

func (*storeGenerationResource) Publish() error { return nil }

func (sgr *storeGenerationResource) AbortReady(
	ctx context.Context,
) error {
	return sgr.retire(ctx)
}

func (*storeGenerationResource) Stop(context.Context) error { return nil }

func (sgr *storeGenerationResource) Finalize() error {
	return sgr.retire(context.Background())
}

func (sgr *storeGenerationResource) supersede() error {
	if sgr == nil {
		return errors.New(
			"jobmgr secrets: nil superseded Store resource",
		)
	}
	sgr.mu.Lock()
	defer sgr.mu.Unlock()
	if sgr.retired {
		return errors.New(
			"jobmgr secrets: Store resource already retired",
		)
	}
	sgr.retired = true
	return nil
}

func (sgr *storeGenerationResource) retire(
	ctx context.Context,
) error {
	if sgr == nil || ctx == nil {
		return errors.New(
			"jobmgr secrets: invalid Store resource retirement",
		)
	}
	sgr.mu.Lock()
	if sgr.retired {
		sgr.mu.Unlock()
		return errors.New(
			"jobmgr secrets: Store resource retired twice",
		)
	}
	sgr.retired = true
	store, key, generation :=
		sgr.store, sgr.key, sgr.storeGen
	sgr.mu.Unlock()
	return store.Retire(ctx, key, generation)
}
