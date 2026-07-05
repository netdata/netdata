// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A "null" (or whitespace-only) document unmarshals to a NIL map with no
// error on both content paths; downstream metadata writes into that map
// would panic on the manager loop, so the parse layer must reject it.
func TestConfigFromPayload_RejectsNullDocuments(t *testing.T) {
	tests := map[string]struct {
		payload     []byte
		contentType string
		wantErr     bool
	}{
		"json object": {payload: []byte(`{"option_str":"x"}`), contentType: "application/json"},
		"yaml object": {payload: []byte("option_str: x")},
		// JSON null is materialized as an empty object by Clone's yaml
		// round-trip (a nil map marshals as {}) - pre-existing behavior,
		// deliberately unchanged: only the YAML path can return nil.
		"json null":  {payload: []byte("null"), contentType: "application/json"},
		"yaml null":  {payload: []byte("null"), wantErr: true},
		"yaml empty": {payload: []byte("   "), wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fn := dyncfg.NewFunction(functions.Function{
				UID:         "u",
				Args:        []string{"test:collector:gated", "add", "j"},
				Payload:     tc.payload,
				ContentType: tc.contentType,
			})

			cfg, err := configFromPayload(fn)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, cfg)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, cfg)
		})
	}
}
