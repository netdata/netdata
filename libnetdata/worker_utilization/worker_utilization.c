#include "worker_utilization.h"

#define WORKER_IDLE 'I'
#define WORKER_BUSY 'B'

struct worker_job_type {
    STRING *name;
    STRING *units;

    // statistics controlled variables
    size_t statistics_last_jobs_started;
    usec_t statistics_last_busy_time;
    NETDATA_DOUBLE statistics_last_custom_value;

    // worker controlled variables
    volatile size_t worker_jobs_started;
    volatile usec_t worker_busy_time;

    WORKER_METRIC_TYPE type;
    NETDATA_DOUBLE custom_value;
};

struct worker {
    pid_t pid;
    const char *tag;
    const char *workname;

    // statistics controlled variables
    volatile usec_t statistics_last_checkpoint;
    size_t statistics_last_jobs_started;
    usec_t statistics_last_busy_time;

    // the worker controlled variables
    size_t worker_max_job_id;
    volatile size_t job_id;
    volatile size_t jobs_started;
    volatile usec_t busy_time;
    volatile usec_t last_action_timestamp;
    volatile char last_action;

    struct worker_job_type per_job_type[WORKER_UTILIZATION_MAX_JOB_TYPES];

    struct worker *next;
    struct worker *prev;
};

static netdata_mutex_t workers_base_lock = NETDATA_MUTEX_INITIALIZER;
static __thread struct worker *worker = NULL;
static Pvoid_t workers_per_workname_JudyHS_array = NULL;

void worker_register(const char *workname) {
    if(unlikely(worker)) return;

    worker = callocz(1, sizeof(struct worker));
    worker->pid = gettid();
    worker->tag = strdupz(netdata_thread_tag());
    worker->workname = strdupz(workname);

    usec_t now = now_monotonic_usec();
    worker->statistics_last_checkpoint = now;
    worker->last_action_timestamp = now;
    worker->last_action = WORKER_IDLE;

    size_t workname_size = strlen(workname) + 1;
    netdata_mutex_lock(&workers_base_lock);

    Pvoid_t *PValue = JudyHSGet(workers_per_workname_JudyHS_array, (void *)workname, workname_size);
    if(!PValue)
        PValue = JudyHSIns(&workers_per_workname_JudyHS_array, (void *)workname, workname_size, PJE0);

    struct worker *base = *PValue;
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(base, worker, prev, next);
    *PValue = base;

    netdata_mutex_unlock(&workers_base_lock);
}

void worker_register_job_custom_metric(size_t job_id, const char *name, const char *units, WORKER_METRIC_TYPE type) {
    if(unlikely(!worker)) return;

    if(unlikely(job_id >= WORKER_UTILIZATION_MAX_JOB_TYPES)) {
        error("WORKER_UTILIZATION: job_id %zu is too big. Max is %zu", job_id, (size_t)(WORKER_UTILIZATION_MAX_JOB_TYPES - 1));
        return;
    }

    if(job_id > worker->worker_max_job_id)
        worker->worker_max_job_id = job_id;

    if(worker->per_job_type[job_id].name) {
        if(strcmp(string2str(worker->per_job_type[job_id].name), name) != 0 || worker->per_job_type[job_id].type != type || strcmp(string2str(worker->per_job_type[job_id].units), units) != 0)
            error("WORKER_UTILIZATION: duplicate job registration: worker '%s' job id %zu is '%s', ignoring the later '%s'", worker->workname, job_id, string2str(worker->per_job_type[job_id].name), name);
        return;
    }

    worker->per_job_type[job_id].name = string_strdupz(name);
    worker->per_job_type[job_id].units = string_strdupz(units);
    worker->per_job_type[job_id].type = type;
}

void worker_register_job_name(size_t job_id, const char *name) {
    worker_register_job_custom_metric(job_id, name, "", WORKER_METRIC_IDLE_BUSY);
}

void worker_unregister(void) {
    if(unlikely(!worker)) return;

    size_t workname_size = strlen(worker->workname) + 1;
    netdata_mutex_lock(&workers_base_lock);
    Pvoid_t *PValue = JudyHSGet(workers_per_workname_JudyHS_array, (void *)worker->workname, workname_size);
    if(PValue) {
        struct worker *base = *PValue;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(base, worker, prev, next);
        *PValue = base;

        if(!base)
            JudyHSDel(&workers_per_workname_JudyHS_array, (void *)worker->workname, workname_size, PJE0);
    }
    netdata_mutex_unlock(&workers_base_lock);

    for(int i  = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES ;i++) {
        string_freez(worker->per_job_type[i].name);
        string_freez(worker->per_job_type[i].units);
    }

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
    if(unlikely(!worker || worker->last_action != WORKER_BUSY)) return;

    worker_is_idle_with_time(now_monotonic_usec());
}

void worker_is_busy(size_t job_id) {
    if(unlikely(!worker || job_id >= WORKER_UTILIZATION_MAX_JOB_TYPES))
        return;

    usec_t now = now_monotonic_usec();

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

void worker_set_metric(size_t job_id, NETDATA_DOUBLE value) {
    if(unlikely(!worker)) return;
    if(unlikely(job_id >= WORKER_UTILIZATION_MAX_JOB_TYPES))
        return;

    switch(worker->per_job_type[job_id].type) {
        case WORKER_METRIC_INCREMENT:
            worker->per_job_type[job_id].custom_value += value;
            break;

        case WORKER_METRIC_INCREMENTAL_TOTAL:
        case WORKER_METRIC_ABSOLUTE:
        default:
            worker->per_job_type[job_id].custom_value = value;
            break;
    }
}

// statistics interface

void workers_foreach(const char *workname, void (*callback)(
                                               void *data
                                               , pid_t pid
                                               , const char *thread_tag
                                               , size_t max_job_id
                                               , size_t utilization_usec
                                               , size_t duration_usec
                                               , size_t jobs_started, size_t is_running
                                               , STRING **job_types_names
                                               , STRING **job_types_units
                                               , WORKER_METRIC_TYPE *job_metric_types
                                               , size_t *job_types_jobs_started
                                               , usec_t *job_types_busy_time
                                               , NETDATA_DOUBLE *job_custom_values
                                               )
                                               , void *data) {
    netdata_mutex_lock(&workers_base_lock);
    usec_t busy_time, delta;
    size_t i, jobs_started, jobs_running;

    size_t workname_size = strlen(workname) + 1;
    struct worker *base = NULL;
    Pvoid_t *PValue = JudyHSGet(workers_per_workname_JudyHS_array, (void *)workname, workname_size);
    if(PValue)
        base = *PValue;

    struct worker *p;
    DOUBLE_LINKED_LIST_FOREACH_FORWARD(base, p, prev, next) {
        usec_t now = now_monotonic_usec();

        // find per job type statistics
        STRING *per_job_type_name[WORKER_UTILIZATION_MAX_JOB_TYPES];
        STRING *per_job_type_units[WORKER_UTILIZATION_MAX_JOB_TYPES];
        WORKER_METRIC_TYPE per_job_metric_type[WORKER_UTILIZATION_MAX_JOB_TYPES];
        size_t per_job_type_jobs_started[WORKER_UTILIZATION_MAX_JOB_TYPES];
        usec_t per_job_type_busy_time[WORKER_UTILIZATION_MAX_JOB_TYPES];
        NETDATA_DOUBLE per_job_custom_values[WORKER_UTILIZATION_MAX_JOB_TYPES];

        size_t max_job_id = p->worker_max_job_id;
        for(i  = 0; i <= max_job_id ;i++) {
            per_job_type_name[i] = p->per_job_type[i].name;
            per_job_type_units[i] = p->per_job_type[i].units;
            per_job_metric_type[i] = p->per_job_type[i].type;

            switch(p->per_job_type[i].type) {
                default:
                case WORKER_METRIC_EMPTY: {
                    per_job_type_jobs_started[i] = 0;
                    per_job_type_busy_time[i] = 0;
                    per_job_custom_values[i] = NAN;
                    break;
                }

                case WORKER_METRIC_IDLE_BUSY: {
                    size_t tmp_jobs_started = p->per_job_type[i].worker_jobs_started;
                    per_job_type_jobs_started[i] = tmp_jobs_started - p->per_job_type[i].statistics_last_jobs_started;
                    p->per_job_type[i].statistics_last_jobs_started = tmp_jobs_started;

                    usec_t tmp_busy_time = p->per_job_type[i].worker_busy_time;
                    per_job_type_busy_time[i] = tmp_busy_time - p->per_job_type[i].statistics_last_busy_time;
                    p->per_job_type[i].statistics_last_busy_time = tmp_busy_time;

                    per_job_custom_values[i] = NAN;
                    break;
                }

                case WORKER_METRIC_ABSOLUTE: {
                    per_job_type_jobs_started[i] = 0;
                    per_job_type_busy_time[i] = 0;

                    per_job_custom_values[i] = p->per_job_type[i].custom_value;
                    break;
                }

                case WORKER_METRIC_INCREMENTAL_TOTAL:
                case WORKER_METRIC_INCREMENT: {
                    per_job_type_jobs_started[i] = 0;
                    per_job_type_busy_time[i] = 0;

                    NETDATA_DOUBLE tmp_custom_value = p->per_job_type[i].custom_value;
                    per_job_custom_values[i] = tmp_custom_value - p->per_job_type[i].statistics_last_custom_value;
                    p->per_job_type[i].statistics_last_custom_value = tmp_custom_value;

                    break;
                }
            }
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

        callback(data
                 , p->pid
                 , p->tag
                 , max_job_id
                 , busy_time
                 , delta
                 , jobs_started
                 , jobs_running
                 , per_job_type_name
                 , per_job_type_units
                 , per_job_metric_type
                 , per_job_type_jobs_started
                 , per_job_type_busy_time
                 , per_job_custom_values
                 );
    }

    netdata_mutex_unlock(&workers_base_lock);
}
