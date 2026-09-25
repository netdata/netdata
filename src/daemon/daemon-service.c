// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon-service.h"

typedef struct service_thread {
    pid_t tid;
    SERVICE_TYPE services;
    char name[ND_THREAD_TAG_MAX + 1];
    bool cancelled;

    ND_THREAD *netdata_thread;

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

SERVICE_THREAD *service_register(request_quit_t request_quit_callback, force_quit_t force_quit_callback, void *data)
{
    SERVICE_THREAD *sth = NULL;
    pid_t tid = gettid_cached();

    spinlock_lock(&service_globals.lock);
    Pvoid_t *PValue = JudyLIns(&service_globals.pid_judy, tid, PJE0);
    if(!*PValue) {
        sth = callocz(1, sizeof(SERVICE_THREAD));
        sth->tid = tid;
        sth->request_quit_callback = request_quit_callback;
        sth->force_quit_callback = force_quit_callback;
        sth->data = data;
        *PValue = sth;
        sth->netdata_thread = nd_thread_self();

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

// Is any thread other than the caller still registered for any of these services?
//
// A pure query: unlike service_wait_exit() it neither signals cancellation nor waits, and it
// answers for exactly the services asked about. service_wait_exit() returns one bool for its
// whole mask, so it cannot be used to ask about one service when it was called with several.
//
// Threads are removed from this registry by service_exits(), which runs from the per-thread
// cleanup callbacks registered in main() - after the thread function has returned and
// therefore after its own cleanup handlers have run. So a "no" here means the thread finished
// its cleanup, not merely that it was asked to stop.
bool service_is_running(SERVICE_TYPE service) {
    bool running = false;

    spinlock_lock(&service_globals.lock);

    Pvoid_t *PValue;
    Word_t tid = 0;
    bool first = true;
    while((PValue = JudyLFirstThenNext(service_globals.pid_judy, &tid, &first))) {
        SERVICE_THREAD *sth = *PValue;
        if((__atomic_load_n(&sth->services, __ATOMIC_ACQUIRE) & service) && sth->tid != gettid_cached()) {
            running = true;
            break;
        }
    }

    spinlock_unlock(&service_globals.lock);

    return running;
}

bool service_running(SERVICE_TYPE service) {
    static __thread SERVICE_THREAD *sth = NULL;

    if(unlikely(!sth))
        sth = service_register(NULL, NULL, NULL);

    // Publish the bit atomically, but only the first time this thread claims a service.
    // service_running() is called constantly from the collector and health loops, so the common
    // case stays a plain relaxed read plus a branch; the atomic OR runs once per thread per
    // service. Readers (service_is_running(), service_signal_exit(), service_wait_exit()) load
    // this with ACQUIRE, so a plain non-atomic RMW here would be a genuine data race - and
    // service_is_running() now makes a shutdown DECISION on the result.
    if(!(__atomic_load_n(&sth->services, __ATOMIC_RELAXED) & service))
        __atomic_or_fetch(&sth->services, service, __ATOMIC_RELEASE);

    return !nd_thread_signaled_to_cancel() && !exit_initiated_get();
}

void service_signal_exit(SERVICE_TYPE service) {
    spinlock_lock(&service_globals.lock);

    Pvoid_t *PValue;
    Word_t tid = 0;
    bool first = true;
    while((PValue = JudyLFirstThenNext(service_globals.pid_judy, &tid, &first))) {
        SERVICE_THREAD *sth = *PValue;

        if((__atomic_load_n(&sth->services, __ATOMIC_ACQUIRE) & service)) {
            nd_thread_signal_cancel(sth->netdata_thread);
            nd_log_daemon(NDLP_DEBUG, "SERVICE: Signal to stop : %s", sth->name);
            request_quit_t request_quit_callback = sth->request_quit_callback;
            void *request_quit_data = sth->data;
            if(request_quit_callback) {
                spinlock_unlock(&service_globals.lock);
                request_quit_callback(request_quit_data);
                spinlock_lock(&service_globals.lock);
            }
        }
    }
    spinlock_unlock(&service_globals.lock);
}

static void service_to_buffer(BUFFER *wb, SERVICE_TYPE service) {
    if(service & SERVICE_COLLECTORS)
        buffer_strcat(wb, "COLLECTORS ");
    if(service & SERVICE_REPLICATION)
        buffer_strcat(wb, "REPLICATION ");
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
            if((__atomic_load_n(&sth->services, __ATOMIC_ACQUIRE) & service) && sth->tid != gettid_cached() && !sth->cancelled) {
                sth->cancelled = true;
                nd_thread_signal_cancel(sth->netdata_thread);
                if(running)
                    buffer_strcat(thread_list, ", ");

                buffer_sprintf(thread_list, "'%s' (%d)", sth->name, sth->tid);

                running++;
                running_services |= __atomic_load_n(&sth->services, __ATOMIC_ACQUIRE) & service;

                force_quit_t force_quit_callback = sth->force_quit_callback;
                void *force_quit_data = sth->data;
                if(force_quit_callback) {
                    spinlock_unlock(&service_globals.lock);
                    force_quit_callback(force_quit_data);
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
    usec_t sleep_ut = 50 * USEC_PER_MS;
    size_t log_countdown_ut = sleep_ut;
    do {
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
            if((__atomic_load_n(&sth->services, __ATOMIC_ACQUIRE) & service) && sth->tid != gettid_cached()) {
                if(running)
                    buffer_strcat(thread_list, ", ");

                buffer_sprintf(thread_list, "'%s' (%d)", sth->name, sth->tid);

                running_services |= __atomic_load_n(&sth->services, __ATOMIC_ACQUIRE) & service;
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
        }

        ended_ut = now_monotonic_usec();
    } while(running && ((ended_ut - started_ut) < timeout_ut));

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
