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
char *ebpf_user_config_dir = CONFIG_DIR;
char *ebpf_stock_config_dir = LIBCONFIG_DIR;
static char *ebpf_configured_log_dir = LOG_DIR;

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

pthread_mutex_t lock;
pthread_mutex_t collect_data_mutex;
pthread_cond_t collect_data_cond_var;

netdata_ebpf_events_t process_probes[] = {
    { .type = 'r', .name = "vfs_write" },
    { .type = 'r', .name = "vfs_writev" },
    { .type = 'r', .name = "vfs_read" },
    { .type = 'r', .name = "vfs_readv" },
    { .type = 'r', .name = "do_sys_open" },
    { .type = 'r', .name = "vfs_unlink" },
    { .type = 'p', .name = "do_exit" },
    { .type = 'p', .name = "release_task" },
    { .type = 'r', .name = "_do_fork" },
    { .type = 'r', .name = "__close_fd" },
    { .type = 'p', .name = "try_to_wake_up" },
    { .type = 'r', .name = "__x64_sys_clone" },
    { .type = 0, .name = NULL }
};

netdata_ebpf_events_t socket_probes[] = {
    { .type = 'p', .name = "tcp_cleanup_rbuf" },
    { .type = 'p', .name = "tcp_close" },
    { .type = 'p', .name = "udp_recvmsg" },
    { .type = 'r', .name = "udp_recvmsg" },
    { .type = 'r', .name = "udp_sendmsg" },
    { .type = 'p', .name = "do_exit" },
    { .type = 'p', .name = "tcp_sendmsg" },
    { .type = 'r', .name = "tcp_sendmsg" },
    { .type = 0, .name = NULL }
};

ebpf_module_t ebpf_modules[] = {
    { .thread_name = "process", .config_name = "process", .enabled = 0, .start_routine = ebpf_process_thread,
      .update_time = 1, .global_charts = 1, .apps_charts = 1, .mode = MODE_ENTRY, .probes = process_probes,
      .optional = 0 },
    { .thread_name = "socket", .config_name = "network viewer", .enabled = 0, .start_routine = ebpf_socket_thread,
      .update_time = 1, .global_charts = 1, .apps_charts = 1, .mode = MODE_ENTRY, .probes = socket_probes,
      .optional = 0  },
    { .thread_name = NULL, .enabled = 0, .start_routine = NULL, .update_time = 1,
      .global_charts = 0, .apps_charts = 1, .mode = MODE_ENTRY, .probes = NULL,
      .optional = 0 },
};

// Link with apps.plugin
pid_t *pid_index;
ebpf_process_stat_t *global_process_stat = NULL;

//Network viewer
ebpf_network_viewer_options_t network_viewer_opt = { .max_dim = NETDATA_NV_CAP_VALUE, .hostname_resolution_enabled = 0,
                                                     .service_resolution_enabled = 0, .excluded_port = NULL,
                                                     .included_port = NULL, .names = NULL, .ipv4_local_ip = NULL,
                                                     .ipv6_local_ip = NULL };

/*****************************************************************
 *
 *  FUNCTIONS USED TO CLEAN MEMORY AND OPERATE SYSTEM FILES
 *
 *****************************************************************/

/**
 * Clean port Structure
 *
 * Clean the allocated list.
 *
 * @param clean the list that will be cleaned
 */
void clean_port_structure(ebpf_network_viewer_port_list_t **clean)
{
    ebpf_network_viewer_port_list_t *move = *clean;
    while (move) {
        ebpf_network_viewer_port_list_t *next = move->next;
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
static void clean_ip_structure(ebpf_network_viewer_ip_list_t **clean)
{
    ebpf_network_viewer_ip_list_t *move = *clean;
    while (move) {
        ebpf_network_viewer_ip_list_t *next = move->next;
        freez(move);

        move = next;
    }
    *clean = NULL;
}

static void change_events()
{
    if (ebpf_modules[0].mode == MODE_ENTRY)
        change_process_event();

    if (ebpf_modules[1].mode == MODE_ENTRY)
        change_socket_event();
}

/**
 * Clean Loaded Events
 *
 * This function cleans the events previous loaded on Linux.
 */
void clean_loaded_events()
{
    int event_pid;
    for (event_pid = 0; ebpf_modules[event_pid].probes; event_pid++)
        clean_kprobe_events(NULL, (int)ebpf_modules[event_pid].thread_id, ebpf_modules[event_pid].probes);
}

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

    clean_apps_groups_target(apps_groups_root_target);

    freez(pid_index);
    freez(global_process_stat);

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
            sleep_usec(200000); // Sleep 200 miliseconds to father dies.
            clean_loaded_events();
        } else {
            error("Cannot become session id leader, so I won't try to clean kprobe_events.\n");
        }
    } else { // parent
        exit(0);
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
 * Call the necessary functions to create a chart.
 *
 * @param family  the chart family
 * @param move    the pointer with the values that will be published
 *
 * @return It returns a variable tha maps the charts that did not have zero values.
 */
void write_io_chart(char *chart, char *family, char *dwrite, char *dread, netdata_publish_vfs_common_t *pvc)
{
    write_begin_chart(family, chart);

    write_chart_dimension(dwrite, (long long)pvc->write);
    write_chart_dimension(dread, (long long)pvc->read);

    write_end_chart();
}

/**
 * Write chart cmd on standard output
 *
 * @param type      the chart type
 * @param id        the chart id
 * @param title     the chart title
 * @param units     the units label
 * @param family    the group name used to attach the chart on dashaboard
 * @param charttype the chart type
 * @param order     the chart order
 */
void ebpf_write_chart_cmd(char *type, char *id, char *title, char *units, char *family, char *charttype, int order)
{
    printf("CHART %s.%s '' '%s' '%s' '%s' '' %s %d %d\n",
           type,
           id,
           title,
           units,
           family,
           charttype,
           order,
           update_every);
}

/**
 * Write the dimension command on standard output
 *
 * @param n the dimension name
 * @param d the dimension information
 */
void ebpf_write_global_dimension(char *n, char *d)
{
    printf("DIMENSION %s %s absolute 1 1\n", n, d);
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
        ebpf_write_global_dimension(move->name, move->dimension);

        move = move->next;
        i++;
    }
}

/**
 *  Call write_chart_cmd to create the charts
 *
 * @param type   the chart type
 * @param id     the chart id
 * @param units   the axis label
 * @param family the group name used to attach the chart on dashaboard
 * @param order  the order number of the specified chart
 * @param ncd    a pointer to a function called to create dimensions
 * @param move   a pointer for a structure that has the dimensions
 * @param end    number of dimensions for the chart created
 */
void ebpf_create_chart(char *type,
                       char *id,
                       char *title,
                       char *units,
                       char *family,
                       int order,
                       void (*ncd)(void *, int),
                       void *move,
                       int end)
{
    ebpf_write_chart_cmd(type, id, title, units, family, "line", order);

    ncd(move, end);
}

/**
 * Create charts on apps submenu
 *
 * @param id   the chart id
 * @param title  the value displayed on vertical axis.
 * @param units  the value displayed on vertical axis.
 * @param family Submenu that the chart will be attached on dashboard.
 * @param order  the chart order
 * @param root   structure used to create the dimensions.
 */
void ebpf_create_charts_on_apps(char *id, char *title, char *units, char *family, int order, struct target *root)
{
    struct target *w;
    ebpf_write_chart_cmd(NETDATA_APPS_FAMILY, id, title, units, family, "stacked", order);

    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed))
            fprintf(stdout, "DIMENSION %s '' absolute 1 1\n", w->name);
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
 * @param end  the number of elements in the previous 4 arguments.
 */
void ebpf_global_labels(netdata_syscall_stat_t *is, netdata_publish_syscall_t *pio, char **dim, char **name, int end)
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
            " SECONDS           set the data collection frequency.\n"
            "\n"
            " --help or -h      show this help.\n"
            "\n"
            " --version or -v   show software version.\n"
            "\n"
            " --global or -g    disable charts per application.\n"
            "\n"
            " --all or -a       Enable all chart groups (global and apps), unless -g is also given.\n"
            "\n"
            " --net or -n       Enable network viewer charts.\n"
            "\n"
            " --process or -p   Enable charts related to process run time.\n"
            "\n"
            " --return or -r    Run the collector in return mode.\n"
            "\n",
            VERSION,
            (year >= 116) ? year + 1900 : 2020);
}

/*****************************************************************
 *
 *  AUXILIAR FUNCTIONS USED DURING INITIALIZATION
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
static int is_ip_inside_range(union netdata_ip_t *rfirst, union netdata_ip_t *rlast,
                              union netdata_ip_t *cmpfirst, union netdata_ip_t *cmplast, int family)
{
    if (family == AF_INET) {
        if (ntohl(rfirst->addr32[0]) <= ntohl(cmpfirst->addr32[0]) &&
            ntohl(rlast->addr32[0]) >= ntohl(cmplast->addr32[0]))
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
 */
static inline void fill_ip_list(ebpf_network_viewer_ip_list_t **out, ebpf_network_viewer_ip_list_t *in, char *table)
{
#ifndef NETDATA_INTERNAL_CHECKS
    UNUSED(table);
#endif
    if (likely(*out)) {
        ebpf_network_viewer_ip_list_t *move = *out, *store = *out;
        while (move) {
            if (in->ver == move->ver && is_ip_inside_range(&move->first, &move->last, &in->first, &in->last, in->ver)) {
                info("The range/value (%s) is inside the range/value (%s) already inserted, it will be ignored.",
                     in->value, move->value);
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

#ifdef NETDATA_INTERNAL_CHECKS
    char first[512], last[512];
    if (in->ver == AF_INET) {
        if (inet_ntop(AF_INET, in->first.addr8, first, INET_ADDRSTRLEN) &&
            inet_ntop(AF_INET, in->last.addr8, last, INET_ADDRSTRLEN))
            info("Adding values %s - %s to %s IP list \"%s\" used on network viewer",
                 first, last,
                 (*out == network_viewer_opt.included_ips)?"included":"excluded",
                 table);
    } else {
        if (inet_ntop(AF_INET6, in->first.addr8, first, INET6_ADDRSTRLEN) &&
            inet_ntop(AF_INET6, in->last.addr8, last, INET6_ADDRSTRLEN))
            info("Adding values %s - %s to %s IP list \"%s\" used on network viewer",
                 first, last,
                 (*out == network_viewer_opt.included_ips)?"included":"excluded",
                 table);
    }
#endif
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
 * Start Ptherad Variable
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
    pid_index = callocz((size_t)pid_max, sizeof(pid_t));
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
    if (!strcasecmp(ptr, "return"))
        ebpf_set_thread_mode(MODE_RETURN);
    else if (!strcasecmp(ptr, "entry"))
        ebpf_set_thread_mode(MODE_ENTRY);
    else
        error("the option %s for \"ebpf load mode\" is not a valid option.", ptr);
}

/**
 * Parse disable apps option
 *
 * @param ptr the option given by users
 *
 * @return It returns 1 to disable the charts or 0 otherwise.
 */
static inline int parse_disable_apps(char *ptr)
{
    if (!strcasecmp(ptr, "yes")) {
        ebpf_disable_apps();
        return 1;
    } else if (strcasecmp(ptr, "no") != 0) {
        error("The option %s for \"disable apps\" is not a valid option.", ptr);
    }

    return 0;
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
            if (cmp_first <= first && first <= cmp_last  &&
                cmp_first <= last && last <= cmp_last ) {
                info("The range/value (%u, %u) is inside the range/value (%u, %u) already inserted, it will be ignored.",
                     first, last, cmp_first, cmp_last);
                freez(in->value);
                freez(in);
                return;
            } else if (first <= cmp_first && cmp_first <= last  &&
                       first <= cmp_last && cmp_last <= last) {
                info("The range (%u, %u) is bigger than previous range (%u, %u) already inserted, the previous will be ignored.",
                     first, last, cmp_first, cmp_last);
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
    info("Adding values %s( %u, %u) to %s port list used on network viewer",
         in->value, ntohs(in->first), ntohs(in->last),
         (*out == network_viewer_opt.included_port)?"included":"excluded");
#endif
}

/**
 * Fill port list
 *
 * Fill an allocated port list with the range given
 *
 * @param out a pointer to store the link list
 * @param range the informed range for the user.
 */
static void parse_port_list(void **out, char *range)
{
    int first, last;
    ebpf_network_viewer_port_list_t **list = (ebpf_network_viewer_port_list_t **)out;

    char *copied = strdupz(range);
    if (*range == '*' && *(range+1) == '\0') {
        first = 1;
        last = 65535;

        clean_port_structure(list);
        goto fillenvpl;
    }

    char *end = range;
    //Move while I cannot find a separator
    while (*end && *end != ':' && *end != '-') end++;

    //It has a range
    if (likely(*end)) {
        *end++ = '\0';
        if (*end == '!') {
            info("The exclusion cannot be in the second part of the range, the range %s will be ignored.", copied);
            freez(copied);
            return;
        }
        last = str2i((const char *)end);
    } else {
        last = 0;
    }

    first = str2i((const char *)range);
    if (first < NETDATA_MINIMUM_PORT_VALUE || first > NETDATA_MAXIMUM_PORT_VALUE) {
        info("The first port %d of the range \"%s\" is invalid and it will be ignored!", first, copied);
        freez(copied);
        return;
    }

    if (!last)
        last = first;

    if (last < NETDATA_MINIMUM_PORT_VALUE || last > NETDATA_MAXIMUM_PORT_VALUE) {
        info("The second port %d of the range \"%s\" is invalid and the whole range will be ignored!", last, copied);
        freez(copied);
        return;
    }

    if (first > last) {
        info("The specified order %s is wrong, the smallest value is always the first, it will be ignored!", copied);
        freez(copied);
        return;
    }

    ebpf_network_viewer_port_list_t *w;
fillenvpl:
    w = callocz(1, sizeof(ebpf_network_viewer_port_list_t));
    w->value = copied;
    w->hash = simple_hash(copied);
    w->first = (uint16_t)htons((uint16_t)first);
    w->last = (uint16_t)htons((uint16_t)last);
    w->cmp_first = (uint16_t)first;
    w->cmp_last = (uint16_t)last;

    fill_port_list(list, w);
}

/**
 * Parse Service List
 *
 * @param out a pointer to store the link list
 * @param service the service used to create the structure that will be linked.
 */
static void parse_service_list(void **out, char *service)
{
    ebpf_network_viewer_port_list_t **list = (ebpf_network_viewer_port_list_t **)out;
    struct servent *serv = getservbyname((const char *)service, "tcp");
    if (!serv)
        serv = getservbyname((const char *)service, "udp");

    if (!serv) {
        info("Cannot resolv the service '%s' with protocols TCP and UDP, it will be ignored", service);
        return;
    }

    ebpf_network_viewer_port_list_t *w = callocz(1, sizeof(ebpf_network_viewer_port_list_t));
    w->value = strdupz(service);
    w->hash = simple_hash(service);

    w->first = w->last = (uint16_t)serv->s_port;

    fill_port_list(list, w);
}

/**
 * Netmask
 *
 * Copied from iprange (https://github.com/firehol/iprange/blob/master/iprange.h)
 *
 * @param prefix create the netmask based in the CIDR value.
 *
 * @return
 */
static inline in_addr_t netmask(int prefix) {

    if (prefix == 0)
        return (~((in_addr_t) - 1));
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
static inline in_addr_t broadcast(in_addr_t addr, int prefix)
{
    return (addr | ~netmask(prefix));
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
static inline in_addr_t ipv4_network(in_addr_t addr, int prefix)
{
    return (addr & netmask(prefix));
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
static inline int ip2nl(uint8_t *dst, char *ip, int domain, char *source)
{
    if (inet_pton(domain, ip, dst) <= 0) {
        error("The address specified (%s) is invalid ", source);
        return -1;
    }

    return 0;
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
    uint64_t mask,tmp;
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
        if (prefix > 0) {
            mask = 0xFFFFFFFFFFFFFFFFULL << (64 - prefix);
            tmp |= ~mask;
        }
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
 * Calculate ipv6 first address
 *
 * @param out the address to store the first address.
 * @param in the address used to do the math.
 * @param prefix number of bits used to calculate the address
 */
static void get_ipv6_first_addr(union netdata_ip_t *out, union netdata_ip_t *in, uint64_t prefix)
{
    uint64_t mask,tmp;
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
        if (prefix > 0) {
            mask = 0xFFFFFFFFFFFFFFFFULL << (64 - prefix);
            tmp &= mask;
        }
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
 * Parse IP List
 *
 * Parse IP list and link it.
 *
 * @param out a pointer to store the link list
 * @param ip the value given as parameter
 */
static void parse_ip_list(void **out, char *ip)
{
    ebpf_network_viewer_ip_list_t **list = (ebpf_network_viewer_ip_list_t **)out;

    char *ipdup = strdupz(ip);
    union netdata_ip_t first = { };
    union netdata_ip_t last = { };
    char *is_ipv6;
    if (*ip == '*' && *(ip+1) == '\0') {
        memset(first.addr8, 0, sizeof(first.addr8));
        memset(last.addr8, 0xFF, sizeof(last.addr8));

        is_ipv6 = ip;

        clean_ip_structure(list);
        goto storethisip;
    }

    char *end = ip;
    // Move while I cannot find a separator
    while (*end && *end != '/' && *end != '-') end++;

    // We will use only the classic IPV6 for while, but we could consider the base 85 in a near future
    // https://tools.ietf.org/html/rfc1924
    is_ipv6 = strchr(ip, ':');

    int select;
    if (*end && !is_ipv6) { // IPV4 range
        select = (*end == '/') ? 0 : 1;
        *end++ = '\0';
        if (*end == '!') {
            info("The exclusion cannot be in the second part of the range %s, it will be ignored.", ipdup);
            goto cleanipdup;
        }

        if (!select) { // CIDR
            select = ip2nl(first.addr8, ip, AF_INET, ipdup);
            if (select)
                goto cleanipdup;

            select = (int) str2i(end);
            if (select < NETDATA_MINIMUM_IPV4_CIDR || select > NETDATA_MAXIMUM_IPV4_CIDR) {
                info("The specified CIDR %s is not valid, the IP %s will be ignored.", end, ip);
                goto cleanipdup;
            }

            last.addr32[0] = htonl(broadcast(ntohl(first.addr32[0]), select));
            // This was added to remove
            // https://app.codacy.com/manual/netdata/netdata/pullRequest?prid=5810941&bid=19021977
            UNUSED(last.addr32[0]);

            uint32_t ipv4_test = htonl(ipv4_network(ntohl(first.addr32[0]), select));
            if (first.addr32[0] != ipv4_test) {
                first.addr32[0] = ipv4_test;
                struct in_addr ipv4_convert;
                ipv4_convert.s_addr = ipv4_test;
                char ipv4_msg[INET_ADDRSTRLEN];
                if(inet_ntop(AF_INET, &ipv4_convert, ipv4_msg, INET_ADDRSTRLEN))
                    info("The network value of CIDR %s was updated for %s .", ipdup, ipv4_msg);
            }
        } else { // Range
            select = ip2nl(first.addr8, ip, AF_INET, ipdup);
            if (select)
                goto cleanipdup;

            select = ip2nl(last.addr8, end, AF_INET, ipdup);
            if (select)
                goto cleanipdup;
        }

        if (htonl(first.addr32[0]) > htonl(last.addr32[0])) {
            info("The specified range %s is invalid, the second address is smallest than the first, it will be ignored.",
                 ipdup);
            goto cleanipdup;
        }
    } else if (is_ipv6) { // IPV6
        if (!*end) { // Unique
            select = ip2nl(first.addr8, ip, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;

            memcpy(last.addr8, first.addr8, sizeof(first.addr8));
        } else if (*end == '-') {
            *end++ = 0x00;
            if (*end == '!') {
                info("The exclusion cannot be in the second part of the range %s, it will be ignored.", ipdup);
                goto cleanipdup;
            }

            select = ip2nl(first.addr8, ip, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;

            select = ip2nl(last.addr8, end, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;
        } else { // CIDR
            *end++ = 0x00;
            if (*end == '!') {
                info("The exclusion cannot be in the second part of the range %s, it will be ignored.", ipdup);
                goto cleanipdup;
            }

            select = str2i(end);
            if (select < 0 || select > 128) {
                info("The CIDR %s is not valid, the address %s will be ignored.", end, ip);
                goto cleanipdup;
            }

            uint64_t prefix = (uint64_t)select;
            select = ip2nl(first.addr8, ip, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;

            get_ipv6_last_addr(&last, &first, prefix);

            union netdata_ip_t ipv6_test;
            get_ipv6_first_addr(&ipv6_test, &first, prefix);

            if (memcmp(first.addr8, ipv6_test.addr8, sizeof(union netdata_ip_t)) != 0) {
                memcpy(first.addr8, ipv6_test.addr8, sizeof(union netdata_ip_t));

                struct in6_addr ipv6_convert;
                memcpy(ipv6_convert.s6_addr,  ipv6_test.addr8, sizeof(union netdata_ip_t));

                char ipv6_msg[INET6_ADDRSTRLEN];
                if(inet_ntop(AF_INET6, &ipv6_convert, ipv6_msg, INET6_ADDRSTRLEN))
                    info("The network value of CIDR %s was updated for %s .", ipdup, ipv6_msg);
            }
        }

        if ((be64toh(*(uint64_t *)&first.addr32[2]) > be64toh(*(uint64_t *)&last.addr32[2]) &&
            !memcmp(first.addr32, last.addr32, 2*sizeof(uint32_t))) ||
            (be64toh(*(uint64_t *)&first.addr32) > be64toh(*(uint64_t *)&last.addr32)) ) {
            info("The specified range %s is invalid, the second address is smallest than the first, it will be ignored.",
                 ipdup);
            goto cleanipdup;
        }
    } else { // Unique ip
        select = ip2nl(first.addr8, ip, AF_INET, ipdup);
        if (select)
            goto cleanipdup;

        memcpy(last.addr8, first.addr8, sizeof(first.addr8));
    }

    ebpf_network_viewer_ip_list_t *store;

storethisip:
    store = callocz(1, sizeof(ebpf_network_viewer_ip_list_t));
    store->value = ipdup;
    store->hash = simple_hash(ipdup);
    store->ver = (uint8_t)(!is_ipv6)?AF_INET:AF_INET6;
    memcpy(store->first.addr8, first.addr8, sizeof(first.addr8));
    memcpy(store->last.addr8, last.addr8, sizeof(last.addr8));

    fill_ip_list(list, store, "socket");
    return;

cleanipdup:
    freez(ipdup);
}

/**
 * Parse IP Range
 *
 * Parse the IP ranges given and create Network Viewer IP Structure
 *
 * @param ptr  is a pointer with the text to parse.
 */
static void parse_ips(char *ptr)
{
    // No value
    if (unlikely(!ptr))
        return;

    while (likely(ptr)) {
        // Move forward until next valid character
        while (isspace(*ptr)) ptr++;

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
            parse_ip_list((!neg)?(void **)&network_viewer_opt.included_ips:(void **)&network_viewer_opt.excluded_ips,
                            ptr);
        }

        ptr = end;
    }
}


/**
 * Parse Port Range
 *
 * Parse the port ranges given and create Network Viewer Port Structure
 *
 * @param ptr  is a pointer with the text to parse.
 */
static void parse_ports(char *ptr)
{
    // No value
    if (unlikely(!ptr))
        return;

    while (likely(ptr)) {
        // Move forward until next valid character
        while (isspace(*ptr)) ptr++;

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
            parse_port_list((!neg)?(void **)&network_viewer_opt.included_port:(void **)&network_viewer_opt.excluded_port,
                            ptr);
        } else if (isalpha(*ptr)) { // Parse service
            parse_service_list((!neg)?(void **)&network_viewer_opt.included_port:(void **)&network_viewer_opt.excluded_port,
                               ptr);
        } else if (*ptr == '*') { // All
            parse_port_list((!neg)?(void **)&network_viewer_opt.included_port:(void **)&network_viewer_opt.excluded_port,
                            ptr);
        }

        ptr = end;
    }
}

/**
 * Link hostname
 *
 * @param out is the output link list
 * @param in the hostname to add to list.
 */
static void link_hostname(ebpf_network_viewer_hostname_list_t **out, ebpf_network_viewer_hostname_list_t *in)
{
    if (likely(*out)) {
        ebpf_network_viewer_hostname_list_t *move = *out;
        for (; move->next ; move = move->next ) {
            if (move->hash == in->hash && !strcmp(move->value, in->value)) {
                info("The hostname %s was already inserted, it will be ignored.", in->value);
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
    info("Adding value %s to %s hostname list used on network viewer",
         in->value,
         (*out == network_viewer_opt.included_hostnames)?"included":"excluded");
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
static void link_hostnames(char *parse)
{
    // No value
    if (unlikely(!parse))
        return;

    while (likely(parse)) {
        // Find the first valid value
        while (isspace(*parse)) parse++;

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

        ebpf_network_viewer_hostname_list_t *hostname = callocz(1 , sizeof(ebpf_network_viewer_hostname_list_t));
        hostname->value = strdupz(parse);
        hostname->hash = simple_hash(parse);
        hostname->value_pattern = simple_pattern_create(parse, NULL, SIMPLE_PATTERN_EXACT);

        link_hostname((!neg)?&network_viewer_opt.included_hostnames:&network_viewer_opt.excluded_hostnames,
                      hostname);

        parse = end;
    }
}

/**
 * Read max dimension.
 *
 * Netdata plot two dimensions per connection, so it is necessary to adjust the values.
 */
static void read_max_dimension()
{
    int maxdim ;
    maxdim = (int) appconfig_get_number(&collector_config,
                                        EBPF_NETWORK_VIEWER_SECTION,
                                        "maximum dimensions",
                                        NETDATA_NV_CAP_VALUE);
    if (maxdim < 0) {
        error("'maximum dimensions = %d' must be a positive number, Netdata will change for default value %ld.",
              maxdim, NETDATA_NV_CAP_VALUE);
        maxdim = NETDATA_NV_CAP_VALUE;
    }

    maxdim /= 2;
    if (!maxdim) {
        info("The number of dimensions is too small (%u), we are setting it to minimum 2", network_viewer_opt.max_dim);
        network_viewer_opt.max_dim = 1;
    }

    network_viewer_opt.max_dim = (uint32_t)maxdim;
}

/**
 * Parse network viewer section
 */
static void parse_network_viewer_section()
{
    read_max_dimension();

    network_viewer_opt.hostname_resolution_enabled = appconfig_get_boolean(&collector_config,
                                                                       EBPF_NETWORK_VIEWER_SECTION,
                                                                       "resolve hostnames",
                                                                       0);

    network_viewer_opt.service_resolution_enabled = appconfig_get_boolean(&collector_config,
                                                                           EBPF_NETWORK_VIEWER_SECTION,
                                                                           "resolve service names",
                                                                           0);

    char *value = appconfig_get(&collector_config, EBPF_NETWORK_VIEWER_SECTION,
                                "ports", NULL);
    parse_ports(value);

    if (network_viewer_opt.hostname_resolution_enabled) {
        value = appconfig_get(&collector_config, EBPF_NETWORK_VIEWER_SECTION, "hostnames", NULL);
        link_hostnames(value);
    } else {
        info("Name resolution is disabled, collector will not parser \"hostnames\" list.");
    }

    value = appconfig_get(&collector_config, EBPF_NETWORK_VIEWER_SECTION,
                          "ips", "!127.0.0.1/8 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 fc00::/7");
    parse_ips(value);
}

/**
 * Link dimension name
 *
 * Link user specified names inside a link list.
 *
 * @param port the port number associated to the dimension name.
 * @param hash the calculated hash for the dimension name.
 * @param name the dimension name.
 */
static void link_dimension_name(char *port, uint32_t hash, char *value)
{
    int test = str2i(port);
    if (test < NETDATA_MINIMUM_PORT_VALUE || test > NETDATA_MAXIMUM_PORT_VALUE){
        error("The dimension given (%s = %s) has an invalid value and it will be ignored.", port, value);
        return;
    }

    ebpf_network_viewer_dim_name_t *w;
    w = callocz(1, sizeof(ebpf_network_viewer_dim_name_t));

    w->name = strdupz(value);
    w->hash = hash;

    w->port = (uint16_t) htons(test);

    ebpf_network_viewer_dim_name_t *names = network_viewer_opt.names;
    if (unlikely(!names)) {
        network_viewer_opt.names = w;
    } else {
        for (; names->next; names = names->next) {
            if (names->port == w->port) {
                info("Dupplicated definition for a service, the name %s will be ignored. ", names->name);
                freez(names->name);
                names->name = w->name;
                names->hash = w->hash;
                freez(w);
                return;
            }
        }
        names->next = w;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    info("Adding values %s( %u) to dimension name list used on network viewer", w->name, htons(w->port));
#endif
}

/**
 * Parse service Name section.
 *
 * This function gets the values that will be used to overwrite dimensions.
 */
static void parse_service_name_section()
{
    struct section *co = appconfig_get_section(&collector_config, EBPF_SERVICE_NAME_SECTION);
    if (co) {
        struct config_option *cv;
        for (cv = co->values; cv ; cv = cv->next) {
            link_dimension_name(cv->name, cv->hash, cv->value);
        }
    }

    // Always associated the default port to Netdata
    ebpf_network_viewer_dim_name_t *names = network_viewer_opt.names;
    if (names) {
        uint16_t default_port = htons(19999);
        while (names) {
            if (names->port == default_port)
                return;

            names = names->next;
        }
    }

    char *port_string = getenv("NETDATA_LISTEN_PORT");
    if (port_string)
        link_dimension_name(port_string, simple_hash(port_string), "Netdata");
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
        value = appconfig_get(&collector_config, EBPF_GLOBAL_SECTION, "load", "entry");
    else
        value = appconfig_get(&collector_config, EBPF_GLOBAL_SECTION, "ebpf load mode", "entry");

    how_to_load(value);

    value = appconfig_get(&collector_config, EBPF_GLOBAL_SECTION, "disable apps", "no");
    *disable_apps = parse_disable_apps(value);

    // Read ebpf programs section
    uint32_t enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, ebpf_modules[0].config_name,
                                             1);
    int started = 0;
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_PROCESS_IDX, *disable_apps);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, ebpf_modules[1].config_name, 1);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SOCKET_IDX, *disable_apps);
        // Read network viewer section if network viewer is enabled
        parse_network_viewer_section();
        parse_service_name_section();
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "network connection monitoring",
                                    0);
    ebpf_modules[1].optional = enabled;

    if (!started){
        ebpf_enable_all_charts(*disable_apps);
        // Read network viewer section
        parse_network_viewer_section();
        parse_service_name_section();
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

    snprintf(lpath, 4095, "%s/%s", path, "ebpf.conf");

    if (!appconfig_load(&collector_config, lpath, 0, NULL))
        return -1;

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
        {"help",     no_argument,    0,  'h' },
        {"version",  no_argument,    0,  'v' },
        {"global",   no_argument,    0,  'g' },
        {"all",      no_argument,    0,  'a' },
        {"net",      no_argument,    0,  'n' },
        {"process",  no_argument,    0,  'p' },
        {"return",   no_argument,    0,  'r' },
        {0, 0, 0, 0}
    };

    if (argc > 1) {
        int n = (int)str2l(argv[1]);
        if (n > 0) {
            freq = n;
        }
    }

    while (1) {
        int c = getopt_long(argc, argv, "hvganpr", long_options, &option_index);
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
            "Does not have a configuration file inside `%s/ebpf.conf. It will try to load stock file.",
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

    if (disable_apps)
        return;

    // Load apps_groups.conf
    if (ebpf_read_apps_groups_conf(
            &apps_groups_default_target, &apps_groups_root_target, ebpf_user_config_dir, "groups")) {
        info(
            "Cannot read process groups configuration file '%s/apps_groups.conf'. Will try '%s/apps_groups.conf'",
            ebpf_user_config_dir, ebpf_stock_config_dir);
        if (ebpf_read_apps_groups_conf(
                &apps_groups_default_target, &apps_groups_root_target, ebpf_stock_config_dir, "groups")) {
            error(
                "Cannot read process groups '%s/apps_groups.conf'. There are no internal defaults. Failing.",
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
        {"EBPF PROCESS", NULL, NULL, 1, NULL, NULL, ebpf_modules[0].start_routine},
        {"EBPF SOCKET" , NULL, NULL, 1, NULL, NULL, ebpf_modules[1].start_routine},
        {NULL          , NULL, NULL, 0, NULL, NULL, NULL}
    };

    change_events();
    clean_loaded_events();

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
