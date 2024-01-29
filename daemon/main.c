// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "buildinfo.h"
#include "static_threads.h"

#include "database/engine/page_test.h"

#if defined(ENV32BIT)
#warning COMPILING 32BIT NETDATA
#endif

bool unittest_running = false;
int netdata_zero_metrics_enabled;
int netdata_anonymous_statistics_enabled;

int libuv_worker_threads = MIN_LIBUV_WORKER_THREADS;
bool ieee754_doubles = false;
time_t netdata_start_time = 0;
struct netdata_static_thread *static_threads;

struct config netdata_config = {
        .first_section = NULL,
        .last_section = NULL,
        .mutex = NETDATA_MUTEX_INITIALIZER,
        .index = {
                .avl_tree = {
                        .root = NULL,
                        .compar = appconfig_section_compare
                },
                .rwlock = AVL_LOCK_INITIALIZER
        }
};

typedef struct service_thread {
    pid_t tid;
    SERVICE_THREAD_TYPE type;
    SERVICE_TYPE services;
    char name[NETDATA_THREAD_NAME_MAX + 1];
    bool stop_immediately;
    bool cancelled;

    union {
        netdata_thread_t netdata_thread;
        uv_thread_t uv_thread;
    };

    force_quit_t force_quit_callback;
    request_quit_t request_quit_callback;
    void *data;
} SERVICE_THREAD;

struct service_globals {
    SPINLOCK lock;
    Pvoid_t pid_judy;
} service_globals = {
        .pid_judy = NULL,
};

SERVICE_THREAD *service_register(SERVICE_THREAD_TYPE thread_type, request_quit_t request_quit_callback, force_quit_t force_quit_callback, void *data, bool update __maybe_unused) {
    SERVICE_THREAD *sth = NULL;
    pid_t tid = gettid();

    spinlock_lock(&service_globals.lock);
    Pvoid_t *PValue = JudyLIns(&service_globals.pid_judy, tid, PJE0);
    if(!*PValue) {
        sth = callocz(1, sizeof(SERVICE_THREAD));
        sth->tid = tid;
        sth->type = thread_type;
        sth->request_quit_callback = request_quit_callback;
        sth->force_quit_callback = force_quit_callback;
        sth->data = data;
        os_thread_get_current_name_np(sth->name);
        *PValue = sth;

        switch(thread_type) {
            default:
            case SERVICE_THREAD_TYPE_NETDATA:
                sth->netdata_thread = netdata_thread_self();
                break;

            case SERVICE_THREAD_TYPE_EVENT_LOOP:
            case SERVICE_THREAD_TYPE_LIBUV:
                sth->uv_thread = uv_thread_self();
                break;
        }
    }
    else {
        sth = *PValue;
    }
    spinlock_unlock(&service_globals.lock);

    return sth;
}

void service_exits(void) {
    pid_t tid = gettid();

    spinlock_lock(&service_globals.lock);
    Pvoid_t *PValue = JudyLGet(service_globals.pid_judy, tid, PJE0);
    if(PValue) {
        freez(*PValue);
        JudyLDel(&service_globals.pid_judy, tid, PJE0);
    }
    spinlock_unlock(&service_globals.lock);
}

bool service_running(SERVICE_TYPE service) {
    static __thread SERVICE_THREAD *sth = NULL;

    if(unlikely(!sth))
        sth = service_register(SERVICE_THREAD_TYPE_NETDATA, NULL, NULL, NULL, false);

    sth->services |= service;

    return !(sth->stop_immediately || netdata_exit);
}

void service_signal_exit(SERVICE_TYPE service) {
    spinlock_lock(&service_globals.lock);

    Pvoid_t *PValue;
    Word_t tid = 0;
    bool first = true;
    while((PValue = JudyLFirstThenNext(service_globals.pid_judy, &tid, &first))) {
        SERVICE_THREAD *sth = *PValue;

        if((sth->services & service)) {
            sth->stop_immediately = true;

            if(sth->request_quit_callback) {
                spinlock_unlock(&service_globals.lock);
                sth->request_quit_callback(sth->data);
                spinlock_lock(&service_globals.lock);
            }
        }
    }

    spinlock_unlock(&service_globals.lock);
}

static void service_to_buffer(BUFFER *wb, SERVICE_TYPE service) {
    if(service & SERVICE_MAINTENANCE)
        buffer_strcat(wb, "MAINTENANCE ");
    if(service & SERVICE_COLLECTORS)
        buffer_strcat(wb, "COLLECTORS ");
    if(service & SERVICE_REPLICATION)
        buffer_strcat(wb, "REPLICATION ");
    if(service & ABILITY_DATA_QUERIES)
        buffer_strcat(wb, "DATA_QUERIES ");
    if(service & ABILITY_WEB_REQUESTS)
        buffer_strcat(wb, "WEB_REQUESTS ");
    if(service & SERVICE_WEB_SERVER)
        buffer_strcat(wb, "WEB_SERVER ");
    if(service & SERVICE_ACLK)
        buffer_strcat(wb, "ACLK ");
    if(service & SERVICE_HEALTH)
        buffer_strcat(wb, "HEALTH ");
    if(service & SERVICE_STREAMING)
        buffer_strcat(wb, "STREAMING ");
    if(service & ABILITY_STREAMING_CONNECTIONS)
        buffer_strcat(wb, "STREAMING_CONNECTIONS ");
    if(service & SERVICE_CONTEXT)
        buffer_strcat(wb, "CONTEXT ");
    if(service & SERVICE_ANALYTICS)
        buffer_strcat(wb, "ANALYTICS ");
    if(service & SERVICE_EXPORTERS)
        buffer_strcat(wb, "EXPORTERS ");
    if(service & SERVICE_HTTPD)
        buffer_strcat(wb, "HTTPD ");
}

static bool service_wait_exit(SERVICE_TYPE service, usec_t timeout_ut) {
    BUFFER *service_list = buffer_create(1024, NULL);
    BUFFER *thread_list = buffer_create(1024, NULL);
    usec_t started_ut = now_monotonic_usec(), ended_ut;
    size_t running;
    SERVICE_TYPE running_services = 0;

    // cancel the threads
    running = 0;
    running_services = 0;
    {
        buffer_flush(thread_list);

        spinlock_lock(&service_globals.lock);

        Pvoid_t *PValue;
        Word_t tid = 0;
        bool first = true;
        while((PValue = JudyLFirstThenNext(service_globals.pid_judy, &tid, &first))) {
            SERVICE_THREAD *sth = *PValue;
            if(sth->services & service && sth->tid != gettid() && !sth->cancelled) {
                sth->cancelled = true;

                switch(sth->type) {
                    default:
                    case SERVICE_THREAD_TYPE_NETDATA:
                        netdata_thread_cancel(sth->netdata_thread);
                        break;

                    case SERVICE_THREAD_TYPE_EVENT_LOOP:
                    case SERVICE_THREAD_TYPE_LIBUV:
                        break;
                }

                if(running)
                    buffer_strcat(thread_list, ", ");

                buffer_sprintf(thread_list, "'%s' (%d)", sth->name, sth->tid);

                running++;
                running_services |= sth->services & service;

                if(sth->force_quit_callback) {
                    spinlock_unlock(&service_globals.lock);
                    sth->force_quit_callback(sth->data);
                    spinlock_lock(&service_globals.lock);
                    continue;
                }
            }
        }

        spinlock_unlock(&service_globals.lock);
    }

    service_signal_exit(service);

    // signal them to stop
    size_t last_running = 0;
    size_t stale_time_ut = 0;
    usec_t sleep_ut = 50 * USEC_PER_MS;
    size_t log_countdown_ut = sleep_ut;
    do {
        if(running != last_running)
            stale_time_ut = 0;

        last_running = running;
        running = 0;
        running_services = 0;
        buffer_flush(thread_list);

        spinlock_lock(&service_globals.lock);

        Pvoid_t *PValue;
        Word_t tid = 0;
        bool first = true;
        while((PValue = JudyLFirstThenNext(service_globals.pid_judy, &tid, &first))) {
            SERVICE_THREAD *sth = *PValue;
            if(sth->services & service && sth->tid != gettid()) {
                if(running)
                    buffer_strcat(thread_list, ", ");

                buffer_sprintf(thread_list, "'%s' (%d)", sth->name, sth->tid);

                running_services |= sth->services & service;
                running++;
            }
        }

        spinlock_unlock(&service_globals.lock);

        if(running) {
            log_countdown_ut -= (log_countdown_ut >= sleep_ut) ? sleep_ut : log_countdown_ut;
            if(log_countdown_ut == 0 || running != last_running) {
                log_countdown_ut = 20 * sleep_ut;

                buffer_flush(service_list);
                service_to_buffer(service_list, running_services);
                netdata_log_info("SERVICE CONTROL: waiting for the following %zu services [ %s] to exit: %s",
                     running, buffer_tostring(service_list),
                     running <= 10 ? buffer_tostring(thread_list) : "");
            }

            sleep_usec(sleep_ut);
            stale_time_ut += sleep_ut;
        }

        ended_ut = now_monotonic_usec();
    } while(running && (ended_ut - started_ut < timeout_ut || stale_time_ut < timeout_ut));

    if(running) {
        buffer_flush(service_list);
        service_to_buffer(service_list, running_services);
        netdata_log_info("SERVICE CONTROL: "
             "the following %zu service(s) [ %s] take too long to exit: %s; "
             "giving up on them...",
             running, buffer_tostring(service_list),
             buffer_tostring(thread_list));
    }

    buffer_free(thread_list);
    buffer_free(service_list);

    return (running == 0);
}

#define delta_shutdown_time(msg)                        \
    do {                                                \
        usec_t now_ut = now_monotonic_usec();           \
        if(prev_msg)                                    \
            netdata_log_info("NETDATA SHUTDOWN: in %7llu ms, %s%s - next: %s", (now_ut - last_ut) / USEC_PER_MS, (timeout)?"(TIMEOUT) ":"", prev_msg, msg); \
        else                                            \
            netdata_log_info("NETDATA SHUTDOWN: next: %s", msg);    \
        last_ut = now_ut;                               \
        prev_msg = msg;                                 \
        timeout = false;                                \
    } while(0)

void web_client_cache_destroy(void);

void netdata_cleanup_and_exit(int ret, const char *action, const char *action_result, const char *action_data) {
    usec_t started_ut = now_monotonic_usec();
    usec_t last_ut = started_ut;
    const char *prev_msg = NULL;
    bool timeout = false;

    nd_log_limits_unlimited();
    netdata_log_info("NETDATA SHUTDOWN: initializing shutdown with code %d...", ret);

    // send the stat from our caller
    analytics_statistic_t statistic = { action, action_result, action_data };
    analytics_statistic_send(&statistic);

    // notify we are exiting
    statistic = (analytics_statistic_t) {"EXIT", ret?"ERROR":"OK","-"};
    analytics_statistic_send(&statistic);

    delta_shutdown_time("create shutdown file");

    char agent_crash_file[FILENAME_MAX + 1];
    char agent_incomplete_shutdown_file[FILENAME_MAX + 1];
    snprintfz(agent_crash_file, FILENAME_MAX, "%s/.agent_crash", netdata_configured_varlib_dir);
    snprintfz(agent_incomplete_shutdown_file, FILENAME_MAX, "%s/.agent_incomplete_shutdown", netdata_configured_varlib_dir);
    (void) rename(agent_crash_file, agent_incomplete_shutdown_file);

#ifdef ENABLE_DBENGINE
    if(dbengine_enabled) {
        delta_shutdown_time("dbengine exit mode");
        for (size_t tier = 0; tier < storage_tiers; tier++)
            rrdeng_exit_mode(multidb_ctx[tier]);
    }
#endif

    delta_shutdown_time("close webrtc connections");

    webrtc_close_all_connections();

    delta_shutdown_time("disable maintenance, new queries, new web requests, new streaming connections and aclk");

    service_signal_exit(
            SERVICE_MAINTENANCE
            | ABILITY_DATA_QUERIES
            | ABILITY_WEB_REQUESTS
            | ABILITY_STREAMING_CONNECTIONS
            | SERVICE_ACLK
            | SERVICE_ACLKSYNC
            );

    delta_shutdown_time("stop maintenance thread");

    timeout = !service_wait_exit(
        SERVICE_MAINTENANCE
        , 3 * USEC_PER_SEC);

    delta_shutdown_time("stop replication, exporters, health and web servers threads");

    timeout = !service_wait_exit(
            SERVICE_EXPORTERS
            | SERVICE_HEALTH
            | SERVICE_WEB_SERVER
            | SERVICE_HTTPD
            , 3 * USEC_PER_SEC);

    delta_shutdown_time("stop collectors and streaming threads");

    timeout = !service_wait_exit(
            SERVICE_COLLECTORS
            | SERVICE_STREAMING
            , 3 * USEC_PER_SEC);

    delta_shutdown_time("stop replication threads");

    timeout = !service_wait_exit(
            SERVICE_REPLICATION // replication has to be stopped after STREAMING, because it cleans up ARAL
            , 3 * USEC_PER_SEC);

    delta_shutdown_time("prepare metasync shutdown");

    metadata_sync_shutdown_prepare();

    delta_shutdown_time("disable ML detection and training threads");

    ml_stop_threads();
    ml_fini();

    delta_shutdown_time("stop context thread");

    timeout = !service_wait_exit(
            SERVICE_CONTEXT
            , 3 * USEC_PER_SEC);

    delta_shutdown_time("clear web client cache");

    web_client_cache_destroy();

    delta_shutdown_time("stop aclk threads");

    timeout = !service_wait_exit(
            SERVICE_ACLK
            , 3 * USEC_PER_SEC);

    delta_shutdown_time("stop all remaining worker threads");

    timeout = !service_wait_exit(~0, 10 * USEC_PER_SEC);

    delta_shutdown_time("cancel main threads");

    cancel_main_threads();

    if(!ret) {
        // exit cleanly

#ifdef ENABLE_DBENGINE
        if(dbengine_enabled) {
            delta_shutdown_time("flush dbengine tiers");
            for (size_t tier = 0; tier < storage_tiers; tier++)
                rrdeng_prepare_exit(multidb_ctx[tier]);

            for (size_t tier = 0; tier < storage_tiers; tier++) {
                if (!multidb_ctx[tier])
                    continue;
                completion_wait_for(&multidb_ctx[tier]->quiesce.completion);
                completion_destroy(&multidb_ctx[tier]->quiesce.completion);
            }
        }
#endif

        // free the database
        delta_shutdown_time("stop collection for all hosts");

        // rrdhost_free_all();
        rrd_finalize_collection_for_all_hosts();

        delta_shutdown_time("stop metasync threads");

        metadata_sync_shutdown();

#ifdef ENABLE_DBENGINE
        if(dbengine_enabled) {
            delta_shutdown_time("wait for dbengine collectors to finish");

            size_t running = 1;
            size_t count = 10;
            while(running && count) {
                running = 0;
                for (size_t tier = 0; tier < storage_tiers; tier++)
                    running += rrdeng_collectors_running(multidb_ctx[tier]);

                if(running) {
                    nd_log_limit_static_thread_var(erl, 1, 100 * USEC_PER_MS);
                    nd_log_limit(&erl, NDLS_DAEMON, NDLP_NOTICE,
                                 "waiting for %zu collectors to finish", running);
                    // sleep_usec(100 * USEC_PER_MS);
                    cleanup_destroyed_dictionaries();
                }
                count--;
            }

            delta_shutdown_time("wait for dbengine main cache to finish flushing");

            while (pgc_hot_and_dirty_entries(main_cache)) {
                pgc_flush_all_hot_and_dirty_pages(main_cache, PGC_SECTION_ALL);
                sleep_usec(100 * USEC_PER_MS);
            }

            delta_shutdown_time("stop dbengine tiers");
            for (size_t tier = 0; tier < storage_tiers; tier++)
                rrdeng_exit(multidb_ctx[tier]);

            rrdeng_enq_cmd(NULL, RRDENG_OPCODE_SHUTDOWN_EVLOOP, NULL, NULL, STORAGE_PRIORITY_BEST_EFFORT, NULL, NULL);
        }
#endif
    }

    delta_shutdown_time("close SQL context db");

    sql_close_context_database();

    delta_shutdown_time("closed SQL main db");

    sql_close_database();

    // unlink the pid
    if(pidfile[0]) {
        delta_shutdown_time("remove pid file");

        if(unlink(pidfile) != 0)
            netdata_log_error("EXIT: cannot unlink pidfile '%s'.", pidfile);
    }

#ifdef ENABLE_HTTPS
    delta_shutdown_time("free openssl structures");
    netdata_ssl_cleanup();
#endif

    delta_shutdown_time("remove incomplete shutdown file");

    (void) unlink(agent_incomplete_shutdown_file);

    delta_shutdown_time("exit");

    usec_t ended_ut = now_monotonic_usec();
    netdata_log_info("NETDATA SHUTDOWN: completed in %llu ms - netdata is now exiting - bye bye...", (ended_ut - started_ut) / USEC_PER_MS);
    exit(ret);
}

void web_server_threading_selection(void) {
    web_server_mode = web_server_mode_id(config_get(CONFIG_SECTION_WEB, "mode", web_server_mode_name(web_server_mode)));

    int static_threaded = (web_server_mode == WEB_SERVER_MODE_STATIC_THREADED);

    int i;
    for (i = 0; static_threads[i].name; i++) {
        if (static_threads[i].start_routine == socket_listen_main_static_threaded)
            static_threads[i].enabled = static_threaded;
    }
}

int make_dns_decision(const char *section_name, const char *config_name, const char *default_value, SIMPLE_PATTERN *p)
{
    char *value = config_get(section_name,config_name,default_value);
    if(!strcmp("yes",value))
        return 1;
    if(!strcmp("no",value))
        return 0;
    if(strcmp("heuristic",value))
        netdata_log_error("Invalid configuration option '%s' for '%s'/'%s'. Valid options are 'yes', 'no' and 'heuristic'. Proceeding with 'heuristic'",
              value, section_name, config_name);

    return simple_pattern_is_potential_name(p);
}

void web_server_config_options(void)
{
    web_client_timeout =
        (int)config_get_number(CONFIG_SECTION_WEB, "disconnect idle clients after seconds", web_client_timeout);
    web_client_first_request_timeout =
        (int)config_get_number(CONFIG_SECTION_WEB, "timeout for first request", web_client_first_request_timeout);
    web_client_streaming_rate_t =
        config_get_number(CONFIG_SECTION_WEB, "accept a streaming request every seconds", web_client_streaming_rate_t);

    respect_web_browser_do_not_track_policy =
        config_get_boolean(CONFIG_SECTION_WEB, "respect do not track policy", respect_web_browser_do_not_track_policy);
    web_x_frame_options = config_get(CONFIG_SECTION_WEB, "x-frame-options response header", "");
    if(!*web_x_frame_options)
        web_x_frame_options = NULL;

    web_allow_connections_from =
            simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow connections from", "localhost *"),
                                  NULL, SIMPLE_PATTERN_EXACT, true);
    web_allow_connections_dns  =
        make_dns_decision(CONFIG_SECTION_WEB, "allow connections by dns", "heuristic", web_allow_connections_from);
    web_allow_dashboard_from   =
            simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow dashboard from", "localhost *"),
                                  NULL, SIMPLE_PATTERN_EXACT, true);
    web_allow_dashboard_dns    =
        make_dns_decision(CONFIG_SECTION_WEB, "allow dashboard by dns", "heuristic", web_allow_dashboard_from);
    web_allow_badges_from      =
            simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow badges from", "*"), NULL, SIMPLE_PATTERN_EXACT,
                                  true);
    web_allow_badges_dns       =
        make_dns_decision(CONFIG_SECTION_WEB, "allow badges by dns", "heuristic", web_allow_badges_from);
    web_allow_registry_from    =
            simple_pattern_create(config_get(CONFIG_SECTION_REGISTRY, "allow from", "*"), NULL, SIMPLE_PATTERN_EXACT,
                                  true);
    web_allow_registry_dns     = make_dns_decision(CONFIG_SECTION_REGISTRY, "allow by dns", "heuristic",
                                                   web_allow_registry_from);
    web_allow_streaming_from   = simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow streaming from", "*"),
                                                       NULL, SIMPLE_PATTERN_EXACT, true);
    web_allow_streaming_dns    = make_dns_decision(CONFIG_SECTION_WEB, "allow streaming by dns", "heuristic",
                                                   web_allow_streaming_from);
    // Note the default is not heuristic, the wildcards could match DNS but the intent is ip-addresses.
    web_allow_netdataconf_from = simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow netdata.conf from",
                                                                  "localhost fd* 10.* 192.168.* 172.16.* 172.17.* 172.18.*"
                                                                  " 172.19.* 172.20.* 172.21.* 172.22.* 172.23.* 172.24.*"
                                                                  " 172.25.* 172.26.* 172.27.* 172.28.* 172.29.* 172.30.*"
                                                                  " 172.31.* UNKNOWN"), NULL, SIMPLE_PATTERN_EXACT,
                                                       true);
    web_allow_netdataconf_dns  =
        make_dns_decision(CONFIG_SECTION_WEB, "allow netdata.conf by dns", "no", web_allow_netdataconf_from);
    web_allow_mgmt_from        =
            simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow management from", "localhost"),
                                  NULL, SIMPLE_PATTERN_EXACT, true);
    web_allow_mgmt_dns         =
        make_dns_decision(CONFIG_SECTION_WEB, "allow management by dns","heuristic",web_allow_mgmt_from);

    web_enable_gzip = config_get_boolean(CONFIG_SECTION_WEB, "enable gzip compression", web_enable_gzip);

    char *s = config_get(CONFIG_SECTION_WEB, "gzip compression strategy", "default");
    if(!strcmp(s, "default"))
        web_gzip_strategy = Z_DEFAULT_STRATEGY;
    else if(!strcmp(s, "filtered"))
        web_gzip_strategy = Z_FILTERED;
    else if(!strcmp(s, "huffman only"))
        web_gzip_strategy = Z_HUFFMAN_ONLY;
    else if(!strcmp(s, "rle"))
        web_gzip_strategy = Z_RLE;
    else if(!strcmp(s, "fixed"))
        web_gzip_strategy = Z_FIXED;
    else {
        netdata_log_error("Invalid compression strategy '%s'. Valid strategies are 'default', 'filtered', 'huffman only', 'rle' and 'fixed'. Proceeding with 'default'.", s);
        web_gzip_strategy = Z_DEFAULT_STRATEGY;
    }

    web_gzip_level = (int)config_get_number(CONFIG_SECTION_WEB, "gzip compression level", 3);
    if(web_gzip_level < 1) {
        netdata_log_error("Invalid compression level %d. Valid levels are 1 (fastest) to 9 (best ratio). Proceeding with level 1 (fastest compression).", web_gzip_level);
        web_gzip_level = 1;
    }
    else if(web_gzip_level > 9) {
        netdata_log_error("Invalid compression level %d. Valid levels are 1 (fastest) to 9 (best ratio). Proceeding with level 9 (best compression).", web_gzip_level);
        web_gzip_level = 9;
    }
}


// killpid kills pid with SIGTERM.
int killpid(pid_t pid) {
    int ret;
    netdata_log_debug(D_EXIT, "Request to kill pid %d", pid);

    int signal = SIGTERM;
//#ifdef NETDATA_INTERNAL_CHECKS
//    if(service_running(SERVICE_COLLECTORS))
//        signal = SIGABRT;
//#endif

    errno = 0;
    ret = kill(pid, signal);
    if (ret == -1) {
        switch(errno) {
            case ESRCH:
                // We wanted the process to exit so just let the caller handle.
                return ret;

            case EPERM:
                netdata_log_error("Cannot kill pid %d, but I do not have enough permissions.", pid);
                break;

            default:
                netdata_log_error("Cannot kill pid %d, but I received an error.", pid);
                break;
        }
    }

    return ret;
}

static void set_nofile_limit(struct rlimit *rl) {
    // get the num files allowed
    if(getrlimit(RLIMIT_NOFILE, rl) != 0) {
        netdata_log_error("getrlimit(RLIMIT_NOFILE) failed");
        return;
    }

    netdata_log_info("resources control: allowed file descriptors: soft = %zu, max = %zu",
         (size_t) rl->rlim_cur, (size_t) rl->rlim_max);

    // make the soft/hard limits equal
    rl->rlim_cur = rl->rlim_max;
    if (setrlimit(RLIMIT_NOFILE, rl) != 0) {
        netdata_log_error("setrlimit(RLIMIT_NOFILE, { %zu, %zu }) failed", (size_t)rl->rlim_cur, (size_t)rl->rlim_max);
    }

    // sanity check to make sure we have enough file descriptors available to open
    if (getrlimit(RLIMIT_NOFILE, rl) != 0) {
        netdata_log_error("getrlimit(RLIMIT_NOFILE) failed");
        return;
    }

    if (rl->rlim_cur < 1024)
        netdata_log_error("Number of open file descriptors allowed for this process is too low (RLIMIT_NOFILE=%zu)", (size_t)rl->rlim_cur);
}

void cancel_main_threads() {
    nd_log_limits_unlimited();

    int i, found = 0;
    usec_t max = 5 * USEC_PER_SEC, step = 100000;
    for (i = 0; static_threads[i].name != NULL ; i++) {
        if (static_threads[i].enabled == NETDATA_MAIN_THREAD_RUNNING) {
            if (static_threads[i].thread) {
                netdata_log_info("EXIT: Stopping main thread: %s", static_threads[i].name);
                netdata_thread_cancel(*static_threads[i].thread);
            } else {
                netdata_log_info("EXIT: No thread running (marking as EXITED): %s", static_threads[i].name);
                static_threads[i].enabled = NETDATA_MAIN_THREAD_EXITED;
            }
            found++;
        }
    }

    netdata_exit = 1;

    while(found && max > 0) {
        max -= step;
        netdata_log_info("Waiting %d threads to finish...", found);
        sleep_usec(step);
        found = 0;
        for (i = 0; static_threads[i].name != NULL ; i++) {
            if (static_threads[i].enabled != NETDATA_MAIN_THREAD_EXITED)
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

    for (i = 0; static_threads[i].name != NULL ; i++)
        freez(static_threads[i].thread);

    freez(static_threads);
}

struct option_def option_definitions[] = {
    // opt description                                    arg name       default value
    { 'c', "Configuration file to load.",                 "filename",    CONFIG_DIR "/" CONFIG_FILENAME},
    { 'D', "Do not fork. Run in the foreground.",         NULL,          "run in the background"},
    { 'd', "Fork. Run in the background.",                NULL,          "run in the background"},
    { 'h', "Display this help message.",                  NULL,          NULL},
    { 'P', "File to save a pid while running.",           "filename",    "do not save pid to a file"},
    { 'i', "The IP address to listen to.",                "IP",          "all IP addresses IPv4 and IPv6"},
    { 'p', "API/Web port to use.",                        "port",        "19999"},
    { 's', "Prefix for /proc and /sys (for containers).", "path",        "no prefix"},
    { 't', "The internal clock of netdata.",              "seconds",     "1"},
    { 'u', "Run as user.",                                "username",    "netdata"},
    { 'v', "Print netdata version and exit.",             NULL,          NULL},
    { 'V', "Print netdata version and exit.",             NULL,          NULL},
    { 'W', "See Advanced options below.",                 "options",     NULL},
};

int help(int exitcode) {
    FILE *stream;
    if(exitcode == 0)
        stream = stdout;
    else
        stream = stderr;

    int num_opts = sizeof(option_definitions) / sizeof(struct option_def);
    int i;
    int max_len_arg = 0;

    // Compute maximum argument length
    for( i = 0; i < num_opts; i++ ) {
        if(option_definitions[i].arg_name) {
            int len_arg = (int)strlen(option_definitions[i].arg_name);
            if(len_arg > max_len_arg) max_len_arg = len_arg;
        }
    }

    if(max_len_arg > 30) max_len_arg = 30;
    if(max_len_arg < 20) max_len_arg = 20;

    fprintf(stream, "%s", "\n"
            " ^\n"
            " |.-.   .-.   .-.   .-.   .  netdata                                         \n"
            " |   '-'   '-'   '-'   '-'   real-time performance monitoring, done right!   \n"
            " +----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+--->\n"
            "\n"
            " Copyright (C) 2016-2023, Netdata, Inc. <info@netdata.cloud>\n"
            " Released under GNU General Public License v3 or later.\n"
            " All rights reserved.\n"
            "\n"
            " Home Page  : https://netdata.cloud\n"
            " Source Code: https://github.com/netdata/netdata\n"
            " Docs       : https://learn.netdata.cloud\n"
            " Support    : https://github.com/netdata/netdata/issues\n"
            " License    : https://github.com/netdata/netdata/blob/master/LICENSE.md\n"
            "\n"
            " Twitter    : https://twitter.com/netdatahq\n"
            " LinkedIn   : https://linkedin.com/company/netdata-cloud/\n"
            " Facebook   : https://facebook.com/linuxnetdata/\n"
            "\n"
            "\n"
    );

    fprintf(stream, " SYNOPSIS: netdata [options]\n");
    fprintf(stream, "\n");
    fprintf(stream, " Options:\n\n");

    // Output options description.
    for( i = 0; i < num_opts; i++ ) {
        fprintf(stream, "  -%c %-*s  %s", option_definitions[i].val, max_len_arg, option_definitions[i].arg_name ? option_definitions[i].arg_name : "", option_definitions[i].description);
        if(option_definitions[i].default_value) {
            fprintf(stream, "\n   %c %-*s  Default: %s\n", ' ', max_len_arg, "", option_definitions[i].default_value);
        } else {
            fprintf(stream, "\n");
        }
        fprintf(stream, "\n");
    }

    fprintf(stream, "\n Advanced options:\n\n"
            "  -W stacksize=N           Set the stacksize (in bytes).\n\n"
            "  -W debug_flags=N         Set runtime tracing to debug.log.\n\n"
            "  -W unittest              Run internal unittests and exit.\n\n"
            "  -W sqlite-meta-recover   Run recovery on the metadata database and exit.\n\n"
            "  -W sqlite-compact        Reclaim metadata database unused space and exit.\n\n"
            "  -W sqlite-analyze        Run update statistics and exit.\n\n"
#ifdef ENABLE_DBENGINE
            "  -W createdataset=N       Create a DB engine dataset of N seconds and exit.\n\n"
            "  -W stresstest=A,B,C,D,E,F,G\n"
            "                           Run a DB engine stress test for A seconds,\n"
            "                           with B writers and C readers, with a ramp up\n"
            "                           time of D seconds for writers, a page cache\n"
            "                           size of E MiB, an optional disk space limit\n"
            "                           of F MiB, G libuv workers (default 16) and exit.\n\n"
#endif
            "  -W set section option value\n"
            "                           set netdata.conf option from the command line.\n\n"
            "  -W buildinfo             Print the version, the configure options,\n"
            "                           a list of optional features, and whether they\n"
            "                           are enabled or not.\n\n"
            "  -W buildinfojson         Print the version, the configure options,\n"
            "                           a list of optional features, and whether they\n"
            "                           are enabled or not, in JSON format.\n\n"
            "  -W simple-pattern pattern string\n"
            "                           Check if string matches pattern and exit.\n\n"
            "  -W \"claim -token=TOKEN -rooms=ROOM1,ROOM2\"\n"
            "                           Claim the agent to the workspace rooms pointed to by TOKEN and ROOM*.\n\n"
    );

    fprintf(stream, "\n Signals netdata handles:\n\n"
            "  - HUP                    Close and reopen log files.\n"
            "  - USR1                   Save internal DB to disk.\n"
            "  - USR2                   Reload health configuration.\n"
            "\n"
    );

    fflush(stream);
    return exitcode;
}

#ifdef ENABLE_HTTPS
static void security_init(){
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/ssl/key.pem",netdata_configured_user_config_dir);
    netdata_ssl_security_key = config_get(CONFIG_SECTION_WEB, "ssl key",  filename);

    snprintfz(filename, FILENAME_MAX, "%s/ssl/cert.pem",netdata_configured_user_config_dir);
    netdata_ssl_security_cert = config_get(CONFIG_SECTION_WEB, "ssl certificate",  filename);

    tls_version    = config_get(CONFIG_SECTION_WEB, "tls version",  "1.3");
    tls_ciphers    = config_get(CONFIG_SECTION_WEB, "tls ciphers",  "none");

    netdata_ssl_initialize_openssl();
}
#endif

static void log_init(void) {
    nd_log_set_facility(config_get(CONFIG_SECTION_LOGS, "facility", "daemon"));

    time_t period = ND_LOG_DEFAULT_THROTTLE_PERIOD;
    size_t logs = ND_LOG_DEFAULT_THROTTLE_LOGS;
    period = config_get_number(CONFIG_SECTION_LOGS, "logs flood protection period", period);
    logs = (unsigned long)config_get_number(CONFIG_SECTION_LOGS, "logs to trigger flood protection", (long long int)logs);
    nd_log_set_flood_protection(logs, period);

    nd_log_set_priority_level(config_get(CONFIG_SECTION_LOGS, "level", NDLP_INFO_STR));

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/debug.log", netdata_configured_log_dir);
    nd_log_set_user_settings(NDLS_DEBUG, config_get(CONFIG_SECTION_LOGS, "debug", filename));

    bool with_journal = is_stderr_connected_to_journal() /* || nd_log_journal_socket_available() */;
    if(with_journal)
        snprintfz(filename, FILENAME_MAX, "journal");
    else
        snprintfz(filename, FILENAME_MAX, "%s/daemon.log", netdata_configured_log_dir);
    nd_log_set_user_settings(NDLS_DAEMON, config_get(CONFIG_SECTION_LOGS, "daemon", filename));

    if(with_journal)
        snprintfz(filename, FILENAME_MAX, "journal");
    else
        snprintfz(filename, FILENAME_MAX, "%s/collector.log", netdata_configured_log_dir);
    nd_log_set_user_settings(NDLS_COLLECTORS, config_get(CONFIG_SECTION_LOGS, "collector", filename));

    snprintfz(filename, FILENAME_MAX, "%s/access.log", netdata_configured_log_dir);
    nd_log_set_user_settings(NDLS_ACCESS, config_get(CONFIG_SECTION_LOGS, "access", filename));

    if(with_journal)
        snprintfz(filename, FILENAME_MAX, "journal");
    else
        snprintfz(filename, FILENAME_MAX, "%s/health.log", netdata_configured_log_dir);
    nd_log_set_user_settings(NDLS_HEALTH, config_get(CONFIG_SECTION_LOGS, "health", filename));

#ifdef ENABLE_ACLK
    aclklog_enabled = config_get_boolean(CONFIG_SECTION_CLOUD, "conversation log", CONFIG_BOOLEAN_NO);
    if (aclklog_enabled) {
        snprintfz(filename, FILENAME_MAX, "%s/aclk.log", netdata_configured_log_dir);
        nd_log_set_user_settings(NDLS_ACLK, config_get(CONFIG_SECTION_CLOUD, "conversation log file", filename));
    }
#endif
}

char *initialize_lock_directory_path(char *prefix)
{
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/lock", prefix);

    return config_get(CONFIG_SECTION_DIRECTORIES, "lock", filename);
}

static void backwards_compatible_config() {
    // move [global] options to the [web] section
    config_move(CONFIG_SECTION_GLOBAL, "http port listen backlog",
                CONFIG_SECTION_WEB,    "listen backlog");

    config_move(CONFIG_SECTION_GLOBAL, "bind socket to IP",
                CONFIG_SECTION_WEB,    "bind to");

    config_move(CONFIG_SECTION_GLOBAL, "bind to",
                CONFIG_SECTION_WEB,    "bind to");

    config_move(CONFIG_SECTION_GLOBAL, "port",
                CONFIG_SECTION_WEB,    "default port");

    config_move(CONFIG_SECTION_GLOBAL, "default port",
                CONFIG_SECTION_WEB,    "default port");

    config_move(CONFIG_SECTION_GLOBAL, "disconnect idle web clients after seconds",
                CONFIG_SECTION_WEB,    "disconnect idle clients after seconds");

    config_move(CONFIG_SECTION_GLOBAL, "respect web browser do not track policy",
                CONFIG_SECTION_WEB,    "respect do not track policy");

    config_move(CONFIG_SECTION_GLOBAL, "web x-frame-options header",
                CONFIG_SECTION_WEB,    "x-frame-options response header");

    config_move(CONFIG_SECTION_GLOBAL, "enable web responses gzip compression",
                CONFIG_SECTION_WEB,    "enable gzip compression");

    config_move(CONFIG_SECTION_GLOBAL, "web compression strategy",
                CONFIG_SECTION_WEB,    "gzip compression strategy");

    config_move(CONFIG_SECTION_GLOBAL, "web compression level",
                CONFIG_SECTION_WEB,    "gzip compression level");

    config_move(CONFIG_SECTION_GLOBAL,      "config directory",
                CONFIG_SECTION_DIRECTORIES, "config");

    config_move(CONFIG_SECTION_GLOBAL,      "stock config directory",
                CONFIG_SECTION_DIRECTORIES, "stock config");

    config_move(CONFIG_SECTION_GLOBAL,      "log directory",
                CONFIG_SECTION_DIRECTORIES, "log");

    config_move(CONFIG_SECTION_GLOBAL,      "web files directory",
                CONFIG_SECTION_DIRECTORIES, "web");

    config_move(CONFIG_SECTION_GLOBAL,      "cache directory",
                CONFIG_SECTION_DIRECTORIES, "cache");

    config_move(CONFIG_SECTION_GLOBAL,      "lib directory",
                CONFIG_SECTION_DIRECTORIES, "lib");

    config_move(CONFIG_SECTION_GLOBAL,      "home directory",
                CONFIG_SECTION_DIRECTORIES, "home");

    config_move(CONFIG_SECTION_GLOBAL,      "lock directory",
                CONFIG_SECTION_DIRECTORIES, "lock");

    config_move(CONFIG_SECTION_GLOBAL,      "plugins directory",
                CONFIG_SECTION_DIRECTORIES, "plugins");

    config_move(CONFIG_SECTION_HEALTH,      "health configuration directory",
                CONFIG_SECTION_DIRECTORIES, "health config");

    config_move(CONFIG_SECTION_HEALTH,      "stock health configuration directory",
                CONFIG_SECTION_DIRECTORIES, "stock health config");

    config_move(CONFIG_SECTION_REGISTRY,    "registry db directory",
                CONFIG_SECTION_DIRECTORIES, "registry");

    config_move(CONFIG_SECTION_GLOBAL, "debug log",
                CONFIG_SECTION_LOGS,   "debug");

    config_move(CONFIG_SECTION_GLOBAL, "error log",
                CONFIG_SECTION_LOGS,   "error");

    config_move(CONFIG_SECTION_GLOBAL, "access log",
                CONFIG_SECTION_LOGS,   "access");

    config_move(CONFIG_SECTION_GLOBAL, "facility log",
                CONFIG_SECTION_LOGS,   "facility");

    config_move(CONFIG_SECTION_GLOBAL, "errors flood protection period",
                CONFIG_SECTION_LOGS,   "errors flood protection period");

    config_move(CONFIG_SECTION_GLOBAL, "errors to trigger flood protection",
                CONFIG_SECTION_LOGS,   "errors to trigger flood protection");

    config_move(CONFIG_SECTION_GLOBAL, "debug flags",
                CONFIG_SECTION_LOGS,   "debug flags");

    config_move(CONFIG_SECTION_GLOBAL,   "TZ environment variable",
                CONFIG_SECTION_ENV_VARS, "TZ");

    config_move(CONFIG_SECTION_PLUGINS,  "PATH environment variable",
                CONFIG_SECTION_ENV_VARS, "PATH");

    config_move(CONFIG_SECTION_PLUGINS,  "PYTHONPATH environment variable",
                CONFIG_SECTION_ENV_VARS, "PYTHONPATH");

    config_move(CONFIG_SECTION_STATSD,  "enabled",
                CONFIG_SECTION_PLUGINS, "statsd");

    config_move(CONFIG_SECTION_GLOBAL,  "memory mode",
                CONFIG_SECTION_DB,      "mode");

    config_move(CONFIG_SECTION_GLOBAL,  "history",
                CONFIG_SECTION_DB,      "retention");

    config_move(CONFIG_SECTION_GLOBAL,  "update every",
                CONFIG_SECTION_DB,      "update every");

    config_move(CONFIG_SECTION_GLOBAL,  "page cache size",
                CONFIG_SECTION_DB,      "dbengine page cache size MB");

    config_move(CONFIG_SECTION_DB,      "page cache size",
                CONFIG_SECTION_DB,      "dbengine page cache size MB");

    config_move(CONFIG_SECTION_GLOBAL,  "page cache uses malloc",
                CONFIG_SECTION_DB,      "dbengine page cache with malloc");

    config_move(CONFIG_SECTION_DB,      "page cache with malloc",
                CONFIG_SECTION_DB,      "dbengine page cache with malloc");

    config_move(CONFIG_SECTION_GLOBAL,  "dbengine disk space",
                CONFIG_SECTION_DB,      "dbengine disk space MB");

    config_move(CONFIG_SECTION_GLOBAL,  "dbengine multihost disk space",
                CONFIG_SECTION_DB,      "dbengine multihost disk space MB");

    config_move(CONFIG_SECTION_GLOBAL,  "memory deduplication (ksm)",
                CONFIG_SECTION_DB,      "memory deduplication (ksm)");

    config_move(CONFIG_SECTION_GLOBAL,  "dbengine page fetch timeout",
                CONFIG_SECTION_DB,      "dbengine page fetch timeout secs");

    config_move(CONFIG_SECTION_GLOBAL,  "dbengine page fetch retries",
                CONFIG_SECTION_DB,      "dbengine page fetch retries");

    config_move(CONFIG_SECTION_GLOBAL,  "dbengine extent pages",
                CONFIG_SECTION_DB,      "dbengine pages per extent");

    config_move(CONFIG_SECTION_GLOBAL,  "cleanup obsolete charts after seconds",
                CONFIG_SECTION_DB,      "cleanup obsolete charts after secs");

    config_move(CONFIG_SECTION_GLOBAL,  "gap when lost iterations above",
                CONFIG_SECTION_DB,      "gap when lost iterations above");

    config_move(CONFIG_SECTION_GLOBAL,  "cleanup orphan hosts after seconds",
                CONFIG_SECTION_DB,      "cleanup orphan hosts after secs");

    config_move(CONFIG_SECTION_GLOBAL,  "delete obsolete charts files",
                CONFIG_SECTION_DB,      "delete obsolete charts files");

    config_move(CONFIG_SECTION_GLOBAL,  "delete orphan hosts files",
                CONFIG_SECTION_DB,      "delete orphan hosts files");

    config_move(CONFIG_SECTION_GLOBAL,  "enable zero metrics",
                CONFIG_SECTION_DB,      "enable zero metrics");

    config_move(CONFIG_SECTION_LOGS,   "error",
                CONFIG_SECTION_LOGS,   "daemon");

    config_move(CONFIG_SECTION_LOGS,   "severity level",
                CONFIG_SECTION_LOGS,   "level");

    config_move(CONFIG_SECTION_LOGS,   "errors to trigger flood protection",
                CONFIG_SECTION_LOGS,   "logs to trigger flood protection");

    config_move(CONFIG_SECTION_LOGS,   "errors flood protection period",
                CONFIG_SECTION_LOGS,   "logs flood protection period");
    config_move(CONFIG_SECTION_HEALTH, "is ephemeral",
                CONFIG_SECTION_GLOBAL, "is ephemeral node");

    config_move(CONFIG_SECTION_HEALTH, "has unstable connection",
                CONFIG_SECTION_GLOBAL, "has unstable connection");
}

static int get_hostname(char *buf, size_t buf_size) {
    if (netdata_configured_host_prefix && *netdata_configured_host_prefix) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/etc/hostname", netdata_configured_host_prefix);

        if (!read_txt_file(filename, buf, buf_size)) {
            trim(buf);
            return 0;
        }
    }

    return gethostname(buf, buf_size);
}

static void get_netdata_configured_variables() {
    backwards_compatible_config();

    // ------------------------------------------------------------------------
    // get the hostname

    netdata_configured_host_prefix = config_get(CONFIG_SECTION_GLOBAL, "host access prefix", "");
    verify_netdata_host_prefix(true);

    char buf[HOSTNAME_MAX + 1];
    if (get_hostname(buf, HOSTNAME_MAX))
        netdata_log_error("Cannot get machine hostname.");

    netdata_configured_hostname = config_get(CONFIG_SECTION_GLOBAL, "hostname", buf);
    netdata_log_debug(D_OPTIONS, "hostname set to '%s'", netdata_configured_hostname);

    // ------------------------------------------------------------------------
    // get default database update frequency

    default_rrd_update_every = (int) config_get_number(CONFIG_SECTION_DB, "update every", UPDATE_EVERY);
    if(default_rrd_update_every < 1 || default_rrd_update_every > 600) {
        netdata_log_error("Invalid data collection frequency (update every) %d given. Defaulting to %d.", default_rrd_update_every, UPDATE_EVERY);
        default_rrd_update_every = UPDATE_EVERY;
        config_set_number(CONFIG_SECTION_DB, "update every", default_rrd_update_every);
    }

    // ------------------------------------------------------------------------
    // get default memory mode for the database

    {
        const char *mode = config_get(CONFIG_SECTION_DB, "mode", rrd_memory_mode_name(default_rrd_memory_mode));
        default_rrd_memory_mode = rrd_memory_mode_id(mode);
        if(strcmp(mode, rrd_memory_mode_name(default_rrd_memory_mode)) != 0) {
            netdata_log_error("Invalid memory mode '%s' given. Using '%s'", mode, rrd_memory_mode_name(default_rrd_memory_mode));
            config_set(CONFIG_SECTION_DB, "mode", rrd_memory_mode_name(default_rrd_memory_mode));
        }
    }

    // ------------------------------------------------------------------------
    // get default database size

    if(default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE && default_rrd_memory_mode != RRD_MEMORY_MODE_NONE) {
        default_rrd_history_entries = (int)config_get_number(
            CONFIG_SECTION_DB, "retention",
            align_entries_to_pagesize(default_rrd_memory_mode, RRD_DEFAULT_HISTORY_ENTRIES));

        long h = align_entries_to_pagesize(default_rrd_memory_mode, default_rrd_history_entries);
        if (h != default_rrd_history_entries) {
            config_set_number(CONFIG_SECTION_DB, "retention", h);
            default_rrd_history_entries = (int)h;
        }
    }

    // ------------------------------------------------------------------------
    // get system paths

    netdata_configured_user_config_dir  = config_get(CONFIG_SECTION_DIRECTORIES, "config",       netdata_configured_user_config_dir);
    netdata_configured_stock_config_dir = config_get(CONFIG_SECTION_DIRECTORIES, "stock config", netdata_configured_stock_config_dir);
    netdata_configured_log_dir          = config_get(CONFIG_SECTION_DIRECTORIES, "log",          netdata_configured_log_dir);
    netdata_configured_web_dir          = config_get(CONFIG_SECTION_DIRECTORIES, "web",          netdata_configured_web_dir);
    netdata_configured_cache_dir        = config_get(CONFIG_SECTION_DIRECTORIES, "cache",        netdata_configured_cache_dir);
    netdata_configured_varlib_dir       = config_get(CONFIG_SECTION_DIRECTORIES, "lib",          netdata_configured_varlib_dir);

    netdata_configured_lock_dir = initialize_lock_directory_path(netdata_configured_varlib_dir);

    {
        pluginsd_initialize_plugin_directories();
        netdata_configured_primary_plugins_dir = plugin_directories[PLUGINSD_STOCK_PLUGINS_DIRECTORY_PATH];
    }

#ifdef ENABLE_DBENGINE
    // ------------------------------------------------------------------------
    // get default Database Engine page type

    const char *page_type = config_get(CONFIG_SECTION_DB, "dbengine page type", "raw");
    if (strcmp(page_type, "gorilla") == 0) {
        tier_page_type[0] = PAGE_GORILLA_METRICS;
    } else if (strcmp(page_type, "raw") != 0) {
        netdata_log_error("Invalid dbengine page type ''%s' given. Defaulting to 'raw'.", page_type);
    }

    // ------------------------------------------------------------------------
    // get default Database Engine page cache size in MiB

    default_rrdeng_page_cache_mb = (int) config_get_number(CONFIG_SECTION_DB, "dbengine page cache size MB", default_rrdeng_page_cache_mb);
    default_rrdeng_extent_cache_mb = (int) config_get_number(CONFIG_SECTION_DB, "dbengine extent cache size MB", default_rrdeng_extent_cache_mb);
    db_engine_journal_check = config_get_boolean(CONFIG_SECTION_DB, "dbengine enable journal integrity check", CONFIG_BOOLEAN_NO);

    if(default_rrdeng_extent_cache_mb < 0)
        default_rrdeng_extent_cache_mb = 0;

    if(default_rrdeng_page_cache_mb < RRDENG_MIN_PAGE_CACHE_SIZE_MB) {
        netdata_log_error("Invalid page cache size %d given. Defaulting to %d.", default_rrdeng_page_cache_mb, RRDENG_MIN_PAGE_CACHE_SIZE_MB);
        default_rrdeng_page_cache_mb = RRDENG_MIN_PAGE_CACHE_SIZE_MB;
        config_set_number(CONFIG_SECTION_DB, "dbengine page cache size MB", default_rrdeng_page_cache_mb);
    }

    // ------------------------------------------------------------------------
    // get default Database Engine disk space quota in MiB

    default_rrdeng_disk_quota_mb = (int) config_get_number(CONFIG_SECTION_DB, "dbengine disk space MB", default_rrdeng_disk_quota_mb);
    if(default_rrdeng_disk_quota_mb < RRDENG_MIN_DISK_SPACE_MB) {
        netdata_log_error("Invalid dbengine disk space %d given. Defaulting to %d.", default_rrdeng_disk_quota_mb, RRDENG_MIN_DISK_SPACE_MB);
        default_rrdeng_disk_quota_mb = RRDENG_MIN_DISK_SPACE_MB;
        config_set_number(CONFIG_SECTION_DB, "dbengine disk space MB", default_rrdeng_disk_quota_mb);
    }

    default_multidb_disk_quota_mb = (int) config_get_number(CONFIG_SECTION_DB, "dbengine multihost disk space MB", compute_multidb_diskspace());
    if(default_multidb_disk_quota_mb < RRDENG_MIN_DISK_SPACE_MB) {
        netdata_log_error("Invalid multidb disk space %d given. Defaulting to %d.", default_multidb_disk_quota_mb, default_rrdeng_disk_quota_mb);
        default_multidb_disk_quota_mb = default_rrdeng_disk_quota_mb;
        config_set_number(CONFIG_SECTION_DB, "dbengine multihost disk space MB", default_multidb_disk_quota_mb);
    }
#else
    if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
       error_report("RRD_MEMORY_MODE_DBENGINE is not supported in this platform. The agent will use db mode 'save' instead.");
       default_rrd_memory_mode = RRD_MEMORY_MODE_RAM;
    }
#endif

    // --------------------------------------------------------------------
    // get KSM settings

#ifdef MADV_MERGEABLE
    enable_ksm = config_get_boolean(CONFIG_SECTION_DB, "memory deduplication (ksm)", enable_ksm);
#endif

    // --------------------------------------------------------------------
    // metric correlations

    enable_metric_correlations = config_get_boolean(CONFIG_SECTION_GLOBAL, "enable metric correlations", enable_metric_correlations);
    default_metric_correlations_method = weights_string_to_method(config_get(
        CONFIG_SECTION_GLOBAL, "metric correlations method",
        weights_method_to_string(default_metric_correlations_method)));

    // --------------------------------------------------------------------

    rrdset_free_obsolete_time_s = config_get_number(CONFIG_SECTION_DB, "cleanup obsolete charts after secs", rrdset_free_obsolete_time_s);
    rrdhost_free_ephemeral_time_s = config_get_number(CONFIG_SECTION_DB, "cleanup ephemeral hosts after secs", rrdhost_free_ephemeral_time_s);
    // Current chart locking and invalidation scheme doesn't prevent Netdata from segmentation faults if a short
    // cleanup delay is set. Extensive stress tests showed that 10 seconds is quite a safe delay. Look at
    // https://github.com/netdata/netdata/pull/11222#issuecomment-868367920 for more information.
    if (rrdset_free_obsolete_time_s < 10) {
        rrdset_free_obsolete_time_s = 10;
        netdata_log_info("The \"cleanup obsolete charts after seconds\" option was set to 10 seconds.");
        config_set_number(CONFIG_SECTION_DB, "cleanup obsolete charts after secs", rrdset_free_obsolete_time_s);
    }

    gap_when_lost_iterations_above = (int)config_get_number(CONFIG_SECTION_DB, "gap when lost iterations above", gap_when_lost_iterations_above);
    if (gap_when_lost_iterations_above < 1) {
        gap_when_lost_iterations_above = 1;
        config_set_number(CONFIG_SECTION_DB, "gap when lost iterations above", gap_when_lost_iterations_above);
    }
    gap_when_lost_iterations_above += 2;

    // --------------------------------------------------------------------
    // get various system parameters

    get_system_HZ();
    get_system_cpus_uncached();
    get_system_pid_max();


}

static void post_conf_load(char **user)
{
    // --------------------------------------------------------------------
    // get the user we should run

    // IMPORTANT: this is required before web_files_uid()
    if(getuid() == 0) {
        *user = config_get(CONFIG_SECTION_GLOBAL, "run as user", NETDATA_USER);
    }
    else {
        struct passwd *passwd = getpwuid(getuid());
        *user = config_get(CONFIG_SECTION_GLOBAL, "run as user", (passwd && passwd->pw_name)?passwd->pw_name:"");
    }
}

static bool load_netdata_conf(char *filename, char overwrite_used, char **user) {
    errno = 0;

    int ret = 0;

    if(filename && *filename) {
        ret = config_load(filename, overwrite_used, NULL);
        if(!ret)
            netdata_log_error("CONFIG: cannot load config file '%s'.", filename);
    }
    else {
        filename = strdupz_path_subpath(netdata_configured_user_config_dir, "netdata.conf");

        ret = config_load(filename, overwrite_used, NULL);
        if(!ret) {
            netdata_log_info("CONFIG: cannot load user config '%s'. Will try the stock version.", filename);
            freez(filename);

            filename = strdupz_path_subpath(netdata_configured_stock_config_dir, "netdata.conf");
            ret = config_load(filename, overwrite_used, NULL);
            if(!ret)
                netdata_log_info("CONFIG: cannot load stock config '%s'. Running with internal defaults.", filename);
        }

        freez(filename);
    }

    post_conf_load(user);
    return ret;
}

// coverity[ +tainted_string_sanitize_content : arg-0 ]
static inline void coverity_remove_taint(char *s)
{
    (void)s;
}

int get_system_info(struct rrdhost_system_info *system_info) {
    char *script;
    script = mallocz(sizeof(char) * (strlen(netdata_configured_primary_plugins_dir) + strlen("system-info.sh") + 2));
    sprintf(script, "%s/%s", netdata_configured_primary_plugins_dir, "system-info.sh");
    if (unlikely(access(script, R_OK) != 0)) {
        netdata_log_error("System info script %s not found.",script);
        freez(script);
        return 1;
    }

    pid_t command_pid;

    FILE *fp_child_input;
    FILE *fp_child_output = netdata_popen(script, &command_pid, &fp_child_input);
    if(fp_child_output) {
        char line[200 + 1];
        // Removed the double strlens, if the Coverity tainted string warning reappears I'll revert.
        // One time init code, but I'm curious about the warning...
        while (fgets(line, 200, fp_child_output) != NULL) {
            char *value=line;
            while (*value && *value != '=') value++;
            if (*value=='=') {
                *value='\0';
                value++;
                char *end = value;
                while (*end && *end != '\n') end++;
                *end = '\0';    // Overwrite newline if present
                coverity_remove_taint(line);    // I/O is controlled result of system_info.sh - not tainted
                coverity_remove_taint(value);

                if(unlikely(rrdhost_set_system_info_variable(system_info, line, value))) {
                    netdata_log_error("Unexpected environment variable %s=%s", line, value);
                } else {
                    setenv(line, value, 1);
                }
            }
        }
        netdata_pclose(fp_child_input, fp_child_output, command_pid);
    }
    freez(script);
    return 0;
}

/* Any config setting that can be accessed without a default value i.e. configget(...,...,NULL) *MUST*
   be set in this procedure to be called in all the relevant code paths.
*/

#define delta_startup_time(msg)                         \
    {                                                   \
        usec_t now_ut = now_monotonic_usec();           \
        if(prev_msg)                                    \
            netdata_log_info("NETDATA STARTUP: in %7llu ms, %s - next: %s", (now_ut - last_ut) / USEC_PER_MS, prev_msg, msg); \
        else                                            \
            netdata_log_info("NETDATA STARTUP: next: %s", msg);    \
        last_ut = now_ut;                               \
        prev_msg = msg;                                 \
    }

int buffer_unittest(void);
int pgc_unittest(void);
int mrg_unittest(void);
int julytest(void);
int pluginsd_parser_unittest(void);
void replication_initialize(void);
void bearer_tokens_init(void);
int unittest_rrdpush_compressions(void);
int uuid_unittest(void);
int progress_unittest(void);
int dyncfg_unittest(void);

int unittest_prepare_rrd(char **user) {
    post_conf_load(user);
    get_netdata_configured_variables();
    default_rrd_update_every = 1;
    default_rrd_memory_mode = RRD_MEMORY_MODE_RAM;
    health_plugin_disable();
    storage_tiers = 1;
    registry_init();
    if(rrd_init("unittest", NULL, true)) {
        fprintf(stderr, "rrd_init failed for unittest\n");
        return 1;
    }
    default_rrdpush_enabled = 0;

    return 0;
}

int main(int argc, char **argv) {
    // initialize the system clocks
    clocks_init();
    netdata_start_time = now_realtime_sec();

    usec_t started_ut = now_monotonic_usec();
    usec_t last_ut = started_ut;
    const char *prev_msg = NULL;

    int i;
    int config_loaded = 0;
    int dont_fork = 0;
    bool close_open_fds = true;
    size_t default_stacksize;
    char *user = NULL;

    static_threads = static_threads_get();

    netdata_ready = false;
    // set the name for logging
    program_name = "netdata";

    if (argc > 1 && strcmp(argv[1], SPAWN_SERVER_COMMAND_LINE_ARGUMENT) == 0) {
        // don't run netdata, this is the spawn server
        spawn_server();
        exit(0);
    }

    // parse options
    {
        int num_opts = sizeof(option_definitions) / sizeof(struct option_def);
        char optstring[(num_opts * 2) + 1];

        int string_i = 0;
        for( i = 0; i < num_opts; i++ ) {
            optstring[string_i] = option_definitions[i].val;
            string_i++;
            if(option_definitions[i].arg_name) {
                optstring[string_i] = ':';
                string_i++;
            }
        }
        // terminate optstring
        optstring[string_i] ='\0';
        optstring[(num_opts *2)] ='\0';

        int opt;
        while( (opt = getopt(argc, argv, optstring)) != -1 ) {
            switch(opt) {
                case 'c':
                    if(!load_netdata_conf(optarg, 1, &user)) {
                        netdata_log_error("Cannot load configuration file %s.", optarg);
                        return 1;
                    }
                    else {
                        netdata_log_debug(D_OPTIONS, "Configuration loaded from %s.", optarg);
                        load_cloud_conf(1);
                        config_loaded = 1;
                    }
                    break;
                case 'D':
                    dont_fork = 1;
                    break;
                case 'd':
                    dont_fork = 0;
                    break;
                case 'h':
                    return help(0);
                case 'i':
                    config_set(CONFIG_SECTION_WEB, "bind to", optarg);
                    break;
                case 'P':
                    strncpy(pidfile, optarg, FILENAME_MAX);
                    pidfile[FILENAME_MAX] = '\0';
                    break;
                case 'p':
                    config_set(CONFIG_SECTION_GLOBAL, "default port", optarg);
                    break;
                case 's':
                    config_set(CONFIG_SECTION_GLOBAL, "host access prefix", optarg);
                    break;
                case 't':
                    config_set(CONFIG_SECTION_GLOBAL, "update every", optarg);
                    break;
                case 'u':
                    config_set(CONFIG_SECTION_GLOBAL, "run as user", optarg);
                    break;
                case 'v':
                case 'V':
                    printf("%s %s\n", program_name, program_version);
                    return 0;
                case 'W':
                    {
                        char* stacksize_string = "stacksize=";
                        char* debug_flags_string = "debug_flags=";
                        char* claim_string = "claim";
#ifdef ENABLE_DBENGINE
                        char* createdataset_string = "createdataset=";
                        char* stresstest_string = "stresstest=";

                        if(strcmp(optarg, "pgd-tests") == 0) {
                            return pgd_test(argc, argv);
                        }
#endif

                        if(strcmp(optarg, "sqlite-meta-recover") == 0) {
                            sql_init_database(DB_CHECK_RECOVER, 0);
                            return 0;
                        }

                        if(strcmp(optarg, "sqlite-compact") == 0) {
                            sql_init_database(DB_CHECK_RECLAIM_SPACE, 0);
                            return 0;
                        }

                        if(strcmp(optarg, "sqlite-analyze") == 0) {
                            sql_init_database(DB_CHECK_ANALYZE, 0);
                            return 0;
                        }

                        if(strcmp(optarg, "unittest") == 0) {
                            unittest_running = true;

                            if (pluginsd_parser_unittest()) return 1;
                            if (unit_test_static_threads()) return 1;
                            if (unit_test_buffer()) return 1;
                            if (unit_test_str2ld()) return 1;
                            if (buffer_unittest()) return 1;
                            if (unit_test_bitmaps()) return 1;

                            // No call to load the config file on this code-path
                            if (unittest_prepare_rrd(&user)) return 1;
                            if (run_all_mockup_tests()) return 1;
                            if (unit_test_storage()) return 1;
#ifdef ENABLE_DBENGINE
                            if (test_dbengine()) return 1;
#endif
                            if (test_sqlite()) return 1;
                            if (string_unittest(10000)) return 1;
                            if (dictionary_unittest(10000)) return 1;
                            if (aral_unittest(10000)) return 1;
                            if (rrdlabels_unittest()) return 1;
                            if (ctx_unittest()) return 1;
                            if (uuid_unittest()) return 1;
                            if (dyncfg_unittest()) return 1;
                            fprintf(stderr, "\n\nALL TESTS PASSED\n\n");
                            return 0;
                        }
                        else if(strcmp(optarg, "escapetest") == 0) {
                            return command_argument_sanitization_tests();
                        }
                        else if(strcmp(optarg, "dicttest") == 0) {
                            unittest_running = true;
                            return dictionary_unittest(10000);
                        }
                        else if(strcmp(optarg, "araltest") == 0) {
                            unittest_running = true;
                            return aral_unittest(10000);
                        }
                        else if(strcmp(optarg, "stringtest") == 0)  {
                            unittest_running = true;
                            return string_unittest(10000);
                        }
                        else if(strcmp(optarg, "rrdlabelstest") == 0) {
                            unittest_running = true;
                            return rrdlabels_unittest();
                        }
                        else if(strcmp(optarg, "buffertest") == 0) {
                            unittest_running = true;
                            return buffer_unittest();
                        }
                        else if(strcmp(optarg, "uuidtest") == 0) {
                            unittest_running = true;
                            return uuid_unittest();
                        }
#ifdef ENABLE_DBENGINE
                        else if(strcmp(optarg, "mctest") == 0) {
                            unittest_running = true;
                            return mc_unittest();
                        }
                        else if(strcmp(optarg, "ctxtest") == 0) {
                            unittest_running = true;
                            return ctx_unittest();
                        }
                        else if(strcmp(optarg, "metatest") == 0) {
                            unittest_running = true;
                            return metadata_unittest();
                        }
                        else if(strcmp(optarg, "pgctest") == 0) {
                            unittest_running = true;
                            return pgc_unittest();
                        }
                        else if(strcmp(optarg, "mrgtest") == 0) {
                            unittest_running = true;
                            return mrg_unittest();
                        }
                        else if(strcmp(optarg, "julytest") == 0) {
                            unittest_running = true;
                            return julytest();
                        }
                        else if(strcmp(optarg, "parsertest") == 0) {
                            unittest_running = true;
                            return pluginsd_parser_unittest();
                        }
                        else if(strcmp(optarg, "rrdpush_compressions_test") == 0) {
                            unittest_running = true;
                            return unittest_rrdpush_compressions();
                        }
                        else if(strcmp(optarg, "progresstest") == 0) {
                            unittest_running = true;
                            return progress_unittest();
                        }
                        else if(strcmp(optarg, "dyncfgtest") == 0) {
                            unittest_running = true;
                            if(unittest_prepare_rrd(&user))
                                return 1;
                            return dyncfg_unittest();
                        }
                        else if(strncmp(optarg, createdataset_string, strlen(createdataset_string)) == 0) {
                            optarg += strlen(createdataset_string);
                            unsigned history_seconds = strtoul(optarg, NULL, 0);
                            post_conf_load(&user);
                            get_netdata_configured_variables();
                            default_rrd_update_every = 1;
                            registry_init();
                            if(rrd_init("dbengine-dataset", NULL, true)) {
                                fprintf(stderr, "rrd_init failed for unittest\n");
                                return 1;
                            }
                            generate_dbengine_dataset(history_seconds);
                            return 0;
                        }
                        else if(strncmp(optarg, stresstest_string, strlen(stresstest_string)) == 0) {
                            char *endptr;
                            unsigned test_duration_sec = 0, dset_charts = 0, query_threads = 0, ramp_up_seconds = 0,
                            page_cache_mb = 0, disk_space_mb = 0, workers = 16;

                            optarg += strlen(stresstest_string);
                            test_duration_sec = (unsigned)strtoul(optarg, &endptr, 0);
                            if (',' == *endptr)
                                dset_charts = (unsigned)strtoul(endptr + 1, &endptr, 0);
                            if (',' == *endptr)
                                query_threads = (unsigned)strtoul(endptr + 1, &endptr, 0);
                            if (',' == *endptr)
                                ramp_up_seconds = (unsigned)strtoul(endptr + 1, &endptr, 0);
                            if (',' == *endptr)
                                page_cache_mb = (unsigned)strtoul(endptr + 1, &endptr, 0);
                            if (',' == *endptr)
                                disk_space_mb = (unsigned)strtoul(endptr + 1, &endptr, 0);
                            if (',' == *endptr)
                                workers = (unsigned)strtoul(endptr + 1, &endptr, 0);

                            if (workers > 1024)
                                workers = 1024;

                            char workers_str[16];
                            snprintf(workers_str, 15, "%u", workers);
                            setenv("UV_THREADPOOL_SIZE", workers_str, 1);
                            dbengine_stress_test(test_duration_sec, dset_charts, query_threads, ramp_up_seconds,
                                                 page_cache_mb, disk_space_mb);
                            return 0;
                        }
#endif
                        else if(strcmp(optarg, "simple-pattern") == 0) {
                            if(optind + 2 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W simple-pattern 'pattern' 'string'\n\n"
                                        " Checks if 'pattern' matches the given 'string'.\n"
                                        " - 'pattern' can be one or more space separated words.\n"
                                        " - each 'word' can contain one or more asterisks.\n"
                                        " - words starting with '!' give negative matches.\n"
                                        " - words are processed left to right\n"
                                        "\n"
                                        "Examples:\n"
                                        "\n"
                                        " > match all veth interfaces, except veth0:\n"
                                        "\n"
                                        "   -W simple-pattern '!veth0 veth*' 'veth12'\n"
                                        "\n"
                                        "\n"
                                        " > match all *.ext files directly in /path/:\n"
                                        "   (this will not match *.ext files in a subdir of /path/)\n"
                                        "\n"
                                        "   -W simple-pattern '!/path/*/*.ext /path/*.ext' '/path/test.ext'\n"
                                        "\n"
                                );
                                return 1;
                            }

                            const char *haystack = argv[optind];
                            const char *needle = argv[optind + 1];
                            size_t len = strlen(needle) + 1;
                            char wildcarded[len];

                            SIMPLE_PATTERN *p = simple_pattern_create(haystack, NULL, SIMPLE_PATTERN_EXACT, true);
                            SIMPLE_PATTERN_RESULT ret = simple_pattern_matches_extract(p, needle, wildcarded, len);
                            simple_pattern_free(p);

                            if(ret == SP_MATCHED_POSITIVE) {
                                fprintf(stdout, "RESULT: POSITIVE MATCHED - pattern '%s' matches '%s', wildcarded '%s'\n", haystack, needle, wildcarded);
                                return 0;
                            }
                            else if(ret == SP_MATCHED_NEGATIVE) {
                                fprintf(stdout, "RESULT: NEGATIVE MATCHED - pattern '%s' matches '%s', wildcarded '%s'\n", haystack, needle, wildcarded);
                                return 0;
                            }
                            else {
                                fprintf(stdout, "RESULT: NOT MATCHED - pattern '%s' does not match '%s', wildcarded '%s'\n", haystack, needle, wildcarded);
                                return 1;
                            }
                        }
                        else if(strncmp(optarg, stacksize_string, strlen(stacksize_string)) == 0) {
                            optarg += strlen(stacksize_string);
                            config_set(CONFIG_SECTION_GLOBAL, "pthread stack size", optarg);
                        }
                        else if(strncmp(optarg, debug_flags_string, strlen(debug_flags_string)) == 0) {
                            optarg += strlen(debug_flags_string);
                            config_set(CONFIG_SECTION_LOGS, "debug flags",  optarg);
                            debug_flags = strtoull(optarg, NULL, 0);
                        }
                        else if(strcmp(optarg, "set") == 0) {
                            if(optind + 3 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W set 'section' 'key' 'value'\n\n"
                                        " Overwrites settings of netdata.conf.\n"
                                        "\n"
                                        " These options interact with: -c netdata.conf\n"
                                        " If -c netdata.conf is given on the command line,\n"
                                        " before -W set... the user may overwrite command\n"
                                        " line parameters at netdata.conf\n"
                                        " If -c netdata.conf is given after (or missing)\n"
                                        " -W set... the user cannot overwrite the command line\n"
                                        " parameters."
                                        "\n"
                                );
                                return 1;
                            }
                            const char *section = argv[optind];
                            const char *key = argv[optind + 1];
                            const char *value = argv[optind + 2];
                            optind += 3;

                            // set this one as the default
                            // only if it is not already set in the config file
                            // so the caller can use -c netdata.conf before or
                            // after this parameter to prevent or allow overwriting
                            // variables at netdata.conf
                            config_set_default(section, key,  value);

                            // fprintf(stderr, "SET section '%s', key '%s', value '%s'\n", section, key, value);
                        }
                        else if(strcmp(optarg, "set2") == 0) {
                            if(optind + 4 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W set 'conf_file' 'section' 'key' 'value'\n\n"
                                        " Overwrites settings of netdata.conf or cloud.conf\n"
                                        "\n"
                                        " These options interact with: -c netdata.conf\n"
                                        " If -c netdata.conf is given on the command line,\n"
                                        " before -W set... the user may overwrite command\n"
                                        " line parameters at netdata.conf\n"
                                        " If -c netdata.conf is given after (or missing)\n"
                                        " -W set... the user cannot overwrite the command line\n"
                                        " parameters."
                                        " conf_file can be \"cloud\" or \"netdata\".\n"
                                        "\n"
                                );
                                return 1;
                            }
                            const char *conf_file = argv[optind]; /* "cloud" is cloud.conf, otherwise netdata.conf */
                            struct config *tmp_config = strcmp(conf_file, "cloud") ? &netdata_config : &cloud_config;
                            const char *section = argv[optind + 1];
                            const char *key = argv[optind + 2];
                            const char *value = argv[optind + 3];
                            optind += 4;

                            // set this one as the default
                            // only if it is not already set in the config file
                            // so the caller can use -c netdata.conf before or
                            // after this parameter to prevent or allow overwriting
                            // variables at netdata.conf
                            appconfig_set_default(tmp_config, section, key,  value);

                            // fprintf(stderr, "SET section '%s', key '%s', value '%s'\n", section, key, value);
                        }
                        else if(strcmp(optarg, "get") == 0) {
                            if(optind + 3 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W get 'section' 'key' 'value'\n\n"
                                        " Prints settings of netdata.conf.\n"
                                        "\n"
                                        " These options interact with: -c netdata.conf\n"
                                        " -c netdata.conf has to be given before -W get.\n"
                                        "\n"
                                );
                                return 1;
                            }

                            if(!config_loaded) {
                                fprintf(stderr, "warning: no configuration file has been loaded. Use -c CONFIG_FILE, before -W get. Using default config.\n");
                                load_netdata_conf(NULL, 0, &user);
                            }

                            get_netdata_configured_variables();

                            const char *section = argv[optind];
                            const char *key = argv[optind + 1];
                            const char *def = argv[optind + 2];
                            const char *value = config_get(section, key, def);
                            printf("%s\n", value);
                            return 0;
                        }
                        else if(strcmp(optarg, "get2") == 0) {
                            if(optind + 4 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W get2 'conf_file' 'section' 'key' 'value'\n\n"
                                        " Prints settings of netdata.conf or cloud.conf\n"
                                        "\n"
                                        " These options interact with: -c netdata.conf\n"
                                        " -c netdata.conf has to be given before -W get2.\n"
                                        " conf_file can be \"cloud\" or \"netdata\".\n"
                                        "\n"
                                );
                                return 1;
                            }

                            if(!config_loaded) {
                                fprintf(stderr, "warning: no configuration file has been loaded. Use -c CONFIG_FILE, before -W get. Using default config.\n");
                                load_netdata_conf(NULL, 0, &user);
                                load_cloud_conf(1);
                            }

                            get_netdata_configured_variables();

                            const char *conf_file = argv[optind]; /* "cloud" is cloud.conf, otherwise netdata.conf */
                            struct config *tmp_config = strcmp(conf_file, "cloud") ? &netdata_config : &cloud_config;
                            const char *section = argv[optind + 1];
                            const char *key = argv[optind + 2];
                            const char *def = argv[optind + 3];
                            const char *value = appconfig_get(tmp_config, section, key, def);
                            printf("%s\n", value);
                            return 0;
                        }
                        else if(strncmp(optarg, claim_string, strlen(claim_string)) == 0) {
                            /* will trigger a claiming attempt when the agent is initialized */
                            claiming_pending_arguments = optarg + strlen(claim_string);
                        }
                        else if(strcmp(optarg, "buildinfo") == 0) {
                            print_build_info();
                            return 0;
                        }
                        else if(strcmp(optarg, "buildinfojson") == 0) {
                            print_build_info_json();
                            return 0;
                        }
                        else if(strcmp(optarg, "keepopenfds") == 0) {
                            // Internal dev option to skip closing inherited
                            // open FDs. Useful, when we want to run the agent
                            // under profiling tools that open/maintain their
                            // own FDs.
                            close_open_fds = false;
                        } else {
                            fprintf(stderr, "Unknown -W parameter '%s'\n", optarg);
                            return help(1);
                        }
                    }
                    break;

                default: /* ? */
                    fprintf(stderr, "Unknown parameter '%c'\n", opt);
                    return help(1);
            }
        }
    }

    if (close_open_fds == true) {
        // close all open file descriptors, except the standard ones
        // the caller may have left open files (lxc-attach has this issue)
        for_each_open_fd(OPEN_FD_ACTION_CLOSE, OPEN_FD_EXCLUDE_STDIN | OPEN_FD_EXCLUDE_STDOUT | OPEN_FD_EXCLUDE_STDERR);
    }


    if(!config_loaded) {
        load_netdata_conf(NULL, 0, &user);
        load_cloud_conf(0);
    }

    // ------------------------------------------------------------------------
    // initialize netdata
    {
        char *pmax = config_get(CONFIG_SECTION_GLOBAL, "glibc malloc arena max for plugins", "1");
        if(pmax && *pmax)
            setenv("MALLOC_ARENA_MAX", pmax, 1);

#if defined(HAVE_C_MALLOPT)
        i = (int)config_get_number(CONFIG_SECTION_GLOBAL, "glibc malloc arena max for netdata", 1);
        if(i > 0)
            mallopt(M_ARENA_MAX, 1);


#ifdef NETDATA_INTERNAL_CHECKS
        mallopt(M_PERTURB, 0x5A);
        // mallopt(M_MXFAST, 0);
#endif
#endif

        // set libuv worker threads
        libuv_worker_threads = (int)get_netdata_cpus() * 6;

        if(libuv_worker_threads < MIN_LIBUV_WORKER_THREADS)
            libuv_worker_threads = MIN_LIBUV_WORKER_THREADS;

        if(libuv_worker_threads > MAX_LIBUV_WORKER_THREADS)
            libuv_worker_threads = MAX_LIBUV_WORKER_THREADS;


        libuv_worker_threads = config_get_number(CONFIG_SECTION_GLOBAL, "libuv worker threads", libuv_worker_threads);
        if(libuv_worker_threads < MIN_LIBUV_WORKER_THREADS) {
            libuv_worker_threads = MIN_LIBUV_WORKER_THREADS;
            config_set_number(CONFIG_SECTION_GLOBAL, "libuv worker threads", libuv_worker_threads);
        }

        {
            char buf[20 + 1];
            snprintfz(buf, sizeof(buf) - 1, "%d", libuv_worker_threads);
            setenv("UV_THREADPOOL_SIZE", buf, 1);
        }

        // prepare configuration environment variables for the plugins
        get_netdata_configured_variables();
        set_global_environment();

        // work while we are cd into config_dir
        // to allow the plugins refer to their config
        // files using relative filenames
        if(chdir(netdata_configured_user_config_dir) == -1)
            fatal("Cannot cd to '%s'", netdata_configured_user_config_dir);

        // Get execution path before switching user to avoid permission issues
        get_netdata_execution_path();
    }

    {
        // --------------------------------------------------------------------
        // get the debugging flags from the configuration file

        char *flags = config_get(CONFIG_SECTION_LOGS, "debug flags",  "0x0000000000000000");
        setenv("NETDATA_DEBUG_FLAGS", flags, 1);

        debug_flags = strtoull(flags, NULL, 0);
        netdata_log_debug(D_OPTIONS, "Debug flags set to '0x%" PRIX64 "'.", debug_flags);

        if(debug_flags != 0) {
            struct rlimit rl = { RLIM_INFINITY, RLIM_INFINITY };
            if(setrlimit(RLIMIT_CORE, &rl) != 0)
                netdata_log_error("Cannot request unlimited core dumps for debugging... Proceeding anyway...");

#ifdef HAVE_SYS_PRCTL_H
            prctl(PR_SET_DUMPABLE, 1, 0, 0, 0);
#endif
        }


        // --------------------------------------------------------------------
        // get log filenames and settings

        log_init();
        nd_log_limits_unlimited();

        // initialize the log files
        nd_log_initialize();
        netdata_log_info("Netdata agent version \""VERSION"\" is starting");

        ieee754_doubles = is_system_ieee754_double();
        if(!ieee754_doubles)
            globally_disabled_capabilities |= STREAM_CAP_IEEE754;

        aral_judy_init();

        get_system_timezone();

        bearer_tokens_init();

        replication_initialize();

        rrd_functions_inflight_init();

        // --------------------------------------------------------------------
        // get the certificate and start security

#ifdef ENABLE_HTTPS
        security_init();
#endif

        // --------------------------------------------------------------------
        // This is the safest place to start the SILENCERS structure

        health_set_silencers_filename();
        health_initialize_global_silencers();

        // --------------------------------------------------------------------
        // Initialize ML configuration

        delta_startup_time("initialize ML");
        ml_init();

        // --------------------------------------------------------------------
        // setup process signals

        // block signals while initializing threads.
        // this causes the threads to block signals.

        delta_startup_time("initialize signals");
        signals_block();
        signals_init(); // setup the signals we want to use

        // --------------------------------------------------------------------
        // check which threads are enabled and initialize them

        delta_startup_time("initialize static threads");

        // setup threads configs
        default_stacksize = netdata_threads_init();

#ifdef NETDATA_INTERNAL_CHECKS
        config_set_boolean(CONFIG_SECTION_PLUGINS, "netdata monitoring", true);
        config_set_boolean(CONFIG_SECTION_PLUGINS, "netdata monitoring extended", true);
#endif

        if(config_get_boolean(CONFIG_SECTION_PLUGINS, "netdata monitoring extended", false))
            // this has to run before starting any other threads that use workers
            workers_utilization_enable();

        for (i = 0; static_threads[i].name != NULL ; i++) {
            struct netdata_static_thread *st = &static_threads[i];

            if(st->config_name)
                st->enabled = config_get_boolean(st->config_section, st->config_name, st->enabled);

            if(st->enabled && st->init_routine)
                st->init_routine();

            if(st->env_name)
                setenv(st->env_name, st->enabled?"YES":"NO", 1);

            if(st->global_variable)
                *st->global_variable = (st->enabled) ? true : false;
        }

        // --------------------------------------------------------------------
        // create the listening sockets

        delta_startup_time("initialize web server");

        web_client_api_v1_init();
        web_server_threading_selection();

        if(web_server_mode != WEB_SERVER_MODE_NONE)
            api_listen_sockets_setup();

#ifdef ENABLE_H2O
        delta_startup_time("initialize h2o server");
        for (int t = 0; static_threads[t].name; t++) {
            if (static_threads[t].start_routine == h2o_main)
                static_threads[t].enabled = httpd_is_enabled();
        }
#endif
    }

    delta_startup_time("set resource limits");

#ifdef NETDATA_INTERNAL_CHECKS
    if(debug_flags != 0) {
        struct rlimit rl = { RLIM_INFINITY, RLIM_INFINITY };
        if(setrlimit(RLIMIT_CORE, &rl) != 0)
            netdata_log_error("Cannot request unlimited core dumps for debugging... Proceeding anyway...");
#ifdef HAVE_SYS_PRCTL_H
        prctl(PR_SET_DUMPABLE, 1, 0, 0, 0);
#endif
    }
#endif /* NETDATA_INTERNAL_CHECKS */

    set_nofile_limit(&rlimit_nofile);

    delta_startup_time("become daemon");

    // fork, switch user, create pid file, set process priority
    if(become_daemon(dont_fork, user) == -1)
        fatal("Cannot daemonize myself.");

    // The "HOME" env var points to the root's home dir because Netdata starts as root. Can't use "HOME".
    struct passwd *pw = getpwuid(getuid());
    if (config_exists(CONFIG_SECTION_DIRECTORIES, "home") || !pw || !pw->pw_dir) {
        netdata_configured_home_dir = config_get(CONFIG_SECTION_DIRECTORIES, "home", netdata_configured_home_dir);
    } else {
        netdata_configured_home_dir = config_get(CONFIG_SECTION_DIRECTORIES, "home", pw->pw_dir);
    }

    setenv("HOME", netdata_configured_home_dir, 1);

    dyncfg_init(true);

    netdata_log_info("netdata started on pid %d.", getpid());

    delta_startup_time("initialize threads after fork");

    netdata_threads_init_after_fork((size_t)config_get_number(CONFIG_SECTION_GLOBAL, "pthread stack size", (long)default_stacksize));

    // initialize internal registry
    delta_startup_time("initialize registry");
    registry_init();

    // fork the spawn server
    delta_startup_time("fork the spawn server");
    spawn_init();

    /*
     * Libuv uv_spawn() uses SIGCHLD internally:
     * https://github.com/libuv/libuv/blob/cc51217a317e96510fbb284721d5e6bc2af31e33/src/unix/process.c#L485
     * and inadvertently replaces the netdata signal handler which was setup during initialization.
     * Thusly, we must explicitly restore the signal handler for SIGCHLD.
     * Warning: extreme care is needed when mixing and matching POSIX and libuv.
     */
    signals_restore_SIGCHLD();

    // ------------------------------------------------------------------------
    // initialize rrd, registry, health, rrdpush, etc.

    delta_startup_time("collecting system info");

    netdata_anonymous_statistics_enabled=-1;
    struct rrdhost_system_info *system_info = callocz(1, sizeof(struct rrdhost_system_info));
    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(struct rrdhost_system_info), __ATOMIC_RELAXED);
    get_system_info(system_info);
    (void) registry_get_this_machine_guid();
    system_info->hops = 0;
    get_install_type(&system_info->install_type, &system_info->prebuilt_arch, &system_info->prebuilt_dist);

    delta_startup_time("initialize RRD structures");

    if(rrd_init(netdata_configured_hostname, system_info, false)) {
        set_late_global_environment(system_info);
        fatal("Cannot initialize localhost instance with name '%s'.", netdata_configured_hostname);
    }

    delta_startup_time("check for incomplete shutdown");

    char agent_crash_file[FILENAME_MAX + 1];
    char agent_incomplete_shutdown_file[FILENAME_MAX + 1];
    snprintfz(agent_incomplete_shutdown_file, FILENAME_MAX, "%s/.agent_incomplete_shutdown", netdata_configured_varlib_dir);
    int incomplete_shutdown_detected = (unlink(agent_incomplete_shutdown_file) == 0);
    snprintfz(agent_crash_file, FILENAME_MAX, "%s/.agent_crash", netdata_configured_varlib_dir);
    int crash_detected = (unlink(agent_crash_file) == 0);
    int fd = open(agent_crash_file, O_WRONLY | O_CREAT | O_TRUNC, 444);
    if (fd >= 0)
        close(fd);


    // ------------------------------------------------------------------------
    // Claim netdata agent to a cloud endpoint

    delta_startup_time("collect claiming info");

    if (claiming_pending_arguments)
         claim_agent(claiming_pending_arguments, false, NULL);
    load_claiming_state();

    // ------------------------------------------------------------------------
    // enable log flood protection

    nd_log_limits_reset();

    // Load host labels
    delta_startup_time("collect host labels");
    reload_host_labels();

    // ------------------------------------------------------------------------
    // spawn the threads

    delta_startup_time("start the static threads");

    web_server_config_options();

    netdata_zero_metrics_enabled = config_get_boolean_ondemand(CONFIG_SECTION_DB, "enable zero metrics", CONFIG_BOOLEAN_NO);

    set_late_global_environment(system_info);
    for (i = 0; static_threads[i].name != NULL ; i++) {
        struct netdata_static_thread *st = &static_threads[i];

        if(st->enabled) {
            st->thread = mallocz(sizeof(netdata_thread_t));
            netdata_log_debug(D_SYSTEM, "Starting thread %s.", st->name);
            netdata_thread_create(st->thread, st->name, NETDATA_THREAD_OPTION_DEFAULT, st->start_routine, st);
        }
        else
            netdata_log_debug(D_SYSTEM, "Not starting thread %s.", st->name);
    }
    ml_start_threads();

    // ------------------------------------------------------------------------
    // Initialize netdata agent command serving from cli and signals

    delta_startup_time("initialize commands API");

    commands_init();

    delta_startup_time("ready");

    usec_t ready_ut = now_monotonic_usec();
    netdata_log_info("NETDATA STARTUP: completed in %llu ms. Enjoy real-time performance monitoring!", (ready_ut - started_ut) / USEC_PER_MS);
    netdata_ready = true;

    analytics_statistic_t start_statistic = { "START", "-",  "-" };
    analytics_statistic_send(&start_statistic);
    if (crash_detected) {
        analytics_statistic_t crash_statistic = { "CRASH", "-",  "-" };
        analytics_statistic_send(&crash_statistic);
    }
    if (incomplete_shutdown_detected) {
        analytics_statistic_t incomplete_shutdown_statistic = { "INCOMPLETE_SHUTDOWN", "-", "-" };
        analytics_statistic_send(&incomplete_shutdown_statistic);
    }

    //check if ANALYTICS needs to start
    if (netdata_anonymous_statistics_enabled == 1) {
        for (i = 0; static_threads[i].name != NULL; i++) {
            if (!strncmp(static_threads[i].name, "ANALYTICS", 9)) {
                struct netdata_static_thread *st = &static_threads[i];
                st->thread = mallocz(sizeof(netdata_thread_t));
                st->enabled = 1;
                netdata_log_debug(D_SYSTEM, "Starting thread %s.", st->name);
                netdata_thread_create(st->thread, st->name, NETDATA_THREAD_OPTION_DEFAULT, st->start_routine, st);
            }
        }
    }

    // ------------------------------------------------------------------------
    // Report ACLK build failure
#ifndef ENABLE_ACLK
    netdata_log_error("This agent doesn't have ACLK.");
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/.aclk_report_sent", netdata_configured_varlib_dir);
    if (netdata_anonymous_statistics_enabled > 0 && access(filename, F_OK)) { // -1 -> not initialized
        analytics_statistic_t statistic = { "ACLK_DISABLED", "-", "-" };
        analytics_statistic_send(&statistic);

        int fd = open(filename, O_WRONLY | O_CREAT | O_TRUNC, 444);
        if (fd == -1)
            netdata_log_error("Cannot create file '%s'. Please fix this.", filename);
        else
            close(fd);
    }
#endif

    // ------------------------------------------------------------------------
    // initialize WebRTC

    webrtc_initialize();

    // ------------------------------------------------------------------------
    // unblock signals

    signals_unblock();

    // ------------------------------------------------------------------------
    // Handle signals

    signals_handle();

    // should never reach this point
    // but we need it for rpmlint #2752
    return 1;
}
