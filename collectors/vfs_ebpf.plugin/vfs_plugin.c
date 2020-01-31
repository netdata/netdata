// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/time.h>
#include <sys/resource.h>

#include "vfs_plugin.h"

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
int (*load_bpf_file)(char *) = NULL;
int *map_fd = NULL;

//Libbpf (It is necessary to have at least kernel 4.10)
int (*bpf_map_lookup_elem)(int, const void *, void *);

static char *plugin_dir = PLUGINS_DIR;
static char *user_config_dir = CONFIG_DIR;
static char *stock_config_dir = LIBCONFIG_DIR;
static char *netdata_configured_log_dir = LOG_DIR;

FILE *developer_log = NULL;

//Global vectors
netdata_syscall_stat_t *aggregated_data = NULL;
netdata_publish_syscall_t *publish_aggregated = NULL;

static int update_every = 1;
static int thread_finished = 0;
static int close_plugin = 0;
static int kretprobe = 1;
struct config collector_config;

pthread_mutex_t lock;

static void int_exit(int sig)
{
    close_plugin = 1;

    //When both threads were not finished case I try to go in front this address, the collector will crash
    if (!thread_finished) {
        return;
    }

    if (aggregated_data) {
        free(aggregated_data);
    }

    if (publish_aggregated) {
        free(publish_aggregated);
    }

    if (developer_log) {
        fclose(developer_log);
    }

    if (libnetdata) {
        dlclose(libnetdata);
    }

    exit(sig);
}

static inline void netdata_write_chart_cmd(char *family
                                    , char *name
                                    , char *msg
                                    , char *axis
                                    , char *web
                                    , int order)
{
    printf("CHART %s.%s '' '%s' '%s' '%s' '' line %d 1 ''\n"
            , family
            , name
            , msg
            , axis
            , web
            , order);
}

static void netdata_write_global_dimension(char *dim)
{
    printf("DIMENSION %s '' absolute 1 1\n", dim);
}

static void netdata_create_global_dimension(void *ptr, int end)
{
    netdata_publish_syscall_t *move = ptr;

    int i = 0;
    while (move && i < end) {
        netdata_write_global_dimension(move->dimension);

        move = move->next;
        i++;
    }
}
static inline void netdata_create_chart(char *family
                                , char *name
                                , char *msg
                                , char *axis
                                , char *web
                                , int order
                                , void (*ncd)(void *, int)
                                , void *move
                                , int end)
{

    netdata_write_chart_cmd(family, name, msg, axis, web, order);

    ncd(move, end);
}

static void netdata_create_io_chart(char *family, char *name, char *msg, char *axis, char *web, int order) {
    printf("CHART %s.%s '' '%s' '%s' '%s' '' line %d 1 ''\n"
            , family
            , name
            , msg
            , axis
            , web
            , order);

    printf("DIMENSION %s '' absolute 1 1\n", NETDATA_VFS_DIM_OUT_FILE_BYTES );
    printf("DIMENSION %s '' absolute 1 1\n", NETDATA_VFS_DIM_IN_FILE_BYTES );
}

static void netdata_global_charts_create() {
    netdata_create_chart(NETDATA_EBPF_FAMILY
            ,NETDATA_FILE_OPEN_CLOSE_COUNT
            , "Count the total of calls made to the operate system per period to open a file descriptor."
            , "Number of calls"
            , NETDATA_FILE_GROUP
            , 970
            , netdata_create_global_dimension
            , publish_aggregated
            , 2);

    netdata_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_VFS_FILE_CLEAN_COUNT
            , "Count the total of calls made to the operate system per period to delete a file from the operate system."
            , "Number of calls"
            , NETDATA_FILE_GROUP
            , 971
            , netdata_create_global_dimension
            , &publish_aggregated[NETDATA_DEL_START]
            , 1);

    netdata_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_VFS_FILE_IO_COUNT
            , "Count the total of calls made to the operate system per period to write inside a file descriptor."
            , "Number of calls"
            , NETDATA_FILE_GROUP
            , 972
            , netdata_create_global_dimension
            , &publish_aggregated[NETDATA_IN_START_BYTE]
            , 2);

    netdata_create_io_chart(NETDATA_EBPF_FAMILY
            , NETDATA_VFS_IO_FILE_BYTES
            , "Total of bytes read or written with success per period."
            , "bytes/s"
            , NETDATA_FILE_GROUP
            , 974);

    if(kretprobe) {
        netdata_create_chart(NETDATA_EBPF_FAMILY
                , NETDATA_VFS_FILE_ERR_COUNT
                , "Count the total of errors"
                , "Number of calls"
                , NETDATA_FILE_GROUP
                , 975
                , netdata_create_global_dimension
                , publish_aggregated
                , NETDATA_FILE_ERRORS);

    }

    netdata_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_PROCESS_SYSCALL
            , "Count the total of calls made to the operate system per period to start a process."
            , "Number of calls"
            , NETDATA_PROCESS_GROUP
            , 976
            , netdata_create_global_dimension
            , &publish_aggregated[NETDATA_PROCESS_START]
            , 2);

    netdata_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_EXIT_SYSCALL
            , "Count the total of calls made to the operate system per period to finish a process."
            , "Number of calls"
            , NETDATA_PROCESS_GROUP
            , 977
            , netdata_create_global_dimension
            , &publish_aggregated[NETDATA_EXIT_START]
            , 2);

    if(kretprobe) {
        netdata_create_chart(NETDATA_EBPF_FAMILY
                , NETDATA_PROCESS_ERROR_NAME
                , "Count the number of errors related to process"
                , "Number of calls"
                , NETDATA_PROCESS_GROUP
                , 979
                , netdata_create_global_dimension
                , &publish_aggregated[NETDATA_EXIT_START]
                , NETDATA_PROCESS_ERRORS);
    }
}


static void netdata_create_charts() {
    netdata_global_charts_create();
}

static void netdata_update_publish(netdata_publish_syscall_t *publish
        , netdata_publish_vfs_common_t *pvc
        , netdata_syscall_stat_t *input) {

    netdata_publish_syscall_t *move = publish;
    while(move) {
        if(input->call != move->pcall) {
            //This condition happens to avoid initial values with dimensions higher than normal values.
            if(move->pcall) {
                move->ncall = (input->call > move->pcall)?input->call - move->pcall: move->pcall - input->call;
                move->nbyte = (input->bytes > move->pbyte)?input->bytes - move->pbyte: move->pbyte - input->bytes;
                move->nerr = (input->ecall > move->nerr)?input->ecall - move->perr: move->perr - input->ecall;
            } else {
                move->ncall = 0;
                move->nbyte = 0;
                move->nerr = 0;
            }

            move->pcall = input->call;
            move->pbyte = input->bytes;
            move->perr = input->ecall;
        } else {
            move->ncall = 0;
            move->nbyte = 0;
            move->nerr = 0;
        }

        input = input->next;
        move = move->next;
    }

    pvc->write = -((long)publish[2].nbyte);
    pvc->read = (long)publish[3].nbyte;
}

static inline void write_begin_chart(char *family, char *name)
{
    int ret = printf( "BEGIN %s.%s\n"
            , family
            , name);

    (void)ret;
}

static inline void write_chart_dimension(char *dim, long long value)
{
    int ret = printf("SET %s = %lld\n", dim, value);
    (void)ret;
}

static void write_global_count_chart(char *name, char *family, netdata_publish_syscall_t *move, int end) {
    write_begin_chart(family, name);

    int i = 0;
    while (move && i < end) {
        write_chart_dimension(move->dimension, move->ncall);

        move = move->next;
        i++;
    }

    printf("END\n");
}

static void write_global_err_chart(char *name, char *family, netdata_publish_syscall_t *move, int end) {
    write_begin_chart(family, name);

    int i = 0;
    while (move && i < end) {
        write_chart_dimension(move->dimension, move->nerr);

        move = move->next;
        i++;
    }

    printf("END\n");
}

static void write_io_chart(char *family, netdata_publish_vfs_common_t *pvc) {
    printf( "BEGIN %s.%s\n"
            , family
            , NETDATA_VFS_IO_FILE_BYTES);

    printf("SET %s = %lld\n", NETDATA_VFS_DIM_IN_FILE_BYTES , (long long) pvc->write);
    printf("SET %s = %lld\n", NETDATA_VFS_DIM_OUT_FILE_BYTES , (long long) pvc->read);

    printf("END\n");
}

static void netdata_publish_data() {
    netdata_publish_vfs_common_t pvc;
    netdata_update_publish(publish_aggregated, &pvc, aggregated_data);

    write_global_count_chart(NETDATA_FILE_OPEN_CLOSE_COUNT, NETDATA_EBPF_FAMILY, publish_aggregated, 2);
    write_global_count_chart(NETDATA_VFS_FILE_CLEAN_COUNT, NETDATA_EBPF_FAMILY, &publish_aggregated[NETDATA_DEL_START], 1);
    write_global_count_chart(NETDATA_VFS_FILE_IO_COUNT, NETDATA_EBPF_FAMILY, &publish_aggregated[NETDATA_IN_START_BYTE], 2);
    write_global_count_chart(NETDATA_EXIT_SYSCALL, NETDATA_EBPF_FAMILY, &publish_aggregated[NETDATA_EXIT_START], 2);
    write_global_count_chart(NETDATA_PROCESS_SYSCALL, NETDATA_EBPF_FAMILY, &publish_aggregated[NETDATA_PROCESS_START], 2);
    if(kretprobe) {
        write_global_err_chart(NETDATA_VFS_FILE_ERR_COUNT, NETDATA_EBPF_FAMILY, publish_aggregated, NETDATA_FILE_ERRORS);
        write_global_err_chart(NETDATA_PROCESS_ERROR_NAME, NETDATA_EBPF_FAMILY, &publish_aggregated[NETDATA_EXIT_START], NETDATA_PROCESS_ERRORS);
    }

    write_io_chart(NETDATA_EBPF_FAMILY, &pvc);
}

void *process_publisher(void *ptr)
{
    (void)ptr;
    netdata_create_charts();

    usec_t step = update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!close_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        pthread_mutex_lock(&lock);
        netdata_publish_data();
        pthread_mutex_unlock(&lock);

        fflush(stdout);
    }

    return NULL;
}

static void move_from_kernel2user_global() {
    uint32_t idx;
    uint32_t val;
    uint32_t res[NETDATA_GLOBAL_VECTOR];

    for (idx = 0; idx < NETDATA_GLOBAL_VECTOR; idx++) {
        if(!bpf_map_lookup_elem(map_fd[1], &idx, &val)) {
            res[idx] = val;
        }
    }

    aggregated_data[0].call = res[0]; //open
    aggregated_data[1].call = res[14]; //close
    aggregated_data[2].call = res[8]; //unlink
    aggregated_data[3].call = res[2]; //write
    aggregated_data[4].call = res[5]; //read
    aggregated_data[5].call = res[10]; //exit
    aggregated_data[6].call = res[11]; //release
    aggregated_data[7].call = res[12]; //fork
    aggregated_data[8].call = res[16]; //thread

    aggregated_data[0].ecall = res[1]; //open
    aggregated_data[1].ecall = res[15]; //close
    aggregated_data[2].ecall = res[9]; //unlink
    aggregated_data[3].ecall = res[3]; //write
    aggregated_data[4].ecall = res[6]; //read
    aggregated_data[7].ecall = res[13]; //fork
    aggregated_data[8].ecall = res[17]; //thread

    aggregated_data[2].bytes = (uint64_t)res[4]; //write
    aggregated_data[3].bytes = (uint64_t)res[7]; //read
}

static void move_from_kernel2user()
{
    move_from_kernel2user_global();
}

void *process_collector(void *ptr)
{
    (void)ptr;

    usec_t step = 778879ULL;
    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!close_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        pthread_mutex_lock(&lock);
        move_from_kernel2user();
        pthread_mutex_unlock(&lock);
    }

    return NULL;
}

void set_global_labels() {
    int i;

    static char *file_names[NETDATA_MAX_MONITOR_VECTOR] = { "open", "close", "unlink", "write", "read", "exit", "release_task", "process", "thread" };

    netdata_syscall_stat_t *is = aggregated_data;
    netdata_syscall_stat_t *prev = NULL;

    netdata_publish_syscall_t *pio = publish_aggregated;
    netdata_publish_syscall_t *publish_prev = NULL;
    for (i = 0; i < NETDATA_MAX_MONITOR_VECTOR; i++) {
        if(prev) {
            prev->next = &is[i];
        }
        prev = &is[i];

        pio[i].dimension = file_names[i];
        if(publish_prev) {
            publish_prev->next = &pio[i];
        }
        publish_prev = &pio[i];
    }
}

int allocate_global_vectors() {
    aggregated_data = callocz(NETDATA_MAX_MONITOR_VECTOR, sizeof(netdata_syscall_stat_t));
    if(!aggregated_data) {
        return -1;
    }

    publish_aggregated = callocz(NETDATA_MAX_MONITOR_VECTOR, sizeof(netdata_publish_syscall_t));
    if(!publish_aggregated) {
        return -1;
    }

    return 0;
}

static void build_complete_path(char *out, size_t length,char *path, char *filename) {
    if(path){
        snprintf(out, length, "%s/%s", path, filename);
    } else {
        snprintf(out, length, "%s", filename);
    }
}

static int ebpf_load_libraries()
{
    char *error = NULL;
    char lpath[4096];

    build_complete_path(lpath, 4096, plugin_dir, "libnetdata_ebpf.so");
    libnetdata = dlopen(lpath, RTLD_LAZY);
    if (!libnetdata) {
        error("[EBPF_PROCESS] Cannot load %s.", lpath);
        return -1;
    } else {
        load_bpf_file = dlsym(libnetdata, "load_bpf_file");
        if ((error = dlerror()) != NULL) {
            error("[EBPF_PROCESS] Cannot find load_bpf_file: %s", error);
            return -1;
        }

        map_fd =  dlsym(libnetdata, "map_fd");
        if ((error = dlerror()) != NULL) {
            fputs(error, stderr);
            return -1;
        }

        bpf_map_lookup_elem = dlsym(libnetdata, "bpf_map_lookup_elem");
        if ((error = dlerror()) != NULL) {
            fputs(error, stderr);
            return -1;
        }
    }

    return 0;
}

int process_load_ebpf()
{
    char lpath[4096];

    char *name = (!kretprobe)?"pnetdata_ebpf_process.o":"rnetdata_ebpf_process.o";

    build_complete_path(lpath, 4096, plugin_dir,  name);
    if (load_bpf_file(lpath) ) {
        error("[EBPF_PROCESS] Cannot load program: %s.", lpath);
        return -1;
    } else {
        info("[EBPF PROCESS]: The eBPF program %s was loaded with success.", name);
    }

    return 0;
}

void set_global_variables() {
    //Get environment variables
    plugin_dir = getenv("NETDATA_PLUGINS_DIR");
    if(!plugin_dir)
        plugin_dir = PLUGINS_DIR;

    user_config_dir = getenv("NETDATA_USER_CONFIG_DIR");
    if(!user_config_dir)
        user_config_dir = CONFIG_DIR;

    stock_config_dir = getenv("NETDATA_STOCK_CONFIG_DIR");
    if(!stock_config_dir)
        stock_config_dir = LIBCONFIG_DIR;

    netdata_configured_log_dir = getenv("NETDATA_LOG_DIR");
    if(!netdata_configured_log_dir)
        netdata_configured_log_dir = LOG_DIR;
}

static void set_global_values(char *ptr) {
    if(!strcmp(ptr, "entry"))
        kretprobe = 0;

}

static int load_collector_file(char *path) {
    char lpath[4096];

    build_complete_path(lpath, 4096, path, "ebpf_process.conf" );

    if (!appconfig_load(&collector_config, lpath, 0, NULL))
        return 1;

    struct section *sec = collector_config.sections;
    while(sec) {
        error("KILLME SEC %s\n", sec->name);
        if(sec) {
            struct config_option *values = sec->values;
            while(values) {
                error("KILLME OPT %s\n", values->name);
                values = values->next;
            }
        }
        sec = sec->next;
    }
    char *def = { "entry" };
    char *s = appconfig_get(&collector_config, CONFIG_SECTION_GLOBAL, "load", def);
    if(s)
    /*
    if (s)
        select_collector_mode(s);
     */


    return 0;
}

void open_developer_log() {
    char filename[FILENAME_MAX+1];
    int tot = sprintf(filename, "%s/%s",  netdata_configured_log_dir, NETDATA_DEVELOPER_LOG_FILE);

    if(tot > 0)
        developer_log = fopen(filename, "a");
}

int main(int argc, char **argv)
{
    (void)argc;
    (void)argv;

    //set name
    program_name = "vfs.plugin";

    //disable syslog
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;

    if (argc > 1) {
        update_every = (int)strtol(argv[1], NULL, 10);
    }

    struct rlimit r = {RLIM_INFINITY, RLIM_INFINITY};
    if (setrlimit(RLIMIT_MEMLOCK, &r)) {
        error("[EBPF PROCESS] setrlimit(RLIMIT_MEMLOCK)");
        return 1;
    }

    set_global_variables();

    if (load_collector_file(user_config_dir)) {
        info("[EBPF PROCESS] does not have a configuration file. It is starting with default options.");
    }

    if(ebpf_load_libraries()) {
        error("[EBPF_PROCESS] Cannot load library.");
        thread_finished++;
        int_exit(2);
    }

    signal(SIGINT, int_exit);
    signal(SIGTERM, int_exit);

    if (process_load_ebpf()) {
        thread_finished++;
        int_exit(3);
    }

    if(allocate_global_vectors()) {
        thread_finished++;
        error("[EBPF_PROCESS] Cannot allocate necessary vectors.");
        int_exit(4);
    }

    set_global_labels();

    open_developer_log();

    if (pthread_mutex_init(&lock, NULL)) {
        thread_finished++;
        int_exit(5);
    }

    pthread_attr_t attr;
    pthread_attr_init(&attr);
    pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_JOINABLE);
    pthread_t thread[NETDATA_VFS_THREAD];

    int i;
    int end = NETDATA_VFS_THREAD;

    void * (*function_pointer[])(void *) = {process_publisher, process_collector };

    for ( i = 0; i < end ; i++ ) {
        if ( ( pthread_create(&thread[i], &attr, function_pointer[i], NULL) ) ) {
            error("[EBPF_PROCESS] Cannot create threads.");
            thread_finished++;
            int_exit(6);
        }
    }

    for ( i = 0; i < end ; i++ ) {
        if ( (pthread_join(thread[i], NULL) ) ) {
            error("[EBPF_PROCESS] Cannot join threads.");
            thread_finished++;
            int_exit(7);
        }
    }

    thread_finished++;
    int_exit(0);

    return 0;
}
