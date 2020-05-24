// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/time.h>
#include <sys/resource.h>

#include "ebpf.h"

// callback required by eval()
int health_variable_lookup(const char *variable, uint32_t hash, struct rrdcalc *rc, calculated_number *result) {
    (void)variable;
    (void)hash;
    (void)rc;
    (void)result;
    return 0;
};

void send_statistics( const char *action, const char *action_result, const char *action_data) {
    (void) action;
    (void) action_result;
    (void) action_data;
    return;
}

// callbacks required by popen()
void signals_block(void) {};
void signals_unblock(void) {};
void signals_reset(void) {};

// required by get_system_cpus()
char *netdata_configured_host_prefix = "";

// callback required by fatal()
void netdata_cleanup_and_exit(int ret) {
    exit(ret);
}

// ----------------------------------------------------------------------
//Netdata eBPF library
void *libnetdata = NULL;
int (*load_bpf_file)(char *, int) = NULL;
int (*set_bpf_perf_event)(int, int) = NULL;
int (*perf_event_unmap)(struct perf_event_mmap_page *, size_t);
int (*perf_event_mmap_header)(int, struct perf_event_mmap_page **, int);
void (*netdata_perf_loop_multi)(int *, struct perf_event_mmap_page **, int, int *, int (*nsb)(void *, int), int);
int *map_fd = NULL;

//Perf event variables
static int pmu_fd[NETDATA_MAX_PROCESSOR];
static struct perf_event_mmap_page *headers[NETDATA_MAX_PROCESSOR];
int page_cnt = 8;

char *ebpf_plugin_dir = PLUGINS_DIR;
static char *user_config_dir = CONFIG_DIR;
static char *stock_config_dir = LIBCONFIG_DIR;
static char *netdata_configured_log_dir = LOG_DIR;

FILE *developer_log = NULL;

//Global vectors
static netdata_syscall_stat_t *aggregated_data = NULL;
static netdata_publish_syscall_t *publish_aggregated = NULL;

static int update_every = 1;
static int thread_finished = 0;
int close_ebpf_plugin = 0;
static netdata_run_mode_t mode = MODE_ENTRY;
struct config collector_config;
int running_on_kernel = 0;
char kernel_string[64];
int ebpf_nprocs;
static int isrh;
netdata_idx_t *hash_values;

pthread_mutex_t lock;

ebpf_module_t ebpf_modules[] = {
    { .thread_name = "process", .enabled = 0, .start_routine = ebpf_process_thread, .update_time = 1, .global_charts = 1, .apps_charts = 1, .mode = MODE_ENTRY },
    { .thread_name = "socket", .enabled = 0, .start_routine = ebpf_socket_thread, .update_time = 1, .global_charts = 1, .apps_charts = 1, .mode = MODE_ENTRY },
    { .thread_name = NULL, .enabled = 0, .start_routine = NULL, .update_time = 1, .global_charts = 0, .apps_charts = 1, .mode = MODE_ENTRY },
};

int event_pid = 0;
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
        { .type = 'r', .name = "__x64_sys_clone" },
        { .type = 0, .name = NULL }
};

void open_developer_log() {
    char filename[FILENAME_MAX+1];
    int tot = sprintf(filename, "%s/%s",  netdata_configured_log_dir, NETDATA_DEVELOPER_LOG_FILE);

    if(tot > 0)
        developer_log = fopen(filename, "a");
}

static void int_exit(int sig)
{
    close_ebpf_plugin = 1;

    //When both threads were not finished case I try to go in front this address, the collector will crash
    if (!thread_finished) {
        return;
    }

    if (libnetdata) {
        dlclose(libnetdata);
        libnetdata = NULL;
    }

    if (developer_log) {
        fclose(developer_log);
        developer_log = NULL;
    }

    if (event_pid) {
        int ret = fork();
        if (ret < 0) //error
            error("[EBPF PROCESS] Cannot fork(), so I won't be able to clean %skprobe_events", NETDATA_DEBUGFS);
        else if (!ret) { //child
            int i;
            for ( i=getdtablesize(); i>=0; --i)
                close(i);

            int fd = open("/dev/null",O_RDWR, 0);
            if (fd != -1) {
                dup2 (fd, STDIN_FILENO);
                dup2 (fd, STDOUT_FILENO);
                dup2 (fd, STDERR_FILENO);
            }

            if (fd > 2)
                close (fd);

            int sid = setsid();
            if(sid >= 0) {
                sleep(1);
                debug(D_EXIT, "Wait for father %d die", event_pid);
                clean_kprobe_events(developer_log, event_pid, process_probes);
            } else {
                error("Cannot become session id leader, so I won't try to clean kprobe_events.\n");
            }
        } else { //parent
            exit(0);
        }

        if (developer_log) {
            fclose(developer_log);
            developer_log = NULL;
        }
    }

    exit(sig);
}

static int netdata_store_bpf(void *data, int size) {
    (void)size;

    if (close_ebpf_plugin)
        return 0;

    netdata_error_report_t *e = data;
    fprintf(developer_log
            ,"%llu %s %u: %s, %d\n"
            , now_realtime_usec() ,e->comm, e->pid, dimension_names[e->type], e->err);
    fflush(developer_log);

    return -2; //LIBBPF_PERF_EVENT_CONT;
}

void *process_log(void *ptr)
{
    (void) ptr;

    if (mode == MODE_DEVMODE && debug_log) {
        netdata_perf_loop_multi(pmu_fd, headers, ebpf_nprocs, &close_ebpf_plugin, netdata_store_bpf, page_cnt);
    }

    return NULL;
}

void ebpf_global_labels(netdata_syscall_stat_t *is, netdata_publish_syscall_t *pio, char **dim, char **name, int end) {
    int i;

    netdata_syscall_stat_t *prev = NULL;
    netdata_publish_syscall_t *publish_prev = NULL;
    for (i = 0; i < end; i++) {
        if(prev) {
            prev->next = &is[i];
        }
        prev = &is[i];

        pio[i].dimension = dim[i];
        pio[i].name = name[i];
        if(publish_prev) {
            publish_prev->next = &pio[i];
        }
        publish_prev = &pio[i];
    }
}

static void build_complete_path(char *out, size_t length,char *path, char *filename) {
    if(path){
        snprintf(out, length, "%s/%s", path, filename);
    } else {
        snprintf(out, length, "%s", filename);
    }
}

static inline void ebpf_set_thread_mode(netdata_run_mode_t lmode) {
    int i ;
    for (i = 0 ; ebpf_modules[i].thread_name ; i++ ) {
        ebpf_modules[i].mode = lmode;
    }
}

/**
 * Enable specific charts selected by user.
 *
 * @param em the structure that will be changed
 * @param disable_apps the status about the apps charts.
 */
static inline void ebpf_enable_specific_chart(struct  ebpf_module *em, int disable_apps) {
    em->enabled = 1;
    if (!disable_apps) {
        em->apps_charts = 1;
    }
    em->global_charts = 1;
}

/**
 * Enable all charts
 *
 * @param apps what is the current status of apps
 */
static inline void ebpf_enable_all_charts(int apps) {
    int i ;
    for (i = 0 ; ebpf_modules[i].thread_name ; i++ ) {
        ebpf_enable_specific_chart(&ebpf_modules[i], apps);
    }
}

/**
 * Enable the specified chart group
 *
 * @param enable         enable (1) or disable (0) chart
 * @param disable_apps   was the apps disabled?
 */
static inline void ebpf_enable_chart(int enable, int disable_apps) {
    int i ;
    for (i = 0 ; ebpf_modules[i].thread_name ; i++ ) {
        if (i == enable) {
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
static inline void ebpf_disable_apps() {
    int i ;
    for (i = 0 ;ebpf_modules[i].thread_name ; i++ ) {
        ebpf_modules[i].apps_charts = 0;
    }
}

/**
 * Print help on standard error for user knows how to use the collector.
 */
void ebpf_print_help() {
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
            "\n"
            , VERSION
            , (year >= 116)?year + 1900: 2020
    );
}

/*****************************************************************
 *
 *  AUXILIAR FUNCTIONS USED DURING INITIALIZATION
 *
 *****************************************************************/

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
    int c;
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
        if(n > 0) {
            freq = n;
        }
    }

    while (1) {
        c = getopt_long(argc, argv, "hvganpr",long_options, &option_index);
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
                info("EBPF running with global chart group, because it was started with the option \"--global\" or \"-g\".");
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
                ebpf_enable_chart(1, disable_apps);
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"NET\" charts, because it was started with the option \"--net\" or \"-n\".");
#endif
                break;
            }
            case 'p': {
                enabled = 1;
                ebpf_enable_chart(0, disable_apps);
#ifdef NETDATA_INTERNAL_CHECKS
                info("EBPF enabling \"PROCESS\" charts, because it was started with the option \"--process\" or \"-p\".");
#endif
                break;
            }
            case 'r': {
                mode = MODE_RETURN;
                ebpf_set_thread_mode(mode);
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

    if (!enabled) {
        ebpf_enable_all_charts(disable_apps);
#ifdef NETDATA_INTERNAL_CHECKS
        info("EBPF running with all charts, because neither \"-n\" or \"-p\" was given.");
#endif
    }
}

/**
 * Fill the ebpf_functions structure with default values
 *
 * @param ef the pointer to set default values
 */
void fill_ebpf_functions(ebpf_functions_t *ef) {
    memset(ef, 0, sizeof(ebpf_functions_t));
    ef->running_on_kernel = running_on_kernel;
    ef->isrh = isrh;
}

/**
 * Define how to load the ebpf programs
 *
 * @param ptr the option given by users
 */
static inline void how_to_load(char *ptr) {
    if (!strcasecmp(ptr, "return"))
        mode = MODE_RETURN;
    else
        change_collector_event();
}

/**
 * Set collector values
 */
static void set_collector_values() {
    struct section *sec = collector_config.first_section;
    int disable_apps = 0;
    while(sec) {
        if(!strcasecmp(sec->name, "global")) {
            struct config_option *values = sec->values;
            while(values) {
                if(!strcasecmp(values->name, "load"))
                    how_to_load(values->value);
                if(!strcasecmp(values->name, "disable apps"))
                    if (!strcasecmp(values->value, "yes")) {
                        ebpf_disable_apps();
                        disable_apps = 1;
                    }
                if(!strcasecmp(values->name, "disable socket"))
                    if (!strcasecmp(values->value, "yes"))
                        ebpf_enable_chart(1, disable_apps);

                values = values->next;
            }
        }
        sec = sec->next;
    }
}

/**
 * Load collector config
 *
 * @param path the path where the file ebpf.conf is stored.
 *
 * @return 0 on success and -1 otherwise.
 */
static int load_collector_config(char *path) {
    char lpath[4096];

    snprintf(lpath, 4095, "%s/%s", path, "ebpf.conf" );

    if (!appconfig_load(&collector_config, lpath, 0, NULL))
        return -1;

    set_collector_values();

    return 0;
}

/**
 * Set global variables reading environment variables
 */
void set_global_variables() {
    //Get environment variables
    ebpf_plugin_dir = getenv("NETDATA_PLUGINS_DIR");
    if(!ebpf_plugin_dir)
        ebpf_plugin_dir = PLUGINS_DIR;

    user_config_dir = getenv("NETDATA_USER_CONFIG_DIR");
    if(!user_config_dir)
        user_config_dir = CONFIG_DIR;

    stock_config_dir = getenv("NETDATA_STOCK_CONFIG_DIR");
    if(!stock_config_dir)
        stock_config_dir = LIBCONFIG_DIR;

    netdata_configured_log_dir = getenv("NETDATA_LOG_DIR");
    if(!netdata_configured_log_dir)
        netdata_configured_log_dir = LOG_DIR;

    page_cnt *= (int)sysconf(_SC_NPROCESSORS_ONLN);

    ebpf_nprocs = (int)sysconf(_SC_NPROCESSORS_ONLN);
    if (ebpf_nprocs > NETDATA_MAX_PROCESSOR) {
        ebpf_nprocs = NETDATA_MAX_PROCESSOR;
    }

    isrh = get_redhat_release();
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
    parse_args(argc, argv);

    running_on_kernel =  get_kernel_version(kernel_string, 63);
    if(!has_condition_to_run(running_on_kernel)) {
        error("[EBPF PROCESS] The current collector cannot run on this kernel.");
        return 1;
    }

    //set name
    program_name = "ebpf.plugin";

    //disable syslog
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;

    struct rlimit r = {RLIM_INFINITY, RLIM_INFINITY};
    if (setrlimit(RLIMIT_MEMLOCK, &r)) {
        error("[EBPF PROCESS] setrlimit(RLIMIT_MEMLOCK)");
        return 2;
    }

    set_global_variables();

    if (load_collector_config(user_config_dir)) {
        info("[EBPF PROCESS] does not have a configuration file. It is starting with default options.");
    }

    signal(SIGINT, int_exit);
    signal(SIGTERM, int_exit);

    if (pthread_mutex_init(&lock, NULL)) {
        thread_finished++;
        error("[EBPF PROCESS] Cannot start the mutex.");
        int_exit(7);
    }

    pthread_attr_t attr;
    pthread_attr_init(&attr);
    pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_JOINABLE);
    pthread_t thread[NETDATA_EBPF_PROCESS_THREADS];

    int i;
    int end = NETDATA_EBPF_PROCESS_THREADS;

    void * (*function_pointer[])(void *) = { ebpf_process_thread, ebpf_socket_thread };

    for ( i = 0; i < end ; i++ ) {
        ebpf_module_t *em = &ebpf_modules[i];
        em->thread_id = i;
        if ( ( pthread_create(&thread[i], &attr, function_pointer[i], (void *) em) ) ) {
            error("[EBPF_PROCESS] Cannot create threads.");
            thread_finished++;
            int_exit(8);
        }
    }

    for ( i = 0; i < end ; i++ ) {
        if ( (pthread_join(thread[i], NULL) ) ) {
            error("[EBPF_PROCESS] Cannot join threads.");
            thread_finished++;
            int_exit(9);
        }
    }

    thread_finished++;
    int_exit(0);

    return 0;
}
