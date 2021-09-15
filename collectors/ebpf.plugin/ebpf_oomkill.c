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
    // the first `i` entries of `keys` will contain the currently active PIDs
    // in the eBPF map.
    uint32_t i = 0;
    int32_t keys[NETDATA_OOMKILL_MAX_ENTRIES] = {0};

    uint32_t curr_key = 0;
    uint32_t key = 0;
    int mapfd = oomkill_maps[OOMKILL_MAP_KILLCNT].map_fd;
    while (bpf_map_get_next_key(mapfd, &curr_key, &key) == 0) {
        curr_key = key;

        keys[i] = key;
        i += 1;

        // delete this key now that we've recorded its existence. there's no
        // race here, as the same PID will only get OOM killed once.
        int test = bpf_map_delete_elem(mapfd, &key);
        if (unlikely(test < 0)) {
            // since there's only 1 thread doing these deletions, it should be
            // impossible to get this condition.
            error("key unexpectedly not available for deletion.");
        }
    }

    // for each app, see if it was OOM killed. record as 1 if so otherwise 0.
    struct target *w;
    for (w = apps_groups_root_target; w != NULL; w = w->next) {
        if (likely(w->exposed && w->processes)) {
            bool was_oomkilled = false;
            struct pid_on_target *pids = w->root_pid;
            while (pids) {
                uint32_t j;
                for (j = 0; j < i; j++) {
                    if (pids->pid == keys[j]) {
                        was_oomkilled = true;
                        // set to 0 so we consider it "done".
                        keys[j] = 0;
                        goto write_dim;
                    }
                }
                pids = pids->next;
            }

        write_dim:;
            write_chart_dimension(w->name, was_oomkilled);
        }
    }

    // for any remaining keys for which we couldn't find a group, this could be
    // for various reasons, but the primary one is that the PID has not yet
    // been picked up by the process thread when parsing the proc filesystem.
    // since it's been OOM killed, it will never be parsed in the future, so
    // we have no choice but to dump it into `other`.
    uint32_t j;
    uint32_t rem_count = 0;
    for (j = 0; j < i; j++) {
        int32_t key = keys[j];
        if (key != 0) {
            rem_count += 1;
        }
    }
    if (rem_count > 0) {
        write_chart_dimension("other", rem_count);
    }
}

/**
* Main loop for this collector.
*/
static void oomkill_collector(ebpf_module_t *em)
{
    UNUSED(em);

    // loop and read until ebpf plugin is closed.
    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);
        pthread_mutex_lock(&lock);

        // write everything from the ebpf map.
        write_begin_chart(NETDATA_APPS_FAMILY, "oomkills");
        oomkill_write_data();
        write_end_chart();

        pthread_mutex_unlock(&lock);
        pthread_mutex_unlock(&collect_data_mutex);
    }
}

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_oomkill_create_apps_charts(struct ebpf_module *em, void *ptr)
{
    UNUSED(em);

    struct target *root = ptr;
    ebpf_create_charts_on_apps("oomkills",
                               "OOM kills",
                               EBPF_COMMON_DIMENSION_KILLS,
                               "mem",
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20020,
                               ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX],
                               root, NETDATA_EBPF_MODULE_NAME_OOMKILL);
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
