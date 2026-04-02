// SPDX-License-Identifier: GPL-3.0-or-later

package gcp

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreInitTimeout(t *testing.T) {
	tests := map[string]struct {
		timeout         confopt.Duration
		wantTimeout     time.Duration
		wantErrContains string
	}{
		"default timeout": {
			wantTimeout: defaultTimeout.Duration(),
		},
		"configured timeout": {
			timeout:     confopt.Duration(7 * time.Second),
			wantTimeout: 7 * time.Second,
		},
		"negative timeout": {
			timeout:         confopt.Duration(-time.Second),
			wantErrContains: "timeout cannot be negative",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := &store{
				Config: Config{
					Mode:    "metadata",
					Timeout: tc.timeout,
				},
				provider: &provider{
					apiClient:      &http.Client{},
					metadataClient: &http.Client{},
				},
			}

			err := s.init(context.Background())
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantTimeout, s.provider.apiClient.Timeout)
			assert.Equal(t, tc.wantTimeout, s.provider.metadataClient.Timeout)
			assert.Equal(t, confopt.Duration(tc.wantTimeout), s.Config.Timeout)
		})
	}
}

func TestCreateReturnsStoreScopedClients(t *testing.T) {
	creator := New()

	first, ok := creator.Create().(*store)
	require.True(t, ok)
	second, ok := creator.Create().(*store)
	require.True(t, ok)

	require.NotNil(t, first.provider)
	require.NotNil(t, second.provider)
	assert.NotSame(t, first.provider, second.provider)
	assert.NotSame(t, first.provider.apiClient, second.provider.apiClient)
	assert.NotSame(t, first.provider.metadataClient, second.provider.metadataClient)
	assert.Equal(t, defaultTimeout, first.Config.Timeout)
	assert.Equal(t, defaultTimeout.Duration(), first.provider.apiClient.Timeout)
	assert.Equal(t, defaultTimeout.Duration(), first.provider.metadataClient.Timeout)
}
