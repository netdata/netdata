// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func TestTestDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"config.json": dataConfigJSON,
		"config.yaml": dataConfigYAML,
	} {
		t.Run(name, func(t *testing.T) { require.NotEmpty(t, data) })
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}
