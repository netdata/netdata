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

type configuredStore struct {
	key        string
	rawConfig  Config
	configHash uint64
	store      Store
}

// ValidateStructure checks available fields and supported reference syntax without
// acquiring credentials or invoking provider initialization.
func (store *SecretStore) ValidateStructure(catalog *CreatorCatalog, cfg Config) error {
	if store == nil || catalog == nil {
		return fmt.Errorf("secretstore: invalid structural validation")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	payload := cfg.ProviderPayload()
	value, hasReferences, err := store.resolver.CloneForValidation(payload)
	if err != nil {
		return err
	}
	if hasReferences {
		keys, err := secretresolver.StoreReferences(payload)
		if err != nil {
			return err
		}
		if len(keys) != 0 {
			return fmt.Errorf("secretstore: Store references are not supported in provider configuration")
		}
	}
	provider, ok := catalog.New(cfg.Kind())
	if !ok || provider.Configuration() == nil {
		return fmt.Errorf("secretstore: provider configuration unavailable")
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, provider.Configuration()); err != nil {
		return err
	}
	_, err = cfg.PayloadJSON()
	return err
}

func configureStore(
	ctx context.Context,
	resolver *secretresolver.AtomicResolver,
	cfg Config,
	newStore func(StoreKind) (Store, bool),
) (configuredStore, error) {
	if cfg == nil {
		return configuredStore{}, fmt.Errorf("store config is nil")
	}
	if newStore == nil {
		return configuredStore{}, fmt.Errorf("store creator is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	raw := cloneConfig(cfg)
	if raw == nil {
		return configuredStore{}, fmt.Errorf("store config is nil")
	}
	if err := raw.Validate(); err != nil {
		return configuredStore{}, err
	}
	rawConfig := cloneConfig(raw)
	rawHash := raw.Hash()
	resolvedPayload, err := resolveProviderPayload(ctx, resolver, raw)
	if err != nil {
		return configuredStore{}, err
	}

	kind := raw.Kind()
	key := raw.ExposedKey()

	store, ok := newStore(kind)
	if !ok {
		return configuredStore{}, fmt.Errorf("store kind '%s' is not supported", kind)
	}
	if store.Configuration() == nil {
		return configuredStore{}, fmt.Errorf("store '%s': configuration is nil", key)
	}

	bs, err := yaml.Marshal(raw)
	if err != nil {
		return configuredStore{}, fmt.Errorf("store '%s': marshaling raw config: %w", key, err)
	}
	if len(resolvedPayload) != 0 {
		maps.Copy(raw, resolvedPayload)
		bs, err = yaml.Marshal(raw)
		if err != nil {
			return configuredStore{}, fmt.Errorf("store '%s': marshaling resolved config: %w", key, err)
		}
	}
	if err := yaml.Unmarshal(bs, store.Configuration()); err != nil {
		return configuredStore{}, fmt.Errorf("store '%s': invalid provider payload: %w", key, err)
	}

	if err := store.Init(ctx); err != nil {
		return configuredStore{}, err
	}

	return configuredStore{
		key:        key,
		rawConfig:  rawConfig,
		configHash: rawHash,
		store:      store,
	}, nil
}

func preparePublishedConfig(
	ctx context.Context,
	resolver *secretresolver.AtomicResolver,
	cfg Config,
	newStore func(StoreKind) (Store, bool),
) (preparedStore, error) {
	configured, err := configureStore(ctx, resolver, cfg, newStore)
	if err != nil {
		return preparedStore{}, err
	}
	published := configured.store.Publish()
	if published == nil {
		return preparedStore{}, fmt.Errorf(
			"store '%s': published resolver state is nil",
			configured.key,
		)
	}

	return preparedStore{
		key:        configured.key,
		rawConfig:  configured.rawConfig,
		configHash: configured.configHash,
		published:  published,
	}, nil
}
