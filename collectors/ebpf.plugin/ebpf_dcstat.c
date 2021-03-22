// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_dcstat.h"

static char *dcstat_counter_dimension_name[NETDATA_DCSTAT_IDX_END] = { "ratio", "reference", "slow", "miss" };
static netdata_syscall_stat_t dcstat_counter_aggregated_data[NETDATA_DCSTAT_IDX_END];
static netdata_publish_syscall_t dcstat_counter_publish_aggregated[NETDATA_DCSTAT_IDX_END];

static ebpf_data_t dcstat_data;

static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;

struct config dcstat_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_dcstat_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled)
        return;

    ebpf_cleanup_publish_syscall(dcstat_counter_publish_aggregated);

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
void ebpf_dcstat_create_apps_charts(struct ebpf_module *em, void *ptr)
{
    UNUSED(em);
    UNUSED(ptr);
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/**
 * Cachestat thread
 *
 * Thread used to make dcstat thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_dcstat_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_dcstat_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    fill_ebpf_data(&dcstat_data);

    ebpf_update_module(em, &dcstat_config, NETDATA_DIRECTORY_DCSTAT_CONFIG_FILE);

    if (!em->enabled)
        goto enddcstat;

    pthread_mutex_lock(&lock);

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects, dcstat_data.map_fd);
    if (!probe_links) {
        pthread_mutex_unlock(&lock);
        goto enddcstat;
    }

    int algorithms[NETDATA_DCSTAT_IDX_END] = {
        NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX
    };

    ebpf_global_labels(dcstat_counter_aggregated_data, dcstat_counter_publish_aggregated,
                       dcstat_counter_dimension_name, dcstat_counter_dimension_name,
                       algorithms, NETDATA_DCSTAT_IDX_END);


    pthread_mutex_unlock(&lock);

enddcstat:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
