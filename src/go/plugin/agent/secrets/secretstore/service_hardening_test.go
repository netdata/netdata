// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePublished struct {
	blockOnCtx       *atomic.Bool
	requireNonNilCtx *atomic.Bool
}

func (p *fakePublished) Resolve(ctx context.Context, req secretstore.ResolveRequest) (string, error) {
	if p.requireNonNilCtx != nil {
		p.requireNonNilCtx.Store(ctx != nil)
		if ctx == nil {
			return "", errors.New("nil context")
		}
	}
	if p.blockOnCtx != nil && p.blockOnCtx.Load() {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return req.Operand, nil
}

type fakeConfig struct {
	Auth map[string]any `json:"auth,omitempty" yaml:"auth,omitempty"`
}

type fakeStore struct {
	cfg              fakeConfig
	failInit         *atomic.Bool
	blockOnCtx       *atomic.Bool
	requireNonNilCtx *atomic.Bool
	published        secretstore.PublishedStore
}

func (s *fakeStore) Configuration() any { return &s.cfg }
func (s *fakeStore) Publish() secretstore.PublishedStore {
	return s.published
}

func (s *fakeStore) Init(context.Context) error {
	if s.failInit != nil && s.failInit.Load() {
		return errors.New("simulated validation error")
	}
	if len(s.cfg.Auth) == 0 {
		return errors.New("auth is required")
	}
	s.published = &fakePublished{
		blockOnCtx:       s.blockOnCtx,
		requireNonNilCtx: s.requireNonNilCtx,
	}
	return nil
}

type validateRaceStore struct {
	cfg             fakeConfig
	initCount       *atomic.Int32
	validateStarted chan struct{}
	validateRelease <-chan struct{}
	published       secretstore.PublishedStore
}

func (s *validateRaceStore) Configuration() any { return &s.cfg }
func (s *validateRaceStore) Publish() secretstore.PublishedStore {
	return s.published
}

func (s *validateRaceStore) Init(context.Context) error {
	if s.initCount != nil && s.initCount.Add(1) == 2 {
		close(s.validateStarted)
		<-s.validateRelease
	}
	if len(s.cfg.Auth) == 0 {
		return errors.New("auth is required")
	}
	s.published = &fakePublished{}
	return nil
}

func newFakeCreator(kind secretstore.StoreKind, failInit, blockOnCtx *atomic.Bool) secretstore.Creator {
	return newFakeCreatorWithCtxProbe(kind, failInit, blockOnCtx, nil)
}

func newFakeCreatorWithCtxProbe(kind secretstore.StoreKind, failInit, blockOnCtx, requireNonNilCtx *atomic.Bool) secretstore.Creator {
	schema := map[string]any{
		"jsonSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"auth": map[string]any{"type": "object"},
			},
			"required": []any{"auth"},
		},
		"uiSchema": map[string]any{},
	}
	bs, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}

	return secretstore.Creator{
		Kind:        kind,
		DisplayName: "Fake Provider",
		Schema:      string(bs),
		Create: func() secretstore.Store {
			return &fakeStore{
				failInit:         failInit,
				blockOnCtx:       blockOnCtx,
				requireNonNilCtx: requireNonNilCtx,
			}
		},
	}
}

func newValidateRaceCreator(kind secretstore.StoreKind, validateStarted chan struct{}, validateRelease <-chan struct{}) secretstore.Creator {
	schema := map[string]any{
		"jsonSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"auth": map[string]any{"type": "object"},
			},
			"required": []any{"auth"},
		},
		"uiSchema": map[string]any{},
	}
	bs, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}

	var initCount atomic.Int32
	return secretstore.Creator{
		Kind:        kind,
		DisplayName: "Fake Provider",
		Schema:      string(bs),
		Create: func() secretstore.Store {
			return &validateRaceStore{
				initCount:       &initCount,
				validateStarted: validateStarted,
				validateRelease: validateRelease,
			}
		},
	}
}

func newFakeStore(_ *testing.T, _ secretstore.Service, kind secretstore.StoreKind, cfg fakeConfig, name string) secretstore.Config {
	bs, err := json.Marshal(cfg)
	if err != nil {
		panic(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(bs, &payload); err != nil {
		panic(err)
	}
	out := secretstore.Config(payload)
	out.SetName(name)
	out.SetKind(kind)
	out.SetSource("dyncfg")
	out.SetSourceType("dyncfg")
	return out
}

func TestServiceStatusLifecycle(t *testing.T) {
	var failInit atomic.Bool
	svc := secretstore.NewService(newFakeCreator(secretstore.KindVault, &failInit, nil))

	store := newFakeStore(t, svc, secretstore.KindVault, fakeConfig{
		Auth: map[string]any{"mode": "token_env"},
	}, "vault_prod")

	err := svc.Add(context.Background(), store)
	require.NoError(t, err)

	failInit.Store(true)
	storeKey := secretstore.StoreKey(secretstore.KindVault, "vault_prod")
	err = svc.ValidateStored(context.Background(), storeKey)
	require.Error(t, err)

	status, ok := svc.GetStatus(storeKey)
	require.True(t, ok)
	require.NotNil(t, status.LastValidation)
	assert.False(t, status.LastValidation.OK)
	assert.Equal(t, "simulated validation error", status.LastErrorSummary)

	failInit.Store(false)
	err = svc.ValidateStored(context.Background(), storeKey)
	require.NoError(t, err)

	status, ok = svc.GetStatus(storeKey)
	require.True(t, ok)
	require.NotNil(t, status.LastValidation)
	assert.True(t, status.LastValidation.OK)
	assert.Empty(t, status.LastErrorSummary)
}

func TestServiceResolveHonorsCanceledContext(t *testing.T) {
	var blockOnCtx atomic.Bool
	blockOnCtx.Store(true)

	svc := secretstore.NewService(newFakeCreator(secretstore.KindVault, nil, &blockOnCtx))
	store := newFakeStore(t, svc, secretstore.KindVault, fakeConfig{
		Auth: map[string]any{"mode": "token_env"},
	}, "vault_prod")
	err := svc.Add(context.Background(), store)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = svc.Resolve(ctx, svc.Capture(), "vault:vault_prod:secret", "${store:vault:vault_prod:secret}")
	require.ErrorIs(t, err, context.Canceled)
}

func TestServiceResolve_NormalizesNilContext(t *testing.T) {
	var requireNonNilCtx atomic.Bool

	svc := secretstore.NewService(newFakeCreatorWithCtxProbe(secretstore.KindVault, nil, nil, &requireNonNilCtx))
	store := newFakeStore(t, svc, secretstore.KindVault, fakeConfig{
		Auth: map[string]any{"mode": "token_env"},
	}, "vault_prod")
	err := svc.Add(context.Background(), store)
	require.NoError(t, err)

	val, err := svc.Resolve(nil, svc.Capture(), "vault:vault_prod:secret/data/app#key", "${store:vault:vault_prod:secret/data/app#key}")
	require.NoError(t, err)
	assert.Equal(t, "secret/data/app#key", val)
	assert.True(t, requireNonNilCtx.Load())
}

func TestServiceConcurrentResolveAndMutation(t *testing.T) {
	svc := secretstore.NewService(newFakeCreator(secretstore.KindVault, nil, nil))
	baseCfg := fakeConfig{
		Auth: map[string]any{"mode": "token_env"},
	}

	err := svc.Add(context.Background(), newFakeStore(t, svc, secretstore.KindVault, baseCfg, "vault_prod"))
	require.NoError(t, err)

	var wg sync.WaitGroup
	errCh := make(chan error, 32)

	wg.Go(func() {
		for range 100 {
			snapshot := svc.Capture()
			val, err := svc.Resolve(context.Background(), snapshot, "vault:vault_prod:secret/data/app#key", "${store:vault:vault_prod:secret/data/app#key}")
			if err != nil {
				errCh <- err
				return
			}
			if val != "secret/data/app#key" {
				errCh <- errors.New("unexpected resolved value")
				return
			}
		}
	})

	wg.Go(func() {
		for i := range 100 {
			updateCfg := baseCfg
			if i%2 == 0 {
				updateCfg.Auth = map[string]any{
					"mode": "token_env",
					"tag":  "alt",
				}
			}
			if err := svc.Update(context.Background(), secretstore.StoreKey(secretstore.KindVault, "vault_prod"), newFakeStore(t, svc, secretstore.KindVault, updateCfg, "vault_prod")); err != nil {
				errCh <- err
				return
			}
		}
	})

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
}

func TestServiceValidateStored_RemovedDuringValidationReturnsNotFound(t *testing.T) {
	validateStarted := make(chan struct{})
	validateRelease := make(chan struct{})

	svc := secretstore.NewService(newValidateRaceCreator(secretstore.KindVault, validateStarted, validateRelease))
	store := newFakeStore(t, svc, secretstore.KindVault, fakeConfig{
		Auth: map[string]any{"mode": "token_env"},
	}, "vault_prod")
	require.NoError(t, svc.Add(context.Background(), store))

	storeKey := secretstore.StoreKey(secretstore.KindVault, "vault_prod")
	errCh := make(chan error, 1)

	go func() {
		errCh <- svc.ValidateStored(context.Background(), storeKey)
	}()

	<-validateStarted
	require.NoError(t, svc.Remove(storeKey))
	close(validateRelease)

	err := <-errCh
	require.Error(t, err)
	assert.ErrorIs(t, err, secretstore.ErrStoreNotFound)
}

func TestServiceValidateStored_UpdatedDuringValidationReturnsRetryWithoutOverwritingStatus(t *testing.T) {
	validateStarted := make(chan struct{})
	validateRelease := make(chan struct{})

	svc := secretstore.NewService(newValidateRaceCreator(secretstore.KindVault, validateStarted, validateRelease))
	store := newFakeStore(t, svc, secretstore.KindVault, fakeConfig{
		Auth: map[string]any{"mode": "token_env"},
	}, "vault_prod")
	require.NoError(t, svc.Add(context.Background(), store))

	storeKey := secretstore.StoreKey(secretstore.KindVault, "vault_prod")
	errCh := make(chan error, 1)

	go func() {
		errCh <- svc.ValidateStored(context.Background(), storeKey)
	}()

	<-validateStarted
	require.NoError(t, svc.Update(context.Background(), storeKey, newFakeStore(t, svc, secretstore.KindVault, fakeConfig{
		Auth: map[string]any{"mode": "token_env", "tag": "new"},
	}, "vault_prod")))
	close(validateRelease)

	err := <-errCh
	require.Error(t, err)
	assert.Contains(t, err.Error(), "changed during validation")

	status, ok := svc.GetStatus(storeKey)
	require.True(t, ok)
	assert.Nil(t, status.LastValidation)
	assert.Empty(t, status.LastErrorSummary)
}
