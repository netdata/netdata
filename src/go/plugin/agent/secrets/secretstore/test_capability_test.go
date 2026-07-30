// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore_test

import (
	"context"
	"errors"
	"testing"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestSecretStoreTestCapability(t *testing.T) {
	t.Run("validation does not run the operational test", func(t *testing.T) {
		state := &testCapabilityState{}
		store, catalog := newTestCapabilityAuthority(t, func() secretstore.Store {
			return &testCapableStore{state: state}
		})

		require.NoError(t, store.Validate(t.Context(), catalog, testCapabilityConfig()))
		require.Zero(t, state.calls)

		tested, err := store.Test(t.Context(), catalog, testCapabilityConfig())
		require.NoError(t, err)
		require.True(t, tested)
		require.Equal(t, 1, state.calls)
		require.Equal(t, "configured", state.value)
		require.Equal(t, secretstore.SecretStoreCensus{}, store.Census())
	})

	t.Run("reports validation only for a Store without the capability", func(t *testing.T) {
		store, catalog := newTestCapabilityAuthority(t, func() secretstore.Store {
			return &testValidationOnlyStore{}
		})

		tested, err := store.Test(t.Context(), catalog, testCapabilityConfig())

		require.NoError(t, err)
		require.False(t, tested)
		require.Equal(t, secretstore.SecretStoreCensus{}, store.Census())
	})

	t.Run("returns an operational failure as a tested result", func(t *testing.T) {
		testErr := errors.New("provider unavailable")
		state := &testCapabilityState{err: testErr}
		store, catalog := newTestCapabilityAuthority(t, func() secretstore.Store {
			return &testCapableStore{state: state}
		})

		tested, err := store.Test(t.Context(), catalog, testCapabilityConfig())

		require.True(t, tested)
		require.ErrorIs(t, err, testErr)
		require.Equal(t, 1, state.calls)
		require.Equal(t, "configured", state.value)
		require.Equal(t, secretstore.SecretStoreCensus{}, store.Census())
	})

	t.Run("rejects an already-canceled request before Store creation", func(t *testing.T) {
		cancelCause := errors.New("test request superseded")
		var creates int
		store, catalog := newTestCapabilityAuthority(t, func() secretstore.Store {
			creates++
			return &testValidationOnlyStore{}
		})
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(cancelCause)

		tested, err := store.Test(ctx, catalog, testCapabilityConfig())

		require.False(t, tested)
		require.ErrorIs(t, err, cancelCause)
		require.Zero(t, creates)
		require.Equal(t, secretstore.SecretStoreCensus{}, store.Census())
	})

	t.Run("caller cancellation wins a Store initialization failure", func(t *testing.T) {
		cancelCause := errors.New("test request superseded")
		initErr := errors.New("provider initialization failed")
		ctx, cancel := context.WithCancelCause(t.Context())
		state := &testCapabilityState{
			init: func(context.Context) error {
				cancel(cancelCause)
				return initErr
			},
		}
		store, catalog := newTestCapabilityAuthority(t, func() secretstore.Store {
			return &testCapableStore{state: state}
		})

		tested, err := store.Test(ctx, catalog, testCapabilityConfig())

		require.False(t, tested)
		require.ErrorIs(t, err, cancelCause)
		require.NotErrorIs(t, err, initErr)
		require.Zero(t, state.calls)
		require.Equal(t, secretstore.SecretStoreCensus{}, store.Census())
	})

	t.Run("caller cancellation wins a provider operational failure", func(t *testing.T) {
		cancelCause := errors.New("test request superseded")
		providerErr := dyncfg.NewPublicError(
			"provider operation timed out",
			context.DeadlineExceeded,
		)
		ctx, cancel := context.WithCancelCause(t.Context())
		state := &testCapabilityState{
			test: func(context.Context) error {
				cancel(cancelCause)
				return providerErr
			},
		}
		store, catalog := newTestCapabilityAuthority(t, func() secretstore.Store {
			return &testCapableStore{state: state}
		})

		tested, err := store.Test(ctx, catalog, testCapabilityConfig())

		require.True(t, tested)
		require.ErrorIs(t, err, cancelCause)
		require.NotErrorIs(t, err, providerErr)
		_, hasPublicMessage := dyncfg.PublicMessage(err)
		require.False(t, hasPublicMessage)
		require.Equal(t, 1, state.calls)
		require.Equal(t, secretstore.SecretStoreCensus{}, store.Census())
	})
}

func newTestCapabilityAuthority(
	t *testing.T,
	create func() secretstore.Store,
) (*secretstore.SecretStore, *secretstore.CreatorCatalog) {
	t.Helper()
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	store, err := secretstore.NewSecretStore(resolver)
	require.NoError(t, err)
	catalog, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: create,
	}})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close(context.Background()))
	})
	return store, catalog
}

func testCapabilityConfig() secretstore.Config {
	return secretstore.Config{
		"name":            "main",
		"kind":            string(secretstore.KindVault),
		"value":           "configured",
		"__source__":      confgroup.TypeDyncfg,
		"__source_type__": confgroup.TypeDyncfg,
	}
}

type testCapabilityState struct {
	calls int
	err   error
	init  func(context.Context) error
	test  func(context.Context) error
	value string
}

type testCapableStore struct {
	state  *testCapabilityState
	config struct {
		Value string `yaml:"value"`
	}
}

func (s *testCapableStore) Configuration() any {
	return &s.config
}

func (s *testCapableStore) Init(ctx context.Context) error {
	if s.state.init != nil {
		return s.state.init(ctx)
	}
	return nil
}

func (s *testCapableStore) Publish() secretstore.PublishedStore {
	return testCapabilityPublished(s.config.Value)
}

func (s *testCapableStore) Test(ctx context.Context) error {
	s.state.calls++
	s.state.value = s.config.Value
	if s.state.test != nil {
		return s.state.test(ctx)
	}
	return s.state.err
}

type testValidationOnlyStore struct {
	config struct {
		Value string `yaml:"value"`
	}
}

func (s *testValidationOnlyStore) Configuration() any {
	return &s.config
}

func (s *testValidationOnlyStore) Init(context.Context) error {
	return nil
}

func (s *testValidationOnlyStore) Publish() secretstore.PublishedStore {
	return testCapabilityPublished(s.config.Value)
}

type testCapabilityPublished string

func (s testCapabilityPublished) Resolve(
	context.Context,
	secretstore.ResolveRequest,
) (string, error) {
	return string(s), nil
}
