// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
)

func TestNew_UsesConfiguredOutForDyncfgResponder(t *testing.T) {
	const pluginName = "test"

	var buf bytes.Buffer
	mgr := New(Config{
		PluginName: pluginName,
		Out:        &buf,
	})

	mgr.dyncfgApi.ConfigCreate(netdataapi.ConfigOpts{
		ID:                "test:collector:module",
		Status:            dyncfg.StatusAccepted.String(),
		ConfigType:        dyncfg.ConfigTypeTemplate.String(),
		Path:              fmt.Sprintf(dyncfgCollectorPath, pluginName),
		SourceType:        "internal",
		Source:            "internal",
		SupportedCommands: "schema",
	})

	assert.Contains(t, buf.String(), "CONFIG test:collector:module create accepted template /collectors/test/Jobs")
}
