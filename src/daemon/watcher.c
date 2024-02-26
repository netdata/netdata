// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "watcher.h"

watcher_step_t *watcher_steps;

static struct completion shutdown_begin_completion;
static struct completion shutdown_end_completion;
static netdata_thread_t watcher_thread;

void watcher_shutdown_begin(void) {
    completion_mark_complete(&shutdown_begin_completion);
}

void watcher_shutdown_end(void) {
    completion_mark_complete(&shutdown_end_completion);
}

void watcher_step_complete(watcher_step_id_t step_id) {
    completion_mark_complete(&watcher_steps[step_id].p);
}

void *watcher_main(void *arg)
{
    UNUSED(arg);

    netdata_log_debug(D_SYSTEM, "Watcher thread started");

    completion_wait_for(&shutdown_begin_completion);
    usec_t shutdown_start_time = now_monotonic_usec();

    // TODO:
    //   - add a version of completion_wait_for with timeout
    //   - check the step's duration and abort when the timeout has expired.
    for (int step_id = 0; step_id != WATCHER_STEP_ID_MAX; step_id++) {
        usec_t step_start_time = now_monotonic_usec();
        completion_wait_for(&watcher_steps[step_id].p);
        usec_t step_end_time = now_monotonic_usec();

        usec_t step_duration = step_end_time - step_start_time;
        netdata_log_info("shutdown step: [%d/%d] - '%s' finished in %llu milliseconds",
                         step_id + 1,
                         WATCHER_STEP_ID_MAX,
                         watcher_steps[step_id].msg,
                         step_duration / USEC_PER_MS);
    }

    netdata_log_error("Shutdown process started");

    completion_wait_for(&shutdown_end_completion);
    usec_t shutdown_end_time = now_monotonic_usec();

    usec_t shutdown_duration = shutdown_end_time - shutdown_start_time;
    netdata_log_error("Shutdown process ended in %llu milliseconds",
                      shutdown_duration / USEC_PER_MS);

    return NULL;
}

void watcher_thread_start() {
    watcher_steps = callocz(WATCHER_STEP_ID_MAX, sizeof(watcher_step_t));

    watcher_steps[WATCHER_STEP_ID_CREATE_SHUTDOWN_FILE].msg =
        "create shutdown file";
    watcher_steps[WATCHER_STEP_ID_DBENGINE_EXIT_MODE].msg =
        "dbengine exit mode";
    watcher_steps[WATCHER_STEP_ID_CLOSE_WEBRTC_CONNECTIONS].msg =
        "close webrtc connections";
    watcher_steps[WATCHER_STEP_ID_DISABLE_MAINTENANCE_NEW_QUERIES_NEW_WEB_REQUESTS_NEW_STREAMING_CONNECTIONS_AND_ACLK].msg =
        "disable maintenance, new queries, new web requests, new streaming connections and aclk";
    watcher_steps[WATCHER_STEP_ID_STOP_MAINTENANCE_THREAD].msg =
        "stop maintenance thread";
    watcher_steps[WATCHER_STEP_ID_STOP_EXPORTERS_HEALTH_AND_WEB_SERVERS_THREADS].msg =
        "stop exporters, health and web servers threads";
    watcher_steps[WATCHER_STEP_ID_STOP_COLLECTORS_AND_STREAMING_THREADS].msg =
        "stop collectors and streaming threads";
    watcher_steps[WATCHER_STEP_ID_STOP_REPLICATION_THREADS].msg =
        "stop replication threads";
    watcher_steps[WATCHER_STEP_ID_PREPARE_METASYNC_SHUTDOWN].msg =
        "prepare metasync shutdown";
    watcher_steps[WATCHER_STEP_ID_DISABLE_ML_DETECTION_AND_TRAINING_THREADS].msg =
        "disable ML detection and training threads";
    watcher_steps[WATCHER_STEP_ID_STOP_CONTEXT_THREAD].msg =
        "stop context thread";
    watcher_steps[WATCHER_STEP_ID_CLEAR_WEB_CLIENT_CACHE].msg =
        "clear web client cache";
    watcher_steps[WATCHER_STEP_ID_STOP_ACLK_THREADS].msg =
        "stop aclk threads";
    watcher_steps[WATCHER_STEP_ID_STOP_ALL_REMAINING_WORKER_THREADS].msg =
        "stop all remaining worker threads";
    watcher_steps[WATCHER_STEP_ID_CANCEL_MAIN_THREADS].msg =
        "cancel main threads";
    watcher_steps[WATCHER_STEP_ID_FLUSH_DBENGINE_TIERS].msg =
        "flush dbengine tiers";
    watcher_steps[WATCHER_STEP_ID_STOP_COLLECTION_FOR_ALL_HOSTS].msg =
        "stop collection for all hosts";
    watcher_steps[WATCHER_STEP_ID_STOP_METASYNC_THREADS].msg =
        "stop metasync threads";
    watcher_steps[WATCHER_STEP_ID_WAIT_FOR_DBENGINE_COLLECTORS_TO_FINISH].msg =
        "wait for dbengine collectors to finish";
    watcher_steps[WATCHER_STEP_ID_WAIT_FOR_DBENGINE_MAIN_CACHE_TO_FINISH_FLUSHING].msg =
        "wait for dbengine main cache to finish flushing";
    watcher_steps[WATCHER_STEP_ID_STOP_DBENGINE_TIERS].msg =
        "stop dbengine tiers";
    watcher_steps[WATCHER_STEP_ID_CLOSE_SQL_CONTEXT_DB].msg =
        "close SQL context db";
    watcher_steps[WATCHER_STEP_ID_CLOSE_SQL_MAIN_DB].msg =
        "close SQL main db";
    watcher_steps[WATCHER_STEP_ID_REMOVE_PID_FILE].msg =
        "remove pid file";
    watcher_steps[WATCHER_STEP_ID_FREE_OPENSSL_STRUCTURES].msg =
        "free openssl structures";
    watcher_steps[WATCHER_STEP_ID_REMOVE_INCOMPLETE_SHUTDOWN_FILE].msg =
        "remove incomplete shutdown file";

    for (size_t i = 0; i != WATCHER_STEP_ID_MAX; i++) {
        completion_init(&watcher_steps[i].p);
    }

    completion_init(&shutdown_begin_completion);
    completion_init(&shutdown_end_completion);

    netdata_thread_create(&watcher_thread, "P[WATCHER]", NETDATA_THREAD_OPTION_JOINABLE, watcher_main, NULL);
}

void watcher_thread_stop() {
    netdata_thread_join(watcher_thread, NULL);

    for (size_t i = 0; i != WATCHER_STEP_ID_MAX; i++) {
        completion_destroy(&watcher_steps[i].p);
    }

    completion_destroy(&shutdown_begin_completion);
    completion_destroy(&shutdown_end_completion);

    freez(watcher_steps);
}
