// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestOutOfTableOIDs(t *testing.T) {
	for name, tc := range map[string]struct {
		input string
		want  []string
	}{
		"columns under the table": {
			input: `metrics: [{table: {OID: 1.2.3, name: t}, symbols: [{OID: 1.2.3.1.2, name: v}],
				metric_tags: [{tag: n, symbol: {OID: 1.2.3.1.3, name: n}}]}]`,
		},
		"table OID names the entry": {
			input: `metrics: [{table: {OID: 1.2.3.1, name: t}, symbols: [{OID: 1.2.3.1.2, name: v}], metric_tags: [{index: 1}]}]`,
		},
		"anchor column with an index prefix": {
			input: `topology: [{kind: ip_if_index, table: {OID: 1.2.3.1.3.1.4, name: t}, symbols: [{OID: 1.2.3.1.3, name: v}],
				metric_tags: [{tag: n, symbol: {OID: 1.2.3.1.3, name: n}}]}]`,
		},
		"parent node spanning sibling tables": {
			input: `metrics: [{table: {OID: 1.2.3, name: t}, symbols: [{OID: 1.2.3.3.1.2, name: v}],
				metric_tags: [{tag: n, symbol: {OID: 1.2.3.2.1.3, name: n}}]}]`,
		},
		"symbol from another table": {
			input: `metrics: [{table: {OID: 1.2.3, name: t}, symbols: [{OID: 1.2.3.1.2, name: ok}, {OID: 1.2.4.1.2, name: v}],
				metric_tags: [{index: 1}]}]`,
			want: []string{"metrics[0].symbols[1]: OID 1.2.4.1.2 is outside table.OID 1.2.3 and is never collected"},
		},
		"same-table tag from another table": {
			input: `metrics: [{table: {OID: 1.2.3, name: t}, symbols: [{OID: 1.2.3.1.2, name: v}],
				metric_tags: [{tag: n, symbol: {OID: 1.2.4.1.3, name: n}}, {tag: m, table: t, symbol: {OID: 1.2.5.1.1, name: m}}]}]`,
			want: []string{
				"metrics[0].metric_tags[0]: OID 1.2.4.1.3 is outside table.OID 1.2.3 and is never collected",
				"metrics[0].metric_tags[1]: OID 1.2.5.1.1 is outside table.OID 1.2.3 and is never collected",
			},
		},
		"legacy tag OID": {
			input: `metrics: [{table: {OID: 1.2.3, name: t}, symbols: [{OID: 1.2.3.1.2, name: v}],
				metric_tags: [{tag: n, OID: 1.2.4.1.3, symbol: n}]}]`,
			want: []string{"metrics[0].metric_tags[0]: OID 1.2.4.1.3 is outside table.OID 1.2.3 and is never collected"},
		},
		"leading dot that drops an arc": {
			input: `metrics: [{table: {OID: 1.2.3, name: t}, symbols: [{OID: .2.3.1.2, name: v}], metric_tags: [{index: 1}]}]`,
			want:  []string{"metrics[0].symbols[0]: OID .2.3.1.2 is outside table.OID 1.2.3 and is never collected"},
		},
		"cross-table tag is not checked": {
			input: `metrics: [{table: {OID: 1.2.3, name: t}, symbols: [{OID: 1.2.3.1.2, name: v}],
				metric_tags: [{tag: n, table: other, symbol: {OID: 1.2.4.1.3, name: n}}]}]`,
		},
		"topology symbol from another table": {
			input: `topology: [{kind: if_name, table: {OID: 1.2.3, name: t}, symbols: [{OID: 1.2.4.1.2, name: v}]}]`,
			want:  []string{"topology[0].symbols[0]: OID 1.2.4.1.2 is outside table.OID 1.2.3 and is never collected"},
		},
		"scalar rows are not checked": {
			input: `metrics: [{symbol: {OID: 1.2.4.0, name: v}}]`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var p ProfileDefinition
			require.NoError(t, yaml.Unmarshal([]byte(tc.input), &p))
			require.NoError(t, ValidateEnrichProfile(&p))

			assert.Equal(t, tc.want, OutOfTableOIDs(&p))
		})
	}
}
