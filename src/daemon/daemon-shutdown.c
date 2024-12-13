// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon-shutdown.h"
#include "daemon-service.h"
#include "daemon/daemon-shutdown-watcher.h"
#include "static_threads.h"
#include "common.h"

#include <curl/curl.h>

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

void netdata_cleanup_and_exit(int ret, const char *action, const char *action_result, const char *action_data) {
    nd_log_limit_static_thread_var(erl, 1, 100 * USEC_PER_MS);

    netdata_exit = 1;

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

#ifdef ENABLE_DBENGINE
    if(dbengine_enabled) {
        for (size_t tier = 0; tier < storage_tiers; tier++)
            rrdeng_exit_mode(multidb_ctx[tier]);
    }
#endif
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
        watcher_step_complete(WATCHER_STEP_ID_FLUSH_DBENGINE_TIERS);
        watcher_step_complete(WATCHER_STEP_ID_STOP_COLLECTION_FOR_ALL_HOSTS);
        watcher_step_complete(WATCHER_STEP_ID_WAIT_FOR_DBENGINE_COLLECTORS_TO_FINISH);
        watcher_step_complete(WATCHER_STEP_ID_WAIT_FOR_DBENGINE_MAIN_CACHE_TO_FINISH_FLUSHING);
        watcher_step_complete(WATCHER_STEP_ID_STOP_DBENGINE_TIERS);
        watcher_step_complete(WATCHER_STEP_ID_STOP_METASYNC_THREADS);
    }
    else
    {
        // exit cleanly

#ifdef ENABLE_DBENGINE
        if(dbengine_enabled) {
            nd_log(NDLS_DAEMON, NDLP_INFO, "Flushing DBENGINE dirty pages...");
            for (size_t tier = 0; tier < storage_tiers; tier++)
                rrdeng_prepare_exit(multidb_ctx[tier]);

            struct pgc_statistics pgc_main_stats = pgc_get_statistics(main_cache);
            nd_log(NDLS_DAEMON, NDLP_INFO, "Waiting for DBENGINE to commit unsaved data to disk (%zu pages, %zu bytes)...",
                   pgc_main_stats.queues[PGC_QUEUE_HOT].entries + pgc_main_stats.queues[PGC_QUEUE_DIRTY].entries,
                   pgc_main_stats.queues[PGC_QUEUE_HOT].size + pgc_main_stats.queues[PGC_QUEUE_DIRTY].size);

            bool finished_tiers[RRD_STORAGE_TIERS] = { 0 };
            size_t waiting_tiers, iterations = 0;
            do {
                waiting_tiers = 0;
                iterations++;

                for (size_t tier = 0; tier < storage_tiers; tier++) {
                    if (!multidb_ctx[tier] || finished_tiers[tier])
                        continue;

                    waiting_tiers++;
                    if (completion_timedwait_for(&multidb_ctx[tier]->quiesce.completion, 1)) {
                        completion_destroy(&multidb_ctx[tier]->quiesce.completion);
                        finished_tiers[tier] = true;
                        waiting_tiers--;
                        nd_log(NDLS_DAEMON, NDLP_INFO, "DBENGINE tier %zu flushed dirty pages!", tier);
                    }
                    else if(iterations % 10 == 0) {
                        pgc_main_stats = pgc_get_statistics(main_cache);
                        nd_log(NDLS_DAEMON, NDLP_INFO,
                               "Still waiting for DBENGINE tier %zu to flush its dirty pages "
                               "(cache pages { hot: %zu, dirty: %zu }, size { hot: %zu, dirty: %zu })...",
                               tier,
                               pgc_main_stats.queues[PGC_QUEUE_HOT].entries, pgc_main_stats.queues[PGC_QUEUE_DIRTY].entries,
                               pgc_main_stats.queues[PGC_QUEUE_HOT].size, pgc_main_stats.queues[PGC_QUEUE_DIRTY].size);
                    }
                }
            } while(waiting_tiers);
            nd_log(NDLS_DAEMON, NDLP_INFO, "DBENGINE flushing of dirty pages completed...");
        }
#endif
        watcher_step_complete(WATCHER_STEP_ID_FLUSH_DBENGINE_TIERS);

        rrd_finalize_collection_for_all_hosts();
        watcher_step_complete(WATCHER_STEP_ID_STOP_COLLECTION_FOR_ALL_HOSTS);

#ifdef ENABLE_DBENGINE
        if(dbengine_enabled) {
            size_t running = 1;
            size_t count = 10;
            while(running && count) {
                running = 0;
                for (size_t tier = 0; tier < storage_tiers; tier++)
                    running += rrdeng_collectors_running(multidb_ctx[tier]);

                if (running) {
                    nd_log_limit(&erl, NDLS_DAEMON, NDLP_NOTICE, "waiting for %zu collectors to finish", running);
                }
                count--;
            }
            watcher_step_complete(WATCHER_STEP_ID_WAIT_FOR_DBENGINE_COLLECTORS_TO_FINISH);

            while (pgc_hot_and_dirty_entries(main_cache)) {
                pgc_flush_all_hot_and_dirty_pages(main_cache, PGC_SECTION_ALL);
                sleep_usec(100 * USEC_PER_MS);
            }
            watcher_step_complete(WATCHER_STEP_ID_WAIT_FOR_DBENGINE_MAIN_CACHE_TO_FINISH_FLUSHING);

            ND_THREAD *th[storage_tiers];
            for (size_t tier = 0; tier < storage_tiers; tier++)
                th[tier] = nd_thread_create("rrdeng-exit", NETDATA_THREAD_OPTION_JOINABLE, rrdeng_exit_background, multidb_ctx[tier]);

            size_t iterations = 0;
            do {
                struct pgc_statistics pgc_main_stats = pgc_get_statistics(main_cache);
                if(iterations % 10 == 0)
                    nd_log_limit(&erl, NDLS_DAEMON, NDLP_INFO,
                                 "DBENGINE flushing threads currently running: %zu "
                                 "(cache pages { hot: %zu, dirty: %zu }, size { hot: %zu, dirty: %zu })...",
                                 pgc_main_stats.workers_flush,
                                 pgc_main_stats.queues[PGC_QUEUE_HOT].entries, pgc_main_stats.queues[PGC_QUEUE_DIRTY].entries,
                                 pgc_main_stats.queues[PGC_QUEUE_HOT].size, pgc_main_stats.queues[PGC_QUEUE_DIRTY].size);

                if(!pgc_main_stats.workers_flush)
                        break;
                else
                    yield_the_processor();

                iterations++;
            } while(true);

            struct pgc_statistics pgc_main_stats = pgc_get_statistics(main_cache);
            nd_log_limit(&erl, NDLS_DAEMON, NDLP_INFO,
                         "DBENGINE flushing threads currently running: %zu "
                         "(cache pages { hot: %zu, dirty: %zu }, size { hot: %zu, dirty: %zu })...",
                         pgc_main_stats.workers_flush,
                         pgc_main_stats.queues[PGC_QUEUE_HOT].entries, pgc_main_stats.queues[PGC_QUEUE_DIRTY].entries,
                         pgc_main_stats.queues[PGC_QUEUE_HOT].size, pgc_main_stats.queues[PGC_QUEUE_DIRTY].size);

            for (size_t tier = 0; tier < storage_tiers; tier++)
                nd_thread_join(th[tier]);

            rrdeng_enq_cmd(NULL, RRDENG_OPCODE_SHUTDOWN_EVLOOP, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
            watcher_step_complete(WATCHER_STEP_ID_STOP_DBENGINE_TIERS);
        }
        else {
            // Skip these steps
            watcher_step_complete(WATCHER_STEP_ID_WAIT_FOR_DBENGINE_COLLECTORS_TO_FINISH);
            watcher_step_complete(WATCHER_STEP_ID_WAIT_FOR_DBENGINE_MAIN_CACHE_TO_FINISH_FLUSHING);
            watcher_step_complete(WATCHER_STEP_ID_STOP_DBENGINE_TIERS);
        }
#else
        // Skip these steps
        watcher_step_complete(WATCHER_STEP_ID_WAIT_FOR_DBENGINE_COLLECTORS_TO_FINISH);
        watcher_step_complete(WATCHER_STEP_ID_WAIT_FOR_DBENGINE_MAIN_CACHE_TO_FINISH_FLUSHING);
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
    nd_sentry_fini();
#endif

    exit(ret);
}
