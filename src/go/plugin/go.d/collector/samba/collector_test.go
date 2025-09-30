// SPDX-License-Identifier: GPL-3.0-or-later

package samba

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataSmbStatusProfile, _ = os.ReadFile("testdata/smbstatus-profile.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":       dataConfigJSON,
		"dataConfigYAML":       dataConfigYAML,
		"dataSmbStatusProfile": dataSmbStatusProfile,
	} {
		require.NotNil(t, data, name)

	}
}

func TestCollector_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success with default config": {
			wantFail: false,
			config:   New().Config,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Collector
	}{
		"not initialized exec": {
			prepare: func() *Collector {
				return New()
			},
		},
		"after check": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOk()
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOk()
				_ = collr.Collect(context.Background())
				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
		})
	}
}

func TestCollectorCharts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockSmbStatusBinary
		wantFail    bool
	}{
		"success case": {
			prepareMock: prepareMockOk,
			wantFail:    false,
		},
		"error on exec": {
			prepareMock: prepareMockErr,
			wantFail:    true,
		},
		"empty response": {
			prepareMock: prepareMockEmptyResponse,
			wantFail:    true,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
			wantFail:    true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockSmbStatusBinary
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success case": {
			prepareMock: prepareMockOk,
			wantCharts:  53 /*syscall count*/ + 8 /*syscall bytes*/ + (19 * 2), /*smb2calls count and bytes*/
			wantMetrics: map[string]int64{
				"smb2_break_count":              0,
				"smb2_break_inbytes":            0,
				"smb2_break_outbytes":           0,
				"smb2_cancel_count":             0,
				"smb2_cancel_inbytes":           0,
				"smb2_cancel_outbytes":          0,
				"smb2_close_count":              0,
				"smb2_close_inbytes":            0,
				"smb2_close_outbytes":           0,
				"smb2_create_count":             0,
				"smb2_create_inbytes":           0,
				"smb2_create_outbytes":          0,
				"smb2_find_count":               0,
				"smb2_find_inbytes":             0,
				"smb2_find_outbytes":            0,
				"smb2_flush_count":              0,
				"smb2_flush_inbytes":            0,
				"smb2_flush_outbytes":           0,
				"smb2_getinfo_count":            0,
				"smb2_getinfo_inbytes":          0,
				"smb2_getinfo_outbytes":         0,
				"smb2_ioctl_count":              0,
				"smb2_ioctl_inbytes":            0,
				"smb2_ioctl_outbytes":           0,
				"smb2_keepalive_count":          0,
				"smb2_keepalive_inbytes":        0,
				"smb2_keepalive_outbytes":       0,
				"smb2_lock_count":               0,
				"smb2_lock_inbytes":             0,
				"smb2_lock_outbytes":            0,
				"smb2_logoff_count":             0,
				"smb2_logoff_inbytes":           0,
				"smb2_logoff_outbytes":          0,
				"smb2_negprot_count":            0,
				"smb2_negprot_inbytes":          0,
				"smb2_negprot_outbytes":         0,
				"smb2_notify_count":             0,
				"smb2_notify_inbytes":           0,
				"smb2_notify_outbytes":          0,
				"smb2_read_count":               0,
				"smb2_read_inbytes":             0,
				"smb2_read_outbytes":            0,
				"smb2_sesssetup_count":          0,
				"smb2_sesssetup_inbytes":        0,
				"smb2_sesssetup_outbytes":       0,
				"smb2_setinfo_count":            0,
				"smb2_setinfo_inbytes":          0,
				"smb2_setinfo_outbytes":         0,
				"smb2_tcon_count":               0,
				"smb2_tcon_inbytes":             0,
				"smb2_tcon_outbytes":            0,
				"smb2_tdis_count":               0,
				"smb2_tdis_inbytes":             0,
				"smb2_tdis_outbytes":            0,
				"smb2_write_count":              0,
				"smb2_write_inbytes":            0,
				"smb2_write_outbytes":           0,
				"syscall_asys_fsync_bytes":      0,
				"syscall_asys_fsync_count":      0,
				"syscall_asys_getxattrat_bytes": 0,
				"syscall_asys_getxattrat_count": 0,
				"syscall_asys_pread_bytes":      0,
				"syscall_asys_pread_count":      0,
				"syscall_asys_pwrite_bytes":     0,
				"syscall_asys_pwrite_count":     0,
				"syscall_brl_cancel_count":      0,
				"syscall_brl_lock_count":        0,
				"syscall_brl_unlock_count":      0,
				"syscall_chdir_count":           0,
				"syscall_chmod_count":           0,
				"syscall_close_count":           0,
				"syscall_closedir_count":        0,
				"syscall_createfile_count":      0,
				"syscall_fallocate_count":       0,
				"syscall_fchmod_count":          0,
				"syscall_fchown_count":          0,
				"syscall_fcntl_count":           0,
				"syscall_fcntl_getlock_count":   0,
				"syscall_fcntl_lock_count":      0,
				"syscall_fdopendir_count":       0,
				"syscall_fntimes_count":         0,
				"syscall_fstat_count":           0,
				"syscall_fstatat_count":         0,
				"syscall_ftruncate_count":       0,
				"syscall_get_alloc_size_count":  0,
				"syscall_get_quota_count":       0,
				"syscall_get_sd_count":          0,
				"syscall_getwd_count":           0,
				"syscall_lchown_count":          0,
				"syscall_linkat_count":          0,
				"syscall_linux_setlease_count":  0,
				"syscall_lseek_count":           0,
				"syscall_lstat_count":           0,
				"syscall_mkdirat_count":         0,
				"syscall_mknodat_count":         0,
				"syscall_open_count":            0,
				"syscall_openat_count":          0,
				"syscall_opendir_count":         0,
				"syscall_pread_bytes":           0,
				"syscall_pread_count":           0,
				"syscall_pwrite_bytes":          0,
				"syscall_pwrite_count":          0,
				"syscall_readdir_count":         0,
				"syscall_readlinkat_count":      0,
				"syscall_realpath_count":        0,
				"syscall_recvfile_bytes":        0,
				"syscall_recvfile_count":        0,
				"syscall_renameat_count":        0,
				"syscall_rewinddir_count":       0,
				"syscall_seekdir_count":         0,
				"syscall_sendfile_bytes":        0,
				"syscall_sendfile_count":        0,
				"syscall_set_quota_count":       0,
				"syscall_set_sd_count":          0,
				"syscall_stat_count":            0,
				"syscall_symlinkat_count":       0,
				"syscall_telldir_count":         0,
				"syscall_unlinkat_count":        0,
			},
		},
		"error on exec": {
			prepareMock: prepareMockErr,
			wantMetrics: nil,
		},
		"empty response": {
			prepareMock: prepareMockEmptyResponse,
			wantMetrics: nil,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				assert.Len(t, *collr.Charts(), test.wantCharts, "want charts")
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func prepareMockOk() *mockSmbStatusBinary {
	return &mockSmbStatusBinary{
		data: dataSmbStatusProfile,
	}
}

func prepareMockErr() *mockSmbStatusBinary {
	return &mockSmbStatusBinary{
		err: true,
	}
}

func prepareMockEmptyResponse() *mockSmbStatusBinary {
	return &mockSmbStatusBinary{}
}

func prepareMockUnexpectedResponse() *mockSmbStatusBinary {
	return &mockSmbStatusBinary{
		data: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

type mockSmbStatusBinary struct {
	err  bool
	data []byte
}

func (m *mockSmbStatusBinary) profile() ([]byte, error) {
	if m.err {
		return nil, errors.New("mock.profile() error")
	}
	return m.data, nil
}
