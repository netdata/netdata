// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_hardirq.h"
#include "libbpf_api/ebpf_library.h"

struct config hardirq_config = APPCONFIG_INITIALIZER;

static char *hardirq_counter_dimension_name[NETDATA_HARDIRQ_DIMENSION] = {"latency"};

static netdata_syscall_stat_t hardirq_counter_aggregated_data[NETDATA_HARDIRQ_DIMENSION];
static netdata_publish_syscall_t hardirq_counter_publish_aggregated[NETDATA_HARDIRQ_DIMENSION];

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

static bool hardirq_safe_clean = false;

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
 *
 * @param obj is the main structure for bpf objects.
 * @param em  structure with configuration
 *
 * @return It returns 0 on success and -1 otherwise.
 */
static inline int ebpf_hardirq_load_and_attach(struct hardirq_bpf *obj)
{
    int ret = hardirq_bpf__load(obj);
    if (ret) {
        return -1;
    }

    ret = hardirq_bpf__attach(obj);
    if (ret) {
        return -1;
    }

    ebpf_hardirq_set_hash_table(obj);

    return 0;
}
#endif

/*****************************************************************
 *
 *  JudyL SECTION
 *
 *****************************************************************/

static Pvoid_t ebpf_hardirq_JudyL = NULL;

/**
 * eBPF hardirq get
 *
 * Get a hardirq_val_t entry to be used with a specific IRQ.
 *
 * @return it returns the address on success.
 */
hardirq_val_t *ebpf_hardirq_get(int irq)
{
    Pvoid_t *PValue = JudyLGet(ebpf_hardirq_JudyL, (Word_t)irq, PJE0);
    if (PValue && *PValue)
        return *PValue;

    JError_t J_Error;
    PValue = JudyLIns(&ebpf_hardirq_JudyL, (Word_t)irq, &J_Error);
    if (unlikely(PValue == PJERR)) {
        netdata_log_error(
            "Cannot insert IRQ %d to JudyL, JU_ERRNO_* == %u, ID == %d", irq, JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
        return NULL;
    }

    if (unlikely(!PValue)) {
        netdata_log_error("JudyLIns returned NULL for IRQ %d", irq);
        return NULL;
    }

    hardirq_val_t *target = callocz(1, sizeof(hardirq_val_t));
    target->irq = irq;
    *PValue = target;

    return target;
}

/**
 * eBPF hardirq release
 *
 * @param irq IRQ number to release.
 */
void ebpf_hardirq_release(int irq)
{
    Pvoid_t *PValue = JudyLGet(ebpf_hardirq_JudyL, (Word_t)irq, PJE0);
    if (PValue && *PValue) {
        freez(*PValue);
        JudyLDel(&ebpf_hardirq_JudyL, (Word_t)irq, PJE0);
    }
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
static void hardirq_cleanup(void *pptr)
{
    return;
    ebpf_module_t *em = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!em)
        return;

    if (ebpf_module_enabled_get(em) == NETDATA_THREAD_EBPF_FUNCTION_RUNNING && !ebpf_plugin_stop()) {
        netdata_mutex_lock(&lock);

        if (hardirq_safe_clean)
            ebpf_obsolete_hardirq_global(em);

        netdata_mutex_unlock(&lock);
        fflush(stdout);
    }

    if (!hardirq_safe_clean) {
        netdata_mutex_lock(&ebpf_exit_cleanup);
        ebpf_module_enabled_set(em, NETDATA_THREAD_EBPF_STOPPED);
        netdata_mutex_unlock(&ebpf_exit_cleanup);
        return;
    }

    if (!ebpf_plugin_stop()) {
        if ((em->load & EBPF_LOAD_LEGACY) && em->probe_links) {
            ebpf_unload_legacy_code(em->objects, em->probe_links);
            em->objects = NULL;
            em->probe_links = NULL;
        }
#ifdef LIBBPF_MAJOR_VERSION
        else if (hardirq_bpf_obj) {
            //hardirq_bpf__destroy(hardirq_bpf_obj);
            hardirq_bpf_obj = NULL;
        }
#endif
    }

    /*
    if (unlikely(ebpf_hardirq_JudyL)) {
        Word_t index = 0;
        Pvoid_t *PValue;
        for (PValue = JudyLFirst(ebpf_hardirq_JudyL, &index, PJE0); PValue != NULL && PValue != PJERR;
             PValue = JudyLNext(ebpf_hardirq_JudyL, &index, PJE0)) {
            hardirq_val_t *v = *PValue;
            if (v)
                freez(v);
        }
        JudyLFreeArray(&ebpf_hardirq_JudyL, PJE0);
        ebpf_hardirq_JudyL = NULL;
    }
    */

    for (int i = 0; hardirq_tracepoints[i].class != NULL; i++) {
        ebpf_disable_tracepoint(&hardirq_tracepoints[i]);
    }

    netdata_mutex_lock(&ebpf_exit_cleanup);
    ebpf_module_enabled_set(em, NETDATA_THREAD_EBPF_STOPPED);
    netdata_mutex_unlock(&ebpf_exit_cleanup);
}

/*****************************************************************
 *  MAIN LOOP
 *****************************************************************/

/**
 * Parse interrupts
 *
 * Parse /proc/interrupts to get names used in metrics
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
        return -1;

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
            snprintfz(irq_name, NETDATA_HARDIRQ_NAME_LEN - 1, "%d_%s", irq, name);
        }
    }

    return 0;
}

/**
 * Read Latency Map
 *
 * Read data from kernel ring to user ring.
 *
 * @param mapfd hash map id.
 *
 * @return it returns 0 on success and -1 otherwise
static int hardirq_read_latency_map(int mapfd)
{
    static hardirq_ebpf_static_val_t *hardirq_ebpf_dynamic_vals = NULL;
    if (!hardirq_ebpf_dynamic_vals)
        hardirq_ebpf_dynamic_vals = callocz(ebpf_nprocs, sizeof(hardirq_ebpf_static_val_t));

    hardirq_ebpf_key_t key = {};
    hardirq_ebpf_key_t next_key = {};

    while (bpf_map_get_next_key(mapfd, &key, &next_key) == 0) {
        if (ebpf_plugin_stop())
            break;

        int test = bpf_map_lookup_elem(mapfd, &key, hardirq_ebpf_dynamic_vals);
        if (unlikely(test < 0)) {
            key = next_key;
            continue;
        }

        if (unlikely(key.irq < 0 || key.irq >= NETDATA_HARDIRQ_MAX_IRQS)) {
            key = next_key;
            continue;
        }

        hardirq_val_t *v = ebpf_hardirq_get(key.irq);
        if (unlikely(!v)) {
            key = next_key;
            continue;
        }

        if (!v->dim_exists) {
            if (hardirq_parse_interrupts(v->name, v->irq)) {
                ebpf_hardirq_release(v->irq);
                key = next_key;
                continue;
            }
            v->dim_exists = true;
        }

        uint64_t latency = 0;
        int i;
        for (i = 0; i < ebpf_nprocs; i++) {
            latency += hardirq_ebpf_dynamic_vals[i].latency / 1000;
        }
        v->latency = latency;

        key = next_key;
    }

    return 0;
}
 */

/**
 * Read Latency Static Map
 *
 * Read data from kernel ring to user ring.
 *
 * @param mapfd array map id.
 */
static void hardirq_read_latency_static_map(int mapfd)
{
    static hardirq_ebpf_static_val_t *hardirq_per_cpu_vals = NULL;
    if (!hardirq_per_cpu_vals)
        hardirq_per_cpu_vals = callocz(ebpf_nprocs, sizeof(hardirq_ebpf_static_val_t));

    int end = (running_on_kernel < NETDATA_KERNEL_V4_15) ? 1 : ebpf_nprocs;
    uint32_t i;
    for (i = 0; i < HARDIRQ_EBPF_STATIC_END; i++) {
        int test = bpf_map_lookup_elem(mapfd, &i, hardirq_per_cpu_vals);
        if (unlikely(test < 0)) {
            continue;
        }

        uint64_t latency = 0;
        int cpu_i;
        for (cpu_i = 0; cpu_i < end; cpu_i++) {
            latency += hardirq_per_cpu_vals[cpu_i].latency / 1000;
        }

        hardirq_static_vals[i].latency = latency;
    }
}

/**
 * Read eBPF maps for hard IRQ.
 *
 * @return When it is not possible to parse /proc, it returns -1, on success it returns 0.
 */
static int hardirq_reader(void)
{
    /*
    if (hardirq_read_latency_map(hardirq_maps[HARDIRQ_MAP_LATENCY].map_fd))
        return -1;
        */

    hardirq_read_latency_static_map(hardirq_maps[HARDIRQ_MAP_LATENCY_STATIC].map_fd);

    return 0;
}

/**
 * Create charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 *
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_hardirq_charts(int update_every)
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
        ebpf_create_global_dimension,
        hardirq_counter_publish_aggregated,
        1,
        update_every,
        NETDATA_EBPF_MODULE_NAME_HARDIRQ);

    fflush(stdout);
}

/**
 * Create static dimensions
 *
 * Create dimensions for static IRQs.
 */
static void hardirq_create_static_dims(void)
{
    uint32_t i;
    for (i = 0; i < HARDIRQ_EBPF_STATIC_END; i++) {
        ebpf_write_global_dimension(
            hardirq_static_vals[i].name, hardirq_static_vals[i].name, ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
    }
}

/**
 * Write dimensions
 *
 * Traverse JudyL array to write dimensions.
 *
 * @return It returns 1 to continue the iteration.
 */
static int hardirq_write_dims(Word_t index, hardirq_val_t *v)
{
    (void)index;

    if (!v->dim_exists) {
        ebpf_write_global_dimension(v->name, v->name, ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
        v->dim_exists = true;
    }

    write_chart_dimension(v->name, v->latency);

    return 1;
}

/**
 * Write all dimensions
 *
 * Traverse JudyL array and call hardirq_write_dims for each entry.
 */
static inline void hardirq_write_all_dims(void)
{
    Word_t index = 0;
    Pvoid_t *PValue;
    for (PValue = JudyLFirst(ebpf_hardirq_JudyL, &index, PJE0); PValue != NULL && PValue != PJERR;
         PValue = JudyLNext(ebpf_hardirq_JudyL, &index, PJE0)) {
        hardirq_val_t *v = *PValue;
        if (v)
            hardirq_write_dims(index, v);
    }
}

/**
 * Write static dimensions
 *
 * Write dimensions for static IRQs.
 */
static inline void hardirq_write_static_dims(void)
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
    netdata_mutex_lock(&lock);
    ebpf_create_hardirq_charts(em->update_every);
    hardirq_create_static_dims();
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_ADD);
    netdata_mutex_unlock(&lock);

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    int update_every = em->update_every;
    int counter = update_every - 1;
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        if (ebpf_plugin_stop())
            break;

        heartbeat_next(&hb);

        if (ebpf_plugin_stop())
            break;

        if (++counter != update_every)
            continue;

        counter = 0;
        if (hardirq_reader()) {
            hardirq_safe_clean = false;
            break;
        }

        netdata_mutex_lock(&lock);

        ebpf_write_begin_chart(NETDATA_EBPF_SYSTEM_GROUP, "hardirq_latency", "");
        //hardirq_write_all_dims();
        hardirq_write_static_dims();
        ebpf_write_end_chart();

        netdata_mutex_unlock(&lock);

        if (ebpf_plugin_stop())
            break;

        netdata_mutex_lock(&ebpf_exit_cleanup);
        if (running_time)
            running_time += update_every;
        else
            running_time = update_every;

        em->running_time = running_time;
        netdata_mutex_unlock(&ebpf_exit_cleanup);
    }
}

/*****************************************************************
 *  EBPF HARDIRQ THREAD
 *****************************************************************/

/**
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
#ifdef LIBBPF_MAJOR_VERSION
    ebpf_define_map_type(em->maps, em->maps_per_core, running_on_kernel);
#endif

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
            if (ret) {
                hardirq_bpf__destroy(hardirq_bpf_obj);
                hardirq_bpf_obj = NULL;
            }
        }
    }
#endif

    if (ret)
        netdata_log_error("%s %s", EBPF_DEFAULT_ERROR_MSG, em->info.thread_name);

    return ret;
}

/**
 * Allocate vectors used with this thread.
 *
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 */
static void ebpf_hardirq_allocate_global_vectors()
{
    memset(hardirq_counter_aggregated_data, 0, NETDATA_HARDIRQ_DIMENSION * sizeof(netdata_syscall_stat_t));
    memset(hardirq_counter_publish_aggregated, 0, NETDATA_HARDIRQ_DIMENSION * sizeof(netdata_publish_syscall_t));
}

/**
 * Hard IRQ latency thread.
 *
 * @param ptr a `ebpf_module_t *`.
 * @return always NULL.
 */
void ebpf_hardirq_thread(void *ptr)
{
    return;
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    CLEANUP_FUNCTION_REGISTER(hardirq_cleanup) cleanup_ptr = em;

    if (!ebpf_module_thread_has_valid_state(em)) {
        goto endhardirq;
    }

    em->maps = hardirq_maps;

    if (ebpf_enable_tracepoints(hardirq_tracepoints) == 0) {
        goto endhardirq;
    }

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_hardirq_load_bpf(em)) {
        goto endhardirq;
    }

    ebpf_hardirq_allocate_global_vectors();

    int algorithms[NETDATA_HARDIRQ_DIMENSION] = {NETDATA_EBPF_INCREMENTAL_IDX};

    ebpf_global_labels(
        hardirq_counter_aggregated_data,
        hardirq_counter_publish_aggregated,
        hardirq_counter_dimension_name,
        hardirq_counter_dimension_name,
        algorithms,
        NETDATA_HARDIRQ_DIMENSION);

    hardirq_safe_clean = true;
    hardirq_collector(em);

endhardirq:
    ebpf_update_disabled_plugin_stats(em);
}
