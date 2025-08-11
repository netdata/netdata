// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_softirq.h"

struct config softirq_config = APPCONFIG_INITIALIZER;

#define SOFTIRQ_MAP_LATENCY 0
static ebpf_local_maps_t softirq_maps[] = {
    {.name = "tbl_softirq",
     .internal_input = NETDATA_SOFTIRQ_MAX_IRQS,
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

#define SOFTIRQ_TP_CLASS_IRQ "irq"
static ebpf_tracepoint_t softirq_tracepoints[] = {
    {.enabled = false, .class = SOFTIRQ_TP_CLASS_IRQ, .event = "softirq_entry"},
    {.enabled = false, .class = SOFTIRQ_TP_CLASS_IRQ, .event = "softirq_exit"},
    /* end */
    {.enabled = false, .class = NULL, .event = NULL}};

// these must be in the order defined by the kernel:
// https://elixir.bootlin.com/linux/v5.12.19/source/include/trace/events/irq.h#L13
static softirq_val_t softirq_vals[] = {
    {.name = "HI", .latency = 0},
    {.name = "TIMER", .latency = 0},
    {.name = "NET_TX", .latency = 0},
    {.name = "NET_RX", .latency = 0},
    {.name = "BLOCK", .latency = 0},
    {.name = "IRQ_POLL", .latency = 0},
    {.name = "TASKLET", .latency = 0},
    {.name = "SCHED", .latency = 0},
    {.name = "HRTIMER", .latency = 0},
    {.name = "RCU", .latency = 0},
};

// tmp store for soft IRQ values we get from a per-CPU eBPF map.
static softirq_ebpf_val_t *softirq_ebpf_vals = NULL;

/**
 * Obsolete global
 *
 * Obsolete global charts created by thread.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_softirq_global(ebpf_module_t *em)
{
    ebpf_write_chart_obsolete(
        NETDATA_EBPF_SYSTEM_GROUP,
        "softirq_latency",
        "",
        "Software IRQ latency",
        EBPF_COMMON_UNITS_MILLISECONDS,
        "softirqs",
        NETDATA_EBPF_CHART_TYPE_STACKED,
        "system.softirq_latency",
        NETDATA_CHART_PRIO_SYSTEM_SOFTIRQS + 1,
        em->update_every);
}

/**
 * Cleanup
 *
 * Clean up allocated memory.
 *
 * @param ptr thread data.
 */
static void softirq_cleanup(void *pptr)
{
    ebpf_module_t *em = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!em)
        return;

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        netdata_mutex_lock(&lock);

        ebpf_obsolete_softirq_global(em);

        netdata_mutex_unlock(&lock);
        fflush(stdout);
    }

    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_REMOVE);

    if (em->objects) {
        ebpf_unload_legacy_code(em->objects, em->probe_links);
        em->objects = NULL;
        em->probe_links = NULL;
    }

    for (int i = 0; softirq_tracepoints[i].class != NULL; i++) {
        ebpf_disable_tracepoint(&softirq_tracepoints[i]);
    }
    freez(softirq_ebpf_vals);
    softirq_ebpf_vals = NULL;

    netdata_mutex_lock(&ebpf_exit_cleanup);
    em->enabled = NETDATA_THREAD_EBPF_STOPPED;
    ebpf_update_stats(&plugin_statistics, em);
    netdata_mutex_unlock(&ebpf_exit_cleanup);
}

/*****************************************************************
 *  MAIN LOOP
 *****************************************************************/

/**
 * Read Latency Map
 *
 * Read data from kernel ring to plot for users.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void softirq_read_latency_map(int maps_per_core)
{
    int fd = softirq_maps[SOFTIRQ_MAP_LATENCY].map_fd;
    int i;
    size_t length = sizeof(softirq_ebpf_val_t);
    if (maps_per_core)
        length *= ebpf_nprocs;

    for (i = 0; i < NETDATA_SOFTIRQ_MAX_IRQS; i++) {
        int test = bpf_map_lookup_elem(fd, &i, softirq_ebpf_vals);
        if (unlikely(test < 0)) {
            continue;
        }

        uint64_t total_latency = 0;
        int cpu_i;
        int end = (maps_per_core) ? ebpf_nprocs : 1;
        for (cpu_i = 0; cpu_i < end; cpu_i++) {
            total_latency += softirq_ebpf_vals[cpu_i].latency / 1000;
        }

        softirq_vals[i].latency = total_latency;
        memset(softirq_ebpf_vals, 0, length);
    }
}

static void softirq_create_charts(int update_every)
{
    ebpf_create_chart(
        NETDATA_EBPF_SYSTEM_GROUP,
        "softirq_latency",
        "Software IRQ latency",
        EBPF_COMMON_UNITS_MILLISECONDS,
        "softirqs",
        "system.softirq_latency",
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_CHART_PRIO_SYSTEM_SOFTIRQS + 1,
        NULL,
        NULL,
        0,
        update_every,
        NETDATA_EBPF_MODULE_NAME_SOFTIRQ);

    fflush(stdout);
}

static void softirq_create_dims()
{
    uint32_t i;
    for (i = 0; i < NETDATA_SOFTIRQ_MAX_IRQS; i++) {
        ebpf_write_global_dimension(
            softirq_vals[i].name, softirq_vals[i].name, ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
    }
}

static inline void softirq_write_dims()
{
    uint32_t i;
    for (i = 0; i < NETDATA_SOFTIRQ_MAX_IRQS; i++) {
        write_chart_dimension(softirq_vals[i].name, softirq_vals[i].latency);
    }
}

/**
* Main loop for this collector.
*/
static void softirq_collector(ebpf_module_t *em)
{
    softirq_ebpf_vals = callocz(ebpf_nprocs, sizeof(softirq_ebpf_val_t));

    // create chart and static dims.
    netdata_mutex_lock(&lock);
    softirq_create_charts(em->update_every);
    softirq_create_dims();
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_ADD);
    netdata_mutex_unlock(&lock);

    // loop and read from published data until ebpf plugin is closed.
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    int update_every = em->update_every;
    int counter = update_every - 1;
    int maps_per_core = em->maps_per_core;
    //This will be cancelled by its parent
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        heartbeat_next(&hb);
        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        counter = 0;
        softirq_read_latency_map(maps_per_core);
        netdata_mutex_lock(&lock);

        // write dims now for all hitherto discovered IRQs.
        ebpf_write_begin_chart(NETDATA_EBPF_SYSTEM_GROUP, "softirq_latency", "");
        softirq_write_dims();
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
 *  EBPF SOFTIRQ THREAD
 *****************************************************************/

/**
 * Soft IRQ latency thread.
 *
 * @param ptr a `ebpf_module_t *`.
 * @return always NULL.
 */
void ebpf_softirq_thread(void *ptr)
{
    ebpf_module_t *em = ptr;

    CLEANUP_FUNCTION_REGISTER(softirq_cleanup) cleanup_ptr = em;

    em->maps = softirq_maps;

    if (ebpf_enable_tracepoints(softirq_tracepoints) == 0) {
        goto endsoftirq;
    }

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_define_map_type(em->maps, em->maps_per_core, running_on_kernel);
#endif
    em->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &em->objects);
    if (!em->probe_links) {
        goto endsoftirq;
    }

    softirq_collector(em);

endsoftirq:
    ebpf_update_disabled_plugin_stats(em);
}
