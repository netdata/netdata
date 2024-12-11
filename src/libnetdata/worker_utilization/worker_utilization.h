#ifndef WORKER_UTILIZATION_H
#define WORKER_UTILIZATION_H 1

#include "../libnetdata.h"

// workers interfaces

#define WORKER_UTILIZATION_MAX_JOB_TYPES 80
#define WORKER_SPINLOCK_CONTENTION_FUNCTIONS 200

typedef enum __attribute__((packed)) {
    WORKER_METRIC_EMPTY = 0,
    WORKER_METRIC_IDLE_BUSY = 1,
    WORKER_METRIC_ABSOLUTE = 2,
    WORKER_METRIC_INCREMENT = 3,
    WORKER_METRIC_INCREMENTAL_TOTAL = 4,
} WORKER_METRIC_TYPE;

void workers_utilization_enable(void);
size_t workers_allocated_memory(void);
void worker_register(const char *name);
void worker_register_job_name(size_t job_id, const char *name);
void worker_register_job_custom_metric(size_t job_id, const char *name, const char *units, WORKER_METRIC_TYPE type);
void worker_unregister(void);

void worker_is_idle(void);
void worker_is_busy(size_t job_id);
void worker_set_metric(size_t job_id, NETDATA_DOUBLE value);
void worker_spinlock_contention(const char *func, size_t spins);

// statistics interface

void workers_foreach(const char *name, void (*callback)(
                                           void *data
                                           , pid_t pid
                                           , const char *thread_tag
                                           , size_t max_job_id
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
                                           , const char *spinlock_functions[]
                                           , size_t *spinlock_locks
                                           , size_t *spinlock_spins
                                           )
                                           , void *data);

#endif // WORKER_UTILIZATION_H
