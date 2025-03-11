// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-workers.h"

#define WORKERS_MIN_PERCENT_DEFAULT 10000.0

struct worker_spinlocks {
    size_t locks;
    size_t spins;

    RRDDIM *rd_locks;
    RRDDIM *rd_spins;
};

DEFINE_JUDYL_TYPED(SPINLOCKS, struct worker_spinlocks *);
SPINLOCKS_JudyLSet ALL_SPINLOCKS = { 0 };

struct worker_job_type_gs {
    STRING *name;
    STRING *units;

    struct {
        size_t jobs_started;
        usec_t busy_time;
    } data[2];

    RRDDIM *rd_jobs_started;
    RRDDIM *rd_busy_time;
    RRDDIM *rd_avg_time;

    WORKER_METRIC_TYPE type;
    NETDATA_DOUBLE min_value;
    NETDATA_DOUBLE max_value;
    NETDATA_DOUBLE sum_value;
    size_t count_value;

    RRDSET *st;
    RRDDIM *rd_min;
    RRDDIM *rd_max;
    RRDDIM *rd_avg;
};

struct worker_thread {
    pid_t pid;
    bool enabled;

    bool cpu_enabled;
    double cpu;

    kernel_uint_t utime;
    kernel_uint_t stime;

    kernel_uint_t utime_old;
    kernel_uint_t stime_old;

    usec_t collected_time;
    usec_t collected_time_old;

    size_t jobs_started;
    usec_t busy_time;

    struct worker_thread *next;
    struct worker_thread *prev;
};

struct worker_utilization {
    const char *name;
    const char *family;
    size_t priority;
    uint32_t flags;

    char *name_lowercase;

    struct worker_job_type_gs per_job_type[WORKER_UTILIZATION_MAX_JOB_TYPES];

    size_t workers_max_job_id;
    size_t workers_registered;
    size_t workers_busy;
    usec_t workers_total_busy_time;
    usec_t workers_total_duration;
    size_t workers_total_jobs_started;
    double workers_min_busy_time;
    double workers_max_busy_time;

    size_t workers_cpu_registered;
    double workers_cpu_min;
    double workers_cpu_max;
    double workers_cpu_total;

    uint64_t memory_calls[WORKERS_MEMORY_CALL_MAX];

    struct worker_thread *threads;

    RRDSET *st_workers_time;
    RRDDIM *rd_workers_time_avg;
    RRDDIM *rd_workers_time_min;
    RRDDIM *rd_workers_time_max;

    RRDSET *st_workers_cpu;
    RRDDIM *rd_workers_cpu_avg;
    RRDDIM *rd_workers_cpu_min;
    RRDDIM *rd_workers_cpu_max;

    RRDSET *st_workers_threads;
    RRDDIM *rd_workers_threads_free;
    RRDDIM *rd_workers_threads_busy;

    RRDSET *st_workers_jobs_per_job_type;
    RRDSET *st_workers_time_per_job_type;
    RRDSET *st_workers_avg_time_per_job_type;

    RRDDIM *rd_total_cpu_utilizaton;

    RRDSET *st_spinlocks_locks;
    RRDSET *st_spinlocks_spins;
    SPINLOCKS_JudyLSet spinlocks;

    RRDSET *st_memory_calls;
    RRDDIM *rd_memory_calls[WORKERS_MEMORY_CALL_MAX];
};

static struct worker_utilization all_workers_utilization[] = {
    { .name = "PULSE",       .family = "workers pulse",                   .priority = 1000000 },
    { .name = "HEALTH",      .family = "workers health alerts",           .priority = 1000000 },
    { .name = "MLTRAIN",     .family = "workers ML training",             .priority = 1000000 },
    { .name = "MLDETECT",    .family = "workers ML detection",            .priority = 1000000 },
    { .name = "STREAM",      .family = "workers streaming",               .priority = 1000000 },
    { .name = "STREAMCNT",   .family = "workers streaming connect",       .priority = 1000000 },
    { .name = "DBENGINE",    .family = "workers dbengine instances",      .priority = 1000000 },
    { .name = "LIBUV",       .family = "workers libuv threadpool",        .priority = 1000000 },
    { .name = "WEB",         .family = "workers web server",              .priority = 1000000 },
    { .name = "ACLK",        .family = "workers aclk",                    .priority = 1000000 },
    { .name = "ACLKSYNC",    .family = "workers aclk sync",               .priority = 1000000 },
    { .name = "METASYNC",    .family = "workers metadata sync",           .priority = 1000000 },
    { .name = "PLUGINSD",    .family = "workers plugins.d",               .priority = 1000000 },
    { .name = "STATSD",      .family = "workers plugin statsd",           .priority = 1000000 },
    { .name = "STATSDFLUSH", .family = "workers plugin statsd flush",     .priority = 1000000 },
    { .name = "PROC",        .family = "workers plugin proc",             .priority = 1000000 },
    { .name = "WIN",        .family = "workers plugin windows",           .priority = 1000000 },
    { .name = "NETDEV",      .family = "workers plugin proc netdev",      .priority = 1000000 },
    { .name = "FREEBSD",     .family = "workers plugin freebsd",          .priority = 1000000 },
    { .name = "MACOS",       .family = "workers plugin macos",            .priority = 1000000 },
    { .name = "CGROUPS",     .family = "workers plugin cgroups",          .priority = 1000000 },
    { .name = "CGROUPSDISC", .family = "workers plugin cgroups find",     .priority = 1000000 },
    { .name = "DISKSPACE",   .family = "workers plugin diskspace",        .priority = 1000000 },
    { .name = "TC",          .family = "workers plugin tc",               .priority = 1000000 },
    { .name = "TIMEX",       .family = "workers plugin timex",            .priority = 1000000 },
    { .name = "IDLEJITTER",  .family = "workers plugin idlejitter",       .priority = 1000000 },
    { .name = "RRDCONTEXT",  .family = "workers contexts",                .priority = 1000000 },
    { .name = "REPLICATION", .family = "workers replication sender",      .priority = 1000000 },
    { .name = "SERVICE",     .family = "workers service",                 .priority = 1000000 },
    { .name = "PROFILER",    .family = "workers profile",                 .priority = 1000000 },
    { .name = "PGCEVICT",    .family = "workers dbengine eviction",       .priority = 1000000 },
    { .name = "BACKFILL",    .family = "workers backfill",                .priority = 1000000 },

    // has to be terminated with a NULL
    { .name = NULL,          .family = NULL       }
};

static void workers_total_spinlock_contention_chart(void) {
    {
        static RRDSET *st = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                "netdata"
                , "spinlock_total_locks"
                , NULL
                , "spinlocks"
                , "netdata.spinlock_total_locks"
                , "Netdata Total Spinlock Locks"
                , "locks"
                , "netdata"
                , "pulse"
                , 920000
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );
        }

        Word_t idx = 0;
        for(struct worker_spinlocks *wusp = SPINLOCKS_FIRST(&ALL_SPINLOCKS, &idx);
             wusp;
             wusp = SPINLOCKS_NEXT(&ALL_SPINLOCKS, &idx)) {
            const char *func = (const char *)idx;
            RRDDIM *rd = rrddim_find(st, func);
            if(!rd) rd = rrddim_add(st, func, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrddim_set_by_pointer(st, rd, (collected_number)wusp->locks);
        }

        rrdset_done(st);
    }

    {
        static RRDSET *st = NULL;
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                "netdata"
                , "spinlock_total_spins"
                , NULL
                , "spinlocks"
                , "netdata.spinlock_total_spins"
                , "Netdata Total Spinlock Spins"
                , "spins"
                , "netdata"
                , "pulse"
                , 920001
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );
        }

        Word_t idx = 0;
        for(struct worker_spinlocks *wusp = SPINLOCKS_FIRST(&ALL_SPINLOCKS, &idx);
             wusp;
             wusp = SPINLOCKS_NEXT(&ALL_SPINLOCKS, &idx)) {
            const char *func = (const char *)idx;
            RRDDIM *rd = rrddim_find(st, func);
            if(!rd) rd = rrddim_add(st, func, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrddim_set_by_pointer(st, rd, (collected_number)wusp->spins);
        }

        rrdset_done(st);
    }

    {
        static RRDSET *st = NULL;
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                "netdata"
                , "spinlock_total_spins_per_lock"
                , NULL
                , "spinlocks"
                , "netdata.spinlock_total_spins_per_lock"
                , "Netdata Average Spinlock Spins Per Lock"
                , "spins"
                , "netdata"
                , "pulse"
                , 920002
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );
        }

        Word_t idx = 0;
        for(struct worker_spinlocks *wusp = SPINLOCKS_FIRST(&ALL_SPINLOCKS, &idx);
             wusp;
             wusp = SPINLOCKS_NEXT(&ALL_SPINLOCKS, &idx)) {
            const char *func = (const char *)idx;
            RRDDIM *rd = rrddim_find(st, func);
            if(!rd) rd = rrddim_add(st, func, NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
            if(!wusp->locks)
                rrddim_set_by_pointer(st, rd, 0);
            else
                rrddim_set_by_pointer(st, rd, (collected_number)((uint64_t)wusp->spins * 10000ULL / (uint64_t)wusp->locks));
        }

        rrdset_done(st);
    }
}

static void workers_total_memory_calls_chart(void) {
    {
        static RRDSET *st = NULL;
        static RRDDIM *rd[WORKERS_MEMORY_CALL_MAX] = { NULL };
        uint64_t memory_calls[WORKERS_MEMORY_CALL_MAX] = { 0 };

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                "netdata"
                , "memory_calls_total"
                , NULL
                , "memory calls"
                , "netdata.memory_calls_total"
                , "Netdata Total Memory Calls"
                , "calls"
                , "netdata"
                , "pulse"
                , 920005
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            for (int j = 0; j < WORKERS_MEMORY_CALL_MAX; ++j)
                rd[j] = rrddim_add(st, WORKERS_MEMORY_CALL_2str(j), NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        for(size_t i = 0; all_workers_utilization[i].name ;i++) {
            struct worker_utilization *wu = &all_workers_utilization[i];

            for (int j = 0; j < WORKERS_MEMORY_CALL_MAX; ++j)
                memory_calls[j] += wu->memory_calls[j];
        }

        for (int j = 0; j < WORKERS_MEMORY_CALL_MAX; ++j)
            rrddim_set_by_pointer(st, rd[j], (collected_number)memory_calls[j]);

        rrdset_done(st);
    }
}

static void workers_total_cpu_utilization_chart(void) {
    size_t i, cpu_enabled = 0;
    for(i = 0; all_workers_utilization[i].name ;i++)
        if(all_workers_utilization[i].workers_cpu_registered) cpu_enabled++;

    if(!cpu_enabled) return;

    static RRDSET *st = NULL;

    if(!st) {
        st = rrdset_create_localhost(
            "netdata",
            "workers_cpu",
            NULL,
            "workers",
            "netdata.workers.cpu_total",
            "Netdata Workers CPU Utilization (100% = 1 core)",
            "%",
            "netdata",
            "pulse",
            999000,
            localhost->rrd_update_every,
            RRDSET_TYPE_STACKED);
    }

    for(i = 0; all_workers_utilization[i].name ;i++) {
        struct worker_utilization *wu = &all_workers_utilization[i];
        if(!wu->workers_cpu_registered) continue;

        if(!wu->rd_total_cpu_utilizaton)
            wu->rd_total_cpu_utilizaton = rrddim_add(st, wu->name_lowercase, NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);

        rrddim_set_by_pointer(st, wu->rd_total_cpu_utilizaton, (collected_number)((double)wu->workers_cpu_total * 100.0));
    }

    rrdset_done(st);
}

#define WORKER_CHART_DECIMAL_PRECISION 100

static void workers_utilization_update_chart(struct worker_utilization *wu) {
    if(!wu->workers_registered) return;

    //fprintf(stderr, "%-12s WORKER UTILIZATION: %-3.2f%%, %zu jobs done, %zu running, on %zu workers, min %-3.02f%%, max %-3.02f%%.\n",
    //        wu->name,
    //        (double)wu->workers_total_busy_time * 100.0 / (double)wu->workers_total_duration,
    //        wu->workers_total_jobs_started, wu->workers_busy, wu->workers_registered,
    //        wu->workers_min_busy_time, wu->workers_max_busy_time);

    // ----------------------------------------------------------------------

    if(unlikely(!wu->st_workers_time)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_time_%s", wu->name_lowercase);

        char context[RRD_ID_LENGTH_MAX + 1];
        snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.time", wu->name_lowercase);

        wu->st_workers_time = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , context
            , "Netdata Workers Busy Time (100% = all workers busy)"
            , "%"
            , "netdata"
            , "pulse"
            , wu->priority
            , localhost->rrd_update_every
            , RRDSET_TYPE_AREA
        );
    }

    // we add the min and max dimensions only when we have multiple workers

    if(unlikely(!wu->rd_workers_time_min && wu->workers_registered > 1))
        wu->rd_workers_time_min = rrddim_add(wu->st_workers_time, "min", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);

    if(unlikely(!wu->rd_workers_time_max && wu->workers_registered > 1))
        wu->rd_workers_time_max = rrddim_add(wu->st_workers_time, "max", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);

    if(unlikely(!wu->rd_workers_time_avg))
        wu->rd_workers_time_avg = rrddim_add(wu->st_workers_time, "average", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);

    if(unlikely(wu->workers_min_busy_time == WORKERS_MIN_PERCENT_DEFAULT)) wu->workers_min_busy_time = 0.0;

    if(wu->rd_workers_time_min)
        rrddim_set_by_pointer(wu->st_workers_time, wu->rd_workers_time_min, (collected_number)((double)wu->workers_min_busy_time * WORKER_CHART_DECIMAL_PRECISION));

    if(wu->rd_workers_time_max)
        rrddim_set_by_pointer(wu->st_workers_time, wu->rd_workers_time_max, (collected_number)((double)wu->workers_max_busy_time * WORKER_CHART_DECIMAL_PRECISION));

    if(wu->workers_total_duration == 0)
        rrddim_set_by_pointer(wu->st_workers_time, wu->rd_workers_time_avg, 0);
    else
        rrddim_set_by_pointer(wu->st_workers_time, wu->rd_workers_time_avg, (collected_number)((double)wu->workers_total_busy_time * 100.0 * WORKER_CHART_DECIMAL_PRECISION / (double)wu->workers_total_duration));

    rrdset_done(wu->st_workers_time);

    // ----------------------------------------------------------------------

#ifdef __linux__
    if(wu->workers_cpu_registered || wu->st_workers_cpu) {
        if(unlikely(!wu->st_workers_cpu)) {
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(name, RRD_ID_LENGTH_MAX, "workers_cpu_%s", wu->name_lowercase);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.cpu", wu->name_lowercase);

            wu->st_workers_cpu = rrdset_create_localhost(
                "netdata"
                , name
                , NULL
                , wu->family
                , context
                , "Netdata Workers CPU Utilization (100% = all workers busy)"
                , "%"
                , "netdata"
                , "pulse"
                , wu->priority + 1
                , localhost->rrd_update_every
                , RRDSET_TYPE_AREA
            );
        }

        if (unlikely(!wu->rd_workers_cpu_min && wu->workers_registered > 1))
            wu->rd_workers_cpu_min = rrddim_add(wu->st_workers_cpu, "min", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);

        if (unlikely(!wu->rd_workers_cpu_max && wu->workers_registered > 1))
            wu->rd_workers_cpu_max = rrddim_add(wu->st_workers_cpu, "max", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);

        if(unlikely(!wu->rd_workers_cpu_avg))
            wu->rd_workers_cpu_avg = rrddim_add(wu->st_workers_cpu, "average", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);

        if(unlikely(wu->workers_cpu_min == WORKERS_MIN_PERCENT_DEFAULT)) wu->workers_cpu_min = 0.0;

        if(wu->rd_workers_cpu_min)
            rrddim_set_by_pointer(wu->st_workers_cpu, wu->rd_workers_cpu_min, (collected_number)(wu->workers_cpu_min * WORKER_CHART_DECIMAL_PRECISION));

        if(wu->rd_workers_cpu_max)
            rrddim_set_by_pointer(wu->st_workers_cpu, wu->rd_workers_cpu_max, (collected_number)(wu->workers_cpu_max * WORKER_CHART_DECIMAL_PRECISION));

        if(wu->workers_cpu_registered == 0)
            rrddim_set_by_pointer(wu->st_workers_cpu, wu->rd_workers_cpu_avg, 0);
        else
            rrddim_set_by_pointer(wu->st_workers_cpu, wu->rd_workers_cpu_avg, (collected_number)( wu->workers_cpu_total * WORKER_CHART_DECIMAL_PRECISION / (NETDATA_DOUBLE)wu->workers_cpu_registered ));

        rrdset_done(wu->st_workers_cpu);
    }
#endif

    // ----------------------------------------------------------------------------------------------------------------

    if(unlikely(!wu->st_workers_jobs_per_job_type)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_jobs_by_type_%s", wu->name_lowercase);

        char context[RRD_ID_LENGTH_MAX + 1];
        snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.jobs_started_by_type", wu->name_lowercase);

        wu->st_workers_jobs_per_job_type = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , context
            , "Netdata Workers Jobs Started by Type"
            , "jobs"
            , "netdata"
            , "pulse"
            , wu->priority + 2
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED
        );
    }

    {
        size_t i;
        for(i = 0; i <= wu->workers_max_job_id ;i++) {
            if(unlikely(wu->per_job_type[i].type != WORKER_METRIC_IDLE_BUSY))
                continue;

            if (wu->per_job_type[i].name) {

                if(unlikely(!wu->per_job_type[i].rd_jobs_started))
                    wu->per_job_type[i].rd_jobs_started = rrddim_add(wu->st_workers_jobs_per_job_type, string2str(wu->per_job_type[i].name), NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrddim_set_by_pointer(wu->st_workers_jobs_per_job_type, wu->per_job_type[i].rd_jobs_started, (collected_number)(wu->per_job_type[i].data[0].jobs_started));
            }
        }
    }

    rrdset_done(wu->st_workers_jobs_per_job_type);

    // ----------------------------------------------------------------------------------------------------------------

    if(unlikely(!wu->st_workers_time_per_job_type)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_busy_time_by_type_%s", wu->name_lowercase);

        char context[RRD_ID_LENGTH_MAX + 1];
        snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.time_by_type", wu->name_lowercase);

        wu->st_workers_time_per_job_type = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , context
            , "Netdata Workers Busy Time by Type"
            , "ms"
            , "netdata"
            , "pulse"
            , wu->priority + 3
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED
        );
    }

    {
        size_t i;
        for(i = 0; i <= wu->workers_max_job_id ;i++) {
            if(unlikely(wu->per_job_type[i].type != WORKER_METRIC_IDLE_BUSY))
                continue;

            if (wu->per_job_type[i].name) {

                if(unlikely(!wu->per_job_type[i].rd_busy_time))
                    wu->per_job_type[i].rd_busy_time = rrddim_add(wu->st_workers_time_per_job_type, string2str(wu->per_job_type[i].name), NULL, 1, USEC_PER_MS, RRD_ALGORITHM_ABSOLUTE);

                rrddim_set_by_pointer(wu->st_workers_time_per_job_type, wu->per_job_type[i].rd_busy_time, (collected_number)(wu->per_job_type[i].data[0].busy_time));
            }
        }
    }

    rrdset_done(wu->st_workers_time_per_job_type);

    // ----------------------------------------------------------------------------------------------------------------

    if(unlikely(!wu->st_workers_avg_time_per_job_type)) {
        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, RRD_ID_LENGTH_MAX, "workers_avg_time_by_type_%s", wu->name_lowercase);

        char context[RRD_ID_LENGTH_MAX + 1];
        snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.avg_time_by_type", wu->name_lowercase);

        wu->st_workers_avg_time_per_job_type = rrdset_create_localhost(
            "netdata"
            , name
            , NULL
            , wu->family
            , context
            , "Netdata Workers Average Time by Type"
            , "ms"
            , "netdata"
            , "pulse"
            , wu->priority + 4
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED
        );
    }

    {
        size_t i;
        for(i = 0; i <= wu->workers_max_job_id ;i++) {
            if(unlikely(wu->per_job_type[i].type != WORKER_METRIC_IDLE_BUSY))
                continue;

            if (wu->per_job_type[i].name) {

                if(unlikely(!wu->per_job_type[i].rd_avg_time))
                    wu->per_job_type[i].rd_avg_time = rrddim_add(wu->st_workers_avg_time_per_job_type, string2str(wu->per_job_type[i].name), NULL, 1, USEC_PER_MS, RRD_ALGORITHM_ABSOLUTE);

                ssize_t jobs_delta = (ssize_t)wu->per_job_type[i].data[0].jobs_started;
                susec_t time_delta = (susec_t)wu->per_job_type[i].data[0].busy_time;
                susec_t average = (jobs_delta != 0) ? time_delta / jobs_delta : 0;
                rrddim_set_by_pointer(wu->st_workers_avg_time_per_job_type, wu->per_job_type[i].rd_avg_time, (collected_number)average);
            }
        }
    }

    rrdset_done(wu->st_workers_avg_time_per_job_type);

    // ----------------------------------------------------------------------------------------------------------------

    if(wu->st_workers_threads || wu->workers_registered > 1) {
        if(unlikely(!wu->st_workers_threads)) {
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(name, RRD_ID_LENGTH_MAX, "workers_threads_%s", wu->name_lowercase);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.threads", wu->name_lowercase);

            wu->st_workers_threads = rrdset_create_localhost(
                "netdata"
                , name
                , NULL
                , wu->family
                , context
                , "Netdata Workers Threads"
                , "threads"
                , "netdata"
                , "pulse"
                , wu->priority + 5
                , localhost->rrd_update_every
                , RRDSET_TYPE_STACKED
            );

            wu->rd_workers_threads_free = rrddim_add(wu->st_workers_threads, "free", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            wu->rd_workers_threads_busy = rrddim_add(wu->st_workers_threads, "busy", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(wu->st_workers_threads, wu->rd_workers_threads_free, (collected_number)(wu->workers_registered - wu->workers_busy));
        rrddim_set_by_pointer(wu->st_workers_threads, wu->rd_workers_threads_busy, (collected_number)(wu->workers_busy));
        rrdset_done(wu->st_workers_threads);
    }

    // ----------------------------------------------------------------------
    // spinlocks

    {
        if(unlikely(!wu->st_spinlocks_locks)) {
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(name, RRD_ID_LENGTH_MAX, "workers_spinlock_locks_%s", wu->name_lowercase);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.spinlock_locks", wu->name_lowercase);

            wu->st_spinlocks_locks = rrdset_create_localhost(
                "netdata"
                , name
                , NULL
                , wu->family
                , context
                , "Netdata Spinlock Locks"
                , "locks"
                , "netdata"
                , "pulse"
                , wu->priority + 6
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );
        }

        Word_t idx = 0;
        for(struct worker_spinlocks *wusp = SPINLOCKS_FIRST(&wu->spinlocks, &idx);
             wusp;
             wusp = SPINLOCKS_NEXT(&wu->spinlocks, &idx)) {
            const char *func = (const char *)idx;
            if(!wusp->rd_locks)
                wusp->rd_locks = rrddim_add(wu->st_spinlocks_locks, func, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(wu->st_spinlocks_locks, wusp->rd_locks, (collected_number)wusp->locks);
        }

        rrdset_done(wu->st_spinlocks_locks);
    }

    {
        if(unlikely(!wu->st_spinlocks_spins)) {
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(name, RRD_ID_LENGTH_MAX, "workers_spinlock_spins_%s", wu->name_lowercase);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.spinlock_spins", wu->name_lowercase);

            wu->st_spinlocks_spins = rrdset_create_localhost(
                "netdata"
                , name
                , NULL
                , wu->family
                , context
                , "Netdata Spinlock Spins"
                , "spins"
                , "netdata"
                , "pulse"
                , wu->priority + 7
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );
        }

        Word_t idx = 0;
        for(struct worker_spinlocks *wusp = SPINLOCKS_FIRST(&wu->spinlocks, &idx);
             wusp;
             wusp = SPINLOCKS_NEXT(&wu->spinlocks, &idx)) {
            const char *func = (const char *)idx;
            if(!wusp->rd_spins)
                wusp->rd_spins = rrddim_add(wu->st_spinlocks_spins, func, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrddim_set_by_pointer(wu->st_spinlocks_spins, wusp->rd_spins, (collected_number)wusp->spins);
        }

        rrdset_done(wu->st_spinlocks_spins);
    }

    // ----------------------------------------------------------------------
    // memory calls
    
    {
        if(unlikely(!wu->st_memory_calls)) {
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(name, RRD_ID_LENGTH_MAX, "workers_memory_calls_%s", wu->name_lowercase);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.memory_calls", wu->name_lowercase);

            wu->st_memory_calls = rrdset_create_localhost(
                "netdata"
                , name
                , NULL
                , wu->family
                , context
                , "Netdata Memory Calls"
                , "calls"
                , "netdata"
                , "pulse"
                , wu->priority + 8
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );
        }

        for(size_t i = 0; i < WORKERS_MEMORY_CALL_MAX; i++) {
            if(!wu->rd_memory_calls[i])
                wu->rd_memory_calls[i] = rrddim_add(wu->st_memory_calls, WORKERS_MEMORY_CALL_2str(i), NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(wu->st_memory_calls, wu->rd_memory_calls[i], (collected_number)wu->memory_calls[i]);
        }

        rrdset_done(wu->st_memory_calls);
    }
    
    // ----------------------------------------------------------------------
    // custom metric types WORKER_METRIC_ABSOLUTE

    {
        size_t i;
        for (i = 0; i <= wu->workers_max_job_id; i++) {
            if(wu->per_job_type[i].type != WORKER_METRIC_ABSOLUTE)
                continue;

            if(!wu->per_job_type[i].count_value)
                continue;

            if(!wu->per_job_type[i].st) {
                size_t job_name_len = string_strlen(wu->per_job_type[i].name);
                if(job_name_len > RRD_ID_LENGTH_MAX) job_name_len = RRD_ID_LENGTH_MAX;

                char job_name_sanitized[job_name_len + 1];
                rrdset_strncpyz_name(job_name_sanitized, string2str(wu->per_job_type[i].name), job_name_len);

                char name[RRD_ID_LENGTH_MAX + 1];
                snprintfz(name, RRD_ID_LENGTH_MAX, "workers_%s_value_%s", wu->name_lowercase, job_name_sanitized);

                char context[RRD_ID_LENGTH_MAX + 1];
                snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.value.%s", wu->name_lowercase, job_name_sanitized);

                char title[1000 + 1];
                snprintf(title, 1000, "Netdata Workers %s value of %s", wu->name_lowercase, string2str(wu->per_job_type[i].name));

                wu->per_job_type[i].st = rrdset_create_localhost(
                    "netdata"
                    , name
                    , NULL
                    , wu->family
                    , context
                    , title
                    , (wu->per_job_type[i].units)?string2str(wu->per_job_type[i].units):"value"
                    , "netdata"
                    , "pulse"
                    , wu->priority + 10 + i
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
                );

                wu->per_job_type[i].rd_min = rrddim_add(wu->per_job_type[i].st, "min", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
                wu->per_job_type[i].rd_max = rrddim_add(wu->per_job_type[i].st, "max", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
                wu->per_job_type[i].rd_avg = rrddim_add(wu->per_job_type[i].st, "average", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(wu->per_job_type[i].st, wu->per_job_type[i].rd_min, (collected_number)(wu->per_job_type[i].min_value * WORKER_CHART_DECIMAL_PRECISION));
            rrddim_set_by_pointer(wu->per_job_type[i].st, wu->per_job_type[i].rd_max, (collected_number)(wu->per_job_type[i].max_value * WORKER_CHART_DECIMAL_PRECISION));
            rrddim_set_by_pointer(wu->per_job_type[i].st, wu->per_job_type[i].rd_avg, (collected_number)(wu->per_job_type[i].sum_value / wu->per_job_type[i].count_value * WORKER_CHART_DECIMAL_PRECISION));

            rrdset_done(wu->per_job_type[i].st);
        }
    }

    // ----------------------------------------------------------------------
    // custom metric types WORKER_METRIC_INCREMENTAL

    {
        size_t i;
        for (i = 0; i <= wu->workers_max_job_id ; i++) {
            if(wu->per_job_type[i].type != WORKER_METRIC_INCREMENT && wu->per_job_type[i].type != WORKER_METRIC_INCREMENTAL_TOTAL)
                continue;

            if(!wu->per_job_type[i].count_value)
                continue;

            if(!wu->per_job_type[i].st) {
                size_t job_name_len = string_strlen(wu->per_job_type[i].name);
                if(job_name_len > RRD_ID_LENGTH_MAX) job_name_len = RRD_ID_LENGTH_MAX;

                char job_name_sanitized[job_name_len + 1];
                rrdset_strncpyz_name(job_name_sanitized, string2str(wu->per_job_type[i].name), job_name_len);

                char name[RRD_ID_LENGTH_MAX + 1];
                snprintfz(name, RRD_ID_LENGTH_MAX, "workers_%s_rate_%s", wu->name_lowercase, job_name_sanitized);

                char context[RRD_ID_LENGTH_MAX + 1];
                snprintf(context, RRD_ID_LENGTH_MAX, "netdata.workers.%s.rate.%s", wu->name_lowercase, job_name_sanitized);

                char title[1000 + 1];
                snprintf(title, 1000, "Netdata Workers %s rate of %s", wu->name_lowercase, string2str(wu->per_job_type[i].name));

                wu->per_job_type[i].st = rrdset_create_localhost(
                    "netdata"
                    , name
                    , NULL
                    , wu->family
                    , context
                    , title
                    , (wu->per_job_type[i].units)?string2str(wu->per_job_type[i].units):"rate"
                    , "netdata"
                    , "pulse"
                    , wu->priority + 10 + i
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
                );

                wu->per_job_type[i].rd_min = rrddim_add(wu->per_job_type[i].st, "min", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
                wu->per_job_type[i].rd_max = rrddim_add(wu->per_job_type[i].st, "max", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
                wu->per_job_type[i].rd_avg = rrddim_add(wu->per_job_type[i].st, "average", NULL, 1, WORKER_CHART_DECIMAL_PRECISION, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(wu->per_job_type[i].st, wu->per_job_type[i].rd_min, (collected_number)(wu->per_job_type[i].min_value * WORKER_CHART_DECIMAL_PRECISION));
            rrddim_set_by_pointer(wu->per_job_type[i].st, wu->per_job_type[i].rd_max, (collected_number)(wu->per_job_type[i].max_value * WORKER_CHART_DECIMAL_PRECISION));
            rrddim_set_by_pointer(wu->per_job_type[i].st, wu->per_job_type[i].rd_avg, (collected_number)(wu->per_job_type[i].sum_value / wu->per_job_type[i].count_value * WORKER_CHART_DECIMAL_PRECISION));

            rrdset_done(wu->per_job_type[i].st);
        }
    }
}

static void workers_utilization_reset_statistics(struct worker_utilization *wu) {
    Word_t idx = 0;
    for(struct worker_spinlocks *wusp = SPINLOCKS_FIRST(&wu->spinlocks, &idx);
         wusp;
         wusp = SPINLOCKS_NEXT(&wu->spinlocks, &idx)) {
        wusp->locks = 0;
        wusp->spins = 0;
    }

    wu->workers_registered = 0;
    wu->workers_busy = 0;
    wu->workers_total_busy_time = 0;
    wu->workers_total_duration = 0;
    wu->workers_total_jobs_started = 0;
    wu->workers_min_busy_time = WORKERS_MIN_PERCENT_DEFAULT;
    wu->workers_max_busy_time = 0;

    wu->workers_cpu_registered = 0;
    wu->workers_cpu_min = WORKERS_MIN_PERCENT_DEFAULT;
    wu->workers_cpu_max = 0;
    wu->workers_cpu_total = 0;

    size_t i;
    for(i = 0; i < WORKER_UTILIZATION_MAX_JOB_TYPES ;i++) {
        if(unlikely(!wu->name_lowercase)) {
            wu->name_lowercase = strdupz(wu->name);
            char *s = wu->name_lowercase;
            for( ; *s ; s++) *s = tolower(*s);
        }

        // copy the usage data
        wu->per_job_type[i].data[1].jobs_started = wu->per_job_type[i].data[0].jobs_started;
        wu->per_job_type[i].data[1].busy_time = wu->per_job_type[i].data[0].busy_time;

        // reset them for the next collection
        wu->per_job_type[i].data[0].jobs_started = 0;
        wu->per_job_type[i].data[0].busy_time = 0;

        wu->per_job_type[i].min_value = NAN;
        wu->per_job_type[i].max_value = NAN;
        wu->per_job_type[i].sum_value = NAN;
        wu->per_job_type[i].count_value = 0;
    }

    struct worker_thread *wt;
    for(wt = wu->threads; wt ; wt = wt->next) {
        wt->enabled = false;
        wt->cpu_enabled = false;
    }

    memset(wu->memory_calls, 0, sizeof(wu->memory_calls));
}

#define TASK_STAT_PREFIX "/proc/self/task/"
#define TASK_STAT_SUFFIX "/stat"

static int read_thread_cpu_time_from_proc_stat(pid_t pid __maybe_unused, kernel_uint_t *utime __maybe_unused, kernel_uint_t *stime __maybe_unused) {
#ifdef __linux__
    static char filename[sizeof(TASK_STAT_PREFIX) + sizeof(TASK_STAT_SUFFIX) + 20] = TASK_STAT_PREFIX;
    static size_t start_pos = sizeof(TASK_STAT_PREFIX) - 1;
    static procfile *ff = NULL;

    // construct the filename
    size_t end_pos = snprintfz(&filename[start_pos], 20, "%d", pid);
    strcpy(&filename[start_pos + end_pos], TASK_STAT_SUFFIX);

    // (re)open the procfile to the new filename
    bool set_quotes = (ff == NULL) ? true : false;
    ff = procfile_reopen(ff, filename, NULL, PROCFILE_FLAG_ERROR_ON_ERROR_LOG);
    if(unlikely(!ff)) return -1;

    if(set_quotes)
        procfile_set_open_close(ff, "(", ")");

    // read the entire file and split it to lines and words
    ff = procfile_readall(ff);
    if(unlikely(!ff)) return -1;

    // parse the numbers we are interested
    *utime = str2kernel_uint_t(procfile_lineword(ff, 0, 13));
    *stime = str2kernel_uint_t(procfile_lineword(ff, 0, 14));

    // leave the file open for the next iteration

    return 0;
#else
    // TODO: add here cpu time detection per thread, for FreeBSD and MacOS
    *utime = 0;
    *stime = 0;
    return 1;
#endif
}

static Pvoid_t workers_by_pid_JudyL_array = NULL;

static void workers_threads_cleanup(struct worker_utilization *wu) {
    struct worker_thread *t = wu->threads;
    while(t) {
        struct worker_thread *next = t->next;

        if(!t->enabled) {
            JudyLDel(&workers_by_pid_JudyL_array, t->pid, PJE0);
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wu->threads, t, prev, next);
            freez(t);
        }
        t = next;
    }
}

static struct worker_thread *worker_thread_find(struct worker_utilization *wu __maybe_unused, pid_t pid) {
    struct worker_thread *wt = NULL;

    Pvoid_t *PValue = JudyLGet(workers_by_pid_JudyL_array, pid, PJE0);
    if(PValue)
        wt = *PValue;

    return wt;
}

static struct worker_thread *worker_thread_create(struct worker_utilization *wu, pid_t pid) {
    struct worker_thread *wt;

    wt = (struct worker_thread *)callocz(1, sizeof(struct worker_thread));
    wt->pid = pid;

    Pvoid_t *PValue = JudyLIns(&workers_by_pid_JudyL_array, pid, PJE0);
    *PValue = wt;

    // link it
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(wu->threads, wt, prev, next);

    return wt;
}

static struct worker_thread *worker_thread_find_or_create(struct worker_utilization *wu, pid_t pid) {
    struct worker_thread *wt;
    wt = worker_thread_find(wu, pid);
    if(!wt) wt = worker_thread_create(wu, pid);

    return wt;
}

static void worker_utilization_charts_callback(void *ptr
                                               , pid_t pid
                                               , const char *thread_tag __maybe_unused
                                               , size_t max_job_id
                                               , size_t utilization_usec
                                               , size_t duration_usec
                                               , size_t jobs_started
                                               , size_t is_running
                                               , STRING **job_types_names
                                               , STRING **job_types_units
                                               , WORKER_METRIC_TYPE *job_types_metric_types
                                               , size_t *job_types_jobs_started
                                               , usec_t *job_types_busy_time
                                               , NETDATA_DOUBLE *job_types_custom_metrics
                                               , const char *spinlock_functions[]
                                               , size_t *spinlock_locks
                                               , size_t *spinlock_spins
                                               , uint64_t *memory_calls
) {
    struct worker_utilization *wu = (struct worker_utilization *)ptr;

    // find the worker_thread in the list
    struct worker_thread *wt = worker_thread_find_or_create(wu, pid);

    if(utilization_usec > duration_usec)
        utilization_usec = duration_usec;

    wt->enabled = true;
    wt->busy_time = utilization_usec;
    wt->jobs_started = jobs_started;

    wt->utime_old = wt->utime;
    wt->stime_old = wt->stime;
    wt->collected_time_old = wt->collected_time;

    if(max_job_id > wu->workers_max_job_id)
        wu->workers_max_job_id = max_job_id;

    wu->workers_total_busy_time += utilization_usec;
    wu->workers_total_duration += duration_usec;
    wu->workers_total_jobs_started += jobs_started;
    wu->workers_busy += is_running;
    wu->workers_registered++;

    double util = (double)utilization_usec * 100.0 / (double)duration_usec;
    if(util > wu->workers_max_busy_time)
        wu->workers_max_busy_time = util;

    if(util < wu->workers_min_busy_time)
        wu->workers_min_busy_time = util;

    // accumulate per job type statistics
    for(size_t i = 0; i <= max_job_id ;i++) {
        if(!wu->per_job_type[i].name && job_types_names[i])
            wu->per_job_type[i].name = string_dup(job_types_names[i]);

        if(!wu->per_job_type[i].units && job_types_units[i])
            wu->per_job_type[i].units = string_dup(job_types_units[i]);

        wu->per_job_type[i].type = job_types_metric_types[i];

        wu->per_job_type[i].data[0].jobs_started += job_types_jobs_started[i];
        wu->per_job_type[i].data[0].busy_time += job_types_busy_time[i];

        NETDATA_DOUBLE value = job_types_custom_metrics[i];
        if(netdata_double_isnumber(value)) {
            if(!wu->per_job_type[i].count_value) {
                wu->per_job_type[i].count_value = 1;
                wu->per_job_type[i].min_value = value;
                wu->per_job_type[i].max_value = value;
                wu->per_job_type[i].sum_value = value;
            }
            else {
                wu->per_job_type[i].count_value++;
                wu->per_job_type[i].sum_value += value;
                if(value < wu->per_job_type[i].min_value) wu->per_job_type[i].min_value = value;
                if(value > wu->per_job_type[i].max_value) wu->per_job_type[i].max_value = value;
            }
        }
    }

    // find its CPU utilization
    if((!read_thread_cpu_time_from_proc_stat(pid, &wt->utime, &wt->stime))) {
        wt->collected_time = now_realtime_usec();
        usec_t delta = wt->collected_time - wt->collected_time_old;

        double utime = (double)(wt->utime - wt->utime_old) / (double)system_hz * 100.0 * (double)USEC_PER_SEC / (double)delta;
        double stime = (double)(wt->stime - wt->stime_old) / (double)system_hz * 100.0 * (double)USEC_PER_SEC / (double)delta;
        double cpu = utime + stime;
        wt->cpu = cpu;
        wt->cpu_enabled = true;

        wu->workers_cpu_total += cpu;
        if(cpu < wu->workers_cpu_min) wu->workers_cpu_min = cpu;
        if(cpu > wu->workers_cpu_max) wu->workers_cpu_max = cpu;
    }
    wu->workers_cpu_registered += (wt->cpu_enabled) ? 1 : 0;

    // ----------------------------------------------------------------------------------------------------------------
    // spinlock contention

    // spinlocks
    for(size_t i = 0; i < WORKER_SPINLOCK_CONTENTION_FUNCTIONS && spinlock_functions[i] ;i++) {
        struct worker_spinlocks *wusp = SPINLOCKS_GET(&wu->spinlocks, (Word_t)spinlock_functions[i]);
        if(!wusp) {
            wusp = callocz(1, sizeof(*wusp));
            SPINLOCKS_SET(&wu->spinlocks, (Word_t)spinlock_functions[i], wusp);
        }
        wusp->locks += spinlock_locks[i];
        wusp->spins += spinlock_spins[i];

        wusp = SPINLOCKS_GET(&ALL_SPINLOCKS, (Word_t)spinlock_functions[i]);
        if(!wusp) {
            wusp = callocz(1, sizeof(*wusp));
            SPINLOCKS_SET(&ALL_SPINLOCKS, (Word_t)spinlock_functions[i], wusp);
        }
        wusp->locks += spinlock_locks[i];
        wusp->spins += spinlock_spins[i];
    }

    // ----------------------------------------------------------------------------------------------------------------
    // memory calls

    for(size_t i = 0; i < WORKERS_MEMORY_CALL_MAX ;i++)
        wu->memory_calls[i] += memory_calls[i];
}

void pulse_workers_cleanup(void) {
    int i, j;
    for(i = 0; all_workers_utilization[i].name ;i++) {
        struct worker_utilization *wu = &all_workers_utilization[i];

        if(wu->name_lowercase) {
            freez(wu->name_lowercase);
            wu->name_lowercase = NULL;
        }

        for(j = 0; j < WORKER_UTILIZATION_MAX_JOB_TYPES ;j++) {
            string_freez(wu->per_job_type[j].name);
            wu->per_job_type[j].name = NULL;

            string_freez(wu->per_job_type[j].units);
            wu->per_job_type[j].units = NULL;
        }

        // mark all threads as not enabled
        struct worker_thread *t;
        for(t = wu->threads; t ; t = t->next)
            t->enabled = false;

        // let the cleanup job free them
        workers_threads_cleanup(wu);
    }
}

void pulse_workers_do(bool extended) {
    if(!extended) return;

    static size_t iterations = 0;
    iterations++;

    Word_t idx = 0;
    for(struct worker_spinlocks *wusp = SPINLOCKS_FIRST(&ALL_SPINLOCKS, &idx);
         wusp;
         wusp = SPINLOCKS_NEXT(&ALL_SPINLOCKS, &idx)) {
        wusp->locks = 0;
        wusp->spins = 0;
    }

    for(int i = 0; all_workers_utilization[i].name ;i++) {
        workers_utilization_reset_statistics(&all_workers_utilization[i]);

        workers_foreach(all_workers_utilization[i].name, worker_utilization_charts_callback, &all_workers_utilization[i]);

        // skip the first iteration, so that we don't accumulate startup utilization to our charts
        if(likely(iterations > 1))
            workers_utilization_update_chart(&all_workers_utilization[i]);

        workers_threads_cleanup(&all_workers_utilization[i]);
    }

    workers_total_cpu_utilization_chart();
    workers_total_spinlock_contention_chart();
    workers_total_memory_calls_chart();
}
