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

func TestNewServiceDiscoveryUsesConfiguredDyncfgOutput(t *testing.T) {
	const pluginName = "test"

	var buf bytes.Buffer
	sd, err := NewServiceDiscovery(Config{
		Epoch:        1,
		Attempts:     newTestAttemptAuthority(t),
		PluginName:   pluginName,
		DyncfgOutput: dyncfg.NewProtocolOutput(&buf),
		Discoverers:  NewRegistry(),
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

	assert.Contains(
		t,
		buf.String(),
		"CONFIG test:sd:discoverer create accepted template /collectors/test/ServiceDiscovery",
	)
}

func emitTestNotification(sd *ServiceDiscovery, n dyncfg.Notification) {
	switch n.Kind {
	case dyncfg.NotificationCreate:
		sd.dyncfgApi.ConfigCreate(n.Config)
	case dyncfg.NotificationStatus:
		sd.dyncfgApi.ConfigStatus(n.ID, n.Status)
	case dyncfg.NotificationDelete:
		sd.dyncfgApi.ConfigDelete(n.ID)
	}
}
