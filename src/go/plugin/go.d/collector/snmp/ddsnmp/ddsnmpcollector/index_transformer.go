// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// IndexTransformer handles index manipulation operations
type IndexTransformer struct{}

// NewIndexTransformer creates a new IndexTransformer
func NewIndexTransformer() *IndexTransformer {
	return &IndexTransformer{}
}

// ExtractPosition extracts a specific position from an index
// Position uses 1-based indexing as per the profile format
// Example: index "7.8.9", position 2 → "8"
func (t *IndexTransformer) ExtractPosition(index string, position uint) (string, bool) {
	if position == 0 {
		return "", false
	}

	var n uint
	for {
		n++
		i := strings.IndexByte(index, '.')
		if i == -1 {
			break
		}
		if n == position {
			return index[:i], true
		}
		index = index[i+1:]
	}

	return index, n == position && index != ""
}

// ApplyTransform applies index transformation rules to extract a subset of the index
// Example: index "1.6.0.36.155.53.3.246", transform [{start: 1, end: 7}] → "6.0.36.155.53.3.246"
func (t *IndexTransformer) ApplyTransform(index string, transforms []ddprofiledefinition.MetricIndexTransform) string {
	if len(transforms) == 0 {
		return index
	}

	parts := strings.Split(index, ".")
	var result []string

	for _, transform := range transforms {
		extracted := t.extractRange(parts, transform.Start, transform.End)
		result = append(result, extracted...)
	}

	return strings.Join(result, ".")
}

// extractRange extracts a range of parts from the index
func (t *IndexTransformer) extractRange(parts []string, start, end uint) []string {
	// Validate bounds
	if int(start) >= len(parts) || end < start || int(end) >= len(parts) {
		return nil
	}

	// Extract the range (inclusive)
	return parts[start : end+1]
}

// GetIndexParts splits an index into its component parts
func (t *IndexTransformer) GetIndexParts(index string) []string {
	return strings.Split(index, ".")
}

// JoinIndexParts joins index parts back into a dotted string
func (t *IndexTransformer) JoinIndexParts(parts []string) string {
	return strings.Join(parts, ".")
}

// ValidateTransform checks if a transform is valid for a given index
func (t *IndexTransformer) ValidateTransform(index string, transform ddprofiledefinition.MetricIndexTransform) error {
	parts := t.GetIndexParts(index)

	if int(transform.Start) >= len(parts) {
		return fmt.Errorf("start position %d is out of bounds for index with %d parts", transform.Start, len(parts))
	}

	if transform.End < transform.Start {
		return fmt.Errorf("end position %d is before start position %d", transform.End, transform.Start)
	}

	if int(transform.End) >= len(parts) {
		return fmt.Errorf("end position %d is out of bounds for index with %d parts", transform.End, len(parts))
	}

	return nil
}

// Global instance for backward compatibility
var indexTransformer = NewIndexTransformer()

// getIndexPosition wraps ExtractPosition for backward compatibility
func getIndexPosition(index string, position uint) (string, bool) {
	return indexTransformer.ExtractPosition(index, position)
}

// applyIndexTransform wraps ApplyTransform for backward compatibility
func applyIndexTransform(index string, transforms []ddprofiledefinition.MetricIndexTransform) string {
	return indexTransformer.ApplyTransform(index, transforms)
}
