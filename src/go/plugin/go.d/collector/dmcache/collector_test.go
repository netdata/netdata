// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dmcache

import (
	"context"
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

func TestCollector_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success with default config": {
			wantFail: false,
			config:   New().Config,
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
				collr.exec = prepareMockOK()
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOK()
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
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantMetrics, mx)

			assert.Len(t, *collr.Charts(), test.wantCharts, "wantCharts")

			module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
		})
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
