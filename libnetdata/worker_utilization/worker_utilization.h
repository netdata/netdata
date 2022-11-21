#ifndef WORKER_UTILIZATION_H
#define WORKER_UTILIZATION_H 1

#include "../libnetdata.h"

// workers interfaces

#define WORKER_UTILIZATION_MAX_JOB_TYPES 50

typedef enum {
    WORKER_METRIC_EMPTY = 0,
    WORKER_METRIC_IDLE_BUSY = 1,
    WORKER_METRIC_ABSOLUTE = 2,
    WORKER_METRIC_INCREMENT = 3,
    WORKER_METRIC_INCREMENTAL_TOTAL = 4,
} WORKER_METRIC_TYPE;

void worker_register(const char *workname);
void worker_register_job_name(size_t job_id, const char *name);
void worker_register_job_custom_metric(size_t job_id, const char *name, const char *units, WORKER_METRIC_TYPE type);
void worker_unregister(void);

void worker_is_idle(void);
void worker_is_busy(size_t job_id);
void worker_set_metric(size_t job_id, NETDATA_DOUBLE value);

// statistics interface

void workers_foreach(const char *workname, void (*callback)(
                                                      void *data
                                                      , pid_t pid
                                                      , const char *thread_tag
                                                      , size_t utilization_usec
                                                      , size_t duration_usec
                                                      , size_t jobs_started
                                                      , size_t is_running
                                                      , STRING **job_types_names
                                                      , STRING **job_types_units
                                                      , WORKER_METRIC_TYPE *job_metric_types
                                                      , size_t *job_types_jobs_started
                                                      , usec_t *job_types_busy_time
                                                      , NETDATA_DOUBLE *job_custom_values
                                                      )
                                                      , void *data);

#endif // WORKER_UTILIZATION_H
