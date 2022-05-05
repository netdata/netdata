#ifndef WORKER_UTILIZATION_H
#define WORKER_UTILIZATION_H 1

#include "../libnetdata.h"

// workers interfaces

#define WORKER_UTILIZATION_MAX_JOB_TYPES 20
#define WORKER_UTILIZATION_MAX_JOB_NAME_LENGTH 22

extern void worker_register(const char *workname);
extern void worker_register_job_name(size_t job_id, const char *name);
extern void worker_unregister(void);

extern void worker_is_idle(void);
extern void worker_is_busy(size_t job_id);

// statistics interface

extern void workers_foreach(const char *workname, void (*callback)(void *data, pid_t pid, const char *thread_tag, size_t utilization_usec, size_t duration_usec, size_t jobs_started, size_t is_running, const char **job_types_names, size_t *job_types_jobs_started, usec_t *job_types_busy_time), void *data);

#endif // WORKER_UTILIZATION_H
