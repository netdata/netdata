#include "worker_utilization.h"

#define WORKER_IDLE 'I'
#define WORKER_BUSY 'B'

struct worker {
    // netdata_mutex_t lock;

    pid_t pid;
    const char *tag;
    const char *workname;
    uint32_t workname_hash;

    // only one variable is set by our statistics callers
    usec_t statistics_last_checkpoint;

    // the worker controlled variables
    size_t jobs_done;
    usec_t utilization;
    usec_t last_checkpoint;
    usec_t last_action_timestamp;
    char last_action;

    struct worker *next;
};

static netdata_mutex_t base_lock = NETDATA_MUTEX_INITIALIZER;
static struct worker *base = NULL;
static __thread struct worker *worker = NULL;

void worker_register(const char *workname) {
    if(worker) return;

    worker = callocz(1, sizeof(struct worker));
    worker->pid = gettid();
    worker->tag = strdupz(netdata_thread_tag());
    worker->workname = strdupz(workname);
    worker->workname_hash = simple_hash(worker->workname);

    usec_t now = now_realtime_usec();
    worker->statistics_last_checkpoint = now;
    worker->last_action_timestamp = now;
    worker->last_checkpoint = now;
    worker->last_action = WORKER_IDLE;

    // netdata_mutex_init(&worker->lock);

    netdata_mutex_lock(&base_lock);
    worker->next = base;
    base = worker;
    netdata_mutex_unlock(&base_lock);
}

void worker_unregister(void) {
    if(unlikely(!worker)) return;

    netdata_mutex_lock(&base_lock);
    if(base == worker)
        base = worker->next;
    else {
        struct worker *p;
        for(p = base; p && p->next && p->next != worker ;p = p->next);
        if(p && p->next == worker)
            p->next = worker->next;
    }
    netdata_mutex_unlock(&base_lock);

    // netdata_mutex_destroy(&worker->lock);
    freez((void *)worker->tag);
    freez((void *)worker->workname);
    freez(worker);

    worker = NULL;
}

void worker_is_idle(void) {
    if(unlikely(!worker)) return;
    if(unlikely(worker->last_action != WORKER_BUSY)) return;

    // netdata_mutex_lock(&worker->lock);
    usec_t now = now_realtime_usec();

    worker->utilization += now - worker->last_action_timestamp;
    worker->jobs_done++;
    worker->last_action_timestamp = now;
    worker->last_action = WORKER_IDLE;

    // netdata_mutex_unlock(&worker->lock);
}

void worker_is_busy(void) {
    if(unlikely(!worker)) return;
    if(unlikely(worker->last_action != WORKER_IDLE)) return;

    // netdata_mutex_lock(&worker->lock);
    usec_t now = now_realtime_usec();

    worker->last_action_timestamp = now;
    worker->last_action = WORKER_BUSY;

    // netdata_mutex_unlock(&worker->lock);
}


// statistics interface

void workers_foreach(const char *workname, void (*callback)(void *data, pid_t pid, const char *thread_tag, size_t utilization_usec, size_t duration_usec, size_t jobs_done, size_t jobs_running), void *data) {
    netdata_mutex_lock(&base_lock);
    uint32_t hash = simple_hash(workname);
    usec_t util, delta, last;
    size_t jobs_done, jobs_running;

    struct worker *p;
    for(p = base; p ; p = p->next) {
        if(hash != p->workname_hash || strcmp(workname, p->workname)) continue;

        // netdata_mutex_lock(&p->lock);
        usec_t now = now_realtime_usec();

        util = p->utilization;
        p->utilization = 0;

        jobs_done = p->jobs_done;
        p->jobs_done = 0;

        last = p->last_action_timestamp;
        p->last_action_timestamp = now;

        jobs_running = 0;
        if(p->last_action == WORKER_BUSY) {
            util += now - last;
            jobs_running = 1;
        }

        delta = now - p->statistics_last_checkpoint;

        p->statistics_last_checkpoint = now;

        // netdata_mutex_unlock(&p->lock);

        callback(data, p->pid, p->tag, util, delta, jobs_done, jobs_running);
    }

    netdata_mutex_unlock(&base_lock);
}
