// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

func TestDyncfgConfigUserconfig_InvalidPayload_Returns400Only(t *testing.T) {
	tests := map[string]struct {
		contentType string
		payload     []byte
	}{
		"invalid json payload should stop after 400": {
			contentType: "application/json",
			payload:     []byte("{"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer

			mgr := New(Config{})
			mgr.modules = prepareMockRegistry()
			mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))))

			fn := dyncfg.NewFunction(functions.Function{
				UID:         "bad-userconfig",
				ContentType: tc.contentType,
				Payload:     tc.payload,
				Args: []string{
					mgr.dyncfgModID("success"),
					string(dyncfg.CommandUserconfig),
					"test",
				},
			})

			mgr.dyncfgCmdUserconfig(fn)

			out := buf.String()
			assert.Equal(t, 1, strings.Count(out, "FUNCTION_RESULT_BEGIN bad-userconfig"))
			assert.Contains(t, out, "\"status\":400")
			assert.NotContains(t, out, "application/yaml")
		})
	}
}
