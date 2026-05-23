// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_DefaultsAnalysisConfig(t *testing.T) {
	c, err := newClient(Config{
		Probe: ProbeConfig{
			Packets:  1,
			Interval: confopt.Duration(time.Millisecond),
			Timeout:  time.Second,
		},
	}, logger.NewWithWriter(nil), &fakeRunner{})
	require.NoError(t, err)

	assert.Equal(t, defaultJitterEWMASamples, c.cfg.Analysis.JitterEWMASamples)
	assert.Equal(t, defaultJitterSMAWindow, c.cfg.Analysis.JitterSMAWindow)
}

func TestNewClient_ValidatesConfig(t *testing.T) {
	tests := map[string]Config{
		"packets": {
			Probe: ProbeConfig{
				Packets:  0,
				Interval: confopt.Duration(time.Millisecond),
				Timeout:  time.Second,
			},
		},
		"interval": {
			Probe: ProbeConfig{
				Packets: 1,
				Timeout: time.Second,
			},
		},
		"timeout": {
			Probe: ProbeConfig{
				Packets:  1,
				Interval: confopt.Duration(time.Millisecond),
			},
		},
	}

	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newClient(cfg, logger.NewWithWriter(nil), &fakeRunner{})
			assert.Error(t, err)
		})
	}
}

func TestNewClient_RequiresRunner(t *testing.T) {
	_, err := newClient(Config{
		Probe: ProbeConfig{
			Packets:  1,
			Interval: confopt.Duration(time.Millisecond),
			Timeout:  time.Second,
		},
	}, logger.NewWithWriter(nil), nil)
	require.Error(t, err)
}
