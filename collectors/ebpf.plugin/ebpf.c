// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/time.h>
#include <sys/resource.h>

#include "ebpf.h"

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
    return;
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
      .update_time = 1, .global_charts = 1, .apps_charts = 1, .mode = MODE_ENTRY, .probes = process_probes },
    { .thread_name = "socket", .config_name = "network viewer", .enabled = 0, .start_routine = ebpf_socket_thread,
      .update_time = 1, .global_charts = 1, .apps_charts = 1, .mode = MODE_ENTRY, .probes = socket_probes },
    { .thread_name = NULL, .enabled = 0, .start_routine = NULL, .update_time = 1,
      .global_charts = 0, .apps_charts = 1, .mode = MODE_ENTRY, .probes = NULL },
};

// Link with apps.plugin
pid_t *pid_index;
ebpf_process_stat_t *global_process_stat = NULL;

/*****************************************************************
 *
 *  FUNCTIONS USED TO CLEAN MEMORY AND OPERATE SYSTEM FILES
 *
 *****************************************************************/

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
            sleep_usec(200000); //Sleep 200 miliseconds to father dies.
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
    } else if (strcasecmp(ptr, "no")) {
        error("The option %s for \"disable apps\" is not a valid option.", ptr);
    }

    return 0;
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
    uint32_t enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, ebpf_modules[0].config_name, 1);
    int started = 0;
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_PROCESS_IDX, *disable_apps);
        started++;
    }

    enabled = appconfig_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, ebpf_modules[1].config_name, 1);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SOCKET_IDX, *disable_apps);
        started++;
    }

    if (!started)
        ebpf_enable_all_charts(*disable_apps);
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
