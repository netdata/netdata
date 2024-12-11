// SPDX-License-Identifier: GPL-3.0-or-later

package whoisquery

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

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Cleanup(t *testing.T) {
	New().Cleanup(context.Background())
}

func TestCollector_Charts(t *testing.T) {
	collr := New()
	collr.Source = "example.com"
	require.NoError(t, collr.Init(context.Background()))

	assert.NotNil(t, collr.Charts())
}

func TestCollector_Init(t *testing.T) {
	const net = iota
	tests := map[string]struct {
		config       Config
		providerType int
		err          bool
	}{
		"ok from net": {
			config:       Config{Source: "example.org"},
			providerType: net,
		},
		"empty source": {
			config: Config{Source: ""},
			err:    true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.err {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				require.NoError(t, collr.Init(context.Background()))

				var typeOK bool
				if test.providerType == net {
					_, typeOK = collr.prov.(*whoisClient)
				}

				assert.True(t, typeOK)
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	collr := New()
	collr.prov = &mockProvider{remTime: 12345.678}

	assert.NoError(t, collr.Check(context.Background()))
}

func TestCollector_Check_ReturnsFalseOnProviderError(t *testing.T) {
	collr := New()
	collr.prov = &mockProvider{err: true}

	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_Collect(t *testing.T) {
	collr := New()
	collr.Source = "example.com"
	require.NoError(t, collr.Init(context.Background()))
	collr.prov = &mockProvider{remTime: 12345}

	mx := collr.Collect(context.Background())

	expected := map[string]int64{
		"expiry":                         12345,
		"days_until_expiration_warning":  30,
		"days_until_expiration_critical": 15,
	}

	assert.NotZero(t, mx)
	assert.Equal(t, expected, mx)
	module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestCollector_Collect_ReturnsNilOnProviderError(t *testing.T) {
	collr := New()
	collr.Source = "example.com"
	require.NoError(t, collr.Init(context.Background()))
	collr.prov = &mockProvider{err: true}

	assert.Nil(t, collr.Collect(context.Background()))
}

type mockProvider struct {
	remTime float64
	err     bool
}

func (m mockProvider) remainingTime() (float64, error) {
	if m.err {
		return 0, errors.New("mock remaining time error")
	}
	return m.remTime, nil
}
