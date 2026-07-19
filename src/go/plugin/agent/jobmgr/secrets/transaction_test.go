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
)

func TestCancelledStoreCommitWithoutDependentsIsSafeUnchanged(
	t *testing.T,
) {
	resolver, err := secretresolver.NewAtomicResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	store, err := secretstore.NewSecretStore(resolver)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := secretstore.NewCreatorCatalog(
		[]secretstore.Creator{{
			Kind:        secretstore.KindVault,
			DisplayName: "Vault",
			Schema:      `{}`,
			Create: func() secretstore.Store {
				return &transactionTestStore{}
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := newPreparedSecretTransaction(
		preparedSecretSpec{
			scope: lifecycle.ResourceTransactionScope{
				ID: "secretstore:vault:main",
			},
			store:      store,
			storeKey:   "vault:main",
			mutation:   &mutation,
			result:     mustSecretMessage(200, ""),
			cleanup:    func() error { return nil },
			controller: &Controller{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := transaction.Apply(ctx); err != nil {
		t.Fatalf("safe cancelled mutation dirtied transaction: %v", err)
	}
	if !carrier.released {
		t.Fatal("safe cancelled mutation retained its carrier")
	}
	if census := store.Census(); census !=
		(secretstore.SecretStoreCensus{}) {
		t.Fatalf("safe cancelled mutation retained ownership: %+v", census)
	}
	if err := store.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

type transactionTestCarrier struct {
	activated bool
	released  bool
}

func (carrier *transactionTestCarrier) Valid() bool {
	return carrier != nil && !carrier.released
}

func (carrier *transactionTestCarrier) Activate() error {
	if !carrier.Valid() || carrier.activated {
		return errors.New("invalid activation")
	}
	carrier.activated = true
	return nil
}

func (carrier *transactionTestCarrier) Release() error {
	if !carrier.Valid() {
		return errors.New("invalid release")
	}
	carrier.released = true
	return nil
}

type transactionTestStore struct {
	config struct {
		Value string `yaml:"value"`
	}
}

func (store *transactionTestStore) Configuration() any {
	return &store.config
}

func (*transactionTestStore) Init(context.Context) error {
	return nil
}

func (store *transactionTestStore) Publish() secretstore.PublishedStore {
	return transactionTestPublished(store.config.Value)
}

type transactionTestPublished string

func (published transactionTestPublished) Resolve(
	context.Context,
	secretstore.ResolveRequest,
) (string, error) {
	return string(published), nil
}
