// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/time.h>
#include <sys/resource.h>

#include "ebpf_process.h"

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
int (*set_bpf_perf_event)(int, int);
int (*perf_event_unmap)(struct perf_event_mmap_page *, size_t);
int (*perf_event_mmap_header)(int, struct perf_event_mmap_page **, int);
void (*netdata_perf_loop_multi)(int *, struct perf_event_mmap_page **, int, int *, int (*nsb)(void *, int), int);
int *map_fd = NULL;

//Perf event variables
static int pmu_fd[NETDATA_MAX_PROCESSOR];
static struct perf_event_mmap_page *headers[NETDATA_MAX_PROCESSOR];
int page_cnt = 8;

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
static int mode = 2;
static int debug_log = 0;
static int use_stdout = 0;
struct config collector_config;
static int mykernel = 0;
static int nprocs;
uint32_t *hash_values;

pthread_mutex_t lock;

static char *dimension_names[NETDATA_MAX_MONITOR_VECTOR] = { "open", "close", "delete", "read", "write", "process", "task", "process", "thread" };
static char *id_names[NETDATA_MAX_MONITOR_VECTOR] = { "do_sys_open", "__close_fd", "vfs_unlink", "vfs_read", "vfs_write", "do_exit", "release_task", "_do_fork", "sys_clone" };
static char *status[] = { "process", "zombie" };

int event_pid = 0;
netdata_ebpf_events_t collector_events[] = {
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

static int unmap_memory() {
    int i;
    int size = (int)sysconf(_SC_PAGESIZE)*(page_cnt + 1);
    for ( i = 0 ; i < nprocs ; i++ ) {
        if (perf_event_unmap(headers[i], size) < 0) {
            fprintf(stderr,"[EBPF PROCESS] CANNOT unmap headers.\n");
            return -1;
        }

        close(pmu_fd[i]);
    }

    return 0;
}

static void int_exit(int sig)
{
    close_plugin = 1;

    //When both threads were not finished case I try to go in front this address, the collector will crash
    if (!thread_finished) {
        return;
    }

    if (aggregated_data) {
        free(aggregated_data);
        aggregated_data = NULL;
    }

    if (publish_aggregated) {
        free(publish_aggregated);
        publish_aggregated = NULL;
    }

    if(mode == 1 && debug_log) {
        unmap_memory();
    }

    if (libnetdata) {
        dlclose(libnetdata);
        libnetdata = NULL;
    }

    if (developer_log) {
        fclose(developer_log);
        developer_log = NULL;
    }

    if (hash_values) {
        freez(hash_values);
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

                if (fd > 2)
                    close (fd);
            }

            int sid = setsid();
            if(sid >= 0) {
                sleep(1);
                if(debug_log) {
                    open_developer_log();
                }
                debug(D_EXIT, "Wait for father %d die", event_pid);
                clean_kprobe_events(developer_log, event_pid, collector_events);
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

static inline void netdata_write_chart_cmd(char *type
                                    , char *id
                                    , char *axis
                                    , char *web
                                    , int order)
{
    printf("CHART %s.%s '' '' '%s' '%s' '' line %d 1 ''\n"
            , type
            , id
            , axis
            , web
            , order);
}

static void netdata_write_global_dimension(char *dimension, char *name)
{
    printf("DIMENSION %s %s absolute 1 1\n", dimension, name);
}

static void netdata_create_global_dimension(void *ptr, int end)
{
    netdata_publish_syscall_t *move = ptr;

    int i = 0;
    while (move && i < end) {
        netdata_write_global_dimension(move->name, move->dimension);

        move = move->next;
        i++;
    }
}
static inline void netdata_create_chart(char *family
                                , char *name
                                , char *axis
                                , char *web
                                , int order
                                , void (*ncd)(void *, int)
                                , void *move
                                , int end)
{
    netdata_write_chart_cmd(family, name, axis, web, order);

    ncd(move, end);
}

static void netdata_create_io_chart(char *family, char *name, char *axis, char *web, int order) {
    printf("CHART %s.%s '' '' '%s' '%s' '' line %d 1 ''\n"
            , family
            , name
            , axis
            , web
            , order);

    printf("DIMENSION %s %s absolute 1 1\n", id_names[3], NETDATA_VFS_DIM_OUT_FILE_BYTES);
    printf("DIMENSION %s %s absolute 1 1\n", id_names[4], NETDATA_VFS_DIM_IN_FILE_BYTES);
}

static void netdata_process_status_chart(char *family, char *name, char *axis, char *web, int order) {
    printf("CHART %s.%s '' '' '%s' '%s' '' line %d 1 ''\n"
            , family
            , name
            , axis
            , web
            , order);

    printf("DIMENSION %s '' absolute 1 1\n", status[0]);
    printf("DIMENSION %s '' absolute 1 1\n", status[1]);
}

static void netdata_global_charts_create() {
    netdata_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_FILE_OPEN_CLOSE_COUNT
            , "Calls"
            , NETDATA_FILE_GROUP
            , 970
            , netdata_create_global_dimension
            , publish_aggregated
            , 2);

    if(mode < 2) {
        netdata_create_chart(NETDATA_EBPF_FAMILY
                , NETDATA_FILE_OPEN_ERR_COUNT
                , "Calls"
                , NETDATA_FILE_GROUP
                , 971
                , netdata_create_global_dimension
                , publish_aggregated
                , 2);
    }

    netdata_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_VFS_FILE_CLEAN_COUNT
            , "Calls"
            , NETDATA_VFS_GROUP
            , 972
            , netdata_create_global_dimension
            , &publish_aggregated[NETDATA_DEL_START]
            , 1);

    netdata_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_VFS_FILE_IO_COUNT
            , "Calls"
            , NETDATA_VFS_GROUP
            , 973
            , netdata_create_global_dimension
            , &publish_aggregated[NETDATA_IN_START_BYTE]
            , 2);

    if(mode < 2) {
        netdata_create_io_chart(NETDATA_EBPF_FAMILY
                , NETDATA_VFS_IO_FILE_BYTES
                , "bytes/s"
                , NETDATA_VFS_GROUP
                , 974);

        netdata_create_chart(NETDATA_EBPF_FAMILY
                , NETDATA_VFS_FILE_ERR_COUNT
                , "Calls"
                , NETDATA_VFS_GROUP
                , 975
                , netdata_create_global_dimension
                , &publish_aggregated[2]
                , NETDATA_VFS_ERRORS);

    }

    netdata_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_PROCESS_SYSCALL
            , "Calls"
            , NETDATA_PROCESS_GROUP
            , 976
            , netdata_create_global_dimension
            , &publish_aggregated[NETDATA_PROCESS_START]
            , 2);

    netdata_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_EXIT_SYSCALL
            , "Calls"
            , NETDATA_PROCESS_GROUP
            , 977
            , netdata_create_global_dimension
            , &publish_aggregated[NETDATA_EXIT_START]
            , 2);

    netdata_process_status_chart(NETDATA_EBPF_FAMILY
            , NETDATA_PROCESS_STATUS_NAME
            , "Total"
            , NETDATA_PROCESS_GROUP
            , 978);

    if(mode < 2) {
        netdata_create_chart(NETDATA_EBPF_FAMILY
                , NETDATA_PROCESS_ERROR_NAME
                , "Calls"
                , NETDATA_PROCESS_GROUP
                , 979
                , netdata_create_global_dimension
                , &publish_aggregated[NETDATA_PROCESS_START]
                , 2);
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

    pvc->running = (long)publish[7].ncall - (long)publish[8].ncall;
    publish[6].ncall = -publish[6].ncall; // release
    pvc->zombie = (long)publish[5].ncall + (long)publish[6].ncall;
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
        write_chart_dimension(move->name, move->ncall);

        move = move->next;
        i++;
    }

    printf("END\n");
}

static void write_global_err_chart(char *name, char *family, netdata_publish_syscall_t *move, int end) {
    write_begin_chart(family, name);

    int i = 0;
    while (move && i < end) {
        write_chart_dimension(move->name, move->nerr);

        move = move->next;
        i++;
    }

    printf("END\n");
}

static void write_io_chart(char *family, netdata_publish_vfs_common_t *pvc) {
    write_begin_chart(family, NETDATA_VFS_IO_FILE_BYTES);

    write_chart_dimension(id_names[3], (long long) pvc->write);
    write_chart_dimension(id_names[4], (long long) pvc->read);

    printf("END\n");
}

static void write_status_chart(char *family, netdata_publish_vfs_common_t *pvc) {
    write_begin_chart(family, NETDATA_PROCESS_STATUS_NAME);

    write_chart_dimension(status[0], (long long) pvc->running);
    write_chart_dimension(status[1], (long long) pvc->zombie);

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

    write_status_chart(NETDATA_EBPF_FAMILY, &pvc);
    if(mode < 2) {
        write_global_err_chart(NETDATA_FILE_OPEN_ERR_COUNT, NETDATA_EBPF_FAMILY, publish_aggregated, 2);
        write_global_err_chart(NETDATA_VFS_FILE_ERR_COUNT, NETDATA_EBPF_FAMILY, &publish_aggregated[2], NETDATA_VFS_ERRORS);
        write_global_err_chart(NETDATA_PROCESS_ERROR_NAME, NETDATA_EBPF_FAMILY, &publish_aggregated[NETDATA_PROCESS_START], 2);

        write_io_chart(NETDATA_EBPF_FAMILY, &pvc);
    }
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
    uint32_t res[NETDATA_GLOBAL_VECTOR];

    uint32_t *val = hash_values;
    for (idx = 0; idx < NETDATA_GLOBAL_VECTOR; idx++) {
        if(!bpf_map_lookup_elem(map_fd[1], &idx, val)) {
            uint32_t total = 0;
            int i;
            int end = (mykernel < 265984)?1:nprocs;
            for (i = 0; i < end; i++)
                total += val[i];

            res[idx] = total;
        } else {
            res[idx] = 0;
        }
    }

    aggregated_data[0].call = res[0]; //open
    aggregated_data[1].call = res[14]; //close
    aggregated_data[2].call = res[8]; //unlink
    aggregated_data[3].call = res[5] + res[21]; //read + readv
    aggregated_data[4].call = res[2] + res[18]; //write + writev
    aggregated_data[5].call = res[10]; //exit
    aggregated_data[6].call = res[11]; //release
    aggregated_data[7].call = res[12]; //fork
    aggregated_data[8].call = res[16]; //thread

    aggregated_data[0].ecall = res[1]; //open
    aggregated_data[1].ecall = res[15]; //close
    aggregated_data[2].ecall = res[9]; //unlink
    aggregated_data[3].ecall = res[6] + res[22];  //read + readv
    aggregated_data[4].ecall = res[3] + res[19]; //write + writev
    aggregated_data[7].ecall = res[13]; //fork
    aggregated_data[8].ecall = res[17]; //thread

    aggregated_data[2].bytes = (uint64_t)res[4] + (uint64_t)res[20]; //write + writev
    aggregated_data[3].bytes = (uint64_t)res[7] + (uint64_t)res[23];//read + readv
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

static int netdata_store_bpf(void *data, int size) {
    (void)size;

    if (close_plugin)
        return 0;

    if(!debug_log)
        return -2; //LIBBPF_PERF_EVENT_CONT;

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

    if (mode == 1 && debug_log) {
        netdata_perf_loop_multi(pmu_fd, headers, nprocs, &close_plugin, netdata_store_bpf, page_cnt);
    }

    return NULL;
}

void set_global_labels() {
    int i;

    netdata_syscall_stat_t *is = aggregated_data;
    netdata_syscall_stat_t *prev = NULL;

    netdata_publish_syscall_t *pio = publish_aggregated;
    netdata_publish_syscall_t *publish_prev = NULL;
    for (i = 0; i < NETDATA_MAX_MONITOR_VECTOR; i++) {
        if(prev) {
            prev->next = &is[i];
        }
        prev = &is[i];

        pio[i].dimension = dimension_names[i];
        pio[i].name = id_names[i];
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

    hash_values = callocz(nprocs, sizeof(uint32_t));
    if(!hash_values) {
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

static int map_memory() {
    int i;
    for (i = 0; i < nprocs; i++) {
        pmu_fd[i] = set_bpf_perf_event(i, 2);

        if (perf_event_mmap_header(pmu_fd[i], &headers[i], page_cnt) < 0) {
            return -1;
        }
    }
    return 0;
}

static int ebpf_load_libraries()
{
    char *err = NULL;
    char lpath[4096];

    build_complete_path(lpath, 4096, plugin_dir, "libnetdata_ebpf.so");
    libnetdata = dlopen(lpath, RTLD_LAZY);
    if (!libnetdata) {
        error("[EBPF_PROCESS] Cannot load %s.", lpath);
        return -1;
    } else {
        load_bpf_file = dlsym(libnetdata, "load_bpf_file");
        if ((err = dlerror()) != NULL) {
            error("[EBPF_PROCESS] Cannot find load_bpf_file: %s", err);
            return -1;
        }

        map_fd =  dlsym(libnetdata, "map_fd");
        if ((err = dlerror()) != NULL) {
            error("[EBPF_PROCESS] Cannot find map_fd: %s", err);
            return -1;
        }

        bpf_map_lookup_elem = dlsym(libnetdata, "bpf_map_lookup_elem");
        if ((err = dlerror()) != NULL) {
            error("[EBPF_PROCESS] Cannot find bpf_map_lookup_elem: %s", err);
            return -1;
        }

        if(mode == 1) {
            set_bpf_perf_event = dlsym(libnetdata, "set_bpf_perf_event");
            if ((err = dlerror()) != NULL) {
                error("[EBPF_PROCESS] Cannot find set_bpf_perf_event: %s", err);
                return -1;
            }

            perf_event_unmap =  dlsym(libnetdata, "perf_event_unmap");
            if ((err = dlerror()) != NULL) {
                error("[EBPF_PROCESS] Cannot find perf_event_unmap: %s", err);
                return -1;
            }

            perf_event_mmap_header =  dlsym(libnetdata, "perf_event_mmap_header");
            if ((err = dlerror()) != NULL) {
                error("[EBPF_PROCESS] Cannot find perf_event_mmap_header: %s", err);
                return -1;
            }

            netdata_perf_loop_multi = dlsym(libnetdata, "netdata_perf_loop_multi");
            if ((err = dlerror()) != NULL) {
                error("[EBPF_PROCESS] Cannot find netdata_perf_loop_multi: %s", err);
                return -1;
            }
        }
    }

    return 0;
}

char *select_file() {
    if(!mode)
        return "rnetdata_ebpf_process.o";
    if(mode == 1)
        return "dnetdata_ebpf_process.o";

    return "pnetdata_ebpf_process.o";
}

int process_load_ebpf()
{
    char lpath[4096];

    char *name = select_file();

    build_complete_path(lpath, 4096, plugin_dir,  name);
    event_pid = getpid();
    if (load_bpf_file(lpath, event_pid) ) {
        error("[EBPF_PROCESS] Cannot load program: %s", lpath);
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

    page_cnt *= (int)sysconf(_SC_NPROCESSORS_ONLN);

    nprocs = (int)sysconf(_SC_NPROCESSORS_ONLN);
    if (nprocs > NETDATA_MAX_PROCESSOR) {
        nprocs = NETDATA_MAX_PROCESSOR;
    }
}

static void change_collector_event() {
    int i;
    for (i = 0; collector_events[i].name ; i++ ) {
        collector_events[i].type = 'p';
    }

    if (mykernel < 328448)
        collector_events[i].name = NULL;
}

static inline void what_to_load(char *ptr) {
    if (!strcasecmp(ptr, "return"))
        mode = 0;
    /*
    else if (!strcasecmp(ptr, "dev"))
        mode = 1;
        */
    else
        change_collector_event();
}

static inline void enable_debug(char *ptr) {
    if (!strcasecmp(ptr, "yes"))
        debug_log = 1;
}

static inline void set_log_file(char *ptr) {
    if (!strcasecmp(ptr, "yes"))
        use_stdout = 1;
}

static void set_global_values() {
    struct section *sec = collector_config.sections;
    while(sec) {
        if(!strcasecmp(sec->name, "global")) {
            struct config_option *values = sec->values;
            while(values) {
                if(!strcasecmp(values->name, "load"))
                    what_to_load(values->value);
                else if(!strcasecmp(values->name, "debug log"))
                    enable_debug(values->value);
                else if(!strcasecmp(values->name, "use stdout"))
                    set_log_file(values->value);

                values = values->next;
            }
        }
        sec = sec->next;
    }
}

static int load_collector_file(char *path) {
    char lpath[4096];

    build_complete_path(lpath, 4096, path, "ebpf_process.conf" );

    if (!appconfig_load(&collector_config, lpath, 0, NULL))
        return 1;

    set_global_values();

    return 0;
}

int main(int argc, char **argv)
{
    (void)argc;
    (void)argv;

    mykernel =  get_kernel_version();
    if(!has_condition_to_run(mykernel))
        return 1;

    //set name
    program_name = "ebpf_process.plugin";

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
        return 2;
    }

    set_global_variables();

    if (load_collector_file(user_config_dir)) {
        info("[EBPF PROCESS] does not have a configuration file. It is starting with default options.");
    }

    if(ebpf_load_libraries()) {
        error("[EBPF_PROCESS] Cannot load library.");
        thread_finished++;
        int_exit(3);
    }

    signal(SIGINT, int_exit);
    signal(SIGTERM, int_exit);

    if (process_load_ebpf()) {
        thread_finished++;
        int_exit(4);
    }

    if(allocate_global_vectors()) {
        thread_finished++;
        error("[EBPF_PROCESS] Cannot allocate necessary vectors.");
        int_exit(5);
    }

    if(mode == 1 && debug_log) {
        if(map_memory()) {
            thread_finished++;
            error("[EBPF_PROCESS] Cannot map memory used with perf events.");
            int_exit(6);
        }
    }

    set_global_labels();

    if(debug_log) {
        open_developer_log();
    }

    if (pthread_mutex_init(&lock, NULL)) {
        thread_finished++;
        int_exit(7);
    }

    pthread_attr_t attr;
    pthread_attr_init(&attr);
    pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_JOINABLE);
    pthread_t thread[NETDATA_EBPF_PROCESS_THREADS];

    int i;
    int end = NETDATA_EBPF_PROCESS_THREADS;

    void * (*function_pointer[])(void *) = {process_publisher, process_collector, process_log };

    for ( i = 0; i < end ; i++ ) {
        if ( ( pthread_create(&thread[i], &attr, function_pointer[i], NULL) ) ) {
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
