// SPDX-License-Identifier: GPL-3.0-or-later

package dmcache

import (
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
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

func TestDmCace_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &DmCache{}, dataConfigJSON, dataConfigYAML)
}

func TestDmCache_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"fails if failed to locate ndsudo": {
			wantFail: true,
			config:   New().Config,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			lvm := New()
			lvm.Config = test.config

			if test.wantFail {
				assert.Error(t, lvm.Init())
			} else {
				assert.NoError(t, lvm.Init())
			}
		})
	}
}

func TestDmCache_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *DmCache
	}{
		"not initialized exec": {
			prepare: func() *DmCache {
				return New()
			},
		},
		"after check": {
			prepare: func() *DmCache {
				lvm := New()
				lvm.exec = prepareMockOK()
				_ = lvm.Check()
				return lvm
			},
		},
		"after collect": {
			prepare: func() *DmCache {
				lvm := New()
				lvm.exec = prepareMockOK()
				_ = lvm.Collect()
				return lvm
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			lvm := test.prepare()

			assert.NotPanics(t, lvm.Cleanup)
		})
	}
}

func TestDmCache_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestDmCache_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockDmsetupExec
		wantFail    bool
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantFail:    false,
		},
		"error on cache status": {
			prepareMock: prepareMockErr,
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
			dmcache := New()
			mock := test.prepareMock()
			dmcache.exec = mock

			if test.wantFail {
				assert.Error(t, dmcache.Check())
			} else {
				assert.NoError(t, dmcache.Check())
			}
		})
	}
}

func TestLVM_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockDmsetupExec
		wantCharts  int
		wantMetrics map[string]int64
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantCharts:  len(deviceChartsTmpl) * 2,
			wantMetrics: map[string]int64{
				"dmcache_device_vg_raid1_md21-media_cache_free_bytes":    1252402397184,
				"dmcache_device_vg_raid1_md21-media_cache_used_bytes":    396412059648,
				"dmcache_device_vg_raid1_md21-media_demotions_bytes":     0,
				"dmcache_device_vg_raid1_md21-media_dirty_bytes":         0,
				"dmcache_device_vg_raid1_md21-media_metadata_free_bytes": 32243712,
				"dmcache_device_vg_raid1_md21-media_metadata_used_bytes": 9699328,
				"dmcache_device_vg_raid1_md21-media_promotions_bytes":    48035266560,
				"dmcache_device_vg_raid1_md21-media_read_hits":           82870357,
				"dmcache_device_vg_raid1_md21-media_read_misses":         5499462,
				"dmcache_device_vg_raid1_md21-media_write_hits":          26280342,
				"dmcache_device_vg_raid1_md21-media_write_misses":        8017854,
				"dmcache_device_vg_raid2_md22-media_cache_free_bytes":    1252402397184,
				"dmcache_device_vg_raid2_md22-media_cache_used_bytes":    396412059648,
				"dmcache_device_vg_raid2_md22-media_demotions_bytes":     0,
				"dmcache_device_vg_raid2_md22-media_dirty_bytes":         0,
				"dmcache_device_vg_raid2_md22-media_metadata_free_bytes": 32243712,
				"dmcache_device_vg_raid2_md22-media_metadata_used_bytes": 9699328,
				"dmcache_device_vg_raid2_md22-media_promotions_bytes":    48035266560,
				"dmcache_device_vg_raid2_md22-media_read_hits":           82870357,
				"dmcache_device_vg_raid2_md22-media_read_misses":         5499462,
				"dmcache_device_vg_raid2_md22-media_write_hits":          26280342,
				"dmcache_device_vg_raid2_md22-media_write_misses":        8017854,
			},
		},
		"error on cache status": {
			prepareMock: prepareMockErr,
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
			dmcache := New()
			mock := test.prepareMock()
			dmcache.exec = mock

			mx := dmcache.Collect()

			assert.Equal(t, test.wantMetrics, mx)
			assert.Len(t, *dmcache.Charts(), test.wantCharts)
			testMetricsHasAllChartsDims(t, dmcache, mx)
		})
	}
}

func testMetricsHasAllChartsDims(t *testing.T, dmcache *DmCache, mx map[string]int64) {
	for _, chart := range *dmcache.Charts() {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			_, ok := mx[dim.ID]
			assert.Truef(t, ok, "collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
		}
	}
}

func prepareMockOK() *mockDmsetupExec {
	return &mockDmsetupExec{
		cacheStatusData: []byte(`
vg_raid1_md21-media: 0 2404139008 cache 8 2368/10240 4096 189024/786216 82870357 5499462 26280342 8017854 0 22905 0 3 metadata2 writethrough no_discard_passdown 2 migration_threshold 32768 mq 10 random_threshold 0 sequential_threshold 0 discard_promote_adjustment 0 read_promote_adjustment 0 write_promote_adjustment 0 rw - 
vg_raid2_md22-media: 0 2404139008 cache 8 2368/10240 4096 189024/786216 82870357 5499462 26280342 8017854 0 22905 0 3 metadata2 writethrough no_discard_passdown 2 migration_threshold 32768 mq 10 random_threshold 0 sequential_threshold 0 discard_promote_adjustment 0 read_promote_adjustment 0 write_promote_adjustment 0 rw - 
`),
	}
}

func prepareMockErr() *mockDmsetupExec {
	return &mockDmsetupExec{
		errOnCacheStatus: true,
	}
}

func prepareMockEmptyResponse() *mockDmsetupExec {
	return &mockDmsetupExec{}
}

func prepareMockUnexpectedResponse() *mockDmsetupExec {
	return &mockDmsetupExec{
		cacheStatusData: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

type mockDmsetupExec struct {
	errOnCacheStatus bool
	cacheStatusData  []byte
}

func (m *mockDmsetupExec) cacheStatus() ([]byte, error) {
	if m.errOnCacheStatus {
		return nil, errors.New("mock.cacheStatus() error")
	}

	return m.cacheStatusData, nil
}
