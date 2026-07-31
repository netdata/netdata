// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"sync"
)

const maxIdentityBindings = maxGraphResources * 32

var errIdentityIntegrity = errors.New("Redfish identity integrity failure")

type identityBinding struct {
	Domain   string
	Key      string
	Preimage string
}

type identityRegistry struct {
	mu       sync.Mutex
	bindings map[string][sha256.Size]byte
}

func (r *identityRegistry) register(values []identityBinding) error {
	if len(values) == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bindings == nil {
		r.bindings = make(map[string][sha256.Size]byte)
	}
	pending := make(map[string][sha256.Size]byte, len(values))
	for _, value := range values {
		if value.Domain == "" || value.Key == "" || value.Preimage == "" {
			return fmt.Errorf("%w: incomplete identity binding", errIdentityIntegrity)
		}
		key := value.Domain + "\x00" + value.Key
		digest := sha256.Sum256([]byte(value.Domain + "\x00" + value.Preimage))
		if previous, exists := r.bindings[key]; exists && previous != digest {
			return fmt.Errorf("%w: %s key collision", errIdentityIntegrity, value.Domain)
		}
		if previous, exists := pending[key]; exists && previous != digest {
			return fmt.Errorf("%w: %s key collision", errIdentityIntegrity, value.Domain)
		}
		pending[key] = digest
	}
	additional := 0
	for key := range pending {
		if _, exists := r.bindings[key]; !exists {
			additional++
		}
	}
	if len(r.bindings)+additional > maxIdentityBindings {
		return fmt.Errorf("%w: identity registry exceeds the internal safety limit", errIdentityIntegrity)
	}
	maps.Copy(r.bindings, pending)
	return nil
}

func identityIntegrityError(err error) bool {
	return errors.Is(err, errIdentityIntegrity)
}
