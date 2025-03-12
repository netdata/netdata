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

    if (rd->rrd_memory_mode == RRD_DB_MODE_DBENGINE) {
        /* only a collector can mark a chart as obsolete, so we must remove the reference */
        if (!rrddim_finalize_collection_and_check_retention(rd)) {
            /* This metric has no data and no references */
            metaqueue_delete_dimension_uuid(uuidmap_uuid_ptr(rd->uuid));
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

    dictionary_garbage_collect(host->rrdset_root_index);

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

    time_t child_connect_time = host->stream.rcv.status.last_connected;

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

        if (!service_running(SERVICE_MAINTENANCE))
            break;

        if(rrdhost_receiver_replicating_charts(host) || rrdhost_sender_replicating_charts(host))
            continue;

        svc_rrdhost_cleanup_charts_marked_obsolete(host);

        if (host == localhost)
            continue;

        rrdhost_receiver_lock(host);

        time_t now = now_realtime_sec();

        if (host->stream.rcv.status.check_obsolete &&
            ((host->stream.rcv.status.last_chart &&
              host->stream.rcv.status.last_chart + host->health.delay_up_to < now) ||
             (host->stream.rcv.status.last_connected + TIME_TO_RUN_OBSOLETIONS_ON_CHILD_CONNECT < now))) {
            svc_rrdhost_detect_obsolete_charts(host);
            host->stream.rcv.status.check_obsolete = false;
        }

        rrdhost_receiver_unlock(host);
    }

    rrd_rdunlock();
}

static void svc_rrdhost_cleanup_orphan_hosts(RRDHOST *protected_host) {
    worker_is_busy(WORKER_JOB_CLEANUP_ORPHAN_HOSTS);

    time_t now = now_realtime_sec();

    rrd_wrlock();
    RRDHOST *host, *next = localhost;
    while((host = next) != NULL) {
        next = host->next;

        if(!rrdhost_should_be_cleaned_up(host, protected_host, now))
            continue;

        bool delete = rrdhost_free_ephemeral_time_s &&
                      now - host->stream.rcv.status.last_disconnected > rrdhost_free_ephemeral_time_s &&
                      rrdhost_option_check(host, RRDHOST_OPTION_EPHEMERAL_HOST);

        if (!delete && rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)) {
            // the node is archived, so the cleanup has already run
            // however, the node may not have any retention now
            // so it may still need to be needed
            time_t from_s = 0, to_s = 0;
            rrdhost_retention(host, now, rrdhost_is_online(host), &from_s, &to_s);
            if(!from_s && !to_s)
                delete = true;
            else
                continue;
        }

        worker_is_busy(WORKER_JOB_FREE_HOST);

        if (delete) {
            netdata_log_info("Host '%s' with machine guid '%s' is archived, ephemeral clean up.", rrdhost_hostname(host), host->machine_guid);
            // we inform cloud a child has been removed
            aclk_host_state_update(host, 0, 0);
            unregister_node(host->machine_guid);
            rrdhost_free___while_having_rrd_wrlock(host);
        }
        else
            rrdhost_cleanup_data_collection_and_health(host);
    }
    rrd_wrunlock();
}

static void service_main_cleanup(void *pptr)
{
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

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

    CLEANUP_FUNCTION_REGISTER(service_main_cleanup) cleanup_ptr = ptr;

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    usec_t step = USEC_PER_SEC * SERVICE_HEARTBEAT;
    usec_t real_step = USEC_PER_SEC;

    netdata_log_debug(D_SYSTEM, "Service thread starts");

    while (service_running(SERVICE_MAINTENANCE)) {
        worker_is_idle();
        heartbeat_next(&hb);
        if (real_step < step) {
            real_step += USEC_PER_SEC;
            continue;
        }
        real_step = USEC_PER_SEC;

        svc_rrd_cleanup_obsolete_charts_from_all_hosts();

        if (service_running(SERVICE_MAINTENANCE))
            svc_rrdhost_cleanup_orphan_hosts(localhost);
    }

    return NULL;
}
