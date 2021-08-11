// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_hardirq.h"

struct config hardirq_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

#define HARDIRQ_MAP_LATENCY 0
static ebpf_local_maps_t hardirq_maps[] = {{.name = "tbl_hardirq", .internal_input = NETDATA_HARDIRQ_MAX_IRQS,
                                         .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                        {.name = NULL, .internal_input = 0, .user_input = 0,
                                         .type = NETDATA_EBPF_MAP_CONTROLLER,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED}};

static ebpf_data_t hardirq_data;

char *tracepoint_irq_type = { "irq" } ;
char *tracepoint_irq_entry = { "irq_handler_entry" };
char *tracepoint_irq_exit = { "irq_handler_exit" };
static int was_irq_entry_enabled = 0;
static int was_irq_exit_enabled = 0;

static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;

static int read_thread_closed = 1;

static hardirq_val_t *hardirq_hash_vals = NULL;
static struct netdata_static_thread hardirq_threads = {"HARDIRQ KERNEL",
                                                    NULL, NULL, 1, NULL,
                                                    NULL, NULL };

/**
 * Enable hard IRQ tracepoints needed for eBPF.
 *
 * @return 0 on success and -1 otherwise.
 */
static int ebpf_hardirq_enable_tracepoints()
{
    int test = ebpf_is_tracepoint_enabled(
        tracepoint_irq_type,
        tracepoint_irq_entry
    );
    if (test == -1) {
        return -1;
    }
    else if (!test) {
        if (ebpf_enable_tracing_values(
            tracepoint_irq_type,
            tracepoint_irq_entry
        )) {
            return -1;
        }
    }
    was_irq_entry_enabled = test;

    test = ebpf_is_tracepoint_enabled(
        tracepoint_irq_type,
        tracepoint_irq_exit
    );
    if (test == -1) {
        return -1;
    } else if (!test) {
        if (ebpf_enable_tracing_values(
            tracepoint_irq_type,
            tracepoint_irq_exit
        )) {
            return -1;
        }
    }
    was_irq_exit_enabled = test;

    return 0;
}

/**
 * Disable hard IRQ tracepoints needed for eBPF.
 */
static void ebpf_hardirq_disable_tracepoints()
{
    char *default_message = { "Cannot disable the tracepoint" };
    if (!was_irq_entry_enabled) {
        if (ebpf_disable_tracing_values(tracepoint_irq_type, tracepoint_irq_entry))
            error("%s %s/%s.", default_message, tracepoint_irq_type, tracepoint_irq_entry);
    }

    if (!was_irq_exit_enabled) {
        if (ebpf_disable_tracing_values(tracepoint_irq_type, tracepoint_irq_exit))
            error("%s %s/%s.", default_message, tracepoint_irq_type, tracepoint_irq_exit);
    }
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_hardirq_cleanup(void *ptr)
{
    ebpf_hardirq_disable_tracepoints();

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled) {
        return;
    }

    heartbeat_t hb;
    heartbeat_init(&hb);
    uint32_t tick = 1 * USEC_PER_MS;
    while (!read_thread_closed) {
        usec_t dt = heartbeat_next(&hb, tick);
        UNUSED(dt);
    }

    freez(hardirq_hash_vals);
    freez(hardirq_threads.thread);

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

/*****************************************************************
 *  MAIN LOOP
 *****************************************************************/

/**
 * Read the eBPF hash map identified by file descriptor `mapfd`.
 *
 * @param mapfd file descriptor for the eBPF hash map.
 */
static void hardirq_read_hash(int mapfd)
{
    hardirq_key_t key = {};
    hardirq_key_t next_key = {};

    while (bpf_map_get_next_key(mapfd, &key, &next_key) == 0) {
        // get val for this key.
        int test = bpf_map_lookup_elem(mapfd, &key, hardirq_hash_vals);
        if (unlikely(test < 0)) {
            key = next_key;
            continue;
        }

        // add up latency value for this IRQ across CPUs (or just 1).
        uint64_t total_latency_across_cpus = 0;
        int i;
        int end = (running_on_kernel < NETDATA_KERNEL_V4_15) ? 1 : ebpf_nprocs;
        for (i = 0; i < end; i++) {
            total_latency_across_cpus += hardirq_hash_vals[i].latency;
        }

        // TODO publish combined data to collector thread.

        key = next_key;
    }
}

/**
 * Read eBPF maps for hard IRQ.
 */
void *ebpf_hardirq_read_hash(void *ptr)
{
    read_thread_closed = 0;

    heartbeat_t hb;
    heartbeat_init(&hb);

    ebpf_module_t *em = (ebpf_module_t *)ptr;

    usec_t step = NETDATA_HARDIRQ_SLEEP_MS * em->update_time;
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        UNUSED(dt);

        hardirq_read_hash(hardirq_maps[HARDIRQ_MAP_LATENCY].map_fd);
    }

    read_thread_closed = 1;
    return NULL;
}

/**
* Main loop for this collector.
*/
static void hardirq_collector(ebpf_module_t *em)
{
    hardirq_hash_vals = callocz(
        (running_on_kernel < NETDATA_KERNEL_V4_15) ? 1 : ebpf_nprocs,
        sizeof(hardirq_val_t)
    );

    hardirq_threads.thread = mallocz(sizeof(netdata_thread_t));
    hardirq_threads.start_routine = ebpf_hardirq_read_hash;
    netdata_thread_create(
        hardirq_threads.thread,
        hardirq_threads.name,
        NETDATA_THREAD_OPTION_JOINABLE,
        ebpf_hardirq_read_hash,
        em
    );

    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);
        pthread_mutex_lock(&lock);

        // TODO send data from published result.

        pthread_mutex_unlock(&lock);
        pthread_mutex_unlock(&collect_data_mutex);
    }
}

/*****************************************************************
 *  EBPF HARDIRQ THREAD
 *****************************************************************/

/**
 * Hard IRQ latency thread.
 *
 * @param ptr a `ebpf_module_t *`.
 * @return always NULL.
 */
void *ebpf_hardirq_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_hardirq_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = hardirq_maps;

    fill_ebpf_data(&hardirq_data);

    if (!em->enabled) {
        goto endhardirq;
    }

    if (ebpf_update_kernel(&hardirq_data)) {
        goto endhardirq;
    }

    if (ebpf_hardirq_enable_tracepoints()) {
        em->enabled = CONFIG_BOOLEAN_NO;
        goto endhardirq;
    }

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects, hardirq_data.map_fd);
    if (!probe_links) {
        goto endhardirq;
    }

    hardirq_collector(em);

endhardirq:
    netdata_thread_cleanup_pop(1);

    return NULL;
}
