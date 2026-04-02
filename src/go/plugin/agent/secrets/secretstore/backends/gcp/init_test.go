// SPDX-License-Identifier: GPL-3.0-or-later

package gcp

import (
	"context"
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
			}

			err := s.init(context.Background())
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantTimeout, s.runtime.apiClient.Timeout)
			assert.Equal(t, tc.wantTimeout, s.runtime.metadataClient.Timeout)
			assert.Equal(t, confopt.Duration(tc.wantTimeout), s.Config.Timeout)
		})
	}
}

func TestInitBuildsStoreScopedRuntime(t *testing.T) {
	creator := New()

	first, ok := creator.Create().(*store)
	require.True(t, ok)
	second, ok := creator.Create().(*store)
	require.True(t, ok)

	assert.Equal(t, defaultTimeout, first.Config.Timeout)
	first.Mode = "metadata"
	second.Mode = "metadata"

	require.NoError(t, first.init(context.Background()))
	require.NoError(t, second.init(context.Background()))

	require.NotNil(t, first.runtime)
	require.NotNil(t, second.runtime)
	assert.NotSame(t, first.runtime, second.runtime)
	assert.NotSame(t, first.runtime.apiClient, second.runtime.apiClient)
	assert.NotSame(t, first.runtime.metadataClient, second.runtime.metadataClient)
	assert.Equal(t, defaultTimeout.Duration(), first.runtime.apiClient.Timeout)
	assert.Equal(t, defaultTimeout.Duration(), first.runtime.metadataClient.Timeout)
}
