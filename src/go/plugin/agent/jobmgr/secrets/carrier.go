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

func (sgc *storeGenerationCarrier) Valid() bool {
	if sgc == nil {
		return false
	}
	sgc.mu.Lock()
	defer sgc.mu.Unlock()
	return sgc.permit.Valid() && !sgc.returned
}

func (sgc *storeGenerationCarrier) Activate() error {
	if sgc == nil {
		return errors.New(
			"jobmgr secrets: nil Store-generation carrier",
		)
	}
	sgc.mu.Lock()
	defer sgc.mu.Unlock()
	if sgc.externalReleased ||
		sgc.bytesReleased ||
		sgc.returned {
		return errors.New(
			"jobmgr secrets: Store-generation carrier cannot activate",
		)
	}
	return sgc.permit.ActivateExternal(
		lifecycle.LongLivedESecretStore,
	)
}

func (sgc *storeGenerationCarrier) Release() error {
	if sgc == nil {
		return errors.New(
			"jobmgr secrets: nil Store-generation carrier",
		)
	}
	sgc.mu.Lock()
	defer sgc.mu.Unlock()
	if sgc.returned {
		return errors.New(
			"jobmgr secrets: Store-generation carrier released twice",
		)
	}
	if !sgc.externalReleased {
		if err := sgc.permit.ReleaseExternal(
			lifecycle.LongLivedESecretStore,
		); err != nil {
			return err
		}
		sgc.externalReleased = true
	}
	if !sgc.bytesReleased {
		if err := sgc.permit.ReleaseBytes(); err != nil {
			return err
		}
		sgc.bytesReleased = true
	}
	if err := sgc.permit.Return(); err != nil {
		return err
	}
	sgc.returned = true
	return nil
}
