// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataDf, _           = os.ReadFile("testdata/df.json")
	dataOsdDf, _        = os.ReadFile("testdata/osd_df.json")
	dataOsdPerf, _      = os.ReadFile("testdata/osd_perf.json")
	dataOsdPoolStats, _ = os.ReadFile("testdata/osd_pool_stats.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":   dataConfigJSON,
		"dataConfigYAML":   dataConfigYAML,
		"dataDfStats":      dataDf,
		"dataOsdDf":        dataOsdDf,
		"dataOsdPerf":      dataOsdPerf,
		"dataOsdPoolStats": dataOsdPoolStats,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCeph_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Ceph{}, dataConfigJSON, dataConfigYAML)
}
