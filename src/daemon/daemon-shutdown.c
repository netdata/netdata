// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon-shutdown.h"
#include "daemon-service.h"
#include "status-file.h"
#include "daemon/daemon-shutdown-watcher.h"
#include "static_threads.h"
#include "common.h"

#include <curl/curl.h>

#ifdef ENABLE_SENTRY
#include "sentry-native/sentry-native.h"
#endif

// External configuration structures that need cleanup
extern struct config netdata_config;
extern struct config cloud_config;

// Functions to free various configurations
void claim_config_free(void);
void rrd_functions_inflight_destroy(void);
void cgroup_netdev_link_destroy(void);
void bearer_tokens_destroy(void);
void alerts_by_x_cleanup(void);
void websocket_threads_join(void);
void mcp_functions_registry_cleanup(void);

static bool abort_on_fatal = true;

void abort_on_fatal_disable(void) {
    abort_on_fatal = false;
}

void abort_on_fatal_enable(void) {
    abort_on_fatal = true;
}

#ifdef ENABLE_SENTRY
NEVER_INLINE
static bool shutdown_on_fatal(void) {
    // keep this as a separate function, to have it logged like this in sentry
    if(abort_on_fatal)
        abort();
    else
        return false;
}
#endif

void web_client_cache_destroy(void);

extern struct netdata_static_thread *static_threads;

void netdata_log_exit_reason(void) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    EXIT_REASON_2buffer(wb, exit_initiated_get(), ", ");

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &netdata_exit_msgid),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_DAEMON, is_exit_reason_normal(exit_initiated_get()) ? NDLP_NOTICE : NDLP_CRIT,
           "NETDATA SHUTDOWN: initializing shutdown with code due to: %s",
           buffer_tostring(wb));
}

void cancel_main_threads(void) {
    nd_log_limits_unlimited();

    if (!static_threads)
        return;

    int i;
    for (i = 0; static_threads[i].name != NULL ; i++) {
        if (static_threads[i].enabled == NETDATA_MAIN_THREAD_RUNNING) {
            if (static_threads[i].thread) {
                netdata_log_info("EXIT: Stopping main thread: %s", static_threads[i].name);
                nd_thread_signal_cancel(static_threads[i].thread);
            } else {
                netdata_log_info("EXIT: No thread running (marking as EXITED): %s", static_threads[i].name);
                static_threads[i].enabled = NETDATA_MAIN_THREAD_EXITED;
            }
        }
    }

    for (i = 0; static_threads[i].name != NULL ; i++) {
        if(static_threads[i].thread && !nd_thread_is_me(static_threads[i].thread)) {
            if (static_threads[i].enabled == NETDATA_MAIN_THREAD_EXITED)
                nd_thread_join(static_threads[i].thread);
        }
    }
    netdata_log_info("All threads finished.");

    freez(static_threads);
    static_threads = NULL;
}

#ifdef ENABLE_DBENGINE
static void rrdeng_exit_background(void *ptr) {
    struct rrdengine_instance *ctx = ptr;
    rrdeng_exit(ctx);
}

static void rrdeng_quiesce_all()
{
    for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
        rrdeng_quiesce(multidb_ctx[tier]);
}

static void rrdeng_flush_everything_and_wait(bool wait_flush, bool wait_collectors, bool dirty_only) {
    static size_t starting_size_to_flush = 0;

    if(!pgc_hot_and_dirty_entries(main_cache))
        return;

    nd_log(NDLS_DAEMON, NDLP_INFO, "Flushing DBENGINE %s dirty pages...", dirty_only ? "only" : "hot &");
    for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
        if (dirty_only)
            rrdeng_flush_dirty(multidb_ctx[tier]);
        else
            rrdeng_flush_all(multidb_ctx[tier]);
    }

    struct pgc_statistics pgc_main_stats = pgc_get_statistics(main_cache);
    size_t size_to_flush = pgc_main_stats.queues[PGC_QUEUE_HOT].size + pgc_main_stats.queues[PGC_QUEUE_DIRTY].size;
    size_t entries_to_flush = pgc_main_stats.queues[PGC_QUEUE_HOT].entries + pgc_main_stats.queues[PGC_QUEUE_DIRTY].entries;
    if(size_to_flush > starting_size_to_flush || !starting_size_to_flush)
        starting_size_to_flush = size_to_flush;

    if(wait_collectors) {
        size_t running = 1;
        size_t count = 50;
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

static void netdata_cleanup_and_exit(EXIT_REASON reason, bool abnormal, bool exit_when_done) {
    exit_initiated_set(reason);

    // don't recurse (due to a fatal, while exiting)
    static bool run = false;
    if(run) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "EXIT: Recursion detected. Exiting immediately.");
        exit(1);
    }
    run = true;
    daemon_status_file_update_status(DAEMON_STATUS_EXITING);

    nd_log_limits_unlimited();
    netdata_log_exit_reason();

    watcher_thread_start();
    usec_t shutdown_start_time = now_monotonic_usec();
    watcher_shutdown_begin();

#ifdef ENABLE_DBENGINE
    if(!abnormal && dbengine_enabled) {
        rrdeng_quiesce_all();
        rrdeng_flush_everything_and_wait(false, false, true);
    }
#endif

    webrtc_close_all_connections();
    watcher_step_complete(WATCHER_STEP_ID_CLOSE_WEBRTC_CONNECTIONS);

    service_signal_exit(ABILITY_WEB_REQUESTS | SERVICE_ACLK | ABILITY_STREAMING_CONNECTIONS | SERVICE_SYSTEMD);

    service_signal_exit(SERVICE_EXPORTERS | SERVICE_HEALTH | SERVICE_WEB_SERVER | SERVICE_HTTPD);

    watcher_step_complete(WATCHER_STEP_ID_DISABLE_MAINTENANCE_NEW_QUERIES_NEW_WEB_REQUESTS_NEW_STREAMING_CONNECTIONS);

    service_wait_exit(SERVICE_SYSTEMD, 5 * USEC_PER_SEC);
    watcher_step_complete(WATCHER_STEP_ID_STOP_MAINTENANCE_THREAD);

    service_wait_exit(SERVICE_EXPORTERS | SERVICE_HEALTH | SERVICE_WEB_SERVER | SERVICE_HTTPD, 3 * USEC_PER_SEC);
    watcher_step_complete(WATCHER_STEP_ID_STOP_EXPORTERS_HEALTH_AND_WEB_SERVERS_THREADS);

    stream_threads_cancel();
    service_wait_exit(SERVICE_COLLECTORS | SERVICE_STREAMING, 20 * USEC_PER_SEC);
    service_signal_exit(SERVICE_STREAMING_CONNECTOR);
    watcher_step_complete(WATCHER_STEP_ID_STOP_COLLECTORS_AND_STREAMING_THREADS);

#ifdef ENABLE_DBENGINE
    if(!abnormal && dbengine_enabled)
        // flush all dirty pages now that all collectors and streaming completed
        rrdeng_flush_everything_and_wait(false, false, true);
#endif

    service_wait_exit(SERVICE_REPLICATION, 5 * USEC_PER_SEC);
    watcher_step_complete(WATCHER_STEP_ID_STOP_REPLICATION_THREADS);

    ml_stop_threads();
    ml_fini();
    watcher_step_complete(WATCHER_STEP_ID_DISABLE_ML_DETEC_AND_TRAIN_THREADS);

    service_wait_exit(SERVICE_CONTEXT, 5 * USEC_PER_SEC);
    watcher_step_complete(WATCHER_STEP_ID_STOP_CONTEXT_THREAD);

    web_client_cache_destroy();
    watcher_step_complete(WATCHER_STEP_ID_CLEAR_WEB_CLIENT_CACHE);

    aclk_synchronization_shutdown();
    watcher_step_complete(WATCHER_STEP_ID_STOP_ACLK_SYNC_THREAD);

    service_signal_exit(SERVICE_ACLK);

    service_wait_exit(SERVICE_ACLK, 3 * USEC_PER_SEC);
    watcher_step_complete(WATCHER_STEP_ID_STOP_ACLK_MQTT_THREAD);

    service_wait_exit(~0, 20 * USEC_PER_SEC);
    watcher_step_complete(WATCHER_STEP_ID_STOP_ALL_REMAINING_WORKER_THREADS);

    cancel_main_threads();
    watcher_step_complete(WATCHER_STEP_ID_CANCEL_MAIN_THREADS);

    if (abnormal) {
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
            rrdeng_flush_everything_and_wait(true, true, false);
            watcher_step_complete(WATCHER_STEP_ID_WAIT_FOR_DBENGINE_COLLECTORS_TO_FINISH);

            ND_THREAD *th[nd_profile.storage_tiers];
            for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
                th[tier] = nd_thread_create("rrdeng-exit", NETDATA_THREAD_OPTION_DEFAULT, rrdeng_exit_background, multidb_ctx[tier]);

            // flush anything remaining again - just in case
            rrdeng_flush_everything_and_wait(true, true, false);

            for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
                nd_thread_join(th[tier]);

            dbengine_shutdown();
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

        metadata_sync_shutdown();
        watcher_step_complete(WATCHER_STEP_ID_STOP_METASYNC_THREADS);
    }

    // Don't register a shutdown event if we crashed
    if (!abnormal)
        add_agent_event(EVENT_AGENT_SHUTDOWN_TIME, (int64_t)(now_monotonic_usec() - shutdown_start_time));

    websocket_threads_join();
    watcher_step_complete(WATCHER_STEP_ID_STOP_WEBSOCKET_THREADS);

    nd_thread_join_threads();
    watcher_step_complete(WATCHER_STEP_ID_JOIN_STATIC_THREADS);

    sqlite_close_databases();
    sqlite_library_shutdown();
    watcher_step_complete(WATCHER_STEP_ID_CLOSE_SQL_DATABASES);

    // unlink the pid
    if(pidfile && *pidfile && unlink(pidfile) != 0)
        netdata_log_error("EXIT: cannot unlink pidfile '%s'.", pidfile);

    // unlink the pipe
    const char *pipe = daemon_pipename();
    if(pipe && *pipe && unlink(pipe) != 0)
        netdata_log_error("EXIT: cannot unlink netdatacli socket file '%s'.", pipe);

    watcher_step_complete(WATCHER_STEP_ID_REMOVE_PID_FILE);

    netdata_ssl_cleanup();
    watcher_step_complete(WATCHER_STEP_ID_FREE_OPENSSL_STRUCTURES);

    watcher_shutdown_end();
    watcher_thread_stop();

#if defined(FSANITIZE_ADDRESS)
    fprintf(stderr, "\n");

    fprintf(stderr, "Stopping spawn server...\n");
    netdata_main_spawn_server_cleanup();

    fprintf(stderr, "Freeing all RRDHOSTs...\n");
    mcp_functions_registry_cleanup();
    rrdhost_free_all();
    dyncfg_shutdown();
    rrd_functions_inflight_destroy();
    health_plugin_destroy();
    cgroup_netdev_link_destroy();
    bearer_tokens_destroy();

    fprintf(stderr, "Cleaning up destroyed dictionaries...\n");
    size_t dictionaries_referenced = cleanup_destroyed_dictionaries(true);
    if(dictionaries_referenced)
        fprintf(stderr, "WARNING: There are %zu dictionaries with references in them, that cannot be destroyed.\n",
                dictionaries_referenced);
                
    // Always report dictionary allocations during ASAN builds
    dictionary_print_still_allocated_stacktraces();

#ifdef ENABLE_DBENGINE
    // destroy the caches in reverse order (extent and open depend on main cache)
    fprintf(stderr, "Destroying extent cache (PGC)...\n");
    pgc_destroy(extent_cache, false);
    fprintf(stderr, "Destroying open cache (PGC)...\n");
    pgc_destroy(open_cache, false);
    fprintf(stderr, "Destroying main cache (PGC)...\n");
    pgc_destroy(main_cache, false);

    fprintf(stderr, "Destroying metrics registry (MRG)...\n");
    size_t metrics_referenced = mrg_destroy(main_mrg);
    if(metrics_referenced)
        fprintf(stderr, "WARNING: MRG had %zu metrics referenced.\n",
            metrics_referenced);

    for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
        if(multidb_ctx[tier]) {
            fprintf(stderr, "Finalizing data files for tier %zu...\n", tier);
            finalize_rrd_files(multidb_ctx[tier]);
            memset(multidb_ctx[tier], 0, sizeof(*multidb_ctx[tier]));
        }
    }
#endif

    fprintf(stderr, "Destroying UUIDMap...\n");
    size_t uuid_referenced = uuidmap_destroy();
    if(uuid_referenced)
        fprintf(stderr, "WARNING: UUIDMAP had %zu UUIDs referenced.\n",
            uuid_referenced);

    fprintf(stderr, "Freeing configuration resources...\n");
    claim_config_free();
    exporting_config_free();
    stream_config_free();
    inicfg_free(&cloud_config);
    inicfg_free(&netdata_config);

    fprintf(stderr, "Cleaning up worker utilization...\n");
    worker_utilization_cleanup();

    alerts_by_x_cleanup();
    size_t strings_referenced = string_destroy();
    if(strings_referenced)
        fprintf(stderr, "WARNING: STRING has %zu strings still allocated.\n",
                strings_referenced);

    rrdlabels_aral_destroy(true);
    fprintf(stderr, "RRDLABELS remaining in registry: %d.\n", rrdlabels_registry_count());

    fprintf(stderr, "All done, exiting...\n");
#endif

    if(!exit_when_done) {
        curl_global_cleanup();
        return;
    }

#ifdef ENABLE_SENTRY
    if(abnormal)
        shutdown_on_fatal();

    nd_sentry_fini();
    curl_global_cleanup();
    exit(abnormal ? 1 : 0);
#else
    if(abnormal)
        _exit(1);
    else {
        curl_global_cleanup();
        exit(0);
    }
#endif
}

void netdata_exit_gracefully(EXIT_REASON reason, bool exit_when_done) {
    exit_initiated_add(reason);
    FUNCTION_RUN_ONCE();
    netdata_cleanup_and_exit(reason, false, exit_when_done);
}

// the final callback for the fatal() function
void netdata_exit_fatal(void) {
    netdata_cleanup_and_exit(EXIT_REASON_FATAL, true, true);
    exit(1);
}
