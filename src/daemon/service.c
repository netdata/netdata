// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

/* Run service jobs every X seconds */
#define SERVICE_HEARTBEAT 10

#define TIME_TO_RUN_OBSOLETIONS_ON_CHILD_CONNECT (3600 / 2)
#define ITERATIONS_TO_RUN_OBSOLETIONS_ON_CHILD_CONNECT 60

#define WORKER_JOB_CHILD_CHART_OBSOLETION_CHECK     1
#define WORKER_JOB_CLEANUP_OBSOLETE_CHARTS          2
#define WORKER_JOB_ARCHIVE_CHART                    3
#define WORKER_JOB_ARCHIVE_CHART_DIMENSIONS         4
#define WORKER_JOB_ARCHIVE_DIMENSION                5
#define WORKER_JOB_CLEANUP_ORPHAN_HOSTS             6
#define WORKER_JOB_CLEANUP_OBSOLETE_CHARTS_ON_HOSTS 7
#define WORKER_JOB_FREE_HOST                        9
#define WORKER_JOB_FREE_CHART                       12
#define WORKER_JOB_FREE_DIMENSION                   15
#define WORKER_JOB_PGC_MAIN_EVICT                   16
#define WORKER_JOB_PGC_MAIN_FLUSH                   17
#define WORKER_JOB_PGC_OPEN_EVICT                   18
#define WORKER_JOB_PGC_OPEN_FLUSH                   19

static void svc_rrddim_obsolete_to_archive(RRDDIM *rd) {
    RRDSET *st = rd->rrdset;

    if(rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED) || !rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE))
        return;

    worker_is_busy(WORKER_JOB_ARCHIVE_DIMENSION);

    rrddim_flag_set(rd, RRDDIM_FLAG_ARCHIVED);
    rrddim_flag_clear(rd, RRDDIM_FLAG_OBSOLETE);

    if (rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
        /* only a collector can mark a chart as obsolete, so we must remove the reference */

        size_t tiers_available = 0, tiers_said_no_retention = 0;
        for(size_t tier = 0; tier < storage_tiers ;tier++) {
            if(rd->tiers[tier].sch) {
                tiers_available++;

                if(storage_engine_store_finalize(rd->tiers[tier].sch))
                    tiers_said_no_retention++;

                rd->tiers[tier].sch = NULL;
            }
        }

        if (tiers_available == tiers_said_no_retention && tiers_said_no_retention) {
            /* This metric has no data and no references */
            metaqueue_delete_dimension_uuid(&rd->metric_uuid);
        }
        else {
            /* Do not delete this dimension */
            return;
        }
    }

    worker_is_busy(WORKER_JOB_FREE_DIMENSION);
    rrddim_free(st, rd);
}

static inline bool svc_rrdset_archive_obsolete_dimensions(RRDSET *st, bool all_dimensions) {
    if(!all_dimensions && !rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE_DIMENSIONS))
        return true;

    worker_is_busy(WORKER_JOB_ARCHIVE_CHART_DIMENSIONS);

    rrdset_flag_clear(st, RRDSET_FLAG_OBSOLETE_DIMENSIONS);

    RRDDIM *rd;
    time_t now = now_realtime_sec();

    size_t dim_candidates = 0;
    size_t dim_archives = 0;

    dfe_start_write(st->rrddim_root_index, rd) {
        bool candidate = (all_dimensions || rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE));

        if(candidate) {
            dim_candidates++;

            if(rd->collector.last_collected_time.tv_sec + rrdset_free_obsolete_time_s < now) {
                size_t references = dictionary_acquired_item_references(rd_dfe.item);
                if(references == 1) {
//                    netdata_log_info("Removing obsolete dimension 'host:%s/chart:%s/dim:%s'",
//                                     rrdhost_hostname(st->rrdhost), rrdset_id(st), rrddim_id(rd));
                    svc_rrddim_obsolete_to_archive(rd);
                    dim_archives++;
                }
//                else
//                    netdata_log_info("Cannot remove obsolete dimension 'host:%s/chart:%s/dim:%s'",
//                            rrdhost_hostname(st->rrdhost), rrdset_id(st), rrddim_id(rd));
            }
        }
    }
    dfe_done(rd);

    if(dim_archives != dim_candidates) {
        rrdset_flag_set(st, RRDSET_FLAG_OBSOLETE_DIMENSIONS);
        return false;
    }

    return true;
}

static void svc_rrdset_obsolete_to_free(RRDSET *st) {
    if(!svc_rrdset_archive_obsolete_dimensions(st, true))
        return;

    worker_is_busy(WORKER_JOB_FREE_CHART);

    rrdcalc_unlink_and_delete_all_rrdset_alerts(st);

    // has to be run after all dimensions are archived - or use-after-free will occur
    rrdvar_delete_all(st->rrdvars);

    rrdset_free(st);
}

static inline void svc_rrdhost_cleanup_charts_marked_obsolete(RRDHOST *host) {
    if(!rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_OBSOLETE_CHARTS|RRDHOST_FLAG_PENDING_OBSOLETE_DIMENSIONS))
        return;

    worker_is_busy(WORKER_JOB_CLEANUP_OBSOLETE_CHARTS);

    rrdhost_flag_clear(host, RRDHOST_FLAG_PENDING_OBSOLETE_CHARTS|RRDHOST_FLAG_PENDING_OBSOLETE_DIMENSIONS);

    size_t full_candidates = 0;
    size_t full_archives = 0;
    size_t partial_candidates = 0;
    size_t partial_archives = 0;

    time_t now = now_realtime_sec();
    RRDSET *st;
    rrdset_foreach_reentrant(st, host) {
        if(rrdset_is_replicating(st))
            continue;

        RRDSET_FLAGS flags = rrdset_flag_get(st);
        bool obsolete_chart = flags & RRDSET_FLAG_OBSOLETE;
        bool obsolete_dims = flags & RRDSET_FLAG_OBSOLETE_DIMENSIONS;

        if(obsolete_dims) {
            partial_candidates++;

            if(svc_rrdset_archive_obsolete_dimensions(st, false))
                partial_archives++;
        }

        if(obsolete_chart) {
            full_candidates++;

            if(unlikely(   st->last_accessed_time_s + rrdset_free_obsolete_time_s < now
                        && st->last_updated.tv_sec + rrdset_free_obsolete_time_s < now
                        && st->last_collected_time.tv_sec + rrdset_free_obsolete_time_s < now
                       )) {
                svc_rrdset_obsolete_to_free(st);
                full_archives++;
            }
        }
    }
    rrdset_foreach_done(st);

    if(partial_archives != partial_candidates)
        rrdhost_flag_set(host, RRDHOST_FLAG_PENDING_OBSOLETE_DIMENSIONS);

    if(full_archives != full_candidates)
        rrdhost_flag_set(host, RRDHOST_FLAG_PENDING_OBSOLETE_CHARTS);
}

static void svc_rrdhost_detect_obsolete_charts(RRDHOST *host) {
    worker_is_busy(WORKER_JOB_CHILD_CHART_OBSOLETION_CHECK);

    time_t now = now_realtime_sec();
    time_t last_entry_t;
    RRDSET *st;

    time_t child_connect_time = host->child_connect_time;

    rrdset_foreach_read(st, host) {
        if(rrdset_is_replicating(st))
            continue;

        last_entry_t = rrdset_last_entry_s(st);

        if (last_entry_t && last_entry_t < child_connect_time &&
            child_connect_time + TIME_TO_RUN_OBSOLETIONS_ON_CHILD_CONNECT +
                    (ITERATIONS_TO_RUN_OBSOLETIONS_ON_CHILD_CONNECT * st->update_every) <
                now)

            rrdset_is_obsolete___safe_from_collector_thread(st);
    }
    rrdset_foreach_done(st);
}

static void svc_rrd_cleanup_obsolete_charts_from_all_hosts() {
    worker_is_busy(WORKER_JOB_CLEANUP_OBSOLETE_CHARTS_ON_HOSTS);

    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host) {
        if(rrdhost_receiver_replicating_charts(host) || rrdhost_sender_replicating_charts(host))
            continue;

        svc_rrdhost_cleanup_charts_marked_obsolete(host);

        if (host == localhost)
            continue;

        netdata_mutex_lock(&host->receiver_lock);

        time_t now = now_realtime_sec();

        if (host->trigger_chart_obsoletion_check &&
            ((host->child_last_chart_command &&
              host->child_last_chart_command + host->health.health_delay_up_to < now) ||
             (host->child_connect_time + TIME_TO_RUN_OBSOLETIONS_ON_CHILD_CONNECT < now))) {
            svc_rrdhost_detect_obsolete_charts(host);
            host->trigger_chart_obsoletion_check = 0;
        }

        netdata_mutex_unlock(&host->receiver_lock);
    }

    rrd_unlock();
}

static void svc_rrdhost_cleanup_orphan_hosts(RRDHOST *protected_host) {
    worker_is_busy(WORKER_JOB_CLEANUP_ORPHAN_HOSTS);
    rrd_wrlock();

    time_t now = now_realtime_sec();

    RRDHOST *host;

restart_after_removal:
    rrdhost_foreach_write(host) {
        if(!rrdhost_should_be_removed(host, protected_host, now))
            continue;

        bool force = false;
        if (rrdhost_option_check(host, RRDHOST_OPTION_EPHEMERAL_HOST) && now - host->last_connected > rrdhost_free_ephemeral_time_s)
            force = true;

        bool is_archived = rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED);
        if (!force && is_archived)
            continue;

       if (force) {
            netdata_log_info("Host '%s' with machine guid '%s' is archived, ephemeral clean up.", rrdhost_hostname(host), host->machine_guid);
        }

        worker_is_busy(WORKER_JOB_FREE_HOST);
#ifdef ENABLE_ACLK
        // in case we have cloud connection we inform cloud
        // a child disconnected
        if (netdata_cloud_enabled && force) {
            aclk_host_state_update(host, 0, 0);
            unregister_node(host->machine_guid);
        }
#endif
        rrdhost_free___while_having_rrd_wrlock(host, force);
        goto restart_after_removal;
    }

    rrd_unlock();
}

static void service_main_cleanup(void *ptr)
{
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    netdata_log_debug(D_SYSTEM, "Cleaning up...");
    worker_unregister();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

/*
 * The service thread.
 */
void *service_main(void *ptr)
{
    worker_register("SERVICE");
    worker_register_job_name(WORKER_JOB_CHILD_CHART_OBSOLETION_CHECK, "child chart obsoletion check");
    worker_register_job_name(WORKER_JOB_CLEANUP_OBSOLETE_CHARTS, "cleanup obsolete charts");
    worker_register_job_name(WORKER_JOB_ARCHIVE_CHART, "archive chart");
    worker_register_job_name(WORKER_JOB_ARCHIVE_CHART_DIMENSIONS, "archive chart dimensions");
    worker_register_job_name(WORKER_JOB_ARCHIVE_DIMENSION, "archive dimension");
    worker_register_job_name(WORKER_JOB_CLEANUP_ORPHAN_HOSTS, "cleanup orphan hosts");
    worker_register_job_name(WORKER_JOB_CLEANUP_OBSOLETE_CHARTS_ON_HOSTS, "cleanup obsolete charts on all hosts");
    worker_register_job_name(WORKER_JOB_FREE_HOST, "free host");
    worker_register_job_name(WORKER_JOB_FREE_CHART, "free chart");
    worker_register_job_name(WORKER_JOB_FREE_DIMENSION, "free dimension");
    worker_register_job_name(WORKER_JOB_PGC_MAIN_EVICT, "main cache evictions");
    worker_register_job_name(WORKER_JOB_PGC_MAIN_FLUSH, "main cache flushes");
    worker_register_job_name(WORKER_JOB_PGC_OPEN_EVICT, "open cache evictions");
    worker_register_job_name(WORKER_JOB_PGC_OPEN_FLUSH, "open cache flushes");

    netdata_thread_cleanup_push(service_main_cleanup, ptr);
    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = USEC_PER_SEC * SERVICE_HEARTBEAT;
    usec_t real_step = USEC_PER_SEC;

    netdata_log_debug(D_SYSTEM, "Service thread starts");

    while (service_running(SERVICE_MAINTENANCE)) {
        worker_is_idle();
        heartbeat_next(&hb, USEC_PER_SEC);
        if (real_step < step) {
            real_step += USEC_PER_SEC;
            continue;
        }
        real_step = USEC_PER_SEC;

        svc_rrd_cleanup_obsolete_charts_from_all_hosts();
        svc_rrdhost_cleanup_orphan_hosts(localhost);
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
