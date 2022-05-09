// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/time.h>
#include <sys/resource.h>
#include <ifaddrs.h>

#include "ebpf.h"
#include "ebpf_socket.h"
#include "libnetdata/required_dummies.h"

/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

char *ebpf_plugin_dir = PLUGINS_DIR;
static char *ebpf_configured_log_dir = LOG_DIR;

char *ebpf_algorithms[] = {"absolute", "incremental"};
static int thread_finished = 0;
int close_ebpf_plugin = 0;
struct config collector_config = { .first_section = NULL,
                                   .last_section = NULL,
                                   .mutex = NETDATA_MUTEX_INITIALIZER,
                                   .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
                                              .rwlock = AVL_LOCK_INITIALIZER } };

int running_on_kernel = 0;
int ebpf_nprocs;
int isrh = 0;
uint32_t finalized_threads = 1;

pthread_mutex_t lock;
pthread_mutex_t collect_data_mutex;
pthread_cond_t collect_data_cond_var;

ebpf_module_t ebpf_modules[] = {
    { .thread_name = "process", .config_name = "process", .enabled = 0, .start_routine = ebpf_process_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_process_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &process_config,
      .config_file = NETDATA_PROCESS_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_10,
      .load = EBPF_LOAD_LEGACY, .targets = NULL},
    { .thread_name = "socket", .config_name = "socket", .enabled = 0, .start_routine = ebpf_socket_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_socket_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &socket_config,
      .config_file = NETDATA_NETWORK_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .load = EBPF_LOAD_LEGACY, .targets = socket_targets},
    { .thread_name = "cachestat", .config_name = "cachestat", .enabled = 0, .start_routine = ebpf_cachestat_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_cachestat_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &cachestat_config,
      .config_file = NETDATA_CACHESTAT_CONFIG_FILE,
      .kernels = NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18|
                 NETDATA_V5_4 | NETDATA_V5_15 | NETDATA_V5_16,
      .load = EBPF_LOAD_LEGACY, .targets = cachestat_targets},
    { .thread_name = "sync", .config_name = "sync", .enabled = 0, .start_routine = ebpf_sync_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0, .apps_routine = NULL, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &sync_config,
      .config_file = NETDATA_SYNC_CONFIG_FILE,
      // All syscalls have the same kernels
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .load = EBPF_LOAD_LEGACY, .targets = sync_targets},
    { .thread_name = "dc", .config_name = "dc", .enabled = 0, .start_routine = ebpf_dcstat_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_dcstat_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &dcstat_config,
      .config_file = NETDATA_DIRECTORY_DCSTAT_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .load = EBPF_LOAD_LEGACY, .targets = dc_targets},
    { .thread_name = "swap", .config_name = "swap", .enabled = 0, .start_routine = ebpf_swap_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_swap_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &swap_config,
      .config_file = NETDATA_DIRECTORY_SWAP_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .load = EBPF_LOAD_LEGACY, .targets = swap_targets},
    { .thread_name = "vfs", .config_name = "vfs", .enabled = 0, .start_routine = ebpf_vfs_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_vfs_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &vfs_config,
      .config_file = NETDATA_DIRECTORY_VFS_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .load = EBPF_LOAD_LEGACY, .targets = NULL},
    { .thread_name = "filesystem", .config_name = "filesystem", .enabled = 0, .start_routine = ebpf_filesystem_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0, .apps_routine = NULL, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &fs_config,
      .config_file = NETDATA_FILESYSTEM_CONFIG_FILE,
      //We are setting kernels as zero, because we load eBPF programs according the kernel running.
      .kernels = 0, .load = EBPF_LOAD_LEGACY, .targets = NULL },
    { .thread_name = "disk", .config_name = "disk", .enabled = 0, .start_routine = ebpf_disk_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0, .apps_routine = NULL, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &disk_config,
      .config_file = NETDATA_DISK_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .load = EBPF_LOAD_LEGACY, .targets = NULL},
    { .thread_name = "mount", .config_name = "mount", .enabled = 0, .start_routine = ebpf_mount_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0, .apps_routine = NULL, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &mount_config,
      .config_file = NETDATA_MOUNT_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .load = EBPF_LOAD_LEGACY, .targets = mount_targets},
    { .thread_name = "fd", .config_name = "fd", .enabled = 0, .start_routine = ebpf_fd_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_fd_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &fd_config,
      .config_file = NETDATA_FD_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4 | NETDATA_V5_11,
      .load = EBPF_LOAD_LEGACY, .targets = NULL},
    { .thread_name = "hardirq", .config_name = "hardirq", .enabled = 0, .start_routine = ebpf_hardirq_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0, .apps_routine = NULL, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &hardirq_config,
      .config_file = NETDATA_HARDIRQ_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .load = EBPF_LOAD_LEGACY, .targets = NULL},
    { .thread_name = "softirq", .config_name = "softirq", .enabled = 0, .start_routine = ebpf_softirq_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0, .apps_routine = NULL, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &softirq_config,
      .config_file = NETDATA_SOFTIRQ_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .load = EBPF_LOAD_LEGACY, .targets = NULL},
    { .thread_name = "oomkill", .config_name = "oomkill", .enabled = 0, .start_routine = ebpf_oomkill_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_oomkill_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &oomkill_config,
      .config_file = NETDATA_OOMKILL_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .load = EBPF_LOAD_LEGACY, .targets = NULL},
    { .thread_name = "shm", .config_name = "shm", .enabled = 0, .start_routine = ebpf_shm_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0,
      .apps_routine = ebpf_shm_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &shm_config,
      .config_file = NETDATA_DIRECTORY_SHM_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .load = EBPF_LOAD_LEGACY, .targets = shm_targets},
    { .thread_name = "mdflush", .config_name = "mdflush", .enabled = 0, .start_routine = ebpf_mdflush_thread,
      .update_every = EBPF_DEFAULT_UPDATE_EVERY, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO,
      .cgroup_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY, .optional = 0, .apps_routine = NULL, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &mdflush_config,
      .config_file = NETDATA_DIRECTORY_MDFLUSH_CONFIG_FILE,
      .kernels =  NETDATA_V3_10 | NETDATA_V4_14 | NETDATA_V4_16 | NETDATA_V4_18 | NETDATA_V5_4,
      .load = EBPF_LOAD_LEGACY, .targets = NULL},
    { .thread_name = NULL, .enabled = 0, .start_routine = NULL, .update_every = EBPF_DEFAULT_UPDATE_EVERY,
      .global_charts = 0, .apps_charts = CONFIG_BOOLEAN_NO, .cgroup_charts = CONFIG_BOOLEAN_NO,
      .mode = MODE_ENTRY, .optional = 0, .apps_routine = NULL, .maps = NULL, .pid_map_size = 0, .names = NULL,
      .cfg = NULL, .config_name = NULL, .kernels = 0, .load = EBPF_LOAD_LEGACY, .targets = NULL},
};

// Link with apps.plugin
ebpf_process_stat_t *global_process_stat = NULL;

// Link with cgroup.plugin
netdata_ebpf_cgroup_shm_t shm_ebpf_cgroup = {NULL, NULL};
int shm_fd_ebpf_cgroup = -1;
sem_t *shm_sem_ebpf_cgroup = SEM_FAILED;
pthread_mutex_t mutex_cgroup_shm;

//Network viewer
ebpf_network_viewer_options_t network_viewer_opt;

// Statistic
ebpf_plugin_stats_t plugin_statistics = {.core = 0, .legacy = 0, .running = 0, .threads = 0, .tracepoints = 0,
                                         .probes = 0, .retprobes = 0, .trampolines = 0};

#ifdef LIBBPF_MAJOR_VERSION
struct btf *default_btf = NULL;
#endif
char *btf_path = NULL;

/*****************************************************************
 *
 *  FUNCTIONS USED TO CLEAN MEMORY AND OPERATE SYSTEM FILES
 *
 *****************************************************************/

/**
 * Clean Loaded Events
 *
 * This function cleans the events previous loaded on Linux.
void clean_loaded_events()
{
    int event_pid;
    for (event_pid = 0; ebpf_modules[event_pid].probes; event_pid++)
        clean_kprobe_events(NULL, (int)ebpf_modules[event_pid].thread_id, ebpf_modules[event_pid].probes);
}
 */

/**
 * Close the collector gracefully
 *
 * @param sig is the signal number used to close the collector
 */
static void ebpf_exit(int sig)
{
    close_ebpf_plugin = 1;
    static int remove_pid = 0;

    // When both threads were not finished case I try to go in front this address, the collector will crash
    if (!thread_finished) {
        return;
    }

    if (ebpf_modules[EBPF_MODULE_SOCKET_IDX].enabled) {
        ebpf_modules[EBPF_MODULE_SOCKET_IDX].enabled = 0;
        clean_socket_apps_structures();
        freez(socket_bandwidth_curr);
    }

    if (ebpf_modules[EBPF_MODULE_CACHESTAT_IDX].enabled) {
        ebpf_modules[EBPF_MODULE_CACHESTAT_IDX].enabled = 0;
        clean_cachestat_pid_structures();
        freez(cachestat_pid);
    }

    if (ebpf_modules[EBPF_MODULE_DCSTAT_IDX].enabled) {
        ebpf_modules[EBPF_MODULE_DCSTAT_IDX].enabled = 0;
        clean_dcstat_pid_structures();
        freez(dcstat_pid);
    }

    if (ebpf_modules[EBPF_MODULE_SWAP_IDX].enabled) {
        ebpf_modules[EBPF_MODULE_SWAP_IDX].enabled = 0;
        clean_swap_pid_structures();
        freez(swap_pid);
    }

    if (ebpf_modules[EBPF_MODULE_VFS_IDX].enabled) {
        ebpf_modules[EBPF_MODULE_VFS_IDX].enabled = 0;
        clean_vfs_pid_structures();
        freez(vfs_pid);
    }

    if (ebpf_modules[EBPF_MODULE_FD_IDX].enabled) {
        ebpf_modules[EBPF_MODULE_FD_IDX].enabled = 0;
        clean_fd_pid_structures();
        freez(fd_pid);
    }

    if (ebpf_modules[EBPF_MODULE_SHM_IDX].enabled) {
        ebpf_modules[EBPF_MODULE_SHM_IDX].enabled = 0;
        clean_shm_pid_structures();
        freez(shm_pid);
    }

    ebpf_close_cgroup_shm();

    ebpf_clean_cgroup_pids();
    /*
    int ret = fork();
    if (ret < 0) // error
        error("Cannot fork(), so I won't be able to clean %skprobe_events", NETDATA_DEBUGFS);
    else if (!ret) { // child
        int i;
        for (i = getdtablesize(); i >= 0; --i)
            close(i);

        int fd = open("/dev/null", O_RDWR, 0);
        if (fd != -1) {
            dup2(fd, STDIN_FILENO);
            dup2(fd, STDOUT_FILENO);
            dup2(fd, STDERR_FILENO);
        }

        if (fd > 2)
            close(fd);

        int sid = setsid();
        if (sid >= 0) {
            debug(D_EXIT, "Wait for father %d die", getpid());
            sleep_usec(200000); // Sleep 200 milliseconds to father dies.
            clean_loaded_events();
        } else {
            error("Cannot become session id leader, so I won't try to clean kprobe_events.\n");
        }
    } else { // parent
        exit(0);
    }
     */

#ifdef LIBBPF_MAJOR_VERSION
    if (default_btf) {
        btf__free(default_btf);
        default_btf = NULL;
    }
#endif

    if (!remove_pid) {
        remove_pid = 1;
        char filename[FILENAME_MAX + 1];
        ebpf_pid_file(filename, FILENAME_MAX);
        if (unlink(filename))
            error("Cannot remove PID file %s", filename);
    }

    exit(sig);
}

/*****************************************************************
 *
 *  FUNCTIONS TO CREATE CHARTS
 *
 *****************************************************************/

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
                                char *algorithm, struct target *root, int update_every, char *module)
{
    struct target *w;
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
        pio[i].algorithm = strdupz(ebpf_algorithms[algorithm[i]]);
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
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        ebpf_modules[i].mode = lmode;
    }
}

/**
 * Enable specific charts selected by user.
 *
 * @param em             the structure that will be changed
 * @param disable_apps   the status about the apps charts.
 * @param disable_cgroup the status about the cgroups charts.
 */
static inline void ebpf_enable_specific_chart(struct ebpf_module *em, int disable_apps, int disable_cgroup)
{
    em->enabled = CONFIG_BOOLEAN_YES;

    // oomkill stores data inside apps submenu, so it always need to have apps_enabled for plugin to create
    // its chart, without this comparison eBPF.plugin will try to store invalid data when apps is disabled.
    if (!disable_apps || !strcmp(em->thread_name, "oomkill")) {
        em->apps_charts = CONFIG_BOOLEAN_YES;
    }

    if (!disable_cgroup) {
        em->cgroup_charts = CONFIG_BOOLEAN_YES;
    }

    em->global_charts = CONFIG_BOOLEAN_YES;
}

/**
 * Enable all charts
 *
 * @param apps    what is the current status of apps
 * @param cgroups what is the current status of cgroups
 */
static inline void ebpf_enable_all_charts(int apps, int cgroups)
{
    int i;
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        ebpf_enable_specific_chart(&ebpf_modules[i], apps, cgroups);
    }
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
 * @param disable_apps   should I keep apps charts?
 */
static inline void ebpf_enable_chart(int idx, int disable_apps, int disable_cgroup)
{
    int i;
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        if (i == idx) {
            ebpf_enable_specific_chart(&ebpf_modules[i], disable_apps, disable_cgroup);
            break;
        }
    }
}

/**
 * Disable APPs
 *
 * Disable charts for apps loading only global charts.
 */
static inline void ebpf_disable_apps()
{
    int i;
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        ebpf_modules[i].apps_charts = 0;
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
            infoerr("failed to enable tracepoint %s:%s",
                tps[i].class, tps[i].event);
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
        error("Cannot get the local IP addresses, it is no possible to do separation between inbound and outbound connections");
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

        fill_ip_list((family == AF_INET)?&network_viewer_opt.ipv4_local_ip:&network_viewer_opt.ipv6_local_ip,
                     w,
                     "selector");
    }

    freeifaddrs(ifaddr);
}

/**
 * Start Pthread Variable
 *
 * This function starts all pthread variables.
 *
 * @return It returns 0 on success and -1.
 */
int ebpf_start_pthread_variables()
{
    pthread_mutex_init(&lock, NULL);
    pthread_mutex_init(&collect_data_mutex, NULL);

    if (pthread_cond_init(&collect_data_cond_var, NULL)) {
        thread_finished++;
        error("Cannot start conditional variable to control Apps charts.");
        return -1;
    }

    return 0;
}

/**
 * Allocate the vectors used for all threads.
 */
static void ebpf_allocate_common_vectors()
{
    all_pids = callocz((size_t)pid_max, sizeof(struct pid_stat *));
    global_process_stat = callocz((size_t)ebpf_nprocs, sizeof(ebpf_process_stat_t));
}

/**
 * Define how to load the ebpf programs
 *
 * @param ptr the option given by users
 */
static inline void how_to_load(char *ptr)
{
    if (!strcasecmp(ptr, EBPF_CFG_LOAD_MODE_RETURN))
        ebpf_set_thread_mode(MODE_RETURN);
    else if (!strcasecmp(ptr, EBPF_CFG_LOAD_MODE_DEFAULT))
        ebpf_set_thread_mode(MODE_ENTRY);
    else
        error("the option %s for \"ebpf load mode\" is not a valid option.", ptr);
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
 * Set Load mode
 *
 * @param load  default load mode.
 */
static inline void ebpf_set_load_mode(netdata_ebpf_load_mode_t load)
{
#ifdef LIBBPF_MAJOR_VERSION
    if (load == EBPF_LOAD_CORE || load == EBPF_LOAD_PLAY_DICE) {
        load = (!default_btf) ? EBPF_LOAD_LEGACY : EBPF_LOAD_CORE;
    }
#else
    load = EBPF_LOAD_LEGACY;
#endif

    int i;
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        // TO DO: Use `load` variable after we change all threads.
        ebpf_modules[i].load = EBPF_LOAD_LEGACY; // load ;
    }
}

/**
 *  Update mode
 *
 *  @param str value read from configuration file.
 */
static inline void epbf_update_load_mode(char *str)
{
    netdata_ebpf_load_mode_t load = epbf_convert_string_to_load_mode(str);

    ebpf_set_load_mode(load);
}

/**
 * Read collector values
 *
 * @param disable_apps    variable to store information related to apps.
 * @param disable_cgroups variable to store information related to cgroups.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void read_collector_values(int *disable_apps, int *disable_cgroups, int update_every)
{
    // Read global section
    char *value;
    if (appconfig_exists(&collector_config, EBPF_GLOBAL_SECTION, "load")) // Backward compatibility
        value = appconfig_get(&collector_config, EBPF_GLOBAL_SECTION, "load",
                              EBPF_CFG_LOAD_MODE_DEFAULT);
    else
        value = appconfig_get(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_LOAD_MODE,
                              EBPF_CFG_LOAD_MODE_DEFAULT);

    how_to_load(value);

    btf_path = appconfig_get(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_PROGRAM_PATH,
                             EBPF_DEFAULT_BTF_PATH);

#ifdef LIBBPF_MAJOR_VERSION
    default_btf = ebpf_load_btf_file(btf_path, EBPF_DEFAULT_BTF_FILE);
#endif

    value = appconfig_get(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_TYPE_FORMAT, EBPF_CFG_DEFAULT_PROGRAM);

    epbf_update_load_mode(value);

    ebpf_update_interval(update_every);

    ebpf_update_table_size();

    // This is kept to keep compatibility
    uint32_t enabled = appconfig_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, "disable apps",
                                             CONFIG_BOOLEAN_NO);
    if (!enabled) {
        // Apps is a positive sentence, so we need to invert the values to disable apps.
        enabled = appconfig_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_APPLICATION,
                                        CONFIG_BOOLEAN_YES);
        enabled =  (enabled == CONFIG_BOOLEAN_NO)?CONFIG_BOOLEAN_YES:CONFIG_BOOLEAN_NO;
    }
    *disable_apps = (int)enabled;

    // Cgroup is a positive sentence, so we need to invert the values to disable apps.
    // We are using the same pattern for cgroup and apps
    enabled = appconfig_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_CGROUP, CONFIG_BOOLEAN_NO);
    *disable_cgroups =  (enabled == CONFIG_BOOLEAN_NO)?CONFIG_BOOLEAN_YES:CONFIG_BOOLEAN_NO;

    // Read ebpf programs section
    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION,
                                    ebpf_modules[EBPF_MODULE_PROCESS_IDX].config_name, CONFIG_BOOLEAN_YES);
    int started = 0;
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_PROCESS_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    // This is kept to keep compatibility
    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "network viewer",
                                    CONFIG_BOOLEAN_NO);
    if (!enabled)
        enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION,
                                        ebpf_modules[EBPF_MODULE_SOCKET_IDX].config_name,
                                        CONFIG_BOOLEAN_NO);

    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SOCKET_IDX, *disable_apps, *disable_cgroups);
        // Read network viewer section if network viewer is enabled
        // This is kept here to keep backward compatibility
        parse_network_viewer_section(&collector_config);
        parse_service_name_section(&collector_config);
        started++;
    }

    // This is kept to keep compatibility
    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "network connection monitoring",
                                    CONFIG_BOOLEAN_NO);
    if (!enabled)
        enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "network connections",
                                        CONFIG_BOOLEAN_NO);
    ebpf_modules[EBPF_MODULE_SOCKET_IDX].optional = (int)enabled;

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "cachestat",
                                    CONFIG_BOOLEAN_NO);

    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_CACHESTAT_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "sync",
                                    CONFIG_BOOLEAN_YES);

    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SYNC_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "dcstat",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_DCSTAT_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "swap",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SWAP_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "vfs",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_VFS_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "filesystem",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_FILESYSTEM_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "disk",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_DISK_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "mount",
                                    CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_MOUNT_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "fd",
                                    CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_FD_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "hardirq",
                                    CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_HARDIRQ_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "softirq",
                                    CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SOFTIRQ_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "oomkill",
                                    CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_OOMKILL_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "shm",
                                    CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SHM_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "mdflush",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_MDFLUSH_IDX, *disable_apps, *disable_cgroups);
        started++;
    }

    if (!started){
        ebpf_enable_all_charts(*disable_apps, *disable_cgroups);
        // Read network viewer section
        // This is kept here to keep backward compatibility
        parse_network_viewer_section(&collector_config);
        parse_service_name_section(&collector_config);
    }
}

/**
 * Load collector config
 *
 * @param path             the path where the file ebpf.conf is stored.
 * @param disable_apps     variable to store the information about apps plugin status.
 * @param disable_cgroups  variable to store the information about cgroups plugin status.
 * @param update_every value to overwrite the update frequency set by the server.
 *
 * @return 0 on success and -1 otherwise.
 */
static int load_collector_config(char *path, int *disable_apps, int *disable_cgroups, int update_every)
{
    char lpath[4096];

    snprintf(lpath, 4095, "%s/%s", path, NETDATA_EBPF_CONFIG_FILE);
    if (!appconfig_load(&collector_config, lpath, 0, NULL)) {
        snprintf(lpath, 4095, "%s/%s", path, NETDATA_EBPF_OLD_CONFIG_FILE);
        if (!appconfig_load(&collector_config, lpath, 0, NULL)) {
            return -1;
        }
    }

    read_collector_values(disable_apps, disable_cgroups, update_every);

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
    if (ebpf_nprocs > NETDATA_MAX_PROCESSOR) {
        ebpf_nprocs = NETDATA_MAX_PROCESSOR;
    }

    isrh = get_redhat_release();
    pid_max = get_system_pid_max();
    running_on_kernel = ebpf_get_kernel_version();
}

/**
 * Load collector config
 *
 * @param lmode  the mode that will be used for them.
 */
static inline void ebpf_load_thread_config()
{
    int i;
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        ebpf_update_module(&ebpf_modules[i]);
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
    int disable_apps = 0;
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

    if (load_collector_config(ebpf_user_config_dir, &disable_apps, &disable_cgroups, freq)) {
        info(
            "Does not have a configuration file inside `%s/ebpf.d.conf. It will try to load stock file.",
            ebpf_user_config_dir);
        if (load_collector_config(ebpf_stock_config_dir, &disable_apps, &disable_cgroups, freq)) {
            info("Does not have a stock file. It is starting with default options.");
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
                info("EBPF enabling \"PROCESS\" charts, because it was started with the option \"[-]-process\".");
#endif
                break;
            }
            case EBPF_MODULE_SOCKET_IDX: {
                select_threads |= 1<<EBPF_MODULE_SOCKET_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"NET\" charts, because it was started with the option \"[-]-net\".");
#endif
                break;
            }
            case EBPF_MODULE_CACHESTAT_IDX: {
                select_threads |= 1<<EBPF_MODULE_CACHESTAT_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"CACHESTAT\" charts, because it was started with the option \"[-]-cachestat\".");
#endif
                break;
            }
            case EBPF_MODULE_SYNC_IDX: {
                select_threads |= 1<<EBPF_MODULE_SYNC_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"SYNC\" chart, because it was started with the option \"[-]-sync\".");
#endif
                break;
            }
            case EBPF_MODULE_DCSTAT_IDX: {
                select_threads |= 1<<EBPF_MODULE_DCSTAT_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"DCSTAT\" charts, because it was started with the option \"[-]-dcstat\".");
#endif
                break;
            }
            case EBPF_MODULE_SWAP_IDX: {
                select_threads |= 1<<EBPF_MODULE_SWAP_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"SWAP\" chart, because it was started with the option \"[-]-swap\".");
#endif
                break;
            }
            case EBPF_MODULE_VFS_IDX: {
                select_threads |= 1<<EBPF_MODULE_VFS_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"VFS\" chart, because it was started with the option \"[-]-vfs\".");
#endif
                break;
            }
            case EBPF_MODULE_FILESYSTEM_IDX: {
                select_threads |= 1<<EBPF_MODULE_FILESYSTEM_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"FILESYSTEM\" chart, because it was started with the option \"[-]-filesystem\".");
#endif
                break;
            }
            case EBPF_MODULE_DISK_IDX: {
                select_threads |= 1<<EBPF_MODULE_DISK_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"DISK\" chart, because it was started with the option \"[-]-disk\".");
#endif
                break;
            }
            case EBPF_MODULE_MOUNT_IDX: {
                select_threads |= 1<<EBPF_MODULE_MOUNT_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"MOUNT\" chart, because it was started with the option \"[-]-mount\".");
#endif
                break;
            }
            case EBPF_MODULE_FD_IDX: {
                select_threads |= 1<<EBPF_MODULE_FD_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"FILEDESCRIPTOR\" chart, because it was started with the option \"[-]-filedescriptor\".");
#endif
                break;
            }
            case EBPF_MODULE_HARDIRQ_IDX: {
                select_threads |= 1<<EBPF_MODULE_HARDIRQ_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"HARDIRQ\" chart, because it was started with the option \"[-]-hardirq\".");
#endif
                break;
            }
            case EBPF_MODULE_SOFTIRQ_IDX: {
                select_threads |= 1<<EBPF_MODULE_SOFTIRQ_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"SOFTIRQ\" chart, because it was started with the option \"[-]-softirq\".");
#endif
                break;
            }
            case EBPF_MODULE_OOMKILL_IDX: {
                select_threads |= 1<<EBPF_MODULE_OOMKILL_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"OOMKILL\" chart, because it was started with the option \"[-]-oomkill\".");
#endif
                break;
            }
            case EBPF_MODULE_SHM_IDX: {
                select_threads |= 1<<EBPF_MODULE_SHM_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"SHM\" chart, because it was started with the option \"[-]-shm\".");
#endif
                break;
            }
            case EBPF_MODULE_MDFLUSH_IDX: {
                select_threads |= 1<<EBPF_MODULE_MDFLUSH_IDX;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"MDFLUSH\" chart, because it was started with the option \"[-]-mdflush\".");
#endif
                break;
            }
            case EBPF_OPTION_ALL_CHARTS: {
                disable_apps = 0;
                disable_cgroups = 0;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF running with all chart groups, because it was started with the option \"[-]-all\".");
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
                disable_apps = 1;
                disable_cgroups = 1;
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF running with global chart group, because it was started with the option  \"[-]-global\".");
#endif
                break;
            }
            case EBPF_OPTION_RETURN_MODE: {
                ebpf_set_thread_mode(MODE_RETURN);
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF running in \"RETURN\" mode, because it was started with the option \"[-]-return\".");
#endif
                break;
            }
            case EBPF_OPTION_LEGACY: {
                ebpf_set_load_mode(EBPF_LOAD_LEGACY);
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF running with \"LEGACY\" code, because it was started with the option \"[-]-legacy\".");
#endif
                break;
            }
            case EBPF_OPTION_CORE: {
                ebpf_set_load_mode(EBPF_LOAD_CORE);
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF running with \"CO-RE\" code, because it was started with the option \"[-]-core\".");
#endif
                break;
            }
            default: {
                break;
            }
        }
    }

    if (disable_apps || disable_cgroups) {
        if (disable_apps)
            ebpf_disable_apps();

        if (disable_cgroups)
            ebpf_disable_cgroups();
    }

    if (select_threads) {
        disable_all_global_charts();
        uint64_t idx;
        for (idx = 0; idx < EBPF_OPTION_ALL_CHARTS; idx++) {
            if (select_threads & 1<<idx)
                ebpf_enable_specific_chart(&ebpf_modules[idx], disable_apps, disable_cgroups);
        }
    }

    // Load apps_groups.conf
    if (ebpf_read_apps_groups_conf(
            &apps_groups_default_target, &apps_groups_root_target, ebpf_user_config_dir, "groups")) {
        info("Cannot read process groups configuration file '%s/apps_groups.conf'. Will try '%s/apps_groups.conf'",
             ebpf_user_config_dir, ebpf_stock_config_dir);
        if (ebpf_read_apps_groups_conf(
                &apps_groups_default_target, &apps_groups_root_target, ebpf_stock_config_dir, "groups")) {
            error("Cannot read process groups '%s/apps_groups.conf'. There are no internal defaults. Failing.",
                  ebpf_stock_config_dir);
            thread_finished++;
            ebpf_exit(1);
        }
    } else
        info("Loaded config file '%s/apps_groups.conf'", ebpf_user_config_dir);
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
        error("Cannot open %s", filename);
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
        old_pid = (pid_t)str2uint32_t(buffer);
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
 * Entry point
 *
 * @param argc the number of arguments
 * @param argv the pointer to the arguments
 *
 * @return it returns 0 on success and another integer otherwise
 */
int main(int argc, char **argv)
{
    clocks_init();

    set_global_variables();
    ebpf_parse_args(argc, argv);
    ebpf_manage_pid(getpid());

    if (!has_condition_to_run(running_on_kernel)) {
        error("The current collector cannot run on this kernel.");
        return 2;
    }

    if (!am_i_running_as_root()) {
        error(
            "ebpf.plugin should either run as root (now running with uid %u, euid %u) or have special capabilities..",
            (unsigned int)getuid(), (unsigned int)geteuid());
        return 3;
    }

    // set name
    program_name = "ebpf.plugin";

    // disable syslog
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;

    struct rlimit r = { RLIM_INFINITY, RLIM_INFINITY };
    if (setrlimit(RLIMIT_MEMLOCK, &r)) {
        error("Setrlimit(RLIMIT_MEMLOCK)");
        return 4;
    }

    signal(SIGINT, ebpf_exit);
    signal(SIGTERM, ebpf_exit);
    signal(SIGPIPE, ebpf_exit);

    if (ebpf_start_pthread_variables()) {
        thread_finished++;
        error("Cannot start mutex to control overall charts.");
        ebpf_exit(5);
    }

    ebpf_allocate_common_vectors();

#ifdef LIBBPF_MAJOR_VERSION
    libbpf_set_strict_mode(LIBBPF_STRICT_ALL);
#endif

    read_local_addresses();
    read_local_ports("/proc/net/tcp", IPPROTO_TCP);
    read_local_ports("/proc/net/tcp6", IPPROTO_TCP);
    read_local_ports("/proc/net/udp", IPPROTO_UDP);
    read_local_ports("/proc/net/udp6", IPPROTO_UDP);

    struct netdata_static_thread ebpf_threads[] = {
        {"EBPF PROCESS", NULL, NULL, 1,
          NULL, NULL, ebpf_modules[EBPF_MODULE_PROCESS_IDX].start_routine},
        {"EBPF SOCKET" , NULL, NULL, 1,
          NULL, NULL, ebpf_modules[EBPF_MODULE_SOCKET_IDX].start_routine},
        {"EBPF CACHESTAT" , NULL, NULL, 1,
            NULL, NULL, ebpf_modules[EBPF_MODULE_CACHESTAT_IDX].start_routine},
        {"EBPF SYNC" , NULL, NULL, 1,
            NULL, NULL, ebpf_modules[EBPF_MODULE_SYNC_IDX].start_routine},
        {"EBPF DCSTAT" , NULL, NULL, 1,
            NULL, NULL, ebpf_modules[EBPF_MODULE_DCSTAT_IDX].start_routine},
        {"EBPF SWAP" , NULL, NULL, 1,
            NULL, NULL, ebpf_modules[EBPF_MODULE_SWAP_IDX].start_routine},
        {"EBPF VFS" , NULL, NULL, 1,
            NULL, NULL, ebpf_modules[EBPF_MODULE_VFS_IDX].start_routine},
        {"EBPF FILESYSTEM" , NULL, NULL, 1,
            NULL, NULL, ebpf_modules[EBPF_MODULE_FILESYSTEM_IDX].start_routine},
        {"EBPF DISK" , NULL, NULL, 1,
            NULL, NULL, ebpf_modules[EBPF_MODULE_DISK_IDX].start_routine},
        {"EBPF MOUNT" , NULL, NULL, 1,
            NULL, NULL, ebpf_modules[EBPF_MODULE_MOUNT_IDX].start_routine},
        {"EBPF FD" , NULL, NULL, 1,
             NULL, NULL, ebpf_modules[EBPF_MODULE_FD_IDX].start_routine},
        {"EBPF HARDIRQ" , NULL, NULL, 1,
            NULL, NULL, ebpf_modules[EBPF_MODULE_HARDIRQ_IDX].start_routine},
        {"EBPF SOFTIRQ" , NULL, NULL, 1,
            NULL, NULL, ebpf_modules[EBPF_MODULE_SOFTIRQ_IDX].start_routine},
        {"EBPF OOMKILL" , NULL, NULL, 1,
            NULL, NULL, ebpf_modules[EBPF_MODULE_OOMKILL_IDX].start_routine},
        {"EBPF SHM" , NULL, NULL, 1,
            NULL, NULL, ebpf_modules[EBPF_MODULE_SHM_IDX].start_routine},
        {"EBPF MDFLUSH" , NULL, NULL, 1,
            NULL, NULL, ebpf_modules[EBPF_MODULE_MDFLUSH_IDX].start_routine},
        {NULL          , NULL, NULL, 0,
          NULL, NULL, NULL}
    };

    //clean_loaded_events();

    int i;
    for (i = 0; ebpf_threads[i].name != NULL; i++) {
        struct netdata_static_thread *st = &ebpf_threads[i];
        st->thread = mallocz(sizeof(netdata_thread_t));

        ebpf_module_t *em = &ebpf_modules[i];
        em->thread_id = i;
        netdata_thread_create(st->thread, st->name, NETDATA_THREAD_OPTION_JOINABLE, st->start_routine, em);
    }

    for (i = 0; ebpf_threads[i].name != NULL; i++) {
        struct netdata_static_thread *st = &ebpf_threads[i];
        netdata_thread_join(*st->thread, NULL);
    }

    thread_finished++;
    ebpf_exit(0);

    return 0;
}
