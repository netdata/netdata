#include "common.h"

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
        snprintfz(filename, FILENAME_MAX, "%s/proc/sys/kernel/sem", netdata_configured_host_prefix);

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
        lim->semmsl = str2i(procfile_lineword(ff, 0, 0));
        lim->semmns = str2i(procfile_lineword(ff, 0, 1));
        lim->semopm = str2i(procfile_lineword(ff, 0, 2));
        lim->semmni = str2i(procfile_lineword(ff, 0, 3));
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

static inline int ipc_sem_get_status(struct ipc_status *st) {
    struct seminfo seminfo;
    union semun arg;

    arg.array = (ushort *)  (void *) &seminfo;

    if(unlikely(semctl (0, 0, SEM_INFO, arg) < 0)) {
        /* kernel not configured for semaphores */
        static int error_shown = 0;
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

int do_ipc(int update_every, usec_t dt) {
    (void)dt;

    static int initialized = 0, read_limits_next = 0;
    static struct ipc_limits limits;
    static struct ipc_status status;
    static RRDVAR *arrays_max = NULL, *semaphores_max = NULL;
    static RRDSET *st_semaphores = NULL, *st_arrays = NULL;
    static RRDDIM *rd_semaphores = NULL, *rd_arrays = NULL;

    if(unlikely(!initialized)) {
        initialized = 1;
        
        // make sure it works
        if(ipc_sem_get_limits(&limits) == -1) {
            error("unable to fetch semaphore limits");
            return 1;
        }

        // make sure it works
        if(ipc_sem_get_status(&status) == -1) {
            error("unable to fetch semaphore statistics");
            return 1;
        }

        arrays_max     = rrdvar_custom_host_variable_create(localhost, "ipc.semaphores.arrays.max");
        semaphores_max = rrdvar_custom_host_variable_create(localhost, "ipc.semaphores.max");

        if(arrays_max)     rrdvar_custom_host_variable_set(localhost, arrays_max, limits.semmni);
        if(semaphores_max) rrdvar_custom_host_variable_set(localhost, semaphores_max, limits.semmns);

        // create the charts
        if(unlikely(!st_semaphores)) {
            st_semaphores = rrdset_create_localhost(
                    "system"
                    , "ipc_semaphores"
                    , NULL
                    , "ipc semaphores"
                    , NULL
                    , "IPC Semaphores"
                    , "semaphores"
                    , "linux"
                    , "ipc"
                    , 1000
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_AREA
            );
            rd_semaphores = rrddim_add(st_semaphores, "semaphores", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        if(unlikely(!st_arrays)) {
            st_arrays = rrdset_create_localhost(
                    "system"
                    , "ipc_semaphore_arrays"
                    , NULL
                    , "ipc semaphores"
                    , NULL
                    , "IPC Semaphore Arrays"
                    , "arrays"
                    , "linux"
                    , "ipc"
                    , 1000
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_AREA
            );
            rd_arrays = rrddim_add(st_arrays, "arrays", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
    }

    if(unlikely(read_limits_next < 0)) {
        if(unlikely(ipc_sem_get_limits(&limits) == -1)) {
            error("Unable to fetch semaphore limits.");
        }
        else {
            if(arrays_max)     rrdvar_custom_host_variable_set(localhost, arrays_max, limits.semmni);
            if(semaphores_max) rrdvar_custom_host_variable_set(localhost, semaphores_max, limits.semmns);

            st_arrays->red = limits.semmni;
            st_semaphores->red = limits.semmns;

            read_limits_next = 60 / update_every;
        }
    }
    else
        read_limits_next--;

    if(unlikely(ipc_sem_get_status(&status) == -1)) {
        error("Unable to get semaphore statistics");
        return 0;
    }

    if(st_semaphores->counter_done) rrdset_next(st_semaphores);
    rrddim_set_by_pointer(st_semaphores, rd_semaphores, status.semaem);
    rrdset_done(st_semaphores);

    if(st_arrays->counter_done) rrdset_next(st_arrays);
    rrddim_set_by_pointer(st_arrays, rd_arrays, status.semusz);
    rrdset_done(st_arrays);

    return 0;
}
