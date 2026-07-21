// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestCancelledStoreCommitWithoutDependentsIsSafeUnchanged(
	t *testing.T,
) {
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	store, err := secretstore.NewSecretStore(resolver)
	require.NoError(t, err)
	catalog, err := secretstore.NewCreatorCatalog(
		[]secretstore.Creator{{
			Kind:   secretstore.KindVault,
			Schema: `{}`,
			Create: func() secretstore.Store {
				return &transactionTestStore{}
			},
		}},
	)
	require.NoError(t, err)
	carrier := &transactionTestCarrier{}
	mutation, err := store.PrepareMutation(
		t.Context(),
		catalog,
		carrier,
		secretstore.Config{
			"name":            "main",
			"kind":            string(secretstore.KindVault),
			"value":           "value",
			"__source__":      confgroup.TypeDyncfg,
			"__source_type__": confgroup.TypeDyncfg,
		},
		0,
	)
	require.NoError(t, err)
	transaction, err := newPreparedSecretTransaction(
		preparedSecretSpec{
			scope: lifecycle.ResourceTransactionScope{
				ID: "secretstore:vault:main",
			},
			store:      store,
			storeKey:   "vault:main",
			mutation:   mutation,
			result:     mustSecretMessage(200, ""),
			cleanup:    func() error { return nil },
			controller: &Controller{},
		},
	)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, applyErr := transaction.Apply(ctx)
	require.NoError(t, applyErr)

	require.True(t, carrier.released)

	census := store.Census()
	require.EqualValues(t, secretstore.SecretStoreCensus{}, census)

	require.NoError(t, store.Close(t.Context()))
}

func TestSecretTransactionAlwaysAbortsUncommittedMutation(t *testing.T) {
	rollbackErr := errors.New("rollback context unavailable")
	stopErr := errors.New("dependent stop failed")
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	store, err := secretstore.NewSecretStore(resolver)
	require.NoError(t, err)
	catalog, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &transactionTestStore{}
		},
	}})
	require.NoError(t, err)
	carrier := &transactionTestCarrier{}
	mutation, err := store.PrepareMutation(
		t.Context(),
		catalog,
		carrier,
		secretstore.Config{
			"name":            "main",
			"kind":            string(secretstore.KindVault),
			"value":           "value",
			"__source__":      confgroup.TypeDyncfg,
			"__source_type__": confgroup.TypeDyncfg,
		},
		0,
	)
	require.NoError(t, err)
	dependencies := NewSecretDependencyIndex()
	dependencies.jobs["module_two"] = jobDependency{
		display: "module:job", running: true,
		storeKeys: []string{"vault:main"},
	}
	dependencies.byStore["vault:main"] = map[string]struct{}{
		"module_two": {},
	}
	restarts, err := NewSecretRestartCommand(
		1,
		dependencies,
		restartTestJobs{stopError: stopErr},
	)
	require.NoError(t, err)
	transaction, err := newPreparedSecretTransaction(preparedSecretSpec{
		scope: lifecycle.ResourceTransactionScope{
			ID: "secretstore:vault:main",
		},
		store: store, storeKey: "vault:main", mutation: mutation,
		restarts:   restarts,
		result:     mustSecretMessage(200, ""),
		cleanup:    func() error { return nil },
		controller: &Controller{},
	})
	require.NoError(t, err)
	commands := &restartTestCommandScope{rollbackContextErr: rollbackErr}

	_, applyErr := transaction.ApplyComposite(t.Context(), commands)

	require.NoError(t, applyErr)
	require.True(t, carrier.released)
	require.Zero(t, store.Census().Preparations)
	require.NoError(t, store.Close(t.Context()))
}

type transactionTestCarrier struct {
	activated bool
	released  bool
}

func (ttc *transactionTestCarrier) Valid() bool {
	return ttc != nil && !ttc.released
}

func (ttc *transactionTestCarrier) Activate() error {
	if !ttc.Valid() || ttc.activated {
		return errors.New("invalid activation")
	}
	ttc.activated = true
	return nil
}

func (ttc *transactionTestCarrier) Release() error {
	if !ttc.Valid() {
		return errors.New("invalid release")
	}
	ttc.released = true
	return nil
}

type transactionTestStore struct {
	config struct {
		Value string `yaml:"value"`
	}
}

func (tts *transactionTestStore) Configuration() any {
	return &tts.config
}

func (*transactionTestStore) Init(context.Context) error {
	return nil
}

func (tts *transactionTestStore) Publish() secretstore.PublishedStore {
	return transactionTestPublished(tts.config.Value)
}

type transactionTestPublished string

func (ttp transactionTestPublished) Resolve(
	context.Context,
	secretstore.ResolveRequest,
) (string, error) {
	return string(ttp), nil
}
