// SPDX-License-Identifier: GPL-3.0-or-later

#include "../../libnetdata/libnetdata.h"

#include "network_viewer_plugin.h"

//Pointer to the libraries and functions
void *libnetdatanv = NULL;
int (*load_bpf_file)(char *) = NULL;
int (*bpf_map_get_next_key)(int , const void *, void *) = NULL;
int (*bpf_map_lookup_elem)(int, const void *, void *) = NULL;
int (*bpf_map_update_elem)(int, void *, void *, uint64_t) = NULL;
int (*test_bpf_perf_event)(int) = NULL;
int (*perf_event_mmap)(int) = NULL;
int (*perf_event_mmap_header)(int, struct perf_event_mmap_page **) = NULL;
void (*my_perf_loop_multi)(int *, struct perf_event_mmap_page **, int, int *, int (*netdata_ebpf_callback)(void *, int));

//Interval used to deliver the data
static int update_every = 1;
static int freq = 0;

//perf variables
int *pmu_fd = NULL;
static struct perf_event_mmap_page *headers = NULL;

static void network_viewer_exit(int sig) {
    if (libnetdatanv) {
        dlclose(libnetdatanv);
    }

    if (pmu_fd) {
        freez(pmu_fd);
    }

    if (headers) {
        freez(headers);
    }
    exit(0);
}

static int print_bpf_output(void *data, int size)
{
    struct {
        uint64_t pid;
        uint32_t saddr;
        uint32_t daddr;
        uint16_t dport;
        uint16_t retransmit;
        uint32_t sent;
        uint64_t recv;
        uint8_t protocol;
    } *e = data;

    if (e) {
        //MUST STORE THIS INSIDE AN AVL AND CASE IT IS 256 I DISPATCH TO THE CLOUD
    }

    //return LIBBPF_PERF_EVENT_CONT;
    return -2;
}

int allocate_memory( ) {
    size_t nprocs = sysconf(_SC_NPROCESSORS_ONLN);
    size_t i;

    if (nprocs < NETDATA_MAX_PROCESSOR) {
        nprocs = NETDATA_MAX_PROCESSOR;
    }

    pmu_fd = callocz(nprocs, sizeof(int));
    if (!pmu_fd) {
        return -1;
    }

    for (i = 0; i < nprocs; i++) {
        pmu_fd[i] = test_bpf_perf_event(i);

        if (perf_event_mmap(pmu_fd[i]) < 0) {
            return -1;
        }
    }

    static struct perf_event_mmap_page *headers = NULL;
    headers = callocz(nprocs, sizeof());
    if (!headers) {
        return -1;
    }

    for (i = 0; i < nprocs; i++) {
        if (perf_event_mmap_header(pmu_fd[i], &headers[i]) < 0) {
            return -1;
        }
    }

    return 0;
}

static int network_viewer_load_libraries() {
    char *error = NULL;

    libbpf = dlopen("./libnetdata_network_viewer.so",RTLD_LAZY);
    if (!libbpf) {
        error("[NETWORK VIEWER] : Cannot load the library libnetdata_network_viewer.so");
        return -1;
    } else {
        load_bpf_file = dlsym(libbpf, "load_bpf_file");
        if ((error = dlerror()) != NULL) {
            goto comm_load_err;
        }

        test_bpf_perf_event = dlsym(libbpf, "test_bpf_perf_event");
        if ((error = dlerror()) != NULL) {
            goto comm_load_err;
        }

        bpf_map_get_next_key = dlsym(libbpf, "bpf_map_get_next_key");
        if ((error = dlerror()) != NULL) {
            goto comm_load_err;
        }

        bpf_map_lookup_elem = dlsym(libbpf, "bpf_map_lookup_elem");
        if ((error = dlerror()) != NULL) {
            goto comm_load_err;
        }

        bpf_map_update_elem = dlsym(libbpf, "bpf_map_update_elem");
        if ((error = dlerror()) != NULL) {
            goto comm_load_err;
        }

        netdata_perf_loop_multi = dlsym(libbpf, "my_perf_loop_multi");
        if ((error = dlerror()) != NULL) {
            goto comm_load_err;
        }

        perf_event_mmap =  dlsym(libbpf, "perf_event_mmap");
        if ((error = dlerror()) != NULL) {
            goto comm_load_err;
        }

        perf_event_mmap_header =  dlsym(libbpf, "perf_event_mmap_header");
        if ((error = dlerror()) != NULL) {
            goto comm_load_err;
        }
    }

    return 0;

comm_load_err:
    error("[NETWORK VIEWER] : %s", error);
    return -1;
}

int main(int argc, char **argv) {
    program_name = "network_viewer";

    //parse input here

    if (freq >= update_every)
        update_every = freq;

    if (network_viewer_load_libraries()) {
        return 1;
    }

    //We are adjusting the limit, because we are not creating limits
    //to the number of connections we are monitoring.
    struct rlimit r = {RLIM_INFINITY, RLIM_INFINITY};
    if (setrlimit(RLIMIT_MEMLOCK, &r)) {
        error("[NETWORK VIEWER]: %s", strerror(errno));
        return 2;
    }

    signal(SIGTERM, network_viewer_exit);

    if (load_bpf_file("netdata_ebpf_network_viewer.o")) {
        error("[NETWORK VIEWER]: Cannot load the eBPF program.")
        return 3;
    }

    error_log_syslog = 0;

    //pass netdata_exit to my_perf_loop_multi

    heartbeat_t hb;
    heartbeat_init(&hb);

    usec_t step = update_every * USEC_PER_SEC;
    for (;;) {
        usec_t dt = heartbeat_next(&hb, step);

        if(unlikely(netdata_exit))
            break;
    }

    //I HAVE TO CREATE TWO THREADS, ONE TO PROCESS AND OTHER TO SEND THE MESSAGES
    //ONE UNIQUE WON'T BE ABLE TO DO EVERYTHING

    network_viewer_exit(SIGTERM);
}
