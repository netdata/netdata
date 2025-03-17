// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon-service.h"

typedef struct service_thread {
    pid_t tid;
    SERVICE_THREAD_TYPE type;
    SERVICE_TYPE services;
    char name[ND_THREAD_TAG_MAX + 1];
    bool stop_immediately;
    bool cancelled;

    union {
        ND_THREAD *netdata_thread;
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
    pid_t tid = gettid_cached();

    spinlock_lock(&service_globals.lock);
    Pvoid_t *PValue = JudyLIns(&service_globals.pid_judy, tid, PJE0);
    if(!*PValue) {
        sth = callocz(1, sizeof(SERVICE_THREAD));
        sth->tid = tid;
        sth->type = thread_type;
        sth->request_quit_callback = request_quit_callback;
        sth->force_quit_callback = force_quit_callback;
        sth->data = data;
        *PValue = sth;

        switch(thread_type) {
            default:
            case SERVICE_THREAD_TYPE_NETDATA:
                sth->netdata_thread = nd_thread_self();
                break;

            case SERVICE_THREAD_TYPE_EVENT_LOOP:
            case SERVICE_THREAD_TYPE_LIBUV:
                sth->uv_thread = uv_thread_self();
                break;
        }

        const char *name = nd_thread_tag();
        if(!name) name = "";
        strncpyz(sth->name, name, sizeof(sth->name) - 1);
    }
    else {
        sth = *PValue;
    }
    spinlock_unlock(&service_globals.lock);

    return sth;
}

void service_exits(void) {
    pid_t tid = gettid_cached();

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

    bool cancelled = false;
    if (sth->type == SERVICE_THREAD_TYPE_NETDATA)
        cancelled = nd_thread_signaled_to_cancel();

    return !sth->stop_immediately && !exit_initiated_get() && !cancelled;
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

            switch(sth->type) {
                default:
                case SERVICE_THREAD_TYPE_NETDATA:
                    nd_thread_signal_cancel(sth->netdata_thread);
                    break;

                case SERVICE_THREAD_TYPE_EVENT_LOOP:
                case SERVICE_THREAD_TYPE_LIBUV:
                    break;
            }

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

bool service_wait_exit(SERVICE_TYPE service, usec_t timeout_ut) {
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
            if(sth->services & service && sth->tid != gettid_cached() && !sth->cancelled) {
                sth->cancelled = true;

                switch(sth->type) {
                    default:
                    case SERVICE_THREAD_TYPE_NETDATA:
                        nd_thread_signal_cancel(sth->netdata_thread);
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
            if(sth->services & service && sth->tid != gettid_cached()) {
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
