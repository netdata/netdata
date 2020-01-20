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
void *libnetdata = NULL;
int (*load_bpf_file)(char *) = NULL;
int (*test_bpf_perf_event)(int) = NULL;
int (*perf_event_unmap)(struct perf_event_mmap_page *, size_t) = NULL;
int (*perf_event_mmap_header)(int, struct perf_event_mmap_page **, int) = NULL;
void (*netdata_perf_loop_multi)(int *, struct perf_event_mmap_page **, int, int *, int (*nsb)(void *, int), int) = NULL;

static char *user_config_dir = NULL;
static char *stock_config_dir = NULL;
static char *plugin_dir = NULL;

//static int *pmu_fd; // When allocate the library gives error
static int pmu_fd[NETDATA_MAX_PROCESSOR];
static struct perf_event_mmap_page *headers[NETDATA_MAX_PROCESSOR];

//Global vectors
netdata_syscall_stat_t *file_syscall = NULL;
netdata_publish_syscall_t *publish_file = NULL;

static int update_every = 1;
static int thread_finished = 0;
static int close_plugin = 0;
static int page_cnt = 8;

static int unmap_memory() {
    int nprocs = sysconf(_SC_NPROCESSORS_ONLN);

    if (nprocs > NETDATA_MAX_PROCESSOR) {
        nprocs = NETDATA_MAX_PROCESSOR;
    }

    int i;
    int size = sysconf(_SC_PAGESIZE)*(page_cnt + 1);
    for (i = 0 ; i < nprocs ; i++) {
        if (perf_event_unmap(headers[i], size) < 0) {
            error("[VFS] Cannot unmap memory.");
            return -1;
        }

        if (close(pmu_fd[i]) )
            error("[VFS] Cannot close file descriptor.");
    }

    return 0;
}

static void int_exit(int sig)
{
    close_plugin = 1;

    //When both threads were not finished case I try to go in front this address, the collector will crash
    if(!thread_finished) {
        return;
    }

    unmap_memory();

    if(file_syscall) {
        free(file_syscall);
    }

    if(publish_file) {
        free(publish_file);
    }

    if(libnetdata) {
        dlclose(libnetdata);
    }

    exit(sig);
}

static void netdata_create_chart(char *family
                                , char *name
                                , char *msg
                                , char *axis
                                , int order
                                , netdata_publish_syscall_t *move
                                , int end)
                                {
    printf("CHART %s.%s '' '%s' '%s' 'syscall' '' line %d 1 ''\n"
            , family
            , name
            , msg
            , axis
            , order);

    int i = 0;
    while (move && i < end) {
        printf("DIMENSION %s '' absolute 1 1\n", move->dimension);

        move = move->next;
        i++;
    }
}

static void netdata_create_io_chart(char *family, char *name, char *msg, char *axis, int order) {
    printf("CHART %s.%s '' '%s' '%s' 'syscall' '' line %d 1 ''\n"
            , family
            , name
            , msg
            , axis
            , order);

    printf("DIMENSION %s '' absolute 1 1\n", NETDATA_VFS_DIM_IN_FILE_BYTES );
    printf("DIMENSION %s '' absolute 1 1\n", NETDATA_VFS_DIM_OUT_FILE_BYTES );
}

static void netdata_create_charts() {
    netdata_create_chart(NETDATA_VFS_FAMILY, NETDATA_VFS_FILE_OPEN_COUNT, "Number of calls for file IO.", "Number of calls", 970, publish_file, 1);
    netdata_create_chart(NETDATA_VFS_FAMILY, NETDATA_VFS_FILE_CLEAN_COUNT, "Number of calls for file IO.", "Number of calls", 971, &publish_file[1], 2);
    netdata_create_chart(NETDATA_VFS_FAMILY, NETDATA_VFS_FILE_WRITE_COUNT, "Number of calls for file IO.", "Number of calls", 972, &publish_file[3], 2);
    netdata_create_chart(NETDATA_VFS_FAMILY, NETDATA_VFS_FILE_READ_COUNT, "Number of calls for file IO.", "Number of calls", 973, &publish_file[5], 2);
    netdata_create_chart(NETDATA_VFS_FAMILY, NETDATA_VFS_FILE_ERR_COUNT, "Number of calls for file IO.", "Number of calls", 974, publish_file, NETDATA_MAX_FILE_VECTOR);

    netdata_create_chart(NETDATA_VFS_FAMILY, NETDATA_VFS_IN_FILE_BYTES, "Number of bytes written to file.", "bytes/s", 975, &publish_file[NETDATA_IN_START_BYTE], 2);
    netdata_create_chart(NETDATA_VFS_FAMILY, NETDATA_VFS_OUT_FILE_BYTES, "Number of bytes read from file.", "bytes/s", 976, &publish_file[NETDATA_OUT_START_BYTE], 2);

    netdata_create_io_chart(NETDATA_VFS_FAMILY, NETDATA_VFS_IO_FILE_BYTES, "Number of bytes read and written.", "bytes/s", 977);
}

static void netdata_update_publish(netdata_publish_syscall_t *publish, netdata_publish_vfs_common_t *pvc, netdata_syscall_stat_t *input) {
    netdata_publish_syscall_t *move = publish;
    while(move) {
        if(input->call != move->pcall) {
            //This condition happens to avoid initial values with dimensions higher than normal values.
            if(move->pcall) {
                move->ncall = (input->call - move->pcall);
                move->nbyte = (input->bytes - move->pbyte);
            } else {
                move->ncall = 0;
                move->nbyte = 0;
            }
            move->pcall = input->call;

            move->pbyte = input->bytes;
        } else {
            move->ncall = 0;
            move->nbyte = 0;
        }

        input = input->next;
        move = move->next;
    }

    pvc->write = -(publish[3].nbyte + publish[4].nbyte);
    pvc->read = (publish[5].nbyte + publish[6].nbyte);
}

static void write_count_chart(char *name,netdata_publish_syscall_t *move, int end) {
    printf( "BEGIN %s.%s\n"
            , NETDATA_VFS_FAMILY
            , name);

    int i = 0;
    while (move && i < end) {
        printf("SET %s = %lld\n", move->dimension, (long long) move->ncall);

        move = move->next;
        i++;
    }

    printf("END\n");
}

static void write_err_chart(char *name,netdata_publish_syscall_t *move) {
    printf( "BEGIN %s.%s\n"
            , NETDATA_VFS_FAMILY
            , name);

    while (move) {
        printf("SET %s = %lld\n", move->dimension, (long long) move->nerr);

        move = move->next;
    }

    printf("END\n");
}

static void write_bytes_chart(char *name,netdata_publish_syscall_t *move, int end) {
    printf( "BEGIN %s.%s\n"
            , NETDATA_VFS_FAMILY
            , name);

    int i = 0;
    while (move && i < end) {
        printf("SET %s = %lld\n", move->dimension, (long long) move->nbyte);

        move = move->next;
        i++;
    }

    printf("END\n");
}

static void write_io_chart(netdata_publish_vfs_common_t *pvc) {
    printf( "BEGIN %s.%s\n"
            , NETDATA_VFS_FAMILY
            , NETDATA_VFS_IO_FILE_BYTES);

    printf("SET %s = %lld\n", NETDATA_VFS_DIM_IN_FILE_BYTES , (long long) pvc->write);
    printf("SET %s = %lld\n", NETDATA_VFS_DIM_OUT_FILE_BYTES , (long long) pvc->read);

    printf("END\n");
}

static void netdata_publish_data() {
    netdata_publish_vfs_common_t pvc;
    netdata_update_publish(publish_file, &pvc, file_syscall);

    write_count_chart(NETDATA_VFS_FILE_OPEN_COUNT, publish_file, 1);
    write_count_chart(NETDATA_VFS_FILE_CLEAN_COUNT, &publish_file[1], 2);
    write_count_chart(NETDATA_VFS_FILE_WRITE_COUNT, &publish_file[3], 2);
    write_count_chart(NETDATA_VFS_FILE_READ_COUNT, &publish_file[5], 2);
    write_err_chart(NETDATA_VFS_FILE_ERR_COUNT, publish_file);

    write_bytes_chart(NETDATA_VFS_IN_FILE_BYTES, &publish_file[NETDATA_IN_START_BYTE], 2);
    write_bytes_chart(NETDATA_VFS_OUT_FILE_BYTES, &publish_file[NETDATA_OUT_START_BYTE], 2);

    write_io_chart(&pvc);
}

void *vfs_publisher(void *ptr)
{
    (void)ptr;
    netdata_create_charts();

    usec_t step = update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!close_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        netdata_publish_data();

        fflush(stdout);
    }

    return NULL;
}

static inline void set_stat_value(netdata_syscall_stat_t *out, netdata_syscall_kern_stat_t *e) {
    if (!e->error) {
        out->bytes += e->bytes;
        out->call++;
    } else {
        out->ecall++;
    }
}

static void set_file_vectors(netdata_syscall_kern_stat_t *e) {
    uint16_t num = e->sc_num;
    switch(num) {
#ifdef __x86_64__
        case 2:
        case 87: {
            if (!e->error) {
                file_syscall[e->idx].call++;
            } else {
                file_syscall[e->idx].ecall++;
            }
            break;
        }
#else
        case 5:
        case 10: {
                if (!e->error) {
                    file_syscall[e->idx].call++;
                } else {
                    file_syscall[e->idx].ecall++;
                }
                break;
            }
#endif
        default:
        {
            set_stat_value(&file_syscall[e->idx], e);
            break;
        }
    }
}

static int netdata_store_bpf(void *data, int size) {
    (void)size;

    if(close_plugin)
        return 0; //LIBBPF_PERF_EVENT_DONE

    netdata_syscall_kern_stat_t *e = data;
    switch(e->type) {
        case FILE_SYSCALL: {
            set_file_vectors(e);
            break;
        }
        default: {
            break;
        }
    }

    return -2; //LIBBPF_PERF_EVENT_CONT;
}

void *vfs_collector(void *ptr)
{
    (void)ptr;
    int nprocs = sysconf(_SC_NPROCESSORS_ONLN);

    netdata_perf_loop_multi(pmu_fd, headers, nprocs, (int *)&close_plugin, netdata_store_bpf, page_cnt);

    return NULL;
}

void set_file_values() {
    int i;

    static char *file_names[NETDATA_MAX_FILE_VECTOR] = { "open", "unlink", "truncate", "write", "writev", "read", "readv" };
#ifdef __x86_64__
    static uint16_t sys_num[NETDATA_MAX_FILE_VECTOR] = { 2, 87, 76, 1,  20, 0,  19 };
#else
    static uint16_t sys_num[NETDATA_MAX_FILE_VECTOR] = { 5, 10, 92, 4, 146, 3, 145 };
#endif

    netdata_syscall_stat_t *is = file_syscall;
    netdata_syscall_stat_t *prev = NULL;

    netdata_publish_syscall_t *pio = publish_file;
    netdata_publish_syscall_t *publish_prev = NULL;
    for (i = 0; i < NETDATA_MAX_FILE_VECTOR; i++) {
        is[i].sc_num = sys_num[i];

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
    file_syscall = callocz(NETDATA_MAX_FILE_VECTOR, sizeof(netdata_syscall_stat_t));
    if(!file_syscall) {
        return -1;
    }

    publish_file = callocz(NETDATA_MAX_FILE_VECTOR, sizeof(netdata_publish_syscall_t));
    if(!publish_file) {
        return -1;
    }

    return 0;
}

static int map_memory() {
    int nprocs = sysconf(_SC_NPROCESSORS_ONLN);

    if (nprocs > NETDATA_MAX_PROCESSOR) {
        nprocs = NETDATA_MAX_PROCESSOR;
    }

    int i;
    for (i = 0 ; i < nprocs ; i++) {
        pmu_fd[i] = test_bpf_perf_event(i);

        if (perf_event_mmap_header(pmu_fd[i], &headers[i], page_cnt) < 0) {
            error("[VFS] Cannot map header used to transfer data.");
            return -1;
        }
    }

    return 0;
}

static void build_complete_path(char *out, size_t length, char *filename) {
    if(plugin_dir){
        snprintf(out, length, "%s/%s", plugin_dir, filename);
    } else {
        snprintf(out, length, "%s", filename);
    }
}

int vfs_load_libraries()
{
    char *error = NULL;
    char lpath[4096];

    build_complete_path(lpath, 4096, "libnetdata_ebpf.so");
    libnetdata = dlopen(lpath, RTLD_LAZY);
    if (!libnetdata) {
        error("[VFS] Cannot load %s.", lpath);
        return -1;
    } else {
        load_bpf_file = dlsym(libnetdata, "load_bpf_file");
        if ((error = dlerror()) != NULL) {
            error("[VFS] Cannot find load_bpf_file: %s", error);
            return -1;
        }

        test_bpf_perf_event = dlsym(libnetdata, "test_bpf_perf_event");
        if ((error = dlerror()) != NULL) {
            error("[VFS] Cannot find test_bpf_perf_event: %s", error);
            return -1;
        }

        netdata_perf_loop_multi = dlsym(libnetdata, "my_perf_loop_multi");
        if ((error = dlerror()) != NULL) {
            error("[VFS] Cannot find netdata_perf_loop_multi: %s", error);
            return -1;
        }

        perf_event_unmap =  dlsym(libnetdata, "perf_event_unmap");
        if ((error = dlerror()) != NULL) {
            error("[VFS] Cannot find perf_event_mmap: %s", error);
            fputs(error, stderr);
            return -1;
        }

        perf_event_mmap_header =  dlsym(libnetdata, "perf_event_mmap_header");
        if ((error = dlerror()) != NULL) {
            error("[VFS] Cannot find  perf_event_mmap_header: %s", error);
            fputs(error, stderr);
            return -1;
        }
    }

    return 0;
}

int vfs_load_ebpf() {
    char lpath[4096];

    build_complete_path(lpath, 4096, "netdata_ebpf_vfs.o" );
    if (load_bpf_file(lpath) ) {
        error("[VFS] Cannot load program: %s.", lpath);
        return -1;
    }

    return 0;
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
        error("[NETWORK VIEWER] setrlimit(RLIMIT_MEMLOCK)");
        return 1;
    }

    //Get environment variables
    user_config_dir = getenv("NETDATA_USER_CONFIG_DIR");
    stock_config_dir = getenv("NETDATA_STOCK_CONFIG_DIR");
    plugin_dir = getenv("NETDATA_PLUGINS_DIR");

    if(vfs_load_libraries()) {
        error("[VFS] Cannot load library.");
        int_exit(2);
    }

    signal(SIGINT, int_exit);
    signal(SIGTERM, int_exit);

    if (vfs_load_ebpf()) {
        int_exit(3);
    }

    if (map_memory()) {
        int_exit(4);
    }

    if(allocate_global_vectors()) {
        error("[VFS] Cannot allocate necessary vectors.");
        int_exit(5);
    }

    set_file_values();
    //set_directory_values();

    pthread_attr_t attr;
    pthread_attr_init(&attr);
    pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_JOINABLE);
    pthread_t thread[2];

    int i;
    int end = 2;

    for ( i = 0; i < end ; i++ ) {
        if ( ( pthread_create(&thread[i], &attr, (!i)?vfs_publisher:vfs_collector, NULL) ) ) {
            error("[VFS] Cannot create threads.");
            int_exit(0);
            return 7;
        }

    }

    for ( i = 0; i < end ; i++ ) {
        if ( (pthread_join(thread[i], NULL) ) ) {
            error("[VFS] Cannot join threads.");
            int_exit(0);
            return 7;
        }
    }

    int_exit(0);

    return 0;
}
