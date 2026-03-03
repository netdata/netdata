// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

			mgr := New(Config{PluginName: testPluginName})
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

func TestDyncfgCollectorExec_TestCommandQueued(t *testing.T) {
	mgr := New(Config{PluginName: testPluginName})
	mgr.ctx = context.Background()
	mgr.dyncfgCh = make(chan dyncfg.Function, 1)

	fn := dyncfg.NewFunction(functions.Function{
		UID:  "queued-test",
		Args: []string{mgr.dyncfgModID("success"), "test", "job"},
	})

	mgr.dyncfgCollectorExec(fn)

	select {
	case queued := <-mgr.dyncfgCh:
		assert.Equal(t, dyncfg.CommandTest, queued.Command())
		assert.Equal(t, fn.UID(), queued.UID())
	case <-time.After(time.Second):
		t.Fatal("test command was not queued")
	}
}

func TestDyncfgCmdTest_WhenWorkerPoolFull_Returns503(t *testing.T) {
	var buf bytes.Buffer

	mgr := New(Config{PluginName: testPluginName})
	mgr.modules = prepareMockRegistry()
	mgr.ctx = context.Background()
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))))

	for i := 0; i < cap(mgr.cmdTestSem); i++ {
		mgr.cmdTestSem <- struct{}{}
	}

	cfg := prepareDyncfgCfg("success", "job")
	payload, err := json.Marshal(cfg)
	require.NoError(t, err)

	fn := dyncfg.NewFunction(functions.Function{
		UID:         "pool-full",
		ContentType: "application/json",
		Payload:     payload,
		Args: []string{
			mgr.dyncfgModID("success"),
			string(dyncfg.CommandTest),
			"job",
		},
	})

	mgr.dyncfgCmdTest(fn)

	out := buf.String()
	assert.Equal(t, 1, strings.Count(out, "FUNCTION_RESULT_BEGIN pool-full"))
	assert.Contains(t, out, "\"status\":503")
}

func TestDyncfgCmdTestTimeout_RequestTimeoutOverridesDefault(t *testing.T) {
	mgr := New(Config{PluginName: testPluginName})

	withRequestTimeout := dyncfg.NewFunction(functions.Function{Timeout: 7 * time.Second})
	assert.Equal(t, 7*time.Second, mgr.dyncfgCmdTestTimeout(withRequestTimeout))

	withoutTimeout := dyncfg.NewFunction(functions.Function{})
	assert.Equal(t, cmdTestDefaultTimeout, mgr.dyncfgCmdTestTimeout(withoutTimeout))
}
