// SPDX-License-Identifier: GPL-3.0-or-later

package program

// DimensionOrderingMode defines deterministic dimension ordering policy.
type DimensionOrderingMode string

const (
	// DimensionOrderingStaticThenDynamicLexical keeps declaration order for
	// static dimensions and lexical ordering for dynamic rendered dimensions.
	DimensionOrderingStaticThenDynamicLexical DimensionOrderingMode = "static_then_dynamic_lexical"
)

// OrderingPolicy captures deterministic ordering choices for one Program.
type OrderingPolicy struct {
	DimensionMode DimensionOrderingMode
}

// DefaultOrderingPolicy returns phase-1 baseline ordering contract.
func DefaultOrderingPolicy() OrderingPolicy {
	return OrderingPolicy{
		DimensionMode: DimensionOrderingStaticThenDynamicLexical,
	}
}
