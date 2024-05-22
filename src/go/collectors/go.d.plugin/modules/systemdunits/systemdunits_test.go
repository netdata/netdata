// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux
// +build linux

package systemdunits

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"

	"github.com/coreos/go-systemd/v22/dbus"
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
		require.NotNil(t, data, name)
	}
}

func TestSystemdUnits_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &SystemdUnits{}, dataConfigJSON, dataConfigYAML)
}

func TestSystemdUnits_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success on default config": {
			config: New().Config,
		},
		"success when 'include' option set": {
			config: Config{
				Include: []string{"*"},
			},
		},
		"fails when 'include' option not set": {
			wantFail: true,
			config:   Config{Include: []string{}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			systemd := New()
			systemd.Config = test.config

			if test.wantFail {
				assert.Error(t, systemd.Init())
			} else {
				assert.NoError(t, systemd.Init())
			}
		})
	}
}

func TestSystemdUnits_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *SystemdUnits
		wantFail bool
	}{
		"success on systemd v230+": {
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.Include = []string{"*"}
				systemd.client = prepareOKClient(230)
				return systemd
			},
		},
		"success on systemd v230-": {
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.Include = []string{"*"}
				systemd.client = prepareOKClient(220)
				return systemd
			},
		},
		"fails when all unites are filtered": {
			wantFail: true,
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.Include = []string{"*.not_exists"}
				systemd.client = prepareOKClient(230)
				return systemd
			},
		},
		"fails on error on connect": {
			wantFail: true,
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.client = prepareClientErrOnConnect()
				return systemd
			},
		},
		"fails on error on get manager property": {
			wantFail: true,
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.client = prepareClientErrOnGetManagerProperty()
				return systemd
			},
		},
		"fails on error on list units": {
			wantFail: true,
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.client = prepareClientErrOnListUnits()
				return systemd
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			systemd := test.prepare()
			require.NoError(t, systemd.Init())

			if test.wantFail {
				assert.Error(t, systemd.Check())
			} else {
				assert.NoError(t, systemd.Check())
			}
		})
	}
}

func TestSystemdUnits_Charts(t *testing.T) {
	systemd := New()
	require.NoError(t, systemd.Init())
	assert.NotNil(t, systemd.Charts())
}

func TestSystemdUnits_Cleanup(t *testing.T) {
	systemd := New()
	systemd.Include = []string{"*"}
	client := prepareOKClient(230)
	systemd.client = client

	require.NoError(t, systemd.Init())
	require.NotNil(t, systemd.Collect())
	conn := systemd.conn
	systemd.Cleanup()

	assert.Nil(t, systemd.conn)
	v, _ := conn.(*mockConn)
	assert.True(t, v.closeCalled)
}

func TestSystemdUnits_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() *SystemdUnits
		wantCollected map[string]int64
	}{
		"success v230+ on collecting all unit type": {
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.Include = []string{"*"}
				systemd.client = prepareOKClient(230)
				return systemd
			},
			wantCollected: map[string]int64{
				"unit_dbus_socket_state_activating":                                0,
				"unit_dbus_socket_state_active":                                    1,
				"unit_dbus_socket_state_deactivating":                              0,
				"unit_dbus_socket_state_failed":                                    0,
				"unit_dbus_socket_state_inactive":                                  0,
				"unit_dev-disk-by-uuid-DE44-CEE0_device_state_activating":          0,
				"unit_dev-disk-by-uuid-DE44-CEE0_device_state_active":              1,
				"unit_dev-disk-by-uuid-DE44-CEE0_device_state_deactivating":        0,
				"unit_dev-disk-by-uuid-DE44-CEE0_device_state_failed":              0,
				"unit_dev-disk-by-uuid-DE44-CEE0_device_state_inactive":            0,
				"unit_dev-nvme0n1_device_state_activating":                         0,
				"unit_dev-nvme0n1_device_state_active":                             1,
				"unit_dev-nvme0n1_device_state_deactivating":                       0,
				"unit_dev-nvme0n1_device_state_failed":                             0,
				"unit_dev-nvme0n1_device_state_inactive":                           0,
				"unit_docker_socket_state_activating":                              0,
				"unit_docker_socket_state_active":                                  1,
				"unit_docker_socket_state_deactivating":                            0,
				"unit_docker_socket_state_failed":                                  0,
				"unit_docker_socket_state_inactive":                                0,
				"unit_getty-pre_target_state_activating":                           0,
				"unit_getty-pre_target_state_active":                               0,
				"unit_getty-pre_target_state_deactivating":                         0,
				"unit_getty-pre_target_state_failed":                               0,
				"unit_getty-pre_target_state_inactive":                             1,
				"unit_init_scope_state_activating":                                 0,
				"unit_init_scope_state_active":                                     1,
				"unit_init_scope_state_deactivating":                               0,
				"unit_init_scope_state_failed":                                     0,
				"unit_init_scope_state_inactive":                                   0,
				"unit_logrotate_timer_state_activating":                            0,
				"unit_logrotate_timer_state_active":                                1,
				"unit_logrotate_timer_state_deactivating":                          0,
				"unit_logrotate_timer_state_failed":                                0,
				"unit_logrotate_timer_state_inactive":                              0,
				"unit_lvm2-lvmetad_socket_state_activating":                        0,
				"unit_lvm2-lvmetad_socket_state_active":                            1,
				"unit_lvm2-lvmetad_socket_state_deactivating":                      0,
				"unit_lvm2-lvmetad_socket_state_failed":                            0,
				"unit_lvm2-lvmetad_socket_state_inactive":                          0,
				"unit_lvm2-lvmpolld_socket_state_activating":                       0,
				"unit_lvm2-lvmpolld_socket_state_active":                           1,
				"unit_lvm2-lvmpolld_socket_state_deactivating":                     0,
				"unit_lvm2-lvmpolld_socket_state_failed":                           0,
				"unit_lvm2-lvmpolld_socket_state_inactive":                         0,
				"unit_man-db_timer_state_activating":                               0,
				"unit_man-db_timer_state_active":                                   1,
				"unit_man-db_timer_state_deactivating":                             0,
				"unit_man-db_timer_state_failed":                                   0,
				"unit_man-db_timer_state_inactive":                                 0,
				"unit_org.cups.cupsd_path_state_activating":                        0,
				"unit_org.cups.cupsd_path_state_active":                            1,
				"unit_org.cups.cupsd_path_state_deactivating":                      0,
				"unit_org.cups.cupsd_path_state_failed":                            0,
				"unit_org.cups.cupsd_path_state_inactive":                          0,
				"unit_pamac-cleancache_timer_state_activating":                     0,
				"unit_pamac-cleancache_timer_state_active":                         1,
				"unit_pamac-cleancache_timer_state_deactivating":                   0,
				"unit_pamac-cleancache_timer_state_failed":                         0,
				"unit_pamac-cleancache_timer_state_inactive":                       0,
				"unit_pamac-mirrorlist_timer_state_activating":                     0,
				"unit_pamac-mirrorlist_timer_state_active":                         1,
				"unit_pamac-mirrorlist_timer_state_deactivating":                   0,
				"unit_pamac-mirrorlist_timer_state_failed":                         0,
				"unit_pamac-mirrorlist_timer_state_inactive":                       0,
				"unit_proc-sys-fs-binfmt_misc_automount_state_activating":          0,
				"unit_proc-sys-fs-binfmt_misc_automount_state_active":              1,
				"unit_proc-sys-fs-binfmt_misc_automount_state_deactivating":        0,
				"unit_proc-sys-fs-binfmt_misc_automount_state_failed":              0,
				"unit_proc-sys-fs-binfmt_misc_automount_state_inactive":            0,
				"unit_remote-fs-pre_target_state_activating":                       0,
				"unit_remote-fs-pre_target_state_active":                           0,
				"unit_remote-fs-pre_target_state_deactivating":                     0,
				"unit_remote-fs-pre_target_state_failed":                           0,
				"unit_remote-fs-pre_target_state_inactive":                         1,
				"unit_rpc_pipefs_target_state_activating":                          0,
				"unit_rpc_pipefs_target_state_active":                              0,
				"unit_rpc_pipefs_target_state_deactivating":                        0,
				"unit_rpc_pipefs_target_state_failed":                              0,
				"unit_rpc_pipefs_target_state_inactive":                            1,
				"unit_run-user-1000-gvfs_mount_state_activating":                   0,
				"unit_run-user-1000-gvfs_mount_state_active":                       1,
				"unit_run-user-1000-gvfs_mount_state_deactivating":                 0,
				"unit_run-user-1000-gvfs_mount_state_failed":                       0,
				"unit_run-user-1000-gvfs_mount_state_inactive":                     0,
				"unit_run-user-1000_mount_state_activating":                        0,
				"unit_run-user-1000_mount_state_active":                            1,
				"unit_run-user-1000_mount_state_deactivating":                      0,
				"unit_run-user-1000_mount_state_failed":                            0,
				"unit_run-user-1000_mount_state_inactive":                          0,
				"unit_session-1_scope_state_activating":                            0,
				"unit_session-1_scope_state_active":                                1,
				"unit_session-1_scope_state_deactivating":                          0,
				"unit_session-1_scope_state_failed":                                0,
				"unit_session-1_scope_state_inactive":                              0,
				"unit_session-2_scope_state_activating":                            0,
				"unit_session-2_scope_state_active":                                1,
				"unit_session-2_scope_state_deactivating":                          0,
				"unit_session-2_scope_state_failed":                                0,
				"unit_session-2_scope_state_inactive":                              0,
				"unit_session-3_scope_state_activating":                            0,
				"unit_session-3_scope_state_active":                                1,
				"unit_session-3_scope_state_deactivating":                          0,
				"unit_session-3_scope_state_failed":                                0,
				"unit_session-3_scope_state_inactive":                              0,
				"unit_session-6_scope_state_activating":                            0,
				"unit_session-6_scope_state_active":                                1,
				"unit_session-6_scope_state_deactivating":                          0,
				"unit_session-6_scope_state_failed":                                0,
				"unit_session-6_scope_state_inactive":                              0,
				"unit_shadow_timer_state_activating":                               0,
				"unit_shadow_timer_state_active":                                   1,
				"unit_shadow_timer_state_deactivating":                             0,
				"unit_shadow_timer_state_failed":                                   0,
				"unit_shadow_timer_state_inactive":                                 0,
				"unit_sound_target_state_activating":                               0,
				"unit_sound_target_state_active":                                   1,
				"unit_sound_target_state_deactivating":                             0,
				"unit_sound_target_state_failed":                                   0,
				"unit_sound_target_state_inactive":                                 0,
				"unit_sys-devices-virtual-net-loopback1_device_state_activating":   0,
				"unit_sys-devices-virtual-net-loopback1_device_state_active":       1,
				"unit_sys-devices-virtual-net-loopback1_device_state_deactivating": 0,
				"unit_sys-devices-virtual-net-loopback1_device_state_failed":       0,
				"unit_sys-devices-virtual-net-loopback1_device_state_inactive":     0,
				"unit_sys-module-fuse_device_state_activating":                     0,
				"unit_sys-module-fuse_device_state_active":                         1,
				"unit_sys-module-fuse_device_state_deactivating":                   0,
				"unit_sys-module-fuse_device_state_failed":                         0,
				"unit_sys-module-fuse_device_state_inactive":                       0,
				"unit_sysinit_target_state_activating":                             0,
				"unit_sysinit_target_state_active":                                 1,
				"unit_sysinit_target_state_deactivating":                           0,
				"unit_sysinit_target_state_failed":                                 0,
				"unit_sysinit_target_state_inactive":                               0,
				"unit_system-getty_slice_state_activating":                         0,
				"unit_system-getty_slice_state_active":                             1,
				"unit_system-getty_slice_state_deactivating":                       0,
				"unit_system-getty_slice_state_failed":                             0,
				"unit_system-getty_slice_state_inactive":                           0,
				"unit_system-netctl_slice_state_activating":                        0,
				"unit_system-netctl_slice_state_active":                            1,
				"unit_system-netctl_slice_state_deactivating":                      0,
				"unit_system-netctl_slice_state_failed":                            0,
				"unit_system-netctl_slice_state_inactive":                          0,
				"unit_system-systemd-fsck_slice_state_activating":                  0,
				"unit_system-systemd-fsck_slice_state_active":                      1,
				"unit_system-systemd-fsck_slice_state_deactivating":                0,
				"unit_system-systemd-fsck_slice_state_failed":                      0,
				"unit_system-systemd-fsck_slice_state_inactive":                    0,
				"unit_system_slice_state_activating":                               0,
				"unit_system_slice_state_active":                                   1,
				"unit_system_slice_state_deactivating":                             0,
				"unit_system_slice_state_failed":                                   0,
				"unit_system_slice_state_inactive":                                 0,
				"unit_systemd-ask-password-console_path_state_activating":          0,
				"unit_systemd-ask-password-console_path_state_active":              1,
				"unit_systemd-ask-password-console_path_state_deactivating":        0,
				"unit_systemd-ask-password-console_path_state_failed":              0,
				"unit_systemd-ask-password-console_path_state_inactive":            0,
				"unit_systemd-ask-password-wall_path_state_activating":             0,
				"unit_systemd-ask-password-wall_path_state_active":                 1,
				"unit_systemd-ask-password-wall_path_state_deactivating":           0,
				"unit_systemd-ask-password-wall_path_state_failed":                 0,
				"unit_systemd-ask-password-wall_path_state_inactive":               0,
				"unit_systemd-ask-password-wall_service_state_activating":          0,
				"unit_systemd-ask-password-wall_service_state_active":              0,
				"unit_systemd-ask-password-wall_service_state_deactivating":        0,
				"unit_systemd-ask-password-wall_service_state_failed":              0,
				"unit_systemd-ask-password-wall_service_state_inactive":            1,
				"unit_systemd-fsck-root_service_state_activating":                  0,
				"unit_systemd-fsck-root_service_state_active":                      0,
				"unit_systemd-fsck-root_service_state_deactivating":                0,
				"unit_systemd-fsck-root_service_state_failed":                      0,
				"unit_systemd-fsck-root_service_state_inactive":                    1,
				"unit_systemd-udevd-kernel_socket_state_activating":                0,
				"unit_systemd-udevd-kernel_socket_state_active":                    1,
				"unit_systemd-udevd-kernel_socket_state_deactivating":              0,
				"unit_systemd-udevd-kernel_socket_state_failed":                    0,
				"unit_systemd-udevd-kernel_socket_state_inactive":                  0,
				"unit_tmp_mount_state_activating":                                  0,
				"unit_tmp_mount_state_active":                                      1,
				"unit_tmp_mount_state_deactivating":                                0,
				"unit_tmp_mount_state_failed":                                      0,
				"unit_tmp_mount_state_inactive":                                    0,
				"unit_user-runtime-dir@1000_service_state_activating":              0,
				"unit_user-runtime-dir@1000_service_state_active":                  1,
				"unit_user-runtime-dir@1000_service_state_deactivating":            0,
				"unit_user-runtime-dir@1000_service_state_failed":                  0,
				"unit_user-runtime-dir@1000_service_state_inactive":                0,
				"unit_user@1000_service_state_activating":                          0,
				"unit_user@1000_service_state_active":                              1,
				"unit_user@1000_service_state_deactivating":                        0,
				"unit_user@1000_service_state_failed":                              0,
				"unit_user@1000_service_state_inactive":                            0,
				"unit_user_slice_state_activating":                                 0,
				"unit_user_slice_state_active":                                     1,
				"unit_user_slice_state_deactivating":                               0,
				"unit_user_slice_state_failed":                                     0,
				"unit_user_slice_state_inactive":                                   0,
				"unit_var-lib-nfs-rpc_pipefs_mount_state_activating":               0,
				"unit_var-lib-nfs-rpc_pipefs_mount_state_active":                   0,
				"unit_var-lib-nfs-rpc_pipefs_mount_state_deactivating":             0,
				"unit_var-lib-nfs-rpc_pipefs_mount_state_failed":                   0,
				"unit_var-lib-nfs-rpc_pipefs_mount_state_inactive":                 1,
			},
		},
		"success v230+ on collecting all unit type with skip transient": {
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.Include = []string{"*"}
				systemd.SkipTransient = true
				systemd.client = prepareOKClient(230)
				return systemd
			},
			wantCollected: map[string]int64{
				"unit_systemd-ask-password-wall_service_state_activating":   0,
				"unit_systemd-ask-password-wall_service_state_active":       0,
				"unit_systemd-ask-password-wall_service_state_deactivating": 0,
				"unit_systemd-ask-password-wall_service_state_failed":       0,
				"unit_systemd-ask-password-wall_service_state_inactive":     1,
				"unit_systemd-fsck-root_service_state_activating":           0,
				"unit_systemd-fsck-root_service_state_active":               0,
				"unit_systemd-fsck-root_service_state_deactivating":         0,
				"unit_systemd-fsck-root_service_state_failed":               0,
				"unit_systemd-fsck-root_service_state_inactive":             1,
				"unit_user-runtime-dir@1000_service_state_activating":       0,
				"unit_user-runtime-dir@1000_service_state_active":           1,
				"unit_user-runtime-dir@1000_service_state_deactivating":     0,
				"unit_user-runtime-dir@1000_service_state_failed":           0,
				"unit_user-runtime-dir@1000_service_state_inactive":         0,
				"unit_user@1000_service_state_activating":                   0,
				"unit_user@1000_service_state_active":                       1,
				"unit_user@1000_service_state_deactivating":                 0,
				"unit_user@1000_service_state_failed":                       0,
				"unit_user@1000_service_state_inactive":                     0,
			},
		},
		"success v230- on collecting all unit types": {
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.Include = []string{"*"}
				systemd.client = prepareOKClient(220)
				return systemd
			},
			wantCollected: map[string]int64{
				"unit_dbus_socket_state_activating":                                0,
				"unit_dbus_socket_state_active":                                    1,
				"unit_dbus_socket_state_deactivating":                              0,
				"unit_dbus_socket_state_failed":                                    0,
				"unit_dbus_socket_state_inactive":                                  0,
				"unit_dev-disk-by-uuid-DE44-CEE0_device_state_activating":          0,
				"unit_dev-disk-by-uuid-DE44-CEE0_device_state_active":              1,
				"unit_dev-disk-by-uuid-DE44-CEE0_device_state_deactivating":        0,
				"unit_dev-disk-by-uuid-DE44-CEE0_device_state_failed":              0,
				"unit_dev-disk-by-uuid-DE44-CEE0_device_state_inactive":            0,
				"unit_dev-nvme0n1_device_state_activating":                         0,
				"unit_dev-nvme0n1_device_state_active":                             1,
				"unit_dev-nvme0n1_device_state_deactivating":                       0,
				"unit_dev-nvme0n1_device_state_failed":                             0,
				"unit_dev-nvme0n1_device_state_inactive":                           0,
				"unit_docker_socket_state_activating":                              0,
				"unit_docker_socket_state_active":                                  1,
				"unit_docker_socket_state_deactivating":                            0,
				"unit_docker_socket_state_failed":                                  0,
				"unit_docker_socket_state_inactive":                                0,
				"unit_getty-pre_target_state_activating":                           0,
				"unit_getty-pre_target_state_active":                               0,
				"unit_getty-pre_target_state_deactivating":                         0,
				"unit_getty-pre_target_state_failed":                               0,
				"unit_getty-pre_target_state_inactive":                             1,
				"unit_init_scope_state_activating":                                 0,
				"unit_init_scope_state_active":                                     1,
				"unit_init_scope_state_deactivating":                               0,
				"unit_init_scope_state_failed":                                     0,
				"unit_init_scope_state_inactive":                                   0,
				"unit_logrotate_timer_state_activating":                            0,
				"unit_logrotate_timer_state_active":                                1,
				"unit_logrotate_timer_state_deactivating":                          0,
				"unit_logrotate_timer_state_failed":                                0,
				"unit_logrotate_timer_state_inactive":                              0,
				"unit_lvm2-lvmetad_socket_state_activating":                        0,
				"unit_lvm2-lvmetad_socket_state_active":                            1,
				"unit_lvm2-lvmetad_socket_state_deactivating":                      0,
				"unit_lvm2-lvmetad_socket_state_failed":                            0,
				"unit_lvm2-lvmetad_socket_state_inactive":                          0,
				"unit_lvm2-lvmpolld_socket_state_activating":                       0,
				"unit_lvm2-lvmpolld_socket_state_active":                           1,
				"unit_lvm2-lvmpolld_socket_state_deactivating":                     0,
				"unit_lvm2-lvmpolld_socket_state_failed":                           0,
				"unit_lvm2-lvmpolld_socket_state_inactive":                         0,
				"unit_man-db_timer_state_activating":                               0,
				"unit_man-db_timer_state_active":                                   1,
				"unit_man-db_timer_state_deactivating":                             0,
				"unit_man-db_timer_state_failed":                                   0,
				"unit_man-db_timer_state_inactive":                                 0,
				"unit_org.cups.cupsd_path_state_activating":                        0,
				"unit_org.cups.cupsd_path_state_active":                            1,
				"unit_org.cups.cupsd_path_state_deactivating":                      0,
				"unit_org.cups.cupsd_path_state_failed":                            0,
				"unit_org.cups.cupsd_path_state_inactive":                          0,
				"unit_pamac-cleancache_timer_state_activating":                     0,
				"unit_pamac-cleancache_timer_state_active":                         1,
				"unit_pamac-cleancache_timer_state_deactivating":                   0,
				"unit_pamac-cleancache_timer_state_failed":                         0,
				"unit_pamac-cleancache_timer_state_inactive":                       0,
				"unit_pamac-mirrorlist_timer_state_activating":                     0,
				"unit_pamac-mirrorlist_timer_state_active":                         1,
				"unit_pamac-mirrorlist_timer_state_deactivating":                   0,
				"unit_pamac-mirrorlist_timer_state_failed":                         0,
				"unit_pamac-mirrorlist_timer_state_inactive":                       0,
				"unit_proc-sys-fs-binfmt_misc_automount_state_activating":          0,
				"unit_proc-sys-fs-binfmt_misc_automount_state_active":              1,
				"unit_proc-sys-fs-binfmt_misc_automount_state_deactivating":        0,
				"unit_proc-sys-fs-binfmt_misc_automount_state_failed":              0,
				"unit_proc-sys-fs-binfmt_misc_automount_state_inactive":            0,
				"unit_remote-fs-pre_target_state_activating":                       0,
				"unit_remote-fs-pre_target_state_active":                           0,
				"unit_remote-fs-pre_target_state_deactivating":                     0,
				"unit_remote-fs-pre_target_state_failed":                           0,
				"unit_remote-fs-pre_target_state_inactive":                         1,
				"unit_rpc_pipefs_target_state_activating":                          0,
				"unit_rpc_pipefs_target_state_active":                              0,
				"unit_rpc_pipefs_target_state_deactivating":                        0,
				"unit_rpc_pipefs_target_state_failed":                              0,
				"unit_rpc_pipefs_target_state_inactive":                            1,
				"unit_run-user-1000-gvfs_mount_state_activating":                   0,
				"unit_run-user-1000-gvfs_mount_state_active":                       1,
				"unit_run-user-1000-gvfs_mount_state_deactivating":                 0,
				"unit_run-user-1000-gvfs_mount_state_failed":                       0,
				"unit_run-user-1000-gvfs_mount_state_inactive":                     0,
				"unit_run-user-1000_mount_state_activating":                        0,
				"unit_run-user-1000_mount_state_active":                            1,
				"unit_run-user-1000_mount_state_deactivating":                      0,
				"unit_run-user-1000_mount_state_failed":                            0,
				"unit_run-user-1000_mount_state_inactive":                          0,
				"unit_session-1_scope_state_activating":                            0,
				"unit_session-1_scope_state_active":                                1,
				"unit_session-1_scope_state_deactivating":                          0,
				"unit_session-1_scope_state_failed":                                0,
				"unit_session-1_scope_state_inactive":                              0,
				"unit_session-2_scope_state_activating":                            0,
				"unit_session-2_scope_state_active":                                1,
				"unit_session-2_scope_state_deactivating":                          0,
				"unit_session-2_scope_state_failed":                                0,
				"unit_session-2_scope_state_inactive":                              0,
				"unit_session-3_scope_state_activating":                            0,
				"unit_session-3_scope_state_active":                                1,
				"unit_session-3_scope_state_deactivating":                          0,
				"unit_session-3_scope_state_failed":                                0,
				"unit_session-3_scope_state_inactive":                              0,
				"unit_session-6_scope_state_activating":                            0,
				"unit_session-6_scope_state_active":                                1,
				"unit_session-6_scope_state_deactivating":                          0,
				"unit_session-6_scope_state_failed":                                0,
				"unit_session-6_scope_state_inactive":                              0,
				"unit_shadow_timer_state_activating":                               0,
				"unit_shadow_timer_state_active":                                   1,
				"unit_shadow_timer_state_deactivating":                             0,
				"unit_shadow_timer_state_failed":                                   0,
				"unit_shadow_timer_state_inactive":                                 0,
				"unit_sound_target_state_activating":                               0,
				"unit_sound_target_state_active":                                   1,
				"unit_sound_target_state_deactivating":                             0,
				"unit_sound_target_state_failed":                                   0,
				"unit_sound_target_state_inactive":                                 0,
				"unit_sys-devices-virtual-net-loopback1_device_state_activating":   0,
				"unit_sys-devices-virtual-net-loopback1_device_state_active":       1,
				"unit_sys-devices-virtual-net-loopback1_device_state_deactivating": 0,
				"unit_sys-devices-virtual-net-loopback1_device_state_failed":       0,
				"unit_sys-devices-virtual-net-loopback1_device_state_inactive":     0,
				"unit_sys-module-fuse_device_state_activating":                     0,
				"unit_sys-module-fuse_device_state_active":                         1,
				"unit_sys-module-fuse_device_state_deactivating":                   0,
				"unit_sys-module-fuse_device_state_failed":                         0,
				"unit_sys-module-fuse_device_state_inactive":                       0,
				"unit_sysinit_target_state_activating":                             0,
				"unit_sysinit_target_state_active":                                 1,
				"unit_sysinit_target_state_deactivating":                           0,
				"unit_sysinit_target_state_failed":                                 0,
				"unit_sysinit_target_state_inactive":                               0,
				"unit_system-getty_slice_state_activating":                         0,
				"unit_system-getty_slice_state_active":                             1,
				"unit_system-getty_slice_state_deactivating":                       0,
				"unit_system-getty_slice_state_failed":                             0,
				"unit_system-getty_slice_state_inactive":                           0,
				"unit_system-netctl_slice_state_activating":                        0,
				"unit_system-netctl_slice_state_active":                            1,
				"unit_system-netctl_slice_state_deactivating":                      0,
				"unit_system-netctl_slice_state_failed":                            0,
				"unit_system-netctl_slice_state_inactive":                          0,
				"unit_system-systemd-fsck_slice_state_activating":                  0,
				"unit_system-systemd-fsck_slice_state_active":                      1,
				"unit_system-systemd-fsck_slice_state_deactivating":                0,
				"unit_system-systemd-fsck_slice_state_failed":                      0,
				"unit_system-systemd-fsck_slice_state_inactive":                    0,
				"unit_system_slice_state_activating":                               0,
				"unit_system_slice_state_active":                                   1,
				"unit_system_slice_state_deactivating":                             0,
				"unit_system_slice_state_failed":                                   0,
				"unit_system_slice_state_inactive":                                 0,
				"unit_systemd-ask-password-console_path_state_activating":          0,
				"unit_systemd-ask-password-console_path_state_active":              1,
				"unit_systemd-ask-password-console_path_state_deactivating":        0,
				"unit_systemd-ask-password-console_path_state_failed":              0,
				"unit_systemd-ask-password-console_path_state_inactive":            0,
				"unit_systemd-ask-password-wall_path_state_activating":             0,
				"unit_systemd-ask-password-wall_path_state_active":                 1,
				"unit_systemd-ask-password-wall_path_state_deactivating":           0,
				"unit_systemd-ask-password-wall_path_state_failed":                 0,
				"unit_systemd-ask-password-wall_path_state_inactive":               0,
				"unit_systemd-ask-password-wall_service_state_activating":          0,
				"unit_systemd-ask-password-wall_service_state_active":              0,
				"unit_systemd-ask-password-wall_service_state_deactivating":        0,
				"unit_systemd-ask-password-wall_service_state_failed":              0,
				"unit_systemd-ask-password-wall_service_state_inactive":            1,
				"unit_systemd-fsck-root_service_state_activating":                  0,
				"unit_systemd-fsck-root_service_state_active":                      0,
				"unit_systemd-fsck-root_service_state_deactivating":                0,
				"unit_systemd-fsck-root_service_state_failed":                      0,
				"unit_systemd-fsck-root_service_state_inactive":                    1,
				"unit_systemd-udevd-kernel_socket_state_activating":                0,
				"unit_systemd-udevd-kernel_socket_state_active":                    1,
				"unit_systemd-udevd-kernel_socket_state_deactivating":              0,
				"unit_systemd-udevd-kernel_socket_state_failed":                    0,
				"unit_systemd-udevd-kernel_socket_state_inactive":                  0,
				"unit_tmp_mount_state_activating":                                  0,
				"unit_tmp_mount_state_active":                                      1,
				"unit_tmp_mount_state_deactivating":                                0,
				"unit_tmp_mount_state_failed":                                      0,
				"unit_tmp_mount_state_inactive":                                    0,
				"unit_user-runtime-dir@1000_service_state_activating":              0,
				"unit_user-runtime-dir@1000_service_state_active":                  1,
				"unit_user-runtime-dir@1000_service_state_deactivating":            0,
				"unit_user-runtime-dir@1000_service_state_failed":                  0,
				"unit_user-runtime-dir@1000_service_state_inactive":                0,
				"unit_user@1000_service_state_activating":                          0,
				"unit_user@1000_service_state_active":                              1,
				"unit_user@1000_service_state_deactivating":                        0,
				"unit_user@1000_service_state_failed":                              0,
				"unit_user@1000_service_state_inactive":                            0,
				"unit_user_slice_state_activating":                                 0,
				"unit_user_slice_state_active":                                     1,
				"unit_user_slice_state_deactivating":                               0,
				"unit_user_slice_state_failed":                                     0,
				"unit_user_slice_state_inactive":                                   0,
				"unit_var-lib-nfs-rpc_pipefs_mount_state_activating":               0,
				"unit_var-lib-nfs-rpc_pipefs_mount_state_active":                   0,
				"unit_var-lib-nfs-rpc_pipefs_mount_state_deactivating":             0,
				"unit_var-lib-nfs-rpc_pipefs_mount_state_failed":                   0,
				"unit_var-lib-nfs-rpc_pipefs_mount_state_inactive":                 1,
			},
		},
		"success v230+ on collecting only 'service' units": {
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.Include = []string{"*.service"}
				systemd.client = prepareOKClient(230)
				return systemd
			},
			wantCollected: map[string]int64{
				"unit_systemd-ask-password-wall_service_state_activating":   0,
				"unit_systemd-ask-password-wall_service_state_active":       0,
				"unit_systemd-ask-password-wall_service_state_deactivating": 0,
				"unit_systemd-ask-password-wall_service_state_failed":       0,
				"unit_systemd-ask-password-wall_service_state_inactive":     1,
				"unit_systemd-fsck-root_service_state_activating":           0,
				"unit_systemd-fsck-root_service_state_active":               0,
				"unit_systemd-fsck-root_service_state_deactivating":         0,
				"unit_systemd-fsck-root_service_state_failed":               0,
				"unit_systemd-fsck-root_service_state_inactive":             1,
				"unit_user-runtime-dir@1000_service_state_activating":       0,
				"unit_user-runtime-dir@1000_service_state_active":           1,
				"unit_user-runtime-dir@1000_service_state_deactivating":     0,
				"unit_user-runtime-dir@1000_service_state_failed":           0,
				"unit_user-runtime-dir@1000_service_state_inactive":         0,
				"unit_user@1000_service_state_activating":                   0,
				"unit_user@1000_service_state_active":                       1,
				"unit_user@1000_service_state_deactivating":                 0,
				"unit_user@1000_service_state_failed":                       0,
				"unit_user@1000_service_state_inactive":                     0,
			},
		},
		"success v230- on collecting only 'service' units": {
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.Include = []string{"*.service"}
				systemd.client = prepareOKClient(220)
				return systemd
			},
			wantCollected: map[string]int64{
				"unit_systemd-ask-password-wall_service_state_activating":   0,
				"unit_systemd-ask-password-wall_service_state_active":       0,
				"unit_systemd-ask-password-wall_service_state_deactivating": 0,
				"unit_systemd-ask-password-wall_service_state_failed":       0,
				"unit_systemd-ask-password-wall_service_state_inactive":     1,
				"unit_systemd-fsck-root_service_state_activating":           0,
				"unit_systemd-fsck-root_service_state_active":               0,
				"unit_systemd-fsck-root_service_state_deactivating":         0,
				"unit_systemd-fsck-root_service_state_failed":               0,
				"unit_systemd-fsck-root_service_state_inactive":             1,
				"unit_user-runtime-dir@1000_service_state_activating":       0,
				"unit_user-runtime-dir@1000_service_state_active":           1,
				"unit_user-runtime-dir@1000_service_state_deactivating":     0,
				"unit_user-runtime-dir@1000_service_state_failed":           0,
				"unit_user-runtime-dir@1000_service_state_inactive":         0,
				"unit_user@1000_service_state_activating":                   0,
				"unit_user@1000_service_state_active":                       1,
				"unit_user@1000_service_state_deactivating":                 0,
				"unit_user@1000_service_state_failed":                       0,
				"unit_user@1000_service_state_inactive":                     0,
			},
		},
		"success v230+ on collecting only 'service' units and files": {
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.Include = []string{"*.service"}
				systemd.IncludeUnitFiles = []string{"*.service", "*.slice"}
				systemd.CollectUnitFiles = true
				systemd.client = prepareOKClient(230)
				return systemd
			},
			wantCollected: map[string]int64{
				"unit_file_/lib/systemd/system/machine.slice_state_alias":                             0,
				"unit_file_/lib/systemd/system/machine.slice_state_bad":                               0,
				"unit_file_/lib/systemd/system/machine.slice_state_disabled":                          0,
				"unit_file_/lib/systemd/system/machine.slice_state_enabled":                           0,
				"unit_file_/lib/systemd/system/machine.slice_state_enabled-runtime":                   0,
				"unit_file_/lib/systemd/system/machine.slice_state_generated":                         0,
				"unit_file_/lib/systemd/system/machine.slice_state_indirect":                          0,
				"unit_file_/lib/systemd/system/machine.slice_state_linked":                            0,
				"unit_file_/lib/systemd/system/machine.slice_state_linked-runtime":                    0,
				"unit_file_/lib/systemd/system/machine.slice_state_masked":                            0,
				"unit_file_/lib/systemd/system/machine.slice_state_masked-runtime":                    0,
				"unit_file_/lib/systemd/system/machine.slice_state_static":                            1,
				"unit_file_/lib/systemd/system/machine.slice_state_transient":                         0,
				"unit_file_/lib/systemd/system/system-systemd-cryptsetup.slice_state_alias":           0,
				"unit_file_/lib/systemd/system/system-systemd-cryptsetup.slice_state_bad":             0,
				"unit_file_/lib/systemd/system/system-systemd-cryptsetup.slice_state_disabled":        0,
				"unit_file_/lib/systemd/system/system-systemd-cryptsetup.slice_state_enabled":         0,
				"unit_file_/lib/systemd/system/system-systemd-cryptsetup.slice_state_enabled-runtime": 0,
				"unit_file_/lib/systemd/system/system-systemd-cryptsetup.slice_state_generated":       0,
				"unit_file_/lib/systemd/system/system-systemd-cryptsetup.slice_state_indirect":        0,
				"unit_file_/lib/systemd/system/system-systemd-cryptsetup.slice_state_linked":          0,
				"unit_file_/lib/systemd/system/system-systemd-cryptsetup.slice_state_linked-runtime":  0,
				"unit_file_/lib/systemd/system/system-systemd-cryptsetup.slice_state_masked":          0,
				"unit_file_/lib/systemd/system/system-systemd-cryptsetup.slice_state_masked-runtime":  0,
				"unit_file_/lib/systemd/system/system-systemd-cryptsetup.slice_state_static":          1,
				"unit_file_/lib/systemd/system/system-systemd-cryptsetup.slice_state_transient":       0,
				"unit_file_/lib/systemd/system/user.slice_state_alias":                                0,
				"unit_file_/lib/systemd/system/user.slice_state_bad":                                  0,
				"unit_file_/lib/systemd/system/user.slice_state_disabled":                             0,
				"unit_file_/lib/systemd/system/user.slice_state_enabled":                              0,
				"unit_file_/lib/systemd/system/user.slice_state_enabled-runtime":                      0,
				"unit_file_/lib/systemd/system/user.slice_state_generated":                            0,
				"unit_file_/lib/systemd/system/user.slice_state_indirect":                             0,
				"unit_file_/lib/systemd/system/user.slice_state_linked":                               0,
				"unit_file_/lib/systemd/system/user.slice_state_linked-runtime":                       0,
				"unit_file_/lib/systemd/system/user.slice_state_masked":                               0,
				"unit_file_/lib/systemd/system/user.slice_state_masked-runtime":                       0,
				"unit_file_/lib/systemd/system/user.slice_state_static":                               1,
				"unit_file_/lib/systemd/system/user.slice_state_transient":                            0,
				"unit_file_/lib/systemd/system/uuidd.service_state_alias":                             0,
				"unit_file_/lib/systemd/system/uuidd.service_state_bad":                               0,
				"unit_file_/lib/systemd/system/uuidd.service_state_disabled":                          0,
				"unit_file_/lib/systemd/system/uuidd.service_state_enabled":                           0,
				"unit_file_/lib/systemd/system/uuidd.service_state_enabled-runtime":                   0,
				"unit_file_/lib/systemd/system/uuidd.service_state_generated":                         0,
				"unit_file_/lib/systemd/system/uuidd.service_state_indirect":                          1,
				"unit_file_/lib/systemd/system/uuidd.service_state_linked":                            0,
				"unit_file_/lib/systemd/system/uuidd.service_state_linked-runtime":                    0,
				"unit_file_/lib/systemd/system/uuidd.service_state_masked":                            0,
				"unit_file_/lib/systemd/system/uuidd.service_state_masked-runtime":                    0,
				"unit_file_/lib/systemd/system/uuidd.service_state_static":                            0,
				"unit_file_/lib/systemd/system/uuidd.service_state_transient":                         0,
				"unit_file_/lib/systemd/system/x11-common.service_state_alias":                        0,
				"unit_file_/lib/systemd/system/x11-common.service_state_bad":                          0,
				"unit_file_/lib/systemd/system/x11-common.service_state_disabled":                     0,
				"unit_file_/lib/systemd/system/x11-common.service_state_enabled":                      0,
				"unit_file_/lib/systemd/system/x11-common.service_state_enabled-runtime":              0,
				"unit_file_/lib/systemd/system/x11-common.service_state_generated":                    0,
				"unit_file_/lib/systemd/system/x11-common.service_state_indirect":                     0,
				"unit_file_/lib/systemd/system/x11-common.service_state_linked":                       0,
				"unit_file_/lib/systemd/system/x11-common.service_state_linked-runtime":               0,
				"unit_file_/lib/systemd/system/x11-common.service_state_masked":                       1,
				"unit_file_/lib/systemd/system/x11-common.service_state_masked-runtime":               0,
				"unit_file_/lib/systemd/system/x11-common.service_state_static":                       0,
				"unit_file_/lib/systemd/system/x11-common.service_state_transient":                    0,
				"unit_file_/run/systemd/generator.late/monit.service_state_alias":                     0,
				"unit_file_/run/systemd/generator.late/monit.service_state_bad":                       0,
				"unit_file_/run/systemd/generator.late/monit.service_state_disabled":                  0,
				"unit_file_/run/systemd/generator.late/monit.service_state_enabled":                   0,
				"unit_file_/run/systemd/generator.late/monit.service_state_enabled-runtime":           0,
				"unit_file_/run/systemd/generator.late/monit.service_state_generated":                 1,
				"unit_file_/run/systemd/generator.late/monit.service_state_indirect":                  0,
				"unit_file_/run/systemd/generator.late/monit.service_state_linked":                    0,
				"unit_file_/run/systemd/generator.late/monit.service_state_linked-runtime":            0,
				"unit_file_/run/systemd/generator.late/monit.service_state_masked":                    0,
				"unit_file_/run/systemd/generator.late/monit.service_state_masked-runtime":            0,
				"unit_file_/run/systemd/generator.late/monit.service_state_static":                    0,
				"unit_file_/run/systemd/generator.late/monit.service_state_transient":                 0,
				"unit_file_/run/systemd/generator.late/sendmail.service_state_alias":                  0,
				"unit_file_/run/systemd/generator.late/sendmail.service_state_bad":                    0,
				"unit_file_/run/systemd/generator.late/sendmail.service_state_disabled":               0,
				"unit_file_/run/systemd/generator.late/sendmail.service_state_enabled":                0,
				"unit_file_/run/systemd/generator.late/sendmail.service_state_enabled-runtime":        0,
				"unit_file_/run/systemd/generator.late/sendmail.service_state_generated":              1,
				"unit_file_/run/systemd/generator.late/sendmail.service_state_indirect":               0,
				"unit_file_/run/systemd/generator.late/sendmail.service_state_linked":                 0,
				"unit_file_/run/systemd/generator.late/sendmail.service_state_linked-runtime":         0,
				"unit_file_/run/systemd/generator.late/sendmail.service_state_masked":                 0,
				"unit_file_/run/systemd/generator.late/sendmail.service_state_masked-runtime":         0,
				"unit_file_/run/systemd/generator.late/sendmail.service_state_static":                 0,
				"unit_file_/run/systemd/generator.late/sendmail.service_state_transient":              0,
				"unit_systemd-ask-password-wall_service_state_activating":                             0,
				"unit_systemd-ask-password-wall_service_state_active":                                 0,
				"unit_systemd-ask-password-wall_service_state_deactivating":                           0,
				"unit_systemd-ask-password-wall_service_state_failed":                                 0,
				"unit_systemd-ask-password-wall_service_state_inactive":                               1,
				"unit_systemd-fsck-root_service_state_activating":                                     0,
				"unit_systemd-fsck-root_service_state_active":                                         0,
				"unit_systemd-fsck-root_service_state_deactivating":                                   0,
				"unit_systemd-fsck-root_service_state_failed":                                         0,
				"unit_systemd-fsck-root_service_state_inactive":                                       1,
				"unit_user-runtime-dir@1000_service_state_activating":                                 0,
				"unit_user-runtime-dir@1000_service_state_active":                                     1,
				"unit_user-runtime-dir@1000_service_state_deactivating":                               0,
				"unit_user-runtime-dir@1000_service_state_failed":                                     0,
				"unit_user-runtime-dir@1000_service_state_inactive":                                   0,
				"unit_user@1000_service_state_activating":                                             0,
				"unit_user@1000_service_state_active":                                                 1,
				"unit_user@1000_service_state_deactivating":                                           0,
				"unit_user@1000_service_state_failed":                                                 0,
				"unit_user@1000_service_state_inactive":                                               0,
			},
		},
		"fails when all unites are filtered": {
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.Include = []string{"*.not_exists"}
				systemd.client = prepareOKClient(230)
				return systemd
			},
			wantCollected: nil,
		},
		"fails on error on connect": {
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.client = prepareClientErrOnConnect()
				return systemd
			},
			wantCollected: nil,
		},
		"fails on error on get manager property": {
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.client = prepareClientErrOnGetManagerProperty()
				return systemd
			},
			wantCollected: nil,
		},
		"fails on error on list units": {
			prepare: func() *SystemdUnits {
				systemd := New()
				systemd.client = prepareClientErrOnListUnits()
				return systemd
			},
			wantCollected: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			systemd := test.prepare()
			require.NoError(t, systemd.Init())

			var mx map[string]int64

			for i := 0; i < 10; i++ {
				mx = systemd.Collect()
			}

			assert.Equal(t, test.wantCollected, mx)
			if len(test.wantCollected) > 0 {
				ensureCollectedHasAllChartsDimsVarsIDs(t, systemd, mx)
			}
		})
	}
}

func TestSystemdUnits_connectionReuse(t *testing.T) {
	systemd := New()
	systemd.Include = []string{"*"}
	client := prepareOKClient(230)
	systemd.client = client
	require.NoError(t, systemd.Init())

	var collected map[string]int64
	for i := 0; i < 10; i++ {
		collected = systemd.Collect()
	}

	assert.NotEmpty(t, collected)
	assert.Equal(t, 1, client.connectCalls)
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, sd *SystemdUnits, collected map[string]int64) {
	for _, chart := range *sd.Charts() {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			_, ok := collected[dim.ID]
			assert.Truef(t, ok, "collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := collected[v.ID]
			assert.Truef(t, ok, "collected metrics has no data for var '%s' chart '%s'", v.ID, chart.ID)
		}
	}
}

func prepareOKClient(ver int) *mockClient {
	return &mockClient{
		conn: &mockConn{
			version:   ver,
			units:     mockSystemdUnits,
			unitFiles: mockSystemdUnitFiles,
		},
	}
}

func prepareClientErrOnConnect() *mockClient {
	return &mockClient{
		errOnConnect: true,
	}
}

func prepareClientErrOnGetManagerProperty() *mockClient {
	return &mockClient{
		conn: &mockConn{
			version:                 230,
			errOnGetManagerProperty: true,
			units:                   mockSystemdUnits,
		},
	}
}

func prepareClientErrOnListUnits() *mockClient {
	return &mockClient{
		conn: &mockConn{
			version:        230,
			errOnListUnits: true,
			units:          mockSystemdUnits,
		},
	}
}

type mockClient struct {
	conn         systemdConnection
	connectCalls int
	errOnConnect bool
}

func (m *mockClient) connect() (systemdConnection, error) {
	m.connectCalls++
	if m.errOnConnect {
		return nil, errors.New("mock 'connect' error")
	}
	return m.conn, nil
}

type mockConn struct {
	version                 int
	errOnGetManagerProperty bool

	units          []dbus.UnitStatus
	errOnListUnits bool

	unitFiles          []dbus.UnitFile
	errOnListUnitFiles bool

	closeCalled bool
}

func (m *mockConn) Close() {
	m.closeCalled = true
}

func (m *mockConn) GetManagerProperty(prop string) (string, error) {
	if m.errOnGetManagerProperty {
		return "", errors.New("'GetManagerProperty' call error")
	}
	if prop != versionProperty {
		return "", fmt.Errorf("'GetManagerProperty' unkown property: %s", prop)
	}

	return fmt.Sprintf("%d.6-1-manjaro", m.version), nil
}

func (m *mockConn) GetUnitPropertyContext(_ context.Context, unit string, propertyName string) (*dbus.Property, error) {
	if propertyName != transientProperty {
		return nil, fmt.Errorf("'GetUnitProperty' unkown property name: %s", propertyName)
	}

	var prop dbus.Property

	if strings.HasSuffix(unit, ".service") {
		prop = dbus.PropDescription("false")
	} else {
		prop = dbus.PropDescription("true")
	}

	prop.Name = propertyName

	return &prop, nil
}

func (m *mockConn) ListUnitsContext(_ context.Context) ([]dbus.UnitStatus, error) {
	if m.errOnListUnits {
		return nil, errors.New("'ListUnits' call error")
	}
	if m.version >= 230 {
		return nil, errors.New("'ListUnits' unsupported function error")
	}

	return append([]dbus.UnitStatus{}, m.units...), nil
}

func (m *mockConn) ListUnitsByPatternsContext(_ context.Context, _ []string, patterns []string) ([]dbus.UnitStatus, error) {
	if m.errOnListUnits {
		return nil, errors.New("'ListUnitsByPatterns' call error")
	}
	if m.version < 230 {
		return nil, errors.New("'ListUnitsByPatterns' unsupported function error")
	}

	if len(m.units) == 0 {
		return nil, nil
	}

	units := append([]dbus.UnitStatus{}, m.units...)

	units = slices.DeleteFunc(units, func(u dbus.UnitStatus) bool {
		name := cleanUnitName(u.Name)
		for _, p := range patterns {
			if ok, _ := filepath.Match(p, name); ok {
				return false
			}
		}
		return true
	})

	return units, nil
}

func (m *mockConn) ListUnitFilesByPatternsContext(_ context.Context, _ []string, patterns []string) ([]dbus.UnitFile, error) {
	if m.errOnListUnitFiles {
		return nil, errors.New("'ListUnitFilesByPatternsContex' call error")
	}
	if m.version < 230 {
		return nil, errors.New("'ListUnitFilesByPatternsContex' unsupported function error")
	}

	if len(m.unitFiles) == 0 {
		return nil, nil
	}

	unitFiles := append([]dbus.UnitFile{}, m.unitFiles...)

	unitFiles = slices.DeleteFunc(unitFiles, func(file dbus.UnitFile) bool {
		_, name := filepath.Split(file.Path)
		for _, p := range patterns {
			if ok, _ := filepath.Match(p, name); ok {
				return false
			}
		}
		return true
	})

	return unitFiles, nil
}

var mockSystemdUnits = []dbus.UnitStatus{
	{Name: `proc-sys-fs-binfmt_misc.automount`, LoadState: "loaded", ActiveState: "active"},
	{Name: `dev-nvme0n1.device`, LoadState: "loaded", ActiveState: "active"},
	{Name: `sys-devices-virtual-net-loopback1.device`, LoadState: "loaded", ActiveState: "active"},
	{Name: `sys-module-fuse.device`, LoadState: "loaded", ActiveState: "active"},
	{Name: `dev-disk-by\x2duuid-DE44\x2dCEE0.device`, LoadState: "loaded", ActiveState: "active"},

	{Name: `var-lib-nfs-rpc_pipefs.mount`, LoadState: "loaded", ActiveState: "inactive"},
	{Name: `var.mount`, LoadState: "not-found", ActiveState: "inactive"},
	{Name: `run-user-1000.mount`, LoadState: "loaded", ActiveState: "active"},
	{Name: `tmp.mount`, LoadState: "loaded", ActiveState: "active"},
	{Name: `run-user-1000-gvfs.mount`, LoadState: "loaded", ActiveState: "active"},

	{Name: `org.cups.cupsd.path`, LoadState: "loaded", ActiveState: "active"},
	{Name: `systemd-ask-password-wall.path`, LoadState: "loaded", ActiveState: "active"},
	{Name: `systemd-ask-password-console.path`, LoadState: "loaded", ActiveState: "active"},

	{Name: `init.scope`, LoadState: "loaded", ActiveState: "active"},
	{Name: `session-3.scope`, LoadState: "loaded", ActiveState: "active"},
	{Name: `session-6.scope`, LoadState: "loaded", ActiveState: "active"},
	{Name: `session-1.scope`, LoadState: "loaded", ActiveState: "active"},
	{Name: `session-2.scope`, LoadState: "loaded", ActiveState: "active"},

	{Name: `systemd-fsck-root.service`, LoadState: "loaded", ActiveState: "inactive"},
	{Name: `httpd.service`, LoadState: "not-found", ActiveState: "inactive"},
	{Name: `user-runtime-dir@1000.service`, LoadState: "loaded", ActiveState: "active"},
	{Name: `systemd-ask-password-wall.service`, LoadState: "loaded", ActiveState: "inactive"},
	{Name: `user@1000.service`, LoadState: "loaded", ActiveState: "active"},

	{Name: `user.slice`, LoadState: "loaded", ActiveState: "active"},
	{Name: `system-getty.slice`, LoadState: "loaded", ActiveState: "active"},
	{Name: `system-netctl.slice`, LoadState: "loaded", ActiveState: "active"},
	{Name: `system.slice`, LoadState: "loaded", ActiveState: "active"},
	{Name: `system-systemd\x2dfsck.slice`, LoadState: "loaded", ActiveState: "active"},

	{Name: `lvm2-lvmpolld.socket`, LoadState: "loaded", ActiveState: "active"},
	{Name: `docker.socket`, LoadState: "loaded", ActiveState: "active"},
	{Name: `systemd-udevd-kernel.socket`, LoadState: "loaded", ActiveState: "active"},
	{Name: `dbus.socket`, LoadState: "loaded", ActiveState: "active"},
	{Name: `lvm2-lvmetad.socket`, LoadState: "loaded", ActiveState: "active"},

	{Name: `getty-pre.target`, LoadState: "loaded", ActiveState: "inactive"},
	{Name: `rpc_pipefs.target`, LoadState: "loaded", ActiveState: "inactive"},
	{Name: `remote-fs-pre.target`, LoadState: "loaded", ActiveState: "inactive"},
	{Name: `sysinit.target`, LoadState: "loaded", ActiveState: "active"},
	{Name: `sound.target`, LoadState: "loaded", ActiveState: "active"},

	{Name: `man-db.timer`, LoadState: "loaded", ActiveState: "active"},
	{Name: `pamac-mirrorlist.timer`, LoadState: "loaded", ActiveState: "active"},
	{Name: `pamac-cleancache.timer`, LoadState: "loaded", ActiveState: "active"},
	{Name: `shadow.timer`, LoadState: "loaded", ActiveState: "active"},
	{Name: `logrotate.timer`, LoadState: "loaded", ActiveState: "active"},
}

var mockSystemdUnitFiles = []dbus.UnitFile{
	{Path: "/lib/systemd/system/systemd-tmpfiles-clean.timer", Type: "static"},
	{Path: "/lib/systemd/system/sysstat-summary.timer", Type: "disabled"},
	{Path: "/lib/systemd/system/sysstat-collect.timer", Type: "disabled"},
	{Path: "/lib/systemd/system/pg_dump@.timer", Type: "disabled"},

	{Path: "/lib/systemd/system/veritysetup.target", Type: "static"},
	{Path: "/lib/systemd/system/veritysetup-pre.target", Type: "static"},
	{Path: "/lib/systemd/system/usb-gadget.target", Type: "static"},
	{Path: "/lib/systemd/system/umount.target", Type: "static"},

	{Path: "/lib/systemd/system/syslog.socket", Type: "static"},
	{Path: "/lib/systemd/system/ssh.socket", Type: "disabled"},
	{Path: "/lib/systemd/system/docker.socket", Type: "enabled"},
	{Path: "/lib/systemd/system/dbus.socket", Type: "static"},

	{Path: "/lib/systemd/system/user.slice", Type: "static"},
	{Path: "/lib/systemd/system/system-systemd\x2dcryptsetup.slice", Type: "static"},
	{Path: "/lib/systemd/system/machine.slice", Type: "static"},

	{Path: "/run/systemd/generator.late/sendmail.service", Type: "generated"},
	{Path: "/run/systemd/generator.late/monit.service", Type: "generated"},
	{Path: "/lib/systemd/system/x11-common.service", Type: "masked"},
	{Path: "/lib/systemd/system/uuidd.service", Type: "indirect"},

	{Path: "/run/systemd/transient/session-144.scope", Type: "transient"},
	{Path: "/run/systemd/transient/session-139.scope", Type: "transient"},
	{Path: "/run/systemd/transient/session-132.scope", Type: "transient"},

	{Path: "/lib/systemd/system/systemd-ask-password-wall.path", Type: "static"},
	{Path: "/lib/systemd/system/systemd-ask-password-console.path", Type: "static"},
	{Path: "/lib/systemd/system/postfix-resolvconf.path", Type: "disabled"},
	{Path: "/lib/systemd/system/ntpsec-systemd-netif.path", Type: "enabled"},

	{Path: "/run/systemd/generator/media-cdrom0.mount", Type: "generated"},
	{Path: "/run/systemd/generator/boot.mount", Type: "generated"},
	{Path: "/run/systemd/generator/-.mount", Type: "generated"},
	{Path: "/lib/systemd/system/sys-kernel-tracing.mount", Type: "static"},
}
