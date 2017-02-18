#define NETDATA_RRD_INTERNALS 1
#include "common.h"

// ----------------------------------------------------------------------------
// RRDHOST

RRDHOST localhost = {
        .hostname = "localhost",
        .machine_guid = "",
        .rrdset_root = NULL,
        .rrdset_root_rwlock = PTHREAD_RWLOCK_INITIALIZER,
        .rrdset_root_index = {
                { NULL, rrdset_compare },
                AVL_LOCK_INITIALIZER
        },
        .rrdset_root_index_name = {
                { NULL, rrdset_compare_name },
                AVL_LOCK_INITIALIZER
        },
        .rrdfamily_root_index = {
                { NULL, rrdfamily_compare },
                AVL_LOCK_INITIALIZER
        },
        .variables_root_index = {
                { NULL, rrdvar_compare },
                AVL_LOCK_INITIALIZER
        },
        .health_log = {
                .next_log_id = 1,
                .next_alarm_id = 1,
                .count = 0,
                .max = 1000,
                .alarms = NULL,
                .alarm_log_rwlock = PTHREAD_RWLOCK_INITIALIZER
        },
        .next = NULL
};

void rrdhost_init(char *hostname) {
    localhost.hostname = hostname;
    localhost.health_log.next_log_id =
        localhost.health_log.next_alarm_id = (uint32_t)now_realtime_sec();
}

void rrdhost_rwlock(RRDHOST *host) {
    pthread_rwlock_wrlock(&host->rrdset_root_rwlock);
}

void rrdhost_rdlock(RRDHOST *host) {
    pthread_rwlock_rdlock(&host->rrdset_root_rwlock);
}

void rrdhost_unlock(RRDHOST *host) {
    pthread_rwlock_unlock(&host->rrdset_root_rwlock);
}

void rrdhost_check_rdlock_int(RRDHOST *host, const char *file, const char *function, const unsigned long line) {
    int ret = pthread_rwlock_trywrlock(&host->rrdset_root_rwlock);

    if(ret == 0)
        fatal("RRDHOST '%s' should be read-locked, but it is not, at function %s() at line %lu of file '%s'", host->hostname, function, line, file);
}

void rrdhost_check_wrlock_int(RRDHOST *host, const char *file, const char *function, const unsigned long line) {
    int ret = pthread_rwlock_tryrdlock(&host->rrdset_root_rwlock);

    if(ret == 0)
        fatal("RRDHOST '%s' should be write-locked, but it is not, at function %s() at line %lu of file '%s'", host->hostname, function, line, file);
}

void rrdhost_free(RRDHOST *host) {
    info("Freeing all memory...");

    rrdhost_rwlock(host);

    RRDSET *st;
    for(st = host->rrdset_root; st ;) {
        RRDSET *next = st->next;

        pthread_rwlock_wrlock(&st->rwlock);

        while(st->variables)
            rrdsetvar_free(st->variables);

        while(st->alarms)
            rrdsetcalc_unlink(st->alarms);

        while(st->dimensions)
            rrddim_free(st, st->dimensions);

        if(unlikely(rrdset_index_del(host, st) != st))
            error("RRDSET: INTERNAL ERROR: attempt to remove from index chart '%s', removed a different chart.", st->id);

        rrdset_index_del_name(host, st);

        st->rrdfamily->use_count--;
        if(!st->rrdfamily->use_count)
            rrdfamily_free(host, st->rrdfamily);

        pthread_rwlock_unlock(&st->rwlock);

        if(st->mapped == RRD_MEMORY_MODE_SAVE || st->mapped == RRD_MEMORY_MODE_MAP) {
            debug(D_RRD_CALLS, "Unmapping stats '%s'.", st->name);
            munmap(st, st->memsize);
        }
        else
            freez(st);

        st = next;
    }
    host->rrdset_root = NULL;

    rrdhost_unlock(host);

    info("Memory cleanup completed...");
}

void rrdhost_save(RRDHOST *host) {
    info("Saving database...");

    RRDSET *st;
    RRDDIM *rd;

    // we get an write lock
    // to ensure only one thread is saving the database
    rrdhost_rwlock(host);

    for(st = host->rrdset_root; st ; st = st->next) {
        pthread_rwlock_rdlock(&st->rwlock);

        if(st->mapped == RRD_MEMORY_MODE_SAVE) {
            debug(D_RRD_CALLS, "Saving stats '%s' to '%s'.", st->name, st->cache_filename);
            savememory(st->cache_filename, st, st->memsize);
        }

        for(rd = st->dimensions; rd ; rd = rd->next) {
            if(likely(rd->memory_mode == RRD_MEMORY_MODE_SAVE)) {
                debug(D_RRD_CALLS, "Saving dimension '%s' to '%s'.", rd->name, rd->cache_filename);
                savememory(rd->cache_filename, rd, rd->memsize);
            }
        }

        pthread_rwlock_unlock(&st->rwlock);
    }

    rrdhost_unlock(host);
}

void rrdhost_free_all(void) {
    RRDHOST *host;

    // FIXME: lock all hosts

    for(host = &localhost; host ;) {
        RRDHOST *next = host = host->next;
        rrdhost_free(host);
        host = next;
    }

    // FIXME: unlock all hosts
}

void rrdhost_save_all(void) {
    RRDHOST *host;
    for(host = &localhost; host ; host = host->next)
        rrdhost_save(host);
}
