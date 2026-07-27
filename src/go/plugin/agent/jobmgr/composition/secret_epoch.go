// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
)

type processSecretEpoch struct {
	generation  uint64
	store       *secretstore.SecretStore
	diagnostics jobmgr.DiagnosticObserver
}

func (epoch *processSecretEpoch) acquireScope(
	keys []string,
) (secretresolver.AtomicScope, error) {
	if epoch == nil || epoch.generation == 0 || epoch.store == nil {
		return nil, errors.New("jobmgr composition: invalid secret Store epoch")
	}
	scope, err := epoch.store.AcquireScope(keys)
	if err != nil {
		return nil, err
	}
	return &processOwnedAtomicScope{
		generation:  epoch.generation,
		diagnostics: epoch.diagnostics,
		scope:       scope,
	}, nil
}

func (epoch *processSecretEpoch) seal() error {
	if epoch == nil || epoch.generation == 0 || epoch.store == nil {
		return errors.New("jobmgr composition: invalid secret Store epoch")
	}
	return epoch.store.Seal()
}

func (epoch *processSecretEpoch) done() <-chan struct{} {
	if epoch == nil || epoch.store == nil {
		return nil
	}
	return epoch.store.Done()
}

type processSecretEpochs struct {
	mu sync.Mutex

	resolver    *secretresolver.AtomicResolver
	diagnostics jobmgr.DiagnosticObserver
	epochs      map[uint64]*processSecretEpoch
	closing     bool
}

func newProcessSecretEpochs(
	resolver *secretresolver.AtomicResolver,
	diagnostics jobmgr.DiagnosticObserver,
) (*processSecretEpochs, error) {
	if resolver == nil || diagnostics == nil {
		return nil, errors.New("jobmgr composition: invalid process secret Store authority")
	}
	return &processSecretEpochs{
		resolver:    resolver,
		diagnostics: diagnostics,
		epochs:      make(map[uint64]*processSecretEpoch),
	}, nil
}

func (epochs *processSecretEpochs) create(
	generation uint64,
) (*processSecretEpoch, error) {
	if epochs == nil || generation == 0 {
		return nil, errors.New("jobmgr composition: invalid secret Store epoch creation")
	}
	store, err := secretstore.NewSecretStore(epochs.resolver)
	if err != nil {
		return nil, err
	}
	epoch := &processSecretEpoch{
		generation:  generation,
		store:       store,
		diagnostics: epochs.diagnostics,
	}
	epochs.mu.Lock()
	if epochs.closing {
		epochs.mu.Unlock()
		_ = store.Seal()
		return nil, errors.New("jobmgr composition: secret Store authority is closing")
	}
	if _, exists := epochs.epochs[generation]; exists {
		epochs.mu.Unlock()
		_ = store.Seal()
		return nil, errors.New("jobmgr composition: duplicate secret Store epoch")
	}
	epochs.epochs[generation] = epoch
	epochs.mu.Unlock()
	go epochs.observeClose(epoch)
	return epoch, nil
}

func (epochs *processSecretEpochs) seal(epoch *processSecretEpoch) error {
	if epochs == nil || epoch == nil {
		return errors.New("jobmgr composition: invalid secret Store epoch seal")
	}
	epochs.mu.Lock()
	owned := epochs.epochs[epoch.generation] == epoch
	epochs.mu.Unlock()
	if !owned {
		return errors.New("jobmgr composition: secret Store epoch is not process-owned")
	}
	return epoch.seal()
}

func (epochs *processSecretEpochs) beginShutdown() {
	if epochs == nil {
		return
	}
	epochs.mu.Lock()
	epochs.closing = true
	snapshot := make([]*processSecretEpoch, 0, len(epochs.epochs))
	for _, epoch := range epochs.epochs {
		snapshot = append(snapshot, epoch)
	}
	epochs.mu.Unlock()
	for _, epoch := range snapshot {
		if err := epoch.seal(); err != nil {
			epochs.observeFailure(epoch.generation, "secret Store epoch seal failed", err)
		}
	}
}

func (epochs *processSecretEpochs) shutdown(ctx context.Context) error {
	if epochs == nil || ctx == nil {
		return errors.New("jobmgr composition: invalid secret Store authority shutdown")
	}
	epochs.beginShutdown()
	epochs.mu.Lock()
	snapshot := make([]*processSecretEpoch, 0, len(epochs.epochs))
	for _, epoch := range epochs.epochs {
		snapshot = append(snapshot, epoch)
	}
	epochs.mu.Unlock()
	var result error
	for _, epoch := range snapshot {
		select {
		case <-epoch.done():
			result = errors.Join(result, epoch.store.Close(context.Background()))
			continue
		default:
		}
		select {
		case <-epoch.done():
			result = errors.Join(result, epoch.store.Close(context.Background()))
		case <-ctx.Done():
			select {
			case <-epoch.done():
				result = errors.Join(result, epoch.store.Close(context.Background()))
				continue
			default:
			}
			result = errors.Join(
				result,
				errors.New("jobmgr composition: retained secret Store epoch"),
				ctx.Err(),
			)
			epochs.observeFailure(
				epoch.generation,
				"secret Store epoch retained at process shutdown",
				ctx.Err(),
			)
			return result
		}
	}
	return result
}

func (epochs *processSecretEpochs) observeClose(epoch *processSecretEpoch) {
	<-epoch.done()
	err := epoch.store.Close(context.Background())
	epochs.mu.Lock()
	if epochs.epochs[epoch.generation] == epoch {
		delete(epochs.epochs, epoch.generation)
	}
	epochs.mu.Unlock()
	if err != nil {
		epochs.observeFailure(epoch.generation, "secret Store epoch closed dirty", err)
	}
}

func (epochs *processSecretEpochs) observeFailure(
	generation uint64,
	name string,
	err error,
) {
	jobmgr.ObserveDiagnostic(epochs.diagnostics, jobmgr.DiagnosticEvent{
		Level:      jobmgr.DiagnosticError,
		Name:       name,
		Generation: generation,
		Err:        err,
	})
}
