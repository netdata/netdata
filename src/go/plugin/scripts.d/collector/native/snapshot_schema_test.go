// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"encoding/json"
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
