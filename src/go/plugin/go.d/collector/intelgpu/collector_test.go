// SPDX-License-Identifier: GPL-3.0-or-later

package intelgpu

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

	dataIntelTopGpuJSON, _ = os.ReadFile("testdata/igt.json")
)

func Test_testDataIsValid(t *testing.T) {

	for name, data := range map[string][]byte{
		"dataConfigJSON":      dataConfigJSON,
		"dataConfigYAML":      dataConfigYAML,
		"dataIntelTopGpuJSON": dataIntelTopGpuJSON,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		prepare  func(collr *Collector)
		wantFail bool
	}{
		"fails if can't locate ndsudo": {
			wantFail: true,
			prepare: func(collr *Collector) {
				collr.ndsudoName += "!!!"
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()

			test.prepare(collr)

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockIntelGpuTop
		wantFail    bool
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantFail:    false,
		},
		"fail on error": {
			prepareMock: prepareMockErrOnGPUSummaryJson,
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
		prepareMock func() *mockIntelGpuTop
		wantMetrics map[string]int64
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantMetrics: map[string]int64{
				"engine_Blitter/0_busy":      0,
				"engine_Render/3D/0_busy":    9609,
				"engine_Video/0_busy":        7295,
				"engine_Video/1_busy":        7740,
				"engine_VideoEnhance/0_busy": 0,
				"frequency_actual":           125308,
				"power_gpu":                  323,
				"power_package":              1665,
			},
		},
		"fail on error": {
			prepareMock: prepareMockErrOnGPUSummaryJson,
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
				assert.Len(t, *collr.Charts(), len(charts)+len(collr.engines))
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

			mock, ok := collr.exec.(*mockIntelGpuTop)

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })

			if ok {
				assert.True(t, mock.stopCalled)
			}
		})
	}
}

func prepareMockOK() *mockIntelGpuTop {
	return &mockIntelGpuTop{
		gpuSummaryJson: dataIntelTopGpuJSON,
	}
}

func prepareMockErrOnGPUSummaryJson() *mockIntelGpuTop {
	return &mockIntelGpuTop{
		errOnQueryGPUSummaryJson: true,
	}
}

type mockIntelGpuTop struct {
	errOnQueryGPUSummaryJson bool
	gpuSummaryJson           []byte

	stopCalled bool
}

func (m *mockIntelGpuTop) queryGPUSummaryJson() ([]byte, error) {
	if m.errOnQueryGPUSummaryJson {
		return nil, errors.New("error on mock.queryGPUSummaryJson()")
	}
	return m.gpuSummaryJson, nil
}

func (m *mockIntelGpuTop) stop() error {
	m.stopCalled = true
	return nil
}
