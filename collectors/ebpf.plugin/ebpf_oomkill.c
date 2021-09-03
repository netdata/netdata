// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_oomkill.h"

struct config oomkill_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

#define OOMKILL_MAP_KILLCNT 0
static ebpf_local_maps_t oomkill_maps[] = {
    {
        .name = "tbl_oomkill",
        .internal_input = NETDATA_OOMKILL_MAX_ENTRIES,
        .user_input = 0,
        .type = NETDATA_EBPF_MAP_STATIC,
        .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED
    },
    /* end */
    {
        .name = NULL,
        .internal_input = 0,
        .user_input = 0,
        .type = NETDATA_EBPF_MAP_CONTROLLER,
        .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED
    }
};

static ebpf_data_t oomkill_data;

static ebpf_tracepoint_t oomkill_tracepoints[] = {
    {.enabled = false, .class = "oom", .event = "mark_victim"},
    /* end */
    {.enabled = false, .class = NULL, .event = NULL}
};

static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;

// tmp store for oomkill values we get from a per-CPU eBPF map.
static oomkill_ebpf_val_t *oomkill_ebpf_vals = NULL;

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void oomkill_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled) {
        return;
    }

    freez(oomkill_ebpf_vals);

    if (probe_links) {
        struct bpf_program *prog;
        size_t i = 0 ;
        bpf_object__for_each_program(prog, objects) {
            bpf_link__destroy(probe_links[i]);
            i++;
        }
        bpf_object__close(objects);
    }
}

static void oomkill_write_data()
{
    uint32_t curr_key = 0;
    uint32_t key = 0;
    int mapfd = oomkill_maps[OOMKILL_MAP_KILLCNT].map_fd;
    while (bpf_map_get_next_key(mapfd, &curr_key, &key) == 0) {
        curr_key = key;

        int test;

        // get val for this key.
        test = bpf_map_lookup_elem(mapfd, &key, oomkill_ebpf_vals);
        if (unlikely(test < 0)) {
            continue;
        }

        // now delete it, since we have a user-space copy, and never need this
        // entry again. there's no race here, because the PID will only get OOM
        // killed once.
        test = bpf_map_delete_elem(mapfd, &key);
        if (unlikely(test < 0)) {
            // since there's only 1 thread doing these deletions, it should be
            // impossible to get this condition.
            error("key unexpectedly not available for deletion.");
        }

        // get command name from PID.
        char comm[MAX_COMPARE_NAME+1];
        test = get_pid_comm(key, sizeof(comm), comm);
        if (test == -1) {
            continue;
        }

        // write dim.
        ebpf_write_global_dimension(
            comm, comm,
            ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]
        );
        write_chart_dimension(comm, 1);
    }
}

static void oomkill_create_charts()
{
    ebpf_create_chart(
        NETDATA_EBPF_MEMORY_GROUP,
        "oomkills",
        "OOM kills",
        EBPF_COMMON_DIMENSION_KILLS,
        "system",
        NULL,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_MEM_SYSTEM_OOM_KILL_PROC,
        NULL, NULL, 0,
        NETDATA_EBPF_MODULE_NAME_OOMKILL
    );

    fflush(stdout);
}

/**
* Main loop for this collector.
*/
static void oomkill_collector(ebpf_module_t *em)
{
    UNUSED(em);

    oomkill_ebpf_vals = callocz(
        (running_on_kernel < NETDATA_KERNEL_V4_15) ? 1 : ebpf_nprocs,
        sizeof(oomkill_ebpf_val_t)
    );

    // create chart.
    pthread_mutex_lock(&lock);
    oomkill_create_charts();
    pthread_mutex_unlock(&lock);

    // loop and read until ebpf plugin is closed.
    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);
        pthread_mutex_lock(&lock);

        // write everything from the ebpf map.
        write_begin_chart(NETDATA_EBPF_MEMORY_GROUP, "oomkills");
        oomkill_write_data();
        write_end_chart();

        pthread_mutex_unlock(&lock);
        pthread_mutex_unlock(&collect_data_mutex);
    }
}

/**
 * OOM kill tracking thread.
 *
 * @param ptr a `ebpf_module_t *`.
 * @return always NULL.
 */
void *ebpf_oomkill_thread(void *ptr)
{
    netdata_thread_cleanup_push(oomkill_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = oomkill_maps;

    fill_ebpf_data(&oomkill_data);

    if (!em->enabled) {
        goto endoomkill;
    }

    if (ebpf_update_kernel(&oomkill_data)) {
        goto endoomkill;
    }

    if (ebpf_enable_tracepoints(oomkill_tracepoints) == 0) {
        em->enabled = CONFIG_BOOLEAN_NO;
        goto endoomkill;
    }

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects, oomkill_data.map_fd);
    if (!probe_links) {
        goto endoomkill;
    }

    oomkill_collector(em);

endoomkill:
    netdata_thread_cleanup_pop(1);

    return NULL;
}
