// SPDX-License-Identifier: GPL-3.0-or-later

package geth

import (
	"os"
	"testing"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"

	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestGeth_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Geth{}, dataConfigJSON, dataConfigYAML)
}
