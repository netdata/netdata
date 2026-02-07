// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/time.h>
#include <sys/resource.h>
#include <ifaddrs.h>
#include <errno.h>

#include "ebpf.h"
#include "ebpf_socket.h"
#include "ebpf_unittest.h"
#include "ebpf_library.h"
#include "libnetdata/required_dummies.h"

/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

char *ebpf_plugin_dir = PLUGINS_DIR;
static char *ebpf_configured_log_dir = LOG_DIR;

char *ebpf_algorithms[] = {EBPF_CHART_ALGORITHM_ABSOLUTE, EBPF_CHART_ALGORITHM_INCREMENTAL};
struct config collector_config = APPCONFIG_INITIALIZER;

int running_on_kernel = 0;
int ebpf_nprocs;
int isrh = 0;
int main_thread_id = 0;
int process_pid_fd = -1;
uint64_t collect_pids = 0;
uint32_t integration_with_collectors = NETDATA_EBPF_INTEGRATION_DISABLED;
ND_THREAD *socket_ipc = NULL;
static size_t global_iterations_counter = 1;
bool publish_internal_metrics = true;

netdata_mutex_t lock;
netdata_mutex_t ebpf_exit_cleanup;
netdata_mutex_t collect_data_mutex;

struct netdata_static_thread cgroup_integration_thread = {
    .name = "EBPF CGROUP INT",
    .config_section = NULL,
    .config_name = NULL,
    .env_name = NULL,
    .enabled = 1,
    .thread = NULL,
    .init_routine = NULL,
    .start_routine = NULL};

ebpf_module_t ebpf_modules[] = {
    {.info =
         {.thread_name = "process", .config_name = "process", .thread_description = NETDATA_EBPF_MODULE_PROCESS_DESC},
     .functions =
         {.start_routine = ebpf_process_thread, .apps_routine = ebpf_process_create_apps_charts, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &process_config,
     .config_file = NETDATA_PROCESS_CONFIG_FILE,
     .kernels =
         NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_10 | NETDATA_V5_14,
     .load = EBPF_LOAD_LEGACY,
     .targets = process_targets,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info = {.thread_name = "socket", .config_name = "socket", .thread_description = NETDATA_EBPF_SOCKET_MODULE_DESC},
     .functions =
         {.start_routine = ebpf_socket_thread,
          .apps_routine = ebpf_socket_create_apps_charts,
          .fnct_routine = ebpf_socket_read_open_connections,
          .fcnt_name = EBPF_FUNCTION_SOCKET,
          .fcnt_desc = EBPF_PLUGIN_SOCKET_FUNCTION_DESCRIPTION,
          .fcnt_thread_chart_name = NULL,
          .fcnt_thread_lifetime_name = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &socket_config,
     .config_file = NETDATA_NETWORK_CONFIG_FILE,
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
     .load = EBPF_LOAD_LEGACY,
     .targets = socket_targets,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info =
         {.thread_name = "cachestat",
          .config_name = "cachestat",
          .thread_description = NETDATA_EBPF_CACHESTAT_MODULE_DESC},
     .functions =
         {.start_routine = ebpf_cachestat_thread,
          .apps_routine = ebpf_cachestat_create_apps_charts,
          .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = cachestat_maps,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &cachestat_config,
     .config_file = NETDATA_CACHESTAT_CONFIG_FILE,
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14 |
                NETDATA_V5_15 | NETDATA_V5_16,
     .load = EBPF_LOAD_LEGACY,
     .targets = cachestat_targets,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info = {.thread_name = "sync", .config_name = "sync", .thread_description = NETDATA_EBPF_SYNC_MODULE_DESC},
     .functions = {.start_routine = ebpf_sync_thread, .apps_routine = NULL, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .maps = NULL,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_NOT_SET,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &sync_config,
     .config_file = NETDATA_SYNC_CONFIG_FILE,
     // All syscalls have the same kernels
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
     .load = EBPF_LOAD_LEGACY,
     .targets = sync_targets,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info = {.thread_name = "dc", .config_name = "dc", .thread_description = NETDATA_EBPF_DC_MODULE_DESC},
     .functions =
         {.start_routine = ebpf_dcstat_thread, .apps_routine = ebpf_dcstat_create_apps_charts, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = dcstat_maps,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &dcstat_config,
     .config_file = NETDATA_DIRECTORY_DCSTAT_CONFIG_FILE,
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
     .load = EBPF_LOAD_LEGACY,
     .targets = dc_targets,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info = {.thread_name = "swap", .config_name = "swap", .thread_description = NETDATA_EBPF_SWAP_MODULE_DESC},
     .functions =
         {.start_routine = ebpf_swap_thread, .apps_routine = ebpf_swap_create_apps_charts, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &swap_config,
     .config_file = NETDATA_DIRECTORY_SWAP_CONFIG_FILE,
     .kernels =
         NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14 | NETDATA_V6_8,
     .load = EBPF_LOAD_LEGACY,
     .targets = swap_targets,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info = {.thread_name = "vfs", .config_name = "vfs", .thread_description = NETDATA_EBPF_VFS_MODULE_DESC},
     .functions = {.start_routine = ebpf_vfs_thread, .apps_routine = ebpf_vfs_create_apps_charts, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &vfs_config,
     .config_file = NETDATA_DIRECTORY_VFS_CONFIG_FILE,
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
     .load = EBPF_LOAD_LEGACY,
     .targets = vfs_targets,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info =
         {.thread_name = "filesystem", .config_name = "filesystem", .thread_description = NETDATA_EBPF_FS_MODULE_DESC},
     .functions = {.start_routine = ebpf_filesystem_thread, .apps_routine = NULL, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_NOT_SET,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &fs_config,
     .config_file = NETDATA_FILESYSTEM_CONFIG_FILE,
     //We are setting kernels as zero, because we load eBPF programs according the kernel running.
     .kernels = 0,
     .load = EBPF_LOAD_LEGACY,
     .targets = NULL,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info = {.thread_name = "disk", .config_name = "disk", .thread_description = NETDATA_EBPF_DISK_MODULE_DESC},
     .functions = {.start_routine = ebpf_disk_thread, .apps_routine = NULL, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_NOT_SET,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &disk_config,
     .config_file = NETDATA_DISK_CONFIG_FILE,
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
     .load = EBPF_LOAD_LEGACY,
     .targets = NULL,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info = {.thread_name = "mount", .config_name = "mount", .thread_description = NETDATA_EBPF_MOUNT_MODULE_DESC},
     .functions = {.start_routine = ebpf_mount_thread, .apps_routine = NULL, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_NOT_SET,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &mount_config,
     .config_file = NETDATA_MOUNT_CONFIG_FILE,
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
     .load = EBPF_LOAD_LEGACY,
     .targets = mount_targets,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info = {.thread_name = "fd", .config_name = "fd", .thread_description = NETDATA_EBPF_FD_MODULE_DESC},
     .functions = {.start_routine = ebpf_fd_thread, .apps_routine = ebpf_fd_create_apps_charts, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &fd_config,
     .config_file = NETDATA_FD_CONFIG_FILE,
     .kernels =
         NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_11 | NETDATA_V5_14,
     .load = EBPF_LOAD_LEGACY,
     .targets = fd_targets,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info =
         {.thread_name = "hardirq", .config_name = "hardirq", .thread_description = NETDATA_EBPF_HARDIRQ_MODULE_DESC},
     .functions = {.start_routine = ebpf_hardirq_thread, .apps_routine = NULL, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_NOT_SET,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &hardirq_config,
     .config_file = NETDATA_HARDIRQ_CONFIG_FILE,
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
     .load = EBPF_LOAD_LEGACY,
     .targets = NULL,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info =
         {.thread_name = "softirq", .config_name = "softirq", .thread_description = NETDATA_EBPF_SOFTIRQ_MODULE_DESC},
     .functions = {.start_routine = ebpf_softirq_thread, .apps_routine = NULL, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_NOT_SET,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &softirq_config,
     .config_file = NETDATA_SOFTIRQ_CONFIG_FILE,
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
     .load = EBPF_LOAD_LEGACY,
     .targets = NULL,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info =
         {.thread_name = "oomkill", .config_name = "oomkill", .thread_description = NETDATA_EBPF_OOMKILL_MODULE_DESC},
     .functions =
         {.start_routine = ebpf_oomkill_thread, .apps_routine = ebpf_oomkill_create_apps_charts, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &oomkill_config,
     .config_file = NETDATA_OOMKILL_CONFIG_FILE,
     .kernels = NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
     .load = EBPF_LOAD_LEGACY,
     .targets = NULL,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info = {.thread_name = "shm", .config_name = "shm", .thread_description = NETDATA_EBPF_SHM_MODULE_DESC},
     .functions = {.start_routine = ebpf_shm_thread, .apps_routine = ebpf_shm_create_apps_charts, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_LEVEL_REAL_PARENT,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &shm_config,
     .config_file = NETDATA_DIRECTORY_SHM_CONFIG_FILE,
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
     .load = EBPF_LOAD_LEGACY,
     .targets = shm_targets,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info = {.thread_name = "mdflush", .config_name = "mdflush", .thread_description = NETDATA_EBPF_MD_MODULE_DESC},
     .functions = {.start_routine = ebpf_mdflush_thread, .apps_routine = NULL, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_NOT_SET,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = &mdflush_config,
     .config_file = NETDATA_DIRECTORY_MDFLUSH_CONFIG_FILE,
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
     .load = EBPF_LOAD_LEGACY,
     .targets = mdflush_targets,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info =
         {.thread_name = "functions",
          .config_name = "functions",
          .thread_description = NETDATA_EBPF_FUNCTIONS_MODULE_DESC},
     .functions = {.start_routine = ebpf_function_thread, .apps_routine = NULL, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 1,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_NOT_SET,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE,
     .names = NULL,
     .cfg = NULL,
     .config_file = NETDATA_DIRECTORY_FUNCTIONS_CONFIG_FILE,
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_14,
     .load = EBPF_LOAD_LEGACY,
     .targets = NULL,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES,
     .lifetime = EBPF_DEFAULT_LIFETIME,
     .running_time = 0},
    {.info = {.thread_name = NULL, .config_name = NULL},
     .functions = {.start_routine = NULL, .apps_routine = NULL, .fnct_routine = NULL},
     .enabled = NETDATA_THREAD_EBPF_NOT_RUNNING,
     .update_every = EBPF_DEFAULT_UPDATE_EVERY,
     .global_charts = 0,
     .apps_charts = NETDATA_EBPF_APPS_FLAG_NO,
     .apps_level = NETDATA_APPS_NOT_SET,
     .cgroup_charts = CONFIG_BOOLEAN_NO,
     .mode = MODE_ENTRY,
     .optional = 0,
     .maps = NULL,
     .pid_map_size = 0,
     .names = NULL,
     .cfg = NULL,
     .kernels = 0,
     .load = EBPF_LOAD_LEGACY,
     .targets = NULL,
     .probe_links = NULL,
     .objects = NULL,
     .thread = NULL,
     .maps_per_core = CONFIG_BOOLEAN_YES},
};

struct netdata_static_thread ebpf_threads[] = {
    {.name = "EBPF PROCESS",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF SOCKET",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF CACHESTAT",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF SYNC",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF DCSTAT",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF SWAP",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF VFS",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF FILESYSTEM",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF DISK",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF MOUNT",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF FD",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF HARDIRQ",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF SOFTIRQ",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF OOMKILL",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF SHM",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF MDFLUSH",
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 1,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
    {.name = "EBPF FUNCTIONS",
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
     .start_routine = NULL},
    {.name = NULL,
     .config_section = NULL,
     .config_name = NULL,
     .env_name = NULL,
     .enabled = 0,
     .thread = NULL,
     .init_routine = NULL,
     .start_routine = NULL},
};

ebpf_filesystem_partitions_t localfs[] = {
    {.filesystem = "ext4",
     .optional_filesystem = NULL,
     .family = "ext4",
     .objects = NULL,
     .probe_links = NULL,
     .flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION,
     .enabled = CONFIG_BOOLEAN_YES,
     .addresses = {.function = NULL, .addr = 0},
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
     .fs_maps = NULL,
     .fs_obj = NULL,
     .functions = {"ext4_file_read_iter", "ext4_file_write_iter", "ext4_file_open", "ext4_sync_file", NULL}},
    {.filesystem = "xfs",
     .optional_filesystem = NULL,
     .family = "xfs",
     .objects = NULL,
     .probe_links = NULL,
     .flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION,
     .enabled = CONFIG_BOOLEAN_YES,
     .addresses = {.function = NULL, .addr = 0},
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
     .fs_maps = NULL,
     .fs_obj = NULL,
     .functions = {"xfs_file_read_iter", "xfs_file_write_iter", "xfs_file_open", "xfs_file_fsync", NULL}},
    {.filesystem = "nfs",
     .optional_filesystem = "nfs4",
     .family = "nfs",
     .objects = NULL,
     .probe_links = NULL,
     .flags = NETDATA_FILESYSTEM_ATTR_CHARTS,
     .enabled = CONFIG_BOOLEAN_YES,
     .addresses = {.function = NULL, .addr = 0},
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
     .fs_maps = NULL,
     .fs_obj = NULL,
     .functions =
         {"nfs_file_read",
          "nfs_file_write",
          "nfs_open",
          "nfs_getattr",
          NULL}}, // "nfs4_file_open" - not present on all kernels
    {.filesystem = "zfs",
     .optional_filesystem = NULL,
     .family = "zfs",
     .objects = NULL,
     .probe_links = NULL,
     .flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION,
     .enabled = CONFIG_BOOLEAN_YES,
     .addresses = {.function = NULL, .addr = 0},
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
     .fs_maps = NULL,
     .fs_obj = NULL,
     .functions = {"zpl_iter_read", "zpl_iter_write", "zpl_open", "zpl_fsync", NULL}},
    {.filesystem = "btrfs",
     .optional_filesystem = NULL,
     .family = "btrfs",
     .objects = NULL,
     .probe_links = NULL,
     .flags = NETDATA_FILESYSTEM_FILL_ADDRESS_TABLE,
     .enabled = CONFIG_BOOLEAN_YES,
     .addresses = {.function = "btrfs_file_operations", .addr = 0},
     .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_10,
     .fs_maps = NULL,
     .fs_obj = NULL,
     .functions = {"btrfs_file_read_iter", "btrfs_file_write_iter", "btrfs_file_open", "btrfs_sync_file", NULL}},
    {.filesystem = NULL,
     .optional_filesystem = NULL,
     .family = NULL,
     .objects = NULL,
     .probe_links = NULL,
     .flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION,
     .enabled = CONFIG_BOOLEAN_YES,
     .addresses = {.function = NULL, .addr = 0},
     .kernels = 0,
     .fs_maps = NULL,
     .fs_obj = NULL}};

ebpf_sync_syscalls_t local_syscalls[] = {
    {.syscall = NETDATA_SYSCALLS_SYNC,
     .enabled = CONFIG_BOOLEAN_YES,
     .objects = NULL,
     .probe_links = NULL,
#ifdef LIBBPF_MAJOR_VERSION
     .sync_obj = NULL,
#endif
     .sync_maps = NULL},
    {.syscall = NETDATA_SYSCALLS_SYNCFS,
     .enabled = CONFIG_BOOLEAN_YES,
     .objects = NULL,
     .probe_links = NULL,
#ifdef LIBBPF_MAJOR_VERSION
     .sync_obj = NULL,
#endif
     .sync_maps = NULL},
    {.syscall = NETDATA_SYSCALLS_MSYNC,
     .enabled = CONFIG_BOOLEAN_YES,
     .objects = NULL,
     .probe_links = NULL,
#ifdef LIBBPF_MAJOR_VERSION
     .sync_obj = NULL,
#endif
     .sync_maps = NULL},
    {.syscall = NETDATA_SYSCALLS_FSYNC,
     .enabled = CONFIG_BOOLEAN_YES,
     .objects = NULL,
     .probe_links = NULL,
#ifdef LIBBPF_MAJOR_VERSION
     .sync_obj = NULL,
#endif
     .sync_maps = NULL},
    {.syscall = NETDATA_SYSCALLS_FDATASYNC,
     .enabled = CONFIG_BOOLEAN_YES,
     .objects = NULL,
     .probe_links = NULL,
#ifdef LIBBPF_MAJOR_VERSION
     .sync_obj = NULL,
#endif
     .sync_maps = NULL},
    {.syscall = NETDATA_SYSCALLS_SYNC_FILE_RANGE,
     .enabled = CONFIG_BOOLEAN_YES,
     .objects = NULL,
     .probe_links = NULL,
#ifdef LIBBPF_MAJOR_VERSION
     .sync_obj = NULL,
#endif
     .sync_maps = NULL},
    {.syscall = NULL,
     .enabled = CONFIG_BOOLEAN_NO,
     .objects = NULL,
     .probe_links = NULL,
#ifdef LIBBPF_MAJOR_VERSION
     .sync_obj = NULL,
#endif
     .sync_maps = NULL}};

// Link with cgroup.plugin
netdata_ebpf_cgroup_shm_t shm_ebpf_cgroup = {NULL, NULL};
int shm_fd_ebpf_cgroup = -1;
sem_t *shm_sem_ebpf_cgroup = SEM_FAILED;
netdata_mutex_t mutex_cgroup_shm;

//Network viewer
ebpf_network_viewer_options_t network_viewer_opt;

// Statistic
ebpf_plugin_stats_t plugin_statistics = {
    .core = 0,
    .legacy = 0,
    .running = 0,
    .threads = 0,
    .tracepoints = 0,
    .probes = 0,
    .retprobes = 0,
    .trampolines = 0,
    .memlock_kern = 0,
    .hash_tables = 0};
netdata_ebpf_judy_pid_t ebpf_judy_pid = {.pid_table = NULL, .index = {.JudyLArray = NULL}};
bool ebpf_plugin_exit = false;

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
struct swap_bpf *swap_bpf_obj = NULL;
struct vfs_bpf *vfs_bpf_obj = NULL;
struct process_bpf *process_bpf_obj = NULL;
#else
void *default_btf = NULL;
#endif
const char *btf_path = NULL;

/*****************************************************************
 *
 *  FUNCTIONS USED TO MANIPULATE JUDY ARRAY
 *
 *****************************************************************/

/**
 * Hashtable insert unsafe
 *
 * Find or create a value associated to the index
 *
 * @return The lsocket = 0 when new item added to the array otherwise the existing item value is returned in *lsocket
 * we return a pointer to a pointer, so that the caller can put anything needed at the value of the index.
 * The pointer to pointer we return has to be used before any other operation that may change the index (insert/delete).
 *
 */
void **ebpf_judy_insert_unsafe(PPvoid_t arr, Word_t key)
{
    JError_t J_Error;
    Pvoid_t *idx = JudyLIns(arr, key, &J_Error);
    if (unlikely(idx == PJERR)) {
        netdata_log_error(
            "Cannot add PID to JudyL, JU_ERRNO_* == %u, ID == %d", JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }

    return idx;
}

/**
 * Get PID from judy
 *
 * Get a pointer for the `pid` from judy_array;
 *
 * @param judy_array a judy array where PID is the primary key
 * @param pid        pid stored.
 */
netdata_ebpf_judy_pid_stats_t *ebpf_get_pid_from_judy_unsafe(PPvoid_t judy_array, uint32_t pid)
{
    netdata_ebpf_judy_pid_stats_t **pid_pptr =
        (netdata_ebpf_judy_pid_stats_t **)ebpf_judy_insert_unsafe(judy_array, pid);
    netdata_ebpf_judy_pid_stats_t *pid_ptr = *pid_pptr;
    if (likely(*pid_pptr == NULL)) {
        // a new PID added to the index
        *pid_pptr = aral_mallocz(ebpf_judy_pid.pid_table);

        pid_ptr = *pid_pptr;

        pid_ptr->cmdline = NULL;
        pid_ptr->socket_stats.JudyLArray = NULL;
        rw_spinlock_init(&pid_ptr->socket_stats.rw_spinlock);
    }

    return pid_ptr;
}

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
        netdata_log_error(
            "Number of elements given is too small, adjusting it for %d", NETDATA_EBPF_ALLOC_MIN_ELEMENTS);
        max_elements = NETDATA_EBPF_ALLOC_MIN_ELEMENTS;
    }

    return aral_create(name, size, 0, 0, NULL, NULL, NULL, false, false, false);
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
    int i;
    usec_t max = USEC_PER_SEC, step = 200000;
    int j;
    while (max) {
        max -= step;
        sleep_usec(step);
        i = 0;
        netdata_mutex_lock(&ebpf_exit_cleanup);
        for (j = 0; ebpf_modules[j].info.thread_name != NULL; j++) {
            if (ebpf_modules[j].enabled < NETDATA_THREAD_EBPF_STOPPING)
                i++;
        }
        netdata_mutex_unlock(&ebpf_exit_cleanup);
        if (!i)
            break;
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
    netdata_mutex_lock(&ebpf_exit_cleanup);
    if (default_btf) {
        btf__free(default_btf);
        default_btf = NULL;
    }
    netdata_mutex_unlock(&ebpf_exit_cleanup);
#endif

    char filename[FILENAME_MAX + 1];
    ebpf_pid_file(filename, FILENAME_MAX);
    if (unlink(filename) == -1 && errno != ENOENT)
        netdata_log_error("Cannot remove PID file %s: %s", filename, strerror(errno));

#ifdef NETDATA_INTERNAL_CHECKS
    netdata_log_error("Good bye world! I was PID %d", main_thread_id);
#endif
    fprintf(stdout, "EXIT\n");
    fflush(stdout);

    ebpf_check_before2go();
    netdata_mutex_lock(&mutex_cgroup_shm);
    if (shm_ebpf_cgroup.header) {
        ebpf_unmap_cgroup_shared_memory();
        shm_unlink(NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME);
    }
    netdata_mutex_unlock(&mutex_cgroup_shm);
    netdata_integration_cleanup_shm();

    exit(0);
}

/**
 * Unload legacy code
 *
 * @param objects       objects loaded from eBPF programs
 * @param probe_links   links from loader
 */
void ebpf_unload_legacy_code(struct bpf_object *objects, struct bpf_link **probe_links)
{
    if (!probe_links || !objects)
        return;

    struct bpf_program *prog;
    size_t j = 0;
    bpf_object__for_each_program(prog, objects)
    {
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
    ebpf_module_t *em = &ebpf_modules[EBPF_MODULE_SOCKET_IDX];
    if (em->enabled != NETDATA_THREAD_EBPF_STOPPED)
        return;

    if (em->load == EBPF_LOAD_LEGACY)
        ebpf_unload_legacy_code(em->objects, em->probe_links);

#ifdef LIBBPF_MAJOR_VERSION
    if (socket_bpf_obj)
        socket_bpf__destroy(socket_bpf_obj);
#endif
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

/**
 *  Read Local Ports
 *
 *  Parse /proc/net/{tcp,udp} and get the ports Linux is listening.
 *
 *  @param filename the proc file to parse.
 *  @param proto is the magic number associated to the protocol file we are reading.
 */
void ebpf_stop_threads(int sig)
{
    UNUSED(sig);
    static int only_one = 0;

    // Child thread should be closed by itself.
    netdata_mutex_lock(&ebpf_exit_cleanup);
    if (main_thread_id != gettid_cached() || only_one) {
        netdata_mutex_unlock(&ebpf_exit_cleanup);
        return;
    }
    only_one = 1;
    int i;
    for (i = 0; ebpf_modules[i].info.thread_name != NULL; i++) {
        if (ebpf_modules[i].enabled < NETDATA_THREAD_EBPF_STOPPING && ebpf_modules[i].thread &&
            ebpf_modules[i].thread->thread) {
            nd_thread_signal_cancel(ebpf_modules[i].thread->thread);
#ifdef NETDATA_DEV_MODE
            netdata_log_info("Sending cancel for thread %s", ebpf_modules[i].info.thread_name);
#endif
        }
    }
    netdata_mutex_unlock(&ebpf_exit_cleanup);

    for (i = 0; ebpf_modules[i].info.thread_name != NULL; i++) {
        if (ebpf_threads[i].thread)
            nd_thread_join(ebpf_threads[i].thread);
    }

    __atomic_store_n(&ebpf_plugin_exit, true, __ATOMIC_RELEASE);

    netdata_mutex_lock(&mutex_cgroup_shm);
    nd_thread_signal_cancel(cgroup_integration_thread.thread);
#ifdef NETDATA_DEV_MODE
    netdata_log_info("Sending cancel for thread %s", cgroup_integration_thread.name);
#endif
    netdata_mutex_unlock(&mutex_cgroup_shm);

    ebpf_check_before2go();

    netdata_mutex_lock(&ebpf_exit_cleanup);
    ebpf_unload_unique_maps();
    ebpf_unload_filesystems();
    ebpf_unload_sync();
    netdata_mutex_unlock(&ebpf_exit_cleanup);

    ebpf_exit();
}

/**
 * Start Pthread Variable
 *
 * This function starts all
 */
static void ebpf_mutex_initialize()
{
    netdata_mutex_init(&lock);
    netdata_mutex_init(&ebpf_exit_cleanup);
    netdata_mutex_init(&collect_data_mutex);
    netdata_mutex_init(&mutex_cgroup_shm);
    rw_spinlock_init(&ebpf_judy_pid.index.rw_spinlock);
}

/**
 * Allocate the vectors used for all threads.
 */
static void ebpf_allocate_common_vectors()
{
    ebpf_judy_pid.pid_table =
        ebpf_allocate_pid_aral(NETDATA_EBPF_PID_SOCKET_ARAL_TABLE_NAME, sizeof(netdata_ebpf_judy_pid_stats_t));
    ebpf_aral_init();
}

/**
 * Set global variables reading environment variables
 */
static void ebpf_set_global_variables()
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
    pid_max = os_get_system_pid_max();
    running_on_kernel = ebpf_get_kernel_version();
    memset(pids_fd, -1, sizeof(pids_fd));
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
        {"process", no_argument, 0, 0},
        {"net", no_argument, 0, 0},
        {"cachestat", no_argument, 0, 0},
        {"sync", no_argument, 0, 0},
        {"dcstat", no_argument, 0, 0},
        {"swap", no_argument, 0, 0},
        {"vfs", no_argument, 0, 0},
        {"filesystem", no_argument, 0, 0},
        {"disk", no_argument, 0, 0},
        {"mount", no_argument, 0, 0},
        {"filedescriptor", no_argument, 0, 0},
        {"hardirq", no_argument, 0, 0},
        {"softirq", no_argument, 0, 0},
        {"oomkill", no_argument, 0, 0},
        {"shm", no_argument, 0, 0},
        {"mdflush", no_argument, 0, 0},
        /* INSERT NEW THREADS BEFORE THIS COMMENT TO KEEP COMPATIBILITY WITH enum ebpf_module_indexes */
        {"all", no_argument, 0, 0},
        {"version", no_argument, 0, 0},
        {"help", no_argument, 0, 0},
        {"global", no_argument, 0, 0},
        {"return", no_argument, 0, 0},
        {"legacy", no_argument, 0, 0},
        {"core", no_argument, 0, 0},
        {"unittest", no_argument, 0, 0},
        {0, 0, 0, 0}};

    memset(&network_viewer_opt, 0, sizeof(network_viewer_opt));
    rw_spinlock_init(&network_viewer_opt.rw_spinlock);

    if (argc > 1) {
        int n = (int)str2l(argv[1]);
        if (n > 0) {
            freq = n;
        }
    }

    if (!freq)
        freq = EBPF_DEFAULT_UPDATE_EVERY;

    //rw_spinlock_write_lock(&network_viewer_opt.rw_spinlock);
    if (ebpf_load_collector_config(ebpf_user_config_dir, &disable_cgroups, freq)) {
        netdata_log_info(
            "Does not have a configuration file inside `%s/ebpf.d.conf. It will try to load stock file.",
            ebpf_user_config_dir);
        if (ebpf_load_collector_config(ebpf_stock_config_dir, &disable_cgroups, freq)) {
            netdata_log_info("Does not have a stock file. It is starting with default options.");
        }
    }

    ebpf_load_thread_config();
    //rw_spinlock_write_unlock(&network_viewer_opt.rw_spinlock);

    while (1) {
        int c = getopt_long_only(argc, argv, "", long_options, &option_index);
        if (c == -1)
            break;

        switch (option_index) {
            case EBPF_MODULE_PROCESS_IDX: {
                select_threads |= 1 << EBPF_MODULE_PROCESS_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF enabling \"PROCESS\" charts, because it was started with the option \"[-]-process\".");
#endif
                break;
            }
            case EBPF_MODULE_SOCKET_IDX: {
                select_threads |= 1 << EBPF_MODULE_SOCKET_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"NET\" charts, because it was started with the option \"[-]-net\".");
#endif
                break;
            }
            case EBPF_MODULE_CACHESTAT_IDX: {
                select_threads |= 1 << EBPF_MODULE_CACHESTAT_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF enabling \"CACHESTAT\" charts, because it was started with the option \"[-]-cachestat\".");
#endif
                break;
            }
            case EBPF_MODULE_SYNC_IDX: {
                select_threads |= 1 << EBPF_MODULE_SYNC_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"SYNC\" chart, because it was started with the option \"[-]-sync\".");
#endif
                break;
            }
            case EBPF_MODULE_DCSTAT_IDX: {
                select_threads |= 1 << EBPF_MODULE_DCSTAT_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF enabling \"DCSTAT\" charts, because it was started with the option \"[-]-dcstat\".");
#endif
                break;
            }
            case EBPF_MODULE_SWAP_IDX: {
                select_threads |= 1 << EBPF_MODULE_SWAP_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"SWAP\" chart, because it was started with the option \"[-]-swap\".");
#endif
                break;
            }
            case EBPF_MODULE_VFS_IDX: {
                select_threads |= 1 << EBPF_MODULE_VFS_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"VFS\" chart, because it was started with the option \"[-]-vfs\".");
#endif
                break;
            }
            case EBPF_MODULE_FILESYSTEM_IDX: {
                select_threads |= 1 << EBPF_MODULE_FILESYSTEM_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF enabling \"FILESYSTEM\" chart, because it was started with the option \"[-]-filesystem\".");
#endif
                break;
            }
            case EBPF_MODULE_DISK_IDX: {
                select_threads |= 1 << EBPF_MODULE_DISK_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"DISK\" chart, because it was started with the option \"[-]-disk\".");
#endif
                break;
            }
            case EBPF_MODULE_MOUNT_IDX: {
                select_threads |= 1 << EBPF_MODULE_MOUNT_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF enabling \"MOUNT\" chart, because it was started with the option \"[-]-mount\".");
#endif
                break;
            }
            case EBPF_MODULE_FD_IDX: {
                select_threads |= 1 << EBPF_MODULE_FD_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF enabling \"FILEDESCRIPTOR\" chart, because it was started with the option \"[-]-filedescriptor\".");
#endif
                break;
            }
            case EBPF_MODULE_HARDIRQ_IDX: {
                select_threads |= 1 << EBPF_MODULE_HARDIRQ_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF enabling \"HARDIRQ\" chart, because it was started with the option \"[-]-hardirq\".");
#endif
                break;
            }
            case EBPF_MODULE_SOFTIRQ_IDX: {
                select_threads |= 1 << EBPF_MODULE_SOFTIRQ_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF enabling \"SOFTIRQ\" chart, because it was started with the option \"[-]-softirq\".");
#endif
                break;
            }
            case EBPF_MODULE_OOMKILL_IDX: {
                select_threads |= 1 << EBPF_MODULE_OOMKILL_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF enabling \"OOMKILL\" chart, because it was started with the option \"[-]-oomkill\".");
#endif
                break;
            }
            case EBPF_MODULE_SHM_IDX: {
                select_threads |= 1 << EBPF_MODULE_SHM_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("EBPF enabling \"SHM\" chart, because it was started with the option \"[-]-shm\".");
#endif
                break;
            }
            case EBPF_MODULE_MDFLUSH_IDX: {
                select_threads |= 1 << EBPF_MODULE_MDFLUSH_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF enabling \"MDFLUSH\" chart, because it was started with the option \"[-]-mdflush\".");
#endif
                break;
            }
            case EBPF_OPTION_ALL_CHARTS: {
                ebpf_set_apps_mode(NETDATA_EBPF_APPS_FLAG_YES);
                disable_cgroups = 0;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF running with all chart groups, because it was started with the option \"[-]-all\".");
#endif
                break;
            }
            case EBPF_OPTION_VERSION: {
                printf("ebpf.plugin %s\n", NETDATA_VERSION);
                exit(0);
            }
            case EBPF_OPTION_HELP: {
                ebpf_print_help();
                exit(0);
            }
            case EBPF_OPTION_GLOBAL_CHART: {
                disable_cgroups = 1;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF running with global chart group, because it was started with the option  \"[-]-global\".");
#endif
                break;
            }
            case EBPF_OPTION_RETURN_MODE: {
                ebpf_set_thread_mode(MODE_RETURN);
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF running in \"RETURN\" mode, because it was started with the option \"[-]-return\".");
#endif
                break;
            }
            case EBPF_OPTION_LEGACY: {
                ebpf_set_load_mode(EBPF_LOAD_LEGACY, EBPF_LOADED_FROM_USER);
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF running with \"LEGACY\" code, because it was started with the option \"[-]-legacy\".");
#endif
                break;
            }
            case EBPF_OPTION_CORE: {
                ebpf_set_load_mode(EBPF_LOAD_CORE, EBPF_LOADED_FROM_USER);
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info(
                    "EBPF running with \"CO-RE\" code, because it was started with the option \"[-]-core\".");
#endif
                break;
            }
            case EBPF_OPTION_UNITTEST: {
                // if we cannot run until the end, we will cancel the unittest
                int exit_code = ECANCELED;
                if (ebpf_can_plugin_load_code(running_on_kernel, NETDATA_EBPF_PLUGIN_NAME))
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
        ebpf_disable_cgroups();
    }

    if (select_threads) {
        disable_all_global_charts();
        uint64_t idx;
        for (idx = 0; idx < EBPF_OPTION_ALL_CHARTS; idx++) {
            if (select_threads & 1 << idx)
                ebpf_enable_specific_chart(&ebpf_modules[idx], disable_cgroups);
        }
    }

    // Load apps_groups.conf
    if (ebpf_read_apps_groups_conf(
            &apps_groups_default_target, &apps_groups_root_target, ebpf_user_config_dir, "groups")) {
        netdata_log_info(
            "Cannot read process groups configuration file '%s/apps_groups.conf'. Will try '%s/apps_groups.conf'",
            ebpf_user_config_dir,
            ebpf_stock_config_dir);
        if (ebpf_read_apps_groups_conf(
                &apps_groups_default_target, &apps_groups_root_target, ebpf_stock_config_dir, "groups")) {
            netdata_log_error(
                "Cannot read process groups '%s/apps_groups.conf'. There are no internal defaults. Failing.",
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
static char *memlock_stat = "memory_locked";
static char *hash_table_stat = "hash_table";
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
    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, chart, "");
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_module_t *wem = &ebpf_modules[i];
        if (wem->functions.apps_routine)
            write_chart_dimension(
                (char *)wem->info.thread_name,
                (wem->enabled < NETDATA_THREAD_EBPF_STOPPING) ? wem->hash_table_stats[idx] : 0);
    }
    ebpf_write_end_chart();
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
    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_HASH_TABLES_GLOBAL_ELEMENTS, "");
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_module_t *wem = &ebpf_modules[i];
        write_chart_dimension(
            (char *)wem->info.thread_name, (wem->enabled < NETDATA_THREAD_EBPF_STOPPING) ? NETDATA_CONTROLLER_END : 0);
    }
    ebpf_write_end_chart();
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

    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_THREADS, "");
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_module_t *wem = &ebpf_modules[i];
        if (wem->functions.fnct_routine)
            continue;

        write_chart_dimension((char *)wem->info.thread_name, (wem->enabled < NETDATA_THREAD_EBPF_STOPPING) ? 1 : 0);
    }
    ebpf_write_end_chart();

    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, "monitoring_pid", "");
    write_chart_dimension("user", ebpf_all_pids_count);
    write_chart_dimension("kernel", ebpf_hash_table_pids_count);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_LIFE_TIME, "");
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_module_t *wem = &ebpf_modules[i];
        // Threads like VFS is slow to load and this can create an invalid number, this is the motive
        // we are also testing wem->lifetime value.
        if (wem->functions.fnct_routine)
            continue;

        write_chart_dimension(
            (char *)wem->info.thread_name,
            (wem->lifetime && wem->enabled < NETDATA_THREAD_EBPF_STOPPING) ?
                (long long)(wem->lifetime - wem->running_time) :
                0);
    }
    ebpf_write_end_chart();

    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_LOAD_METHOD, "");
    write_chart_dimension(load_event_stat[NETDATA_EBPF_LOAD_STAT_LEGACY], (long long)plugin_statistics.legacy);
    write_chart_dimension(load_event_stat[NETDATA_EBPF_LOAD_STAT_CORE], (long long)plugin_statistics.core);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_KERNEL_MEMORY, "");
    write_chart_dimension(memlock_stat, (long long)plugin_statistics.memlock_kern);
    ebpf_write_end_chart();

    ebpf_user_mem_stat_t ipc_data;
    netdata_integration_current_ipc_data(&ipc_data);
    NETDATA_DOUBLE ipc_value = 0.0;
    if (ipc_data.total > 0)
        ipc_value = ((NETDATA_DOUBLE)ipc_data.current / (NETDATA_DOUBLE)ipc_data.total) * 100.0;
    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_IPC_USAGE, "");
    write_chart_dimension("positions", (long long)ipc_value);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_HASH_TABLES_LOADED, "");
    write_chart_dimension(hash_table_stat, (long long)plugin_statistics.hash_tables);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, NETDATA_EBPF_HASH_TABLES_PER_CORE, "");
    write_chart_dimension(hash_table_core[NETDATA_EBPF_THREAD_PER_CORE], (long long)plugin_statistics.hash_percpu);
    write_chart_dimension(hash_table_core[NETDATA_EBPF_THREAD_UNIQUE], (long long)plugin_statistics.hash_unique);
    ebpf_write_end_chart();

    ebpf_send_global_hash_table_data();

    ebpf_send_hash_table_pid_data(
        NETDATA_EBPF_HASH_TABLES_INSERT_PID_ELEMENTS, NETDATA_EBPF_GLOBAL_TABLE_PID_TABLE_ADD);
    ebpf_send_hash_table_pid_data(
        NETDATA_EBPF_HASH_TABLES_REMOVE_PID_ELEMENTS, NETDATA_EBPF_GLOBAL_TABLE_PID_TABLE_DEL);
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
 * Create PIDS Chart
 *
 * Write to standard output current values for PIDSs charts.
 *
 * @param order        order to display chart
 * @param update_every time used to update charts
 */
static void ebpf_create_pids_chart(int order, int update_every)
{
    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        "monitoring_pid",
        "",
        "Total number of monitored PIDs",
        "pids",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_LINE,
        "netdata.ebpf_pids",
        order,
        update_every,
        "main");

    ebpf_write_global_dimension("user", "user", ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    ebpf_write_global_dimension("kernel", "kernel", ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
}

/**
 * Create Thread Chart
 *
 * Write to standard output current values for threads charts.
 *
 * @param name         is the chart name
 * @param title        chart title.
 * @param units        chart units
 * @param order        is the chart order
 * @param update_every time used to update charts
 * @param module       a module to create a specific chart.
 */
static void
ebpf_create_thread_chart(char *name, char *title, char *units, int order, int update_every, ebpf_module_t *module)
{
    // common call for specific and all charts.
    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        name,
        "",
        title,
        units,
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NULL,
        order,
        update_every,
        "main");

    if (module) {
        ebpf_write_global_dimension(
            (char *)module->info.thread_name,
            (char *)module->info.thread_name,
            ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
        return;
    }

    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_module_t *em = &ebpf_modules[i];
        if (em->functions.fnct_routine)
            continue;

        ebpf_write_global_dimension(
            (char *)em->info.thread_name, (char *)em->info.thread_name, ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
    }
}

/**
 * Create chart for Load Thread
 *
 * Write to standard output current values for load mode.
 *
 * @param update_every time used to update charts
 */
static inline void ebpf_create_statistic_ipc_usage(int update_every)
{
    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        NETDATA_EBPF_IPC_USAGE,
        "",
        "IPC used array positions.",
        "%",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NULL,
        NETDATA_EBPF_ORDER_PIDS_IPC,
        update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension("positions", "positions", ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
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
    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        NETDATA_EBPF_LOAD_METHOD,
        "",
        "Load info.",
        "methods",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NULL,
        NETDATA_EBPF_ORDER_STAT_LOAD_METHOD,
        update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(
        load_event_stat[NETDATA_EBPF_LOAD_STAT_LEGACY],
        load_event_stat[NETDATA_EBPF_LOAD_STAT_LEGACY],
        ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    ebpf_write_global_dimension(
        load_event_stat[NETDATA_EBPF_LOAD_STAT_CORE],
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
    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        NETDATA_EBPF_KERNEL_MEMORY,
        "",
        "Memory allocated for hash tables.",
        "bytes",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NULL,
        NETDATA_EBPF_ORDER_STAT_KERNEL_MEMORY,
        update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(memlock_stat, memlock_stat, ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
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
    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        NETDATA_EBPF_HASH_TABLES_LOADED,
        "",
        "Number of hash tables loaded.",
        "hash tables",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NULL,
        NETDATA_EBPF_ORDER_STAT_HASH_TABLES,
        update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(hash_table_stat, hash_table_stat, ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
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
    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        NETDATA_EBPF_HASH_TABLES_PER_CORE,
        "",
        "How threads are loading hash/array tables.",
        "threads",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NULL,
        NETDATA_EBPF_ORDER_STAT_HASH_CORE,
        update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(
        hash_table_core[NETDATA_EBPF_THREAD_PER_CORE],
        hash_table_core[NETDATA_EBPF_THREAD_PER_CORE],
        ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    ebpf_write_global_dimension(
        hash_table_core[NETDATA_EBPF_THREAD_UNIQUE],
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
    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        NETDATA_EBPF_HASH_TABLES_GLOBAL_ELEMENTS,
        "",
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
        ebpf_write_global_dimension(
            (char *)ebpf_modules[i].info.thread_name,
            (char *)ebpf_modules[i].info.thread_name,
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
    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        id,
        "",
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
        if (wem->functions.apps_routine)
            ebpf_write_global_dimension(
                (char *)wem->info.thread_name,
                (char *)wem->info.thread_name,
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

    ebpf_create_thread_chart(
        NETDATA_EBPF_THREADS, "Threads running.", "boolean", NETDATA_EBPF_ORDER_STAT_THREADS, update_every, NULL);

    ebpf_create_pids_chart(NETDATA_EBPF_ORDER_PIDS, update_every);

    ebpf_create_thread_chart(
        NETDATA_EBPF_LIFE_TIME,
        "Time remaining for thread.",
        "seconds",
        NETDATA_EBPF_ORDER_STAT_LIFE_TIME,
        update_every,
        NULL);

    int i, j;
    char name[256];
    for (i = 0, j = NETDATA_EBPF_ORDER_FUNCTION_PER_THREAD; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_module_t *em = &ebpf_modules[i];
        if (!em->functions.fnct_routine)
            continue;

        em->functions.order_thread_chart = j;
        snprintfz(name, sizeof(name) - 1, "%s_%s", NETDATA_EBPF_THREADS, em->info.thread_name);
        ebpf_create_thread_chart(name, "Threads running.", "boolean", j++, update_every, em);

        em->functions.order_thread_lifetime = j;
        snprintfz(name, sizeof(name) - 1, "%s_%s", NETDATA_EBPF_LIFE_TIME, em->info.thread_name);
        ebpf_create_thread_chart(name, "Time remaining for thread.", "seconds", j++, update_every, em);
    }

    ebpf_create_statistic_ipc_usage(update_every);

    ebpf_create_statistic_load_chart(update_every);

    ebpf_create_statistic_kernel_memory(update_every);

    ebpf_create_statistic_hash_tables(update_every);

    ebpf_create_statistic_hash_per_core(update_every);

    ebpf_create_statistic_hash_global_elements(update_every);

    ebpf_create_statistic_hash_pid_table(
        update_every,
        NETDATA_EBPF_HASH_TABLES_INSERT_PID_ELEMENTS,
        "Elements inserted into PID table",
        NETDATA_EBPF_ORDER_STAT_HASH_PID_TABLE_ADDED);

    ebpf_create_statistic_hash_pid_table(
        update_every,
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
    snprintfz(filename, sizeof(filename) - 1, "/proc/%d/status", pid);

    procfile *ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
    if (unlikely(!ff)) {
        netdata_log_error("Cannot open %s", filename);
        return name;
    }

    ff = procfile_readall(ff);
    if (unlikely(!ff)) {
        procfile_close(ff);
        return name;
    }

    unsigned long i, lines = procfile_lines(ff);
    for (i = 0; i < lines; i++) {
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
        old_pid = (pid_t)str2uint32_t(buffer, NULL);
    }
    fclose(fp);

    return old_pid;
}

/**
 * Validate Data Sharing Selection
 *
 * Validate user input avoid sigsegv
 */
void ebpf_validate_data_sharing_selection()
{
    uint32_t enabled = CONFIG_BOOLEAN_NO;
    for (uint32_t i = 0; ebpf_modules[i].info.thread_name != NULL; i++) {
        if (ebpf_modules[i].apps_charts || ebpf_modules[i].cgroup_charts) {
            enabled = CONFIG_BOOLEAN_YES;
            break;
        }
    }

    // TODO: MODIFY IN NEXT PRs THE OPTION TO ALSO USE SOCKET
    if (enabled && integration_with_collectors != NETDATA_EBPF_INTEGRATION_SHM) {
        //if (enabled && integration_with_collectors == NETDATA_EBPF_INTEGRATION_DISABLED) {
        integration_with_collectors = NETDATA_EBPF_INTEGRATION_SHM;
    }
}

/**
 * Initialize Data Sharing
 *
 * Start sharing according to user configuration.
 */
static void ebpf_initialize_data_sharing()
{
    ebpf_validate_data_sharing_selection();

    // Initialize
    switch (integration_with_collectors) {
        case NETDATA_EBPF_INTEGRATION_SOCKET: {
            socket_ipc =
                nd_thread_create("ebpf_socket_ipc", NETDATA_THREAD_OPTION_DEFAULT, ebpf_socket_thread_ipc, NULL);
            break;
        }
        case NETDATA_EBPF_INTEGRATION_SHM:
            // All pid_map_size have the same value
            if (netdata_integration_initialize_shm(ebpf_modules[EBPF_MODULE_PROCESS_IDX].pid_map_size)) {
                ebpf_set_apps_mode(NETDATA_EBPF_APPS_FLAG_NO);
                ebpf_disable_cgroups();
            }
            break;
        case NETDATA_EBPF_INTEGRATION_DISABLED:
        default:
            break;
    }
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
    pid_t old_pid = ebpf_read_previous_pid(filename);
    if (!old_pid)
        return;

    char *prev_name = ebpf_get_process_name(old_pid);
    if (!prev_name)
        return;

    char *current_name = ebpf_get_process_name(pid);
    if (current_name && !strcmp(prev_name, current_name))
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
    snprintfz(filename, length - 1, "%s/var/run/ebpf.pid", netdata_configured_host_prefix);
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
    for (i = 0; ebpf_modules[i].info.thread_name != NULL; i++) {
        ebpf_threads[i].start_routine = ebpf_modules[i].functions.start_routine;
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
    nd_log_initialize_for_external_plugins(NETDATA_EBPF_PLUGIN_NAME);
    netdata_threads_init_for_external_plugins(0);

    ebpf_set_global_variables();
    if (ebpf_can_plugin_load_code(running_on_kernel, NETDATA_EBPF_PLUGIN_NAME))
        return 2;

    if (ebpf_adjust_memory_limit())
        return 3;

    main_thread_id = gettid_cached();

    ebpf_parse_args(argc, argv);
    ebpf_manage_pid(getpid());

    signal(SIGINT, ebpf_stop_threads);
    signal(SIGQUIT, ebpf_stop_threads);
    signal(SIGTERM, ebpf_stop_threads);
    signal(SIGPIPE, ebpf_stop_threads);

    ebpf_mutex_initialize();

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if (verify_netdata_host_prefix(true) == -1)
        ebpf_exit();

    ebpf_allocate_common_vectors();

#ifdef LIBBPF_MAJOR_VERSION
    libbpf_set_strict_mode(LIBBPF_STRICT_ALL);

#ifndef NETDATA_INTERNAL_CHECKS
    libbpf_set_print(netdata_silent_libbpf_vfprintf);
#endif
#endif

    ebpf_read_local_addresses_unsafe();
    read_local_ports("/proc/net/tcp", IPPROTO_TCP);
    read_local_ports("/proc/net/tcp6", IPPROTO_TCP);
    read_local_ports("/proc/net/udp", IPPROTO_UDP);
    read_local_ports("/proc/net/udp6", IPPROTO_UDP);

    ebpf_set_static_routine();

    cgroup_integration_thread.start_routine = ebpf_cgroup_integration;

    cgroup_integration_thread.thread =
        nd_thread_create(cgroup_integration_thread.name, NETDATA_THREAD_OPTION_DEFAULT, ebpf_cgroup_integration, NULL);

    ebpf_initialize_data_sharing();

    uint32_t i;
    for (i = 0; ebpf_threads[i].name != NULL; i++) {
        struct netdata_static_thread *st = &ebpf_threads[i];

        ebpf_module_t *em = &ebpf_modules[i];
        em->thread = st;
        em->thread_id = i;
        if (em->enabled != NETDATA_THREAD_EBPF_NOT_RUNNING) {
            em->enabled = NETDATA_THREAD_EBPF_RUNNING;
            em->lifetime = EBPF_NON_FUNCTION_LIFE_TIME;

            if (em->functions.apps_routine && (em->apps_charts || em->cgroup_charts)) {
                collect_pids |= 1 << i;
            }
            st->thread = nd_thread_create(st->name, NETDATA_THREAD_OPTION_DEFAULT, st->start_routine, em);
        } else {
            em->lifetime = EBPF_DEFAULT_LIFETIME;
        }
    }

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    int update_apps_every = (int)EBPF_CFG_UPDATE_APPS_EVERY_DEFAULT;
    int update_apps_list = update_apps_every - 1;
    //Plugin will be killed when it receives a signal
    for (; !ebpf_plugin_stop(); global_iterations_counter++) {
        (void)heartbeat_next(&hb);

        if (global_iterations_counter % EBPF_DEFAULT_UPDATE_EVERY == 0) {
            netdata_mutex_lock(&lock);
            ebpf_create_statistic_charts(EBPF_DEFAULT_UPDATE_EVERY);

            ebpf_send_statistic_data();
            fflush(stdout);
            netdata_mutex_unlock(&lock);
        }

        if (++update_apps_list == update_apps_every) {
            update_apps_list = 0;
            netdata_mutex_lock(&lock);
            if (collect_pids) {
                netdata_mutex_lock(&collect_data_mutex);
                ebpf_parse_proc_files();
                ebpf_create_apps_charts(apps_groups_root_target);
                netdata_mutex_unlock(&collect_data_mutex);
            }
            netdata_mutex_unlock(&lock);
        }
    }

    ebpf_stop_threads(0);

    return 0;
}
