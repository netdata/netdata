// SPDX-License-Identifier: GPL-3.0-or-later

package whoisquery

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

func TestWhoisQuery_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &WhoisQuery{}, dataConfigJSON, dataConfigYAML)
}

func TestWhoisQuery_Cleanup(t *testing.T) {
	New().Cleanup()
}

func TestWhoisQuery_Charts(t *testing.T) {
	whoisquery := New()
	whoisquery.Source = "example.com"
	require.NoError(t, whoisquery.Init())

	assert.NotNil(t, whoisquery.Charts())
}

func TestWhoisQuery_Init(t *testing.T) {
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
			whoisquery := New()
			whoisquery.Config = test.config

			if test.err {
				assert.Error(t, whoisquery.Init())
			} else {
				require.NoError(t, whoisquery.Init())

				var typeOK bool
				if test.providerType == net {
					_, typeOK = whoisquery.prov.(*whoisClient)
				}

				assert.True(t, typeOK)
			}
		})
	}
}

func TestWhoisQuery_Check(t *testing.T) {
	whoisquery := New()
	whoisquery.prov = &mockProvider{remTime: 12345.678}

	assert.NoError(t, whoisquery.Check())
}

func TestWhoisQuery_Check_ReturnsFalseOnProviderError(t *testing.T) {
	whoisquery := New()
	whoisquery.prov = &mockProvider{err: true}

	assert.Error(t, whoisquery.Check())
}

func TestWhoisQuery_Collect(t *testing.T) {
	whoisquery := New()
	whoisquery.Source = "example.com"
	require.NoError(t, whoisquery.Init())
	whoisquery.prov = &mockProvider{remTime: 12345}

	mx := whoisquery.Collect()

	expected := map[string]int64{
		"expiry":                         12345,
		"days_until_expiration_warning":  30,
		"days_until_expiration_critical": 15,
	}

	assert.NotZero(t, mx)
	assert.Equal(t, expected, mx)
	module.TestMetricsHasAllChartsDims(t, whoisquery.Charts(), mx)
}

func TestWhoisQuery_Collect_ReturnsNilOnProviderError(t *testing.T) {
	whoisquery := New()
	whoisquery.Source = "example.com"
	require.NoError(t, whoisquery.Init())
	whoisquery.prov = &mockProvider{err: true}

	assert.Nil(t, whoisquery.Collect())
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
