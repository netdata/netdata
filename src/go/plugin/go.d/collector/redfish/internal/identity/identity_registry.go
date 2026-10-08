// SPDX-License-Identifier: GPL-3.0-or-later

package identity

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"sync"
)

var ErrIntegrity = errors.New("Redfish identity integrity failure")

type Binding struct {
	Domain   string
	Key      string
	Preimage string
}

type Registry struct {
	mu       sync.Mutex
	bindings map[string][sha256.Size]byte
}

func (r *Registry) Register(values []Binding) error {
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
			return fmt.Errorf("%w: incomplete identity binding", ErrIntegrity)
		}
		key := value.Domain + "\x00" + value.Key
		digest := sha256.Sum256([]byte(value.Domain + "\x00" + value.Preimage))
		if previous, exists := r.bindings[key]; exists && previous != digest {
			return fmt.Errorf("%w: %s key collision", ErrIntegrity, value.Domain)
		}
		if previous, exists := pending[key]; exists && previous != digest {
			return fmt.Errorf("%w: %s key collision", ErrIntegrity, value.Domain)
		}
		pending[key] = digest
	}
	maps.Copy(r.bindings, pending)
	return nil
}

func IsIntegrityError(err error) bool {
	return errors.Is(err, ErrIntegrity)
}
