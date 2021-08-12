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

// store for "published" data from the reader thread, which the collector
// thread will write to netdata agent.
static avl_tree_lock hardirq_pub;

// temporary store for hard IRQ values we get from a per-CPU eBPF map.
static hardirq_ebpf_val_t *hardirq_hash_vals = NULL;

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
static void hardirq_cleanup(void *ptr)
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
 * Compare hard IRQ values.
 *
 * @param a `hardirq_val_t *`.
 * @param b `hardirq_val_t *`.
 *
 * @return 0 if a==b, 1 if a>b, -1 if a<b.
*/
static int hardirq_val_cmp(void *a, void *b)
{
    hardirq_val_t *ptr1 = a;
    hardirq_val_t *ptr2 = b;

    if (ptr1->irq > ptr2->irq) {
        return 1;
    }
    else if (ptr1->irq < ptr2->irq) {
        return -1;
    }
    else {
        return 0;
    }
}

/**
 * Read the eBPF hash map identified by file descriptor `mapfd`.
 *
 * @param mapfd file descriptor for the eBPF hash map.
 */
static void hardirq_read_hash(int mapfd)
{
    hardirq_ebpf_key_t key = {};
    hardirq_ebpf_key_t next_key = {};
    hardirq_val_t search_v = {};
    hardirq_val_t *v = NULL;

    while (bpf_map_get_next_key(mapfd, &key, &next_key) == 0) {
        // get val for this key.
        int test = bpf_map_lookup_elem(mapfd, &key, hardirq_hash_vals);
        if (unlikely(test < 0)) {
            key = next_key;
            continue;
        }

        // is this IRQ saved yet?
        //
        // if not, make a new one, mark it as unsaved for now, and continue; we
        // will insert it at the end after all of its values are correctly set,
        // so that we can safely publish it to the collector within a single,
        // short locked operation.
        //
        // otherwise simply continue; we will only update the latency, which
        // can be republished safely without a lock.
        //
        // NOTE: lock isn't strictly necessary for this initial search, as only
        // this thread does writing, but the AVL is using a read-write lock so
        // there is no congestion.
        bool v_is_new = false;
        search_v.irq = key.irq;
        v = (hardirq_val_t *)avl_search_lock(&hardirq_pub, (avl_t *)&search_v);
        if (unlikely(v == NULL)) {
            // latency/name can only be added reliably at a later time.
            // when they're added, only then will we AVL insert.
            v = callocz(1, sizeof(hardirq_val_t));
            v->irq = key.irq;
            v->dim_exists = false;

            v_is_new = true;
        }

        // note two things:
        // 1. we must add up latency value for this IRQ across all CPUs.
        // 2. the name is unfortunately *not* available on all CPU maps - only
        //    a single map contains the name, so we must find it. we only need
        //    to copy it though if the IRQ is new for us.
        bool name_saved = false;
        uint64_t total_latency = 0;
        int i;
        int end = (running_on_kernel < NETDATA_KERNEL_V4_15) ? 1 : ebpf_nprocs;
        for (i = 0; i < end; i++) {
            total_latency += hardirq_hash_vals[i].latency/1000;

            // copy name for new IRQs.
            if (v_is_new && !name_saved && hardirq_hash_vals[i].name[0] != '\0') {
                strncpyz(
                    v->name,
                    hardirq_hash_vals[i].name,
                    NETDATA_HARDIRQ_NAME_LEN
                );
                name_saved = true;
            }
        }

        // can now safely publish latency for existing IRQs.
        v->latency = total_latency;

        // can now safely publish new IRQ.
        if (v_is_new) {
            avl_insert_lock(&hardirq_pub, (avl_t *)v);
        }

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

static void hardirq_create_charts()
{
    ebpf_create_chart(
        "system",
        "hardirq_latency",
        "Hardware IRQ latency",
        "latency (milliseconds)",
        "interrupts",
        NULL,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_CHART_PRIO_HARDIRQ_LATENCY,
        NULL, NULL, 0,
        NETDATA_EBPF_MODULE_NAME_HARDIRQ
    );

    fflush(stdout);
}

// callback for avl tree traversal on `hardirq_pub`.
static int hardirq_write_dims(void *entry, void *data)
{
    UNUSED(data);

    hardirq_val_t *v = entry;

    // IRQs get dynamically added in, so add the dimension if we haven't yet.
    if (!v->dim_exists) {
        ebpf_write_global_dimension(
            v->name, v->name,
            ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]
        );
        v->dim_exists = true;
    }

    write_chart_dimension(v->name, v->latency);

    return 1;
}

/**
* Main loop for this collector.
*/
static void hardirq_collector(ebpf_module_t *em)
{
    hardirq_hash_vals = callocz(
        (running_on_kernel < NETDATA_KERNEL_V4_15) ? 1 : ebpf_nprocs,
        sizeof(hardirq_ebpf_val_t)
    );

    avl_init_lock(&hardirq_pub, hardirq_val_cmp);

    // create reader thread.
    hardirq_threads.thread = mallocz(sizeof(netdata_thread_t));
    hardirq_threads.start_routine = ebpf_hardirq_read_hash;
    netdata_thread_create(
        hardirq_threads.thread,
        hardirq_threads.name,
        NETDATA_THREAD_OPTION_JOINABLE,
        ebpf_hardirq_read_hash,
        em
    );

    // create chart.
    pthread_mutex_lock(&lock);
    hardirq_create_charts();
    pthread_mutex_unlock(&lock);

    // loop and read from published data until ebpf plugin is closed.
    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);
        pthread_mutex_lock(&lock);

        // write dims now for all hitherto discovered IRQs.
        write_begin_chart("system", "hardirq_latency");
        avl_traverse_lock(&hardirq_pub, hardirq_write_dims, NULL);
        write_end_chart();

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
    netdata_thread_cleanup_push(hardirq_cleanup, ptr);

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
