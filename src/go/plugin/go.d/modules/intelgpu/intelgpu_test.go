// SPDX-License-Identifier: GPL-3.0-or-later

package intelgpu

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

func TestIntelGPU_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &IntelGPU{}, dataConfigJSON, dataConfigYAML)
}

func TestIntelGPU_Init(t *testing.T) {
	tests := map[string]struct {
		prepare  func(igt *IntelGPU)
		wantFail bool
	}{
		"fails if can't locate ndsudo": {
			wantFail: true,
			prepare: func(igt *IntelGPU) {
				igt.ndsudoName += "!!!"
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			igt := New()

			test.prepare(igt)

			if test.wantFail {
				assert.Error(t, igt.Init())
			} else {
				assert.NoError(t, igt.Init())
			}
		})
	}
}

func TestIntelGPU_Check(t *testing.T) {
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
			igt := New()
			mock := test.prepareMock()
			igt.exec = mock

			if test.wantFail {
				assert.Error(t, igt.Check())
			} else {
				assert.NoError(t, igt.Check())
			}
		})
	}
}

func TestIntelGPU_Collect(t *testing.T) {
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
			igt := New()
			mock := test.prepareMock()
			igt.exec = mock

			mx := igt.Collect()

			assert.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				assert.Len(t, *igt.Charts(), len(charts)+len(igt.engines))
			}
		})
	}
}

func TestIntelGPU_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *IntelGPU
	}{
		"not initialized exec": {
			prepare: func() *IntelGPU {
				return New()
			},
		},
		"after check": {
			prepare: func() *IntelGPU {
				igt := New()
				igt.exec = prepareMockOK()
				_ = igt.Check()
				return igt
			},
		},
		"after collect": {
			prepare: func() *IntelGPU {
				igt := New()
				igt.exec = prepareMockOK()
				_ = igt.Collect()
				return igt
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			igt := test.prepare()

			mock, ok := igt.exec.(*mockIntelGpuTop)

			assert.NotPanics(t, igt.Cleanup)

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
