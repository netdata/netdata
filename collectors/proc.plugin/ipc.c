// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

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

struct message_queue {
    unsigned long long id;
    int found;

    RRDDIM *rd_messages;
    RRDDIM *rd_bytes;
    unsigned long long messages;
    unsigned long long bytes;

    struct message_queue * next;
};

struct shm_stats {
    unsigned long long segments;
    unsigned long long bytes;
};

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

int ipc_msq_get_info(char *msg_filename, struct message_queue **message_queue_root) {
    static procfile *ff;
    struct message_queue *msq;

    if(unlikely(!ff)) {
        ff = procfile_open(msg_filename, " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 1;

    size_t lines = procfile_lines(ff);
    size_t words = 0;

    if(unlikely(lines < 2)) {
        error("Cannot read %s. Expected 2 or more lines, read %zu.", ff->filename, lines);
        return 1;
    }

    // loop through all lines except the first and the last ones
    size_t l;
    for(l = 1; l < lines - 1; l++) {
        words = procfile_linewords(ff, l);
        if(unlikely(words < 2)) continue;
        if(unlikely(words < 14)) {
            error("Cannot read %s line. Expected 14 params, read %zu.", ff->filename, words);
            continue;
        }

        // find the id in the linked list or create a new structure
        int found = 0;

        unsigned long long id = str2ull(procfile_lineword(ff, l, 1));
        for(msq = *message_queue_root; msq ; msq = msq->next) {
            if(unlikely(id == msq->id)) {
                found = 1;
                break;
            }
        }

        if(unlikely(!found)) {
            msq = callocz(1, sizeof(struct message_queue));
            msq->next = *message_queue_root;
            *message_queue_root = msq;
            msq->id = id;
        }

        msq->messages = str2ull(procfile_lineword(ff, l, 4));
        msq->bytes    = str2ull(procfile_lineword(ff, l, 3));
        msq->found    = 1;
    }

    return 0;
}

int ipc_shm_get_info(char *shm_filename, struct shm_stats *shm) {
    static procfile *ff;

    if(unlikely(!ff)) {
        ff = procfile_open(shm_filename, " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 1;

    size_t lines = procfile_lines(ff);
    size_t words = 0;

    if(unlikely(lines < 2)) {
        error("Cannot read %s. Expected 2 or more lines, read %zu.", ff->filename, lines);
        return 1;
    }

    shm->segments = 0;
    shm->bytes = 0;

    // loop through all lines except the first and the last ones
    size_t l;
    for(l = 1; l < lines - 1; l++) {
        words = procfile_linewords(ff, l);
        if(unlikely(words < 2)) continue;
        if(unlikely(words < 16)) {
            error("Cannot read %s line. Expected 16 params, read %zu.", ff->filename, words);
            continue;
        }

        shm->segments++;
        shm->bytes += str2ull(procfile_lineword(ff, l, 3));
    }

    return 0;
}

int do_ipc(int update_every, usec_t dt) {
    (void)dt;

    static int do_sem = -1, do_msg = -1, do_shm = -1;
    static int read_limits_next = -1;
    static struct ipc_limits limits;
    static struct ipc_status status;
    static RRDVAR *arrays_max = NULL, *semaphores_max = NULL;
    static RRDSET *st_semaphores = NULL, *st_arrays = NULL;
    static RRDDIM *rd_semaphores = NULL, *rd_arrays = NULL;
    static char *msg_filename = NULL;
    static struct message_queue *message_queue_root = NULL;
    static long long dimensions_limit;
    static char *shm_filename = NULL;

    if(unlikely(do_sem == -1)) {
        do_msg = config_get_boolean("plugin:proc:ipc", "message queues", CONFIG_BOOLEAN_YES);
        do_sem = config_get_boolean("plugin:proc:ipc", "semaphore totals", CONFIG_BOOLEAN_YES);
        do_shm = config_get_boolean("plugin:proc:ipc", "shared memory totals", CONFIG_BOOLEAN_YES);

        char filename[FILENAME_MAX + 1];

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/sysvipc/msg");
        msg_filename = config_get("plugin:proc:ipc", "msg filename to monitor", filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/sysvipc/shm");
        shm_filename = config_get("plugin:proc:ipc", "shm filename to monitor", filename);

        dimensions_limit = config_get_number("plugin:proc:ipc", "max dimensions in memory allowed", 50);

        // make sure it works
        if(ipc_sem_get_limits(&limits) == -1) {
            error("unable to fetch semaphore limits");
            do_sem = CONFIG_BOOLEAN_NO;
        }
        else if(ipc_sem_get_status(&status) == -1) {
            error("unable to fetch semaphore statistics");
            do_sem = CONFIG_BOOLEAN_NO;
        }
        else {
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
                        , PLUGIN_PROC_NAME
                        , "ipc"
                        , NETDATA_CHART_PRIO_SYSTEM_IPC_SEMAPHORES
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
                        , PLUGIN_PROC_NAME
                        , "ipc"
                        , NETDATA_CHART_PRIO_SYSTEM_IPC_SEM_ARRAYS
                        , localhost->rrd_update_every
                        , RRDSET_TYPE_AREA
                );
                rd_arrays = rrddim_add(st_arrays, "arrays", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            // variables
            semaphores_max = rrdvar_custom_host_variable_create(localhost, "ipc_semaphores_max");
            arrays_max     = rrdvar_custom_host_variable_create(localhost, "ipc_semaphores_arrays_max");
        }

        struct stat stbuf;
        if (stat(msg_filename, &stbuf)) {
            do_msg = CONFIG_BOOLEAN_NO;
        }

        if(unlikely(do_sem == CONFIG_BOOLEAN_NO && do_msg == CONFIG_BOOLEAN_NO)) {
            error("ipc module disabled");
            return 1;
        }
    }

    if(likely(do_sem != CONFIG_BOOLEAN_NO)) {
        if(unlikely(read_limits_next < 0)) {
            if(unlikely(ipc_sem_get_limits(&limits) == -1)) {
                error("Unable to fetch semaphore limits.");
            }
            else {
                if(semaphores_max) rrdvar_custom_host_variable_set(localhost, semaphores_max, limits.semmns);
                if(arrays_max)     rrdvar_custom_host_variable_set(localhost, arrays_max,     limits.semmni);

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
    }

    // --------------------------------------------------------------------

    if(likely(do_msg != CONFIG_BOOLEAN_NO)) {
        static RRDSET *st_msq_messages = NULL, *st_msq_bytes = NULL;

        int ret = ipc_msq_get_info(msg_filename, &message_queue_root);

        if(!ret && message_queue_root) {
            if(unlikely(!st_msq_messages))
                st_msq_messages = rrdset_create_localhost(
                        "system"
                        , "message_queue_messages"
                        , NULL
                        , "ipc message queues"
                        , NULL
                        , "IPC Message Queue Number of Messages"
                        , "messages"
                        , PLUGIN_PROC_NAME
                        , "ipc"
                        , NETDATA_CHART_PRIO_SYSTEM_IPC_MSQ_MESSAGES
                        , update_every
                        , RRDSET_TYPE_STACKED
                );
            else
                rrdset_next(st_msq_messages);

            if(unlikely(!st_msq_bytes))
                st_msq_bytes = rrdset_create_localhost(
                        "system"
                        , "message_queue_bytes"
                        , NULL
                        , "ipc message queues"
                        , NULL
                        , "IPC Message Queue Used Bytes"
                        , "bytes"
                        , PLUGIN_PROC_NAME
                        , "ipc"
                        , NETDATA_CHART_PRIO_SYSTEM_IPC_MSQ_SIZE
                        , update_every
                        , RRDSET_TYPE_STACKED
                );
            else
                rrdset_next(st_msq_bytes);

            struct message_queue *msq = message_queue_root, *msq_prev = NULL;
            while(likely(msq)){
                if(likely(msq->found)) {
                    if(unlikely(!msq->rd_messages || !msq->rd_bytes)) {
                        char id[RRD_ID_LENGTH_MAX + 1];
                        snprintfz(id, RRD_ID_LENGTH_MAX, "%llu", msq->id);
                        if(likely(!msq->rd_messages)) msq->rd_messages = rrddim_add(st_msq_messages, id, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        if(likely(!msq->rd_bytes)) msq->rd_bytes = rrddim_add(st_msq_bytes, id, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    }

                    rrddim_set_by_pointer(st_msq_messages, msq->rd_messages, msq->messages);
                    rrddim_set_by_pointer(st_msq_bytes, msq->rd_bytes, msq->bytes);

                    msq->found = 0;
                }
                else {
                    rrddim_is_obsolete(st_msq_messages, msq->rd_messages);
                    rrddim_is_obsolete(st_msq_bytes, msq->rd_bytes);

                    // remove message queue from the linked list
                    if(!msq_prev)
                        message_queue_root = msq->next;
                    else
                        msq_prev->next = msq->next;
                    freez(msq);
                    msq = NULL;
                }
                if(likely(msq)) {
                    msq_prev = msq;
                    msq = msq->next;
                }
                else if(!msq_prev)
                    msq = message_queue_root;
                else
                    msq = msq_prev->next;
            }

            rrdset_done(st_msq_messages);
            rrdset_done(st_msq_bytes);

            long long dimensions_num = 0;
            RRDDIM *rd;
            rrdset_rdlock(st_msq_messages);
            rrddim_foreach_read(rd, st_msq_messages) dimensions_num++;
            rrdset_unlock(st_msq_messages);

            if(unlikely(dimensions_num > dimensions_limit)) {
                info("Message queue statistics has been disabled");
                info("There are %lld dimensions in memory but limit was set to %lld", dimensions_num, dimensions_limit);
                rrdset_is_obsolete(st_msq_messages);
                rrdset_is_obsolete(st_msq_bytes);
                st_msq_messages = NULL;
                st_msq_bytes = NULL;
                do_msg = CONFIG_BOOLEAN_NO;
            }
            else if(unlikely(!message_queue_root)) {
                info("Making chart %s (%s) obsolete since it does not have any dimensions", rrdset_name(st_msq_messages), st_msq_messages->id);
                rrdset_is_obsolete(st_msq_messages);
                st_msq_messages = NULL;

                info("Making chart %s (%s) obsolete since it does not have any dimensions", rrdset_name(st_msq_bytes), st_msq_bytes->id);
                rrdset_is_obsolete(st_msq_bytes);
                st_msq_bytes = NULL;
            }
        }
    }

    // --------------------------------------------------------------------

    if(likely(do_shm != CONFIG_BOOLEAN_NO)) {
        static RRDSET *st_shm_segments = NULL, *st_shm_bytes = NULL;
        static RRDDIM *rd_shm_segments = NULL, *rd_shm_bytes = NULL;
        struct shm_stats shm;

        if(!ipc_shm_get_info(shm_filename, &shm)) {
            if(unlikely(!st_shm_segments)) {
                st_shm_segments = rrdset_create_localhost(
                        "system"
                        , "shared_memory_segments"
                        , NULL
                        , "ipc shared memory"
                        , NULL
                        , "IPC Shared Memory Number of Segments"
                        , "segments"
                        , PLUGIN_PROC_NAME
                        , "ipc"
                        , NETDATA_CHART_PRIO_SYSTEM_IPC_SHARED_MEM_SEGS
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rd_shm_segments = rrddim_add(st_shm_segments, "segments", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else
                rrdset_next(st_shm_segments);

            rrddim_set_by_pointer(st_shm_segments, rd_shm_segments, shm.segments);

            rrdset_done(st_shm_segments);

            // --------------------------------------------------------------------

            if(unlikely(!st_shm_bytes)) {
                st_shm_bytes = rrdset_create_localhost(
                        "system"
                        , "shared_memory_bytes"
                        , NULL
                        , "ipc shared memory"
                        , NULL
                        , "IPC Shared Memory Used Bytes"
                        , "bytes"
                        , PLUGIN_PROC_NAME
                        , "ipc"
                        , NETDATA_CHART_PRIO_SYSTEM_IPC_SHARED_MEM_SIZE
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rd_shm_bytes = rrddim_add(st_shm_bytes, "bytes", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else
                rrdset_next(st_shm_bytes);

            rrddim_set_by_pointer(st_shm_bytes, rd_shm_bytes, shm.bytes);

            rrdset_done(st_shm_bytes);
        }
    }

    return 0;
}
