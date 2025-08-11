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

char *ebpf_algorithms[] = {EBPF_CHART_ALGORITHM_ABSOLUTE, EBPF_CHART_ALGORITHM_INCREMENTAL};
struct config collector_config = APPCONFIG_INITIALIZER;

int running_on_kernel = 0;
int ebpf_nprocs;
int isrh = 0;
int main_thread_id = 0;
int process_pid_fd = -1;
uint64_t collect_pids = 0;
static uint32_t integration_with_collectors = NETDATA_EBPF_INTEGRATION_DISABLED;
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
     .targets = NULL,
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
          NULL}}, // // "nfs4_file_open" - not present on all kernels
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
struct swap_bpf *bpf_obj = NULL;
struct vfs_bpf *vfs_bpf_obj = NULL;
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
    int i = EBPF_OPTION_ALL_CHARTS;
    usec_t max = USEC_PER_SEC, step = 200000;
    while (i && max) {
        max -= step;
        sleep_usec(step);
        i = 0;
        int j;
        netdata_mutex_lock(&ebpf_exit_cleanup);
        for (j = 0; ebpf_modules[j].info.thread_name != NULL; j++) {
            if (ebpf_modules[j].enabled < NETDATA_THREAD_EBPF_STOPPING)
                i++;
        }
        netdata_mutex_unlock(&ebpf_exit_cleanup);
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
    if (unlink(filename))
        netdata_log_error("Cannot remove PID file %s", filename);

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
    int i;
    for (i = 0; ebpf_modules[i].info.thread_name; i++) {
        // These threads are cleaned with other functions
        if (i != EBPF_MODULE_SOCKET_IDX)
            continue;

        if (ebpf_modules[i].enabled != NETDATA_THREAD_EBPF_STOPPED) {
            if (ebpf_modules[i].enabled != NETDATA_THREAD_EBPF_NOT_RUNNING)
                netdata_log_error(
                    "Cannot unload maps for thread %s, because it is not stopped.", ebpf_modules[i].info.thread_name);

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

/**
 * Close the collector gracefully
 *
 * @param sig is the signal number used to close the collector
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
        if (ebpf_modules[i].enabled < NETDATA_THREAD_EBPF_STOPPING) {
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

    ebpf_plugin_exit = true;

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
static inline void ebpf_create_apps_for_module(ebpf_module_t *em, struct ebpf_target *root)
{
    if (em->enabled < NETDATA_THREAD_EBPF_STOPPING && em->apps_charts && em->functions.apps_routine)
        em->functions.apps_routine(em, root);
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
    //    if (unlikely(!ebpf_pids))
    //        return;

    struct ebpf_target *w;
    int newly_added = 0;

    for (w = root; w; w = w->next) {
        if (w->target)
            continue;

        if (unlikely(w->processes && (debug_enabled || w->debug_enabled))) {
            struct ebpf_pid_on_target *pid_on_target;

            fprintf(
                stderr,
                "ebpf.plugin: target '%s' has aggregated %u process%s:",
                w->name,
                w->processes,
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

    if (newly_added) {
        int i;
        for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
            if (!(collect_pids & (1 << i)))
                continue;

            ebpf_module_t *current = &ebpf_modules[i];
            ebpf_create_apps_for_module(current, root);
        }
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
    ebpf_write_begin_chart(family, name, "");

    uint32_t i = 0;
    while (move && i < end) {
        write_chart_dimension(move->name, move->ncall);

        move = move->next;
        i++;
    }

    ebpf_write_end_chart();
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
    ebpf_write_begin_chart(family, name, "");

    int i = 0;
    while (move && i < end) {
        write_chart_dimension(move->name, move->nerr);

        move = move->next;
        i++;
    }

    ebpf_write_end_chart();
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
    ebpf_write_begin_chart(family, chart, "");

    write_chart_dimension(dim, v1);

    ebpf_write_end_chart();
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
    ebpf_write_begin_chart(family, chart, "");

    write_chart_dimension(dwrite, vwrite);
    write_chart_dimension(dread, vread);

    ebpf_write_end_chart();
}

/**
 * Write chart cmd on standard output
 *
 * @param type          chart type
 * @param id            chart id (the apps group name).
 * @param suffix        suffix to differentiate charts
 * @param title         chart title
 * @param units         units label
 * @param family        group name used to attach the chart on dashboard
 * @param charttype     chart type
 * @param context       chart context
 * @param order         chart order
 * @param update_every  update interval used by plugin
 * @param module        chart module name, this is the eBPF thread.
 */
void ebpf_write_chart_cmd(
    char *type,
    char *id,
    char *suffix,
    char *title,
    char *units,
    char *family,
    char *charttype,
    char *context,
    int order,
    int update_every,
    char *module)
{
    printf(
        "CHART %s.%s%s '' '%s' '%s' '%s' '%s' '%s' %d %d '' 'ebpf.plugin' '%s'\n",
        type,
        id,
        suffix,
        title,
        units,
        (family) ? family : "",
        (context) ? context : "",
        (charttype) ? charttype : "",
        order,
        update_every,
        module);
}

/**
 * Write chart cmd on standard output
 *
 * @param type      chart type
 * @param id        chart id
 * @param suffix    add suffix to obsolete charts.
 * @param title     chart title
 * @param units     units label
 * @param family    group name used to attach the chart on dashboard
 * @param charttype chart type
 * @param context   chart context
 * @param order     chart order
 * @param update_every value to overwrite the update frequency set by the server.
 */
void ebpf_write_chart_obsolete(
    char *type,
    char *id,
    char *suffix,
    char *title,
    char *units,
    char *family,
    char *charttype,
    char *context,
    int order,
    int update_every)
{
    printf(
        "CHART %s.%s%s '' '%s' '%s' '%s' '%s' '%s' %d %d 'obsolete'\n",
        type,
        id,
        suffix,
        title,
        units,
        (family) ? family : "",
        (context) ? context : "",
        (charttype) ? charttype : "",
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
void ebpf_create_chart(
    char *type,
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
    ebpf_write_chart_cmd(type, id, "", title, units, family, charttype, context, order, update_every, module);

    if (ncd) {
        ncd(move, end);
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
    ebpf_write_begin_chart(family, name, "");

    uint32_t i;
    for (i = 0; i < end; i++) {
        write_chart_dimension(dimensions[i], (long long)hist[i]);
    }

    ebpf_write_end_chart();

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
    char *mem = {NETDATA_EBPF_STAT_DIMENSION_MEMORY};
    char *aral = {NETDATA_EBPF_STAT_DIMENSION_ARAL};

    snprintfz(em->memory_usage, NETDATA_EBPF_CHART_MEM_LENGTH - 1, "aral_%s_size", name);
    snprintfz(em->memory_allocations, NETDATA_EBPF_CHART_MEM_LENGTH - 1, "aral_%s_alloc", name);

    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        em->memory_usage,
        "",
        "Bytes allocated for ARAL.",
        "bytes",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        "netdata.ebpf_aral_stat_size",
        priority++,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(mem, mem, ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        em->memory_allocations,
        "",
        "Calls to allocate memory.",
        "calls",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        "netdata.ebpf_aral_stat_alloc",
        priority++,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(aral, aral, ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

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
    ebpf_write_chart_obsolete(
        NETDATA_MONITORING_FAMILY,
        em->memory_allocations,
        "",
        "Calls to allocate memory.",
        "calls",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        "netdata.ebpf_aral_stat_alloc",
        prio++,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_MONITORING_FAMILY,
        em->memory_allocations,
        "",
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
    char *mem = {NETDATA_EBPF_STAT_DIMENSION_MEMORY};
    char *aral = {NETDATA_EBPF_STAT_DIMENSION_ARAL};

    struct aral_statistics *stats = aral_get_statistics(memory);

    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, em->memory_usage, "");
    write_chart_dimension(mem, (long long)stats->structures.allocated_bytes);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, em->memory_allocations, "");
    write_chart_dimension(aral, (long long)stats->structures.allocations);
    ebpf_write_end_chart();
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
void ebpf_read_global_table_stats(
    netdata_idx_t *stats,
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
            int before = (maps_per_core) ? ebpf_nprocs : 1;
            netdata_idx_t total = 0;
            for (i = 0; i < before; i++)
                total += values[i];

            stats[order] = total;
        }
    }
}

/*****************************************************************
 *
 *  FUNCTIONS USED WITH SOCKET
 *
 *****************************************************************/

/**
 * Netmask
 *
 * Copied from iprange (https://github.com/firehol/iprange/blob/master/iprange.h)
 *
 * @param prefix create the netmask based in the CIDR value.
 *
 * @return
 */
static inline in_addr_t ebpf_netmask(int prefix)
{
    if (prefix == 0)
        return (~((in_addr_t)-1));
    else
        return (in_addr_t)(~((1 << (32 - prefix)) - 1));
}

/**
 * Broadcast
 *
 * Copied from iprange (https://github.com/firehol/iprange/blob/master/iprange.h)
 *
 * @param addr is the ip address
 * @param prefix is the CIDR value.
 *
 * @return It returns the last address of the range
 */
static inline in_addr_t ebpf_broadcast(in_addr_t addr, int prefix)
{
    return (addr | ~ebpf_netmask(prefix));
}

/**
 * Network
 *
 * Copied from iprange (https://github.com/firehol/iprange/blob/master/iprange.h)
 *
 * @param addr is the ip address
 * @param prefix is the CIDR value.
 *
 * @return It returns the first address of the range.
 */
static inline in_addr_t ebpf_ipv4_network(in_addr_t addr, int prefix)
{
    return (addr & ebpf_netmask(prefix));
}

/**
 * Calculate ipv6 first address
 *
 * @param out the address to store the first address.
 * @param in the address used to do the math.
 * @param prefix number of bits used to calculate the address
 */
static void get_ipv6_first_addr(union netdata_ip_t *out, union netdata_ip_t *in, uint64_t prefix)
{
    uint64_t mask, tmp;
    uint64_t ret[2];

    memcpy(ret, in->addr32, sizeof(union netdata_ip_t));

    if (prefix == 128) {
        memcpy(out->addr32, in->addr32, sizeof(union netdata_ip_t));
        return;
    } else if (!prefix) {
        ret[0] = ret[1] = 0;
        memcpy(out->addr32, ret, sizeof(union netdata_ip_t));
        return;
    } else if (prefix <= 64) {
        ret[1] = 0ULL;

        tmp = be64toh(ret[0]);
        mask = 0xFFFFFFFFFFFFFFFFULL << (64 - prefix);
        tmp &= mask;
        ret[0] = htobe64(tmp);
    } else {
        mask = 0xFFFFFFFFFFFFFFFFULL << (128 - prefix);
        tmp = be64toh(ret[1]);
        tmp &= mask;
        ret[1] = htobe64(tmp);
    }

    memcpy(out->addr32, ret, sizeof(union netdata_ip_t));
}

/**
 * Get IPV6 Last Address
 *
 * @param out the address to store the last address.
 * @param in the address used to do the math.
 * @param prefix number of bits used to calculate the address
 */
static void get_ipv6_last_addr(union netdata_ip_t *out, union netdata_ip_t *in, uint64_t prefix)
{
    uint64_t mask, tmp;
    uint64_t ret[2];
    memcpy(ret, in->addr32, sizeof(union netdata_ip_t));

    if (prefix == 128) {
        memcpy(out->addr32, in->addr32, sizeof(union netdata_ip_t));
        return;
    } else if (!prefix) {
        ret[0] = ret[1] = 0xFFFFFFFFFFFFFFFF;
        memcpy(out->addr32, ret, sizeof(union netdata_ip_t));
        return;
    } else if (prefix <= 64) {
        ret[1] = 0xFFFFFFFFFFFFFFFFULL;

        tmp = be64toh(ret[0]);
        mask = 0xFFFFFFFFFFFFFFFFULL << (64 - prefix);
        tmp |= ~mask;
        ret[0] = htobe64(tmp);
    } else {
        mask = 0xFFFFFFFFFFFFFFFFULL << (128 - prefix);
        tmp = be64toh(ret[1]);
        tmp |= ~mask;
        ret[1] = htobe64(tmp);
    }

    memcpy(out->addr32, ret, sizeof(union netdata_ip_t));
}

/**
 * IP to network long
 *
 * @param dst the vector to store the result
 * @param ip the source ip given by our users.
 * @param domain the ip domain (IPV4 or IPV6)
 * @param source the original string
 *
 * @return it returns 0 on success and -1 otherwise.
 */
static inline int ebpf_ip2nl(uint8_t *dst, const char *ip, int domain, char *source)
{
    if (inet_pton(domain, ip, dst) <= 0) {
        netdata_log_error("The address specified (%s) is invalid ", source);
        return -1;
    }

    return 0;
}

/**
 * Clean port Structure
 *
 * Clean the allocated list.
 *
 * @param clean the list that will be cleaned
 */
void ebpf_clean_port_structure(ebpf_network_viewer_port_list_t **clean)
{
    ebpf_network_viewer_port_list_t *move = *clean;
    while (move) {
        ebpf_network_viewer_port_list_t *next = move->next;
        freez(move->value);
        freez(move);

        move = next;
    }
    *clean = NULL;
}

/**
 * Clean IP structure
 *
 * Clean the allocated list.
 *
 * @param clean the list that will be cleaned
 */
void ebpf_clean_ip_structure(ebpf_network_viewer_ip_list_t **clean)
{
    ebpf_network_viewer_ip_list_t *move = *clean;
    while (move) {
        ebpf_network_viewer_ip_list_t *next = move->next;
        freez(move->value);
        freez(move);

        move = next;
    }
    *clean = NULL;
}

/**
 * Parse IP List
 *
 * Parse IP list and link it.
 *
 * @param out a pointer to store the link list
 * @param ip the value given as parameter
 */
static void ebpf_parse_ip_list_unsafe(void **out, const char *ip)
{
    ebpf_network_viewer_ip_list_t **list = (ebpf_network_viewer_ip_list_t **)out;

    char *ipdup = strdupz(ip);
    union netdata_ip_t first = {};
    union netdata_ip_t last = {};
    const char *is_ipv6;
    if (*ip == '*' && *(ip + 1) == '\0') {
        memset(first.addr8, 0, sizeof(first.addr8));
        memset(last.addr8, 0xFF, sizeof(last.addr8));

        is_ipv6 = ip;

        ebpf_clean_ip_structure(list);
        goto storethisip;
    }

    char *enddup = strdupz(ip);
    char *end = enddup;
    // Move while I cannot find a separator
    while (*end && *end != '/' && *end != '-')
        end++;

    // We will use only the classic IPV6 for while, but we could consider the base 85 in a near future
    // https://tools.ietf.org/html/rfc1924
    is_ipv6 = strchr(ip, ':');

    int select;
    if (*end && !is_ipv6) { // IPV4 range
        select = (*end == '/') ? 0 : 1;
        *end++ = '\0';
        if (*end == '!') {
            netdata_log_info("The exclusion cannot be in the second part of the range %s, it will be ignored.", ipdup);
            goto cleanipdup;
        }

        if (!select) { // CIDR
            select = ebpf_ip2nl(first.addr8, ip, AF_INET, ipdup);
            if (select)
                goto cleanipdup;

            select = (int)str2i(end);
            if (select < NETDATA_MINIMUM_IPV4_CIDR || select > NETDATA_MAXIMUM_IPV4_CIDR) {
                netdata_log_info("The specified CIDR %s is not valid, the IP %s will be ignored.", end, ip);
                goto cleanipdup;
            }

            last.addr32[0] = htonl(ebpf_broadcast(ntohl(first.addr32[0]), select));
            // This was added to remove
            // https://app.codacy.com/manual/netdata/netdata/pullRequest?prid=5810941&bid=19021977
            UNUSED(last.addr32[0]);

            uint32_t ipv4_test = htonl(ebpf_ipv4_network(ntohl(first.addr32[0]), select));
            if (first.addr32[0] != ipv4_test) {
                first.addr32[0] = ipv4_test;
                struct in_addr ipv4_convert;
                ipv4_convert.s_addr = ipv4_test;
                char ipv4_msg[INET_ADDRSTRLEN];
                if (inet_ntop(AF_INET, &ipv4_convert, ipv4_msg, INET_ADDRSTRLEN))
                    netdata_log_info("The network value of CIDR %s was updated for %s .", ipdup, ipv4_msg);
            }
        } else { // Range
            select = ebpf_ip2nl(first.addr8, ip, AF_INET, ipdup);
            if (select)
                goto cleanipdup;

            select = ebpf_ip2nl(last.addr8, end, AF_INET, ipdup);
            if (select)
                goto cleanipdup;
        }

        if (htonl(first.addr32[0]) > htonl(last.addr32[0])) {
            netdata_log_info(
                "The specified range %s is invalid, the second address is smallest than the first, it will be ignored.",
                ipdup);
            goto cleanipdup;
        }
    } else if (is_ipv6) { // IPV6
        if (!*end) {      // Unique
            select = ebpf_ip2nl(first.addr8, ip, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;

            memcpy(last.addr8, first.addr8, sizeof(first.addr8));
        } else if (*end == '-') {
            *end++ = 0x00;
            if (*end == '!') {
                netdata_log_info(
                    "The exclusion cannot be in the second part of the range %s, it will be ignored.", ipdup);
                goto cleanipdup;
            }

            select = ebpf_ip2nl(first.addr8, ip, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;

            select = ebpf_ip2nl(last.addr8, end, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;
        } else { // CIDR
            *end++ = 0x00;
            if (*end == '!') {
                netdata_log_info(
                    "The exclusion cannot be in the second part of the range %s, it will be ignored.", ipdup);
                goto cleanipdup;
            }

            select = str2i(end);
            if (select < 0 || select > 128) {
                netdata_log_info("The CIDR %s is not valid, the address %s will be ignored.", end, ip);
                goto cleanipdup;
            }

            uint64_t prefix = (uint64_t)select;
            select = ebpf_ip2nl(first.addr8, ip, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;

            get_ipv6_last_addr(&last, &first, prefix);

            union netdata_ip_t ipv6_test;
            get_ipv6_first_addr(&ipv6_test, &first, prefix);

            if (memcmp(first.addr8, ipv6_test.addr8, sizeof(union netdata_ip_t)) != 0) {
                memcpy(first.addr8, ipv6_test.addr8, sizeof(union netdata_ip_t));

                struct in6_addr ipv6_convert;
                memcpy(ipv6_convert.s6_addr, ipv6_test.addr8, sizeof(union netdata_ip_t));

                char ipv6_msg[INET6_ADDRSTRLEN];
                if (inet_ntop(AF_INET6, &ipv6_convert, ipv6_msg, INET6_ADDRSTRLEN))
                    netdata_log_info("The network value of CIDR %s was updated for %s .", ipdup, ipv6_msg);
            }
        }

        if ((be64toh(*(uint64_t *)&first.addr64[1]) > be64toh(*(uint64_t *)&last.addr64[1]) &&
             !memcmp(first.addr64, last.addr64, sizeof(uint64_t))) ||
            (be64toh(*(uint64_t *)&first.addr64) > be64toh(*(uint64_t *)&last.addr64))) {
            netdata_log_info(
                "The specified range %s is invalid, the second address is smallest than the first, it will be ignored.",
                ipdup);
            goto cleanipdup;
        }
    } else { // Unique ip
        select = ebpf_ip2nl(first.addr8, ip, AF_INET, ipdup);
        if (select)
            goto cleanipdup;

        memcpy(last.addr8, first.addr8, sizeof(first.addr8));
    }

    ebpf_network_viewer_ip_list_t *store;

storethisip:
    store = callocz(1, sizeof(ebpf_network_viewer_ip_list_t));
    store->value = ipdup;
    store->hash = simple_hash(ipdup);
    store->ver = (uint8_t)(!is_ipv6) ? AF_INET : AF_INET6;
    memcpy(store->first.addr8, first.addr8, sizeof(first.addr8));
    memcpy(store->last.addr8, last.addr8, sizeof(last.addr8));

    ebpf_fill_ip_list_unsafe(list, store, "socket");
    return;

cleanipdup:
    freez(ipdup);
    freez(enddup);
}

/**
 * Parse IP Range
 *
 * Parse the IP ranges given and create Network Viewer IP Structure
 *
 * @param ptr  is a pointer with the text to parse.
 */
void ebpf_parse_ips_unsafe(const char *ptr)
{
    // No value
    if (unlikely(!ptr))
        return;

    while (likely(ptr)) {
        // Move forward until next valid character
        while (isspace(*ptr))
            ptr++;

        // No valid value found
        if (unlikely(!*ptr))
            return;

        // Find space that ends the list
        char *end = strchr(ptr, ' ');
        if (end) {
            *end++ = '\0';
        }

        int neg = 0;
        if (*ptr == '!') {
            neg++;
            ptr++;
        }

        if (isascii(*ptr)) { // Parse port
            ebpf_parse_ip_list_unsafe(
                (!neg) ? (void **)&network_viewer_opt.included_ips : (void **)&network_viewer_opt.excluded_ips, ptr);
        }

        ptr = end;
    }
}

/**
 * Fill Port list
 *
 * @param out a pointer to the link list.
 * @param in the structure that will be linked.
 */
static inline void fill_port_list(ebpf_network_viewer_port_list_t **out, ebpf_network_viewer_port_list_t *in)
{
    if (likely(*out)) {
        ebpf_network_viewer_port_list_t *move = *out, *store = *out;
        uint16_t first = ntohs(in->first);
        uint16_t last = ntohs(in->last);
        while (move) {
            uint16_t cmp_first = ntohs(move->first);
            uint16_t cmp_last = ntohs(move->last);
            if (cmp_first <= first && first <= cmp_last && cmp_first <= last && last <= cmp_last) {
                netdata_log_info(
                    "The range/value (%u, %u) is inside the range/value (%u, %u) already inserted, it will be ignored.",
                    first,
                    last,
                    cmp_first,
                    cmp_last);
                freez(in->value);
                freez(in);
                return;
            } else if (first <= cmp_first && cmp_first <= last && first <= cmp_last && cmp_last <= last) {
                netdata_log_info(
                    "The range (%u, %u) is bigger than previous range (%u, %u) already inserted, the previous will be ignored.",
                    first,
                    last,
                    cmp_first,
                    cmp_last);
                freez(move->value);
                move->value = in->value;
                move->first = in->first;
                move->last = in->last;
                freez(in);
                return;
            }

            store = move;
            move = move->next;
        }

        store->next = in;
    } else {
        *out = in;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    netdata_log_info(
        "Adding values %s( %u, %u) to %s port list used on network viewer",
        in->value,
        in->first,
        in->last,
        (*out == network_viewer_opt.included_port) ? "included" : "excluded");
#endif
}

/**
 * Parse Service List
 *
 * @param out a pointer to store the link list
 * @param service the service used to create the structure that will be linked.
 */
static void ebpf_parse_service_list(void **out, const char *service)
{
    ebpf_network_viewer_port_list_t **list = (ebpf_network_viewer_port_list_t **)out;
    struct servent *serv = getservbyname((const char *)service, "tcp");
    if (!serv)
        serv = getservbyname((const char *)service, "udp");

    if (!serv) {
        netdata_log_info("Cannot resolve the service '%s' with protocols TCP and UDP, it will be ignored", service);
        return;
    }

    ebpf_network_viewer_port_list_t *w = callocz(1, sizeof(ebpf_network_viewer_port_list_t));
    w->value = strdupz(service);
    w->hash = simple_hash(service);

    w->first = w->last = (uint16_t)serv->s_port;

    fill_port_list(list, w);
}

/**
 * Parse port list
 *
 * Parse an allocated port list with the range given
 *
 * @param out a pointer to store the link list
 * @param range the informed range for the user.
 */
static void ebpf_parse_port_list(void **out, const char *range_param)
{
    char range[strlen(range_param) + 1];
    strncpyz(range, range_param, strlen(range_param));

    int first, last;
    ebpf_network_viewer_port_list_t **list = (ebpf_network_viewer_port_list_t **)out;

    char *copied = strdupz(range);
    if (*range == '*' && *(range + 1) == '\0') {
        first = 1;
        last = 65535;

        ebpf_clean_port_structure(list);
        goto fillenvpl;
    }

    char *end = range;
    //Move while I cannot find a separator
    while (*end && *end != ':' && *end != '-')
        end++;

    //It has a range
    if (likely(*end)) {
        *end++ = '\0';
        if (*end == '!') {
            netdata_log_info(
                "The exclusion cannot be in the second part of the range, the range %s will be ignored.", copied);
            freez(copied);
            return;
        }
        last = str2i((const char *)end);
    } else {
        last = 0;
    }

    first = str2i((const char *)range);
    if (first < NETDATA_MINIMUM_PORT_VALUE || first > NETDATA_MAXIMUM_PORT_VALUE) {
        netdata_log_info("The first port %d of the range \"%s\" is invalid and it will be ignored!", first, copied);
        freez(copied);
        return;
    }

    if (!last)
        last = first;

    if (last < NETDATA_MINIMUM_PORT_VALUE || last > NETDATA_MAXIMUM_PORT_VALUE) {
        netdata_log_info(
            "The second port %d of the range \"%s\" is invalid and the whole range will be ignored!", last, copied);
        freez(copied);
        return;
    }

    if (first > last) {
        netdata_log_info(
            "The specified order %s is wrong, the smallest value is always the first, it will be ignored!", copied);
        freez(copied);
        return;
    }

    ebpf_network_viewer_port_list_t *w;
fillenvpl:
    w = callocz(1, sizeof(ebpf_network_viewer_port_list_t));
    w->value = copied;
    w->hash = simple_hash(copied);
    w->first = (uint16_t)first;
    w->last = (uint16_t)last;
    w->cmp_first = (uint16_t)first;
    w->cmp_last = (uint16_t)last;

    fill_port_list(list, w);
}

/**
 * Parse Port Range
 *
 * Parse the port ranges given and create Network Viewer Port Structure
 *
 * @param ptr  is a pointer with the text to parse.
 */
void ebpf_parse_ports(const char *ptr)
{
    // No value
    if (unlikely(!ptr))
        return;

    while (likely(ptr)) {
        // Move forward until next valid character
        while (isspace(*ptr))
            ptr++;

        // No valid value found
        if (unlikely(!*ptr))
            return;

        // Find space that ends the list
        char *end = strchr(ptr, ' ');
        if (end) {
            *end++ = '\0';
        }

        int neg = 0;
        if (*ptr == '!') {
            neg++;
            ptr++;
        }

        if (isdigit(*ptr)) { // Parse port
            ebpf_parse_port_list(
                (!neg) ? (void **)&network_viewer_opt.included_port : (void **)&network_viewer_opt.excluded_port, ptr);
        } else if (isalpha(*ptr)) { // Parse service
            ebpf_parse_service_list(
                (!neg) ? (void **)&network_viewer_opt.included_port : (void **)&network_viewer_opt.excluded_port, ptr);
        } else if (*ptr == '*') { // All
            ebpf_parse_port_list(
                (!neg) ? (void **)&network_viewer_opt.included_port : (void **)&network_viewer_opt.excluded_port, ptr);
        }

        ptr = end;
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
void ebpf_global_labels(
    netdata_syscall_stat_t *is,
    netdata_publish_syscall_t *pio,
    char **dim,
    char **name,
    int *algorithm,
    int end)
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
    em->enabled = NETDATA_THREAD_EBPF_RUNNING;

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
    for (i = 0; ebpf_modules[i].info.thread_name; i++) {
        ebpf_modules[i].enabled = NETDATA_THREAD_EBPF_NOT_RUNNING;
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
    for (i = 0; ebpf_modules[i].info.thread_name; i++) {
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
    for (i = 0; ebpf_modules[i].info.thread_name; i++) {
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
    netdata_mutex_lock(&lock);
    ebpf_update_stats(&plugin_statistics, em);
    netdata_mutex_unlock(&lock);
}

/**
 * Print help on standard error for user knows how to use the collector.
 */
void ebpf_print_help()
{
    fprintf(
        stderr,
        "\n"
        " Netdata ebpf.plugin %s\n"
        " Copyright 2018-2025 Netdata Inc.\n"
        " Released under GNU General Public License v3 or later.\n"
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
        NETDATA_VERSION);
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
        } else {
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
 * Is ip inside the range
 *
 * Check if the ip is inside a IP range
 *
 * @param rfirst    the first ip address of the range
 * @param rlast     the last ip address of the range
 * @param cmpfirst  the first ip to compare
 * @param cmplast   the last ip to compare
 * @param family    the IP family
 *
 * @return It returns 1 if the IP is inside the range and 0 otherwise
 */
static int ebpf_is_ip_inside_range(
    union netdata_ip_t *rfirst,
    union netdata_ip_t *rlast,
    union netdata_ip_t *cmpfirst,
    union netdata_ip_t *cmplast,
    int family)
{
    if (family == AF_INET) {
        if ((rfirst->addr32[0] <= cmpfirst->addr32[0]) && (rlast->addr32[0] >= cmplast->addr32[0]))
            return 1;
    } else {
        if (memcmp(rfirst->addr8, cmpfirst->addr8, sizeof(union netdata_ip_t)) <= 0 &&
            memcmp(rlast->addr8, cmplast->addr8, sizeof(union netdata_ip_t)) >= 0) {
            return 1;
        }
    }
    return 0;
}

/**
 * Fill IP list
 *
 * @param out a pointer to the link list.
 * @param in the structure that will be linked.
 * @param table the modified table.
 */
void ebpf_fill_ip_list_unsafe(
    ebpf_network_viewer_ip_list_t **out,
    ebpf_network_viewer_ip_list_t *in,
    char *table __maybe_unused)
{
    if (in->ver == AF_INET) { // It is simpler to compare using host order
        in->first.addr32[0] = ntohl(in->first.addr32[0]);
        in->last.addr32[0] = ntohl(in->last.addr32[0]);
    }
    if (likely(*out)) {
        ebpf_network_viewer_ip_list_t *move = *out, *store = *out;
        while (move) {
            if (in->ver == move->ver &&
                ebpf_is_ip_inside_range(&move->first, &move->last, &in->first, &in->last, in->ver)) {
#ifdef NETDATA_DEV_MODE
                netdata_log_info(
                    "The range/value (%s) is inside the range/value (%s) already inserted, it will be ignored.",
                    in->value,
                    move->value);
#endif
                freez(in->value);
                freez(in);
                return;
            }
            store = move;
            move = move->next;
        }

        store->next = in;
    } else {
        *out = in;
    }

#ifdef NETDATA_DEV_MODE
    char first[256], last[512];
    if (in->ver == AF_INET) {
        netdata_log_info(
            "Adding values %s: (%u - %u) to %s IP list \"%s\" used on network viewer",
            in->value,
            in->first.addr32[0],
            in->last.addr32[0],
            (*out == network_viewer_opt.included_ips) ? "included" : "excluded",
            table);
    } else {
        if (inet_ntop(AF_INET6, in->first.addr8, first, INET6_ADDRSTRLEN) &&
            inet_ntop(AF_INET6, in->last.addr8, last, INET6_ADDRSTRLEN))
            netdata_log_info(
                "Adding values %s - %s to %s IP list \"%s\" used on network viewer",
                first,
                last,
                (*out == network_viewer_opt.included_ips) ? "included" : "excluded",
                table);
    }
#endif
}

/**
 * Link hostname
 *
 * @param out is the output link list
 * @param in the hostname to add to list.
 */
static void ebpf_link_hostname(ebpf_network_viewer_hostname_list_t **out, ebpf_network_viewer_hostname_list_t *in)
{
    if (likely(*out)) {
        ebpf_network_viewer_hostname_list_t *move = *out;
        for (; move->next; move = move->next) {
            if (move->hash == in->hash && !strcmp(move->value, in->value)) {
                netdata_log_info("The hostname %s was already inserted, it will be ignored.", in->value);
                freez(in->value);
                simple_pattern_free(in->value_pattern);
                freez(in);
                return;
            }
        }

        move->next = in;
    } else {
        *out = in;
    }
#ifdef NETDATA_INTERNAL_CHECKS
    netdata_log_info(
        "Adding value %s to %s hostname list used on network viewer",
        in->value,
        (*out == network_viewer_opt.included_hostnames) ? "included" : "excluded");
#endif
}

/**
 * Link Hostnames
 *
 * Parse the list of hostnames to create the link list.
 * This is not associated with the IP, because simple patterns like *example* cannot be resolved to IP.
 *
 * @param out is the output link list
 * @param parse is a pointer with the text to parser.
 */
static void ebpf_link_hostnames(const char *parse)
{
    // No value
    if (unlikely(!parse))
        return;

    while (likely(parse)) {
        // Find the first valid value
        while (isspace(*parse))
            parse++;

        // No valid value found
        if (unlikely(!*parse))
            return;

        // Find space that ends the list
        char *end = strchr(parse, ' ');
        if (end) {
            *end++ = '\0';
        }

        int neg = 0;
        if (*parse == '!') {
            neg++;
            parse++;
        }

        ebpf_network_viewer_hostname_list_t *hostname = callocz(1, sizeof(ebpf_network_viewer_hostname_list_t));
        hostname->value = strdupz(parse);
        hostname->hash = simple_hash(parse);
        hostname->value_pattern = simple_pattern_create(parse, NULL, SIMPLE_PATTERN_EXACT, true);

        ebpf_link_hostname(
            (!neg) ? &network_viewer_opt.included_hostnames : &network_viewer_opt.excluded_hostnames, hostname);

        parse = end;
    }
}

/**
 * Parse network viewer section
 *
 * @param cfg the configuration structure
 */
void parse_network_viewer_section(struct config *cfg)
{
    network_viewer_opt.hostname_resolution_enabled =
        inicfg_get_boolean(cfg, EBPF_NETWORK_VIEWER_SECTION, EBPF_CONFIG_RESOLVE_HOSTNAME, CONFIG_BOOLEAN_NO);

    network_viewer_opt.service_resolution_enabled =
        inicfg_get_boolean(cfg, EBPF_NETWORK_VIEWER_SECTION, EBPF_CONFIG_RESOLVE_SERVICE, CONFIG_BOOLEAN_YES);

    const char *value = inicfg_get(cfg, EBPF_NETWORK_VIEWER_SECTION, EBPF_CONFIG_PORTS, NULL);
    ebpf_parse_ports(value);

    if (network_viewer_opt.hostname_resolution_enabled) {
        value = inicfg_get(cfg, EBPF_NETWORK_VIEWER_SECTION, EBPF_CONFIG_HOSTNAMES, NULL);
        ebpf_link_hostnames(value);
    } else {
        netdata_log_info("Name resolution is disabled, collector will not parse \"hostnames\" list.");
    }

    value = inicfg_get(cfg, EBPF_NETWORK_VIEWER_SECTION, "ips", NULL);
    //"ips", "!127.0.0.1/8 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 fc00::/7 !::1/128");
    ebpf_parse_ips_unsafe(value);
}

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
    for (l = 0; l < lines; l++) {
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
void ebpf_read_local_addresses_unsafe()
{
    struct ifaddrs *ifaddr, *ifa;
    if (getifaddrs(&ifaddr) == -1) {
        netdata_log_error(
            "Cannot get the local IP addresses, it is no possible to do separation between inbound and outbound connections");
        return;
    }

    char *notext = {"No text representation"};
    for (ifa = ifaddr; ifa != NULL; ifa = ifa->ifa_next) {
        if (ifa->ifa_addr == NULL)
            continue;

        if ((ifa->ifa_addr->sa_family != AF_INET) && (ifa->ifa_addr->sa_family != AF_INET6))
            continue;

        ebpf_network_viewer_ip_list_t *w = callocz(1, sizeof(ebpf_network_viewer_ip_list_t));

        int family = ifa->ifa_addr->sa_family;
        w->ver = (uint8_t)family;
        char text[INET6_ADDRSTRLEN];
        if (family == AF_INET) {
            struct sockaddr_in *in = (struct sockaddr_in *)ifa->ifa_addr;

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
            struct sockaddr_in6 *in6 = (struct sockaddr_in6 *)ifa->ifa_addr;

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

        ebpf_fill_ip_list_unsafe(
            (family == AF_INET) ? &network_viewer_opt.ipv4_local_ip : &network_viewer_opt.ipv6_local_ip, w, "selector");
    }

    freeifaddrs(ifaddr);
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
    //    ebpf_pids = callocz((size_t)pid_max, sizeof(ebpf_pid_data_t));
    ebpf_aral_init();
}

/**
 * Define how to load the ebpf programs
 *
 * @param ptr the option given by users
 */
static inline void ebpf_how_to_load(const char *ptr)
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

    int value = (int)inicfg_get_number(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_UPDATE_EVERY, update_every);

    for (i = 0; ebpf_modules[i].info.thread_name; i++) {
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
    uint32_t value = (uint32_t)inicfg_get_number(
        &collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_PID_SIZE, ND_EBPF_DEFAULT_PID_SIZE);
    for (int i = 0; ebpf_modules[i].info.thread_name; i++) {
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
    uint32_t value =
        (uint32_t)inicfg_get_number(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_LIFETIME, EBPF_DEFAULT_LIFETIME);

    for (int i = 0; ebpf_modules[i].info.thread_name; i++) {
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
    for (i = 0; ebpf_modules[i].info.thread_name; i++) {
        ebpf_modules[i].load &= ~NETDATA_EBPF_LOAD_METHODS;
        ebpf_modules[i].load |= load | origin;
    }
}

/**
 *  Update mode
 *
 *  @param str      value read from configuration file.
 *  @param origin   specify the configuration file loaded
 */
static inline void epbf_update_load_mode(const char *str, netdata_ebpf_load_mode_t origin)
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
    int value = inicfg_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_MAPS_PER_CORE, CONFIG_BOOLEAN_YES);

    for (int i = 0; ebpf_modules[i].info.thread_name; i++) {
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
static void read_collector_values(int *disable_cgroups, int update_every, netdata_ebpf_load_mode_t origin)
{
    // Read global section
    const char *value;
    if (inicfg_exists(&collector_config, EBPF_GLOBAL_SECTION, "load")) // Backward compatibility
        value = inicfg_get(&collector_config, EBPF_GLOBAL_SECTION, "load", EBPF_CFG_LOAD_MODE_DEFAULT);
    else
        value = inicfg_get(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_LOAD_MODE, EBPF_CFG_LOAD_MODE_DEFAULT);

    ebpf_how_to_load(value);

    btf_path = inicfg_get(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_PROGRAM_PATH, EBPF_DEFAULT_BTF_PATH);

#ifdef LIBBPF_MAJOR_VERSION
    default_btf = ebpf_load_btf_file(btf_path, EBPF_DEFAULT_BTF_FILE);
#endif

    value = inicfg_get(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_TYPE_FORMAT, EBPF_CFG_DEFAULT_PROGRAM);

    epbf_update_load_mode(value, origin);

    ebpf_update_interval(update_every);

    ebpf_update_table_size();

    ebpf_update_lifetime();

    // This is kept to keep compatibility
    uint32_t enabled = inicfg_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, "disable apps", CONFIG_BOOLEAN_NO);
    if (!enabled) {
        // Apps is a positive sentence, so we need to invert the values to disable apps.
        enabled = inicfg_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_APPLICATION, CONFIG_BOOLEAN_YES);
        enabled = (enabled == CONFIG_BOOLEAN_NO) ? CONFIG_BOOLEAN_YES : CONFIG_BOOLEAN_NO;
    }

    ebpf_set_apps_mode(!enabled);

    // Cgroup is a positive sentence, so we need to invert the values to disable apps.
    // We are using the same pattern for cgroup and apps
    enabled = inicfg_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_CGROUP, CONFIG_BOOLEAN_NO);
    *disable_cgroups = (enabled == CONFIG_BOOLEAN_NO) ? CONFIG_BOOLEAN_YES : CONFIG_BOOLEAN_NO;

    ebpf_update_map_per_core();

    // Read ebpf programs section
    enabled = inicfg_get_boolean(
        &collector_config,
        EBPF_PROGRAMS_SECTION,
        ebpf_modules[EBPF_MODULE_PROCESS_IDX].info.config_name,
        CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_PROCESS_IDX, *disable_cgroups);
    }

    // This is kept to keep compatibility
    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "network viewer", CONFIG_BOOLEAN_NO);
    if (!enabled)
        enabled = inicfg_get_boolean(
            &collector_config,
            EBPF_PROGRAMS_SECTION,
            ebpf_modules[EBPF_MODULE_SOCKET_IDX].info.config_name,
            CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SOCKET_IDX, *disable_cgroups);
    }

    // This is kept to keep compatibility
    enabled = inicfg_get_boolean(
        &collector_config, EBPF_PROGRAMS_SECTION, "network connection monitoring", CONFIG_BOOLEAN_YES);
    if (!enabled)
        enabled =
            inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "network connections", CONFIG_BOOLEAN_YES);

    network_viewer_opt.enabled = enabled;
    if (enabled) {
        if (!ebpf_modules[EBPF_MODULE_SOCKET_IDX].enabled)
            ebpf_enable_chart(EBPF_MODULE_SOCKET_IDX, *disable_cgroups);

        // Read network viewer section if network viewer is enabled
        // This is kept here to keep backward compatibility
        parse_network_viewer_section(&collector_config);
        ebpf_parse_service_name_section(&collector_config);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "cachestat", CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_CACHESTAT_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "sync", CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SYNC_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "dcstat", CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_DCSTAT_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "swap", CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SWAP_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "vfs", CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_VFS_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "filesystem", CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_FILESYSTEM_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "disk", CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_DISK_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "mount", CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_MOUNT_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "fd", CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_FD_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "hardirq", CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_HARDIRQ_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "softirq", CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SOFTIRQ_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "oomkill", CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_OOMKILL_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "shm", CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SHM_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "mdflush", CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_MDFLUSH_IDX, *disable_cgroups);
    }
}

static void ebpf_set_ipc_value(const char *integration)
{
    if (!strcmp(integration, NETDATA_EBPF_IPC_INTEGRATION_SHM)) {
        integration_with_collectors = NETDATA_EBPF_INTEGRATION_SHM;
        return;
    } else if (!strcmp(integration, NETDATA_EBPF_IPC_INTEGRATION_SOCKET)) {
        integration_with_collectors = NETDATA_EBPF_INTEGRATION_SOCKET;
        return;
    }
    integration_with_collectors = NETDATA_EBPF_INTEGRATION_DISABLED;
}

static void ebpf_parse_ipc_section()
{
    const char *integration = inicfg_get(
        &collector_config,
        NETDATA_EBPF_IPC_SECTION,
        NETDATA_EBPF_IPC_INTEGRATION,
        NETDATA_EBPF_IPC_INTEGRATION_DISABLED);
    ebpf_set_ipc_value(integration);

    ipc_sockets.default_bind_to = inicfg_get(
        &collector_config, NETDATA_EBPF_IPC_SECTION, NETDATA_EBPF_IPC_BIND_TO, NETDATA_EBPF_IPC_BIND_TO_DEFAULT);

    ipc_sockets.backlog =
        (int)inicfg_get_number(&collector_config, NETDATA_EBPF_IPC_SECTION, NETDATA_EBPF_IPC_BACKLOG, 20);
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
    if (!inicfg_load(&collector_config, lpath, 0, NULL)) {
        snprintf(lpath, 4095, "%s/%s", path, NETDATA_EBPF_OLD_CONFIG_FILE);
        if (!inicfg_load(&collector_config, lpath, 0, NULL)) {
            return -1;
        }
        origin = EBPF_LOADED_FROM_STOCK;
    } else
        origin = EBPF_LOADED_FROM_USER;

    read_collector_values(disable_cgroups, update_every, origin);
    ebpf_parse_ipc_section();

    return 0;
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
    if (ipc_data.total > 0 )
        ipc_value = ( (NETDATA_DOUBLE)ipc_data.current/(NETDATA_DOUBLE)ipc_data.total )*100.0;
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

    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_module_t *wem = &ebpf_modules[i];
        if (!wem->functions.fnct_routine)
            continue;

        ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, (char *)wem->functions.fcnt_thread_chart_name, "");
        write_chart_dimension((char *)wem->info.thread_name, (wem->enabled < NETDATA_THREAD_EBPF_STOPPING) ? 1 : 0);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, (char *)wem->functions.fcnt_thread_lifetime_name, "");
        write_chart_dimension(
            (char *)wem->info.thread_name,
            (wem->lifetime && wem->enabled < NETDATA_THREAD_EBPF_STOPPING) ?
                (long long)(wem->lifetime - wem->running_time) :
                0);
        ebpf_write_end_chart();
    }
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

    ebpf_write_global_dimension(
        "positions",
        "positions",
        ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);
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
        em->functions.fcnt_thread_chart_name = strdupz(name);
        ebpf_create_thread_chart(name, "Threads running.", "boolean", j++, update_every, em);

        em->functions.order_thread_lifetime = j;
        snprintfz(name, sizeof(name) - 1, "%s_%s", NETDATA_EBPF_LIFE_TIME, em->info.thread_name);
        em->functions.fcnt_thread_lifetime_name = strdupz(name);
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
    snprintfz(filename, FILENAME_MAX, "/proc/%d/status", pid);

    procfile *ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
    if (unlikely(!ff)) {
        netdata_log_error("Cannot open %s", filename);
        return name;
    }

    ff = procfile_readall(ff);
    if (unlikely(!ff))
        return name;

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

    // Process is not running
    char *prev_name = ebpf_get_process_name(old_pid);
    if (!prev_name)
        return;

    char *current_name = ebpf_get_process_name(pid);

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
    snprintfz(filename, length, "%s/var/run/ebpf.pid", netdata_configured_host_prefix);
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
    for (i = 0; ebpf_modules[i].info.thread_name; i++) {
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
