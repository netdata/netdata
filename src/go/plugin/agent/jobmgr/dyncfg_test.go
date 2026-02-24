// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

func TestDyncfgConfig_ShutdownDoesNotQueue(t *testing.T) {
	tests := map[string]struct {
		fn functions.Function
	}{
		"canceled context sends single 503 response": {
			fn: functions.Function{
				UID:  "shutdown",
				Args: []string{"unknown:id", "schema"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer

			mgr := New()
			mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))))

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			mgr.ctx = ctx

			mgr.dyncfgConfig(dyncfg.NewFunction(tc.fn))

			// On shutdown, handler should respond once and stop.
			assert.Equal(t, 1, strings.Count(buf.String(), "FUNCTION_RESULT_BEGIN shutdown"))
			assert.Contains(t, buf.String(), "\"status\":503")
		})
	}
}
