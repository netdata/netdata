// SPDX-License-Identifier: GPL-3.0-or-later

package redfish_logs

import (
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func TestCollectorConfigurationSerialize(t *testing.T) {
	require.NotEmpty(t, dataConfigJSON)
	require.NotEmpty(t, dataConfigYAML)
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestConfigSchemaMatchesMetadata(t *testing.T) {
	collecttest.AssertConfigSchemaMatchesMetadata(t, "config_schema.json", "metadata.yaml")
}

func TestConfigRejectsReservedJournalSourceNames(t *testing.T) {
	for name := range reservedJournalSourceNames {
		t.Run(name, func(t *testing.T) {
			cfg := Config{}
			cfg.applyDefaults()
			err := cfg.validate(name)
			require.ErrorContains(t, err, "reserved by the journal query interface")
		})
	}

	cfg := Config{}
	cfg.applyDefaults()
	assert.NoError(t, cfg.validate("default"))
}
