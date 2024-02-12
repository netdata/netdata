// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux
// +build linux

package systemdunits

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/netdata/go.d.plugin/agent/module"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	assert.Implements(t, (*module.Module)(nil), New())
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
				assert.False(t, systemd.Init())
			} else {
				assert.True(t, systemd.Init())
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
			require.True(t, systemd.Init())

			if test.wantFail {
				assert.False(t, systemd.Check())
			} else {
				assert.True(t, systemd.Check())
			}
		})
	}
}

func TestSystemdUnits_Charts(t *testing.T) {
	systemd := New()
	require.True(t, systemd.Init())
	assert.NotNil(t, systemd.Charts())
}

func TestSystemdUnits_Cleanup(t *testing.T) {
	systemd := New()
	systemd.Include = []string{"*"}
	client := prepareOKClient(230)
	systemd.client = client

	require.True(t, systemd.Init())
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
		"success on systemd v230+ on collecting all unit type": {
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
		"success on systemd v230- on collecting all unit types": {
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
		"success on systemd v230+ on collecting only 'service' unit type": {
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
		"success on systemd v230- on collecting only 'service' unit type": {
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
			require.True(t, systemd.Init())

			var collected map[string]int64

			for i := 0; i < 10; i++ {
				collected = systemd.Collect()
			}

			assert.Equal(t, test.wantCollected, collected)
			if len(test.wantCollected) > 0 {
				ensureCollectedHasAllChartsDimsVarsIDs(t, systemd, collected)
			}
		})
	}
}

func TestSystemdUnits_connectionReuse(t *testing.T) {
	systemd := New()
	systemd.Include = []string{"*"}
	client := prepareOKClient(230)
	systemd.client = client
	require.True(t, systemd.Init())

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
			version: ver,
			units:   mockSystemdUnits,
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
	units                   []dbus.UnitStatus
	errOnGetManagerProperty bool
	errOnListUnits          bool
	closeCalled             bool
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

func (m *mockConn) ListUnitsContext(_ context.Context) ([]dbus.UnitStatus, error) {
	if m.errOnListUnits {
		return nil, errors.New("'ListUnits' call error")
	}
	if m.version >= 230 {
		return nil, errors.New("'ListUnits' unsupported function error")
	}
	return append([]dbus.UnitStatus{}, m.units...), nil
}

func (m *mockConn) ListUnitsByPatternsContext(_ context.Context, _ []string, ps []string) ([]dbus.UnitStatus, error) {
	if m.errOnListUnits {
		return nil, errors.New("'ListUnitsByPatterns' call error")
	}
	if m.version < 230 {
		return nil, errors.New("'ListUnitsByPatterns' unsupported function error")
	}

	matches := func(name string) bool {
		for _, p := range ps {
			if ok, _ := filepath.Match(p, name); ok {
				return true
			}
		}
		return false
	}

	var units []dbus.UnitStatus
	for _, unit := range m.units {
		if matches(unit.Name) {
			units = append(units, unit)
		}
	}
	return units, nil
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
