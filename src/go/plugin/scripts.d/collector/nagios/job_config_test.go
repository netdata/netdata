// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/stretchr/testify/assert"
)

func TestJobConfigSetDefaults(t *testing.T) {
	cfg := JobConfig{Name: "sample", Plugin: "/usr/lib/nagios/plugins/check_ping"}
	cfg.setDefaults()

	assert.Equal(t, confopt.Duration(5*time.Second), cfg.Timeout)
	assert.NotZero(t, cfg.CheckInterval)
	assert.NotZero(t, cfg.RetryInterval)
	assert.NotZero(t, cfg.MaxCheckAttempts)
	assert.NotEmpty(t, cfg.CheckPeriod)
}

func TestJobConfigValidate(t *testing.T) {
	tests := map[string]struct {
		cfg     JobConfig
		wantErr bool
	}{
		"valid": {
			cfg: JobConfig{Name: "sample", Plugin: "/bin/true"},
		},
		"arg_values over limit": {
			cfg: func() JobConfig {
				cfg := JobConfig{Name: "sample", Plugin: "/bin/true"}
				for range maxArgMacros + 1 {
					cfg.ArgValues = append(cfg.ArgValues, "value")
				}
				return cfg
			}(),
			wantErr: true,
		},
		"relative plugin path": {
			cfg:     JobConfig{Name: "sample", Plugin: "check_ping"},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.cfg.setDefaults()

			if tc.wantErr {
				assert.Error(t, tc.cfg.validate())
				return
			}

			assert.NoError(t, tc.cfg.validate())
		})
	}
}

func TestJobConfigNormalizedCheckName(t *testing.T) {
	tests := map[string]struct {
		cfg           JobConfig
		wantCheckName string
	}{
		"derives from plugin basename when omitted": {
			cfg:           JobConfig{Name: "sample", Plugin: "/usr/lib/nagios/plugins/check_ping.pl"},
			wantCheckName: "check_ping",
		},
		"sanitizes explicit check name": {
			cfg:           JobConfig{Name: "sample", Plugin: "/bin/bash", CheckName: "Database Health Check"},
			wantCheckName: "database_health_check",
		},
		"explicit check name does not strip suffixes as file extensions": {
			cfg:           JobConfig{Name: "sample", Plugin: "/bin/bash", CheckName: "check_service.ps1"},
			wantCheckName: "check_service_ps1",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := tc.cfg.normalized()
			assert.NoError(t, err)
			assert.Equal(t, tc.wantCheckName, got.CheckName)
		})
	}
}
