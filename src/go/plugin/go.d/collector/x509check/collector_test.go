// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"context"
	"crypto/x509"
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"

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
		assert.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Charts(t *testing.T) {
	collr := New()
	collr.Source = "https://example.com"
	require.NoError(t, collr.Init(context.Background()))
	assert.NotNil(t, collr.Charts())
}

func TestCollector_Init(t *testing.T) {
	const (
		file = iota
		net
		smtp
	)
	tests := map[string]struct {
		config       Config
		providerType int
		err          bool
	}{
		"ok from net https": {
			config:       Config{Source: "https://example.org"},
			providerType: net,
		},
		"ok from net tcp": {
			config:       Config{Source: "tcp://example.org"},
			providerType: net,
		},
		"ok from file": {
			config:       Config{Source: "file:///home/me/cert.pem"},
			providerType: file,
		},
		"ok from smtp": {
			config:       Config{Source: "smtp://smtp.my_mail.org:587"},
			providerType: smtp,
		},
		"empty source": {
			config: Config{Source: ""},
			err:    true},
		"unknown provider": {
			config: Config{Source: "http://example.org"},
			err:    true,
		},
		"nonexistent TLSCA": {
			config: Config{Source: "https://example.org", TLSConfig: tlscfg.TLSConfig{TLSCA: "testdata/tls"}},
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
				switch test.providerType {
				case file:
					_, typeOK = collr.prov.(*fromFile)
				case net:
					_, typeOK = collr.prov.(*fromNet)
				case smtp:
					_, typeOK = collr.prov.(*fromSMTP)
				}

				assert.True(t, typeOK)
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	collr := New()
	collr.prov = &mockProvider{certs: []*x509.Certificate{{}}}

	assert.NoError(t, collr.Check(context.Background()))
}

func TestCollector_Check_ReturnsFalseOnProviderError(t *testing.T) {
	collr := New()
	collr.prov = &mockProvider{err: true}

	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_Collect(t *testing.T) {
	collr := New()
	collr.Source = "https://example.com"
	require.NoError(t, collr.Init(context.Background()))
	collr.prov = &mockProvider{certs: []*x509.Certificate{{}}}

	mx := collr.Collect(context.Background())

	assert.NotZero(t, mx)
	module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestCollector_Collect_ReturnsNilOnProviderError(t *testing.T) {
	collr := New()
	collr.prov = &mockProvider{err: true}

	assert.Nil(t, collr.Collect(context.Background()))
}

func TestCollector_Collect_ReturnsNilOnZeroCertificates(t *testing.T) {
	collr := New()
	collr.prov = &mockProvider{certs: []*x509.Certificate{}}
	mx := collr.Collect(context.Background())

	assert.Nil(t, mx)
}

type mockProvider struct {
	certs []*x509.Certificate
	err   bool
}

func (m mockProvider) certificates() ([]*x509.Certificate, error) {
	if m.err {
		return nil, errors.New("mock certificates error")
	}
	return m.certs, nil
}
