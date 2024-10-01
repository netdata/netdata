// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
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

func TestOracleDB_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &OracleDB{}, dataConfigJSON, dataConfigYAML)
}
