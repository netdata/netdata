// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon-shutdown.h"
#include "daemon-service.h"
#include "daemon/daemon-shutdown-watcher.h"
#include "static_threads.h"
#include "common.h"

#include <curl/curl.h>

#ifdef ENABLE_SENTRY
#include "sentry-native/sentry-native.h"
#endif

void web_client_cache_destroy(void);

extern struct netdata_static_thread *static_threads;

void cancel_main_threads(void) {
    nd_log_limits_unlimited();

    if (!static_threads)
        return;

    int i, found = 0;
    usec_t max = 5 * USEC_PER_SEC, step = 100000;
    for (i = 0; static_threads[i].name != NULL ; i++) {
        if (static_threads[i].enabled == NETDATA_MAIN_THREAD_RUNNING) {
            if (static_threads[i].thread) {
                netdata_log_info("EXIT: Stopping main thread: %s", static_threads[i].name);
                nd_thread_signal_cancel(static_threads[i].thread);
            } else {
                netdata_log_info("EXIT: No thread running (marking as EXITED): %s", static_threads[i].name);
                static_threads[i].enabled = NETDATA_MAIN_THREAD_EXITED;
            }
            found++;
        }
    }

    while(found && max > 0) {
        max -= step;
        netdata_log_info("Waiting %d threads to finish...", found);
        sleep_usec(step);
        found = 0;
        for (i = 0; static_threads[i].name != NULL ; i++) {
            if (static_threads[i].enabled == NETDATA_MAIN_THREAD_EXITED)
                continue;

            // Don't wait ourselves.
            if (nd_thread_is_me(static_threads[i].thread))
                continue;

            found++;
        }
    }

    if(found) {
        for (i = 0; static_threads[i].name != NULL ; i++) {
            if (static_threads[i].enabled != NETDATA_MAIN_THREAD_EXITED)
                netdata_log_error("Main thread %s takes too long to exit. Giving up...", static_threads[i].name);
        }
    }
    else
        netdata_log_info("All threads finished.");

    freez(static_threads);
    static_threads = NULL;
}

static void *rrdeng_exit_background(void *ptr) {
    struct rrdengine_instance *ctx = ptr;
    rrdeng_exit(ctx);
    return NULL;
}

#ifdef ENABLE_DBENGINE
static void rrdeng_flush_everything_and_wait(bool wait_flush, bool wait_collectors) {
    static size_t starting_size_to_flush = 0;

    if(!pgc_hot_and_dirty_entries(main_cache))
        return;

    nd_log(NDLS_DAEMON, NDLP_INFO, "Flushing DBENGINE dirty pages...");
    for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
        rrdeng_quiesce(multidb_ctx[tier]);

    struct pgc_statistics pgc_main_stats = pgc_get_statistics(main_cache);
    size_t size_to_flush = pgc_main_stats.queues[PGC_QUEUE_HOT].size + pgc_main_stats.queues[PGC_QUEUE_DIRTY].size;
    size_t entries_to_flush = pgc_main_stats.queues[PGC_QUEUE_HOT].entries + pgc_main_stats.queues[PGC_QUEUE_DIRTY].entries;
    if(size_to_flush > starting_size_to_flush || !starting_size_to_flush)
        starting_size_to_flush = size_to_flush;

    if(wait_collectors) {
        size_t running = 1;
        size_t count = 10;
        while (running && count) {
            running = 0;
            for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
                running += rrdeng_collectors_running(multidb_ctx[tier]);

            if (running) {
                nd_log_limit_static_thread_var(erl, 1, 100 * USEC_PER_MS);
                nd_log_limit(&erl, NDLS_DAEMON, NDLP_NOTICE, "waiting for %zu collectors to finish", running);
            }
            count--;
        }
    }

    if(!wait_flush)
        return;

    for(size_t iterations = 0; true ;iterations++) {
        pgc_main_stats = pgc_get_statistics(main_cache);
        size_to_flush = pgc_main_stats.queues[PGC_QUEUE_HOT].size + pgc_main_stats.queues[PGC_QUEUE_DIRTY].size;
        entries_to_flush = pgc_main_stats.queues[PGC_QUEUE_HOT].entries + pgc_main_stats.queues[PGC_QUEUE_DIRTY].entries;
        if(!starting_size_to_flush || size_to_flush > starting_size_to_flush)
            starting_size_to_flush = size_to_flush;

        if(!size_to_flush || !entries_to_flush)
            break;

        size_t flushed = starting_size_to_flush - size_to_flush;

        if(iterations % 10 == 0) {
            char hot[64], dirty[64];
            size_snprintf(hot, sizeof(hot), pgc_main_stats.queues[PGC_QUEUE_HOT].size, "B", false);
            size_snprintf(dirty, sizeof(hot), pgc_main_stats.queues[PGC_QUEUE_DIRTY].size, "B", false);

            nd_log(NDLS_DAEMON, NDLP_INFO, "DBENGINE: flushing at %.2f%% { hot: %s, dirty: %s }...",
                   (double)flushed * 100.0 / (double)starting_size_to_flush,
                   hot, dirty);
        }
        sleep_usec(100 * USEC_PER_MS);
    }
    nd_log(NDLS_DAEMON, NDLP_INFO, "DBENGINE: flushing completed!");
}
#endif

void netdata_cleanup_and_exit(int ret, const char *action, const char *action_result, const char *action_data) {
    netdata_exit = 1;

#ifdef ENABLE_DBENGINE
    if(!ret && dbengine_enabled)
        // flush all dirty pages asap
        rrdeng_flush_everything_and_wait(false, false);
#endif

    usec_t shutdown_start_time = now_monotonic_usec();
    watcher_shutdown_begin();

    nd_log_limits_unlimited();
    netdata_log_info("NETDATA SHUTDOWN: initializing shutdown with code %d...", ret);

    // send the stat from our caller
    analytics_statistic_t statistic = { action, action_result, action_data };
    analytics_statistic_send(&statistic);

    // notify we are exiting
    statistic = (analytics_statistic_t) {"EXIT", ret?"ERROR":"OK","-"};
    analytics_statistic_send(&statistic);

    char agent_crash_file[FILENAME_MAX + 1];
    char agent_incomplete_shutdown_file[FILENAME_MAX + 1];
    snprintfz(agent_crash_file, FILENAME_MAX, "%s/.agent_crash", netdata_configured_varlib_dir);
    snprintfz(agent_incomplete_shutdown_file, FILENAME_MAX, "%s/.agent_incomplete_shutdown", netdata_configured_varlib_dir);
    (void) rename(agent_crash_file, agent_incomplete_shutdown_file);
    watcher_step_complete(WATCHER_STEP_ID_CREATE_SHUTDOWN_FILE);

    netdata_main_spawn_server_cleanup();
    watcher_step_complete(WATCHER_STEP_ID_DESTROY_MAIN_SPAWN_SERVER);

    watcher_step_complete(WATCHER_STEP_ID_DBENGINE_EXIT_MODE);

    webrtc_close_all_connections();
    watcher_step_complete(WATCHER_STEP_ID_CLOSE_WEBRTC_CONNECTIONS);

    service_signal_exit(SERVICE_MAINTENANCE | ABILITY_DATA_QUERIES | ABILITY_WEB_REQUESTS |
                        ABILITY_STREAMING_CONNECTIONS | SERVICE_ACLK);
    watcher_step_complete(WATCHER_STEP_ID_DISABLE_MAINTENANCE_NEW_QUERIES_NEW_WEB_REQUESTS_NEW_STREAMING_CONNECTIONS_AND_ACLK);

    service_wait_exit(SERVICE_MAINTENANCE, 3 * USEC_PER_SEC);
    watcher_step_complete(WATCHER_STEP_ID_STOP_MAINTENANCE_THREAD);

    service_wait_exit(SERVICE_EXPORTERS | SERVICE_HEALTH | SERVICE_WEB_SERVER | SERVICE_HTTPD, 3 * USEC_PER_SEC);
    watcher_step_complete(WATCHER_STEP_ID_STOP_EXPORTERS_HEALTH_AND_WEB_SERVERS_THREADS);

    stream_threads_cancel();
    service_wait_exit(SERVICE_COLLECTORS | SERVICE_STREAMING, 3 * USEC_PER_SEC);
    watcher_step_complete(WATCHER_STEP_ID_STOP_COLLECTORS_AND_STREAMING_THREADS);

#ifdef ENABLE_DBENGINE
    if(!ret && dbengine_enabled)
        // flush all dirty pages now that all collectors and streaming completed
        rrdeng_flush_everything_and_wait(false, false);
#endif

    service_wait_exit(SERVICE_REPLICATION, 3 * USEC_PER_SEC);
    watcher_step_complete(WATCHER_STEP_ID_STOP_REPLICATION_THREADS);

    ml_stop_threads();
    ml_fini();
    watcher_step_complete(WATCHER_STEP_ID_DISABLE_ML_DETECTION_AND_TRAINING_THREADS);

    service_wait_exit(SERVICE_CONTEXT, 3 * USEC_PER_SEC);
    watcher_step_complete(WATCHER_STEP_ID_STOP_CONTEXT_THREAD);

    web_client_cache_destroy();
    watcher_step_complete(WATCHER_STEP_ID_CLEAR_WEB_CLIENT_CACHE);

    service_wait_exit(SERVICE_ACLK, 3 * USEC_PER_SEC);
    watcher_step_complete(WATCHER_STEP_ID_STOP_ACLK_THREADS);

    service_wait_exit(~0, 10 * USEC_PER_SEC);
    watcher_step_complete(WATCHER_STEP_ID_STOP_ALL_REMAINING_WORKER_THREADS);

    cancel_main_threads();
    watcher_step_complete(WATCHER_STEP_ID_CANCEL_MAIN_THREADS);

    metadata_sync_shutdown_background();
    watcher_step_complete(WATCHER_STEP_ID_PREPARE_METASYNC_SHUTDOWN);

    if (ret)
    {
        watcher_step_complete(WATCHER_STEP_ID_STOP_COLLECTION_FOR_ALL_HOSTS);
        watcher_step_complete(WATCHER_STEP_ID_WAIT_FOR_DBENGINE_COLLECTORS_TO_FINISH);
        watcher_step_complete(WATCHER_STEP_ID_STOP_DBENGINE_TIERS);
        watcher_step_complete(WATCHER_STEP_ID_STOP_METASYNC_THREADS);
    }
    else
    {
        // exit cleanly
        rrd_finalize_collection_for_all_hosts();
        watcher_step_complete(WATCHER_STEP_ID_STOP_COLLECTION_FOR_ALL_HOSTS);

#ifdef ENABLE_DBENGINE
        if(dbengine_enabled) {
            // flush anything remaining and wait for collectors to finish
            rrdeng_flush_everything_and_wait(true, true);
            watcher_step_complete(WATCHER_STEP_ID_WAIT_FOR_DBENGINE_COLLECTORS_TO_FINISH);

            ND_THREAD *th[nd_profile.storage_tiers];
            for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
                th[tier] = nd_thread_create("rrdeng-exit", NETDATA_THREAD_OPTION_JOINABLE, rrdeng_exit_background, multidb_ctx[tier]);

            // flush anything remaining again - just in case
            rrdeng_flush_everything_and_wait(true, false);

            for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
                nd_thread_join(th[tier]);

            rrdeng_enq_cmd(NULL, RRDENG_OPCODE_SHUTDOWN_EVLOOP, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
            watcher_step_complete(WATCHER_STEP_ID_STOP_DBENGINE_TIERS);
        }
        else {
            // Skip these steps
            watcher_step_complete(WATCHER_STEP_ID_WAIT_FOR_DBENGINE_COLLECTORS_TO_FINISH);
            watcher_step_complete(WATCHER_STEP_ID_STOP_DBENGINE_TIERS);
        }
#else
        // Skip these steps
        watcher_step_complete(WATCHER_STEP_ID_WAIT_FOR_DBENGINE_COLLECTORS_TO_FINISH);
        watcher_step_complete(WATCHER_STEP_ID_STOP_DBENGINE_TIERS);
#endif

        metadata_sync_shutdown_background_wait();
        watcher_step_complete(WATCHER_STEP_ID_STOP_METASYNC_THREADS);
    }

    // Don't register a shutdown event if we crashed
    if (!ret)
        add_agent_event(EVENT_AGENT_SHUTDOWN_TIME, (int64_t)(now_monotonic_usec() - shutdown_start_time));
    sqlite_close_databases();
    watcher_step_complete(WATCHER_STEP_ID_CLOSE_SQL_DATABASES);
    sqlite_library_shutdown();


    // unlink the pid
    if(pidfile && *pidfile) {
        if(unlink(pidfile) != 0)
            netdata_log_error("EXIT: cannot unlink pidfile '%s'.", pidfile);
    }
    watcher_step_complete(WATCHER_STEP_ID_REMOVE_PID_FILE);

    netdata_ssl_cleanup();
    watcher_step_complete(WATCHER_STEP_ID_FREE_OPENSSL_STRUCTURES);

    (void) unlink(agent_incomplete_shutdown_file);
    watcher_step_complete(WATCHER_STEP_ID_REMOVE_INCOMPLETE_SHUTDOWN_FILE);

    watcher_shutdown_end();
    watcher_thread_stop();
    curl_global_cleanup();

#ifdef OS_WINDOWS
    return;
#endif

#ifdef ENABLE_SENTRY
    if (ret) {
        abort();
    } else {
        nd_sentry_fini();
        exit(ret);
    }
#else
    exit(ret);
#endif
}
