// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServiceDiscovery_UsesConfiguredOutForDyncfgResponder(t *testing.T) {
	const pluginName = "test"

	var buf bytes.Buffer
	sd, err := NewServiceDiscovery(Config{
		PluginName:  pluginName,
		Out:         &buf,
		Discoverers: NewRegistry(),
	})
	require.NoError(t, err)

	sd.dyncfgApi.ConfigCreate(netdataapi.ConfigOpts{
		ID:                "test:sd:discoverer",
		Status:            dyncfg.StatusAccepted.String(),
		ConfigType:        dyncfg.ConfigTypeTemplate.String(),
		Path:              fmt.Sprintf(dyncfgSDPath, pluginName),
		SourceType:        "internal",
		Source:            "internal",
		SupportedCommands: "schema",
	})

	assert.Contains(t, buf.String(), "CONFIG test:sd:discoverer create accepted template /collectors/test/ServiceDiscovery")
}
