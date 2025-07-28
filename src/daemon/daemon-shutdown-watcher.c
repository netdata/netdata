// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon-shutdown-watcher.h"
#include "status-file.h"

#ifdef ENABLE_SENTRY
#include "sentry-native/sentry-native.h"
#endif

watcher_step_t *watcher_steps;

static struct completion shutdown_begin_completion;
static struct completion shutdown_end_completion;
static ND_THREAD *watcher_thread;

static BUFFER *steps_timings = NULL;

NEVER_INLINE
static void shutdown_timed_out(void) {
    // keep this as a separate function, to have it logged like this in sentry
    daemon_status_file_shutdown_timeout(steps_timings);
#ifdef ENABLE_SENTRY
    nd_sentry_add_shutdown_timeout_as_breadcrumb();
#endif
    abort();
}

void watcher_shutdown_begin(void) {
    completion_mark_complete(&shutdown_begin_completion);
}

void watcher_shutdown_end(void) {
    completion_mark_complete(&shutdown_end_completion);
}

void watcher_step_complete(watcher_step_id_t step_id) {
    completion_mark_complete(&watcher_steps[step_id].p);
}

static void watcher_wait_for_step(const watcher_step_id_t step_id, usec_t shutdown_start_time)
{
    if(!steps_timings) {
        steps_timings = buffer_create(0, NULL);
        buffer_strcat(steps_timings, STACK_TRACE_INFO_PREFIX " shutdown steps timings");
    }

    usec_t step_start_time = now_monotonic_usec();
    usec_t step_start_duration = step_start_time - shutdown_start_time;

    char start_duration_txt[64];
    duration_snprintf(
        start_duration_txt, sizeof(start_duration_txt), (int64_t)step_start_duration, "us", true);

    netdata_log_info("shutdown step: [%d/%d] - {at %s} started '%s'...",
                     (int)step_id + 1, (int)WATCHER_STEP_ID_MAX, start_duration_txt,
                     watcher_steps[step_id].msg);

#if defined(FSANITIZE_ADDRESS)
    fprintf(stdout, " > shutdown step: [%d/%d] - {at %s} started '%s'...\n",
            (int)step_id + 1, (int)WATCHER_STEP_ID_MAX, start_duration_txt,
            watcher_steps[step_id].msg);
#endif

    daemon_status_file_shutdown_step(watcher_steps[step_id].msg, buffer_tostring(steps_timings));

    // Wait with a timeout
    time_t timeout = 135; // systemd gives us 150, we timeout at 135

    time_t remaining_seconds = timeout - (time_t)(step_start_duration / USEC_PER_SEC);
    if(remaining_seconds < 0)
        remaining_seconds = 0;

#if defined(FSANITIZE_ADDRESS)
    completion_wait_for(&watcher_steps[step_id].p);
    bool ok = true;
#else
    bool ok = completion_timedwait_for(&watcher_steps[step_id].p, remaining_seconds);
#endif

    usec_t step_duration = now_monotonic_usec() - step_start_time;

    char step_duration_txt[64];
    duration_snprintf(
        step_duration_txt, sizeof(step_duration_txt), (int64_t)(step_duration), "us", true);

    buffer_sprintf(steps_timings, "\n#%u '%s': %s", step_id + 1, watcher_steps[step_id].msg, step_duration_txt);

    if (ok) {
        netdata_log_info("shutdown step: [%d/%d] - {at %s} finished '%s' in %s",
                         (int)step_id + 1, (int)WATCHER_STEP_ID_MAX, start_duration_txt,
                         watcher_steps[step_id].msg, step_duration_txt);

#if defined(FSANITIZE_ADDRESS)
        fprintf(stdout, " > shutdown step: [%d/%d] - {at %s} finished '%s' in %s\n",
                (int)step_id + 1, (int)WATCHER_STEP_ID_MAX, start_duration_txt,
                watcher_steps[step_id].msg, step_duration_txt);
#endif
    } else {
        // Do not call fatal() because it will try to execute the exit
        // sequence twice.
        netdata_log_error("shutdown step: [%d/%d] - {at %s} timeout '%s' takes too long (%s) - giving up...",
                          (int)step_id + 1, (int)WATCHER_STEP_ID_MAX, start_duration_txt,
                          watcher_steps[step_id].msg, step_duration_txt);

#if defined(FSANITIZE_ADDRESS)
        fprintf(stdout, "shutdown step: [%d/%d] - {at %s} timeout '%s' takes too long (%s) - giving up...\n",
                (int)step_id + 1, (int)WATCHER_STEP_ID_MAX, start_duration_txt,
                watcher_steps[step_id].msg, step_duration_txt);
#endif

        shutdown_timed_out();
    }
}

void watcher_main(void *arg)
{
    UNUSED(arg);

    netdata_log_debug(D_SYSTEM, "Watcher thread started");

    // wait until the agent starts the shutdown process
    completion_wait_for(&shutdown_begin_completion);
    netdata_log_info("Shutdown process started");

    usec_t shutdown_start_time = now_monotonic_usec();

    watcher_wait_for_step(WATCHER_STEP_ID_CLOSE_WEBRTC_CONNECTIONS, shutdown_start_time);
    watcher_wait_for_step(
        WATCHER_STEP_ID_DISABLE_MAINTENANCE_NEW_QUERIES_NEW_WEB_REQUESTS_NEW_STREAMING_CONNECTIONS, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_STOP_MAINTENANCE_THREAD, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_STOP_EXPORTERS_HEALTH_AND_WEB_SERVERS_THREADS, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_STOP_COLLECTORS_AND_STREAMING_THREADS, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_STOP_REPLICATION_THREADS, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_DISABLE_ML_DETEC_AND_TRAIN_THREADS, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_STOP_CONTEXT_THREAD, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_CLEAR_WEB_CLIENT_CACHE, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_STOP_ACLK_SYNC_THREAD, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_STOP_ACLK_MQTT_THREAD, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_STOP_ALL_REMAINING_WORKER_THREADS, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_CANCEL_MAIN_THREADS, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_STOP_COLLECTION_FOR_ALL_HOSTS, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_WAIT_FOR_DBENGINE_COLLECTORS_TO_FINISH, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_STOP_DBENGINE_TIERS, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_STOP_METASYNC_THREADS, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_STOP_WEBSOCKET_THREADS, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_JOIN_STATIC_THREADS, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_CLOSE_SQL_DATABASES, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_REMOVE_PID_FILE, shutdown_start_time);
    watcher_wait_for_step(WATCHER_STEP_ID_FREE_OPENSSL_STRUCTURES, shutdown_start_time);

    completion_wait_for(&shutdown_end_completion);
    usec_t shutdown_end_time = now_monotonic_usec();

    usec_t shutdown_duration = shutdown_end_time - shutdown_start_time;

    char shutdown_timing[64];
    duration_snprintf(shutdown_timing, sizeof(shutdown_timing), (int64_t)shutdown_duration, "us", 1);
    netdata_log_info("Shutdown process ended in %s", shutdown_timing);

    daemon_status_file_shutdown_step(NULL, buffer_tostring(steps_timings));
    daemon_status_file_update_status(DAEMON_STATUS_EXITED);
}

void watcher_thread_start() {
    watcher_steps = callocz(WATCHER_STEP_ID_MAX, sizeof(watcher_step_t));

    watcher_steps[WATCHER_STEP_ID_CLOSE_WEBRTC_CONNECTIONS].msg = "close webrtc connections";
    watcher_steps[WATCHER_STEP_ID_DISABLE_MAINTENANCE_NEW_QUERIES_NEW_WEB_REQUESTS_NEW_STREAMING_CONNECTIONS]
        .msg = "disable maintenance, new queries, new web requests, new streaming connections and aclk";
    watcher_steps[WATCHER_STEP_ID_STOP_MAINTENANCE_THREAD].msg = "stop maintenance thread";
    watcher_steps[WATCHER_STEP_ID_STOP_EXPORTERS_HEALTH_AND_WEB_SERVERS_THREADS].msg =
        "stop exporters, health and web servers threads";
    watcher_steps[WATCHER_STEP_ID_STOP_COLLECTORS_AND_STREAMING_THREADS].msg = "stop collectors and streaming threads";
    watcher_steps[WATCHER_STEP_ID_STOP_REPLICATION_THREADS].msg = "stop replication threads";
    watcher_steps[WATCHER_STEP_ID_DISABLE_ML_DETEC_AND_TRAIN_THREADS].msg = "disable ML detection and training threads";
    watcher_steps[WATCHER_STEP_ID_STOP_CONTEXT_THREAD].msg = "stop context thread";
    watcher_steps[WATCHER_STEP_ID_CLEAR_WEB_CLIENT_CACHE].msg = "clear web client cache";
    watcher_steps[WATCHER_STEP_ID_STOP_ACLK_SYNC_THREAD].msg = "stop ACLK sync thread";
    watcher_steps[WATCHER_STEP_ID_STOP_ACLK_MQTT_THREAD].msg = "stop ACLK MQTT connection thread";
    watcher_steps[WATCHER_STEP_ID_STOP_ALL_REMAINING_WORKER_THREADS].msg = "stop all remaining worker threads";
    watcher_steps[WATCHER_STEP_ID_CANCEL_MAIN_THREADS].msg = "cancel main threads";
    watcher_steps[WATCHER_STEP_ID_STOP_COLLECTION_FOR_ALL_HOSTS].msg = "stop collection for all hosts";
    watcher_steps[WATCHER_STEP_ID_WAIT_FOR_DBENGINE_COLLECTORS_TO_FINISH].msg =
        "wait for dbengine collectors to finish";
    watcher_steps[WATCHER_STEP_ID_STOP_DBENGINE_TIERS].msg = "stop dbengine tiers";
    watcher_steps[WATCHER_STEP_ID_STOP_METASYNC_THREADS].msg = "stop metasync threads";
    watcher_steps[WATCHER_STEP_ID_STOP_WEBSOCKET_THREADS].msg = "stop websocket threads";
    watcher_steps[WATCHER_STEP_ID_JOIN_STATIC_THREADS].msg = "join static threads";
    watcher_steps[WATCHER_STEP_ID_CLOSE_SQL_DATABASES].msg = "close SQL databases";
    watcher_steps[WATCHER_STEP_ID_REMOVE_PID_FILE].msg = "remove pid file";
    watcher_steps[WATCHER_STEP_ID_FREE_OPENSSL_STRUCTURES].msg = "free openssl structures";

    for (size_t i = 0; i != WATCHER_STEP_ID_MAX; i++) {
        completion_init(&watcher_steps[i].p);
    }

    completion_init(&shutdown_begin_completion);
    completion_init(&shutdown_end_completion);

    watcher_thread = nd_thread_create("EXIT_WATCHER", NETDATA_THREAD_OPTION_DEFAULT, watcher_main, NULL);
}

void watcher_thread_stop() {
    nd_thread_join(watcher_thread);

    for (size_t i = 0; i != WATCHER_STEP_ID_MAX; i++) {
        completion_destroy(&watcher_steps[i].p);
    }

    completion_destroy(&shutdown_begin_completion);
    completion_destroy(&shutdown_end_completion);

    freez(watcher_steps);
}
