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
	cancelCause := errors.New("test request superseded")
	testErr := errors.New("provider unavailable")
	initErr := errors.New("provider initialization failed")
	providerErr := dyncfg.NewPublicError(
		"provider operation timed out",
		context.DeadlineExceeded,
	)
	tests := map[string]struct {
		validationOnly      bool
		validateFirst       bool
		storeErr            error
		initErr             error
		testResult          error
		cancelBefore        bool
		cancelDuringInit    bool
		cancelDuringTest    bool
		wantTested          bool
		wantErr             error
		wantNotErr          error
		wantCalls           int
		wantValue           string
		wantNoCreates       bool
		wantNoPublicMessage bool
	}{
		"validation does not run the operational test": {
			validateFirst: true,
			wantTested:    true,
			wantCalls:     1,
			wantValue:     "configured",
		},
		"reports validation only for a Store without the capability": {
			validationOnly: true,
		},
		"reports validation only when the configured Store does not support a test": {
			storeErr:  dyncfg.ErrTestUnsupported,
			wantCalls: 1,
			wantValue: "configured",
		},
		"returns an operational failure as a tested result": {
			storeErr:   testErr,
			wantTested: true,
			wantErr:    testErr,
			wantCalls:  1,
			wantValue:  "configured",
		},
		"rejects an already-canceled request before Store creation": {
			validationOnly: true,
			cancelBefore:   true,
			wantErr:        cancelCause,
			wantNoCreates:  true,
		},
		"caller cancellation wins a Store initialization failure": {
			initErr:          initErr,
			cancelDuringInit: true,
			wantErr:          cancelCause,
			wantNotErr:       initErr,
		},
		"caller cancellation wins a provider operational failure": {
			testResult:          providerErr,
			cancelDuringTest:    true,
			wantTested:          true,
			wantErr:             cancelCause,
			wantNotErr:          providerErr,
			wantCalls:           1,
			wantValue:           "configured",
			wantNoPublicMessage: true,
		},
		"caller cancellation wins an unsupported result": {
			testResult:       dyncfg.ErrTestUnsupported,
			cancelDuringTest: true,
			wantTested:       true,
			wantErr:          cancelCause,
			wantNotErr:       dyncfg.ErrTestUnsupported,
			wantCalls:        1,
			wantValue:        "configured",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			var cancel context.CancelCauseFunc
			if tc.cancelBefore || tc.cancelDuringInit || tc.cancelDuringTest {
				ctx, cancel = context.WithCancelCause(ctx)
				defer cancel(nil)
			}
			state := &testCapabilityState{err: tc.storeErr}
			if tc.initErr != nil {
				state.init = func(context.Context) error {
					if tc.cancelDuringInit {
						cancel(cancelCause)
					}
					return tc.initErr
				}
			}
			if tc.testResult != nil {
				state.test = func(context.Context) error {
					if tc.cancelDuringTest {
						cancel(cancelCause)
					}
					return tc.testResult
				}
			}
			var creates int
			store, catalog := newTestCapabilityAuthority(t, func() secretstore.Store {
				creates++
				if tc.validationOnly {
					return &testValidationOnlyStore{}
				}
				return &testCapableStore{state: state}
			})
			if tc.validateFirst {
				require.NoError(t, store.Validate(ctx, catalog, testCapabilityConfig()))
				require.Zero(t, state.calls)
			}
			if tc.cancelBefore {
				cancel(cancelCause)
			}

			tested, err := store.Test(ctx, catalog, testCapabilityConfig())

			require.Equal(t, tc.wantTested, tested)
			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}
			if tc.wantNotErr != nil {
				require.NotErrorIs(t, err, tc.wantNotErr)
			}
			if tc.wantNoPublicMessage {
				_, hasPublicMessage := dyncfg.PublicMessage(err)
				require.False(t, hasPublicMessage)
			}
			require.Equal(t, tc.wantCalls, state.calls)
			require.Equal(t, tc.wantValue, state.value)
			if tc.wantNoCreates {
				require.Zero(t, creates)
			}
			require.Equal(t, secretstore.SecretStoreCensus{}, store.Census())
		})
	}
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
