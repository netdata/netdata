// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
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

func TestX509Check_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &X509Check{}, dataConfigJSON, dataConfigYAML)
}

func TestX509Check_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestX509Check_Charts(t *testing.T) {
	x509Check := New()
	x509Check.Source = "https://example.com"
	require.NoError(t, x509Check.Init())
	assert.NotNil(t, x509Check.Charts())
}

func TestX509Check_Init(t *testing.T) {
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
			x509Check := New()
			x509Check.Config = test.config

			if test.err {
				assert.Error(t, x509Check.Init())
			} else {
				require.NoError(t, x509Check.Init())

				var typeOK bool
				switch test.providerType {
				case file:
					_, typeOK = x509Check.prov.(*fromFile)
				case net:
					_, typeOK = x509Check.prov.(*fromNet)
				case smtp:
					_, typeOK = x509Check.prov.(*fromSMTP)
				}

				assert.True(t, typeOK)
			}
		})
	}
}

func TestX509Check_Check(t *testing.T) {
	x509Check := New()
	x509Check.prov = &mockProvider{certs: []*x509.Certificate{{}}}

	assert.NoError(t, x509Check.Check())
}

func TestX509Check_Check_ReturnsFalseOnProviderError(t *testing.T) {
	x509Check := New()
	x509Check.prov = &mockProvider{err: true}

	assert.Error(t, x509Check.Check())
}

func TestX509Check_Collect(t *testing.T) {
	x509Check := New()
	x509Check.Source = "https://example.com"
	require.NoError(t, x509Check.Init())
	x509Check.prov = &mockProvider{certs: []*x509.Certificate{{}}}

	mx := x509Check.Collect()

	assert.NotZero(t, mx)
	module.TestMetricsHasAllChartsDims(t, x509Check.Charts(), mx)
}

func TestX509Check_Collect_ReturnsNilOnProviderError(t *testing.T) {
	x509Check := New()
	x509Check.prov = &mockProvider{err: true}

	assert.Nil(t, x509Check.Collect())
}

func TestX509Check_Collect_ReturnsNilOnZeroCertificates(t *testing.T) {
	x509Check := New()
	x509Check.prov = &mockProvider{certs: []*x509.Certificate{}}
	mx := x509Check.Collect()

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
