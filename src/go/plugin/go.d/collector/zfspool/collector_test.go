// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package zfspool

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataZpoolList, _                  = os.ReadFile("testdata/zpool-list.txt")
	dataZpoolListWithVdev, _          = os.ReadFile("testdata/zpool-list-vdev.txt")
	dataZpoolListWithVdevLogsCache, _ = os.ReadFile("testdata/zpool-list-vdev-logs-cache.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataZpoolList":                  dataZpoolList,
		"dataZpoolListWithVdev":          dataZpoolListWithVdev,
		"dataZpoolListWithVdevLogsCache": dataZpoolListWithVdevLogsCache,
	} {
		require.NotNil(t, data, name)

	}
}

func TestCollector_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"fails if 'binary_path' is not set": {
			wantFail: true,
			config: Config{
				BinaryPath: "",
			},
		},
		"fails if failed to find binary": {
			wantFail: true,
			config: Config{
				BinaryPath: "zpool!!!",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Collector
	}{
		"not initialized exec": {
			prepare: func() *Collector {
				return New()
			},
		},
		"after check": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOk()
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOk()
				_ = collr.Collect(context.Background())
				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockZpoolCLIExec
		wantFail    bool
	}{
		"success case": {
			prepareMock: prepareMockOk,
			wantFail:    false,
		},
		"error on list call": {
			prepareMock: prepareMockErrOnList,
			wantFail:    true,
		},
		"empty response": {
			prepareMock: prepareMockEmptyResponse,
			wantFail:    true,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
			wantFail:    true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockZpoolCLIExec
		wantMetrics map[string]int64
	}{
		"success case": {
			prepareMock: prepareMockOk,
			wantMetrics: map[string]int64{
				"vdev_rpool/mirror-0/nvme0n1p3_health_state_degraded":  0,
				"vdev_rpool/mirror-0/nvme0n1p3_health_state_faulted":   0,
				"vdev_rpool/mirror-0/nvme0n1p3_health_state_offline":   0,
				"vdev_rpool/mirror-0/nvme0n1p3_health_state_online":    1,
				"vdev_rpool/mirror-0/nvme0n1p3_health_state_removed":   0,
				"vdev_rpool/mirror-0/nvme0n1p3_health_state_suspended": 0,
				"vdev_rpool/mirror-0/nvme0n1p3_health_state_unavail":   0,
				"vdev_rpool/mirror-0/nvme2n1p3_health_state_degraded":  0,
				"vdev_rpool/mirror-0/nvme2n1p3_health_state_faulted":   0,
				"vdev_rpool/mirror-0/nvme2n1p3_health_state_offline":   0,
				"vdev_rpool/mirror-0/nvme2n1p3_health_state_online":    1,
				"vdev_rpool/mirror-0/nvme2n1p3_health_state_removed":   0,
				"vdev_rpool/mirror-0/nvme2n1p3_health_state_suspended": 0,
				"vdev_rpool/mirror-0/nvme2n1p3_health_state_unavail":   0,
				"vdev_rpool/mirror-0_health_state_degraded":            0,
				"vdev_rpool/mirror-0_health_state_faulted":             0,
				"vdev_rpool/mirror-0_health_state_offline":             0,
				"vdev_rpool/mirror-0_health_state_online":              1,
				"vdev_rpool/mirror-0_health_state_removed":             0,
				"vdev_rpool/mirror-0_health_state_suspended":           0,
				"vdev_rpool/mirror-0_health_state_unavail":             0,
				"vdev_zion/mirror-0/nvme0n1p3_health_state_degraded":   0,
				"vdev_zion/mirror-0/nvme0n1p3_health_state_faulted":    0,
				"vdev_zion/mirror-0/nvme0n1p3_health_state_offline":    0,
				"vdev_zion/mirror-0/nvme0n1p3_health_state_online":     1,
				"vdev_zion/mirror-0/nvme0n1p3_health_state_removed":    0,
				"vdev_zion/mirror-0/nvme0n1p3_health_state_suspended":  0,
				"vdev_zion/mirror-0/nvme0n1p3_health_state_unavail":    0,
				"vdev_zion/mirror-0/nvme2n1p3_health_state_degraded":   0,
				"vdev_zion/mirror-0/nvme2n1p3_health_state_faulted":    0,
				"vdev_zion/mirror-0/nvme2n1p3_health_state_offline":    0,
				"vdev_zion/mirror-0/nvme2n1p3_health_state_online":     1,
				"vdev_zion/mirror-0/nvme2n1p3_health_state_removed":    0,
				"vdev_zion/mirror-0/nvme2n1p3_health_state_suspended":  0,
				"vdev_zion/mirror-0/nvme2n1p3_health_state_unavail":    0,
				"vdev_zion/mirror-0_health_state_degraded":             0,
				"vdev_zion/mirror-0_health_state_faulted":              0,
				"vdev_zion/mirror-0_health_state_offline":              0,
				"vdev_zion/mirror-0_health_state_online":               1,
				"vdev_zion/mirror-0_health_state_removed":              0,
				"vdev_zion/mirror-0_health_state_suspended":            0,
				"vdev_zion/mirror-0_health_state_unavail":              0,
				"zpool_rpool_alloc":                  9051643576,
				"zpool_rpool_cap":                    42,
				"zpool_rpool_frag":                   33,
				"zpool_rpool_free":                   12240656794,
				"zpool_rpool_health_state_degraded":  0,
				"zpool_rpool_health_state_faulted":   0,
				"zpool_rpool_health_state_offline":   0,
				"zpool_rpool_health_state_online":    1,
				"zpool_rpool_health_state_removed":   0,
				"zpool_rpool_health_state_suspended": 0,
				"zpool_rpool_health_state_unavail":   0,
				"zpool_rpool_size":                   21367462298,
				"zpool_zion_health_state_degraded":   0,
				"zpool_zion_health_state_faulted":    1,
				"zpool_zion_health_state_offline":    0,
				"zpool_zion_health_state_online":     0,
				"zpool_zion_health_state_removed":    0,
				"zpool_zion_health_state_suspended":  0,
				"zpool_zion_health_state_unavail":    0,
			},
		},
		"success case vdev logs and cache": {
			prepareMock: prepareMockOkVdevLogsCache,
			wantMetrics: map[string]int64{
				"vdev_rpool/cache/sdb2_health_state_degraded":                          0,
				"vdev_rpool/cache/sdb2_health_state_faulted":                           0,
				"vdev_rpool/cache/sdb2_health_state_offline":                           0,
				"vdev_rpool/cache/sdb2_health_state_online":                            1,
				"vdev_rpool/cache/sdb2_health_state_removed":                           0,
				"vdev_rpool/cache/sdb2_health_state_suspended":                         0,
				"vdev_rpool/cache/sdb2_health_state_unavail":                           0,
				"vdev_rpool/cache/wwn-0x500151795954c095-part2_health_state_degraded":  0,
				"vdev_rpool/cache/wwn-0x500151795954c095-part2_health_state_faulted":   0,
				"vdev_rpool/cache/wwn-0x500151795954c095-part2_health_state_offline":   0,
				"vdev_rpool/cache/wwn-0x500151795954c095-part2_health_state_online":    0,
				"vdev_rpool/cache/wwn-0x500151795954c095-part2_health_state_removed":   0,
				"vdev_rpool/cache/wwn-0x500151795954c095-part2_health_state_suspended": 0,
				"vdev_rpool/cache/wwn-0x500151795954c095-part2_health_state_unavail":   1,
				"vdev_rpool/logs/mirror-1/14807975228228307538_health_state_degraded":  0,
				"vdev_rpool/logs/mirror-1/14807975228228307538_health_state_faulted":   0,
				"vdev_rpool/logs/mirror-1/14807975228228307538_health_state_offline":   0,
				"vdev_rpool/logs/mirror-1/14807975228228307538_health_state_online":    0,
				"vdev_rpool/logs/mirror-1/14807975228228307538_health_state_removed":   0,
				"vdev_rpool/logs/mirror-1/14807975228228307538_health_state_suspended": 0,
				"vdev_rpool/logs/mirror-1/14807975228228307538_health_state_unavail":   1,
				"vdev_rpool/logs/mirror-1/sdb1_health_state_degraded":                  0,
				"vdev_rpool/logs/mirror-1/sdb1_health_state_faulted":                   0,
				"vdev_rpool/logs/mirror-1/sdb1_health_state_offline":                   0,
				"vdev_rpool/logs/mirror-1/sdb1_health_state_online":                    1,
				"vdev_rpool/logs/mirror-1/sdb1_health_state_removed":                   0,
				"vdev_rpool/logs/mirror-1/sdb1_health_state_suspended":                 0,
				"vdev_rpool/logs/mirror-1/sdb1_health_state_unavail":                   0,
				"vdev_rpool/logs/mirror-1_health_state_degraded":                       1,
				"vdev_rpool/logs/mirror-1_health_state_faulted":                        0,
				"vdev_rpool/logs/mirror-1_health_state_offline":                        0,
				"vdev_rpool/logs/mirror-1_health_state_online":                         0,
				"vdev_rpool/logs/mirror-1_health_state_removed":                        0,
				"vdev_rpool/logs/mirror-1_health_state_suspended":                      0,
				"vdev_rpool/logs/mirror-1_health_state_unavail":                        0,
				"vdev_rpool/mirror-0/sdc2_health_state_degraded":                       0,
				"vdev_rpool/mirror-0/sdc2_health_state_faulted":                        0,
				"vdev_rpool/mirror-0/sdc2_health_state_offline":                        0,
				"vdev_rpool/mirror-0/sdc2_health_state_online":                         1,
				"vdev_rpool/mirror-0/sdc2_health_state_removed":                        0,
				"vdev_rpool/mirror-0/sdc2_health_state_suspended":                      0,
				"vdev_rpool/mirror-0/sdc2_health_state_unavail":                        0,
				"vdev_rpool/mirror-0/sdd2_health_state_degraded":                       0,
				"vdev_rpool/mirror-0/sdd2_health_state_faulted":                        0,
				"vdev_rpool/mirror-0/sdd2_health_state_offline":                        0,
				"vdev_rpool/mirror-0/sdd2_health_state_online":                         1,
				"vdev_rpool/mirror-0/sdd2_health_state_removed":                        0,
				"vdev_rpool/mirror-0/sdd2_health_state_suspended":                      0,
				"vdev_rpool/mirror-0/sdd2_health_state_unavail":                        0,
				"vdev_rpool/mirror-0_health_state_degraded":                            0,
				"vdev_rpool/mirror-0_health_state_faulted":                             0,
				"vdev_rpool/mirror-0_health_state_offline":                             0,
				"vdev_rpool/mirror-0_health_state_online":                              1,
				"vdev_rpool/mirror-0_health_state_removed":                             0,
				"vdev_rpool/mirror-0_health_state_suspended":                           0,
				"vdev_rpool/mirror-0_health_state_unavail":                             0,
				"vdev_zion/cache/sdb2_health_state_degraded":                           0,
				"vdev_zion/cache/sdb2_health_state_faulted":                            0,
				"vdev_zion/cache/sdb2_health_state_offline":                            0,
				"vdev_zion/cache/sdb2_health_state_online":                             1,
				"vdev_zion/cache/sdb2_health_state_removed":                            0,
				"vdev_zion/cache/sdb2_health_state_suspended":                          0,
				"vdev_zion/cache/sdb2_health_state_unavail":                            0,
				"vdev_zion/cache/wwn-0x500151795954c095-part2_health_state_degraded":   0,
				"vdev_zion/cache/wwn-0x500151795954c095-part2_health_state_faulted":    0,
				"vdev_zion/cache/wwn-0x500151795954c095-part2_health_state_offline":    0,
				"vdev_zion/cache/wwn-0x500151795954c095-part2_health_state_online":     0,
				"vdev_zion/cache/wwn-0x500151795954c095-part2_health_state_removed":    0,
				"vdev_zion/cache/wwn-0x500151795954c095-part2_health_state_suspended":  0,
				"vdev_zion/cache/wwn-0x500151795954c095-part2_health_state_unavail":    1,
				"vdev_zion/logs/mirror-1/14807975228228307538_health_state_degraded":   0,
				"vdev_zion/logs/mirror-1/14807975228228307538_health_state_faulted":    0,
				"vdev_zion/logs/mirror-1/14807975228228307538_health_state_offline":    0,
				"vdev_zion/logs/mirror-1/14807975228228307538_health_state_online":     0,
				"vdev_zion/logs/mirror-1/14807975228228307538_health_state_removed":    0,
				"vdev_zion/logs/mirror-1/14807975228228307538_health_state_suspended":  0,
				"vdev_zion/logs/mirror-1/14807975228228307538_health_state_unavail":    1,
				"vdev_zion/logs/mirror-1/sdb1_health_state_degraded":                   0,
				"vdev_zion/logs/mirror-1/sdb1_health_state_faulted":                    0,
				"vdev_zion/logs/mirror-1/sdb1_health_state_offline":                    0,
				"vdev_zion/logs/mirror-1/sdb1_health_state_online":                     1,
				"vdev_zion/logs/mirror-1/sdb1_health_state_removed":                    0,
				"vdev_zion/logs/mirror-1/sdb1_health_state_suspended":                  0,
				"vdev_zion/logs/mirror-1/sdb1_health_state_unavail":                    0,
				"vdev_zion/logs/mirror-1_health_state_degraded":                        1,
				"vdev_zion/logs/mirror-1_health_state_faulted":                         0,
				"vdev_zion/logs/mirror-1_health_state_offline":                         0,
				"vdev_zion/logs/mirror-1_health_state_online":                          0,
				"vdev_zion/logs/mirror-1_health_state_removed":                         0,
				"vdev_zion/logs/mirror-1_health_state_suspended":                       0,
				"vdev_zion/logs/mirror-1_health_state_unavail":                         0,
				"vdev_zion/mirror-0/sdc2_health_state_degraded":                        0,
				"vdev_zion/mirror-0/sdc2_health_state_faulted":                         0,
				"vdev_zion/mirror-0/sdc2_health_state_offline":                         0,
				"vdev_zion/mirror-0/sdc2_health_state_online":                          1,
				"vdev_zion/mirror-0/sdc2_health_state_removed":                         0,
				"vdev_zion/mirror-0/sdc2_health_state_suspended":                       0,
				"vdev_zion/mirror-0/sdc2_health_state_unavail":                         0,
				"vdev_zion/mirror-0/sdd2_health_state_degraded":                        0,
				"vdev_zion/mirror-0/sdd2_health_state_faulted":                         0,
				"vdev_zion/mirror-0/sdd2_health_state_offline":                         0,
				"vdev_zion/mirror-0/sdd2_health_state_online":                          1,
				"vdev_zion/mirror-0/sdd2_health_state_removed":                         0,
				"vdev_zion/mirror-0/sdd2_health_state_suspended":                       0,
				"vdev_zion/mirror-0/sdd2_health_state_unavail":                         0,
				"vdev_zion/mirror-0_health_state_degraded":                             0,
				"vdev_zion/mirror-0_health_state_faulted":                              0,
				"vdev_zion/mirror-0_health_state_offline":                              0,
				"vdev_zion/mirror-0_health_state_online":                               1,
				"vdev_zion/mirror-0_health_state_removed":                              0,
				"vdev_zion/mirror-0_health_state_suspended":                            0,
				"vdev_zion/mirror-0_health_state_unavail":                              0,
				"zpool_rpool_alloc":                  9051643576,
				"zpool_rpool_cap":                    42,
				"zpool_rpool_frag":                   33,
				"zpool_rpool_free":                   12240656794,
				"zpool_rpool_health_state_degraded":  0,
				"zpool_rpool_health_state_faulted":   0,
				"zpool_rpool_health_state_offline":   0,
				"zpool_rpool_health_state_online":    1,
				"zpool_rpool_health_state_removed":   0,
				"zpool_rpool_health_state_suspended": 0,
				"zpool_rpool_health_state_unavail":   0,
				"zpool_rpool_size":                   21367462298,
				"zpool_zion_health_state_degraded":   0,
				"zpool_zion_health_state_faulted":    1,
				"zpool_zion_health_state_offline":    0,
				"zpool_zion_health_state_online":     0,
				"zpool_zion_health_state_removed":    0,
				"zpool_zion_health_state_suspended":  0,
				"zpool_zion_health_state_unavail":    0,
			},
		},
		"error on list call": {
			prepareMock: prepareMockErrOnList,
			wantMetrics: nil,
		},
		"empty response": {
			prepareMock: prepareMockEmptyResponse,
			wantMetrics: nil,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				want := len(zpoolChartsTmpl)*len(collr.seenZpools) + len(vdevChartsTmpl)*len(collr.seenVdevs)

				assert.Len(t, *collr.Charts(), want, "want charts")

				module.TestMetricsHasAllChartsDimsSkip(t, collr.Charts(), mx, func(chart *module.Chart, _ *module.Dim) bool {
					return strings.HasPrefix(chart.ID, "zfspool_zion") && !strings.HasSuffix(chart.ID, "health_state")
				})
			}
		})
	}
}

func TestCollector_parseZpoolListDevOutput(t *testing.T) {
	tests := map[string]struct {
		input string
		want  []vdevEntry
	}{
		"": {
			input: `
NAME                                     SIZE          ALLOC           FREE  CKPOINT  EXPANDSZ   FRAG    CAP  DEDUP    HEALTH  ALTROOT
store                           9981503995904  3046188658688  6935315337216        -         -      9     30   1.00  DEGRADED  -
  mirror-0                      9981503995904  3046188658688  6935315337216        -         -      9     30      -    ONLINE
    sdc2                        9998683602944      -      -        -         -      -      -      -    ONLINE
    sdd2                        9998683602944      -      -        -         -      -      -      -    ONLINE
logs                                -      -      -        -         -      -      -      -         -
  mirror-1                      17716740096  393216  17716346880        -         -      0      0      -  DEGRADED
    sdb1                        17951621120      -      -        -         -      -      -      -    ONLINE
    14807975228228307538            -      -      -        -         -      -      -      -   UNAVAIL
cache                               -      -      -        -         -      -      -      -         -
  sdb2                          99000254464  98755866624  239665152        -         -      0     99      -    ONLINE
  wwn-0x500151795954c095-part2      -      -      -        -         -      -      -      -   UNAVAIL
`,
			want: []vdevEntry{
				{
					name:   "mirror-0",
					health: "online",
					vdev:   "store/mirror-0",
					level:  2,
				},
				{
					name:   "sdc2",
					health: "online",
					vdev:   "store/mirror-0/sdc2",
					level:  4,
				},
				{
					name:   "sdd2",
					health: "online",
					vdev:   "store/mirror-0/sdd2",
					level:  4,
				},
				{
					name:   "logs",
					health: "-",
					vdev:   "store/logs",
					level:  0,
				},
				{
					name:   "mirror-1",
					health: "degraded",
					vdev:   "store/logs/mirror-1",
					level:  2,
				},
				{
					name:   "sdb1",
					health: "online",
					vdev:   "store/logs/mirror-1/sdb1",
					level:  4,
				},
				{
					name:   "14807975228228307538",
					health: "unavail",
					vdev:   "store/logs/mirror-1/14807975228228307538",
					level:  4,
				},
				{
					name:   "cache",
					health: "-",
					vdev:   "store/cache",
					level:  0,
				},
				{
					name:   "sdb2",
					health: "online",
					vdev:   "store/cache/sdb2",
					level:  2,
				},
				{
					name:   "wwn-0x500151795954c095-part2",
					health: "unavail",
					vdev:   "store/cache/wwn-0x500151795954c095-part2",
					level:  2,
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			v, err := parseZpoolListVdevOutput([]byte(test.input))
			require.NoError(t, err)
			assert.Equal(t, test.want, v)
		})
	}
}

func prepareMockOk() *mockZpoolCLIExec {
	return &mockZpoolCLIExec{
		listData:         dataZpoolList,
		listWithVdevData: dataZpoolListWithVdev,
	}
}

func prepareMockOkVdevLogsCache() *mockZpoolCLIExec {
	return &mockZpoolCLIExec{
		listData:         dataZpoolList,
		listWithVdevData: dataZpoolListWithVdevLogsCache,
	}
}

func prepareMockErrOnList() *mockZpoolCLIExec {
	return &mockZpoolCLIExec{
		errOnList: true,
	}
}

func prepareMockEmptyResponse() *mockZpoolCLIExec {
	return &mockZpoolCLIExec{}
}

func prepareMockUnexpectedResponse() *mockZpoolCLIExec {
	return &mockZpoolCLIExec{
		listData: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

type mockZpoolCLIExec struct {
	errOnList        bool
	listData         []byte
	listWithVdevData []byte
}

func (m *mockZpoolCLIExec) list() ([]byte, error) {
	if m.errOnList {
		return nil, errors.New("mock.list() error")
	}

	return m.listData, nil
}

func (m *mockZpoolCLIExec) listWithVdev(pool string) ([]byte, error) {
	s := string(m.listWithVdevData)
	s = strings.Replace(s, "rpool", pool, 1)

	return []byte(s), nil
}
