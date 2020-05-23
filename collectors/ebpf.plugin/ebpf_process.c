// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/time.h>
#include <sys/resource.h>

#include "ebpf.h"

/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

static netdata_idx_t *hash_values = NULL;
static netdata_syscall_stat_t *aggregated_data = NULL;
static netdata_publish_syscall_t *publish_aggregated = NULL;

ebpf_functions_t functions;


/**
 * Pointers used when collector is dynamically linked
 */
void *libnetdata = NULL;
int (*load_bpf_file)(char *, int) = NULL;
int (*set_bpf_perf_event)(int, int) = NULL;
//Libbpf (It is necessary to have at least kernel 4.10)
int (*bpf_map_lookup_elem)(int, const void *, void *);

int *map_fd = NULL;
/**
 * End of the pointers
 */

/*****************************************************************
 *
 *  PROCESS DATA AND SEND TO NETDATA
 *
 *****************************************************************/

/*****************************************************************
 *
 *  READ INFORMATION FROM KERNEL RING
 *
 *****************************************************************/

static void read_hash_global_tables()
{
    uint64_t idx;
    netdata_idx_t res[NETDATA_GLOBAL_VECTOR];

    netdata_idx_t *val = hash_values;
    for (idx = 0; idx < NETDATA_GLOBAL_VECTOR; idx++) {
        if(!bpf_map_lookup_elem(map_fd[1], &idx, val)) {
            uint64_t total = 0;
            int i;
            int end = (running_on_kernel < NETDATA_KERNEL_V4_15)?1:ebpf_nprocs;
            for (i = 0; i < end; i++)
                total += val[i];

            res[idx] = total;
        } else {
            res[idx] = 0;
        }
    }

    aggregated_data[0].call = res[NETDATA_KEY_CALLS_DO_SYS_OPEN];
    aggregated_data[1].call = res[NETDATA_KEY_CALLS_CLOSE_FD];
    aggregated_data[2].call = res[NETDATA_KEY_CALLS_VFS_UNLINK];
    aggregated_data[3].call = res[NETDATA_KEY_CALLS_VFS_READ] + res[NETDATA_KEY_CALLS_VFS_READV];
    aggregated_data[4].call = res[NETDATA_KEY_CALLS_VFS_WRITE] + res[NETDATA_KEY_CALLS_VFS_WRITEV];
    aggregated_data[5].call = res[NETDATA_KEY_CALLS_DO_EXIT];
    aggregated_data[6].call = res[NETDATA_KEY_CALLS_RELEASE_TASK];
    aggregated_data[7].call = res[NETDATA_KEY_CALLS_DO_FORK];
    aggregated_data[8].call = res[NETDATA_KEY_CALLS_SYS_CLONE];

    aggregated_data[0].ecall = res[NETDATA_KEY_ERROR_DO_SYS_OPEN];
    aggregated_data[1].ecall = res[NETDATA_KEY_ERROR_CLOSE_FD];
    aggregated_data[2].ecall = res[NETDATA_KEY_ERROR_VFS_UNLINK];
    aggregated_data[3].ecall = res[NETDATA_KEY_ERROR_VFS_READ] + res[NETDATA_KEY_ERROR_VFS_READV];
    aggregated_data[4].ecall = res[NETDATA_KEY_ERROR_VFS_WRITE] + res[NETDATA_KEY_ERROR_VFS_WRITEV];
    aggregated_data[7].ecall = res[NETDATA_KEY_ERROR_DO_FORK];
    aggregated_data[8].ecall = res[NETDATA_KEY_ERROR_SYS_CLONE];

    aggregated_data[2].bytes = (uint64_t)res[NETDATA_KEY_BYTES_VFS_WRITE] + (uint64_t)res[NETDATA_KEY_BYTES_VFS_WRITEV];
    aggregated_data[3].bytes = (uint64_t)res[NETDATA_KEY_BYTES_VFS_READ] + (uint64_t)res[NETDATA_KEY_BYTES_VFS_READV];
}

/**
 * Main loop for this collector.
 *
 * @param step number of microseconds used with heart beat
 */
static void process_collector(usec_t step)
{
    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_hash_global_tables();
    }
}

/*****************************************************************
 *
 *  FUNCTIONS RELATED TO END OF THE THREAD
 *
 *****************************************************************/

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_process_cleanup(void *ptr)
{
    freez(aggregated_data);
    freez(publish_aggregated);
    freez(hash_values);
}

/*****************************************************************
 *
 *  FUNCTIONS RELATED TO START OF THE THREAD
 *
 *****************************************************************/

/**
 * Allocate vectors used with this thread.
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 */
void ebpf_process_allocate_global_vectors() {
    aggregated_data = callocz(NETDATA_MAX_MONITOR_VECTOR, sizeof(netdata_syscall_stat_t));
    publish_aggregated = callocz(NETDATA_MAX_MONITOR_VECTOR, sizeof(netdata_publish_syscall_t));
    hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));
}

/**
 * Process thread
 *
 * Thread used to generate process charts.
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_process_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_process_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t)ptr;

    ebpf_process_allocate_global_vectors();

    process_collector((usec_t)(em->update_time*USEC_PER_SEC);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
