// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
)

const maximumRetainedSecretEpochSamples = 8

type processSecretEpoch struct {
	generation  uint64
	store       *secretstore.SecretStore
	diagnostics jobmgr.DiagnosticObserver
}

func (e *processSecretEpoch) acquireScope(
	keys []string,
) (secretresolver.AtomicScope, error) {
	if e == nil || e.generation == 0 || e.store == nil {
		return nil, errors.New("jobmgr composition: invalid secret Store epoch")
	}
	scope, err := e.store.AcquireScope(keys)
	if err != nil {
		return nil, err
	}
	return &processOwnedAtomicScope{
		generation:  e.generation,
		diagnostics: e.diagnostics,
		scope:       scope,
	}, nil
}

func (e *processSecretEpoch) seal() error {
	if e == nil || e.generation == 0 || e.store == nil {
		return errors.New("jobmgr composition: invalid secret Store epoch")
	}
	return e.store.Seal()
}

func (e *processSecretEpoch) done() <-chan struct{} {
	if e == nil || e.store == nil {
		return nil
	}
	return e.store.Done()
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

func (es *processSecretEpochs) create(
	generation uint64,
) (*processSecretEpoch, error) {
	if es == nil || generation == 0 {
		return nil, errors.New("jobmgr composition: invalid secret Store epoch creation")
	}
	store, err := secretstore.NewSecretStore(es.resolver)
	if err != nil {
		return nil, err
	}
	epoch := &processSecretEpoch{
		generation:  generation,
		store:       store,
		diagnostics: es.diagnostics,
	}
	es.mu.Lock()
	if es.closing {
		es.mu.Unlock()
		_ = store.Seal()
		return nil, errors.New("jobmgr composition: secret Store authority is closing")
	}
	if _, exists := es.epochs[generation]; exists {
		es.mu.Unlock()
		_ = store.Seal()
		return nil, errors.New("jobmgr composition: duplicate secret Store epoch")
	}
	es.epochs[generation] = epoch
	es.mu.Unlock()
	go es.observeClose(epoch)
	return epoch, nil
}

func (es *processSecretEpochs) seal(epoch *processSecretEpoch) error {
	if es == nil || epoch == nil {
		return errors.New("jobmgr composition: invalid secret Store epoch seal")
	}
	es.mu.Lock()
	owned := es.epochs[epoch.generation] == epoch
	es.mu.Unlock()
	if !owned {
		return errors.New("jobmgr composition: secret Store epoch is not process-owned")
	}
	return epoch.seal()
}

func (es *processSecretEpochs) beginShutdown() {
	if es == nil {
		return
	}
	es.mu.Lock()
	es.closing = true
	snapshot := make([]*processSecretEpoch, 0, len(es.epochs))
	for _, epoch := range es.epochs {
		snapshot = append(snapshot, epoch)
	}
	es.mu.Unlock()
	for _, epoch := range snapshot {
		if err := epoch.seal(); err != nil {
			es.observeFailure(epoch.generation, "secret Store epoch seal failed", err)
		}
	}
}

func (es *processSecretEpochs) shutdown(ctx context.Context) error {
	if es == nil || ctx == nil {
		return errors.New("jobmgr composition: invalid secret Store authority shutdown")
	}
	es.beginShutdown()
	es.mu.Lock()
	snapshot := make([]*processSecretEpoch, 0, len(es.epochs))
	for _, epoch := range es.epochs {
		snapshot = append(snapshot, epoch)
	}
	es.mu.Unlock()
	sort.Slice(snapshot, func(left, right int) bool {
		return snapshot[left].generation < snapshot[right].generation
	})
	var result error
	for index, epoch := range snapshot {
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
			retained := make([]*processSecretEpoch, 0, len(snapshot)-index)
			for _, candidate := range snapshot[index:] {
				select {
				case <-candidate.done():
					result = errors.Join(result, candidate.store.Close(context.Background()))
				default:
					retained = append(retained, candidate)
				}
			}
			if len(retained) == 0 {
				return result
			}
			result = errors.Join(
				result,
				errors.New("jobmgr composition: retained secret Store epoch"),
				ctx.Err(),
			)
			es.observeRetained(retained, ctx.Err())
			return result
		}
	}
	return result
}

func (es *processSecretEpochs) observeRetained(
	epochs []*processSecretEpoch,
	err error,
) {
	if len(epochs) == 0 {
		return
	}
	jobmgr.ObserveDiagnostic(es.diagnostics, jobmgr.DiagnosticEvent{
		Level: jobmgr.DiagnosticError,
		Name:  "secret Store epochs retained at process shutdown",
		Count: len(epochs),
		Err:   err,
	})
	for _, epoch := range epochs[:min(len(epochs), maximumRetainedSecretEpochSamples)] {
		jobmgr.ObserveDiagnostic(es.diagnostics, jobmgr.DiagnosticEvent{
			Level:      jobmgr.DiagnosticWarning,
			Name:       "secret Store epoch retained at process shutdown",
			Generation: epoch.generation,
			Err:        err,
		})
	}
}

func (es *processSecretEpochs) observeClose(epoch *processSecretEpoch) {
	<-epoch.done()
	err := epoch.store.Close(context.Background())
	es.mu.Lock()
	if es.epochs[epoch.generation] == epoch {
		delete(es.epochs, epoch.generation)
	}
	es.mu.Unlock()
	if err != nil {
		es.observeFailure(epoch.generation, "secret Store epoch closed dirty", err)
	}
}

func (es *processSecretEpochs) observeFailure(
	generation uint64,
	name string,
	err error,
) {
	jobmgr.ObserveDiagnostic(es.diagnostics, jobmgr.DiagnosticEvent{
		Level:      jobmgr.DiagnosticError,
		Name:       name,
		Generation: generation,
		Err:        err,
	})
}
