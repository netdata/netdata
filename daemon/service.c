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
#define WORKER_JOB_SAVE_HOST_CHARTS                 10
#define WORKER_JOB_DELETE_HOST_CHARTS               11
#define WORKER_JOB_FREE_CHART                       12
#define WORKER_JOB_SAVE_CHART                       13
#define WORKER_JOB_DELETE_CHART                     14
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

    const char *cache_filename = rrddim_cache_filename(rd);
    if(cache_filename) {
        info("Deleting dimension file '%s'.", cache_filename);
        if (unlikely(unlink(cache_filename) == -1))
            error("Cannot delete dimension file '%s'", cache_filename);
    }

    if (rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
        rrddimvar_delete_all(rd);

        /* only a collector can mark a chart as obsolete, so we must remove the reference */

        size_t tiers_available = 0, tiers_said_no_retention = 0;
        for(size_t tier = 0; tier < storage_tiers ;tier++) {
            if(rd->tiers[tier].db_collection_handle) {
                tiers_available++;

                if(storage_engine_store_finalize(rd->tiers[tier].db_collection_handle))
                    tiers_said_no_retention++;

                rd->tiers[tier].db_collection_handle = NULL;
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

static bool svc_rrdset_archive_obsolete_dimensions(RRDSET *st, bool all_dimensions) {
    worker_is_busy(WORKER_JOB_ARCHIVE_CHART_DIMENSIONS);

    RRDDIM *rd;
    time_t now = now_realtime_sec();

    bool done_all_dimensions = true;

    dfe_start_write(st->rrddim_root_index, rd) {
        if(unlikely(
                all_dimensions ||
                (rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE) && (rd->last_collected_time.tv_sec + rrdset_free_obsolete_time_s < now))
                    )) {

            if(dictionary_acquired_item_references(rd_dfe.item) == 1) {
                info("Removing obsolete dimension '%s' (%s) of '%s' (%s).", rrddim_name(rd), rrddim_id(rd), rrdset_name(st), rrdset_id(st));
                svc_rrddim_obsolete_to_archive(rd);
            }
            else
                done_all_dimensions = false;
        }
        else
            done_all_dimensions = false;
    }
    dfe_done(rd);

    return done_all_dimensions;
}

static void svc_rrdset_obsolete_to_archive(RRDSET *st) {
    worker_is_busy(WORKER_JOB_ARCHIVE_CHART);

    if(!svc_rrdset_archive_obsolete_dimensions(st, true))
        return;

    rrdset_flag_set(st, RRDSET_FLAG_ARCHIVED);
    rrdset_flag_clear(st, RRDSET_FLAG_OBSOLETE);

    rrdcalc_unlink_all_rrdset_alerts(st);

    rrdsetvar_release_and_delete_all(st);

    // has to be run after all dimensions are archived - or use-after-free will occur
    rrdvar_delete_all(st->rrdvars);

    if(st->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
        if(rrdhost_option_check(st->rrdhost, RRDHOST_OPTION_DELETE_OBSOLETE_CHARTS)) {
            worker_is_busy(WORKER_JOB_DELETE_CHART);
            rrdset_delete_files(st);
        }
        else {
            worker_is_busy(WORKER_JOB_SAVE_CHART);
            rrdset_save(st);
        }

        worker_is_busy(WORKER_JOB_FREE_CHART);
        rrdset_free(st);
    }
}

static void svc_rrdhost_cleanup_obsolete_charts(RRDHOST *host) {
    worker_is_busy(WORKER_JOB_CLEANUP_OBSOLETE_CHARTS);

    time_t now = now_realtime_sec();
    RRDSET *st;
    rrdset_foreach_reentrant(st, host) {
        if(rrdset_is_replicating(st))
            continue;

        if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)
                     && st->last_accessed_time_s + rrdset_free_obsolete_time_s < now
                     && st->last_updated.tv_sec + rrdset_free_obsolete_time_s < now
                     && st->last_collected_time.tv_sec + rrdset_free_obsolete_time_s < now
                     )) {
            svc_rrdset_obsolete_to_archive(st);
        }
        else if(rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE_DIMENSIONS)) {
            rrdset_flag_clear(st, RRDSET_FLAG_OBSOLETE_DIMENSIONS);
            svc_rrdset_archive_obsolete_dimensions(st, false);
        }
    }
    rrdset_foreach_done(st);
}

static void svc_rrdset_check_obsoletion(RRDHOST *host) {
    worker_is_busy(WORKER_JOB_CHILD_CHART_OBSOLETION_CHECK);

    time_t now = now_realtime_sec();
    time_t last_entry_t;
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        if(rrdset_is_replicating(st))
            continue;

        last_entry_t = rrdset_last_entry_s(st);

        if(last_entry_t && last_entry_t < host->child_connect_time &&
           host->child_connect_time + TIME_TO_RUN_OBSOLETIONS_ON_CHILD_CONNECT + ITERATIONS_TO_RUN_OBSOLETIONS_ON_CHILD_CONNECT * st->update_every
             < now)

            rrdset_is_obsolete(st);
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

        if(rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_OBSOLETE_CHARTS|RRDHOST_FLAG_PENDING_OBSOLETE_DIMENSIONS)) {
            rrdhost_flag_clear(host, RRDHOST_FLAG_PENDING_OBSOLETE_CHARTS|RRDHOST_FLAG_PENDING_OBSOLETE_DIMENSIONS);
            svc_rrdhost_cleanup_obsolete_charts(host);
        }

        if(host != localhost
            && host->trigger_chart_obsoletion_check
            && (
                   (
                    host->child_last_chart_command
                 && host->child_last_chart_command + host->health.health_delay_up_to < now_realtime_sec()
                   )
                || (host->child_connect_time + TIME_TO_RUN_OBSOLETIONS_ON_CHILD_CONNECT < now_realtime_sec())
                )
            ) {
            svc_rrdset_check_obsoletion(host);
            host->trigger_chart_obsoletion_check = 0;
        }
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

        info("Host '%s' with machine guid '%s' is obsolete - cleaning up.", rrdhost_hostname(host), host->machine_guid);

        if (rrdhost_option_check(host, RRDHOST_OPTION_DELETE_ORPHAN_HOST)
            /* don't delete multi-host DB host files */
            && !(host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE && is_storage_engine_shared(host->db[0].instance))
        ) {
            worker_is_busy(WORKER_JOB_DELETE_HOST_CHARTS);
            rrdhost_delete_charts(host);
        }
        else {
            worker_is_busy(WORKER_JOB_SAVE_HOST_CHARTS);
            rrdhost_save_charts(host);
        }

        worker_is_busy(WORKER_JOB_FREE_HOST);
        rrdhost_free___while_having_rrd_wrlock(host, false);
        goto restart_after_removal;
    }

    rrd_unlock();
}

static void service_main_cleanup(void *ptr)
{
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    debug(D_SYSTEM, "Cleaning up...");
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
    worker_register_job_name(WORKER_JOB_SAVE_HOST_CHARTS, "save host charts");
    worker_register_job_name(WORKER_JOB_DELETE_HOST_CHARTS, "delete host charts");
    worker_register_job_name(WORKER_JOB_FREE_CHART, "free chart");
    worker_register_job_name(WORKER_JOB_SAVE_CHART, "save chart");
    worker_register_job_name(WORKER_JOB_DELETE_CHART, "delete chart");
    worker_register_job_name(WORKER_JOB_FREE_DIMENSION, "free dimension");
    worker_register_job_name(WORKER_JOB_PGC_MAIN_EVICT, "main cache evictions");
    worker_register_job_name(WORKER_JOB_PGC_MAIN_FLUSH, "main cache flushes");
    worker_register_job_name(WORKER_JOB_PGC_OPEN_EVICT, "open cache evictions");
    worker_register_job_name(WORKER_JOB_PGC_OPEN_FLUSH, "open cache flushes");

    netdata_thread_cleanup_push(service_main_cleanup, ptr);
    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = USEC_PER_SEC * SERVICE_HEARTBEAT;

    debug(D_SYSTEM, "Service thread starts");

    while (service_running(SERVICE_MAINTENANCE)) {
        worker_is_idle();
        heartbeat_next(&hb, step);

        svc_rrd_cleanup_obsolete_charts_from_all_hosts();
        svc_rrdhost_cleanup_orphan_hosts(localhost);
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
