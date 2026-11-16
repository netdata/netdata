// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"fmt"
	"strings"
)

const maxCollectionDiagnostics = 256

// Diagnostics preserves a cycle's deduplication and omission accounting across
// acquisition and projection. After Acquire, the result's caller owns it exclusively.
type Diagnostics struct {
	values  []string
	seen    map[string]struct{}
	omitted int
}

func (a *Diagnostics) Add(value string) {
	value = BoundDiagnostic(strings.TrimSpace(value))
	if value == "" {
		return
	}
	if _, exists := a.seen[value]; exists {
		return
	}
	if len(a.values) >= maxCollectionDiagnostics {
		a.omitted++
		return
	}
	if a.seen == nil {
		a.seen = make(map[string]struct{})
	}
	a.seen[value] = struct{}{}
	a.values = append(a.values, value)
}

func (a *Diagnostics) Values() []string {
	result := append([]string(nil), a.values...)
	if a.omitted > 0 {
		result = append(result, fmt.Sprintf(
			"%d additional Redfish collection diagnostics were omitted by the fixed internal bound",
			a.omitted,
		))
	}
	return result
}

// BoundDiagnostic limits a single remote diagnostic before collection or logging.
func BoundDiagnostic(value string) string {
	const max = 1024
	if len(value) <= max {
		return value
	}
	return value[:max]
}
