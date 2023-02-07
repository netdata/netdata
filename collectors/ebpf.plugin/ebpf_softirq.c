// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_softirq.h"

struct config softirq_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

#define SOFTIRQ_MAP_LATENCY 0
static ebpf_local_maps_t softirq_maps[] = {
    {
        .name = "tbl_softirq",
        .internal_input = NETDATA_SOFTIRQ_MAX_IRQS,
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

#define SOFTIRQ_TP_CLASS_IRQ "irq"
static ebpf_tracepoint_t softirq_tracepoints[] = {
    {.enabled = false, .class = SOFTIRQ_TP_CLASS_IRQ, .event = "softirq_entry"},
    {.enabled = false, .class = SOFTIRQ_TP_CLASS_IRQ, .event = "softirq_exit"},
    /* end */
    {.enabled = false, .class = NULL, .event = NULL}
};

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
 * Cachestat Free
 *
 * Cleanup variables after child threads to stop
 *
 * @param ptr thread data.
 */
static void ebpf_softirq_free(ebpf_module_t *em)
{
    pthread_mutex_lock(&ebpf_exit_cleanup);
    em->thread->enabled = NETDATA_THREAD_EBPF_STOPPING;
    pthread_mutex_unlock(&ebpf_exit_cleanup);

    for (int i = 0; softirq_tracepoints[i].class != NULL; i++) {
        ebpf_disable_tracepoint(&softirq_tracepoints[i]);
    }
    freez(softirq_ebpf_vals);

    pthread_mutex_lock(&ebpf_exit_cleanup);
    em->thread->enabled = NETDATA_THREAD_EBPF_STOPPED;
    pthread_mutex_unlock(&ebpf_exit_cleanup);
}

/**
 * Cleanup
 *
 * Clean up allocated memory.
 *
 * @param ptr thread data.
 */
static void softirq_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    ebpf_softirq_free(em);
}

/*****************************************************************
 *  MAIN LOOP
 *****************************************************************/

static void softirq_read_latency_map()
{
    int fd = softirq_maps[SOFTIRQ_MAP_LATENCY].map_fd;
    int i;
    for (i = 0; i < NETDATA_SOFTIRQ_MAX_IRQS; i++) {
        int test = bpf_map_lookup_elem(fd, &i, softirq_ebpf_vals);
        if (unlikely(test < 0)) {
            continue;
        }

        uint64_t total_latency = 0;
        int cpu_i;
        int end = ebpf_nprocs;
        for (cpu_i = 0; cpu_i < end; cpu_i++) {
            total_latency += softirq_ebpf_vals[cpu_i].latency/1000;
        }

        softirq_vals[i].latency = total_latency;
    }
}

static void softirq_create_charts(int update_every)
{
    ebpf_create_chart(
        NETDATA_EBPF_SYSTEM_GROUP,
        "softirq_latency",
        "Software IRQ latency",
        EBPF_COMMON_DIMENSION_MILLISECONDS,
        "softirqs",
        NULL,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_CHART_PRIO_SYSTEM_SOFTIRQS+1,
        NULL, NULL, 0, update_every,
        NETDATA_EBPF_MODULE_NAME_SOFTIRQ
    );

    fflush(stdout);
}

static void softirq_create_dims()
{
    uint32_t i;
    for (i = 0; i < NETDATA_SOFTIRQ_MAX_IRQS; i++) {
        ebpf_write_global_dimension(
            softirq_vals[i].name, softirq_vals[i].name,
            ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]
        );
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
    pthread_mutex_lock(&lock);
    softirq_create_charts(em->update_every);
    softirq_create_dims();
    ebpf_update_stats(&plugin_statistics, em);
    pthread_mutex_unlock(&lock);

    // loop and read from published data until ebpf plugin is closed.
    heartbeat_t hb;
    heartbeat_init(&hb);
    int update_every = em->update_every;
    int counter = update_every - 1;
    //This will be cancelled by its parent
    while (!ebpf_exit_plugin) {
        (void)heartbeat_next(&hb, USEC_PER_SEC);
        if (ebpf_exit_plugin || ++counter != update_every)
            continue;

        counter = 0;
        softirq_read_latency_map();
        pthread_mutex_lock(&lock);

        // write dims now for all hitherto discovered IRQs.
        write_begin_chart(NETDATA_EBPF_SYSTEM_GROUP, "softirq_latency");
        softirq_write_dims();
        write_end_chart();

        pthread_mutex_unlock(&lock);
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
void *ebpf_softirq_thread(void *ptr)
{
    netdata_thread_cleanup_push(softirq_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = softirq_maps;

    if (ebpf_enable_tracepoints(softirq_tracepoints) == 0) {
        em->thread->enabled = NETDATA_THREAD_EBPF_STOPPED;
        goto endsoftirq;
    }

    em->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &em->objects);
    if (!em->probe_links) {
        em->thread->enabled = NETDATA_THREAD_EBPF_STOPPED;
        goto endsoftirq;
    }

    softirq_collector(em);

endsoftirq:
    ebpf_update_disabled_plugin_stats(em);

    netdata_thread_cleanup_pop(1);

    return NULL;
}
