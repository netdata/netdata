// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		setup   func(*Collector, *fakeAPIClient)
		wantErr string
		check   func(*testing.T, *Collector, *fakeAPIClient)
	}{
		"success with injected client": {
			check: func(t *testing.T, c *Collector, _ *fakeAPIClient) {
				require.NotNil(t, c.client)
				require.Equal(t, defaultEntitySelector, c.SiteSelector)
				require.NotNil(t, c.siteMatcher)
				require.True(t, c.siteMatcher.MatchString("any-site"))
				require.NotNil(t, c.bgp.bySite)
			},
		},
		"missing account id": {
			setup: func(c *Collector, _ *fakeAPIClient) {
				c.AccountID = ""
			},
			wantErr: "'account_id' is required",
		},
		"missing api key": {
			setup: func(c *Collector, _ *fakeAPIClient) {
				c.APIKey = ""
			},
			wantErr: "'api_key' is required",
		},
		"client factory error": {
			setup: func(c *Collector, _ *fakeAPIClient) {
				c.client = nil
				c.newClient = func(Config, *http.Client) (apiClient, error) {
					return nil, errors.New("factory failed")
				}
			},
			wantErr: "init Cato client: factory failed",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, fake := newTestCollector()
			if tc.setup != nil {
				tc.setup(c, fake)
			}

			err := c.Init(context.Background())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, c, fake)
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		setup     func(*Collector, *fakeAPIClient)
		skipInit  bool
		wantErr   string
		wantErrIs error
		wantClass string
		check     func(*testing.T, *Collector, *fakeAPIClient)
	}{
		"success uses cheap probe only": {
			setup: func(_ *Collector, fake *fakeAPIClient) {
				fake.metricsErrSites = map[string]error{"1001": errors.New("metrics should not be called")}
				fake.bgpErrSites = map[string]error{"1001": errors.New("BGP should not be called")}
			},
			check: func(t *testing.T, c *Collector, fake *fakeAPIClient) {
				require.Equal(t, 1, fake.probeCalls)
				require.Zero(t, fake.lookupCalls)
				require.Zero(t, fake.snapshotCalls)
				require.Zero(t, fake.metricsCalls)
				require.Zero(t, fake.bgpCalls)
				require.Empty(t, c.discovery.siteIDs)
				require.Empty(t, c.bgp.bySite)
				_, ok := c.topology.CurrentTopology()
				require.False(t, ok)
			},
		},
		"does not populate BGP cache": {
			check: func(t *testing.T, c *Collector, _ *fakeAPIClient) {
				require.Empty(t, c.bgp.bySite)
				require.True(t, c.bgp.nextRefresh.IsZero())
			},
		},
		"preserves probe error cause": {
			setup: func(_ *Collector, fake *fakeAPIClient) {
				fake.probeErr = context.Canceled
			},
			wantErr:   "Cato API probe failed",
			wantErrIs: context.Canceled,
			wantClass: "canceled",
		},
		"fails before Init": {
			setup: func(c *Collector, _ *fakeAPIClient) {
				c.client = nil
			},
			skipInit: true,
			wantErr:  "Cato client is not initialized",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, fake := newTestCollector()
			if tc.setup != nil {
				tc.setup(c, fake)
			}
			if !tc.skipInit {
				initCollector(t, c)
			}

			err := c.Check(context.Background())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				if tc.wantErrIs != nil {
					require.ErrorIs(t, err, tc.wantErrIs)
				}
				if tc.wantClass != "" {
					require.Equal(t, tc.wantClass, classifyCatoError(err))
				}
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, c, fake)
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	router := &cleanupHandler{}
	c := New()
	c.funcRouter = router

	c.Cleanup(context.Background())

	require.True(t, router.called)
}

func TestCollector_MetricStore(t *testing.T) {
	require.NotNil(t, New().MetricStore())
}

type cleanupHandler struct {
	called bool
}

func (h *cleanupHandler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, errors.New("not implemented")
}

func (h *cleanupHandler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return nil
}

func (h *cleanupHandler) Cleanup(context.Context) {
	h.called = true
}
