// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_sync.h"

static ebpf_data_t sync_data;

static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;


static char *sync_counter_dimension_name[NETDATA_SYNC_END] = { "sync" };
static netdata_syscall_stat_t sync_counter_aggregated_data;
static netdata_publish_syscall_t sync_counter_publish_aggregated;

/*****************************************************************
 *
 *  CLEANUP THREAD
 *
 *****************************************************************/

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_sync_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled)
        return;

    struct bpf_program *prog;
    size_t i = 0 ;
    bpf_object__for_each_program(prog, objects) {
        bpf_link__destroy(probe_links[i]);
        i++;
    }
    bpf_object__close(objects);
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/**
 * Cachestat thread
 *
 * Thread used to make sync thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_sync_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_sync_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    fill_ebpf_data(&sync_data);

    if (!em->enabled)
        goto endsync;

    if (ebpf_update_kernel(&sync_data)) {
        pthread_mutex_unlock(&lock);
        goto endsync;
    }

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects, sync_data.map_fd);
    if (!probe_links) {
        pthread_mutex_unlock(&lock);
        goto endsync;
    }

    int algorithm = NETDATA_EBPF_INCREMENTAL_IDX;
    ebpf_global_labels(&sync_counter_aggregated_data, &sync_counter_publish_aggregated,
                       sync_counter_dimension_name, sync_counter_dimension_name,
                       &algorithm, NETDATA_SYNC_END);

endsync:
    netdata_thread_cleanup_pop(1);
    return NULL;
}