#include "worker_utilization.h"

#define WORKER_IDLE 'I'
#define WORKER_BUSY 'B'

struct worker_job_type {
    char name[WORKER_UTILIZATION_MAX_JOB_NAME_LENGTH + 1];

    // statistics controlled variables
    size_t statistics_last_jobs_started;
    usec_t statistics_last_busy_time;

    // worker controlled variables
    volatile size_t worker_jobs_started;
    volatile usec_t worker_busy_time;
};

struct worker {
    pid_t pid;
    const char *tag;
    const char *workname;
    uint32_t workname_hash;

    // statistics controlled variables
    volatile usec_t statistics_last_checkpoint;
    size_t statistics_last_jobs_started;
    usec_t statistics_last_busy_time;

    // the worker controlled variables
    volatile size_t job_id;
    volatile size_t jobs_started;
    volatile usec_t busy_time;
    volatile usec_t last_action_timestamp;
    volatile char last_action;

    struct worker_job_type per_job_type[WORKER_UTILIZATION_MAX_JOB_TYPES];

    struct worker *next;
};

static netdata_mutex_t base_lock = NETDATA_MUTEX_INITIALIZER;
static struct worker *base = NULL;
static __thread struct worker *worker = NULL;

void worker_register(const char *workname) {
    if(unlikely(worker)) return;

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

void worker_register_job_name(size_t job_id, const char *name) {
    if(unlikely(!worker)) return;

    if(unlikely(job_id >= WORKER_UTILIZATION_MAX_JOB_TYPES)) {
        error("WORKER_UTILIZATION: job_id %zu is too big. Max is %zu", job_id, (size_t)(WORKER_UTILIZATION_MAX_JOB_TYPES - 1));
        return;
    }
    if (*worker->per_job_type[job_id].name) {
        error("WORKER_UTILIZATION: duplicate job registration: worker '%s' job id %zu is '%s', ignoring '%s'", worker->workname, job_id, worker->per_job_type[job_id].name, name);
        return;
    }

    strncpy(worker->per_job_type[job_id].name, name, WORKER_UTILIZATION_MAX_JOB_NAME_LENGTH);
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

static inline void worker_is_idle_with_time(usec_t now) {
    usec_t delta = now - worker->last_action_timestamp;
    worker->busy_time += delta;
    worker->per_job_type[worker->job_id].worker_busy_time += delta;

    // the worker was busy
    // set it to idle before we set the timestamp

    worker->last_action = WORKER_IDLE;
    if(likely(worker->last_action_timestamp < now))
        worker->last_action_timestamp = now;
}

void worker_is_idle(void) {
    if(unlikely(!worker)) return;
    if(unlikely(worker->last_action != WORKER_BUSY)) return;

    worker_is_idle_with_time(now_realtime_usec());
}

void worker_is_busy(size_t job_id) {
    if(unlikely(!worker)) return;
    if(unlikely(job_id >= WORKER_UTILIZATION_MAX_JOB_TYPES))
        job_id = 0;

    usec_t now = now_realtime_usec();

    if(worker->last_action == WORKER_BUSY)
        worker_is_idle_with_time(now);

    // the worker was idle
    // set the timestamp and then set it to busy

    worker->job_id = job_id;
    worker->per_job_type[job_id].worker_jobs_started++;
    worker->jobs_started++;
    worker->last_action_timestamp = now;
    worker->last_action = WORKER_BUSY;
}


// statistics interface

void workers_foreach(const char *workname, void (*callback)(void *data, pid_t pid, const char *thread_tag, size_t utilization_usec, size_t duration_usec, size_t jobs_started, size_t is_running, const char **job_types_names, size_t *job_types_jobs_started, usec_t *job_types_busy_time), void *data) {
    netdata_mutex_lock(&base_lock);
    uint32_t hash = simple_hash(workname);
    usec_t busy_time, delta;
    size_t i, jobs_started, jobs_running;

    struct worker *p;
    for(p = base; p ; p = p->next) {
        if(hash != p->workname_hash || strcmp(workname, p->workname)) continue;

        usec_t now = now_realtime_usec();

        // find per job type statistics
        const char *per_job_type_name[WORKER_UTILIZATION_MAX_JOB_TYPES];
        size_t per_job_type_jobs_started[WORKER_UTILIZATION_MAX_JOB_TYPES];
        usec_t per_job_type_busy_time[WORKER_UTILIZATION_MAX_JOB_TYPES];
        for(i  = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES ;i++) {
            per_job_type_name[i] = p->per_job_type[i].name;

            size_t tmp_jobs_started = p->per_job_type[i].worker_jobs_started;
            per_job_type_jobs_started[i] = tmp_jobs_started - p->per_job_type[i].statistics_last_jobs_started;
            p->per_job_type[i].statistics_last_jobs_started = tmp_jobs_started;

            usec_t tmp_busy_time = p->per_job_type[i].worker_busy_time;
            per_job_type_busy_time[i] = tmp_busy_time - p->per_job_type[i].statistics_last_busy_time;
            p->per_job_type[i].statistics_last_busy_time = tmp_busy_time;
        }

        // get a copy of the worker variables
        size_t worker_job_id = p->job_id;
        usec_t worker_busy_time = p->busy_time;
        size_t worker_jobs_started = p->jobs_started;
        char worker_last_action = p->last_action;
        usec_t worker_last_action_timestamp = p->last_action_timestamp;

        delta = now - p->statistics_last_checkpoint;
        p->statistics_last_checkpoint = now;

        // this is the only variable both the worker thread and the statistics thread are writing
        // we set this only when the worker is busy, so that the worker will not
        // accumulate all the busy time, but only the time after the point we collected statistics
        if(worker_last_action == WORKER_BUSY && p->last_action_timestamp == worker_last_action_timestamp && p->last_action == WORKER_BUSY)
            p->last_action_timestamp = now;

        // calculate delta busy time
        busy_time = worker_busy_time - p->statistics_last_busy_time;
        p->statistics_last_busy_time = worker_busy_time;

        // calculate delta jobs done
        jobs_started = worker_jobs_started - p->statistics_last_jobs_started;
        p->statistics_last_jobs_started = worker_jobs_started;

        jobs_running = 0;
        if(worker_last_action == WORKER_BUSY) {
            // the worker is still busy with something
            // let's add that busy time to the reported one
            usec_t dt = now - worker_last_action_timestamp;
            busy_time += dt;
            per_job_type_busy_time[worker_job_id] += dt;
            jobs_running = 1;
        }

        callback(data, p->pid, p->tag, busy_time, delta, jobs_started, jobs_running, per_job_type_name, per_job_type_jobs_started, per_job_type_busy_time);
    }

    netdata_mutex_unlock(&base_lock);
}
