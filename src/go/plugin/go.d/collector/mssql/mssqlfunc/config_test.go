// SPDX-License-Identifier: GPL-3.0-or-later

package mssqlfunc

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/stretchr/testify/assert"
)

func TestConfig_FunctionTimeouts(t *testing.T) {
	cfg := FunctionsConfig{}
	assert.Equal(t, 30*time.Second, cfg.topQueriesTimeout().value)
	assert.Equal(t, 30*time.Second, cfg.deadlockInfoTimeout().value)
	assert.Equal(t, 30*time.Second, cfg.errorInfoTimeout().value)
	cfg.TopQueries.Timeout = confopt.Duration(11 * time.Second)
	cfg.DeadlockInfo.Timeout = confopt.Duration(12 * time.Second)
	cfg.ErrorInfo.Timeout = confopt.Duration(13 * time.Second)
	assert.Equal(t, 11*time.Second, cfg.topQueriesTimeout().value)
	assert.Equal(t, 12*time.Second, cfg.deadlockInfoTimeout().value)
	assert.Equal(t, 13*time.Second, cfg.errorInfoTimeout().value)
}

func TestConfig_FunctionsDisabledDefaults(t *testing.T) {
	cfg := FunctionsConfig{}
	assert.False(t, cfg.DeadlockInfo.Disabled, "deadlock_info should be enabled by default")
	assert.False(t, cfg.ErrorInfo.Disabled, "error_info should be enabled by default")
	assert.False(t, cfg.TopQueries.Disabled, "top_queries should be enabled by default")
}

func TestConfig_ErrorInfoSessionName(t *testing.T) {
	tests := []struct {
		name string
		cfg  FunctionsConfig
		want string
	}{
		{
			name: "default session name",
			cfg:  FunctionsConfig{},
			want: "netdata_errors",
		},
		{
			name: "explicit session name",
			cfg: FunctionsConfig{
				ErrorInfo: ErrorInfoConfig{
					SessionName: "custom_errors",
				},
			},
			want: "custom_errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.errorInfoSessionName())
		})
	}
}
