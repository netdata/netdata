// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

/**
 * Create a chart for the main exporting thread CPU usage
 *
 * @param st_rusage the thead CPU usage chart
 * @param rd_user a dimension for user CPU usage
 * @param rd_system a dimension for system CPU usage
 */
void create_main_rusage_chart(RRDSET **st_rusage, RRDDIM **rd_user, RRDDIM **rd_system)
{
    if (!pulse_enabled)
        return;
        
    if (*st_rusage && *rd_user && *rd_system)
        return;

    *st_rusage = rrdset_create_localhost(
        "netdata",
        "exporting_main_thread_cpu",
        NULL,
        "exporting",
        "netdata.exporting_cpu_usage",
        "Netdata Main Exporting Thread CPU Usage",
        "milliseconds/s",
        "exporting",
        NULL,
        130600,
        localhost->rrd_update_every,
        RRDSET_TYPE_STACKED);

    *rd_user = rrddim_add(*st_rusage, "user", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
    *rd_system = rrddim_add(*st_rusage, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
}

/**
 * Send the main exporting thread CPU usage
 *
 * @param st_rusage a thead CPU usage chart
 * @param rd_user a dimension for user CPU usage
 * @param rd_system a dimension for system CPU usage
 */
void send_main_rusage(RRDSET *st_rusage, RRDDIM *rd_user, RRDDIM *rd_system)
{
    if (!pulse_enabled)
        return;

    struct rusage thread;
    getrusage(RUSAGE_THREAD, &thread);

    rrddim_set_by_pointer(st_rusage, rd_user,   thread.ru_utime.tv_sec * 1000000ULL + thread.ru_utime.tv_usec);
    rrddim_set_by_pointer(st_rusage, rd_system, thread.ru_stime.tv_sec * 1000000ULL + thread.ru_stime.tv_usec);

    rrdset_done(st_rusage);
}

/**
 * Send internal metrics for an instance
 *
 * Send performance metrics for the operation of exporting engine itself to the Netdata database.
 *
 * @param instance an instance data structure.
 */
void send_internal_metrics(struct instance *instance)
{
    if (!pulse_enabled)
        return;

    struct stats *stats = &instance->stats;

    // ------------------------------------------------------------------------
    // create charts for monitoring the exporting operations

    if (!stats->initialized) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintf(id, RRD_ID_LENGTH_MAX, "exporting_%s_metrics", instance->config.name);
        netdata_fix_chart_id(id);

        stats->st_metrics = rrdset_create_localhost(
            "netdata",
            id,
            NULL,
            "exporting",
            "netdata.exporting_buffer",
            "Netdata Buffered Metrics",
            "metrics",
            "exporting",
            NULL,
            130610,
            instance->config.update_every,
            RRDSET_TYPE_LINE);

        stats->rd_buffered_metrics = rrddim_add(stats->st_metrics, "buffered", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        stats->rd_lost_metrics     = rrddim_add(stats->st_metrics, "lost", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        stats->rd_sent_metrics     = rrddim_add(stats->st_metrics, "sent", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        // ------------------------------------------------------------------------

        snprintf(id, RRD_ID_LENGTH_MAX, "exporting_%s_bytes", instance->config.name);
        netdata_fix_chart_id(id);

        stats->st_bytes = rrdset_create_localhost(
            "netdata",
            id,
            NULL,
            "exporting",
            "netdata.exporting_data_size",
            "Netdata Exporting Data Size",
            "KiB",
            "exporting",
            NULL,
            130620,
            instance->config.update_every,
            RRDSET_TYPE_AREA);

        stats->rd_buffered_bytes = rrddim_add(stats->st_bytes, "buffered", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        stats->rd_lost_bytes     = rrddim_add(stats->st_bytes, "lost", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        stats->rd_sent_bytes     = rrddim_add(stats->st_bytes, "sent", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        stats->rd_received_bytes = rrddim_add(stats->st_bytes, "received", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);

        // ------------------------------------------------------------------------

        snprintf(id, RRD_ID_LENGTH_MAX, "exporting_%s_ops", instance->config.name);
        netdata_fix_chart_id(id);

        stats->st_ops = rrdset_create_localhost(
            "netdata",
            id,
            NULL,
            "exporting",
            "netdata.exporting_operations",
            "Netdata Exporting Operations",
            "operations",
            "exporting",
            NULL,
            130630,
            instance->config.update_every,
            RRDSET_TYPE_LINE);

        stats->rd_transmission_successes = rrddim_add(stats->st_ops, "write", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        stats->rd_data_lost_events       = rrddim_add(stats->st_ops, "discard", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        stats->rd_reconnects             = rrddim_add(stats->st_ops, "reconnect", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        stats->rd_transmission_failures  = rrddim_add(stats->st_ops, "failure", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        stats->rd_receptions             = rrddim_add(stats->st_ops, "read", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        // ------------------------------------------------------------------------

        snprintf(id, RRD_ID_LENGTH_MAX, "exporting_%s_thread_cpu", instance->config.name);
        netdata_fix_chart_id(id);

        stats->st_rusage = rrdset_create_localhost(
            "netdata",
            id,
            NULL,
            "exporting",
            "netdata.exporting_instance",
            "Netdata Exporting Instance Thread CPU Usage",
            "milliseconds/s",
            "exporting",
            NULL,
            130640,
            instance->config.update_every,
            RRDSET_TYPE_STACKED);

        stats->rd_user   = rrddim_add(stats->st_rusage, "user", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
        stats->rd_system = rrddim_add(stats->st_rusage, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);

        stats->initialized = 1;
    }

    // ------------------------------------------------------------------------
    // update the monitoring charts

    rrddim_set_by_pointer(stats->st_metrics, stats->rd_buffered_metrics, stats->buffered_metrics);
    rrddim_set_by_pointer(stats->st_metrics, stats->rd_lost_metrics,     stats->lost_metrics);
    rrddim_set_by_pointer(stats->st_metrics, stats->rd_sent_metrics,     stats->sent_metrics);
    rrdset_done(stats->st_metrics);

    rrddim_set_by_pointer(stats->st_bytes, stats->rd_buffered_bytes, stats->buffered_bytes);
    rrddim_set_by_pointer(stats->st_bytes, stats->rd_lost_bytes,     stats->lost_bytes);
    rrddim_set_by_pointer(stats->st_bytes, stats->rd_sent_bytes,     stats->sent_bytes);
    rrddim_set_by_pointer(stats->st_bytes, stats->rd_received_bytes, stats->received_bytes);
    rrdset_done(stats->st_bytes);

    rrddim_set_by_pointer(stats->st_ops, stats->rd_transmission_successes, stats->transmission_successes);
    rrddim_set_by_pointer(stats->st_ops, stats->rd_data_lost_events,       stats->data_lost_events);
    rrddim_set_by_pointer(stats->st_ops, stats->rd_reconnects,             stats->reconnects);
    rrddim_set_by_pointer(stats->st_ops, stats->rd_transmission_failures,  stats->transmission_failures);
    rrddim_set_by_pointer(stats->st_ops, stats->rd_receptions,             stats->receptions);
    rrdset_done(stats->st_ops);

    struct rusage thread;
    getrusage(RUSAGE_THREAD, &thread);

    rrddim_set_by_pointer(stats->st_rusage, stats->rd_user,   thread.ru_utime.tv_sec * 1000000ULL + thread.ru_utime.tv_usec);
    rrddim_set_by_pointer(stats->st_rusage, stats->rd_system, thread.ru_stime.tv_sec * 1000000ULL + thread.ru_stime.tv_usec);
    rrdset_done(stats->st_rusage);
}
