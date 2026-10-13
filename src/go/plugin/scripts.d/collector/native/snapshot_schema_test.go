// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func TestSnapshotSchema(t *testing.T) {
	schema, err := jsonschema.NewCompiler().Compile("snapshot_schema.json")
	require.NoError(t, err)
	for _, group := range []struct {
		cases map[string]string
		valid bool
	}{{validSnapshots, true}, {invalidSnapshots, false}} {
		for name, text := range group.cases {
			t.Run(name, func(t *testing.T) {
				var document any
				require.NoError(t, json.Unmarshal([]byte(text), &document))
				err := schema.Validate(document)
				_, runtimeErr := decodeSnapshot([]byte(text))
				if group.valid {
					require.NoError(t, err)
					require.NoError(t, runtimeErr)
				} else {
					require.Error(t, err)
					require.Error(t, runtimeErr)
				}
			})
		}
	}
}

func TestSnapshotStateNames(t *testing.T) {
	schema, err := jsonschema.NewCompiler().Compile("snapshot_schema.json")
	require.NoError(t, err)
	for _, tc := range []struct {
		name  string
		valid bool
	}{
		{"in progress", true},
		{`λ "ready"`, true},
		{"path/up", true},
		{"", false},
		{" up", false},
		{"up ", false},
		{"\tup", false},
		{"up\t", false},
		{"\vup", false},
		{"\u00a0up", false},
		{"up\u00a0", false},
		{"\u3000up", false},
		{"up\u3000", false},
		{"'", false},
		{"u'p", false},
		{`path\up`, false},
		{"u\np", false},
		{"u\rp", false},
		{"u\x00p", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(tc.name)
			require.NoError(t, err)
			frame := metricFrame(strings.ReplaceAll(
				`{"name":"state","type":"stateset","states":[STATE],"samples":[{"active":[STATE]}]}`,
				"STATE", string(encoded)))
			var document any
			require.NoError(t, json.Unmarshal([]byte(frame), &document))
			_, runtimeErr := decodeSnapshot([]byte(frame))
			if tc.valid {
				require.NoError(t, schema.Validate(document))
				require.NoError(t, runtimeErr)
			} else {
				require.Error(t, schema.Validate(document))
				require.Error(t, runtimeErr)
			}
		})
	}
}
