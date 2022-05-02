#ifndef WORKER_UTILIZATION_H
#define WORKER_UTILIZATION_H 1

#include "../libnetdata.h"

// workers interfaces

extern void worker_register(const char *workname);
extern void worker_unregister(void);

extern void worker_is_idle(void);
extern void worker_is_busy(void);

// statistics interface

extern void workers_foreach(const char *workname, void (*callback)(pid_t pid, const char *thread_tag, size_t utilization_usec, size_t duration_usec, size_t jobs));

#endif // WORKER_UTILIZATION_H
