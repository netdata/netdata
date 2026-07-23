// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"context"
	"fmt"
	"maps"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"gopkg.in/yaml.v2"
)

type preparedStore struct {
	key        string
	rawConfig  Config
	configHash uint64
	published  PublishedStore
}

func preparePublishedConfig(
	ctx context.Context,
	resolver *secretresolver.AtomicResolver,
	cfg Config,
	newStore func(StoreKind) (Store, bool),
) (preparedStore, error) {
	if cfg == nil {
		return preparedStore{}, fmt.Errorf("store config is nil")
	}
	if newStore == nil {
		return preparedStore{}, fmt.Errorf("store creator is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	raw := cloneConfig(cfg)
	if raw == nil {
		return preparedStore{}, fmt.Errorf("store config is nil")
	}
	if err := raw.Validate(); err != nil {
		return preparedStore{}, err
	}
	rawConfig := cloneConfig(raw)
	rawHash := raw.Hash()
	resolvedPayload, err := resolveProviderPayload(ctx, resolver, raw)
	if err != nil {
		return preparedStore{}, err
	}

	kind := raw.Kind()
	key := raw.ExposedKey()

	store, ok := newStore(kind)
	if !ok {
		return preparedStore{}, fmt.Errorf("store kind '%s' is not supported", kind)
	}
	if store.Configuration() == nil {
		return preparedStore{}, fmt.Errorf("store '%s': configuration is nil", key)
	}

	bs, err := yaml.Marshal(raw)
	if err != nil {
		return preparedStore{}, fmt.Errorf("store '%s': marshaling raw config: %w", key, err)
	}
	if len(resolvedPayload) != 0 {
		maps.Copy(raw, resolvedPayload)
		bs, err = yaml.Marshal(raw)
		if err != nil {
			return preparedStore{}, fmt.Errorf("store '%s': marshaling resolved config: %w", key, err)
		}
	}
	if err := yaml.Unmarshal(bs, store.Configuration()); err != nil {
		return preparedStore{}, fmt.Errorf("store '%s': invalid provider payload: %w", key, err)
	}

	if err := store.Init(ctx); err != nil {
		return preparedStore{}, err
	}

	published := store.Publish()
	if published == nil {
		return preparedStore{}, fmt.Errorf("store '%s': published resolver state is nil", key)
	}

	return preparedStore{
		key:        key,
		rawConfig:  rawConfig,
		configHash: rawHash,
		published:  published,
	}, nil
}
