#include "common.h"
#include <inttypes.h>

#include <sys/sem.h>
#include <sys/msg.h>
#include <sys/shm.h>

#ifndef SEMVMX
#define SEMVMX  32767  /* <= 32767 semaphore maximum value */
#endif

/* Some versions of libc only define IPC_INFO when __USE_GNU is defined. */
#ifndef IPC_INFO
#define IPC_INFO   3
#endif

struct ipc_limits {
    uint64_t        shmmni;     /* max number of segments */
    uint64_t        shmmax;     /* max segment size */
    uint64_t        shmall;     /* max total shared memory */
    uint64_t        shmmin;     /* min segment size */

    int             semmni;     /* max number of arrays */
    int             semmsl;     /* max semaphores per array */
    int             semmns;     /* max semaphores system wide */
    int             semopm;     /* max ops per semop call */
    unsigned int    semvmx;     /* semaphore max value (constant) */

    int             msgmni;     /* max queues system wide */
    size_t          msgmax;     /* max size of message */
    int             msgmnb;     /* default max size of queue */
};

struct ipc_status {
    int             semusz;     /* current number of arrays */
    int             semaem;     /* current semaphores system wide */
};

/*
 *  The last arg of semctl is a union semun, but where is it defined? X/OPEN
 *  tells us to define it ourselves, but until recently Linux include files
 *  would also define it.
 */
#ifndef HAVE_UNION_SEMUN
/* according to X/OPEN we have to define it ourselves */
union semun {
    int val;
    struct semid_ds *buf;
    unsigned short int *array;
    struct seminfo *__buf;
};
#endif

static inline int ipc_sem_get_limits(struct ipc_limits *lim) {
    static procfile *ff = NULL;
    static int error_shown = 0;
    static char filename[FILENAME_MAX + 1] = "";

    if(unlikely(!filename[0]))
        snprintfz(filename, FILENAME_MAX, "%s/proc/sys/kernel/sem", global_host_prefix);

    if(unlikely(!ff)) {
        ff = procfile_open(filename, NULL, PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) {
            if(unlikely(!error_shown)) {
                error("IPC: Cannot open file '%s'.", filename);
                error_shown = 1;
            }
            goto ipc;
        }
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) {
        if(unlikely(!error_shown)) {
            error("IPC: Cannot read file '%s'.", filename);
            error_shown = 1;
        }
        goto ipc;
    }

    if(procfile_lines(ff) >= 1 && procfile_linewords(ff, 0) >= 4) {
        lim->semvmx = SEMVMX;
        lim->semmsl = atoi(procfile_lineword(ff, 0, 0));
        lim->semmns = atoi(procfile_lineword(ff, 0, 1));
        lim->semopm = atoi(procfile_lineword(ff, 0, 2));
        lim->semmni = atoi(procfile_lineword(ff, 0, 3));
        return 0;
    }
    else {
        if(unlikely(!error_shown)) {
            error("IPC: Invalid content in file '%s'.", filename);
            error_shown = 1;
        }
        goto ipc;
    }

ipc:
    // cannot do it from the file
    // query IPC
    {
        struct seminfo seminfo = {.semmni = 0};
        union semun arg = {.array = (ushort *) &seminfo};

        if(unlikely(semctl(0, 0, IPC_INFO, arg) < 0)) {
            error("IPC: Failed to read '%s' and request IPC_INFO with semctl().", filename);
            goto error;
        }

        lim->semvmx = SEMVMX;
        lim->semmni = seminfo.semmni;
        lim->semmsl = seminfo.semmsl;
        lim->semmns = seminfo.semmns;
        lim->semopm = seminfo.semopm;
        return 0;
    }

error:
    lim->semvmx = 0;
    lim->semmni = 0;
    lim->semmsl = 0;
    lim->semmns = 0;
    lim->semopm = 0;
    return -1;
}

static inline int ipc_sem_get_status(struct ipc_status *st) {
    static int error_shown = 0;
    struct seminfo seminfo;
    union semun arg;

    arg.array = (ushort *)  (void *) &seminfo;

    if(unlikely(semctl (0, 0, SEM_INFO, arg) < 0)) {
        /* kernel not configured for semaphores */
        if(unlikely(!error_shown)) {
            error("IPC: kernel is not configured for semaphores");
            error_shown = 1;
        }
        st->semusz = 0;
        st->semaem = 0;
        return -1;
    }

    st->semusz = seminfo.semusz;
    st->semaem = seminfo.semaem;
    return 0;
}

void *ipc_main(void *ptr) {
    (void)ptr;

    info("IPC thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    int vdo_cpu_netdata = config_get_boolean("plugin:ipc", "ipc plugin resources", 1);
    struct rusage thread;

    struct ipc_limits limits;
    struct ipc_status status;

    // make sure it works
    if(ipc_sem_get_limits(&limits) == -1) {
        error("unable to fetch semaphore limits");
        goto cleanup;
    }

    // make sure it works
    if(ipc_sem_get_status(&status) == -1) {
        error("unable to fetch semaphore statistics");
        goto cleanup;
    }

    RRDVAR *arrays_max     = rrdvar_custom_host_variable_create(&localhost, "ipc.semaphores.arrays.max");
    RRDVAR *semaphores_max = rrdvar_custom_host_variable_create(&localhost, "ipc.semaphores.max");

    rrdvar_custom_host_variable_set(arrays_max, limits.semmni);
    rrdvar_custom_host_variable_set(semaphores_max, limits.semmns);

    // create the charts
    RRDSET *semaphores = rrdset_find("system.ipc_semaphores");
    if(!semaphores) {
        semaphores = rrdset_create("system", "ipc_semaphores", NULL, "ipc semaphores", NULL, "IPC Semaphores", "semaphores", 1000, rrd_update_every, RRDSET_TYPE_AREA);
        rrddim_add(semaphores, "semaphores", NULL, 1, 1, RRDDIM_ABSOLUTE);
    }
    RRDSET *arrays = rrdset_find("system.ipc_semaphore_arrays");
    if(!arrays) {
        arrays = rrdset_create("system", "ipc_semaphore_arrays", NULL, "ipc semaphores", NULL, "IPC Semaphore Arrays", "arrays", 1000, rrd_update_every, RRDSET_TYPE_AREA);
        rrddim_add(arrays, "arrays", NULL, 1, 1, RRDDIM_ABSOLUTE);
    }
    RRDSET *stcpu_thread = rrdset_create("netdata", "plugin_ipc_cpu", NULL, "proc.internal", NULL, "NetData IPC Plugin CPU usage", "milliseconds/s", 132000, rrd_update_every, RRDSET_TYPE_STACKED);
    rrddim_add(stcpu_thread, "user",  NULL,  1, 1000, RRDDIM_INCREMENTAL);
    rrddim_add(stcpu_thread, "system", NULL, 1, 1000, RRDDIM_INCREMENTAL);

    unsigned long long now, next, step = rrd_update_every * 1000000ULL, read_limits_next = 0;
    while(1) {
        now = time_usec();
        next = now - (now % step) + step;
        sleep_usec(next - now);
        now = next;

        if(unlikely(read_limits_next < now)) {
            if(unlikely(ipc_sem_get_limits(&limits) == -1)) {
                error("Unable to fetch semaphore limits.");
            }
            else {
                rrdvar_custom_host_variable_set(arrays_max, limits.semmni);
                rrdvar_custom_host_variable_set(semaphores_max, limits.semmns);

                arrays->red = limits.semmni;
                semaphores->red = limits.semmns;

                read_limits_next = now + step * 10;
            }
        }

        if(unlikely(ipc_sem_get_status(&status) == -1)) {
            error("Unable to get semaphore statistics");
            continue;
        }

        if(unlikely(netdata_exit)) break;

        if(semaphores->counter_done) rrdset_next(semaphores);
        rrddim_set(semaphores, "semaphores", status.semaem);
        rrdset_done(semaphores);

        if(arrays->counter_done) rrdset_next(arrays);
        rrddim_set(arrays, "arrays", status.semusz);
        rrdset_done(arrays);

        if(unlikely(netdata_exit)) break;

        if(vdo_cpu_netdata) {
            getrusage(RUSAGE_THREAD, &thread);

            if(stcpu_thread->counter_done) rrdset_next(stcpu_thread);
            rrddim_set(stcpu_thread, "user"  , thread.ru_utime.tv_sec * 1000000ULL + thread.ru_utime.tv_usec);
            rrddim_set(stcpu_thread, "system", thread.ru_stime.tv_sec * 1000000ULL + thread.ru_stime.tv_usec);
            rrdset_done(stcpu_thread);
        }
    }

    /*
    printf ("------ Semaphore Limits --------\n");
    printf ("max number of arrays = %d\n", limits.semmni);
    printf ("max semaphores per array = %d\n", limits.semmsl);
    printf ("max semaphores system wide = %d\n", limits.semmns);
    printf ("max ops per semop call = %d\n", limits.semopm);
    printf ("semaphore max value = %u\n", limits.semvmx);

    printf ("------ Semaphore Status --------\n");
    printf ("used arrays = %d\n", status.semusz);
    printf ("allocated semaphores = %d\n", status.semaem);
    */

    rrdvar_custom_host_variable_destroy(&localhost, "ipc.semaphores.arrays.max");
    rrdvar_custom_host_variable_destroy(&localhost, "ipc.semaphores.max");

cleanup:
    info("IPC thread exiting");
    pthread_exit(NULL);
    return NULL;
}
