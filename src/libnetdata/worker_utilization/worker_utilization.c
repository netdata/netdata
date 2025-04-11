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

struct worker_spinlock {
    const char *function;
    size_t locks;
    size_t spins;

    size_t statistics_last_locks;
    size_t statistics_last_spins;
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

    size_t spinlocks_used;
    struct worker_spinlock spinlocks[WORKER_SPINLOCK_CONTENTION_FUNCTIONS];

    uint64_t memory_calls[WORKERS_MEMORY_CALL_MAX];

    struct worker *next;
    struct worker *prev;
};

struct workers_workname {                           // this is what we add to JudyHS
    SPINLOCK spinlock;
    struct worker *base;
};

ENUM_STR_MAP_DEFINE(WORKERS_MEMORY_CALL) = {
    {WORKERS_MEMORY_CALL_LIBC_MALLOC, "malloc"},
    {WORKERS_MEMORY_CALL_LIBC_CALLOC, "calloc"},
    {WORKERS_MEMORY_CALL_LIBC_REALLOC, "realloc"},
    {WORKERS_MEMORY_CALL_LIBC_FREE, "free"},
    {WORKERS_MEMORY_CALL_LIBC_STRDUP, "strdup"},
    {WORKERS_MEMORY_CALL_LIBC_STRNDUP, "strndup"},
    {WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN, "posix_memalign"},
    {WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN_FREE, "posix_memalign_free"},
    {WORKERS_MEMORY_CALL_MMAP, "mmap"},
    {WORKERS_MEMORY_CALL_MUNMAP, "munmap"},

    // terminator
    {0, NULL},
};

ENUM_STR_DEFINE_FUNCTIONS(WORKERS_MEMORY_CALL, WORKERS_MEMORY_CALL_LIBC_MALLOC, "other");

static struct workers_globals {
    bool enabled;

    SPINLOCK spinlock;
    Pvoid_t worknames_JudyHS;
    size_t memory;
    
#ifdef FSANITIZE_ADDRESS
    // For tracking all registered worker items during ASAN builds
    Pvoid_t workers_JudyL;         // JudyL array of all worker structs
    Pvoid_t worknames_JudyL;       // JudyL array of all workname structs
    Pvoid_t worker_strings_JudyL;  // JudyL array of all STRING objects in workers
#endif

} workers_globals = {                           // workers globals, the base of all worknames
    .enabled = false,
    .spinlock = SPINLOCK_INITIALIZER,           // a lock for the worknames index
    .worknames_JudyHS = NULL,                   // the worknames index
#ifdef FSANITIZE_ADDRESS
    .workers_JudyL = NULL,
    .worknames_JudyL = NULL,
    .worker_strings_JudyL = NULL,
#endif
};

static __thread struct worker *worker = NULL; // the current thread worker
static __thread size_t last_job_id = 0;

size_t workers_get_last_job_id() {
    return last_job_id;
}

static ALWAYS_INLINE usec_t worker_now_monotonic_usec(void) {
#ifdef NETDATA_WITHOUT_WORKERS_LATENCY
    return 0;
#else
    return now_monotonic_usec();
#endif
}

void workers_utilization_enable(void) {
    workers_globals.enabled = true;
}

size_t workers_allocated_memory(void) {
    if(!workers_globals.enabled)
        return 0;

    spinlock_lock(&workers_globals.spinlock);
    size_t memory = workers_globals.memory;
    spinlock_unlock(&workers_globals.spinlock);

    return memory;
}

void worker_register(const char *name) {
    if(likely(worker || !workers_globals.enabled))
        return;

    worker = callocz(1, sizeof(struct worker));
    worker->pid = gettid_cached();
    worker->tag = strdupz(nd_thread_tag());
    worker->workname = strdupz(name);

    usec_t now = worker_now_monotonic_usec();
    worker->statistics_last_checkpoint = now;
    worker->last_action_timestamp = now;
    worker->last_action = WORKER_IDLE;

    size_t name_size = strlen(name) + 1;
    spinlock_lock(&workers_globals.spinlock);

    workers_globals.memory += sizeof(struct worker) + strlen(worker->tag) + 1 + strlen(worker->workname) + 1;

#ifdef FSANITIZE_ADDRESS
    // Track the worker struct in our JudyL array for ASAN builds
    Pvoid_t *WValue = JudyLIns(&workers_globals.workers_JudyL, (Word_t)worker, PJE0);
    if (WValue != PJERR)
        *WValue = (void *)1;
#endif

    JudyAllocThreadPulseReset();
    Pvoid_t *PValue = JudyHSIns(&workers_globals.worknames_JudyHS, (void *)name, name_size, PJE0);
    int64_t judy_mem = JudyAllocThreadPulseGetAndReset();

    struct workers_workname *workname = *PValue;
    if(!workname) {
        workname = mallocz(sizeof(struct workers_workname));
        spinlock_init(&workname->spinlock);
        workname->base = NULL;
        *PValue = workname;

        workers_globals.memory = (int64_t)workers_globals.memory + (int64_t)sizeof(struct workers_workname) + judy_mem;
        
#ifdef FSANITIZE_ADDRESS
        // Track the workname struct in our JudyL array for ASAN builds
        Pvoid_t *WValue = JudyLIns(&workers_globals.worknames_JudyL, (Word_t)workname, PJE0);
        if (WValue != PJERR)
            *WValue = (void *)1;
#endif
    }

    spinlock_lock(&workname->spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(workname->base, worker, prev, next);
    spinlock_unlock(&workname->spinlock);

    spinlock_unlock(&workers_globals.spinlock);
}

void worker_register_job_custom_metric(size_t job_id, const char *name, const char *units, WORKER_METRIC_TYPE type) {
    if(likely(!worker)) return;

    if(unlikely(job_id >= WORKER_UTILIZATION_MAX_JOB_TYPES)) {
        netdata_log_error("WORKER_UTILIZATION: job_id %zu is too big. Max is %zu", job_id, (size_t)(WORKER_UTILIZATION_MAX_JOB_TYPES - 1));
        return;
    }

    if(job_id > worker->worker_max_job_id)
        worker->worker_max_job_id = job_id;

    if(worker->per_job_type[job_id].name) {
        if(strcmp(string2str(worker->per_job_type[job_id].name), name) != 0 || worker->per_job_type[job_id].type != type || strcmp(string2str(worker->per_job_type[job_id].units), units) != 0)
            netdata_log_error("WORKER_UTILIZATION: duplicate job registration: worker '%s' job id %zu is '%s', ignoring the later '%s'", worker->workname, job_id, string2str(worker->per_job_type[job_id].name), name);
        return;
    }

    STRING *name_str = string_strdupz(name);
    STRING *units_str = string_strdupz(units);
    
    worker->per_job_type[job_id].name = name_str;
    worker->per_job_type[job_id].units = units_str;
    worker->per_job_type[job_id].type = type;
    
#ifdef FSANITIZE_ADDRESS
    // Track the strings in our JudyL array for ASAN builds
    spinlock_lock(&workers_globals.spinlock);
    
    // Track the name STRING with reference counting
    if (name_str) {
        Pvoid_t *PValue = JudyLIns(&workers_globals.worker_strings_JudyL, (Word_t)name_str, PJE0);
        if (PValue != PJERR) {
            // Increment the reference count (or initialize to 1 if new)
            size_t count = (size_t)(uintptr_t)*PValue;
            count++;
            *PValue = (void *)(uintptr_t)count;
        }
    }
    
    // Track the units STRING with reference counting
    if (units_str) {
        Pvoid_t *PValue = JudyLIns(&workers_globals.worker_strings_JudyL, (Word_t)units_str, PJE0);
        if (PValue != PJERR) {
            // Increment the reference count (or initialize to 1 if new)
            size_t count = (size_t)(uintptr_t)*PValue;
            count++;
            *PValue = (void *)(uintptr_t)count;
        }
    }
    
    spinlock_unlock(&workers_globals.spinlock);
#endif
}

void worker_register_job_name(size_t job_id, const char *name) {
    worker_register_job_custom_metric(job_id, name, "", WORKER_METRIC_IDLE_BUSY);
}

void worker_unregister(void) {
    if(likely(!worker)) return;

    size_t workname_size = strlen(worker->workname) + 1;
    spinlock_lock(&workers_globals.spinlock);
    Pvoid_t *PValue = JudyHSGet(workers_globals.worknames_JudyHS, (void *)worker->workname, workname_size);
    if(PValue) {
        struct workers_workname *workname = *PValue;
        spinlock_lock(&workname->spinlock);
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(workname->base, worker, prev, next);
        spinlock_unlock(&workname->spinlock);

        if(!workname->base) {
            JudyAllocThreadPulseReset();
            JudyHSDel(&workers_globals.worknames_JudyHS, (void *) worker->workname, workname_size, PJE0);
            int64_t judy_mem = JudyAllocThreadPulseGetAndReset();
            freez(workname);
            workers_globals.memory = (int64_t)workers_globals.memory - (int64_t)sizeof(struct workers_workname) + judy_mem;
        }
    }
    workers_globals.memory -= sizeof(struct worker) + strlen(worker->tag) + 1 + strlen(worker->workname) + 1;
    
#ifdef FSANITIZE_ADDRESS
    // Remove this worker from the tracking array
    JudyLDel(&workers_globals.workers_JudyL, (Word_t)worker, PJE0);
#endif
    
    spinlock_unlock(&workers_globals.spinlock);

    // Free all thread-local resources associated with this worker
    for(int i = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES; i++) {
#ifdef FSANITIZE_ADDRESS
        // Decrement reference count in tracking array before freeing
        if (worker->per_job_type[i].name) {
            spinlock_lock(&workers_globals.spinlock);
            Pvoid_t *PValue = JudyLGet(workers_globals.worker_strings_JudyL, (Word_t)worker->per_job_type[i].name, PJE0);
            if (PValue) {
                size_t count = (size_t)(uintptr_t)*PValue;
                if (count > 1) {
                    // Decrement reference count
                    count--;
                    *PValue = (void *)(uintptr_t)count;
                } else {
                    // Last reference, remove from tracking
                    JudyLDel(&workers_globals.worker_strings_JudyL, (Word_t)worker->per_job_type[i].name, PJE0);
                }
            }
            spinlock_unlock(&workers_globals.spinlock);
        }
        if (worker->per_job_type[i].units) {
            spinlock_lock(&workers_globals.spinlock);
            Pvoid_t *PValue = JudyLGet(workers_globals.worker_strings_JudyL, (Word_t)worker->per_job_type[i].units, PJE0);
            if (PValue) {
                size_t count = (size_t)(uintptr_t)*PValue;
                if (count > 1) {
                    // Decrement reference count
                    count--;
                    *PValue = (void *)(uintptr_t)count;
                } else {
                    // Last reference, remove from tracking
                    JudyLDel(&workers_globals.worker_strings_JudyL, (Word_t)worker->per_job_type[i].units, PJE0);
                }
            }
            spinlock_unlock(&workers_globals.spinlock);
        }
#endif
        // Then free the strings
        string_freez(worker->per_job_type[i].name);
        string_freez(worker->per_job_type[i].units);
    }

    freez((void *)worker->tag);
    freez((void *)worker->workname);
    freez(worker);

    worker = NULL;
}

// Cleanup all worker utilization resources
void worker_utilization_cleanup(void) {
    if(!workers_globals.enabled)
        return;

    // Clean up the current thread's worker if it exists
    worker_unregister();
    
    spinlock_lock(&workers_globals.spinlock);
    
#ifdef FSANITIZE_ADDRESS
    // Free any remaining strings in the tracking array according to their reference counts
    if (workers_globals.worker_strings_JudyL) {
        Word_t string_ptr = 0;
        Pvoid_t *PValue = JudyLFirst(workers_globals.worker_strings_JudyL, &string_ptr, PJE0);
        size_t total_strings = 0;
        size_t total_refs = 0;
        
        // First pass: count how many strings and references
        while (PValue) {
            total_strings++;
            size_t refs = (size_t)(uintptr_t)*PValue;
            total_refs += refs;
            PValue = JudyLNext(workers_globals.worker_strings_JudyL, &string_ptr, PJE0);
        }
        
        // If any strings remain, emit info about them
        if (total_strings > 0) {
            fprintf(stderr, "INFO: Freeing %zu STRING objects with %zu total references\n", 
                    total_strings, total_refs);
            
            // Second pass: free each string the correct number of times
            string_ptr = 0;
            PValue = JudyLFirst(workers_globals.worker_strings_JudyL, &string_ptr, PJE0);
            
            while (PValue) {
                STRING *str = (STRING *)string_ptr;
                size_t refs = (size_t)(uintptr_t)*PValue;
                
                // Get the next one before we potentially delete this entry
                PValue = JudyLNext(workers_globals.worker_strings_JudyL, &string_ptr, PJE0);
                
                // Free the string exactly the number of times it was referenced
                for (size_t i = 0; i < refs; i++) {
                    string_freez(str);
                }
            }
        }
        
        // Free the array itself
        JudyLFreeArray(&workers_globals.worker_strings_JudyL, PJE0);
        workers_globals.worker_strings_JudyL = NULL;
    }
    
    // We don't need to free worker or workname structs here as they should
    // be freed by worker_unregister, but we should free the tracking arrays
    if (workers_globals.workers_JudyL) {
        JudyLFreeArray(&workers_globals.workers_JudyL, PJE0);
        workers_globals.workers_JudyL = NULL;
    }
    
    if (workers_globals.worknames_JudyL) {
        JudyLFreeArray(&workers_globals.worknames_JudyL, PJE0);
        workers_globals.worknames_JudyL = NULL;
    }
#endif
    
    // Free the JudyHS array that contains workname structs
    if (workers_globals.worknames_JudyHS) {
#ifdef FSANITIZE_ADDRESS
        // For ASAN builds, we need to free any remaining workname structures
        // First, collect all workname pointers
        Pvoid_t worknames_to_free = NULL;
        
        // Cannot iterate JudyHS directly, but we can use worknames_JudyL which tracks all worknames
        if (workers_globals.worknames_JudyL) {
            Word_t workname_ptr = 0;
            Pvoid_t *PValue = JudyLFirst(workers_globals.worknames_JudyL, &workname_ptr, PJE0);
            size_t count = 0;
            
            while (PValue) {
                // Store this workname to free it later
                Pvoid_t *StoreValue = JudyLIns(&worknames_to_free, workname_ptr, PJE0);
                if (StoreValue != PJERR)
                    *StoreValue = (void *)1;
                
                count++;
                PValue = JudyLNext(workers_globals.worknames_JudyL, &workname_ptr, PJE0);
            }
            
            if (count > 0) {
                fprintf(stderr, "INFO: Freeing %zu workers_workname structures from tracking array\n", count);
            }
        }
        
        // Also try to get the LIBUV workname directly since it's the one leaking
        // This is a direct approach to ensure we don't miss any worknames
        const char *libuv_name = "LIBUV";
        size_t libuv_name_size = strlen(libuv_name) + 1;
        Pvoid_t *LiuvValue = JudyHSGet(workers_globals.worknames_JudyHS, (void *)libuv_name, libuv_name_size);
        if (LiuvValue && *LiuvValue) {
            struct workers_workname *libuv_workname = *LiuvValue;
            // Check if we've already got this workname in our tracking array
            bool already_tracked = false;
            if (worknames_to_free) {
                Pvoid_t *ExistingValue = JudyLGet(worknames_to_free, (Word_t)libuv_workname, PJE0);
                already_tracked = (ExistingValue != NULL);
            }
            
            if (!already_tracked) {
                // Add this workname to our list
                Pvoid_t *StoreValue = JudyLIns(&worknames_to_free, (Word_t)libuv_workname, PJE0);
                if (StoreValue != PJERR) {
                    *StoreValue = (void *)1;
                    fprintf(stderr, "INFO: Found LIBUV workname not in tracking array\n");
                }
            }
        }
#endif
        
        // Free the JudyHS array
        JudyHSFreeArray(&workers_globals.worknames_JudyHS, PJE0);
        workers_globals.worknames_JudyHS = NULL;
        
#ifdef FSANITIZE_ADDRESS
        // Now free all the workname structures we collected
        if (worknames_to_free) {
            Word_t workname_ptr = 0;
            Pvoid_t *PValue = JudyLFirst(worknames_to_free, &workname_ptr, PJE0);
            
            while (PValue) {
                struct workers_workname *workname = (struct workers_workname *)workname_ptr;
                freez(workname);
                
                PValue = JudyLNext(worknames_to_free, &workname_ptr, PJE0);
            }
            
            JudyLFreeArray(&worknames_to_free, PJE0);
        }
#endif
    }
    
    // Reset memory count
    workers_globals.memory = 0;
    
    spinlock_unlock(&workers_globals.spinlock);
}

static void worker_is_idle_with_time(usec_t now) {
    usec_t delta = now - worker->last_action_timestamp;
    worker->busy_time += delta;
    worker->per_job_type[worker->job_id].worker_busy_time += delta;

    // the worker was busy
    // set it to idle before we set the timestamp

    worker->last_action = WORKER_IDLE;
    if(likely(worker->last_action_timestamp < now))
        worker->last_action_timestamp = now;
}

ALWAYS_INLINE void worker_is_idle(void) {
    if(likely(!worker || worker->last_action != WORKER_BUSY)) return;

    last_job_id = WORKER_UTILIZATION_MAX_JOB_TYPES;
    worker_is_idle_with_time(worker_now_monotonic_usec());
}

static void worker_is_busy_do(size_t job_id) {
    usec_t now = worker_now_monotonic_usec();

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

ALWAYS_INLINE void worker_is_busy(size_t job_id) {
    last_job_id = job_id;

    if(likely(!worker || job_id >= WORKER_UTILIZATION_MAX_JOB_TYPES))
        return;

    worker_is_busy_do(job_id);
}

static void worker_set_metric_do(size_t job_id, NETDATA_DOUBLE value) {
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

ALWAYS_INLINE void worker_set_metric(size_t job_id, NETDATA_DOUBLE value) {
    if(likely(!worker || job_id >= WORKER_UTILIZATION_MAX_JOB_TYPES))
        return;

    worker_set_metric_do(job_id, value);
}

// --------------------------------------------------------------------------------------------------------------------

static ALWAYS_INLINE size_t pointer_hash_function(const char *func) {
    uintptr_t addr = (uintptr_t)func;
    return (size_t)(((addr >> 4) | (addr >> 16)) + func[0]) % WORKER_SPINLOCK_CONTENTION_FUNCTIONS;
}

static void worker_spinlock_contention_do(const char *func, size_t spins) {
    size_t hash = pointer_hash_function(func);
    for (size_t i = 0; i < WORKER_SPINLOCK_CONTENTION_FUNCTIONS; i++) {
        size_t slot = (hash + i) % WORKER_SPINLOCK_CONTENTION_FUNCTIONS;
        if (worker->spinlocks[slot].function == func || worker->spinlocks[slot].function == NULL) {
            // Either an empty slot or a matching slot

            worker->spinlocks[slot].function = func;
            worker->spinlocks[slot].locks++;
            worker->spinlocks[slot].spins += spins;

            return;
        }
    }

    // Array is full - do nothing
}

ALWAYS_INLINE void worker_spinlock_contention(const char *func, size_t spins) {
    if(likely(!worker))
        return;

    worker_spinlock_contention_do(func, spins);
}

ALWAYS_INLINE void workers_memory_call(WORKERS_MEMORY_CALL call) {
    if(likely(!worker || call >= WORKERS_MEMORY_CALL_MAX))
        return;

    worker->memory_calls[call]++;
}

// statistics interface

void workers_foreach(const char *name, void (*callback)(
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
                                           , const char *spinlock_functions[]
                                           , size_t *spinlock_locks
                                           , size_t *spinlock_spins
                                           , uint64_t *memory_calls
                                           )
                                           , void *data) {
    if(!workers_globals.enabled)
        return;

    spinlock_lock(&workers_globals.spinlock);
    usec_t busy_time, delta;
    size_t jobs_started, jobs_running;

    size_t workname_size = strlen(name) + 1;
    struct workers_workname *workname;
    Pvoid_t *PValue = JudyHSGet(workers_globals.worknames_JudyHS, (void *)name, workname_size);
    if(PValue) {
        workname = *PValue;
        spinlock_lock(&workname->spinlock);
    }
    else
        workname = NULL;

    spinlock_unlock(&workers_globals.spinlock);

    if(!workname)
        return;

    struct worker *p;
    DOUBLE_LINKED_LIST_FOREACH_FORWARD(workname->base, p, prev, next) {
        usec_t now = worker_now_monotonic_usec();

        // find per job type statistics
        STRING *per_job_type_name[WORKER_UTILIZATION_MAX_JOB_TYPES];
        STRING *per_job_type_units[WORKER_UTILIZATION_MAX_JOB_TYPES];
        WORKER_METRIC_TYPE per_job_metric_type[WORKER_UTILIZATION_MAX_JOB_TYPES];
        size_t per_job_type_jobs_started[WORKER_UTILIZATION_MAX_JOB_TYPES];
        usec_t per_job_type_busy_time[WORKER_UTILIZATION_MAX_JOB_TYPES];
        NETDATA_DOUBLE per_job_custom_values[WORKER_UTILIZATION_MAX_JOB_TYPES];

        const char *spinlock_functions[WORKER_SPINLOCK_CONTENTION_FUNCTIONS];
        size_t spinlock_locks[WORKER_SPINLOCK_CONTENTION_FUNCTIONS];
        size_t spinlock_spins[WORKER_SPINLOCK_CONTENTION_FUNCTIONS];

        uint64_t memory_calls[WORKERS_MEMORY_CALL_MAX];

        size_t max_job_id = p->worker_max_job_id;
        for(size_t i  = 0; i <= max_job_id ;i++) {
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

        // ------------------------------------------------------------------------------------------------------------
        // spinlock contention

        size_t t = 0;
        for(size_t i = 0; i < WORKER_SPINLOCK_CONTENTION_FUNCTIONS ;i++) {
            if(!p->spinlocks[i].function) continue;

            spinlock_functions[t] = p->spinlocks[i].function;

            size_t tmp = p->spinlocks[i].locks;
            spinlock_locks[t] = tmp - p->spinlocks[i].statistics_last_locks;
            p->spinlocks[i].statistics_last_locks = tmp;

            tmp = p->spinlocks[i].spins;
            spinlock_spins[t] = tmp - p->spinlocks[i].statistics_last_spins;
            p->spinlocks[i].statistics_last_spins = tmp;

            t++;
        }

        for(; t < WORKER_SPINLOCK_CONTENTION_FUNCTIONS ;t++) {
            spinlock_functions[t] = NULL;
            spinlock_locks[t] = 0;
            spinlock_spins[t] = 0;
        }

        // ------------------------------------------------------------------------------------------------------------

        memcpy(memory_calls, p->memory_calls, sizeof(memory_calls));

        // ------------------------------------------------------------------------------------------------------------

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
                 , spinlock_functions
                 , spinlock_locks
                 , spinlock_spins
                 , memory_calls
                 );
    }

    spinlock_unlock(&workname->spinlock);
}
