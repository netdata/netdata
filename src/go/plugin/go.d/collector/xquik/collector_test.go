// SPDX-License-Identifier: GPL-3.0-or-later

package xquik

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		setup   func(*Collector)
		wantErr string
	}{
		"success": {
			setup: setValidCollectorConfig,
		},
		"missing user": {
			setup:   func(c *Collector) { c.APIKey = "secret" },
			wantErr: "'user' must be",
		},
		"missing API key": {
			setup:   func(c *Collector) { c.User = "netdata" },
			wantErr: "'api_key' is required",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			tc.setup(c)
			err := c.Init(context.Background())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, c.client)
		})
	}
}

func TestCollector_Check(t *testing.T) {
	profileErr := errors.New("profile failed")
	tests := map[string]struct {
		client  profileClient
		wantErr string
	}{
		"success": {
			client: &fakeProfileClient{result: completeProfile()},
		},
		"profile failure": {
			client:  &fakeProfileClient{err: profileErr},
			wantErr: "check Xquik profile: profile failed",
		},
		"before init": {
			wantErr: "Xquik client is not initialized",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.User = "netdata"
			c.client = tc.client

			err := c.Check(context.Background())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		result     profile
		err        error
		wantErr    string
		wantValues bool
	}{
		"complete profile": {
			result:     completeProfile(),
			wantValues: true,
		},
		"optional fields absent": {
			result: profile{ID: "783214", Username: "netdata", Name: "Netdata"},
		},
		"profile failure": {
			err:     errors.New("profile failed"),
			wantErr: "collect Xquik profile: profile failed",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.User = "netdata"
			c.client = &fakeProfileClient{result: tc.result, err: tc.err}
			cc := mustCycleController(t, c.MetricStore())
			cc.BeginCycle()
			err := c.Collect(context.Background())
			if tc.wantErr != "" {
				cc.AbortCycle()
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NoError(t, cc.CommitCycleSuccess())

			labels := metrix.Labels{"user_id": "783214", "username": "netdata", "name": "Netdata"}
			if !tc.wantValues {
				_, ok := c.MetricStore().Read().Value("followers", labels)
				require.False(t, ok)
				return
			}

			assertMetricValue(t, c.MetricStore().Read(), "followers", labels, 24500)
			assertMetricValue(t, c.MetricStore().Read(), "following", labels, 175)
			assertMetricValue(t, c.MetricStore().Read(), "statuses_total", labels, 8200)

			meta, ok := c.MetricStore().Read().SeriesMeta("statuses_total", labels)
			require.True(t, ok)
			require.Equal(t, metrix.MetricKindCounter, meta.Kind)

			state, ok := c.MetricStore().Read().StateSet("verification_status", labels)
			require.True(t, ok)
			require.Equal(t, map[string]bool{"verified": true, "unverified": false}, state.States)

			flat := c.MetricStore().Read(metrix.ReadFlatten())
			verifiedLabels := cloneLabels(labels)
			verifiedLabels["verification_status"] = "verified"
			assertMetricValue(t, flat, "verification_status", verifiedLabels, 1)
			unverifiedLabels := cloneLabels(labels)
			unverifiedLabels["verification_status"] = "unverified"
			assertMetricValue(t, flat, "verification_status", unverifiedLabels, 0)

			collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
		})
	}
}

func TestCollector_CollectForwardsContext(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "collect")
	fake := &fakeProfileClient{result: completeProfile()}
	c := New()
	c.User = "netdata"
	c.client = fake
	cc := mustCycleController(t, c.MetricStore())
	cc.BeginCycle()
	require.NoError(t, c.Collect(ctx))
	require.NoError(t, cc.CommitCycleSuccess())

	require.Equal(t, "collect", fake.ctx.Value(contextKey{}))
	require.Equal(t, "netdata", fake.user)
}

func TestCollector_CollectBeforeInit(t *testing.T) {
	require.ErrorContains(t, New().Collect(context.Background()), "Xquik client is not initialized")
}

func TestCollector_Cleanup(t *testing.T) {
	c := New()
	require.NotPanics(t, func() {
		c.Cleanup(context.Background())
		c.Cleanup(context.Background())
	})

	setValidCollectorConfig(c)
	require.NoError(t, c.Init(context.Background()))
	require.NotPanics(t, func() {
		c.Cleanup(context.Background())
		c.Cleanup(context.Background())
	})
}

func TestCollector_ChartTemplateYAML(t *testing.T) {
	templateYAML := New().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, templateYAML)

	spec, err := charttpl.DecodeYAML([]byte(templateYAML))
	require.NoError(t, err)
	require.NoError(t, spec.Validate())
	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
}

func setValidCollectorConfig(c *Collector) {
	c.User = "netdata"
	c.APIKey = "secret"
}

func completeProfile() profile {
	followers := int64(24500)
	following := int64(175)
	statuses := int64(8200)
	verified := true
	return profile{
		ID:            "783214",
		Username:      "netdata",
		Name:          "Netdata",
		Followers:     &followers,
		Following:     &following,
		StatusesCount: &statuses,
		Verified:      &verified,
	}
}

func mustCycleController(t *testing.T, store metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok, "store does not expose cycle control")
	return managed.CycleController()
}

func assertMetricValue(t *testing.T, reader metrix.Reader, name string, labels metrix.Labels, want float64) {
	t.Helper()
	got, ok := reader.Value(name, labels)
	require.Truef(t, ok, "expected metric %s labels=%v", name, labels)
	require.InDelta(t, want, got, 1e-9)
}

func cloneLabels(labels metrix.Labels) metrix.Labels {
	cloned := make(metrix.Labels, len(labels)+1)
	maps.Copy(cloned, labels)
	return cloned
}

type fakeProfileClient struct {
	result profile
	err    error
	ctx    context.Context
	user   string
}

func (c *fakeProfileClient) Profile(ctx context.Context, user string) (profile, error) {
	c.ctx = ctx
	c.user = user
	return c.result, c.err
}

var _ profileClient = (*fakeProfileClient)(nil)
