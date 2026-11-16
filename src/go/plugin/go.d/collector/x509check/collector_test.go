// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
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

func TestCollector_ConfigSchemaMatchesMetadata(t *testing.T) {
	collecttest.AssertConfigSchemaMatchesMetadataWith(t, "config_schema.json", "metadata.yaml",
		collecttest.ConfigSchemaCheck{
			Defaults: true,
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
		"revocation_status":       {"not_revoked", "revoked"},
		"revocation_check_status": checkOutcomes,
	})
	collecttest.AssertHealthAlertsTargetChartTemplate(t, health, template)
	// The metadata-unit assertion does not apply: alerts convert seconds to days, boolean to status and check
	// outcomes to a percentage of checks.
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config       func(*Config)
		wantProvider certificateProvider
		wantErr      bool
	}{
		"https": {
			config:       func(c *Config) { c.Source = "https://example.org:443" },
			wantProvider: &netProvider{},
		},
		"tcp": {
			config:       func(c *Config) { c.Source = "tcp://example.org:443" },
			wantProvider: &netProvider{},
		},
		"smtp": {
			config:       func(c *Config) { c.Source = "smtp://smtp.example.org:587" },
			wantProvider: &smtpProvider{},
		},
		"file": {
			config:       func(c *Config) { c.Source = "file:///home/me/cert.pem" },
			wantProvider: &fileProvider{},
		},
		"file ignores TLS options": {
			config: func(c *Config) {
				c.Source = "file:///home/me/cert.pem"
				c.TLSCA = "testdata/tls"
			},
			wantProvider: &fileProvider{},
		},
		"tls server name": {
			config: func(c *Config) {
				c.Source = "https://192.0.2.10:443"
				c.TLSServerName = "example.org"
			},
			wantProvider: &netProvider{},
		},
		"tls server name with smtp": {
			config: func(c *Config) {
				c.Source = "smtp://192.0.2.10:587"
				c.TLSServerName = "smtp.example.org"
			},
			wantProvider: &smtpProvider{},
		},
		"tls server name with file source": {
			config: func(c *Config) {
				c.Source = "file:///home/me/cert.pem"
				c.TLSServerName = "example.org"
			},
			wantErr: true,
		},
		"empty source": {
			config:  func(c *Config) { c.Source = "" },
			wantErr: true,
		},
		"zero timeout": {
			config: func(c *Config) {
				c.Source = "https://example.org:443"
				c.Timeout = 0
			},
			wantErr: true,
		},
		"unsupported scheme": {
			config:  func(c *Config) { c.Source = "http://example.org" },
			wantErr: true,
		},
		"udp scheme": {
			config:  func(c *Config) { c.Source = "udp://example.org:443" },
			wantErr: true,
		},
		"nonexistent TLS CA": {
			config: func(c *Config) {
				c.Source = "https://example.org:443"
				c.TLSCA = "testdata/tls"
			},
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			test.config(&c.Config)

			if test.wantErr {
				assert.Error(t, c.Init(t.Context()))
				return
			}
			require.NoError(t, c.Init(t.Context()))
			assert.IsType(t, test.wantProvider, c.provider)
		})
	}
}

func TestCollector_Check(t *testing.T) {
	for name, test := range map[string]struct {
		provider *mockProvider
		wantErr  bool
	}{
		"certificate": {
			provider: &mockProvider{
				certs: []*x509.Certificate{{}},
			},
		},
		"provider error": {
			provider: &mockProvider{
				err: true,
			},
			wantErr: true,
		},
		"empty chain": {
			provider: &mockProvider{},
			wantErr:  true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.provider = test.provider
			c.CheckRevocation = true
			c.revocationStatus = func(context.Context, *x509.Certificate, []*x509.Certificate, time.Time) (bool, error) {
				t.Fatal("Check must not perform optional revocation I/O")
				return false, nil
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
		fullChain       bool
		checkRevocation bool
		revoked         bool
		revocationErr   error
		want            map[string]float64
	}{
		"leaf only": {
			want: map[string]float64{"expiry/0/": 3600},
		},
		"full chain": {
			fullChain: true,
			want:      map[string]float64{"expiry/0/": 3600, "expiry/1/": -60},
		},
		"not revoked": {
			checkRevocation: true,
			want: withCheckStatus(checkSuccess, map[string]float64{
				"expiry/0/":                       3600,
				"revocation_status/0/not_revoked": 1,
				"revocation_status/0/revoked":     0,
			}),
		},
		"revoked leaf in chain": {
			fullChain:       true,
			checkRevocation: true,
			revoked:         true,
			want: withCheckStatus(checkSuccess, map[string]float64{
				"expiry/0/":                       3600,
				"expiry/1/":                       -60,
				"revocation_status/0/not_revoked": 0,
				"revocation_status/0/revoked":     1,
			}),
		},
		"undetermined revocation": {
			checkRevocation: true,
			revocationErr:   failCheck(checkIssuerNotFound, errors.New("issuer certificate not found")),
			want:            withCheckStatus(checkIssuerNotFound, map[string]float64{"expiry/0/": 3600}),
		},
		"undetermined revocation without an outcome": {
			checkRevocation: true,
			revocationErr:   errors.New("undetermined"),
			want:            map[string]float64{"expiry/0/": 3600},
		},
	} {
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			certs := []*x509.Certificate{
				{NotAfter: now.Add(time.Hour + 900*time.Millisecond)},
				{NotAfter: now.Add(-time.Minute - 900*time.Millisecond)},
			}
			// A root that TLS verification found and the source did not send.
			root := &x509.Certificate{
				NotAfter: now.Add(24 * time.Hour),
			}
			c := New()
			c.Source = "https://example.org:443"
			c.CheckFullChain = test.fullChain
			c.CheckRevocation = test.checkRevocation
			c.now = func() time.Time { return now }
			c.provider = &mockProvider{
				certs:    certs,
				verified: [][]*x509.Certificate{{certs[0], root}},
			}
			calls := 0
			c.revocationStatus = func(
				_ context.Context,
				cert *x509.Certificate,
				issuers []*x509.Certificate,
				at time.Time,
			) (bool, error) {
				calls++
				assert.Same(t, certs[0], cert)
				// The rest of the chain, also when only the server certificate is charted, and the verified issuer.
				assert.Equal(t, []*x509.Certificate{certs[1], root}, issuers)
				assert.Equal(t, now, at)
				return test.revoked, test.revocationErr
			}

			require.NoError(t, collectCycle(t, c))

			assert.Equal(t, test.want, readMetrics(c))
			if test.checkRevocation {
				assert.Equal(t, 1, calls)
			} else {
				assert.Zero(t, calls)
			}
			exp := collecttest.ChartCoverageExpectation{}
			if !test.checkRevocation || test.revocationErr != nil {
				exp.ExcludeContextPatterns = append(exp.ExcludeContextPatterns, "x509check.revocation_status")
			}
			if !test.checkRevocation || checkOutcome(test.revocationErr) == "" {
				exp.ExcludeContextPatterns = append(exp.ExcludeContextPatterns, "x509check.revocation_check_status")
			}
			collecttest.AssertChartCoverage(t, c, exp)
		})
	}
}

func TestCollector_CollectTakesRevocationIssuerFromVerifiedChain(t *testing.T) {
	for scheme, serve := range map[string]func(*testing.T, *tls.Config) string{
		"https": serveTLS,
		"smtp":  serveSMTP,
	} {
		t.Run(scheme, func(t *testing.T) {
			pki := newTestPKI(t, testPKIEndpoints{
				issuers: []testPKIHandler{unexpected},
				crls:    []testPKIHandler{serveCRL(true)},
			})
			// The server sends only its certificate, which a root the job trusts through tls_ca issued.
			server := pki.issueServerCertificate(t, pki.leaf.SerialNumber, pki.ca, pki.caKey,
				func(template *x509.Certificate) { template.DNSNames = []string{"localhost"} })
			_, port, err := net.SplitHostPort(serve(t, serverConfig(server)))
			require.NoError(t, err)
			roots := x509.NewCertPool()
			roots.AddCert(pki.ca)
			c := New()
			c.Source = scheme + "://localhost:" + port
			c.CheckFullChain = true
			c.CheckRevocation = true
			c.now = func() time.Time { return pki.now }
			require.NoError(t, c.Init(t.Context()))
			// The CA option reads through the privileged file helper, which tests cannot run, so trust the root
			// directly.
			switch p := c.provider.(type) {
			case *netProvider:
				p.tlsConfig.RootCAs = roots
			case *smtpProvider:
				p.tlsConfig.RootCAs = roots
			}

			require.NoError(t, collectCycle(t, c))

			// The root decides the revocation status but is not charted: the source did not send it.
			assert.Equal(t, withCheckStatus(checkSuccess, map[string]float64{
				"expiry/0/":                       server.Leaf.NotAfter.Sub(pki.now).Seconds(),
				"revocation_status/0/not_revoked": 0,
				"revocation_status/0/revoked":     1,
			}), readMetrics(c))
		})
	}
}

func TestCollector_CollectHonorsCancellationDuringRevocationCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pki := newTestPKI(t, testPKIEndpoints{
		crls: []testPKIHandler{cancelAndStall(cancel)},
	})
	c := New()
	c.Source = "https://example.org:443"
	c.CheckRevocation = true
	// The presented CA signed the server certificate, so the CRL is requested.
	c.provider = &mockProvider{
		certs: []*x509.Certificate{pki.leaf, pki.ca},
	}
	store, ok := metrix.AsCycleManagedStore(c.MetricStore())
	require.True(t, ok)
	cc := store.CycleController()
	cc.BeginCycle()
	defer cc.AbortCycle()

	done := make(chan error, 1)
	go func() { done <- c.Collect(ctx) }()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("Collect is still waiting for a revocation check after cancellation")
	}
	assert.Equal(t, int64(1), pki.hits["crl"].Load())
	assert.Eventually(t, func() bool { return pki.inFlight.Load() == 0 }, 5*time.Second, 10*time.Millisecond,
		"the cancelled CRL request must end, not run on in the background")
}

func TestCollector_CollectAbort(t *testing.T) {
	for name, provider := range map[string]*mockProvider{
		"provider error": {err: true},
		"empty chain":    {},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.provider = &mockProvider{
				certs: []*x509.Certificate{{NotAfter: time.Now().Add(time.Hour)}},
			}
			require.NoError(t, collectCycle(t, c))
			before := readMetrics(c)

			c.provider = provider
			require.Error(t, collectCycle(t, c))

			assert.Equal(t, before, readMetrics(c, metrix.ReadRaw()), "abort must retain committed values")
			assert.Empty(t, readMetrics(c), "failed observations must not appear fresh")
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	c := New()
	assert.NotPanics(t, func() {
		c.Cleanup(context.Background())
		c.Cleanup(context.Background())
	})
}

// collectCycle runs Collect inside a metric-store cycle, committing on success and aborting on error.
func collectCycle(tb testing.TB, c *Collector) error {
	tb.Helper()
	store, ok := metrix.AsCycleManagedStore(c.MetricStore())
	require.True(tb, ok)
	cc := store.CycleController()
	cc.BeginCycle()
	if err := c.Collect(tb.Context()); err != nil {
		cc.AbortCycle()
		return err
	}
	require.NoError(tb, cc.CommitCycleSuccess())
	return nil
}

// readMetrics returns the stored values keyed by "metric/depth/state".
func readMetrics(c *Collector, opts ...metrix.ReadOption) map[string]float64 {
	values := make(map[string]float64)
	c.MetricStore().
		Read(append(opts, metrix.ReadFlatten())...).
		ForEachSeries(func(name string, labels metrix.LabelView, value float64) {
			depth, _ := labels.Get("depth")
			// A stateset sample carries its state in a label named after the metric.
			state, _ := labels.Get(name)
			values[name+"/"+depth+"/"+state] = value
		})
	return values
}

// withCheckStatus adds to values the revocation check status of the server certificate, as readMetrics keys it, with
// outcome as the active state.
func withCheckStatus(outcome string, values map[string]float64) map[string]float64 {
	for _, o := range checkOutcomes {
		values["revocation_check_status/0/"+o] = 0
	}
	values["revocation_check_status/0/"+outcome] = 1
	return values
}

type mockProvider struct {
	certs    []*x509.Certificate
	verified [][]*x509.Certificate
	err      bool
}

func (m *mockProvider) certificates(context.Context) (sourceCertificates, error) {
	if m.err {
		return sourceCertificates{}, errors.New("mock certificates error")
	}
	return sourceCertificates{
		chain:    m.certs,
		verified: m.verified,
	}, nil
}
