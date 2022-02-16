// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_hardirq.h"

struct config hardirq_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

#define HARDIRQ_MAP_LATENCY 0
#define HARDIRQ_MAP_LATENCY_STATIC 1
static ebpf_local_maps_t hardirq_maps[] = {
    {
        .name = "tbl_hardirq",
        .internal_input = NETDATA_HARDIRQ_MAX_IRQS,
        .user_input = 0,
        .type = NETDATA_EBPF_MAP_STATIC,
        .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED
    },
    {
        .name = "tbl_hardirq_static",
        .internal_input = HARDIRQ_EBPF_STATIC_END,
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

#define HARDIRQ_TP_CLASS_IRQ "irq"
#define HARDIRQ_TP_CLASS_IRQ_VECTORS "irq_vectors"
static ebpf_tracepoint_t hardirq_tracepoints[] = {
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ, .event = "irq_handler_entry"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ, .event = "irq_handler_exit"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "thermal_apic_entry"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "thermal_apic_exit"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "threshold_apic_entry"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "threshold_apic_exit"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "error_apic_entry"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "error_apic_exit"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "deferred_error_apic_entry"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "deferred_error_apic_exit"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "spurious_apic_entry"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "spurious_apic_exit"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "call_function_entry"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "call_function_exit"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "call_function_single_entry"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "call_function_single_exit"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "reschedule_entry"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "reschedule_exit"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "local_timer_entry"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "local_timer_exit"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "irq_work_entry"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "irq_work_exit"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "x86_platform_ipi_entry"},
    {.enabled = false, .class = HARDIRQ_TP_CLASS_IRQ_VECTORS, .event = "x86_platform_ipi_exit"},
    /* end */
    {.enabled = false, .class = NULL, .event = NULL}
};

static hardirq_static_val_t hardirq_static_vals[] = {
    {
        .idx = HARDIRQ_EBPF_STATIC_APIC_THERMAL,
        .name = "apic_thermal",
        .latency = 0
    },
    {
        .idx = HARDIRQ_EBPF_STATIC_APIC_THRESHOLD,
        .name = "apic_threshold",
        .latency = 0
    },
    {
        .idx = HARDIRQ_EBPF_STATIC_APIC_ERROR,
        .name = "apic_error",
        .latency = 0
    },
    {
        .idx = HARDIRQ_EBPF_STATIC_APIC_DEFERRED_ERROR,
        .name = "apic_deferred_error",
        .latency = 0
    },
    {
        .idx = HARDIRQ_EBPF_STATIC_APIC_SPURIOUS,
        .name = "apic_spurious",
        .latency = 0
    },
    {
        .idx = HARDIRQ_EBPF_STATIC_FUNC_CALL,
        .name = "func_call",
        .latency = 0
    },
    {
        .idx = HARDIRQ_EBPF_STATIC_FUNC_CALL_SINGLE,
        .name = "func_call_single",
        .latency = 0
    },
    {
        .idx = HARDIRQ_EBPF_STATIC_RESCHEDULE,
        .name = "reschedule",
        .latency = 0
    },
    {
        .idx = HARDIRQ_EBPF_STATIC_LOCAL_TIMER,
        .name = "local_timer",
        .latency = 0
    },
    {
        .idx = HARDIRQ_EBPF_STATIC_IRQ_WORK,
        .name = "irq_work",
        .latency = 0
    },
    {
        .idx = HARDIRQ_EBPF_STATIC_X86_PLATFORM_IPI,
        .name = "x86_platform_ipi",
        .latency = 0
    },
};

static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;

static int read_thread_closed = 1;

// store for "published" data from the reader thread, which the collector
// thread will write to netdata agent.
static avl_tree_lock hardirq_pub;

// tmp store for dynamic hard IRQ values we get from a per-CPU eBPF map.
static hardirq_ebpf_val_t *hardirq_ebpf_vals = NULL;

// tmp store for static hard IRQ values we get from a per-CPU eBPF map.
static hardirq_ebpf_static_val_t *hardirq_ebpf_static_vals = NULL;

static struct netdata_static_thread hardirq_threads = {"HARDIRQ KERNEL",
                                                    NULL, NULL, 1, NULL,
                                                    NULL, NULL };

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void hardirq_cleanup(void *ptr)
{
    for (int i = 0; hardirq_tracepoints[i].class != NULL; i++) {
        ebpf_disable_tracepoint(&hardirq_tracepoints[i]);
    }

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

    freez(hardirq_ebpf_vals);
    freez(hardirq_ebpf_static_vals);
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

static void hardirq_read_latency_map(int mapfd)
{
    hardirq_ebpf_key_t key = {};
    hardirq_ebpf_key_t next_key = {};
    hardirq_val_t search_v = {};
    hardirq_val_t *v = NULL;

    while (bpf_map_get_next_key(mapfd, &key, &next_key) == 0) {
        // get val for this key.
        int test = bpf_map_lookup_elem(mapfd, &key, hardirq_ebpf_vals);
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
            total_latency += hardirq_ebpf_vals[i].latency/1000;

            // copy name for new IRQs.
            if (v_is_new && !name_saved && hardirq_ebpf_vals[i].name[0] != '\0') {
                strncpyz(
                    v->name,
                    hardirq_ebpf_vals[i].name,
                    NETDATA_HARDIRQ_NAME_LEN
                );
                name_saved = true;
            }
        }

        // can now safely publish latency for existing IRQs.
        v->latency = total_latency;

        // can now safely publish new IRQ.
        if (v_is_new) {
            avl_t *check = avl_insert_lock(&hardirq_pub, (avl_t *)v);
            if (check != (avl_t *)v) {
                error("Internal error, cannot insert the AVL tree.");
            }
        }

        key = next_key;
    }
}

static void hardirq_read_latency_static_map(int mapfd)
{
    uint32_t i;
    for (i = 0; i < HARDIRQ_EBPF_STATIC_END; i++) {
        uint32_t map_i = hardirq_static_vals[i].idx;
        int test = bpf_map_lookup_elem(mapfd, &map_i, hardirq_ebpf_static_vals);
        if (unlikely(test < 0)) {
            continue;
        }

        uint64_t total_latency = 0;
        int cpu_i;
        int end = (running_on_kernel < NETDATA_KERNEL_V4_15) ? 1 : ebpf_nprocs;
        for (cpu_i = 0; cpu_i < end; cpu_i++) {
            total_latency += hardirq_ebpf_static_vals[cpu_i].latency/1000;
        }

        hardirq_static_vals[i].latency = total_latency;
    }
}

/**
 * Read eBPF maps for hard IRQ.
 */
static void *hardirq_reader(void *ptr)
{
    read_thread_closed = 0;

    heartbeat_t hb;
    heartbeat_init(&hb);

    ebpf_module_t *em = (ebpf_module_t *)ptr;

    usec_t step = NETDATA_HARDIRQ_SLEEP_MS * em->update_every;
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        UNUSED(dt);

        hardirq_read_latency_map(hardirq_maps[HARDIRQ_MAP_LATENCY].map_fd);
        hardirq_read_latency_static_map(hardirq_maps[HARDIRQ_MAP_LATENCY_STATIC].map_fd);
    }

    read_thread_closed = 1;
    return NULL;
}

static void hardirq_create_charts(int update_every)
{
    ebpf_create_chart(
        NETDATA_EBPF_SYSTEM_GROUP,
        "hardirq_latency",
        "Hardware IRQ latency",
        EBPF_COMMON_DIMENSION_MILLISECONDS,
        "interrupts",
        NULL,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_CHART_PRIO_HARDIRQ_LATENCY,
        NULL, NULL, 0, update_every,
        NETDATA_EBPF_MODULE_NAME_HARDIRQ
    );

    fflush(stdout);
}

static void hardirq_create_static_dims()
{
    uint32_t i;
    for (i = 0; i < HARDIRQ_EBPF_STATIC_END; i++) {
        ebpf_write_global_dimension(
            hardirq_static_vals[i].name, hardirq_static_vals[i].name,
            ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]
        );
    }
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

static inline void hardirq_write_static_dims()
{
    uint32_t i;
    for (i = 0; i < HARDIRQ_EBPF_STATIC_END; i++) {
        write_chart_dimension(
            hardirq_static_vals[i].name,
            hardirq_static_vals[i].latency
        );
    }
}

/**
* Main loop for this collector.
*/
static void hardirq_collector(ebpf_module_t *em)
{
    hardirq_ebpf_vals = callocz(
        (running_on_kernel < NETDATA_KERNEL_V4_15) ? 1 : ebpf_nprocs,
        sizeof(hardirq_ebpf_val_t)
    );
    hardirq_ebpf_static_vals = callocz(
        (running_on_kernel < NETDATA_KERNEL_V4_15) ? 1 : ebpf_nprocs,
        sizeof(hardirq_ebpf_static_val_t)
    );

    avl_init_lock(&hardirq_pub, hardirq_val_cmp);

    // create reader thread.
    hardirq_threads.thread = mallocz(sizeof(netdata_thread_t));
    hardirq_threads.start_routine = hardirq_reader;
    netdata_thread_create(
        hardirq_threads.thread,
        hardirq_threads.name,
        NETDATA_THREAD_OPTION_JOINABLE,
        hardirq_reader,
        em
    );

    // create chart and static dims.
    pthread_mutex_lock(&lock);
    hardirq_create_charts(em->update_every);
    hardirq_create_static_dims();
    ebpf_update_stats(&plugin_statistics, em);
    pthread_mutex_unlock(&lock);

    // loop and read from published data until ebpf plugin is closed.
    int update_every = em->update_every;
    int counter = update_every - 1;
    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);

        if (++counter == update_every) {
            counter = 0;
            pthread_mutex_lock(&lock);

            // write dims now for all hitherto discovered IRQs.
            write_begin_chart(NETDATA_EBPF_SYSTEM_GROUP, "hardirq_latency");
            avl_traverse_lock(&hardirq_pub, hardirq_write_dims, NULL);
            hardirq_write_static_dims();
            write_end_chart();

            pthread_mutex_unlock(&lock);
        }

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

    if (!em->enabled) {
        goto endhardirq;
    }

    if (ebpf_enable_tracepoints(hardirq_tracepoints) == 0) {
        em->enabled = CONFIG_BOOLEAN_NO;
        goto endhardirq;
    }

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &objects);
    if (!probe_links) {
        em->enabled = CONFIG_BOOLEAN_NO;
        goto endhardirq;
    }

    hardirq_collector(em);

endhardirq:
    if (!em->enabled)
        ebpf_update_disabled_plugin_stats(em);

    netdata_thread_cleanup_pop(1);

    return NULL;
}
