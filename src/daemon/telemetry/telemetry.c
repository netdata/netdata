// SPDX-License-Identifier: GPL-3.0-or-later

#define TELEMETRY_INTERNALS 1
#include "daemon/common.h"

#define WORKER_JOB_TELEMETRY_DAEMON      0
#define WORKER_JOB_SQLITE3               1
#define WORKER_JOB_TELEMETRY_HTTP_API    2
#define WORKER_JOB_TELEMETRY_QUERIES     3
#define WORKER_JOB_TELEMETRY_INGESTION   4
#define WORKER_JOB_DBENGINE              5
#define WORKER_JOB_STRINGS               6
#define WORKER_JOB_DICTIONARIES          7
#define WORKER_JOB_TELEMETRY_ML          8
#define WORKER_JOB_TELEMETRY_GORILLA     9
#define WORKER_JOB_HEARTBEAT            10
#define WORKER_JOB_WORKERS              11
#define WORKER_JOB_MALLOC_TRACE         12
#define WORKER_JOB_REGISTRY             13

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 14
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 14
#endif

bool telemetry_enabled = true;
bool telemetry_extended_enabled = false;

static void telemetry_register_workers(void) {
    worker_register("STATS");

    worker_register_job_name(WORKER_JOB_TELEMETRY_DAEMON, "daemon");
    worker_register_job_name(WORKER_JOB_SQLITE3, "sqlite3");
    worker_register_job_name(WORKER_JOB_TELEMETRY_HTTP_API, "http-api");
    worker_register_job_name(WORKER_JOB_TELEMETRY_QUERIES, "queries");
    worker_register_job_name(WORKER_JOB_TELEMETRY_INGESTION, "ingestion");
    worker_register_job_name(WORKER_JOB_DBENGINE, "dbengine");
    worker_register_job_name(WORKER_JOB_STRINGS, "strings");
    worker_register_job_name(WORKER_JOB_DICTIONARIES, "dictionaries");
    worker_register_job_name(WORKER_JOB_TELEMETRY_ML, "ML");
    worker_register_job_name(WORKER_JOB_TELEMETRY_GORILLA, "gorilla");
    worker_register_job_name(WORKER_JOB_HEARTBEAT, "heartbeat");
    worker_register_job_name(WORKER_JOB_WORKERS, "workers");
    worker_register_job_name(WORKER_JOB_MALLOC_TRACE, "malloc_trace");
    worker_register_job_name(WORKER_JOB_REGISTRY, "registry");
}

static void telementry_cleanup(void *pptr)
{
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    telemetry_workers_cleanup();
    worker_unregister();
    netdata_log_info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *telemetry_thread_main(void *ptr) {
    CLEANUP_FUNCTION_REGISTER(telementry_cleanup) cleanup_ptr = ptr;
    telemetry_register_workers();

    int update_every =
        (int)config_get_duration_seconds(CONFIG_SECTION_TELEMETRY, "update every", localhost->rrd_update_every);
    if (update_every < localhost->rrd_update_every) {
        update_every = localhost->rrd_update_every;
        config_set_duration_seconds(CONFIG_SECTION_TELEMETRY, "update every", update_every);
    }

    usec_t step = update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    usec_t real_step = USEC_PER_SEC;

    // keep the randomness at zero
    // to make sure we are not close to any other thread
    hb.randomness = 0;

    while (service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();
        heartbeat_next(&hb);
        if (real_step < step) {
            real_step += USEC_PER_SEC;
            continue;
        }
        real_step = USEC_PER_SEC;

        worker_is_busy(WORKER_JOB_TELEMETRY_DAEMON);
        telemetry_daemon_do(telemetry_extended_enabled);

        worker_is_busy(WORKER_JOB_TELEMETRY_INGESTION);
        telemetry_ingestion_do(telemetry_extended_enabled);

        worker_is_busy(WORKER_JOB_TELEMETRY_HTTP_API);
        telemetry_web_do(telemetry_extended_enabled);

        worker_is_busy(WORKER_JOB_TELEMETRY_QUERIES);
        telemetry_queries_do(telemetry_extended_enabled);

        worker_is_busy(WORKER_JOB_TELEMETRY_ML);
        telemetry_ml_do(telemetry_extended_enabled);

        worker_is_busy(WORKER_JOB_TELEMETRY_GORILLA);
        telemetry_gorilla_do(telemetry_extended_enabled);

        worker_is_busy(WORKER_JOB_HEARTBEAT);
        telemetry_heartbeat_do(telemetry_extended_enabled);

#ifdef ENABLE_DBENGINE
        if(dbengine_enabled) {
            worker_is_busy(WORKER_JOB_DBENGINE);
            telemetry_dbengine_do(telemetry_extended_enabled);
        }
#endif

        worker_is_busy(WORKER_JOB_REGISTRY);
        registry_statistics();

        worker_is_busy(WORKER_JOB_STRINGS);
        telemetry_string_do(telemetry_extended_enabled);

#ifdef DICT_WITH_STATS
        worker_is_busy(WORKER_JOB_DICTIONARIES);
        telemetry_dictionary_do(telemetry_extended_enabled);
#endif

#ifdef NETDATA_TRACE_ALLOCATIONS
        worker_is_busy(WORKER_JOB_MALLOC_TRACE);
        telemetry_trace_allocations_do(telemetry_extended_enabled);
#endif

        worker_is_busy(WORKER_JOB_WORKERS);
        telemetry_workers_do(telemetry_extended_enabled);
    }

    return NULL;
}

// ---------------------------------------------------------------------------------------------------------------------
// telemetry extended thread

static void telemetry_thread_sqlite3_cleanup(void *pptr)
{
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!static_thread)
        return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    netdata_log_info("cleaning up...");

    worker_unregister();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *telemetry_thread_sqlite3_main(void *ptr) {
    CLEANUP_FUNCTION_REGISTER(telemetry_thread_sqlite3_cleanup) cleanup_ptr = ptr;
    telemetry_register_workers();

    int update_every =
        (int)config_get_duration_seconds(CONFIG_SECTION_TELEMETRY, "update every", localhost->rrd_update_every);
    if (update_every < localhost->rrd_update_every) {
        update_every = localhost->rrd_update_every;
        config_set_duration_seconds(CONFIG_SECTION_TELEMETRY, "update every", update_every);
    }

    usec_t step = update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    usec_t real_step = USEC_PER_SEC;

    while (service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();
        heartbeat_next(&hb);
        if (real_step < step) {
            real_step += USEC_PER_SEC;
            continue;
        }
        real_step = USEC_PER_SEC;

        worker_is_busy(WORKER_JOB_SQLITE3);
        telemetry_sqlite3_do(telemetry_extended_enabled);
    }

    return NULL;
}
