// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/time.h>
#include <sys/resource.h>

#include "syscall_plugin.h"

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
void *libnetdatanv = NULL;
int (*load_bpf_file)(char *) = NULL;
int (*test_bpf_perf_event)(int);
int (*perf_event_mmap)(int);
int (*perf_event_mmap_header)(int, struct perf_event_mmap_page **);
void (*netdata_perf_loop_multi)(int *, struct perf_event_mmap_page **, int, int *, int (*nsb)(void *, int));

static char *user_config_dir = NULL;
static char *stock_config_dir = NULL;
static char *plugin_dir = NULL;

//static int *pmu_fd; // When allocate the library gives error
static int pmu_fd[NETDATA_MAX_PROCESSOR];
static struct perf_event_mmap_page *headers[NETDATA_MAX_PROCESSOR];

//Global vectors
netdata_syscall_stat_t *file_syscall = NULL;

netdata_publish_syscall_t *publish_file = NULL;

static void int_exit(int sig)
{
    (void)sig;

    if(libnetdatanv) {
        dlclose(libnetdatanv);
    }

    if(file_syscall) {
        free(file_syscall);
    }

    if(publish_file) {
        free(publish_file);
    }

    exit(sig);
}

static void netdata_create_chart(char *family, char *name, char *msg, char *axis, int order, netdata_publish_syscall_t *move) {
    printf("CHART %s.%s '' '%s' '%s' 'syscall' '' line %d 1 ''\n"
            , family
            , name
            , msg
            , axis
            , order);

    while (move) {
        printf("DIMENSION %s '' absolute 1 1\n", move->dimension);

        move = move->next;
    }
}


static void netdata_create_charts() {
    netdata_create_chart(SYSCALL_FAMILY, SYSCALL_IO_FILE_COUNT, "Number of calls for file IO.", "Number of calls", 970, publish_file);
    netdata_create_chart(SYSCALL_FAMILY, SYSCALL_IO_FILE_BYTES, "Number of bytes transferred during file IO..", "bytes/s", 971, &publish_file[NETDATA_IO_START_BYTE]);
}

static void netdata_update_publish(netdata_publish_syscall_t *publish, netdata_syscall_stat_t *input) {
    while(publish) {
        if(input->call != publish->pcall) {
            publish->ncall = (input->call - publish->pcall);
            publish->pcall = input->call;

            publish->nbyte = (input->bytes - publish->nbyte);
            publish->pbyte = input->bytes;
        } else {
            publish->ncall = 0;
            publish->nbyte = 0;
        }

        input = input->next;
        publish = publish->next;
    }
}

static void write_count_chart(char *name,netdata_publish_syscall_t *move) {
    printf( "BEGIN %s.%s\n"
            , SYSCALL_FAMILY
            , name);

    while (move) {
        printf("SET %s = %lld\n", move->dimension, (long long) move->ncall);

        move = move->next;
    }

    printf("END\n");
}

static void write_bytes_chart(char *name,netdata_publish_syscall_t *move) {
    printf( "BEGIN %s.%s\n"
            , SYSCALL_FAMILY
            , name);

    while (move) {
        printf("SET %s = %lld\n", move->dimension, (long long) move->nbyte);

        move = move->next;
    }

    printf("END\n");
}

static void netdata_publish_data() {
    netdata_update_publish(publish_file, file_syscall);

    write_count_chart(SYSCALL_IO_FILE_COUNT, publish_file);

    write_bytes_chart(SYSCALL_IO_FILE_BYTES, &publish_file[NETDATA_IO_START_BYTE]);
}

void *syscall_publisher(void *ptr)
{
    (void)ptr;
    netdata_create_charts();

    while(!netdata_exit) {
        sleep(1);

        netdata_publish_data();

        fflush(stdout);
    }

    return NULL;
}

static inline void set_stat_value(netdata_syscall_stat_t *out, netdata_syscall_kern_stat_t *e) {
    out->bytes += e->bytes;
    out->call++;
}

static void set_file_vectors(netdata_syscall_kern_stat_t *e) {
    uint16_t num = e->sc_num;
    switch(num) {
#ifdef __x86_64__
        case 2:
        case 87: {
            file_syscall[e->idx].call++;
            break;
        }
#else
        case 5:
        case 10: {
                file_syscall[e->idx].call++;
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

    if(netdata_exit)
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

void *syscall_collector(void *ptr)
{
    (void)ptr;
    int nprocs = sysconf(_SC_NPROCESSORS_ONLN);

    netdata_perf_loop_multi(pmu_fd, headers, nprocs, (int *)&netdata_exit, netdata_store_bpf);

    return NULL;
}

void set_file_values() {
    int i;

    static char *file_names[NETDATA_MAX_FILE_VECTOR] = { "open", "unlink", "truncate", "mknod", "write", "read", "writev", "readv" };
#ifdef __x86_64__
    uint16_t sys_num[NETDATA_MAX_FILE_VECTOR] = { 2, 87, 76, 133, 1, 0,  20,  19 };
#else
    uint16_t sys_num[NETDATA_MAX_FILE_VECTOR] = { 5, 10, 92,  14, 4, 3, 146, 145 };
#endif

    netdata_syscall_stat_t *is = file_syscall;
    netdata_syscall_stat_t *prev = NULL;

    netdata_publish_syscall_t *pio = publish_file;
    netdata_publish_syscall_t *publish_prev = NULL;
    for (i =0 ; i < NETDATA_MAX_FILE_VECTOR; i++) {
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

    if ( nprocs >NETDATA_MAX_PROCESSOR ) {
        nprocs =NETDATA_MAX_PROCESSOR;
    }

    int i;
    for ( i = 0 ; i < nprocs ; i++ ) {
        pmu_fd[i] = test_bpf_perf_event(i);

        if (perf_event_mmap(pmu_fd[i]) < 0) {
            error("[SYSCALL] Cannot map memory used to transfer data.");
            return -1;
        }
    }

    for ( i = 0 ; i < nprocs ; i++ ) {
        if (perf_event_mmap_header(pmu_fd[i], &headers[i]) < 0) {
            error("[SYSCALL] Cannot map header used to transfer data.");
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

int syscall_load_libraries()
{
    char *error = NULL;
    char lpath[4096];

    build_complete_path(lpath, 4096, "libnetdata_ebpf.so");
    libnetdatanv = dlopen(lpath, RTLD_LAZY);
    if (!libnetdatanv)
    {
        error("[SYSCALL] Cannot load %s.", lpath);
        return -1;
    }
    else
    {
        load_bpf_file = dlsym(libnetdatanv, "load_bpf_file");
        if ((error = dlerror()) != NULL) {
            error("[SYSCALL] Cannot find load_bpf_file: %s", error);
            return -1;
        }

        test_bpf_perf_event = dlsym(libnetdatanv, "test_bpf_perf_event");
        if ((error = dlerror()) != NULL) {
            error("[SYSCALL] Cannot find test_bpf_perf_event: %s", error);
            return -1;
        }

        netdata_perf_loop_multi = dlsym(libnetdatanv, "my_perf_loop_multi");
        if ((error = dlerror()) != NULL) {
            error("[SYSCALL] Cannot find netdata_perf_loop_multi: %s", error);
            return -1;
        }

        perf_event_mmap =  dlsym(libnetdatanv, "perf_event_mmap");
        if ((error = dlerror()) != NULL) {
            error("[SYSCALL] Cannot find perf_event_mmap: %s", error);
            fputs(error, stderr);
            return -1;
        }

        perf_event_mmap_header =  dlsym(libnetdatanv, "perf_event_mmap_header");
        if ((error = dlerror()) != NULL) {
            error("[SYSCALL] Cannot find  perf_event_mmap_header: %s", error);
            fputs(error, stderr);
            return -1;
        }
    }

    return 0;
}

int main(int argc, char **argv)
{
    (void)argc;
    (void)argv;

    //set name
    program_name = "syscall.plugin";

    //disable syslog
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;

    //Get environment variables
    user_config_dir = getenv("NETDATA_USER_CONFIG_DIR");
    stock_config_dir = getenv("NETDATA_STOCK_CONFIG_DIR");
    plugin_dir = getenv("NETDATA_PLUGINS_DIR");

    struct rlimit r = {RLIM_INFINITY, RLIM_INFINITY};
    if (setrlimit(RLIMIT_MEMLOCK, &r)) {
        perror("setrlimit(RLIMIT_MEMLOCK)");
        int_exit(1);
    }

    if(syscall_load_libraries()) {
        error("[SYSCALL] Cannot load eBPF program.");
        int_exit(2);
    }

    signal(SIGINT, int_exit);
    signal(SIGTERM, int_exit);

    if (load_bpf_file("netdata_ebpf_syscall.o")) {
        int_exit(3);
    }

    if (map_memory()) {
        int_exit(4);
    }

    if(allocate_global_vectors()) {
        error("[SYSCALL] Cannot allocate necessary vectors.");
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
        if ( ( pthread_create(&thread[i], &attr, (!i)?syscall_publisher:syscall_collector, NULL) ) ) {
            perror("");
            int_exit(0);
            return 7;
        }

    }

    for ( i = 0; i < end ; i++ ) {
        if ( (pthread_join(thread[i], NULL) ) ) {
            perror("");
            int_exit(0);
            return 7;
        }
    }

    return 0;
}
