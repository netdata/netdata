// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_cachestat.h"

static ebpf_data_t cachestat_data;
netdata_publish_cachestat_t **cachestat_pid;

static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;

static char *cachestat_counter_dimension_name[NETDATA_CACHESTAT_END] = { "ratio", "dirty", "hit",
                                                                         "miss" };
static netdata_syscall_stat_t *cachestat_counter_aggregated_data = NULL;
static netdata_publish_syscall_t *cachestat_counter_publish_aggregated = NULL;

netdata_cachestat_pid_t *cachestat_vector = NULL;

static netdata_idx_t *cachestat_hash_values = NULL;

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

/**
 * Clean PID structures
 *
 * Clean the allocated structures.
 */
static void clean_pid_structures() {
    struct pid_stat *pids = root_of_pids;
    while (pids) {
        freez(cachestat_pid[pids->pid]);

        pids = pids->next;
    }
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_cachestat_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled)
        return;

    clean_pid_structures();
    freez(cachestat_pid);

    freez(cachestat_counter_aggregated_data);
    ebpf_cleanup_publish_syscall(cachestat_counter_publish_aggregated);
    freez(cachestat_counter_publish_aggregated);

    freez(cachestat_vector);
    freez(cachestat_hash_values);

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
 *  APPS
 *
 *****************************************************************/

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_cachestat_create_apps_charts(struct ebpf_module *em, void *ptr)
{
    UNUSED(em);
}

/*****************************************************************
 *
 *  INITIALIZE THREAD
 *
 *****************************************************************/

/**
 * Allocate vectors used with this thread.
 *
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 *
 * @param length is the length for the vectors used inside the collector.
 */
static void ebpf_cachestat_allocate_global_vectors(size_t length)
{
    cachestat_pid = callocz((size_t)pid_max, sizeof(netdata_publish_cachestat_t *));
    cachestat_vector = callocz((size_t)ebpf_nprocs, sizeof(netdata_cachestat_pid_t));

    cachestat_hash_values = callocz(length, sizeof(netdata_idx_t));

    cachestat_counter_aggregated_data = callocz(length, sizeof(netdata_syscall_stat_t));
    cachestat_counter_publish_aggregated = callocz(length, sizeof(netdata_publish_syscall_t));
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/**
 * Cachestat thread
 *
 * Thread used to make cachestat thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_cachestat_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_cachestat_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    fill_ebpf_data(&cachestat_data);

    if (!em->enabled)
        goto endcachestat;

    pthread_mutex_lock(&lock);
    ebpf_cachestat_allocate_global_vectors(NETDATA_CACHESTAT_END);
    if (ebpf_update_kernel(&cachestat_data)) {
        pthread_mutex_unlock(&lock);
        goto endcachestat;
    }

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects, cachestat_data.map_fd);
    if (!probe_links) {
        pthread_mutex_unlock(&lock);
        goto endcachestat;
    }

    int algorithms[NETDATA_CACHESTAT_END] = {
        NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX
    };

    ebpf_global_labels(cachestat_counter_aggregated_data, cachestat_counter_publish_aggregated,
                       cachestat_counter_dimension_name, cachestat_counter_dimension_name,
                       algorithms, NETDATA_CACHESTAT_END);

    pthread_mutex_unlock(&lock);

endcachestat:
    netdata_thread_cleanup_pop(1);
    return NULL;
}