#include "worker_utilization.h"

#define WORKER_IDLE 'I'
#define WORKER_BUSY 'B'

struct worker {
    pid_t pid;
    const char *tag;
    const char *workname;
    uint32_t workname_hash;

    // only one variable is set by our statistics callers
    usec_t statistics_last_checkpoint;
    size_t statistics_last_jobs_done;
    usec_t statistics_last_busy_time;

    // the worker controlled variables
    volatile size_t jobs_done;
    volatile usec_t busy_time;
    volatile usec_t last_action_timestamp;
    volatile char last_action;

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
    worker->last_action = WORKER_IDLE;

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

    freez((void *)worker->tag);
    freez((void *)worker->workname);
    freez(worker);

    worker = NULL;
}

void worker_is_idle(void) {
    if(unlikely(!worker)) return;
    if(unlikely(worker->last_action != WORKER_BUSY)) return;

    usec_t now = now_realtime_usec();

    worker->busy_time += now - worker->last_action_timestamp;
    worker->jobs_done++;

    // the worker was busy
    // set it to idle before we set the timestamp

    worker->last_action = WORKER_IDLE;
    if(worker->last_action_timestamp < now)
        worker->last_action_timestamp = now;
}

void worker_is_busy(void) {
    if(unlikely(!worker)) return;
    if(unlikely(worker->last_action != WORKER_IDLE)) return;

    usec_t now = now_realtime_usec();

    // the worker was idle
    // set the timestamp and then set it to busy

    worker->last_action_timestamp = now;
    worker->last_action = WORKER_BUSY;
}


// statistics interface

void workers_foreach(const char *workname, void (*callback)(void *data, pid_t pid, const char *thread_tag, size_t utilization_usec, size_t duration_usec, size_t jobs_done, size_t jobs_running), void *data) {
    netdata_mutex_lock(&base_lock);
    uint32_t hash = simple_hash(workname);
    usec_t busy_time, delta;
    size_t jobs_done, jobs_running;

    struct worker *p;
    for(p = base; p ; p = p->next) {
        if(hash != p->workname_hash || strcmp(workname, p->workname)) continue;

        usec_t now = now_realtime_usec();

        // get a copy of the worker variables
        usec_t worker_busy_time = p->busy_time;
        size_t worker_jobs_done = p->jobs_done;
        char worker_last_action = p->last_action;
        usec_t worker_last_action_timestamp = p->last_action_timestamp;

        // this is the only variable both the worker thread and the statistics thread are writing
        // we set this only when the worker is busy, so that worker will not
        // accumulate all the busy time, but only the time after the point we collected statistics
        if(worker_last_action == WORKER_BUSY && p->last_action_timestamp == worker_last_action_timestamp && p->last_action == WORKER_BUSY)
            p->last_action_timestamp = now;

        // calculate delta busy time
        busy_time = worker_busy_time - p->statistics_last_busy_time;
        p->statistics_last_busy_time = worker_busy_time;

        // calculate delta jobs done
        jobs_done = worker_jobs_done - p->statistics_last_jobs_done;
        p->statistics_last_jobs_done = worker_jobs_done;

        jobs_running = 0;
        if(worker_last_action == WORKER_BUSY) {
            // the worker is still busy with something
            // let's add that busy time to the reported one
            busy_time += now - worker_last_action_timestamp;
            jobs_running = 1;
        }

        delta = now - p->statistics_last_checkpoint;

        p->statistics_last_checkpoint = now;

        callback(data, p->pid, p->tag, busy_time, delta, jobs_done, jobs_running);
    }

    netdata_mutex_unlock(&base_lock);
}
