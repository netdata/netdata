// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRecordFixtures(t *testing.T) {
	tests := map[string]struct {
		want sample
	}{
		"tx1": {want: sample{
			GPUUtilization: measurement(0),
			GPUFrequency:   measurement(76),
			EMCUtilization: measurement(7),
			EMCFrequency:   measurement(408),
		}},
		"tx2": {want: sample{
			GPUUtilization: measurement(0),
			GPUFrequency:   measurement(624),
			EMCUtilization: measurement(4),
			EMCFrequency:   measurement(1600),
		}},
		"nano": {want: sample{
			GPUUtilization: measurement(0),
			GPUFrequency:   measurement(76),
			EMCUtilization: measurement(0),
			EMCFrequency:   measurement(204),
		}},
		"agx-xavier": {want: sample{
			GPUUtilization: measurement(0),
			GPUFrequency:   measurement(318),
			EMCUtilization: measurement(0),
			EMCFrequency:   measurement(665),
		}},
		"xavier-nx": {want: sample{
			GPUUtilization: measurement(62),
			GPUFrequency:   measurement(306),
			EMCUtilization: measurement(10),
			EMCFrequency:   measurement(1600),
		}},
		"power-units": {want: sample{
			GPUUtilization: measurement(0),
			GPUFrequency:   measurement(611),
			EMCUtilization: measurement(0),
			EMCFrequency:   measurement(2133),
		}},
		"orin-r36": {want: sample{
			GPUUtilization: measurement(0),
			GPCFrequencies: []*float64{measurement(305), measurement(305)},
			EMCUtilization: measurement(1),
			EMCFrequency:   measurement(2133),
		}},
		"orin-utilization": {want: sample{
			GPUUtilization: measurement(0),
		}},
		"thor-frequency": {want: sample{
			GPCFrequencies: []*float64{measurement(494), measurement(494), measurement(494)},
			EMCUtilization: measurement(0),
			EMCFrequency:   measurement(2750),
		}},
		"thor-r38.4": {want: sample{
			GPCFrequencies: []*float64{measurement(314), measurement(314), measurement(314)},
			EMCUtilization: measurement(0),
			EMCFrequency:   measurement(2750),
		}},
		"thor-no-gpu-emc": {want: sample{}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "tegrastats", name+".txt"))
			require.NoError(t, err)
			got, ok := parseRecord(string(data))
			assert.True(t, ok)
			assert.Equal(t, test.want, got)
		})
	}
}

// testRecordPrefix is a minimal record envelope that synthetic readings follow.
const testRecordPrefix = "RAM 20/100MB (lfb 3x4MB) CPU [0%@100,off] "

func TestParseRecordReadings(t *testing.T) {
	tests := map[string]struct {
		fields string
		want   sample
	}{
		"utilization only": {fields: "GR3D_FREQ 21% EMC_FREQ 32%", want: sample{
			GPUUtilization: measurement(21),
			EMCUtilization: measurement(32),
		}},
		"frequency only": {fields: "GR3D_FREQ @400 EMC_FREQ @1600", want: sample{
			GPUFrequency: measurement(400),
			EMCFrequency: measurement(1600),
		}},
		"decimal": {fields: "GR3D_FREQ 25.5%@400.25 EMC_FREQ 2.5%@1600.5", want: sample{
			GPUUtilization: measurement(25.5),
			GPUFrequency:   measurement(400.25),
			EMCUtilization: measurement(2.5),
			EMCFrequency:   measurement(1600.5),
		}},
		"zero": {fields: "GR3D_FREQ 0%@0 EMC_FREQ 0%@0", want: sample{
			GPUUtilization: measurement(0),
			GPUFrequency:   measurement(0),
			EMCUtilization: measurement(0),
			EMCFrequency:   measurement(0),
		}},
		"full utilization": {fields: "GR3D_FREQ 100% EMC_FREQ 100%", want: sample{
			GPUUtilization: measurement(100),
			EMCUtilization: measurement(100),
		}},
		"array whitespace": {fields: "GR3D_FREQ\t20%@[0, 300,\t400] EMC_FREQ 3%", want: sample{
			GPUUtilization: measurement(20),
			GPCFrequencies: []*float64{measurement(0), measurement(300), measurement(400)},
			EMCUtilization: measurement(3),
		}},
		"missing array elements retain indexes": {fields: "GR3D_FREQ @[N/A,200,,400,off]", want: sample{
			GPCFrequencies: []*float64{nil, measurement(200), nil, measurement(400), nil},
		}},
		"invalid array elements retain indexes": {fields: "GR3D_FREQ @[NaN,Inf,-2,1e999,500]", want: sample{
			GPCFrequencies: []*float64{nil, nil, nil, nil, measurement(500)},
		}},
		"utilization survives invalid clocks": {fields: "GR3D_FREQ 20%@N/A EMC_FREQ 40%@NaN", want: sample{
			GPUUtilization: measurement(20),
			EMCUtilization: measurement(40),
		}},
		"clocks survive invalid utilization": {fields: "GR3D_FREQ N/A%@[200,300] EMC_FREQ 101%@400", want: sample{
			GPCFrequencies: []*float64{measurement(200), measurement(300)},
			EMCFrequency:   measurement(400),
		}},
		"missing GPU value keeps EMC": {fields: "GR3D_FREQ EMC_FREQ 30%", want: sample{
			EMCUtilization: measurement(30),
		}},
		"off":          {fields: "GR3D_FREQ off EMC_FREQ off"},
		"missing":      {fields: "VDD_GPU 20mW/30mW gpu@45C"},
		"unavailable":  {fields: "GR3D_FREQ N/A EMC_FREQ N/A"},
		"nonfinite":    {fields: "GR3D_FREQ NaN%@Inf EMC_FREQ Inf%@NaN"},
		"negative":     {fields: "GR3D_FREQ -1%@-2 EMC_FREQ -3%@-4"},
		"out of range": {fields: "GR3D_FREQ 100.01% EMC_FREQ 101%"},
		"malformed":    {fields: "GR3D_FREQ 20oops%@400MHz EMC_FREQ 3%%@4oops"},
		"bare number is not utilization or frequency": {fields: "GR3D_FREQ 400 EMC_FREQ 1600"},
		"empty array": {fields: "GR3D_FREQ @[]"},
		"unclosed array preserves utilization": {fields: "GR3D_FREQ 20%@[200,300 EMC_FREQ 30%", want: sample{
			GPUUtilization: measurement(20),
			EMCUtilization: measurement(30),
		}},
		"unrelated fields": {
			fields: "FUTURE_FREQ 100%@900 GR3D_FREQ 8% GPU 30/40 EMC_FREQ @1500 NEW [1,2,3]",
			want: sample{
				GPUUtilization: measurement(8),
				EMCFrequency:   measurement(1500),
			},
		},
		"exact key": {fields: "NOT_GR3D_FREQ 20% OTHER_EMC_FREQ 40%"},
		"repeated key replaces the reading": {fields: "GR3D_FREQ 10%@[1,2] GR3D_FREQ 20%", want: sample{
			GPUUtilization: measurement(20),
		}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := parseRecord(testRecordPrefix + test.fields)
			assert.True(t, ok)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestParseRecordRecognition(t *testing.T) {
	tests := map[string]struct {
		line string
		ok   bool
	}{
		"empty":            {},
		"warning":          {line: "Warning: failed to read GR3D_FREQ 20%"},
		"metric fragment":  {line: "GR3D_FREQ 20% EMC_FREQ 10%"},
		"RAM fragment":     {line: "RAM 20/100MB (lfb 3x4MB)"},
		"malformed RAM":    {line: "RAM broken (lfb 3x4MB) CPU [0%@100] GR3D_FREQ 20%"},
		"malformed CPU":    {line: "RAM 20/100MB (lfb 3x4MB) CPU [ GR3D_FREQ 20%"},
		"arbitrary prefix": {line: "Warning: RAM 20/100MB (lfb 3x4MB) CPU [0%@100]"},
		"empty record":     {line: "RAM 20/100MB (lfb 3x4MB) CPU [0%@100]", ok: true},
		"date prefix":      {line: "06-12-2026 13:04:13 RAM 20/100MB (lfb 3x4MB) CPU [0%@100]", ok: true},
		"whitespace":       {line: " \tRAM\t20/100MB  (lfb\t3x4MB)\tCPU\t[0%@100] \r\n", ok: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := parseRecord(test.line)
			assert.Equal(t, test.ok, ok)
			assert.Equal(t, sample{}, got)
		})
	}
}

func TestSampleHasReadings(t *testing.T) {
	tests := map[string]struct {
		sample sample
		want   bool
	}{
		"empty": {},
		"GPU zero utilization": {sample: sample{
			GPUUtilization: measurement(0),
		}, want: true},
		"GPU scalar zero clock": {sample: sample{
			GPUFrequency: measurement(0),
		}, want: true},
		"EMC zero utilization": {sample: sample{
			EMCUtilization: measurement(0),
		}, want: true},
		"EMC zero clock": {sample: sample{
			EMCFrequency: measurement(0),
		}, want: true},
		"all unavailable GPCs": {sample: sample{
			GPCFrequencies: []*float64{nil, nil},
		}},
		"second GPC zero clock": {sample: sample{
			GPCFrequencies: []*float64{nil, measurement(0)},
		}, want: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) { assert.Equal(t, test.want, test.sample.hasReadings()) })
	}
}

func measurement(value float64) *float64 { return &value }
