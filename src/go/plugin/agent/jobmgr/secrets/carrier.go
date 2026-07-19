// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

type storeGenerationCarrier struct {
	mu sync.Mutex

	permit           lifecycle.LongLivedPermit
	externalReleased bool
	bytesReleased    bool
	returned         bool
}

func newStoreGenerationCarrier(
	permit lifecycle.LongLivedPermit,
	owner lifecycle.ResourceIdentity,
) (*storeGenerationCarrier, error) {
	if !permit.Valid() ||
		permit.Owner() != owner ||
		permit.Class() != lifecycle.LongLivedSecretStore {
		return nil, errors.New(
			"jobmgr secrets: invalid Store-generation carrier",
		)
	}
	return &storeGenerationCarrier{permit: permit}, nil
}

func (carrier *storeGenerationCarrier) Valid() bool {
	if carrier == nil {
		return false
	}
	carrier.mu.Lock()
	defer carrier.mu.Unlock()
	return carrier.permit.Valid() && !carrier.returned
}

func (carrier *storeGenerationCarrier) ResourceCapacityBytes() int64 {
	if carrier == nil {
		return 0
	}
	carrier.mu.Lock()
	defer carrier.mu.Unlock()
	capacity := carrier.permit.CapacityBytes()
	if capacity <= lifecycle.TaskChildExecutionBytes {
		return 0
	}
	return capacity - lifecycle.TaskChildExecutionBytes
}

func (carrier *storeGenerationCarrier) Activate() error {
	if carrier == nil {
		return errors.New(
			"jobmgr secrets: nil Store-generation carrier",
		)
	}
	carrier.mu.Lock()
	defer carrier.mu.Unlock()
	if carrier.externalReleased ||
		carrier.bytesReleased ||
		carrier.returned {
		return errors.New(
			"jobmgr secrets: Store-generation carrier cannot activate",
		)
	}
	return carrier.permit.ActivateExternal(
		lifecycle.LongLivedESecretStore,
	)
}

func (carrier *storeGenerationCarrier) Release() error {
	if carrier == nil {
		return errors.New(
			"jobmgr secrets: nil Store-generation carrier",
		)
	}
	carrier.mu.Lock()
	defer carrier.mu.Unlock()
	if carrier.returned {
		return errors.New(
			"jobmgr secrets: Store-generation carrier released twice",
		)
	}
	if !carrier.externalReleased {
		if err := carrier.permit.ReleaseExternal(
			lifecycle.LongLivedESecretStore,
		); err != nil {
			return err
		}
		carrier.externalReleased = true
	}
	if !carrier.bytesReleased {
		if err := carrier.permit.ReleaseBytes(); err != nil {
			return err
		}
		carrier.bytesReleased = true
	}
	if err := carrier.permit.Return(); err != nil {
		return err
	}
	carrier.returned = true
	return nil
}
