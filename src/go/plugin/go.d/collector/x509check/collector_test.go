// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"context"
	"crypto/x509"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
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
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Cleanup(t *testing.T) {
	c := New()
	assert.NotPanics(t, func() {
		c.Cleanup(context.Background())
		c.Cleanup(context.Background())
	})
}

func TestCollector_ChartTemplate(t *testing.T) {
	template := New().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, template)
	spec, err := charttpl.DecodeYAML([]byte(template))
	require.NoError(t, err)
	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	health, err := os.ReadFile("../../../../../health/health.d/x509check.conf")
	require.NoError(t, err)
	collecttest.AssertMetadataDocumentsChartTemplate(t, metadata, template, map[string][]string{
		"revocation_status": {"not_revoked", "revoked"},
	})
	collecttest.AssertHealthAlertsTargetChartTemplate(t, health, template)
	// The metadata-unit assertion does not apply: alerts convert seconds to days and boolean to status.
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
			config: Config{
				Source: "https://example.org",
			},
			providerType: net,
		},
		"ok from net tcp": {
			config: Config{
				Source: "tcp://example.org",
			},
			providerType: net,
		},
		"ok from file": {
			config: Config{
				Source: "file:///home/me/cert.pem",
			},
			providerType: file,
		},
		"ok from smtp": {
			config: Config{
				Source: "smtp://smtp.my_mail.org:587",
			},
			providerType: smtp,
		},
		"empty source": {
			config: Config{
				Source: "",
			},
			err: true},
		"unknown provider": {
			config: Config{
				Source: "http://example.org",
			},
			err: true,
		},
		"nonexistent TLSCA": {
			config: Config{
				Source: "https://example.org",
				TLSConfig: tlscfg.TLSConfig{
					TLSCA: "testdata/tls",
				},
			},
			err: true,
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
	for name, test := range map[string]struct {
		provider *mockProvider
		wantErr  bool
	}{
		"certificate": {provider: &mockProvider{
			certs: []*x509.Certificate{{}},
		}},
		"provider error": {provider: &mockProvider{
			err: true,
		}, wantErr: true},
		"empty chain": {provider: &mockProvider{}, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.prov = test.provider
			c.CheckRevocation = true
			c.verifyCertificate = func(*x509.Certificate) (bool, bool, error) {
				t.Fatal("Check must not perform optional revocation I/O")
				return false, false, nil
			}
			err := c.Check(t.Context())
			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Empty(t, readMetrics(c))
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	for name, test := range map[string]struct {
		fullChain, checkRevocation bool
		revoked, verified          bool
		verifyErr                  error
		want                       map[string]float64
	}{
		"leaf only":  {want: map[string]float64{"expiry/0/": 3600}},
		"full chain": {fullChain: true, want: map[string]float64{"expiry/0/": 3600, "expiry/1/": -60}},
		"not revoked": {checkRevocation: true, verified: true, want: map[string]float64{
			"expiry/0/": 3600, "revocation_status/0/not_revoked": 1, "revocation_status/0/revoked": 0,
		}},
		"revoked leaf in chain": {fullChain: true, checkRevocation: true, revoked: true, verified: true,
			want: map[string]float64{
				"expiry/0/": 3600, "expiry/1/": -60, "revocation_status/0/not_revoked": 0, "revocation_status/0/revoked": 1,
			}},
		"unverified": {checkRevocation: true, want: map[string]float64{"expiry/0/": 3600}},
		"verification error": {checkRevocation: true, verified: true, verifyErr: errors.New("verification failed"),
			want: map[string]float64{"expiry/0/": 3600}},
	} {
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			certs := []*x509.Certificate{
				{NotAfter: now.Add(time.Hour + 900*time.Millisecond)},
				{NotAfter: now.Add(-time.Minute - 900*time.Millisecond)},
			}
			c := New()
			c.Source = "https://example.org:443"
			c.now = func() time.Time { return now }
			c.prov = &mockProvider{
				certs: certs,
			}
			c.CheckFullChain, c.CheckRevocation = test.fullChain, test.checkRevocation
			calls := 0
			c.verifyCertificate = func(cert *x509.Certificate) (bool, bool, error) {
				calls++
				assert.Same(t, certs[0], cert)
				return test.revoked, test.verified, test.verifyErr
			}
			require.NoError(t, collectCycle(t, c))
			assert.Equal(t, test.want, readMetrics(c))
			if test.checkRevocation {
				assert.Equal(t, 1, calls)
			} else {
				assert.Zero(t, calls)
			}
			exp := collecttest.ChartCoverageExpectation{}
			if !test.checkRevocation || !test.verified || test.verifyErr != nil {
				exp.ExcludeContextPatterns = []string{"x509check.revocation_status"}
			}
			collecttest.AssertChartCoverage(t, c, exp)
		})
	}
}

func TestCollector_CollectAbort(t *testing.T) {
	for name, provider := range map[string]*mockProvider{
		"provider error": {err: true},
		"empty chain":    {},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.prov = &mockProvider{
				certs: []*x509.Certificate{{NotAfter: time.Now().Add(time.Hour)}},
			}
			require.NoError(t, collectCycle(t, c))
			before := readMetrics(c)
			c.prov = provider
			require.Error(t, collectCycle(t, c))
			assert.Equal(t, before, readMetrics(c, metrix.ReadRaw()), "abort must retain committed values")
			assert.Empty(t, readMetrics(c), "failed observations must not appear fresh")
		})
	}
}

func collectCycle(t *testing.T, c *Collector) error {
	t.Helper()
	store, ok := metrix.AsCycleManagedStore(c.MetricStore())
	require.True(t, ok)
	cc := store.CycleController()
	cc.BeginCycle()
	if err := c.Collect(t.Context()); err != nil {
		cc.AbortCycle()
		return err
	}
	require.NoError(t, cc.CommitCycleSuccess())
	return nil
}

func readMetrics(c *Collector, opts ...metrix.ReadOption) map[string]float64 {
	values := make(map[string]float64)
	c.MetricStore().
		Read(append(opts, metrix.ReadFlatten())...).
		ForEachSeries(func(name string, labels metrix.LabelView, value float64) {
			depth, _ := labels.Get("depth")
			state, _ := labels.Get("revocation_status")
			values[name+"/"+depth+"/"+state] = value
		})
	return values
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
