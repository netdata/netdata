// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestSecretStoreFailureMessage(t *testing.T) {
	privateErr := errors.New("private provider failure")
	tests := map[string]struct {
		err             error
		want            string
		wantNotContains []string
	}{
		"unmarked error stays generic": {
			err:             privateErr,
			want:            msgSecretStoreTestFailed,
			wantNotContains: []string{privateErr.Error()},
		},
		"marked static detail is appended": {
			err: dyncfg.NewPublicError(
				"the configured provider endpoint is unavailable",
				privateErr,
			),
			want: "Secretstore operational test failed: the configured provider endpoint is unavailable",
			wantNotContains: []string{
				privateErr.Error(),
			},
		},
		"wrapped marked detail is appended": {
			err: fmt.Errorf(
				"private wrapper: %w",
				dyncfg.NewPublicError(
					"the configured provider credential is unavailable",
					privateErr,
				),
			),
			want: "Secretstore operational test failed: the configured provider credential is unavailable",
			wantNotContains: []string{
				"private wrapper",
				privateErr.Error(),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := secretFailureMessage(msgSecretStoreTestFailed, tc.err)

			require.Equal(t, tc.want, got)
			require.LessOrEqual(t, len(got), maximumSecretJobSummaryBytes)
			for _, value := range tc.wantNotContains {
				require.NotContains(t, got, value)
			}
		})
	}

	got := secretFailureMessage(
		msgSecretStoreTestFailed,
		dyncfg.NewPublicError(strings.Repeat("é", maximumSecretJobSummaryBytes), privateErr),
	)
	require.LessOrEqual(t, len(got), maximumSecretJobSummaryBytes)
	require.True(t, strings.HasSuffix(got, "... [truncated]"))
	require.True(t, strings.ToValidUTF8(got, "") == got)
}

func TestStoreOperationRunsOptionalOperationalTest(t *testing.T) {
	testErr := errors.New("provider unavailable")
	state := &storeTestCapabilityState{err: testErr}
	operations, attempts := newStoreTestCapabilityOperations(t, func() secretstore.Store {
		return &storeTestCapableProvider{state: state}
	})
	stage, err := operations.prepare(storeOperationSpec{
		target: secretTarget{
			command: dyncfg.CommandTest,
			key:     secretstore.StoreKey(secretstore.KindVault, "main"),
			kind:    secretstore.KindVault,
			name:    "main",
		},
		config:       storeTestCapabilityConfig(),
		mode:         storeOperationValidation,
		testIdentity: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		stage.Release()
		shutdownStoreTestCapabilityAttempts(t, attempts)
	})
	stage.Start()
	select {
	case <-stage.Ready():
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "Store operational test did not settle")
	}

	result, err := stage.take()
	require.NoError(t, err)
	require.True(t, result.operational)
	require.ErrorIs(t, result.err, testErr)
	require.Equal(t, 1, state.calls)
}

func TestSecretStoreTestResponseClassification(t *testing.T) {
	testErr := errors.New("provider unavailable")
	tests := map[string]struct {
		result storeOperationResult
		status int
	}{
		"operational failure is unprocessable": {
			result: storeOperationResult{
				config:      storeTestCapabilityConfig(),
				err:         testErr,
				operational: true,
			},
			status: 422,
		},
		"configuration failure is a bad request": {
			result: storeOperationResult{
				config: storeTestCapabilityConfig(),
				err:    testErr,
			},
			status: 400,
		},
		"unsupported operational test is a validation-only success": {
			result: storeOperationResult{
				config:         storeTestCapabilityConfig(),
				validationOnly: true,
			},
			status: 200,
		},
		"supported operational test preserves the impact-preview response": {
			result: storeOperationResult{
				config:         storeTestCapabilityConfig(),
				validationOnly: true,
				operational:    true,
			},
			status: 202,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			controller, target := newStoreTestCapabilityController(t)
			stage := (&StoreOperations{}).immediate(test.result)
			stage.Start()
			select {
			case <-stage.Ready():
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "immediate Store test did not settle")
			}

			prepared, err := controller.prepareTest(
				lifecycle.ResourceTransactionScope{ID: "secretstore:vault:main"},
				nil,
				target,
				stage,
			)
			require.NoError(t, err)
			applied, err := prepared.Apply(t.Context())
			require.NoError(t, err)
			require.Equal(t, test.status, applied.ResultStatus())
			stage.Release()
		})
	}
}

func TestSecretStoreTestQuarantineIsUnavailable(t *testing.T) {
	state := &storeTestCapabilityState{panicOnTest: true}
	operations, attempts := newStoreTestCapabilityOperations(t, func() secretstore.Store {
		return &storeTestCapableProvider{state: state}
	})
	defer shutdownStoreTestCapabilityAttempts(t, attempts)
	controller, target := newStoreTestCapabilityController(t)

	for range 2 {
		stage, err := operations.prepare(storeOperationSpec{
			target:       target,
			config:       storeTestCapabilityConfig(),
			mode:         storeOperationValidation,
			testIdentity: true,
		})
		require.NoError(t, err)
		stage.Start()
		select {
		case <-stage.Ready():
		case <-time.After(time.Second):
			require.FailNow(t, "test failed", "panicking Store test did not settle")
		}

		prepared, err := controller.prepareTest(
			lifecycle.ResourceTransactionScope{ID: "secretstore:vault:main"},
			nil,
			target,
			stage,
		)
		require.NoError(t, err)
		applied, err := prepared.Apply(t.Context())
		require.NoError(t, err)
		require.Equal(t, 503, applied.ResultStatus())
		stage.Release()
	}

	require.Equal(t, 1, state.calls)
	require.Equal(t, containment.Census{Quarantined: 1}, attempts.Census())
}

func newStoreTestCapabilityOperations(
	t *testing.T,
	create func() secretstore.Store,
) (*StoreOperations, *containment.Authority) {
	t.Helper()
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	store, err := secretstore.NewSecretStore(resolver)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close(context.Background()))
	})
	catalog, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: create,
	}})
	require.NoError(t, err)
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	operations, err := NewStoreOperations(StoreOperationsConfig{
		Epoch:    1,
		Attempts: attempts,
		Store:    store,
		Creators: catalog,
	})
	require.NoError(t, err)
	return operations, attempts
}

func shutdownStoreTestCapabilityAttempts(
	t *testing.T,
	attempts *containment.Authority,
) {
	t.Helper()
	attempts.BeginShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, attempts.Shutdown(ctx))
}

func newStoreTestCapabilityController(
	t *testing.T,
) (*Controller, secretTarget) {
	t.Helper()
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	store, err := secretstore.NewSecretStore(resolver)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close(context.Background()))
	})
	config := storeTestCapabilityConfig()
	key := config.ExposedKey()
	return &Controller{
		store:        store,
		dependencies: NewSecretDependencyIndex(),
		entries: map[string]secretEntry{
			key: {
				config: config,
				status: dyncfg.StatusRunning,
			},
		},
	}, secretTarget{
		command: dyncfg.CommandTest,
		key:     key,
		kind:    secretstore.KindVault,
		name:    "main",
	}
}

func storeTestCapabilityConfig() secretstore.Config {
	return secretstore.Config{
		"name":            "main",
		"kind":            string(secretstore.KindVault),
		"value":           "configured",
		"__source__":      confgroup.TypeDyncfg,
		"__source_type__": confgroup.TypeDyncfg,
	}
}

type storeTestCapabilityState struct {
	calls       int
	err         error
	panicOnTest bool
}

type storeTestCapableProvider struct {
	state  *storeTestCapabilityState
	config struct {
		Value string `yaml:"value"`
	}
}

func (s *storeTestCapableProvider) Configuration() any {
	return &s.config
}

func (s *storeTestCapableProvider) Init(context.Context) error {
	return nil
}

func (s *storeTestCapableProvider) Publish() secretstore.PublishedStore {
	return transactionTestPublished(s.config.Value)
}

func (s *storeTestCapableProvider) Test(context.Context) error {
	s.state.calls++
	if s.state.panicOnTest {
		panic("provider panic")
	}
	return s.state.err
}
