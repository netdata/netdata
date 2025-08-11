// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_hardirq.h"

struct config hardirq_config = APPCONFIG_INITIALIZER;

static ebpf_local_maps_t hardirq_maps[] = {
    {.name = "tbl_hardirq",
     .internal_input = NETDATA_HARDIRQ_MAX_IRQS,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    },
    {.name = "tbl_hardirq_static",
     .internal_input = HARDIRQ_EBPF_STATIC_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    /* end */
    {.name = NULL,
     .internal_input = 0,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    }};

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
    {.enabled = false, .class = NULL, .event = NULL}};

static hardirq_static_val_t hardirq_static_vals[] = {
    {.idx = HARDIRQ_EBPF_STATIC_APIC_THERMAL, .name = "apic_thermal", .latency = 0},
    {.idx = HARDIRQ_EBPF_STATIC_APIC_THRESHOLD, .name = "apic_threshold", .latency = 0},
    {.idx = HARDIRQ_EBPF_STATIC_APIC_ERROR, .name = "apic_error", .latency = 0},
    {.idx = HARDIRQ_EBPF_STATIC_APIC_DEFERRED_ERROR, .name = "apic_deferred_error", .latency = 0},
    {.idx = HARDIRQ_EBPF_STATIC_APIC_SPURIOUS, .name = "apic_spurious", .latency = 0},
    {.idx = HARDIRQ_EBPF_STATIC_FUNC_CALL, .name = "func_call", .latency = 0},
    {.idx = HARDIRQ_EBPF_STATIC_FUNC_CALL_SINGLE, .name = "func_call_single", .latency = 0},
    {.idx = HARDIRQ_EBPF_STATIC_RESCHEDULE, .name = "reschedule", .latency = 0},
    {.idx = HARDIRQ_EBPF_STATIC_LOCAL_TIMER, .name = "local_timer", .latency = 0},
    {.idx = HARDIRQ_EBPF_STATIC_IRQ_WORK, .name = "irq_work", .latency = 0},
    {.idx = HARDIRQ_EBPF_STATIC_X86_PLATFORM_IPI, .name = "x86_platform_ipi", .latency = 0},
};

// store for "published" data from the reader thread, which the collector
// thread will write to netdata agent.
static avl_tree_lock hardirq_pub;

#ifdef LIBBPF_MAJOR_VERSION
/**
 * Set hash table
 *
 * Set the values for maps according the value given by kernel.
 *
 * @param obj is the main structure for bpf objects.
 */
static inline void ebpf_hardirq_set_hash_table(struct hardirq_bpf *obj)
{
    hardirq_maps[HARDIRQ_MAP_LATENCY].map_fd = bpf_map__fd(obj->maps.tbl_hardirq);
    hardirq_maps[HARDIRQ_MAP_LATENCY_STATIC].map_fd = bpf_map__fd(obj->maps.tbl_hardirq_static);
}

/**
 * Load and Attach
 *
 * Load and attach bpf software.
 */
static inline int ebpf_hardirq_load_and_attach(struct hardirq_bpf *obj)
{
    int ret = hardirq_bpf__load(obj);
    if (ret) {
        return -1;
    }

    return hardirq_bpf__attach(obj);
}
#endif

/*****************************************************************
 *
 *  ARAL SECTION
 *
 *****************************************************************/

// ARAL vectors used to speed up processing
ARAL *ebpf_aral_hardirq = NULL;

/**
 * eBPF hardirq Aral init
 *
 * Initiallize array allocator that will be used when integration with apps is enabled.
 */
static inline void ebpf_hardirq_aral_init()
{
    ebpf_aral_hardirq = ebpf_allocate_pid_aral(NETDATA_EBPF_HARDIRQ_ARAL_NAME, sizeof(hardirq_val_t));
}

/**
 * eBPF hardirq get
 *
 * Get a hardirq_val_t entry to be used with a specific IRQ.
 *
 * @return it returns the address on success.
 */
hardirq_val_t *ebpf_hardirq_get(void)
{
    hardirq_val_t *target = aral_mallocz(ebpf_aral_hardirq);
    memset(target, 0, sizeof(hardirq_val_t));
    return target;
}

/**
 * eBPF hardirq release
 *
 * @param stat Release a target after usage.
 */
void ebpf_hardirq_release(hardirq_val_t *stat)
{
    aral_freez(ebpf_aral_hardirq, stat);
}

/*****************************************************************
 *
 *  EXIT FUNCTIONS
 *
 *****************************************************************/

/**
 * Obsolete global
 *
 * Obsolete global charts created by thread.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_hardirq_global(ebpf_module_t *em)
{
    ebpf_write_chart_obsolete(
        NETDATA_EBPF_SYSTEM_GROUP,
        "hardirq_latency",
        "",
        "Hardware IRQ latency",
        EBPF_COMMON_UNITS_MILLISECONDS,
        "interrupts",
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_EBPF_SYSTEM_HARDIRQ_LATENCY_CTX,
        NETDATA_CHART_PRIO_HARDIRQ_LATENCY,
        em->update_every);
}

/**
 * Hardirq Exit
 *
 * Cancel child and exit.
 *
 * @param ptr thread data.
 */
static void hardirq_exit(void *pptr)
{
    ebpf_module_t *em = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!em)
        return;

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        netdata_mutex_lock(&lock);

        ebpf_obsolete_hardirq_global(em);

        netdata_mutex_unlock(&lock);
        fflush(stdout);
    }

    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_REMOVE);

    if (em->objects) {
        ebpf_unload_legacy_code(em->objects, em->probe_links);
        em->objects = NULL;
        em->probe_links = NULL;
    }

    for (int i = 0; hardirq_tracepoints[i].class != NULL; i++) {
        ebpf_disable_tracepoint(&hardirq_tracepoints[i]);
    }

    netdata_mutex_lock(&ebpf_exit_cleanup);
    em->enabled = NETDATA_THREAD_EBPF_STOPPED;
    ebpf_update_stats(&plugin_statistics, em);
    netdata_mutex_unlock(&ebpf_exit_cleanup);
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
    } else if (ptr1->irq < ptr2->irq) {
        return -1;
    } else {
        return 0;
    }
}

/**
 * Parse interrupts
 *
 * Parse /proc/interrupts to get names  used in metrics
 *
 * @param irq_name vector to store data.
 * @param irq      irq value
 *
 * @return It returns 0 on success and -1 otherwise
 */
static int hardirq_parse_interrupts(char *irq_name, int irq)
{
    static procfile *ff = NULL;
    static int cpus = -1;
    if (unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/interrupts");
        ff = procfile_open(filename, " \t:", PROCFILE_FLAG_DEFAULT);
    }
    if (unlikely(!ff))
        return -1;

    ff = procfile_readall(ff);
    if (unlikely(!ff))
        return -1; // we return 0, so that we will retry to open it next time

    size_t words = procfile_linewords(ff, 0);
    if (unlikely(cpus == -1)) {
        uint32_t w;
        cpus = 0;
        for (w = 0; w < words; w++) {
            if (likely(strncmp(procfile_lineword(ff, 0, w), "CPU", 3) == 0))
                cpus++;
        }
    }

    size_t lines = procfile_lines(ff), l;
    if (unlikely(!lines)) {
        collector_error("Cannot read /proc/interrupts, zero lines reported.");
        return -1;
    }

    for (l = 1; l < lines; l++) {
        words = procfile_linewords(ff, l);
        if (unlikely(!words))
            continue;
        const char *id = procfile_lineword(ff, l, 0);
        if (!isdigit(id[0]))
            continue;

        int cmp = str2i(id);
        if (cmp != irq)
            continue;

        if (unlikely((uint32_t)(cpus + 2) < words)) {
            const char *name = procfile_lineword(ff, l, words - 1);
            // On some motherboards IRQ can have the same name, so we append IRQ id to differentiate.
            snprintfz(irq_name, NETDATA_HARDIRQ_NAME_LEN - 1, "%d_%s", irq, name);
        }
    }

    return 0;
}

/**
 * Read Latency MAP
 *
 * Read data from kernel ring to user ring.
 *
 * @param mapfd hash map id.
 *
 * @return it returns 0 on success and -1 otherwise
 */
static int hardirq_read_latency_map(int mapfd)
{
    static hardirq_ebpf_static_val_t *hardirq_ebpf_vals = NULL;
    if (!hardirq_ebpf_vals)
        hardirq_ebpf_vals = callocz(ebpf_nprocs + 1, sizeof(hardirq_ebpf_static_val_t));

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
            v = ebpf_hardirq_get();
            v->irq = key.irq;
            v->dim_exists = false;

            v_is_new = true;
        }

        // note two things:
        // 1. we must add up latency value for this IRQ across all CPUs.
        // 2. the name is unfortunately *not* available on all CPU maps - only
        //    a single map contains the name, so we must find it. we only need
        //    to copy it though if the IRQ is new for us.
        uint64_t total_latency = 0;
        int i;
        for (i = 0; i < ebpf_nprocs; i++) {
            total_latency += hardirq_ebpf_vals[i].latency / 1000;
        }

        // can now safely publish latency for existing IRQs.
        v->latency = total_latency;

        // can now safely publish new IRQ.
        if (v_is_new) {
            if (hardirq_parse_interrupts(v->name, v->irq)) {
                ebpf_hardirq_release(v);
                return -1;
            }

            avl_t *check = avl_insert_lock(&hardirq_pub, (avl_t *)v);
            if (check != (avl_t *)v) {
                netdata_log_error("Internal error, cannot insert the AVL tree.");
            }
        }

        key = next_key;
    }

    return 0;
}

static void hardirq_read_latency_static_map(int mapfd)
{
    static hardirq_ebpf_static_val_t *hardirq_ebpf_static_vals = NULL;
    if (!hardirq_ebpf_static_vals)
        hardirq_ebpf_static_vals = callocz(ebpf_nprocs + 1, sizeof(hardirq_ebpf_static_val_t));

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
            total_latency += hardirq_ebpf_static_vals[cpu_i].latency / 1000;
        }

        hardirq_static_vals[i].latency = total_latency;
    }
}

/**
 * Read eBPF maps for hard IRQ.
 *
 * @return When it is not possible to parse /proc, it returns -1, on success it returns 0;
 */
static int hardirq_reader()
{
    if (hardirq_read_latency_map(hardirq_maps[HARDIRQ_MAP_LATENCY].map_fd))
        return -1;

    hardirq_read_latency_static_map(hardirq_maps[HARDIRQ_MAP_LATENCY_STATIC].map_fd);

    return 0;
}

static void hardirq_create_charts(int update_every)
{
    ebpf_create_chart(
        NETDATA_EBPF_SYSTEM_GROUP,
        "hardirq_latency",
        "Hardware IRQ latency",
        EBPF_COMMON_UNITS_MILLISECONDS,
        "interrupts",
        NETDATA_EBPF_SYSTEM_HARDIRQ_LATENCY_CTX,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_CHART_PRIO_HARDIRQ_LATENCY,
        NULL,
        NULL,
        0,
        update_every,
        NETDATA_EBPF_MODULE_NAME_HARDIRQ);

    fflush(stdout);
}

static void hardirq_create_static_dims()
{
    uint32_t i;
    for (i = 0; i < HARDIRQ_EBPF_STATIC_END; i++) {
        ebpf_write_global_dimension(
            hardirq_static_vals[i].name, hardirq_static_vals[i].name, ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
    }
}

// callback for avl tree traversal on `hardirq_pub`.
static int hardirq_write_dims(void *entry, void *data)
{
    UNUSED(data);

    hardirq_val_t *v = entry;

    // IRQs get dynamically added in, so add the dimension if we haven't yet.
    if (!v->dim_exists) {
        ebpf_write_global_dimension(v->name, v->name, ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
        v->dim_exists = true;
    }

    write_chart_dimension(v->name, v->latency);

    return 1;
}

static inline void hardirq_write_static_dims()
{
    uint32_t i;
    for (i = 0; i < HARDIRQ_EBPF_STATIC_END; i++) {
        write_chart_dimension(hardirq_static_vals[i].name, hardirq_static_vals[i].latency);
    }
}

/**
* Main loop for this collector.
 *
 * @param em the main thread structure.
*/
static void hardirq_collector(ebpf_module_t *em)
{
    memset(&hardirq_pub, 0, sizeof(hardirq_pub));
    avl_init_lock(&hardirq_pub, hardirq_val_cmp);
    ebpf_hardirq_aral_init();

    // create chart and static dims.
    netdata_mutex_lock(&lock);
    hardirq_create_charts(em->update_every);
    hardirq_create_static_dims();
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_ADD);
    netdata_mutex_unlock(&lock);

    // loop and read from published data until ebpf plugin is closed.
    int update_every = em->update_every;
    int counter = update_every - 1;
    //This will be cancelled by its parent
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        heartbeat_next(&hb);

        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        counter = 0;
        if (hardirq_reader())
            break;

        netdata_mutex_lock(&lock);

        // write dims now for all hitherto discovered IRQs.
        ebpf_write_begin_chart(NETDATA_EBPF_SYSTEM_GROUP, "hardirq_latency", "");
        avl_traverse_lock(&hardirq_pub, hardirq_write_dims, NULL);
        hardirq_write_static_dims();
        ebpf_write_end_chart();

        netdata_mutex_unlock(&lock);

        netdata_mutex_lock(&ebpf_exit_cleanup);
        if (running_time && !em->running_time)
            running_time = update_every;
        else
            running_time += update_every;

        em->running_time = running_time;
        netdata_mutex_unlock(&ebpf_exit_cleanup);
    }
}

/*****************************************************************
 *  EBPF HARDIRQ THREAD
 *****************************************************************/

/*
 * Load BPF
 *
 * Load BPF files.
 *
 * @param em the structure with configuration
 *
 * @return It returns 0 on success and -1 otherwise.
 */
static int ebpf_hardirq_load_bpf(ebpf_module_t *em)
{
    int ret = 0;
    if (em->load & EBPF_LOAD_LEGACY) {
        em->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &em->objects);
        if (!em->probe_links) {
            ret = -1;
        }
    }
#ifdef LIBBPF_MAJOR_VERSION
    else {
        hardirq_bpf_obj = hardirq_bpf__open();
        if (!hardirq_bpf_obj)
            ret = -1;
        else {
            ret = ebpf_hardirq_load_and_attach(hardirq_bpf_obj);
            if (!ret)
                ebpf_hardirq_set_hash_table(hardirq_bpf_obj);
        }
    }
#endif

    return ret;
}

/**
 * Hard IRQ latency thread.
 *
 * @param ptr a `ebpf_module_t *`.
 * @return always NULL.
 */
void ebpf_hardirq_thread(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    CLEANUP_FUNCTION_REGISTER(hardirq_exit) cleanup_ptr = em;

    em->maps = hardirq_maps;

    if (ebpf_enable_tracepoints(hardirq_tracepoints) == 0) {
        goto endhardirq;
    }

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_define_map_type(em->maps, em->maps_per_core, running_on_kernel);
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_hardirq_load_bpf(em)) {
        goto endhardirq;
    }

    hardirq_collector(em);

endhardirq:
    ebpf_update_disabled_plugin_stats(em);
}
