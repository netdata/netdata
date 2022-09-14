// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

/* Run service jobs every X seconds */
#define SERVICE_HEARTBEAT 10

#define WORKER_JOB_HOSTS                        1
#define WORKER_JOB_CHILD_CHART_OBSOLETION_CHECK 2
#define WORKER_JOB_CLEANUP_OBSOLETE_CHARTS      3
#define WORKER_JOB_ARCHIVE_CHART                4
#define WORKER_JOB_ARCHIVE_CHART_DIMENSIONS     5
#define WORKER_JOB_ARCHIVE_DIMENSION            6


static void rrddim_obsolete_to_archive(RRDDIM *rd) {
    worker_is_busy(WORKER_JOB_ARCHIVE_DIMENSION);

    RRDSET *st = rd->rrdset;

    if(rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED | RRDDIM_FLAG_ACLK) || !rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE))
        return;

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

        size_t tiers_available = 0, tiers_said_yes = 0;
        for(int tier = 0; tier < storage_tiers ;tier++) {
            if(rd->tiers[tier]) {
                tiers_available++;

                if(rd->tiers[tier]->collect_ops.finalize(rd->tiers[tier]->db_collection_handle))
                    tiers_said_yes++;

                rd->tiers[tier]->db_collection_handle = NULL;
            }
        }

        if (tiers_available == tiers_said_yes && tiers_said_yes) {
            /* This metric has no data and no references */
            delete_dimension_uuid(&rd->metric_uuid);
        }
        else {
            /* Do not delete this dimension */
#ifdef ENABLE_ACLK
            queue_dimension_to_aclk(rd, calc_dimension_liveness(rd, now_realtime_sec()));
#endif
            return;
        }
    }

    rrddim_free(st, rd);
}

static void rrdset_archive_obsolete_dimensions(RRDSET *st, bool all_dimensions) {
    worker_is_busy(WORKER_JOB_ARCHIVE_CHART_DIMENSIONS);

    RRDDIM *rd;
    time_t now = now_realtime_sec();

    dfe_start_reentrant(st->rrddim_root_index, rd) {
        if(unlikely(
                all_dimensions ||
                (rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE) && (rd->last_collected_time.tv_sec + rrdset_free_obsolete_time < now))
                    )) {

            info("Removing obsolete dimension '%s' (%s) of '%s' (%s).", rrddim_name(rd), rrddim_id(rd), rrdset_name(st), rrdset_id(st));
            rrddim_obsolete_to_archive(rd);

        }
    }
    dfe_done(rd);
}

static void rrdset_obsolete_to_archive(RRDSET *st) {
    worker_is_busy(WORKER_JOB_ARCHIVE_CHART);

    rrdset_flag_set(st, RRDSET_FLAG_ARCHIVED);
    rrdset_flag_clear(st, RRDSET_FLAG_OBSOLETE);

    rrdcalc_unlink_all_rrdset_alerts(st);

    rrdset_archive_obsolete_dimensions(st, true);

    rrdsetvar_release_and_delete_all(st);

    // has to be run after all dimensions are archived - or use-after-free will occur
    rrdvar_delete_all(st->rrdvars);

    if(st->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
        if(rrdhost_flag_check(st->rrdhost, RRDHOST_FLAG_DELETE_OBSOLETE_CHARTS))
            rrdset_delete_files(st);
        else
            rrdset_save(st);

        rrdset_free(st);
    }
}

static void rrdhost_cleanup_obsolete_charts(RRDHOST *host) {
    worker_is_busy(WORKER_JOB_CLEANUP_OBSOLETE_CHARTS);

    time_t now = now_realtime_sec();
    RRDSET *st;
    rrdset_foreach_reentrant(st, host) {
        if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)
                     && st->last_accessed_time + rrdset_free_obsolete_time < now
                     && st->last_updated.tv_sec + rrdset_free_obsolete_time < now
                     && st->last_collected_time.tv_sec + rrdset_free_obsolete_time < now
                     )) {
            rrdset_obsolete_to_archive(st);
        }
        else if(rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE_DIMENSIONS)) {
            rrdset_flag_clear(st, RRDSET_FLAG_OBSOLETE_DIMENSIONS);
            rrdset_archive_obsolete_dimensions(st, false);
        }
#ifdef ENABLE_ACLK
        else
            sql_check_chart_liveness(st);
#endif
    }
    rrdset_foreach_done(st);
}

static void rrdset_check_obsoletion(RRDHOST *host) {
    worker_is_busy(WORKER_JOB_CHILD_CHART_OBSOLETION_CHECK);

    time_t last_entry_t;
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        last_entry_t = rrdset_last_entry_t(st);

        if(last_entry_t && last_entry_t < host->senders_connect_time)
            rrdset_is_obsolete(st);

    }
    rrdset_foreach_done(st);
}

static void rrd_cleanup_obsolete_charts_from_all_hosts() {
    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host) {
        worker_is_busy(WORKER_JOB_HOSTS);

        if(rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_OBSOLETE_CHARTS|RRDHOST_FLAG_PENDING_OBSOLETE_DIMENSIONS)) {
            rrdhost_flag_clear(host, RRDHOST_FLAG_PENDING_OBSOLETE_CHARTS|RRDHOST_FLAG_PENDING_OBSOLETE_DIMENSIONS);
            rrdhost_cleanup_obsolete_charts(host);
        }

        if(host != localhost
            && host->trigger_chart_obsoletion_check
            && (
                   (
                    host->senders_last_chart_command
                 && host->senders_last_chart_command + host->health_delay_up_to < now_realtime_sec()
                   )
                || (host->senders_connect_time + 300 < now_realtime_sec())
                )
            ) {

            rrdset_check_obsoletion(host);
            host->trigger_chart_obsoletion_check = 0;

        }
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
    worker_register_job_name(WORKER_JOB_HOSTS, "hosts");
    worker_register_job_name(WORKER_JOB_CHILD_CHART_OBSOLETION_CHECK, "child chart obsoletion check");
    worker_register_job_name(WORKER_JOB_CLEANUP_OBSOLETE_CHARTS, "cleanup obsolete charts");
    worker_register_job_name(WORKER_JOB_ARCHIVE_CHART, "archive chart");
    worker_register_job_name(WORKER_JOB_ARCHIVE_CHART_DIMENSIONS, "archive chart dimensions");
    worker_register_job_name(WORKER_JOB_ARCHIVE_DIMENSION, "archive dimension");

    netdata_thread_cleanup_push(service_main_cleanup, ptr);
    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = USEC_PER_SEC * SERVICE_HEARTBEAT;

    debug(D_SYSTEM, "Service thread starts");

    while (!netdata_exit) {
        heartbeat_next(&hb, step);

        rrd_cleanup_obsolete_charts_from_all_hosts();

        rrd_wrlock();
        rrdhost_cleanup_orphan_hosts_nolock(localhost);
        rrd_unlock();

    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
