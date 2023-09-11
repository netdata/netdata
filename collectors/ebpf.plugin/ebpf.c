// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/time.h>
#include <sys/resource.h>
#include <ifaddrs.h>

#include "ebpf.h"
#include "ebpf_socket.h"
#include "ebpf_unittest.h"
#include "libnetdata/required_dummies.h"

/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

char *ebpf_plugin_dir = PLUGINS_DIR;
static char *ebpf_configured_log_dir = LOG_DIR;

char *ebpf_algorithms[] = {"absolute", "incremental"};
struct config collector_config = { .first_section = NULL,
                                   .last_section = NULL,
                                   .mutex = NETDATA_MUTEX_INITIALIZER,
                                   .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
                                              .rwlock = AVL_LOCK_INITIALIZER } };

int running_on_kernel = 0;
int ebpf_nprocs;
int isrh = 0;
int main_thread_id = 0;
int process_pid_fd = -1;
static size_t global_iterations_counter = 1;
bool publish_internal_metrics = true;

pthread_mutex_t lock;
pthread_mutex_t ebpf_exit_cleanup;
pthread_mutex_t collect_data_mutex;

struct netdata_static_thread cgroup_integration_thread = {
    .name = "EBPF CGROUP INT",
    .config_section = NULL,
    .config_name = NULL,
    .env_name = NULL,
    .enabled = 1,
    .thread = NULL,
    .init_routine = NULL,
    .start_routine = NULL
};

ebpf_module_t ebpf_modules[] = {
    { .thread_name = "process", .config_name = "process", .thread_description = NETDATA_EBPF_MODULE_PROCESS_DESC,
      .enabled = 0, .start_routine = ebpf_process_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_process_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &process_config,
      .config_file = NETDATA_PROCESS_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_10 |
                  NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = NULL, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0 },
    { .thread_name = "socket", .config_name = "socket", .thread_description = NETDATA_EBPF_SOCKET_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_socket_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_socket_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &socket_config,
      .config_file = NETDATA_NETWORK_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = socket_targets, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "cachestat", .config_name = "cachestat", .thread_description = NETDATA_EBPF_CACHESTAT_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_cachestat_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_cachestat_create_apps_charts, .maps = cachestat_maps,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &cachestat_config,
      .config_file = NETDATA_CACHESTAT_CONFIG_FILE,
      .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18|
                 NETDATA_V5_4 | NETDATA_V5_14 | NETDATA_V5_15 | NETDATA_V5_16,
      .load = EBPF_LOAD_LEGACY, .targets = cachestat_targets, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "sync", .config_name = "sync", .thread_description = NETDATA_EBPF_SYNC_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_sync_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_NOT_SET, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = NULL, .maps = NULL, .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &sync_config,
      .config_file = NETDATA_SYNC_CONFIG_FILE,
      // All syscalls have the same kernels
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = sync_targets, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "dc", .config_name = "dc", .thread_description = NETDATA_EBPF_DC_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_dcstat_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_dcstat_create_apps_charts, .maps = dcstat_maps,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &dcstat_config,
      .config_file = NETDATA_DIRECTORY_DCSTAT_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = dc_targets, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "swap", .config_name = "swap", .thread_description = NETDATA_EBPF_SWAP_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_swap_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_swap_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &swap_config,
      .config_file = NETDATA_DIRECTORY_SWAP_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = swap_targets, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "vfs", .config_name = "vfs", .thread_description = NETDATA_EBPF_VFS_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_vfs_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_vfs_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &vfs_config,
      .config_file = NETDATA_DIRECTORY_VFS_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = vfs_targets, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "filesystem", .config_name = "filesystem", .thread_description = NETDATA_EBPF_FS_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_filesystem_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_NOT_SET, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = NULL, .maps = NULL, .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &fs_config,
      .config_file = NETDATA_FILESYSTEM_CONFIG_FILE,
      //We are setting kernels as zero, because we load eBPF programs according the kernel running.
      .kernels = 0, .load = EBPF_LOAD_LEGACY, .targets = NULL, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "disk", .config_name = "disk", .thread_description = NETDATA_EBPF_DISK_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_disk_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_NOT_SET, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = NULL, .maps = NULL, .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &disk_config,
      .config_file = NETDATA_DISK_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = NULL, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "mount", .config_name = "mount", .thread_description = NETDATA_EBPF_MOUNT_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_mount_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_NOT_SET, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = NULL, .maps = NULL, .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &mount_config,
      .config_file = NETDATA_MOUNT_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = mount_targets, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "fd", .config_name = "fd", .thread_description = NETDATA_EBPF_FD_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_fd_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_fd_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &fd_config,
      .config_file = NETDATA_FD_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_11 |
                  NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = fd_targets, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "hardirq", .config_name = "hardirq", .thread_description = NETDATA_EBPF_HARDIRQ_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_hardirq_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_NOT_SET, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = NULL, .maps = NULL, .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &hardirq_config,
      .config_file = NETDATA_HARDIRQ_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = NULL, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "softirq", .config_name = "softirq", .thread_description = NETDATA_EBPF_SOFTIRQ_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_softirq_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_NOT_SET, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = NULL, .maps = NULL, .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &softirq_config,
      .config_file = NETDATA_SOFTIRQ_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = NULL, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "oomkill", .config_name = "oomkill", .thread_description = NETDATA_EBPF_OOMKILL_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_oomkill_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_oomkill_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &oomkill_config,
      .config_file = NETDATA_OOMKILL_CONFIG_FILE,
      .kernels =  NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = NULL, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "shm", .config_name = "shm", .thread_description = NETDATA_EBPF_SHM_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_shm_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_shm_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &shm_config,
      .config_file = NETDATA_DIRECTORY_SHM_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = shm_targets, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "mdflush", .config_name = "mdflush", .thread_description = NETDATA_EBPF_MD_MODULE_DESC,
      .enabled = 0, .start_routine = ebpf_mdflush_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_NOT_SET, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = NULL, .maps = NULL, .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &mdflush_config,
      .config_file = NETDATA_DIRECTORY_MDFLUSH_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = mdflush_targets, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = "functions", .config_name = "functions", .thread_description = NETDATA_EBPF_FUNCTIONS_MODULE_DESC,
      .enabled = 1, .start_routine = ebpf_function_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
      .apps_level = NETDATA_APPS_NOT_SET, .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = NULL, .maps = NULL, .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = NULL,
      .config_file = NETDATA_DIRECTORY_FUNCTIONS_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
      .load = EBPF_LOAD_LEGACY, .targets = NULL, .probe_links = NULL, .objects = NULL,
      .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES, .lifetime = EBPF_DEFAULT_LIFETIME, .running_time = 0},
    { .thread_name = NULL, .enabled = 0, .start_routine = NULL, .update_every = EBPF_DEFAULT_UPDATE_EVERY,
      .global_charts = 0, .apps_charts = NETDATA_EBPF_APPS_FLAG_NO, .apps_level = NETDATA_APPS_NOT_SET,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0, .apps_routine = NULL, .maps = NULL,
      .pid_map_size = 0, .names = NULL, .cfg = NULL, .config_name = NULL, .kernels = 0, .load = EBPF_LOAD_LEGACY,
      .targets = NULL, .probe_links = NULL, .objects = NULL, .thread = NULL, .maps_per_core = CONFIG_BOOLEAN_YES},
};

struct netdata_static_thread ebpf_threads[] = {
    {
        .name = "EBPF PROCESS",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF SOCKET",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF CACHESTAT",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF SYNC",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF DCSTAT",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF SWAP",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF VFS",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF FILESYSTEM",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF DISK",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF MOUNT",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF FD",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF HARDIRQ",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF SOFTIRQ",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF OOMKILL",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF SHM",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF MDFLUSH",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = "EBPF FUNCTIONS",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
#ifdef NETDATA_DEV_MODE
        .enabled = 1,
#else
        .enabled = 0,
#endif
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
    {
        .name = NULL,
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    },
};

ebpf_filesystem_partitions_t localfs[] =
    {{.filesystem = "ext4",
      .optional_filesystem = NULL,
      .family = "ext4",
      .objects = NULL,
      .probe_links = NULL,
      .flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION,
      .enabled = CONFIG_BOOLEAN_YES,
      .addresses = {.function = NULL, .addr = 0},
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .fs_maps = NULL,
      .fs_obj = NULL,
      .functions = {  "ext4_file_read_iter",
                      "ext4_file_write_iter",
                      "ext4_file_open",
                      "ext4_sync_file",
                      NULL }},
     {.filesystem = "xfs",
      .optional_filesystem = NULL,
      .family = "xfs",
      .objects = NULL,
      .probe_links = NULL,
      .flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION,
      .enabled = CONFIG_BOOLEAN_YES,
      .addresses = {.function = NULL, .addr = 0},
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .fs_maps = NULL,
      .fs_obj = NULL,
      .functions = {  "xfs_file_read_iter",
                      "xfs_file_write_iter",
                      "xfs_file_open",
                      "xfs_file_fsync",
                      NULL }},
     {.filesystem = "nfs",
      .optional_filesystem = "nfs4",
      .family = "nfs",
      .objects = NULL,
      .probe_links = NULL,
      .flags = NETDATA_FILESYSTEM_ATTR_CHARTS,
      .enabled = CONFIG_BOOLEAN_YES,
      .addresses = {.function = NULL, .addr = 0},
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .fs_maps = NULL,
      .fs_obj = NULL,
      .functions = {  "nfs_file_read",
                      "nfs_file_write",
                      "nfs_open",
                      "nfs_getattr",
                      NULL }}, // // "nfs4_file_open" - not present on all kernels
     {.filesystem = "zfs",
      .optional_filesystem = NULL,
      .family = "zfs",
      .objects = NULL,
      .probe_links = NULL,
      .flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION,
      .enabled = CONFIG_BOOLEAN_YES,
      .addresses = {.function = NULL, .addr = 0},
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .fs_maps = NULL,
      .fs_obj = NULL,
      .functions = {  "zpl_iter_read",
                      "zpl_iter_write",
                      "zpl_open",
                      "zpl_fsync",
                      NULL }},
     {.filesystem = "btrfs",
      .optional_filesystem = NULL,
      .family = "btrfs",
      .objects = NULL,
      .probe_links = NULL,
      .flags = NETDATA_FILESYSTEM_FILL_ADDRESS_TABLE,
      .enabled = CONFIG_BOOLEAN_YES,
      .addresses = {.function = "btrfs_file_operations", .addr = 0},
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_10,
      .fs_maps = NULL,
      .fs_obj = NULL,
      .functions = {  "btrfs_file_read_iter",
                      "btrfs_file_write_iter",
                      "btrfs_file_open",
                      "btrfs_sync_file",
                      NULL }},
     {.filesystem = NULL,
      .optional_filesystem = NULL,
      .family = NULL,
      .objects = NULL,
      .probe_links = NULL,
      .flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION,
      .enabled = CONFIG_BOOLEAN_YES,
      .addresses = {.function = NULL, .addr = 0},
      .kernels = 0, .fs_maps = NULL, .fs_obj = NULL}};

ebpf_sync_syscalls_t local_syscalls[] = {
    {.syscall = NETDATA_SYSCALLS_SYNC, .enabled = CONFIG_BOOLEAN_YES, .objects = NULL, .probe_links = NULL,
#ifdef LIBBPF_MAJOR_VERSION
     .sync_obj = NULL,
#endif
      .sync_maps = NULL
    },
    {.syscall = NETDATA_SYSCALLS_SYNCFS, .enabled = CONFIG_BOOLEAN_YES, .objects = NULL, .probe_links = NULL,
#ifdef LIBBPF_MAJOR_VERSION
     .sync_obj = NULL,
#endif
     .sync_maps = NULL
    },
    {.syscall = NETDATA_SYSCALLS_MSYNC, .enabled = CONFIG_BOOLEAN_YES, .objects = NULL, .probe_links = NULL,
#ifdef LIBBPF_MAJOR_VERSION
     .sync_obj = NULL,
#endif
     .sync_maps = NULL
    },
    {.syscall = NETDATA_SYSCALLS_FSYNC, .enabled = CONFIG_BOOLEAN_YES, .objects = NULL, .probe_links = NULL,
#ifdef LIBBPF_MAJOR_VERSION
     .sync_obj = NULL,
#endif
     .sync_maps = NULL
    },
    {.syscall = NETDATA_SYSCALLS_FDATASYNC, .enabled = CONFIG_BOOLEAN_YES, .objects = NULL, .probe_links = NULL,
#ifdef LIBBPF_MAJOR_VERSION
     .sync_obj = NULL,
#endif
     .sync_maps = NULL
    },
    {.syscall = NETDATA_SYSCALLS_SYNC_FILE_RANGE, .enabled = CONFIG_BOOLEAN_YES, .objects = NULL, .probe_links = NULL,
#ifdef LIBBPF_MAJOR_VERSION
     .sync_obj = NULL,
#endif
     .sync_maps = NULL
    },
    {.syscall = NULL, .enabled = CONFIG_BOOLEAN_NO, .objects = NULL, .probe_links = NULL,
#ifdef LIBBPF_MAJOR_VERSION
     .sync_obj = NULL,
#endif
     .sync_maps = NULL
    }
};


// Link with cgroup.plugin
netdata_ebpf_cgroup_shm_t shm_ebpf_cgroup = {NULL, NULL};
int shm_fd_ebpf_cgroup = -1;
sem_t *shm_sem_ebpf_cgroup = SEM_FAILED;
pthread_mutex_t mutex_cgroup_shm;

//Network viewer
ebpf_network_viewer_options_t network_viewer_opt;

// Statistic
ebpf_plugin_stats_t plugin_statistics = {.core = 0, .legacy = 0, .running = 0, .threads = 0, .tracepoints = 0,
                                         .probes = 0, .retprobes = 0, .trampolines = 0, .memlock_kern = 0,
                                         .hash_tables = 0};

#ifdef LIBBPF_MAJOR_VERSION
struct btf *default_btf = NULL;
struct cachestat_bpf *cachestat_bpf_obj = NULL;
struct dc_bpf *dc_bpf_obj = NULL;
struct disk_bpf *disk_bpf_obj = NULL;
struct fd_bpf *fd_bpf_obj = NULL;
struct hardirq_bpf *hardirq_bpf_obj = NULL;
struct mdflush_bpf *mdflush_bpf_obj = NULL;
struct mount_bpf *mount_bpf_obj = NULL;
struct shm_bpf *shm_bpf_obj = NULL;
struct socket_bpf *socket_bpf_obj = NULL;
struct swap_bpf *bpf_obj = NULL;
struct vfs_bpf *vfs_bpf_obj = NULL;
#else
void *default_btf = NULL;
#endif
char *btf_path = NULL;

/*****************************************************************
 *
 *  FUNCTIONS USED TO ALLOCATE APPS/CGROUP MEMORIES (ARAL)
 *
 *****************************************************************/

/**
 * Allocate PID ARAL
 *
 * Allocate memory using ARAL functions to speed up processing.
 *
 * @param name the internal name used for allocated region.
 * @param size size of each element inside allocated space
 *
 * @return It returns the address on success and NULL otherwise.
 */
ARAL *ebpf_allocate_pid_aral(char *name, size_t size)
{
    static size_t max_elements = NETDATA_EBPF_ALLOC_MAX_PID;
    if (max_elements < NETDATA_EBPF_ALLOC_MIN_ELEMENTS) {
        netdata_log_error("Number of elements given is too small, adjusting it for %d", NETDATA_EBPF_ALLOC_MIN_ELEMENTS);
        max_elements = NETDATA_EBPF_ALLOC_MIN_ELEMENTS;
    }

    return aral_create(name, size,
        0, max_elements,
        NULL, NULL, NULL, false, false);
}

/*****************************************************************
 *
 *  FUNCTIONS USED TO CLEAN MEMORY AND OPERATE SYSTEM FILES
 *
 *****************************************************************/

/**
 * Wait to avoid possible coredumps while process is closing.
 */
static inline void ebpf_check_before2go()
{
    int i = EBPF_OPTION_ALL_CHARTS;
    usec_t max = USEC_PER_SEC, step = 200000;
    while (i && max) {
        max -= step;
        sleep_usec(step);
        i = 0;
        int j;
        pthread_mutex_lock(&ebpf_exit_cleanup);
        for (j = 0; ebpf_modules[j].thread_name != NULL; j++) {
            if (ebpf_modules[j].enabled < NETDATA_THREAD_EBPF_STOPPING)
                i++;
        }
        pthread_mutex_unlock(&ebpf_exit_cleanup);
    }

    if (i) {
        netdata_log_error("eBPF cannot unload all threads on time, but it will go away");
    }
}

/**
 * Close the collector gracefully
 */
static void ebpf_exit()
{
#ifdef LIBBPF_MAJOR_VERSION
    pthread_mutex_lock(&ebpf_exit_cleanup);
    if (default_btf) {
        btf__free(default_btf);
        default_btf = NULL;
    }
    pthread_mutex_unlock(&ebpf_exit_cleanup);
#endif

    char filename[FILENAME_MAX + 1];
    ebpf_pid_file(filename, FILENAME_MAX);
    if (unlink(filename))
        netdata_log_error("Cannot remove PID file %s", filename);

#ifdef NETDATA_INTERNAL_CHECKS
    netdata_log_error("Good bye world! I was PID %d", main_thread_id);
#endif
    fprintf(stdout, "EXIT\n");
    fflush(stdout);

    ebpf_check_before2go();
    pthread_mutex_lock(&mutex_cgroup_shm);
    if (shm_ebpf_cgroup.header) {
        ebpf_unmap_cgroup_shared_memory();
        shm_unlink(NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME);
    }
    pthread_mutex_unlock(&mutex_cgroup_shm);

    exit(0);
}

/**
 * Unload loegacy code
 *
 * @param objects       objects loaded from eBPF programs
 * @param probe_links   links from loader
 */
void ebpf_unload_legacy_code(struct bpf_object *objects, struct bpf_link **probe_links)
{
    if (!probe_links || !objects)
        return;

    struct bpf_program *prog;
    size_t j = 0 ;
    bpf_object__for_each_program(prog, objects) {
        bpf_link__destroy(probe_links[j]);
        j++;
    }
    freez(probe_links);
    if (objects)
        bpf_object__close(objects);
}

/**
 * Unload Unique maps
 *
 * This function unload all BPF maps from threads using one unique BPF object.
 */
static void ebpf_unload_unique_maps()
{
    int i;
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        // These threads are cleaned with other functions
        if (i != EBPF_MODULE_SOCKET_IDX)
            continue;

        if (ebpf_modules[i].enabled != NETDATA_THREAD_EBPF_STOPPED) {
            if (ebpf_modules[i].enabled != NETDATA_THREAD_EBPF_NOT_RUNNING)
                netdata_log_error("Cannot unload maps for thread %s, because it is not stopped.", ebpf_modules[i].thread_name);

            continue;
        }

        if (ebpf_modules[i].load == EBPF_LOAD_LEGACY) {
            ebpf_unload_legacy_code(ebpf_modules[i].objects, ebpf_modules[i].probe_links);
            continue;
        }

#ifdef LIBBPF_MAJOR_VERSION
        if (socket_bpf_obj)
            socket_bpf__destroy(socket_bpf_obj);
#endif
    }
}

/**
 * Unload filesystem maps
 *
 * This function unload all BPF maps from filesystem thread.
 */
static void ebpf_unload_filesystems()
{
    if (ebpf_modules[EBPF_MODULE_FILESYSTEM_IDX].enabled == NETDATA_THREAD_EBPF_NOT_RUNNING ||
        ebpf_modules[EBPF_MODULE_FILESYSTEM_IDX].enabled < NETDATA_THREAD_EBPF_STOPPING ||
        ebpf_modules[EBPF_MODULE_FILESYSTEM_IDX].load != EBPF_LOAD_LEGACY)
        return;

    int i;
    for (i = 0; localfs[i].filesystem != NULL; i++) {
        if (!localfs[i].objects)
            continue;

        ebpf_unload_legacy_code(localfs[i].objects, localfs[i].probe_links);
    }
}

/**
 * Unload sync maps
 *
 * This function unload all BPF maps from sync thread.
 */
static void ebpf_unload_sync()
{
    if (ebpf_modules[EBPF_MODULE_SYNC_IDX].enabled == NETDATA_THREAD_EBPF_NOT_RUNNING ||
    ebpf_modules[EBPF_MODULE_SYNC_IDX].enabled < NETDATA_THREAD_EBPF_STOPPING)
        return;

    int i;
    for (i = 0; local_syscalls[i].syscall != NULL; i++) {
        if (!local_syscalls[i].enabled)
            continue;

#ifdef LIBBPF_MAJOR_VERSION
        if (local_syscalls[i].sync_obj) {
            sync_bpf__destroy(local_syscalls[i].sync_obj);
            continue;
        }
#endif
        ebpf_unload_legacy_code(local_syscalls[i].objects, local_syscalls[i].probe_links);
    }
}

int ebpf_exit_plugin = 0;
/**
 * Close the collector gracefully
 *
 * @param sig is the signal number used to close the collector
 */
static void ebpf_stop_threads(int sig)
{
    UNUSED(sig);
    static int only_one = 0;

    // Child thread should be closed by itself.
    pthread_mutex_lock(&ebpf_exit_cleanup);
    if (main_thread_id != gettid() || only_one) {
        pthread_mutex_unlock(&ebpf_exit_cleanup);
        return;
    }
    only_one = 1;
    int i;
    for (i = 0; ebpf_modules[i].thread_name != NULL; i++) {
        if (ebpf_modules[i].enabled < NETDATA_THREAD_EBPF_STOPPING) {
            netdata_thread_cancel(*ebpf_modules[i].thread->thread);
#ifdef NETDATA_DEV_MODE
            netdata_log_info("Sending cancel for thread %s", ebpf_modules[i].thread_name);
#endif
        }
    }
    pthread_mutex_unlock(&ebpf_exit_cleanup);

    pthread_mutex_lock(&mutex_cgroup_shm);
    netdata_thread_cancel(*cgroup_integration_thread.thread);
#ifdef NETDATA_DEV_MODE
    netdata_log_info("Sending cancel for thread %s", cgroup_integration_thread.name);
#endif
    pthread_mutex_unlock(&mutex_cgroup_shm);

    ebpf_exit_plugin = 1;

    ebpf_check_before2go();

    pthread_mutex_lock(&ebpf_exit_cleanup);
    ebpf_unload_unique_maps();
    ebpf_unload_filesystems();
    ebpf_unload_sync();
    pthread_mutex_unlock(&ebpf_exit_cleanup);

    ebpf_exit();
}

/*****************************************************************
 *
 *  FUNCTIONS TO CREATE CHARTS
 *
 *****************************************************************/

/**
 * Create apps for module
 *
 * Create apps chart that will be used with specific module
 *
 * @param em     the module main structure.
 * @param root   a pointer for the targets.
 */
static inline void ebpf_create_apps_for_module(ebpf_module_t *em, struct ebpf_target *root) {
    if (em->enabled < NETDATA_THREAD_EBPF_STOPPING && em->apps_charts && em->apps_routine)
        em->apps_routine(em, root);
}

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param root a pointer for the targets.
 */
static void ebpf_create_apps_charts(struct ebpf_target *root)
{
    if (unlikely(!ebpf_all_pids))
        return;

    struct ebpf_target *w;
    int newly_added = 0;

    for (w = root; w; w = w->next) {
        if (w->target)
            continue;

        if (unlikely(w->processes && (debug_enabled || w->debug_enabled))) {
            struct ebpf_pid_on_target *pid_on_target;

            fprintf(
                stderr, "ebpf.plugin: target '%s' has aggregated %u process%s:", w->name, w->processes,
                (w->processes == 1) ? "" : "es");

            for (pid_on_target = w->root_pid; pid_on_target; pid_on_target = pid_on_target->next) {
                fprintf(stderr, " %d", pid_on_target->pid);
            }

            fputc('\n', stderr);
        }

        if (!w->exposed && w->processes) {
            newly_added++;
            w->exposed = 1;
            if (debug_enabled || w->debug_enabled)
                debug_log_int("%s just added - regenerating charts.", w->name);
        }
    }

    int i;
    if (!newly_added) {
        for (i = 0; i < EBPF_MODULE_FUNCTION_IDX ; i++) {
            ebpf_module_t *current = &ebpf_modules[i];
            if (current->apps_charts & NETDATA_EBPF_APPS_FLAG_CHART_CREATED)
                continue;

            ebpf_create_apps_for_module(current, root);
        }
        return;
    }

    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX ; i++) {
        ebpf_module_t *current = &ebpf_modules[i];
        ebpf_create_apps_for_module(current, root);
    }
}

/**
 * Get a value from a structure.
 *
 * @param basis  it is the first address of the structure
 * @param offset it is the offset of the data you want to access.
 * @return
 */
collected_number get_value_from_structure(char *basis, size_t offset)
{
    collected_number *value = (collected_number *)(basis + offset);

    collected_number ret = (collected_number)llabs(*value);
    // this reset is necessary to avoid keep a constant value while processing is not executing a task
    *value = 0;

    return ret;
}

/**
 * Write begin command on standard output
 *
 * @param family the chart family name
 * @param name   the chart name
 */
void write_begin_chart(char *family, char *name)
{
    printf("BEGIN %s.%s\n", family, name);
}

/**
 * Write END command on stdout.
 */
inline void write_end_chart()
{
    printf("END\n");
}

/**
 * Write set command on standard output
 *
 * @param dim    the dimension name
 * @param value  the value for the dimension
 */
void write_chart_dimension(char *dim, long long value)
{
    printf("SET %s = %lld\n", dim, value);
}

/**
 * Call the necessary functions to create a chart.
 *
 * @param name    the chart name
 * @param family  the chart family
 * @param move    the pointer with the values that will be published
 * @param end     the number of values that will be written on standard output
 *
 * @return It returns a variable that maps the charts that did not have zero values.
 */
void write_count_chart(char *name, char *family, netdata_publish_syscall_t *move, uint32_t end)
{
    write_begin_chart(family, name);

    uint32_t i = 0;
    while (move && i < end) {
        write_chart_dimension(move->name, move->ncall);

        move = move->next;
        i++;
    }

    write_end_chart();
}

/**
 * Call the necessary functions to create a chart.
 *
 * @param name    the chart name
 * @param family  the chart family
 * @param move    the pointer with the values that will be published
 * @param end     the number of values that will be written on standard output
 */
void write_err_chart(char *name, char *family, netdata_publish_syscall_t *move, int end)
{
    write_begin_chart(family, name);

    int i = 0;
    while (move && i < end) {
        write_chart_dimension(move->name, move->nerr);

        move = move->next;
        i++;
    }

    write_end_chart();
}

/**
 * Write charts
 *
 * Write the current information to publish the charts.
 *
 * @param family chart family
 * @param chart  chart id
 * @param dim    dimension name
 * @param v1     value.
 */
void ebpf_one_dimension_write_charts(char *family, char *chart, char *dim, long long v1)
{
    write_begin_chart(family, chart);

    write_chart_dimension(dim, v1);

    write_end_chart();
}

/**
 * Call the necessary functions to create a chart.
 *
 * @param chart  the chart name
 * @param family  the chart family
 * @param dwrite the dimension name
 * @param vwrite the value for previous dimension
 * @param dread the dimension name
 * @param vread the value for previous dimension
 *
 * @return It returns a variable that maps the charts that did not have zero values.
 */
void write_io_chart(char *chart, char *family, char *dwrite, long long vwrite, char *dread, long long vread)
{
    write_begin_chart(family, chart);

    write_chart_dimension(dwrite, vwrite);
    write_chart_dimension(dread, vread);

    write_end_chart();
}

/**
 * Write chart cmd on standard output
 *
 * @param type          chart type
 * @param id            chart id
 * @param title         chart title
 * @param units         units label
 * @param family        group name used to attach the chart on dashboard
 * @param charttype     chart type
 * @param context       chart context
 * @param order         chart order
 * @param update_every  update interval used by plugin
 * @param module        chart module name, this is the eBPF thread.
 */
void ebpf_write_chart_cmd(char *type, char *id, char *title, char *units, char *family,
                          char *charttype, char *context, int order, int update_every, char *module)
{
    printf("CHART %s.%s '' '%s' '%s' '%s' '%s' '%s' %d %d '' 'ebpf.plugin' '%s'\n",
           type,
           id,
           title,
           units,
           (family)?family:"",
           (context)?context:"",
           (charttype)?charttype:"",
           order,
           update_every,
           module);
}

/**
 * Write chart cmd on standard output
 *
 * @param type      chart type
 * @param id        chart id
 * @param title     chart title
 * @param units     units label
 * @param family    group name used to attach the chart on dashboard
 * @param charttype chart type
 * @param context   chart context
 * @param order     chart order
 * @param update_every value to overwrite the update frequency set by the server.
 */
void ebpf_write_chart_obsolete(char *type, char *id, char *title, char *units, char *family,
                               char *charttype, char *context, int order, int update_every)
{
    printf("CHART %s.%s '' '%s' '%s' '%s' '%s' '%s' %d %d 'obsolete'\n",
           type,
           id,
           title,
           units,
           (family)?family:"",
           (context)?context:"",
           (charttype)?charttype:"",
           order,
           update_every);
}

/**
 * Write the dimension command on standard output
 *
 * @param name the dimension name
 * @param id the dimension id
 * @param algo the dimension algorithm
 */
void ebpf_write_global_dimension(char *name, char *id, char *algorithm)
{
    printf("DIMENSION %s %s %s 1 1\n", name, id, algorithm);
}

/**
 * Call ebpf_write_global_dimension to create the dimensions for a specific chart
 *
 * @param ptr a pointer to a structure of the type netdata_publish_syscall_t
 * @param end the number of dimensions for the structure ptr
 */
void ebpf_create_global_dimension(void *ptr, int end)
{
    netdata_publish_syscall_t *move = ptr;

    int i = 0;
    while (move && i < end) {
        ebpf_write_global_dimension(move->name, move->dimension, move->algorithm);

        move = move->next;
        i++;
    }
}

/**
 *  Call write_chart_cmd to create the charts
 *
 * @param type         chart type
 * @param id           chart id
 * @param title        chart title
 * @param units        axis label
 * @param family       group name used to attach the chart on dashboard
 * @param context      chart context
 * @param charttype    chart type
 * @param order        order number of the specified chart
 * @param ncd          a pointer to a function called to create dimensions
 * @param move         a pointer for a structure that has the dimensions
 * @param end          number of dimensions for the chart created
 * @param update_every update interval used with chart.
 * @param module       chart module name, this is the eBPF thread.
 */
void ebpf_create_chart(char *type,
                       char *id,
                       char *title,
                       char *units,
                       char *family,
                       char *context,
                       char *charttype,
                       int order,
                       void (*ncd)(void *, int),
                       void *move,
                       int end,
                       int update_every,
                       char *module)
{
    ebpf_write_chart_cmd(type, id, title, units, family, charttype, context, order, update_every, module);

    if (ncd) {
        ncd(move, end);
    }
}

/**
 * Create charts on apps submenu
 *
 * @param id   the chart id
 * @param title  the value displayed on vertical axis.
 * @param units  the value displayed on vertical axis.
 * @param family Submenu that the chart will be attached on dashboard.
 * @param charttype chart type
 * @param order  the chart order
 * @param algorithm the algorithm used by dimension
 * @param root   structure used to create the dimensions.
 * @param update_every  update interval used by plugin
 * @param module    chart module name, this is the eBPF thread.
 */
void ebpf_create_charts_on_apps(char *id, char *title, char *units, char *family, char *charttype, int order,
                                char *algorithm, struct ebpf_target *root, int update_every, char *module)
{
    struct ebpf_target *w;
    ebpf_write_chart_cmd(NETDATA_APPS_FAMILY, id, title, units, family, charttype, NULL, order,
                         update_every, module);

    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' %s 1 1\n", w->name, algorithm);
    }
}

/**
 * Call the necessary functions to create a name.
 *
 *  @param family     family name
 *  @param name       chart name
 *  @param hist0      histogram values
 *  @param dimensions dimension values.
 *  @param end        number of bins that will be sent to Netdata.
 *
 * @return It returns a variable that maps the charts that did not have zero values.
 */
void write_histogram_chart(char *family, char *name, const netdata_idx_t *hist, char **dimensions, uint32_t end)
{
    write_begin_chart(family, name);

    uint32_t i;
    for (i = 0; i < end; i++) {
        write_chart_dimension(dimensions[i], (long long) hist[i]);
    }

    write_end_chart();

    fflush(stdout);
}

/**
 * ARAL Charts
 *
 * Add chart to monitor ARAL usage
 * Caller must call this function with mutex locked.
 *
 * @param name    the name used to create aral
 * @param em      a pointer to the structure with the default values.
 */
int ebpf_statistic_create_aral_chart(char *name, ebpf_module_t *em)
{
    static int priority = NETATA_EBPF_ORDER_STAT_ARAL_BEGIN;
    char *mem = { NETDATA_EBPF_STAT_DIMENSION_MEMORY };
    char *aral = { NETDATA_EBPF_STAT_DIMENSION_ARAL };

    snprintfz(em->memory_usage, NETDATA_EBPF_CHART_MEM_LENGTH -1, "aral_%s_size", name);
    snprintfz(em->memory_allocations, NETDATA_EBPF_CHART_MEM_LENGTH -1, "aral_%s_alloc", name);

    ebpf_write_chart_cmd(NETDATA_MONITORING_FAMILY,
                         em->memory_usage,
                         "Bytes allocated for ARAL.",
                         "bytes",
                         NETDATA_EBPF_FAMILY,
                         NETDATA_EBPF_CHART_TYPE_STACKED,
                         "netdata.ebpf_aral_stat_size",
                         priority++,
                         em->update_every,
                         NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(mem,
                                mem,
                                ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    ebpf_write_chart_cmd(NETDATA_MONITORING_FAMILY,
                         em->memory_allocations,
                         "Calls to allocate memory.",
                         "calls",
                         NETDATA_EBPF_FAMILY,
                         NETDATA_EBPF_CHART_TYPE_STACKED,
                         "netdata.ebpf_aral_stat_alloc",
                         priority++,
                         em->update_every,
                         NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(aral,
                                aral,
                                ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    return priority - 2;
}

/**
 * ARAL Charts
 *
 * Add chart to monitor ARAL usage
 * Caller must call this function with mutex locked.
 *
 * @param em      a pointer to the structure with the default values.
 * @param prio    the initial priority used to disable charts.
 */
void ebpf_statistic_obsolete_aral_chart(ebpf_module_t *em, int prio)
{
    ebpf_write_chart_obsolete(NETDATA_MONITORING_FAMILY,
                              em->memory_allocations,
                              "Calls to allocate memory.",
                              "calls",
                              NETDATA_EBPF_FAMILY,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              "netdata.ebpf_aral_stat_alloc",
                              prio++,
                              em->update_every);

    ebpf_write_chart_obsolete(NETDATA_MONITORING_FAMILY,
                              em->memory_allocations,
                              "Calls to allocate memory.",
                              "calls",
                              NETDATA_EBPF_FAMILY,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              "netdata.ebpf_aral_stat_alloc",
                              prio++,
                              em->update_every);
}

/**
 * Send data from aral chart
 *
 * Send data for eBPF plugin
 *
 * @param memory  a pointer to the allocated address
 * @param em      a pointer to the structure with the default values.
 */
void ebpf_send_data_aral_chart(ARAL *memory, ebpf_module_t *em)
{
    char *mem = { NETDATA_EBPF_STAT_DIMENSION_MEMORY };
    char *aral = { NETDATA_EBPF_STAT_DIMENSION_ARAL };

    struct aral_statistics *stats = aral_statistics(memory);

    write_begin_chart(NETDATA_MONITORING_FAMILY, em->memory_usage);
    write_chart_dimension(mem, (long long)stats->structures.allocated_bytes);
    write_end_chart();

    write_begin_chart(NETDATA_MONITORING_FAMILY, em->memory_allocations);
    write_chart_dimension(aral, (long long)stats->structures.allocations);
    write_end_chart();
}

/*****************************************************************
 *
 *  FUNCTIONS TO READ GLOBAL HASH TABLES
 *
 *****************************************************************/

/**
 * Read Global Table Stats
 *
 * Read data from specified table (map_fd) using array allocated inside thread(values) and storing
 * them in stats vector starting from the first position.
 *
 * For PID tables is recommended to use a function to parse the specific data.
 *
 * @param stats             vector used to store data
 * @param values            helper to read data from hash tables.
 * @param map_fd            table that has data
 * @param maps_per_core     Is necessary to read data from all cores?
 * @param begin             initial value to query hash table
 * @param end               last value that will not be used.
 */
void ebpf_read_global_table_stats(netdata_idx_t *stats,
                                  netdata_idx_t *values,
                                  int map_fd,
                                  int maps_per_core,
                                  uint32_t begin,
                                  uint32_t end)
{
    uint32_t idx, order;

    for (idx = begin, order = 0; idx < end; idx++, order++) {
        if (!bpf_map_lookup_elem(map_fd, &idx, values)) {
            int i;
            int before = (maps_per_core) ? ebpf_nprocs: 1;
            netdata_idx_t total = 0;
            for (i = 0; i < before; i++)
                total += values[i];

            stats[order] = total;
        }
    }
}

/*****************************************************************
 *
 *  FUNCTIONS TO DEFINE OPTIONS
 *
 *****************************************************************/

/**
 * Define labels used to generate charts
 *
 * @param is   structure with information about number of calls made for a function.
 * @param pio  structure used to generate charts.
 * @param dim  a pointer for the dimensions name
 * @param name a pointer for the tensor with the name of the functions.
 * @param algorithm a vector with the algorithms used to make the charts
 * @param end  the number of elements in the previous 4 arguments.
 */
void ebpf_global_labels(netdata_syscall_stat_t *is, netdata_publish_syscall_t *pio, char **dim,
                        char **name, int *algorithm, int end)
{
    int i;

    netdata_syscall_stat_t *prev = NULL;
    netdata_publish_syscall_t *publish_prev = NULL;
    for (i = 0; i < end; i++) {
        if (prev) {
            prev->next = &is[i];
        }
        prev = &is[i];

        pio[i].dimension = dim[i];
        pio[i].name = name[i];
        pio[i].algorithm = ebpf_algorithms[algorithm[i]];
        if (publish_prev) {
            publish_prev->next = &pio[i];
        }
        publish_prev = &pio[i];
    }
}

/**
 * Define thread mode for all ebpf program.
 *
 * @param lmode  the mode that will be used for them.
 */
static inline void ebpf_set_thread_mode(netdata_run_mode_t lmode)
{
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_modules[i].mode = lmode;
    }
}

/**
 * Enable specific charts selected by user.
 *
 * @param em             the structure that will be changed
 * @param disable_cgroup the status about the cgroups charts.
 */
static inline void ebpf_enable_specific_chart(struct ebpf_module *em, int disable_cgroup)
{
    em->enabled = CONFIG_BOOLEAN_YES;

    // oomkill stores data inside apps submenu, so it always need to have apps_enabled for plugin to create
    // its chart, without this comparison eBPF.plugin will try to store invalid data when apps is disabled.
    if (!strcmp(em->thread_name, "oomkill")) {
        em->apps_charts = NETDATA_EBPF_APPS_FLAG_YES;
    }

    if (!disable_cgroup) {
        em->cgroup_charts = CONFIG_BOOLEAN_YES;
    }

    em->global_charts = CONFIG_BOOLEAN_YES;
}

/**
 * Disable all Global charts
 *
 * Disable charts
 */
static inline void disable_all_global_charts()
{
    int i;
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        ebpf_modules[i].enabled = 0;
        ebpf_modules[i].global_charts = 0;
    }
}

/**
 * Enable the specified chart group
 *
 * @param idx            the index of ebpf_modules that I am enabling
 */
static inline void ebpf_enable_chart(int idx, int disable_cgroup)
{
    int i;
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        if (i == idx) {
            ebpf_enable_specific_chart(&ebpf_modules[i], disable_cgroup);
            break;
        }
    }
}

/**
 * Disable Cgroups
 *
 * Disable charts for apps loading only global charts.
 */
static inline void ebpf_disable_cgroups()
{
    int i;
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        ebpf_modules[i].cgroup_charts = 0;
    }
}

/**
 * Update Disabled Plugins
 *
 * This function calls ebpf_update_stats to update statistics for collector.
 *
 * @param em  a pointer to `struct ebpf_module`
 */
void ebpf_update_disabled_plugin_stats(ebpf_module_t *em)
{
    pthread_mutex_lock(&lock);
    ebpf_update_stats(&plugin_statistics, em);
    pthread_mutex_unlock(&lock);
}

/**
 * Print help on standard error for user knows how to use the collector.
 */
void ebpf_print_help()
{
    const time_t t = time(NULL);
    struct tm ct;
    struct tm *test = localtime_r(&t, &ct);
    int year;
    if (test)
        year = ct.tm_year;
    else
        year = 0;

    fprintf(stderr,
            "\n"
            " Netdata ebpf.plugin %s\n"
            " Copyright (C) 2016-%d Costa Tsaousis <costa@tsaousis.gr>\n"
            " Released under GNU General Public License v3 or later.\n"
            " All rights reserved.\n"
            "\n"
            " This eBPF.plugin is a data collector plugin for netdata.\n"
            "\n"
            " This plugin only accepts long options with one or two dashes. The available command line options are:\n"
            "\n"
            " SECONDS               Set the data collection frequency.\n"
            "\n"
            " [-]-help              Show this help.\n"
            "\n"
            " [-]-version           Show software version.\n"
            "\n"
            " [-]-global            Disable charts per application and cgroup.\n"
            "\n"
            " [-]-all               Enable all chart groups (global, apps, and cgroup), unless -g is also given.\n"
            "\n"
            " [-]-cachestat         Enable charts related to process run time.\n"
            "\n"
            " [-]-dcstat            Enable charts related to directory cache.\n"
            "\n"
            " [-]-disk              Enable charts related to disk monitoring.\n"
            "\n"
            " [-]-filesystem        Enable chart related to filesystem run time.\n"
            "\n"
            " [-]-hardirq           Enable chart related to hard IRQ latency.\n"
            "\n"
            " [-]-mdflush           Enable charts related to multi-device flush.\n"
            "\n"
            " [-]-mount             Enable charts related to mount monitoring.\n"
            "\n"
            " [-]-net               Enable network viewer charts.\n"
            "\n"
            " [-]-oomkill           Enable chart related to OOM kill tracking.\n"
            "\n"
            " [-]-process           Enable charts related to process run time.\n"
            "\n"
            " [-]-return            Run the collector in return mode.\n"
            "\n"
            " [-]-shm               Enable chart related to shared memory tracking.\n"
            "\n"
            " [-]-softirq           Enable chart related to soft IRQ latency.\n"
            "\n"
            " [-]-sync              Enable chart related to sync run time.\n"
            "\n"
            " [-]-swap              Enable chart related to swap run time.\n"
            "\n"
            " [-]-vfs               Enable chart related to vfs run time.\n"
            "\n"
            " [-]-legacy            Load legacy eBPF programs.\n"
            "\n"
            " [-]-core              Use CO-RE when available(Working in progress).\n"
            "\n",
            VERSION,
            (year >= 116) ? year + 1900 : 2020);
}

/*****************************************************************
 *
 *  TRACEPOINT MANAGEMENT FUNCTIONS
 *
 *****************************************************************/

/**
 * Enable a tracepoint.
 *
 * @return 0 on success, -1 on error.
 */
int ebpf_enable_tracepoint(ebpf_tracepoint_t *tp)
{
    int test = ebpf_is_tracepoint_enabled(tp->class, tp->event);

    // err?
    if (test == -1) {
        return -1;
    }
    // disabled?
    else if (test == 0) {
        // enable it then.
        if (ebpf_enable_tracing_values(tp->class, tp->event)) {
            return -1;
        }
    }

    // enabled now or already was.
    tp->enabled = true;

    return 0;
}

/**
 * Disable a tracepoint if it's enabled.
 *
 * @return 0 on success, -1 on error.
 */
int ebpf_disable_tracepoint(ebpf_tracepoint_t *tp)
{
    int test = ebpf_is_tracepoint_enabled(tp->class, tp->event);

    // err?
    if (test == -1) {
        return -1;
    }
    // enabled?
    else if (test == 1) {
        // disable it then.
        if (ebpf_disable_tracing_values(tp->class, tp->event)) {
            return -1;
        }
    }

    // disable now or already was.
    tp->enabled = false;

    return 0;
}

/**
 * Enable multiple tracepoints on a list of tracepoints which end when the
 * class is NULL.
 *
 * @return the number of successful enables.
 */
uint32_t ebpf_enable_tracepoints(ebpf_tracepoint_t *tps)
{
    uint32_t cnt = 0;
    for (int i = 0; tps[i].class != NULL; i++) {
        if (ebpf_enable_tracepoint(&tps[i]) == -1) {
            netdata_log_error("Failed to enable tracepoint %s:%s", tps[i].class, tps[i].event);
        }
        else {
            cnt += 1;
        }
    }
    return cnt;
}

/*****************************************************************
 *
 *  AUXILIARY FUNCTIONS USED DURING INITIALIZATION
 *
 *****************************************************************/

/**
 *  Read Local Ports
 *
 *  Parse /proc/net/{tcp,udp} and get the ports Linux is listening.
 *
 *  @param filename the proc file to parse.
 *  @param proto is the magic number associated to the protocol file we are reading.
 */
static void read_local_ports(char *filename, uint8_t proto)
{
    procfile *ff = procfile_open(filename, " \t:", PROCFILE_FLAG_DEFAULT);
    if (!ff)
        return;

    ff = procfile_readall(ff);
    if (!ff)
        return;

    size_t lines = procfile_lines(ff), l;
    netdata_passive_connection_t values = {.counter = 0, .tgid = 0, .pid = 0};
    for(l = 0; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        // This is header or end of file
        if (unlikely(words < 14))
            continue;

        // https://elixir.bootlin.com/linux/v5.7.8/source/include/net/tcp_states.h
        // 0A = TCP_LISTEN
        if (strcmp("0A", procfile_lineword(ff, l, 5)))
            continue;

        // Read local port
        uint16_t port = (uint16_t)strtol(procfile_lineword(ff, l, 2), NULL, 16);
        update_listen_table(htons(port), proto, &values);
    }

    procfile_close(ff);
}

/**
 * Read Local addresseses
 *
 * Read the local address from the interfaces.
 */
static void read_local_addresses()
{
    struct ifaddrs *ifaddr, *ifa;
    if (getifaddrs(&ifaddr) == -1) {
        netdata_log_error("Cannot get the local IP addresses, it is no possible to do separation between inbound and outbound connections");
        return;
    }

    char *notext = { "No text representation" };
    for (ifa = ifaddr; ifa != NULL; ifa = ifa->ifa_next) {
        if (ifa->ifa_addr == NULL)
            continue;

        if ((ifa->ifa_addr->sa_family != AF_INET) && (ifa->ifa_addr->sa_family != AF_INET6))
            continue;

        ebpf_network_viewer_ip_list_t *w = callocz(1, sizeof(ebpf_network_viewer_ip_list_t));

        int family = ifa->ifa_addr->sa_family;
        w->ver = (uint8_t) family;
        char text[INET6_ADDRSTRLEN];
        if (family == AF_INET) {
            struct sockaddr_in *in = (struct sockaddr_in*) ifa->ifa_addr;

            w->first.addr32[0] = in->sin_addr.s_addr;
            w->last.addr32[0] = in->sin_addr.s_addr;

            if (inet_ntop(AF_INET, w->first.addr8, text, INET_ADDRSTRLEN)) {
                w->value = strdupz(text);
                w->hash = simple_hash(text);
            } else {
                w->value = strdupz(notext);
                w->hash = simple_hash(notext);
            }
        } else {
            struct sockaddr_in6 *in6 = (struct sockaddr_in6*) ifa->ifa_addr;

            memcpy(w->first.addr8, (void *)&in6->sin6_addr, sizeof(struct in6_addr));
            memcpy(w->last.addr8, (void *)&in6->sin6_addr, sizeof(struct in6_addr));

            if (inet_ntop(AF_INET6, w->first.addr8, text, INET_ADDRSTRLEN)) {
                w->value = strdupz(text);
                w->hash = simple_hash(text);
            } else {
                w->value = strdupz(notext);
                w->hash = simple_hash(notext);
            }
        }

        ebpf_fill_ip_list((family == AF_INET)?&network_viewer_opt.ipv4_local_ip:&network_viewer_opt.ipv6_local_ip,
                     w,
                     "selector");
    }

    freeifaddrs(ifaddr);
}

/**
 * Start Pthread Variable
 *
 * This function starts all pthread variables.
 */
void ebpf_start_pthread_variables()
{
    pthread_mutex_init(&lock, NULL);
    pthread_mutex_init(&ebpf_exit_cleanup, NULL);
    pthread_mutex_init(&collect_data_mutex, NULL);
    pthread_mutex_init(&mutex_cgroup_shm, NULL);
}

/**
 * Allocate the vectors used for all threads.
 */
static void ebpf_allocate_common_vectors()
{
    ebpf_all_pids = callocz((size_t)pid_max, sizeof(struct ebpf_pid_stat *));
    ebpf_aral_init();
}

/**
 * Define how to load the ebpf programs
 *
 * @param ptr the option given by users
 */
static inline void ebpf_how_to_load(char *ptr)
{
    if (!strcasecmp(ptr, EBPF_CFG_LOAD_MODE_RETURN))
        ebpf_set_thread_mode(MODE_RETURN);
    else if (!strcasecmp(ptr, EBPF_CFG_LOAD_MODE_DEFAULT))
        ebpf_set_thread_mode(MODE_ENTRY);
    else
        netdata_log_error("the option %s for \"ebpf load mode\" is not a valid option.", ptr);
}

/**
 * Define whether we should have charts for apps
 *
 * @param lmode  the mode that will be used for them.
 */
static inline void ebpf_set_apps_mode(netdata_apps_integration_flags_t value)
{
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_modules[i].apps_charts = value;
    }
}


/**
 * Update interval
 *
 * Update default interval with value from user
 *
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_update_interval(int update_every)
{
    int i;
    int value = (int) appconfig_get_number(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_UPDATE_EVERY,
                                          update_every);
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        ebpf_modules[i].update_every = value;
    }
}

/**
 * Update PID table size
 *
 * Update default size with value from user
 */
static void ebpf_update_table_size()
{
    int i;
    uint32_t value = (uint32_t) appconfig_get_number(&collector_config, EBPF_GLOBAL_SECTION,
                                                    EBPF_CFG_PID_SIZE, ND_EBPF_DEFAULT_PID_SIZE);
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        ebpf_modules[i].pid_map_size = value;
    }
}

/**
 * Update lifetime
 *
 * Update the period of time that specific thread will run
 */
static void ebpf_update_lifetime()
{
    int i;
    uint32_t value = (uint32_t) appconfig_get_number(&collector_config, EBPF_GLOBAL_SECTION,
                                                     EBPF_CFG_LIFETIME, EBPF_DEFAULT_LIFETIME);
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        ebpf_modules[i].lifetime = value;
    }
}

/**
 * Set Load mode
 *
 * @param origin specify the configuration file loaded
 */
static inline void ebpf_set_load_mode(netdata_ebpf_load_mode_t load, netdata_ebpf_load_mode_t origin)
{
    int i;
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        ebpf_modules[i].load &= ~NETDATA_EBPF_LOAD_METHODS;
        ebpf_modules[i].load |= load | origin ;
    }
}

/**
 *  Update mode
 *
 *  @param str      value read from configuration file.
 *  @param origin   specify the configuration file loaded
 */
static inline void epbf_update_load_mode(char *str, netdata_ebpf_load_mode_t origin)
{
    netdata_ebpf_load_mode_t load = epbf_convert_string_to_load_mode(str);

    ebpf_set_load_mode(load, origin);
}

/**
 * Update Map per core
 *
 * Define the map type used with some hash tables.
 */
static void ebpf_update_map_per_core()
{
    int i;
    int value = appconfig_get_boolean(&collector_config, EBPF_GLOBAL_SECTION,
                                      EBPF_CFG_MAPS_PER_CORE, CONFIG_BOOLEAN_YES);
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        ebpf_modules[i].maps_per_core = value;
    }
}

/**
 * Read collector values
 *
 * @param disable_cgroups variable to store information related to cgroups.
 * @param update_every    value to overwrite the update frequency set by the server.
 * @param origin          specify the configuration file loaded
 */
static void read_collector_values(int *disable_cgroups,
                                  int update_every, netdata_ebpf_load_mode_t origin)
{
    // Read global section
    char *value;
    if (appconfig_exists(&collector_config, EBPF_GLOBAL_SECTION, "load")) // Backward compatibility
        value = appconfig_get(&collector_config, EBPF_GLOBAL_SECTION, "load",
                              EBPF_CFG_LOAD_MODE_DEFAULT);
    else
        value = appconfig_get(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_LOAD_MODE,
                              EBPF_CFG_LOAD_MODE_DEFAULT);

    ebpf_how_to_load(value);

    btf_path = appconfig_get(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_PROGRAM_PATH,
                             EBPF_DEFAULT_BTF_PATH);

#ifdef LIBBPF_MAJOR_VERSION
    default_btf = ebpf_load_btf_file(btf_path, EBPF_DEFAULT_BTF_FILE);
#endif

    value = appconfig_get(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_TYPE_FORMAT, EBPF_CFG_DEFAULT_PROGRAM);

    epbf_update_load_mode(value, origin);

    ebpf_update_interval(update_every);

    ebpf_update_table_size();

    ebpf_update_lifetime();

    // This is kept to keep compatibility
    uint32_t enabled = appconfig_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, "disable apps",
                                             CONFIG_BOOLEAN_NO);
    if (!enabled) {
        // Apps is a positive sentence, so we need to invert the values to disable apps.
        enabled = appconfig_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_APPLICATION,
                                        CONFIG_BOOLEAN_YES);
        enabled =  (enabled == CONFIG_BOOLEAN_NO)?CONFIG_BOOLEAN_YES:CONFIG_BOOLEAN_NO;
    }

    ebpf_set_apps_mode(!enabled);

    // Cgroup is a positive sentence, so we need to invert the values to disable apps.
    // We are using the same pattern for cgroup and apps
    enabled = appconfig_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_CGROUP, CONFIG_BOOLEAN_NO);
    *disable_cgroups =  (enabled == CONFIG_BOOLEAN_NO)?CONFIG_BOOLEAN_YES:CONFIG_BOOLEAN_NO;

    ebpf_update_map_per_core();

    // Read ebpf programs section
    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION,
                                    ebpf_modules[EBPF_MODULE_PROCESS_IDX].config_name, CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_PROCESS_IDX, *disable_cgroups);
    }

    // This is kept to keep compatibility
    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "network viewer",
                                    CONFIG_BOOLEAN_NO);
    if (!enabled)
        enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION,
                                        ebpf_modules[EBPF_MODULE_SOCKET_IDX].config_name,
                                        CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SOCKET_IDX, *disable_cgroups);
    }

    // This is kept to keep compatibility
    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "network connection monitoring",
                                    CONFIG_BOOLEAN_NO);
    if (!enabled)
        enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "network connections",
                                        CONFIG_BOOLEAN_NO);
    network_viewer_opt.enabled = enabled;
    if (enabled) {
        if (!ebpf_modules[EBPF_MODULE_SOCKET_IDX].enabled)
            ebpf_enable_chart(EBPF_MODULE_SOCKET_IDX, *disable_cgroups);

        // Read network viewer section if network viewer is enabled
        // This is kept here to keep backward compatibility
        parse_network_viewer_section(&collector_config);
        parse_service_name_section(&collector_config);
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "cachestat",
                                    CONFIG_BOOLEAN_NO);

    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_CACHESTAT_IDX, *disable_cgroups);
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "sync",
                                    CONFIG_BOOLEAN_YES);

    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SYNC_IDX, *disable_cgroups);
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "dcstat",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_DCSTAT_IDX, *disable_cgroups);
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "swap",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SWAP_IDX, *disable_cgroups);
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "vfs",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_VFS_IDX, *disable_cgroups);
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "filesystem",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_FILESYSTEM_IDX, *disable_cgroups);
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "disk",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_DISK_IDX, *disable_cgroups);
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "mount",
                                    CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_MOUNT_IDX, *disable_cgroups);
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "fd",
                                    CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_FD_IDX, *disable_cgroups);
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "hardirq",
                                    CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_HARDIRQ_IDX, *disable_cgroups);
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "softirq",
                                    CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SOFTIRQ_IDX, *disable_cgroups);
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "oomkill",
                                    CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_OOMKILL_IDX, *disable_cgroups);
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "shm",
                                    CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SHM_IDX, *disable_cgroups);
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "mdflush",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_MDFLUSH_IDX, *disable_cgroups);
    }
}

/**
 * Load collector config
 *
 * @param path             the path where the file ebpf.conf is stored.
 * @param disable_cgroups  variable to store the information about cgroups plugin status.
 * @param update_every value to overwrite the update frequency set by the server.
 *
 * @return 0 on success and -1 otherwise.
 */
static int ebpf_load_collector_config(char *path, int *disable_cgroups, int update_every)
{
    char lpath[4096];
    netdata_ebpf_load_mode_t origin;

    snprintf(lpath, 4095, "%s/%s", path, NETDATA_EBPF_CONFIG_FILE);
    if (!appconfig_load(&collector_config, lpath, 0, NULL)) {
        snprintf(lpath, 4095, "%s/%s", path, NETDATA_EBPF_OLD_CONFIG_FILE);
        if (!appconfig_load(&collector_config, lpath, 0, NULL)) {
            return -1;
        }
        origin = EBPF_LOADED_FROM_STOCK;
    } else
        origin = EBPF_LOADED_FROM_USER;

    read_collector_values(disable_cgroups, update_every, origin);

    return 0;
}

/**
 * Set global variables reading environment variables
 */
void set_global_variables()
{
    // Get environment variables
    ebpf_plugin_dir = getenv("NETDATA_PLUGINS_DIR");
    if (!ebpf_plugin_dir)
        ebpf_plugin_dir = PLUGINS_DIR;

    ebpf_user_config_dir = getenv("NETDATA_USER_CONFIG_DIR");
    if (!ebpf_user_config_dir)
        ebpf_user_config_dir = CONFIG_DIR;

    ebpf_stock_config_dir = getenv("NETDATA_STOCK_CONFIG_DIR");
    if (!ebpf_stock_config_dir)
        ebpf_stock_config_dir = LIBCONFIG_DIR;

    ebpf_configured_log_dir = getenv("NETDATA_LOG_DIR");
    if (!ebpf_configured_log_dir)
        ebpf_configured_log_dir = LOG_DIR;

    ebpf_nprocs = (int)sysconf(_SC_NPROCESSORS_ONLN);
    if (ebpf_nprocs < 0) {
        ebpf_nprocs = NETDATA_MAX_PROCESSOR;
        netdata_log_error("Cannot identify number of process, using default value %d", ebpf_nprocs);
    }

    isrh = get_redhat_release();
    pid_max = get_system_pid_max();
    running_on_kernel = ebpf_get_kernel_version();
}

/**
 * Load collector config
 */
static inline void ebpf_load_thread_config()
{
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_update_module(&ebpf_modules[i], default_btf, running_on_kernel, isrh);
    }
}

/**
 * Check Conditions
 *
 * This function checks kernel that plugin is running and permissions.
 *
 * @return It returns 0 on success and -1 otherwise
 */
int ebpf_check_conditions()
{
    if (!has_condition_to_run(running_on_kernel)) {
        netdata_log_error("The current collector cannot run on this kernel.");
        return -1;
    }

    if (!am_i_running_as_root()) {
        netdata_log_error(
            "ebpf.plugin should either run as root (now running with uid %u, euid %u) or have special capabilities..",
            (unsigned int)getuid(), (unsigned int)geteuid());
        return -1;
    }

    return 0;
}

/**
 * Adjust memory
 *
 * Adjust memory values to load eBPF programs.
 *
 * @return It returns 0 on success and -1 otherwise
 */
int ebpf_adjust_memory_limit()
{
    struct rlimit r = { RLIM_INFINITY, RLIM_INFINITY };
    if (setrlimit(RLIMIT_MEMLOCK, &r)) {
        netdata_log_error("Setrlimit(RLIMIT_MEMLOCK)");
        return -1;
    }

    return 0;
}

/**
 * Parse arguments given from user.
 *
 * @param argc the number of arguments
 * @param argv the pointer to the arguments
 */
static void ebpf_parse_args(int argc, char **argv)
{
    int disable_cgroups = 1;
    int freq = 0;
    int option_index = 0;
    uint64_t select_threads = 0;
    static struct option long_options[] = {
        {"process",        no_argument,    0,  0 },
        {"net",            no_argument,    0,  0 },
        {"cachestat",      no_argument,    0,  0 },
        {"sync",           no_argument,    0,  0 },
        {"dcstat",         no_argument,    0,  0 },
        {"swap",           no_argument,    0,  0 },
        {"vfs",            no_argument,    0,  0 },
        {"filesystem",     no_argument,    0,  0 },
        {"disk",           no_argument,    0,  0 },
        {"mount",          no_argument,    0,  0 },
        {"filedescriptor", no_argument,    0,  0 },
        {"hardirq",        no_argument,    0,  0 },
        {"softirq",        no_argument,    0,  0 },
        {"oomkill",        no_argument,    0,  0 },
        {"shm",            no_argument,    0,  0 },
        {"mdflush",        no_argument,    0,  0 },
        /* INSERT NEW THREADS BEFORE THIS COMMENT TO KEEP COMPATIBILITY WITH enum ebpf_module_indexes */
        {"all",            no_argument,    0,  0 },
        {"version",        no_argument,    0,  0 },
        {"help",           no_argument,    0,  0 },
        {"global",         no_argument,    0,  0 },
        {"return",         no_argument,    0,  0 },
        {"legacy",         no_argument,    0,  0 },
        {"core",           no_argument,    0,  0 },
        {"unittest",       no_argument,    0,  0 },
        {0, 0, 0, 0}
    };

    memset(&network_viewer_opt, 0, sizeof(network_viewer_opt));
    network_viewer_opt.max_dim = NETDATA_NV_CAP_VALUE;

    if (argc > 1) {
        int n = (int)str2l(argv[1]);
        if (n > 0) {
            freq = n;
        }
    }

    if (!freq)
        freq = EBPF_DEFAULT_UPDATE_EVERY;

    if (ebpf_load_collector_config(ebpf_user_config_dir, &disable_cgroups, freq)) {
        netdata_log_info(
            "Does not have a configuration file inside `%s/ebpf.d.conf. It will try to load stock file.",
            ebpf_user_config_dir);
        if (ebpf_load_collector_config(ebpf_stock_config_dir, &disable_cgroups, freq)) {
            netdata_log_info("Does not have a stock file. It is starting with default options.");
        }
    }

    ebpf_load_thread_config();

    while (1) {
        int c = getopt_long_only(argc, argv, "", long_options, &option_index);
        if (c == -1)
            break;

        switch (option_index) {
            case EBPF_MODULE_PROCESS_IDX: {
                select_threads |= 1<<EBPF_MODULE_PROCESS_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"PROCESS\" charts, because it was started with the option \"[-]-process\".");
#endif
                break;
            }
            case EBPF_MODULE_SOCKET_IDX: {
                select_threads |= 1<<EBPF_MODULE_SOCKET_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"NET\" charts, because it was started with the option \"[-]-net\".");
#endif
                break;
            }
            case EBPF_MODULE_CACHESTAT_IDX: {
                select_threads |= 1<<EBPF_MODULE_CACHESTAT_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"CACHESTAT\" charts, because it was started with the option \"[-]-cachestat\".");
#endif
                break;
            }
            case EBPF_MODULE_SYNC_IDX: {
                select_threads |= 1<<EBPF_MODULE_SYNC_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"SYNC\" chart, because it was started with the option \"[-]-sync\".");
#endif
                break;
            }
            case EBPF_MODULE_DCSTAT_IDX: {
                select_threads |= 1<<EBPF_MODULE_DCSTAT_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"DCSTAT\" charts, because it was started with the option \"[-]-dcstat\".");
#endif
                break;
            }
            case EBPF_MODULE_SWAP_IDX: {
                select_threads |= 1<<EBPF_MODULE_SWAP_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"SWAP\" chart, because it was started with the option \"[-]-swap\".");
#endif
                break;
            }
            case EBPF_MODULE_VFS_IDX: {
                select_threads |= 1<<EBPF_MODULE_VFS_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"VFS\" chart, because it was started with the option \"[-]-vfs\".");
#endif
                break;
            }
            case EBPF_MODULE_FILESYSTEM_IDX: {
                select_threads |= 1<<EBPF_MODULE_FILESYSTEM_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"FILESYSTEM\" chart, because it was started with the option \"[-]-filesystem\".");
#endif
                break;
            }
            case EBPF_MODULE_DISK_IDX: {
                select_threads |= 1<<EBPF_MODULE_DISK_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"DISK\" chart, because it was started with the option \"[-]-disk\".");
#endif
                break;
            }
            case EBPF_MODULE_MOUNT_IDX: {
                select_threads |= 1<<EBPF_MODULE_MOUNT_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"MOUNT\" chart, because it was started with the option \"[-]-mount\".");
#endif
                break;
            }
            case EBPF_MODULE_FD_IDX: {
                select_threads |= 1<<EBPF_MODULE_FD_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"FILEDESCRIPTOR\" chart, because it was started with the option \"[-]-filedescriptor\".");
#endif
                break;
            }
            case EBPF_MODULE_HARDIRQ_IDX: {
                select_threads |= 1<<EBPF_MODULE_HARDIRQ_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"HARDIRQ\" chart, because it was started with the option \"[-]-hardirq\".");
#endif
                break;
            }
            case EBPF_MODULE_SOFTIRQ_IDX: {
                select_threads |= 1<<EBPF_MODULE_SOFTIRQ_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"SOFTIRQ\" chart, because it was started with the option \"[-]-softirq\".");
#endif
                break;
            }
            case EBPF_MODULE_OOMKILL_IDX: {
                select_threads |= 1<<EBPF_MODULE_OOMKILL_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"OOMKILL\" chart, because it was started with the option \"[-]-oomkill\".");
#endif
                break;
            }
            case EBPF_MODULE_SHM_IDX: {
                select_threads |= 1<<EBPF_MODULE_SHM_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"SHM\" chart, because it was started with the option \"[-]-shm\".");
#endif
                break;
            }
            case EBPF_MODULE_MDFLUSH_IDX: {
                select_threads |= 1<<EBPF_MODULE_MDFLUSH_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"MDFLUSH\" chart, because it was started with the option \"[-]-mdflush\".");
#endif
                break;
            }
            case EBPF_OPTION_ALL_CHARTS: {
                ebpf_set_apps_mode(NETDATA_EBPF_APPS_FLAG_YES);
                disable_cgroups = 0;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF running with all chart groups, because it was started with the option \"[-]-all\".");
#endif
                break;
            }
            case EBPF_OPTION_VERSION: {
                printf("ebpf.plugin %s\n", VERSION);
                exit(0);
            }
            case EBPF_OPTION_HELP: {
                ebpf_print_help();
                exit(0);
            }
            case EBPF_OPTION_GLOBAL_CHART: {
                disable_cgroups = 1;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF running with global chart group, because it was started with the option  \"[-]-global\".");
#endif
                break;
            }
            case EBPF_OPTION_RETURN_MODE: {
                ebpf_set_thread_mode(MODE_RETURN);
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF running in \"RETURN\" mode, because it was started with the option \"[-]-return\".");
#endif
                break;
            }
            case EBPF_OPTION_LEGACY: {
                ebpf_set_load_mode(EBPF_LOAD_LEGACY, EBPF_LOADED_FROM_USER);
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF running with \"LEGACY\" code, because it was started with the option \"[-]-legacy\".");
#endif
                break;
            }
            case EBPF_OPTION_CORE: {
                ebpf_set_load_mode(EBPF_LOAD_CORE, EBPF_LOADED_FROM_USER);
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF running with \"CO-RE\" code, because it was started with the option \"[-]-core\".");
#endif
                break;
            }
            case EBPF_OPTION_UNITTEST: {
                // if we cannot run until the end, we will cancel the unittest
                int exit_code = ECANCELED;
                if (ebpf_check_conditions())
                    goto unittest;

                if (ebpf_adjust_memory_limit())
                    goto unittest;

                // Load binary in entry mode
                ebpf_ut_initialize_structure(MODE_ENTRY);
                if (ebpf_ut_load_real_binary())
                    goto unittest;

                ebpf_ut_cleanup_memory();

                // Do not load a binary in entry mode
                ebpf_ut_initialize_structure(MODE_ENTRY);
                if (ebpf_ut_load_fake_binary())
                    goto unittest;

                ebpf_ut_cleanup_memory();

                exit_code = 0;
unittest:
                exit(exit_code);
            }
            default: {
                break;
            }
        }
    }

    if (disable_cgroups) {
        if (disable_cgroups)
            ebpf_disable_cgroups();
    }

    if (select_threads) {
        disable_all_global_charts();
        uint64_t idx;
        for (idx = 0; idx < EBPF_OPTION_ALL_CHARTS; idx++) {
            if (select_threads & 1<<idx)
                ebpf_enable_specific_chart(&ebpf_modules[idx], disable_cgroups);
        }
    }

    // Load apps_groups.conf
    if (ebpf_read_apps_groups_conf(
            &apps_groups_default_target, &apps_groups_root_target, ebpf_user_config_dir, "groups")) {
        netdata_log_info("Cannot read process groups configuration file '%s/apps_groups.conf'. Will try '%s/apps_groups.conf'",
             ebpf_user_config_dir, ebpf_stock_config_dir);
        if (ebpf_read_apps_groups_conf(
                &apps_groups_default_target, &apps_groups_root_target, ebpf_stock_config_dir, "groups")) {
            netdata_log_error("Cannot read process groups '%s/apps_groups.conf'. There are no internal defaults. Failing.",
                  ebpf_stock_config_dir);
            ebpf_exit();
        }
    } else
        netdata_log_info("Loaded config file '%s/apps_groups.conf'", ebpf_user_config_dir);
}

/*****************************************************************
 *
 *  Collector charts
 *
 *****************************************************************/

static char *load_event_stat[NETDATA_EBPF_LOAD_STAT_END] = {"legacy", "co-re"};
static char *memlock_stat = {"memory_locked"};
static char *hash_table_stat = {"hash_table"};
static char *hash_table_core[NETDATA_EBPF_LOAD_STAT_END] = {"per_core", "unique"};

/**
 * Send Hash Table PID data
 *
 * Send all information associated with a specific pid table.
 *
 * @param chart   chart id
 * @param idx     index position in hash_table_stats
 */
static inline void ebpf_send_hash_table_pid_data(char *chart, uint32_t idx)
{
    int i;
    write_begin_chart(NETDATA_MONITORING_FAMILY, chart);
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_module_t *wem = &ebpf_modules[i];
        if (wem->apps_routine)
            write_chart_dimension((char *)wem->thread_name,
                                  (wem->enabled < NETDATA_THREAD_EBPF_STOPPING) ?
                                  wem->hash_table_stats[idx]:
                                  0);
    }
    write_end_chart();
}

/**
 * Send Global Hash Table data
 *
 * Send all information associated with a specific pid table.
 *
 */
static inline void ebpf_send_global_hash_table_data()
{
    int i;
    write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_HASH_TABLES_GLOBAL_ELEMENTS);
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_module_t *wem = &ebpf_modules[i];
        write_chart_dimension((char *)wem->thread_name,
                              (wem->enabled < NETDATA_THREAD_EBPF_STOPPING) ? NETDATA_CONTROLLER_END: 0);
    }
    write_end_chart();
}

/**
 * Send Statistic Data
 *
 * Send statistic information to netdata.
 */
void ebpf_send_statistic_data()
{
    if (!publish_internal_metrics)
        return;

    write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_THREADS);
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_module_t *wem = &ebpf_modules[i];
        write_chart_dimension((char *)wem->thread_name, (wem->enabled < NETDATA_THREAD_EBPF_STOPPING) ? 1 : 0);
    }
    write_end_chart();

    write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_LIFE_TIME);
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX ; i++) {
        ebpf_module_t *wem = &ebpf_modules[i];
        // Threads like VFS is slow to load and this can create an invalid number, this is the motive
        // we are also testing wem->lifetime value.
        write_chart_dimension((char *)wem->thread_name,
                              (wem->lifetime && wem->enabled < NETDATA_THREAD_EBPF_STOPPING) ?
                              (long long) (wem->lifetime - wem->running_time):
                              0) ;
    }
    write_end_chart();

    write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_LOAD_METHOD);
    write_chart_dimension(load_event_stat[NETDATA_EBPF_LOAD_STAT_LEGACY], (long long)plugin_statistics.legacy);
    write_chart_dimension(load_event_stat[NETDATA_EBPF_LOAD_STAT_CORE], (long long)plugin_statistics.core);
    write_end_chart();

    write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_KERNEL_MEMORY);
    write_chart_dimension(memlock_stat, (long long)plugin_statistics.memlock_kern);
    write_end_chart();

    write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_HASH_TABLES_LOADED);
    write_chart_dimension(hash_table_stat, (long long)plugin_statistics.hash_tables);
    write_end_chart();

    write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_HASH_TABLES_PER_CORE);
    write_chart_dimension(hash_table_core[NETDATA_EBPF_THREAD_PER_CORE], (long long)plugin_statistics.hash_percpu);
    write_chart_dimension(hash_table_core[NETDATA_EBPF_THREAD_UNIQUE], (long long)plugin_statistics.hash_unique);
    write_end_chart();

    ebpf_send_global_hash_table_data();

    ebpf_send_hash_table_pid_data(NETDATA_EBPF_HASH_TABLES_INSERT_PID_ELEMENTS, NETDATA_EBPF_GLOBAL_TABLE_PID_TABLE_ADD);
    ebpf_send_hash_table_pid_data(NETDATA_EBPF_HASH_TABLES_REMOVE_PID_ELEMENTS, NETDATA_EBPF_GLOBAL_TABLE_PID_TABLE_DEL);
}

/**
 * Update Internal Metric variable
 *
 * By default eBPF.plugin sends internal metrics for netdata, but user can
 * disable this.
 *
 * The function updates the variable used to send charts.
 */
static void update_internal_metric_variable()
{
    const char *s = getenv("NETDATA_INTERNALS_MONITORING");
    if (s && *s && strcmp(s, "NO") == 0)
        publish_internal_metrics = false;
}

/**
 * Create chart for Statistic Thread
 *
 * Write to standard output current values for threads.
 *
 * @param update_every time used to update charts
 */
static inline void ebpf_create_statistic_thread_chart(int update_every)
{
    ebpf_write_chart_cmd(NETDATA_MONITORING_FAMILY,
                         NETDATA_EBPF_THREADS,
                         "Threads running.",
                         "boolean",
                         NETDATA_EBPF_FAMILY,
                         NETDATA_EBPF_CHART_TYPE_LINE,
                         NULL,
                         NETDATA_EBPF_ORDER_STAT_THREADS,
                         update_every,
                         NETDATA_EBPF_MODULE_NAME_PROCESS);

    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_write_global_dimension((char *)ebpf_modules[i].thread_name,
                                    (char *)ebpf_modules[i].thread_name,
                                    ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
    }
}

/**
 * Create lifetime Thread Chart
 *
 * Write to standard output current values for threads lifetime.
 *
 * @param update_every time used to update charts
 */
static inline void ebpf_create_lifetime_thread_chart(int update_every)
{
    ebpf_write_chart_cmd(NETDATA_MONITORING_FAMILY,
                         NETDATA_EBPF_LIFE_TIME,
                         "Threads running.",
                         "seconds",
                         NETDATA_EBPF_FAMILY,
                         NETDATA_EBPF_CHART_TYPE_LINE,
                         NULL,
                         NETDATA_EBPF_ORDER_STAT_LIFE_TIME,
                         update_every,
                         NETDATA_EBPF_MODULE_NAME_PROCESS);

    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_write_global_dimension((char *)ebpf_modules[i].thread_name,
                                    (char *)ebpf_modules[i].thread_name,
                                    ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
    }
}

/**
 * Create chart for Load Thread
 *
 * Write to standard output current values for load mode.
 *
 * @param update_every time used to update charts
 */
static inline void ebpf_create_statistic_load_chart(int update_every)
{
    ebpf_write_chart_cmd(NETDATA_MONITORING_FAMILY,
                         NETDATA_EBPF_LOAD_METHOD,
                         "Load info.",
                         "methods",
                         NETDATA_EBPF_FAMILY,
                         NETDATA_EBPF_CHART_TYPE_LINE,
                         NULL,
                         NETDATA_EBPF_ORDER_STAT_LOAD_METHOD,
                         update_every,
                         NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(load_event_stat[NETDATA_EBPF_LOAD_STAT_LEGACY],
                                load_event_stat[NETDATA_EBPF_LOAD_STAT_LEGACY],
                                ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    ebpf_write_global_dimension(load_event_stat[NETDATA_EBPF_LOAD_STAT_CORE],
                                load_event_stat[NETDATA_EBPF_LOAD_STAT_CORE],
                                ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
}

/**
 * Create chart for Kernel Memory
 *
 * Write to standard output current values for allocated memory.
 *
 * @param update_every time used to update charts
 */
static inline void ebpf_create_statistic_kernel_memory(int update_every)
{
    ebpf_write_chart_cmd(NETDATA_MONITORING_FAMILY,
                         NETDATA_EBPF_KERNEL_MEMORY,
                         "Memory allocated for hash tables.",
                         "bytes",
                         NETDATA_EBPF_FAMILY,
                         NETDATA_EBPF_CHART_TYPE_LINE,
                         NULL,
                         NETDATA_EBPF_ORDER_STAT_KERNEL_MEMORY,
                         update_every,
                         NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(memlock_stat,
                                memlock_stat,
                                ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
}

/**
 * Create chart Hash Table
 *
 * Write to standard output number of hash tables used with this software.
 *
 * @param update_every time used to update charts
 */
static inline void ebpf_create_statistic_hash_tables(int update_every)
{
    ebpf_write_chart_cmd(NETDATA_MONITORING_FAMILY,
                         NETDATA_EBPF_HASH_TABLES_LOADED,
                         "Number of hash tables loaded.",
                         "hash tables",
                         NETDATA_EBPF_FAMILY,
                         NETDATA_EBPF_CHART_TYPE_LINE,
                         NULL,
                         NETDATA_EBPF_ORDER_STAT_HASH_TABLES,
                         update_every,
                         NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(hash_table_stat,
                                hash_table_stat,
                                ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
}

/**
 * Create chart for percpu stats
 *
 * Write to standard output current values for threads.
 *
 * @param update_every time used to update charts
 */
static inline void ebpf_create_statistic_hash_per_core(int update_every)
{
    ebpf_write_chart_cmd(NETDATA_MONITORING_FAMILY,
                         NETDATA_EBPF_HASH_TABLES_PER_CORE,
                         "How threads are loading hash/array tables.",
                         "threads",
                         NETDATA_EBPF_FAMILY,
                         NETDATA_EBPF_CHART_TYPE_LINE,
                         NULL,
                         NETDATA_EBPF_ORDER_STAT_HASH_CORE,
                         update_every,
                         NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(hash_table_core[NETDATA_EBPF_THREAD_PER_CORE],
                                hash_table_core[NETDATA_EBPF_THREAD_PER_CORE],
                                ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    ebpf_write_global_dimension(hash_table_core[NETDATA_EBPF_THREAD_UNIQUE],
                                hash_table_core[NETDATA_EBPF_THREAD_UNIQUE],
                                ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
}

/**
 * Hash table global elements
 *
 * Write to standard output current values inside global tables.
 *
 * @param update_every time used to update charts
 */
static void ebpf_create_statistic_hash_global_elements(int update_every)
{
    ebpf_write_chart_cmd(NETDATA_MONITORING_FAMILY,
                         NETDATA_EBPF_HASH_TABLES_GLOBAL_ELEMENTS,
                         "Controllers inside global table",
                         "rows",
                         NETDATA_EBPF_FAMILY,
                         NETDATA_EBPF_CHART_TYPE_LINE,
                         NULL,
                         NETDATA_EBPF_ORDER_STAT_HASH_GLOBAL_TABLE_TOTAL,
                         update_every,
                         NETDATA_EBPF_MODULE_NAME_PROCESS);

    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_write_global_dimension((char *)ebpf_modules[i].thread_name,
                                    (char *)ebpf_modules[i].thread_name,
                                    ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
    }
}

/**
 * Hash table global elements
 *
 * Write to standard output current values inside global tables.
 *
 * @param update_every time used to update charts
 * @param id           chart id
 * @param title        chart title
 * @param order        ordder chart will be shown on dashboard.
 */
static void ebpf_create_statistic_hash_pid_table(int update_every, char *id, char *title, int order)
{
    ebpf_write_chart_cmd(NETDATA_MONITORING_FAMILY,
                         id,
                         title,
                         "rows",
                         NETDATA_EBPF_FAMILY,
                         NETDATA_EBPF_CHART_TYPE_LINE,
                         NULL,
                         order,
                         update_every,
                         NETDATA_EBPF_MODULE_NAME_PROCESS);

    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_module_t *wem = &ebpf_modules[i];
        if (wem->apps_routine)
            ebpf_write_global_dimension((char *)wem->thread_name,
                                        (char *)wem->thread_name,
                                        ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
    }
}

/**
 * Create Statistics Charts
 *
 * Create charts that will show statistics related to eBPF plugin.
 *
 * @param update_every time used to update charts
 */
static void ebpf_create_statistic_charts(int update_every)
{
    static char create_charts = 1;
    update_internal_metric_variable();
    if (!publish_internal_metrics)
        return;

    if (!create_charts)
        return;

    create_charts = 0;

    ebpf_create_statistic_thread_chart(update_every);
#ifdef NETDATA_DEV_MODE
    EBPF_PLUGIN_FUNCTIONS(EBPF_FUNCTION_THREAD, EBPF_PLUGIN_THREAD_FUNCTION_DESCRIPTION);
#endif

    ebpf_create_lifetime_thread_chart(update_every);
#ifdef NETDATA_DEV_MODE
    EBPF_PLUGIN_FUNCTIONS(EBPF_FUNCTION_THREAD, EBPF_PLUGIN_THREAD_FUNCTION_DESCRIPTION);
#endif

    ebpf_create_statistic_load_chart(update_every);

    ebpf_create_statistic_kernel_memory(update_every);

    ebpf_create_statistic_hash_tables(update_every);

    ebpf_create_statistic_hash_per_core(update_every);

    ebpf_create_statistic_hash_global_elements(update_every);

    ebpf_create_statistic_hash_pid_table(update_every,
                                         NETDATA_EBPF_HASH_TABLES_INSERT_PID_ELEMENTS,
                                         "Elements inserted into PID table",
                                         NETDATA_EBPF_ORDER_STAT_HASH_PID_TABLE_ADDED);

    ebpf_create_statistic_hash_pid_table(update_every,
                                         NETDATA_EBPF_HASH_TABLES_REMOVE_PID_ELEMENTS,
                                         "Elements removed from PID table",
                                         NETDATA_EBPF_ORDER_STAT_HASH_PID_TABLE_REMOVED);

    fflush(stdout);
}

/*****************************************************************
 *
 *  COLLECTOR ENTRY POINT
 *
 *****************************************************************/

/**
 * Update PID file
 *
 * Update the content of PID file
 *
 * @param filename is the full name of the file.
 * @param pid that identifies the process
 */
static void ebpf_update_pid_file(char *filename, pid_t pid)
{
    FILE *fp = fopen(filename, "w");
    if (!fp)
        return;

    fprintf(fp, "%d", pid);
    fclose(fp);
}

/**
 * Get Process Name
 *
 * Get process name from /proc/PID/status
 *
 * @param pid that identifies the process
 */
static char *ebpf_get_process_name(pid_t pid)
{
    char *name = NULL;
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "/proc/%d/status", pid);

    procfile *ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
    if(unlikely(!ff)) {
        netdata_log_error("Cannot open %s", filename);
        return name;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return name;

    unsigned long i, lines = procfile_lines(ff);
    for(i = 0; i < lines ; i++) {
        char *cmp = procfile_lineword(ff, i, 0);
        if (!strcmp(cmp, "Name:")) {
            name = strdupz(procfile_lineword(ff, i, 1));
            break;
        }
    }

    procfile_close(ff);

    return name;
}

/**
 * Read Previous PID
 *
 * @param filename is the full name of the file.
 *
 * @return It returns the PID used during previous execution on success or 0 otherwise
 */
static pid_t ebpf_read_previous_pid(char *filename)
{
    FILE *fp = fopen(filename, "r");
    if (!fp)
        return 0;

    char buffer[64];
    size_t length = fread(buffer, sizeof(*buffer), 63, fp);
    pid_t old_pid = 0;
    if (length) {
        if (length > 63)
            length = 63;

        buffer[length] = '\0';
        old_pid = (pid_t) str2uint32_t(buffer, NULL);
    }
    fclose(fp);

    return old_pid;
}

/**
 * Kill previous process
 *
 * Kill previous process whether it was not closed.
 *
 * @param filename is the full name of the file.
 * @param pid that identifies the process
 */
static void ebpf_kill_previous_process(char *filename, pid_t pid)
{
    pid_t old_pid =  ebpf_read_previous_pid(filename);
    if (!old_pid)
        return;

    // Process is not running
    char *prev_name =  ebpf_get_process_name(old_pid);
    if (!prev_name)
        return;

    char *current_name =  ebpf_get_process_name(pid);

    if (!strcmp(prev_name, current_name))
        kill(old_pid, SIGKILL);

    freez(prev_name);
    freez(current_name);

    // wait few microseconds before start new plugin
    sleep_usec(USEC_PER_MS * 300);
}

/**
 * PID file
 *
 * Write the filename for PID inside the given vector.
 *
 * @param filename  vector where we will store the name.
 * @param length    number of bytes available in filename vector
 */
void ebpf_pid_file(char *filename, size_t length)
{
    snprintfz(filename, length, "%s%s/ebpf.d/ebpf.pid", netdata_configured_host_prefix, ebpf_plugin_dir);
}

/**
 * Manage PID
 *
 * This function kills another instance of eBPF whether it is necessary and update the file content.
 *
 * @param pid that identifies the process
 */
static void ebpf_manage_pid(pid_t pid)
{
    char filename[FILENAME_MAX + 1];
    ebpf_pid_file(filename, FILENAME_MAX);

    ebpf_kill_previous_process(filename, pid);
    ebpf_update_pid_file(filename, pid);
}

/**
 * Set start routine
 *
 * Set static routine before threads to be created.
 */
 static void ebpf_set_static_routine()
 {
     int i;
     for (i = 0; ebpf_modules[i].thread_name; i++) {
         ebpf_threads[i].start_routine = ebpf_modules[i].start_routine;
     }
 }

/**
 * Entry point
 *
 * @param argc the number of arguments
 * @param argv the pointer to the arguments
 *
 * @return it returns 0 on success and another integer otherwise
 */
int main(int argc, char **argv)
{
    stderror = stderr;
    clocks_init();
    main_thread_id = gettid();

    set_global_variables();
    ebpf_parse_args(argc, argv);
    ebpf_manage_pid(getpid());

    if (ebpf_check_conditions())
        return 2;

    // set name
    program_name = "ebpf.plugin";

    // disable syslog
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;

    if (ebpf_adjust_memory_limit())
        return 3;

    signal(SIGINT, ebpf_stop_threads);
    signal(SIGQUIT, ebpf_stop_threads);
    signal(SIGTERM, ebpf_stop_threads);
    signal(SIGPIPE, ebpf_stop_threads);

    ebpf_start_pthread_variables();

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(verify_netdata_host_prefix() == -1) ebpf_exit(6);

    ebpf_allocate_common_vectors();

#ifdef LIBBPF_MAJOR_VERSION
    libbpf_set_strict_mode(LIBBPF_STRICT_ALL);
#endif

    read_local_addresses();
    read_local_ports("/proc/net/tcp", IPPROTO_TCP);
    read_local_ports("/proc/net/tcp6", IPPROTO_TCP);
    read_local_ports("/proc/net/udp", IPPROTO_UDP);
    read_local_ports("/proc/net/udp6", IPPROTO_UDP);

    ebpf_set_static_routine();

    cgroup_integration_thread.thread = mallocz(sizeof(netdata_thread_t));
    cgroup_integration_thread.start_routine = ebpf_cgroup_integration;

    netdata_thread_create(cgroup_integration_thread.thread, cgroup_integration_thread.name,
                          NETDATA_THREAD_OPTION_DEFAULT, ebpf_cgroup_integration, NULL);

    int i;
    for (i = 0; ebpf_threads[i].name != NULL; i++) {
        struct netdata_static_thread *st = &ebpf_threads[i];

        ebpf_module_t *em = &ebpf_modules[i];
        em->thread = st;
        em->thread_id = i;
        if (em->enabled) {
            st->thread = mallocz(sizeof(netdata_thread_t));
            em->enabled = NETDATA_THREAD_EBPF_RUNNING;
            em->lifetime = EBPF_NON_FUNCTION_LIFE_TIME;
            netdata_thread_create(st->thread, st->name, NETDATA_THREAD_OPTION_DEFAULT, st->start_routine, em);
        } else {
            em->enabled = NETDATA_THREAD_EBPF_NOT_RUNNING;
        }
    }

    usec_t step = USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    int update_apps_every = (int) EBPF_CFG_UPDATE_APPS_EVERY_DEFAULT;
    int update_apps_list = update_apps_every - 1;
    int process_maps_per_core = ebpf_modules[EBPF_MODULE_PROCESS_IDX].maps_per_core;
    //Plugin will be killed when it receives a signal
    for ( ; !ebpf_exit_plugin ; global_iterations_counter++) {
        (void)heartbeat_next(&hb, step);

        if (global_iterations_counter % EBPF_DEFAULT_UPDATE_EVERY == 0) {
            pthread_mutex_lock(&lock);
            ebpf_create_statistic_charts(EBPF_DEFAULT_UPDATE_EVERY);

            ebpf_send_statistic_data();
            pthread_mutex_unlock(&lock);
            fflush(stdout);
        }

        pthread_mutex_lock(&ebpf_exit_cleanup);
        pthread_mutex_lock(&collect_data_mutex);
        if (++update_apps_list == update_apps_every) {
            update_apps_list = 0;
            cleanup_exited_pids();
            collect_data_for_all_processes(process_pid_fd, process_maps_per_core);

            pthread_mutex_lock(&lock);
            ebpf_create_apps_charts(apps_groups_root_target);
            pthread_mutex_unlock(&lock);
        }
        pthread_mutex_unlock(&collect_data_mutex);
        pthread_mutex_unlock(&ebpf_exit_cleanup);
    }

    ebpf_stop_threads(0);

    return 0;
}
