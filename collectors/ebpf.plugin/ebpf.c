// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/time.h>
#include <sys/resource.h>
#include <ifaddrs.h>

#include "ebpf.h"
#include "ebpf_socket.h"

/*****************************************************************
 *
 *  FUNCTIONS USED BY NETDATA
 *
 *****************************************************************/

// callback required by eval()
int health_variable_lookup(const char *variable, uint32_t hash, struct rrdcalc *rc, calculated_number *result)
{
    UNUSED(variable);
    UNUSED(hash);
    UNUSED(rc);
    UNUSED(result);
    return 0;
};

void send_statistics(const char *action, const char *action_result, const char *action_data)
{
    UNUSED(action);
    UNUSED(action_result);
    UNUSED(action_data);
}

// callbacks required by popen()
void signals_block(void){};
void signals_unblock(void){};
void signals_reset(void){};

// required by get_system_cpus()
char *netdata_configured_host_prefix = "";

// callback required by fatal()
void netdata_cleanup_and_exit(int ret)
{
    exit(ret);
}

// ----------------------------------------------------------------------
/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

char *ebpf_plugin_dir = PLUGINS_DIR;
static char *ebpf_configured_log_dir = LOG_DIR;

char *ebpf_algorithms[] = {"absolute", "incremental"};
int update_every = 1;
static int thread_finished = 0;
int close_ebpf_plugin = 0;
struct config collector_config = { .first_section = NULL,
                                   .last_section = NULL,
                                   .mutex = NETDATA_MUTEX_INITIALIZER,
                                   .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
                                              .rwlock = AVL_LOCK_INITIALIZER } };

int running_on_kernel = 0;
char kernel_string[64];
int ebpf_nprocs;
static int isrh;
uint32_t finalized_threads = 1;

pthread_mutex_t lock;
pthread_mutex_t collect_data_mutex;
pthread_cond_t collect_data_cond_var;

ebpf_module_t ebpf_modules[] = {
    { .thread_name = "process", .config_name = "process", .enabled = 0, .start_routine = ebpf_process_thread,
      .update_time = 1, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY,
      .optional = 0, .apps_routine = ebpf_process_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &process_config,
      .config_file = NETDATA_PROCESS_CONFIG_FILE},
    { .thread_name = "socket", .config_name = "socket", .enabled = 0, .start_routine = ebpf_socket_thread,
      .update_time = 1, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY,
      .optional = 0, .apps_routine = ebpf_socket_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &socket_config,
      .config_file = NETDATA_NETWORK_CONFIG_FILE},
    { .thread_name = "cachestat", .config_name = "cachestat", .enabled = 0, .start_routine = ebpf_cachestat_thread,
      .update_time = 1, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY,
      .optional = 0, .apps_routine = ebpf_cachestat_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &cachestat_config,
      .config_file = NETDATA_CACHESTAT_CONFIG_FILE},
    { .thread_name = "sync", .config_name = "sync", .enabled = 0, .start_routine = ebpf_sync_thread,
      .update_time = 1, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY,
      .optional = 0, .apps_routine = NULL, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &sync_config,
      .config_file = NETDATA_SYNC_CONFIG_FILE},
    { .thread_name = "dc", .config_name = "dc", .enabled = 0, .start_routine = ebpf_dcstat_thread,
      .update_time = 1, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY,
      .optional = 0, .apps_routine = ebpf_dcstat_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &dcstat_config,
      .config_file = NETDATA_DIRECTORY_DCSTAT_CONFIG_FILE},
    { .thread_name = "swap", .config_name = "swap", .enabled = 0, .start_routine = ebpf_swap_thread,
      .update_time = 1, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY,
      .optional = 0, .apps_routine = ebpf_swap_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &swap_config,
      .config_file = NETDATA_DIRECTORY_SWAP_CONFIG_FILE},
    { .thread_name = "vfs", .config_name = "swap", .enabled = 0, .start_routine = ebpf_vfs_thread,
      .update_time = 1, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY,
      .optional = 0, .apps_routine = ebpf_vfs_create_apps_charts, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &vfs_config,
      .config_file = NETDATA_DIRECTORY_VFS_CONFIG_FILE },
    { .thread_name = "filesystem", .config_name = "filesystem", .enabled = 0, .start_routine = ebpf_filesystem_thread,
      .update_time = 1, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY,
      .optional = 0, .apps_routine = NULL, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &fs_config,
      .config_file = NETDATA_SYNC_CONFIG_FILE},
    { .thread_name = "disk", .config_name = "disk", .enabled = 0, .start_routine = ebpf_disk_thread,
      .update_time = 1, .global_charts = 1, .apps_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY,
      .optional = 0, .apps_routine = NULL, .maps = NULL,
      .pid_map_size = ND_EBPF_DEFAULT_PID_SIZE, .names = NULL, .cfg = &disk_config,
      .config_file = NETDATA_SYNC_CONFIG_FILE},
    { .thread_name = NULL, .enabled = 0, .start_routine = NULL, .update_time = 1,
      .global_charts = 0, .apps_charts = CONFIG_BOOLEAN_NO, .mode = MODE_ENTRY,
      .optional = 0, .apps_routine = NULL, .maps = NULL, .pid_map_size = 0, .names = NULL,
      .cfg = NULL, .config_name = NULL},
};

// Link with apps.plugin
ebpf_process_stat_t *global_process_stat = NULL;

//Network viewer
ebpf_network_viewer_options_t network_viewer_opt;

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
    int ret = printf("SET %s = %lld\n", dim, value);
    UNUSED(ret);
}

/**
 * Call the necessary functions to create a chart.
 *
 * @param name    the chart name
 * @param family  the chart family
 * @param move    the pointer with the values that will be published
 * @param end     the number of values that will be written on standard output
 *
 * @return It returns a variable tha maps the charts that did not have zero values.
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
 * @return It returns a variable tha maps the charts that did not have zero values.
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
 * @param type      chart type
 * @param id        chart id
 * @param title     chart title
 * @param units     units label
 * @param family    group name used to attach the chart on dashboard
 * @param charttype chart type
 * @param context   chart context
 * @param order     chart order
 */
void ebpf_write_chart_cmd(char *type, char *id, char *title, char *units, char *family,
                          char *charttype, char *context, int order)
{
    printf("CHART %s.%s '' '%s' '%s' '%s' '%s' '%s' %d %d\n",
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
 */
void ebpf_write_chart_obsolete(char *type, char *id, char *title, char *units, char *family,
                               char *charttype, char *context, int order)
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
 * @param type      chart type
 * @param id        chart id
 * @param title     chart title
 * @param units     axis label
 * @param family    group name used to attach the chart on dashboard
 * @param context   chart context
 * @param charttype chart type
 * @param order     order number of the specified chart
 * @param ncd       a pointer to a function called to create dimensions
 * @param move      a pointer for a structure that has the dimensions
 * @param end       number of dimensions for the chart created
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
                       int end)
{
    ebpf_write_chart_cmd(type, id, title, units, family, charttype, context, order);

    ncd(move, end);
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
 */
void ebpf_create_charts_on_apps(char *id, char *title, char *units, char *family, char *charttype, int order,
                                char *algorithm, struct target *root)
{
    struct target *w;
    ebpf_write_chart_cmd(NETDATA_APPS_FAMILY, id, title, units, family, charttype, NULL, order);

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
 * @return It returns a variable tha maps the charts that did not have zero values.
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
 * @param em      the structure that will be changed
 * @param enable the status about the apps charts.
 */
static inline void ebpf_enable_specific_chart(struct ebpf_module *em, int enable)
{
    em->enabled = 1;
    if (!enable) {
        em->apps_charts = 1;
    }
    em->global_charts = 1;
}

/**
 * Enable all charts
 *
 * @param apps what is the current status of apps
 */
static inline void ebpf_enable_all_charts(int apps)
{
    int i;
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        ebpf_enable_specific_chart(&ebpf_modules[i], apps);
    }
}

/**
 * Enable the specified chart group
 *
 * @param idx            the index of ebpf_modules that I am enabling
 * @param disable_apps   should I keep apps charts?
 */
static inline void ebpf_enable_chart(int idx, int disable_apps)
{
    int i;
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        if (i == idx) {
            ebpf_enable_specific_chart(&ebpf_modules[i], disable_apps);
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
            " This program is a data collector plugin for netdata.\n"
            "\n"
            " Available command line options:\n"
            "\n"
            " SECONDS             Set the data collection frequency.\n"
            "\n"
            " --help or -h        Show this help.\n"
            "\n"
            " --version or -v     Show software version.\n"
            "\n"
            " --global or -g      Disable charts per application.\n"
            "\n"
            " --all or -a         Enable all chart groups (global and apps), unless -g is also given.\n"
            "\n"
            " --cachestat or -c   Enable charts related to process run time.\n"
            "\n"
            " --dcstat or -d      Enable charts related to directory cache.\n"
            "\n"
            " --disk or -k        Enable charts related to disk monitoring.\n"
            "\n"
            " --filesystem or -i  Enable chart related to filesystem run time.\n"
            "\n"
            " --net or -n         Enable network viewer charts.\n"
            "\n"
            " --process or -p     Enable charts related to process run time.\n"
            "\n"
            " --return or -r      Run the collector in return mode.\n"
            "\n",
            " --sync or -s        Enable chart related to sync run time.\n"
            "\n"
            " --swap or -w        Enable chart related to swap run time.\n"
            "\n"
            " --vfs or -f         Enable chart related to vfs run time.\n"
            "\n"
            VERSION,
            (year >= 116) ? year + 1900 : 2020);
}

/*****************************************************************
 *
 *  AUXILIAR FUNCTIONS USED DURING INITIALIZATION
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
        update_listen_table(htons(port), proto);
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
 * Fill the ebpf_data structure with default values
 *
 * @param ef the pointer to set default values
 */
void fill_ebpf_data(ebpf_data_t *ef)
{
    memset(ef, 0, sizeof(ebpf_data_t));
    ef->kernel_string = kernel_string;
    ef->running_on_kernel = running_on_kernel;
    ef->map_fd = callocz(EBPF_MAX_MAPS, sizeof(int));
    ef->isrh = isrh;
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
 */
static void ebpf_update_interval()
{
    int i;
    int value = (int) appconfig_get_number(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_UPDATE_EVERY, 1);
    for (i = 0; ebpf_modules[i].thread_name; i++) {
        ebpf_modules[i].update_time = value;
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
 * Read collector values
 *
 * @param disable_apps variable to store information related to apps.
 */
static void read_collector_values(int *disable_apps)
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

    ebpf_update_interval();

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

    // Read ebpf programs section
    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION,
                                    ebpf_modules[EBPF_MODULE_PROCESS_IDX].config_name, CONFIG_BOOLEAN_YES);
    int started = 0;
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_PROCESS_IDX, *disable_apps);
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
        ebpf_enable_chart(EBPF_MODULE_SOCKET_IDX, *disable_apps);
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
    ebpf_modules[EBPF_MODULE_SOCKET_IDX].optional = enabled;

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "cachestat",
                                    CONFIG_BOOLEAN_NO);

    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_CACHESTAT_IDX, *disable_apps);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "sync",
                                    CONFIG_BOOLEAN_YES);

    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SYNC_IDX, *disable_apps);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "dcstat",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_DCSTAT_IDX, *disable_apps);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "swap",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SWAP_IDX, *disable_apps);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "vfs",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_VFS_IDX, *disable_apps);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "filesystem",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_FILESYSTEM_IDX, *disable_apps);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "disk",
                                    CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_DISK_IDX, *disable_apps);
        started++;
    }

    if (!started){
        ebpf_enable_all_charts(*disable_apps);
        // Read network viewer section
        // This is kept here to keep backward compatibility
        parse_network_viewer_section(&collector_config);
        parse_service_name_section(&collector_config);
    }
}

/**
 * Load collector config
 *
 * @param path          the path where the file ebpf.conf is stored.
 * @param disable_apps  variable to store the information about apps plugin status.
 *
 * @return 0 on success and -1 otherwise.
 */
static int load_collector_config(char *path, int *disable_apps)
{
    char lpath[4096];

    snprintf(lpath, 4095, "%s/%s", path, NETDATA_EBPF_CONFIG_FILE);
    if (!appconfig_load(&collector_config, lpath, 0, NULL)) {
        snprintf(lpath, 4095, "%s/%s", path, NETDATA_EBPF_OLD_CONFIG_FILE);
        if (!appconfig_load(&collector_config, lpath, 0, NULL)) {
            return -1;
        }
    }

    read_collector_values(disable_apps);

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
}

/**
 * Parse arguments given from user.
 *
 * @param argc the number of arguments
 * @param argv the pointer to the arguments
 */
static void parse_args(int argc, char **argv)
{
    int enabled = 0;
    int disable_apps = 0;
    int freq = 0;
    int option_index = 0;
    static struct option long_options[] = {
        {"help",       no_argument,    0,  'h' },
        {"version",    no_argument,    0,  'v' },
        {"global",     no_argument,    0,  'g' },
        {"all",        no_argument,    0,  'a' },
        {"cachestat",  no_argument,    0,  'c' },
        {"dcstat",     no_argument,    0,  'd' },
        {"disk",       no_argument,    0,  'k' },
        {"filesystem", no_argument,    0,  'i' },
        {"net",        no_argument,    0,  'n' },
        {"process",    no_argument,    0,  'p' },
        {"return",     no_argument,    0,  'r' },
        {"sync",       no_argument,    0,  's' },
        {"swap",       no_argument,    0,  'w' },
        {"vfs",        no_argument,    0,  'f' },
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

    while (1) {
        int c = getopt_long(argc, argv, "hvgacdnprsw", long_options, &option_index);
        if (c == -1)
            break;

        switch (c) {
            case 'h': {
                ebpf_print_help();
                exit(0);
            }
            case 'v': {
                printf("ebpf.plugin %s\n", VERSION);
                exit(0);
            }
            case 'g': {
                disable_apps = 1;
                ebpf_disable_apps();
#ifdef NETDATA_INTERNAL_CHECKS
                info(
                    "EBPF running with global chart group, because it was started with the option \"--global\" or \"-g\".");
#endif
                break;
            }
            case 'a': {
                ebpf_enable_all_charts(disable_apps);
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF running with all chart groups, because it was started with the option \"--all\" or \"-a\".");
#endif
                break;
            }
            case 'c': {
                enabled = 1;
                ebpf_enable_chart(EBPF_MODULE_CACHESTAT_IDX, disable_apps);
#ifdef NETDATA_INTERNAL_CHECKS
                info(
                    "EBPF enabling \"CACHESTAT\" charts, because it was started with the option \"--cachestat\" or \"-c\".");
#endif
                break;
            }
            case 'd': {
                enabled = 1;
                ebpf_enable_chart(EBPF_MODULE_DCSTAT_IDX, disable_apps);
#ifdef NETDATA_INTERNAL_CHECKS
                info(
                    "EBPF enabling \"DCSTAT\" charts, because it was started with the option \"--dcstat\" or \"-d\".");
#endif
                break;
            }
            case 'i': {
                enabled = 1;
                ebpf_enable_chart(EBPF_MODULE_FILESYSTEM_IDX, disable_apps);
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"filesystem\" chart, because it was started with the option \"--filesystem\" or \"-i\".");
#endif
                break;
            }
            case 'k': {
                enabled = 1;
                ebpf_enable_chart(EBPF_MODULE_DISK_IDX, disable_apps);
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"disk\" chart, because it was started with the option \"--disk\" or \"-k\".");
#endif
                break;
            }
            case 'n': {
                enabled = 1;
                ebpf_enable_chart(EBPF_MODULE_SOCKET_IDX, disable_apps);
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"NET\" charts, because it was started with the option \"--net\" or \"-n\".");
#endif
                break;
            }
            case 'p': {
                enabled = 1;
                ebpf_enable_chart(EBPF_MODULE_PROCESS_IDX, disable_apps);
#ifdef NETDATA_INTERNAL_CHECKS
                info(
                    "EBPF enabling \"PROCESS\" charts, because it was started with the option \"--process\" or \"-p\".");
#endif
                break;
            }
            case 'r': {
                ebpf_set_thread_mode(MODE_RETURN);
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF running in \"return\" mode, because it was started with the option \"--return\" or \"-r\".");
#endif
                break;
            }
            case 's': {
                enabled = 1;
                ebpf_enable_chart(EBPF_MODULE_SYNC_IDX, disable_apps);
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"sync\" chart, because it was started with the option \"--sync\" or \"-s\".");
#endif
                break;
            }
            case 'w': {
                enabled = 1;
                ebpf_enable_chart(EBPF_MODULE_SWAP_IDX, disable_apps);
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"swap\" chart, because it was started with the option \"--swap\" or \"-w\".");
#endif
                break;
            }
            case 'f': {
                enabled = 1;
                ebpf_enable_chart(EBPF_MODULE_VFS_IDX, disable_apps);
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"vfs\" chart, because it was started with the option \"--vfs\" or \"-f\".");
#endif
                break;
            }
            default: {
                break;
            }
        }
    }

    if (freq > 0) {
        update_every = freq;
    }

    if (load_collector_config(ebpf_user_config_dir, &disable_apps)) {
        info(
            "Does not have a configuration file inside `%s/ebpf.d.conf. It will try to load stock file.",
            ebpf_user_config_dir);
        if (load_collector_config(ebpf_stock_config_dir, &disable_apps)) {
            info("Does not have a stock file. It is starting with default options.");
        } else {
            enabled = 1;
        }
    } else {
        enabled = 1;
    }

    if (!enabled) {
        ebpf_enable_all_charts(disable_apps);
#ifdef NETDATA_INTERNAL_CHECKS
        info("EBPF running with all charts, because neither \"-n\" or \"-p\" was given.");
#endif
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
 * Manage PID
 *
 * This function kills another instance of eBPF whether it is necessary and update the file content.
 *
 * @param pid that identifies the process
 */
static void ebpf_manage_pid(pid_t pid)
{
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s%s/ebpf.d/ebpf.pid", netdata_configured_host_prefix, ebpf_plugin_dir);

    ebpf_kill_previous_process(filename, pid);
    ebpf_update_pid_file(filename, pid);
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
 * Entry point
 *
 * @param argc the number of arguments
 * @param argv the pointer to the arguments
 *
 * @return it returns 0 on success and another integer otherwise
 */
int main(int argc, char **argv)
{
    set_global_variables();
    parse_args(argc, argv);
    ebpf_load_thread_config();
    ebpf_manage_pid(getpid());

    running_on_kernel = get_kernel_version(kernel_string, 63);
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
