// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerationWrapRetiresSecretAuthorityFreelistHeads(t *testing.T) {
	tests := map[string]func(*testing.T){
		"preparation": func(t *testing.T) {
			store := &SecretStore{
				preparations: []*preparationSlot{
					{generation: math.MaxUint64, freeNext: 2},
					{generation: 7},
				},
				freePreparation: 1,
			}

			index, slot, err := store.allocatePreparation()

			require.NoError(t, err)
			require.Equal(t, uint32(1), index)
			require.Equal(t, uint64(7), slot.generation)
		},
		"scope": func(t *testing.T) {
			store := &SecretStore{
				scopes: []*scopeSlot{
					{generation: math.MaxUint64, freeNext: 2},
					{generation: 7},
				},
				freeScope: 1,
			}

			index, slot, err := store.allocateScope()

			require.NoError(t, err)
			require.Equal(t, uint32(1), index)
			require.Equal(t, uint64(7), slot.generation)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
