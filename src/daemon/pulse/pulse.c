// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "daemon/common.h"

#define WORKER_JOB_DAEMON                0
#define WORKER_JOB_SQLITE3               1
#define WORKER_JOB_HTTP_API              2
#define WORKER_JOB_QUERIES               3
#define WORKER_JOB_INGESTION             4
#define WORKER_JOB_DBENGINE              5
#define WORKER_JOB_STRINGS               6
#define WORKER_JOB_DICTIONARIES          7
#define WORKER_JOB_ML                    8
#define WORKER_JOB_GORILLA               9
#define WORKER_JOB_HEARTBEAT            10
#define WORKER_JOB_WORKERS              11
#define WORKER_JOB_MALLOC_TRACE         12
#define WORKER_JOB_REGISTRY             13
#define WORKER_JOB_ARAL                 14
#define WORKER_JOB_NETWORK              15
#define WORKER_JOB_PARENTS              16
#define WORKER_JOB_MEMORY_EXTENDED      17

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 18
#error "WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 18"
#endif

bool pulse_enabled = true;
bool pulse_extended_enabled = false;

static void pulse_register_workers(void) {
    worker_register("PULSE");

    worker_register_job_name(WORKER_JOB_DAEMON, "daemon");
    worker_register_job_name(WORKER_JOB_SQLITE3, "sqlite3");
    worker_register_job_name(WORKER_JOB_HTTP_API, "http-api");
    worker_register_job_name(WORKER_JOB_QUERIES, "queries");
    worker_register_job_name(WORKER_JOB_INGESTION, "ingestion");
    worker_register_job_name(WORKER_JOB_DBENGINE, "dbengine");
    worker_register_job_name(WORKER_JOB_STRINGS, "strings");
    worker_register_job_name(WORKER_JOB_DICTIONARIES, "dictionaries");
    worker_register_job_name(WORKER_JOB_ML, "ML");
    worker_register_job_name(WORKER_JOB_GORILLA, "gorilla");
    worker_register_job_name(WORKER_JOB_HEARTBEAT, "heartbeat");
    worker_register_job_name(WORKER_JOB_WORKERS, "workers");
    worker_register_job_name(WORKER_JOB_MALLOC_TRACE, "malloc trace");
    worker_register_job_name(WORKER_JOB_REGISTRY, "registry");
    worker_register_job_name(WORKER_JOB_ARAL, "aral");
    worker_register_job_name(WORKER_JOB_NETWORK, "network");
    worker_register_job_name(WORKER_JOB_PARENTS, "parents");
    worker_register_job_name(WORKER_JOB_MEMORY_EXTENDED, "memory extended");
}

// ---------------------------------------------------------------------------------------------------------------------
// the interval all pulse threads run at

// [pulse].update every, never below the localhost update every.
// The pulse threads start concurrently, so the config is read once, under a lock.
int pulse_update_every(void) {
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    static int update_every = 0;

    spinlock_lock(&spinlock);
    if(!update_every) {
        update_every = (int)inicfg_get_duration_seconds(
            &netdata_config, CONFIG_SECTION_PULSE, "update every", localhost->rrd_update_every);

        if(update_every < localhost->rrd_update_every) {
            update_every = localhost->rrd_update_every;
            inicfg_set_duration_seconds(&netdata_config, CONFIG_SECTION_PULSE, "update every", update_every);
        }
    }
    int ret = update_every;
    spinlock_unlock(&spinlock);

    return ret;
}

// ---------------------------------------------------------------------------------------------------------------------
// the loop all pulse threads run

// Registers the PULSE worker, calls init() once (if given), then do_once() every pulse_update_every() seconds,
// until the agent stops collecting. cleanup() (if given) runs while the thread is exiting.
static void pulse_thread_loop(struct netdata_static_thread *static_thread, void (*init)(void), void (*do_once)(void), void (*cleanup)(void)) {
    pulse_register_workers();

    if(init)
        init();

    usec_t step = pulse_update_every() * USEC_PER_SEC;
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

        do_once();
    }

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    if(cleanup)
        cleanup();

    worker_unregister();
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

// ---------------------------------------------------------------------------------------------------------------------
// pulse main thread

static void pulse_main_init(void) {
    pulse_aral_init();
    aclk_time_histogram_init();
}

static void pulse_main_do_once(void) {
    worker_is_busy(WORKER_JOB_INGESTION);
    pulse_ingestion_do(pulse_extended_enabled);

    worker_is_busy(WORKER_JOB_HTTP_API);
    pulse_web_do(pulse_extended_enabled);

    worker_is_busy(WORKER_JOB_QUERIES);
    pulse_queries_do(pulse_extended_enabled);

    worker_is_busy(WORKER_JOB_NETWORK);
    pulse_network_do(pulse_extended_enabled);

    worker_is_busy(WORKER_JOB_ML);
    pulse_ml_do(pulse_extended_enabled);

    worker_is_busy(WORKER_JOB_GORILLA);
    pulse_gorilla_do(pulse_extended_enabled);

    worker_is_busy(WORKER_JOB_HEARTBEAT);
    pulse_heartbeat_do(pulse_extended_enabled);

#ifdef ENABLE_DBENGINE
    if(dbengine_enabled) {
        worker_is_busy(WORKER_JOB_DBENGINE);
        pulse_dbengine_do(pulse_extended_enabled);
        dbengine_retention_statistics(pulse_extended_enabled);
    }
#endif

    worker_is_busy(WORKER_JOB_REGISTRY);
    registry_statistics();

    worker_is_busy(WORKER_JOB_STRINGS);
    pulse_string_do(pulse_extended_enabled);

#ifdef DICT_WITH_STATS
    worker_is_busy(WORKER_JOB_DICTIONARIES);
    pulse_dictionary_do(pulse_extended_enabled);
#endif

    worker_is_busy(WORKER_JOB_ARAL);
    pulse_aral_do(pulse_extended_enabled);

    worker_is_busy(WORKER_JOB_PARENTS);
    pulse_parents_do(pulse_extended_enabled);

    // keep this last to have access to the memory counters
    // exposed by everyone else
    worker_is_busy(WORKER_JOB_DAEMON);
    pulse_daemon_do(pulse_extended_enabled);
}

void pulse_thread_main(void *ptr) {
    pulse_thread_loop(ptr, pulse_main_init, pulse_main_do_once, NULL);
}

// ---------------------------------------------------------------------------------------------------------------------
// pulse sqlite3 thread

static void pulse_sqlite3_do_once(void) {
    worker_is_busy(WORKER_JOB_SQLITE3);
    pulse_sqlite3_do(pulse_extended_enabled);
}

void pulse_thread_sqlite3_main(void *ptr) {
    pulse_thread_loop(ptr, NULL, pulse_sqlite3_do_once, NULL);
}

// ---------------------------------------------------------------------------------------------------------------------
// pulse workers thread

static void pulse_workers_do_once(void) {
    worker_is_busy(WORKER_JOB_WORKERS);
    pulse_workers_do(pulse_extended_enabled);
}

void pulse_thread_workers_main(void *ptr) {
    pulse_thread_loop(ptr, NULL, pulse_workers_do_once, pulse_workers_cleanup);
}

// ---------------------------------------------------------------------------------------------------------------------
// pulse memory extended thread

static void pulse_memory_extended_do_once(void) {
#ifdef NETDATA_TRACE_ALLOCATIONS
    worker_is_busy(WORKER_JOB_MALLOC_TRACE);
    pulse_trace_allocations_do(pulse_extended_enabled);
#endif

    worker_is_busy(WORKER_JOB_MEMORY_EXTENDED);
    pulse_daemon_memory_system_do(pulse_extended_enabled);
}

void pulse_thread_memory_extended_main(void *ptr) {
    pulse_thread_loop(ptr, NULL, pulse_memory_extended_do_once, NULL);
}
