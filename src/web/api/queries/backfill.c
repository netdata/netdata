// SPDX-License-Identifier: GPL-3.0-or-later

#include "backfill.h"

struct backfill_request {
    size_t rrdhost_receiver_state_id;
    RRDSET_ACQUIRED *rsa;
    uint32_t works;
    uint32_t successful;
    uint32_t failed;
    backfill_callback_t cb;
    void *data;
};

struct backfill_dim_work {
    RRDDIM_ACQUIRED *rda;
    struct backfill_request *br;
};

DEFINE_JUDYL_TYPED(BACKFILL, struct backfill_dim_work *);

static struct {
    struct completion completion;

    SPINLOCK spinlock;
    bool running;
    Word_t id;
    BACKFILL_JudyLSet queue;

} backfill_globals = {
    .spinlock = SPINLOCK_INITIALIZER,
    .queue = { 0 },
};

bool backfill_request_add(RRDSET *st, backfill_callback_t cb, void *data) {
    size_t dimensions = dictionary_entries(st->rrddim_root_index);
    size_t added = 0;
    struct backfill_dim_work *array[dimensions];

    bool rc = true;
    if(backfill_globals.running) {
        struct backfill_request *br = callocz(1, sizeof(*br));
        br->rrdhost_receiver_state_id =__atomic_load_n(&st->rrdhost->stream.rcv.status.state_id, __ATOMIC_RELAXED);
        br->rsa = rrdset_find_and_acquire(st->rrdhost, string2str(st->id));
        if(br->rsa) {
            br->cb = cb;
            br->data = data;

            RRDDIM *rd;
            rrddim_foreach_read(rd, st) {
                if(added >= dimensions)
                    break;

                if (!rrddim_option_check(rd, RRDDIM_OPTION_BACKFILLED_HIGH_TIERS)) {
                    struct backfill_dim_work *bdm = callocz(1, sizeof(*bdm));
                    bdm->rda = (RRDDIM_ACQUIRED *)dictionary_acquired_item_dup(st->rrddim_root_index, rd_dfe.item);
                    bdm->br = br;
                    br->works++;
                    array[added++] = bdm;
                }
            }
            rrddim_foreach_done(rd);
        }

        if (!added) {
            rrdset_acquired_release(br->rsa);
            freez(br);
            rc = false;
        }
    }
    else
        rc = false;

    spinlock_lock(&backfill_globals.spinlock);
    if(rc) {
        for(size_t i = 0; i < added ;i++)
            BACKFILL_SET(&backfill_globals.queue, backfill_globals.id++, array[i]);
    }
    spinlock_unlock(&backfill_globals.spinlock);

    if(rc)
        completion_mark_complete_a_job(&backfill_globals.completion);

    return rc;
}

bool backfill_execute(struct backfill_dim_work *bdm) {
    RRDSET *st = rrdset_acquired_to_rrdset(bdm->br->rsa);

    if(bdm->br->rrdhost_receiver_state_id !=__atomic_load_n(&st->rrdhost->stream.rcv.status.state_id, __ATOMIC_RELAXED))
        return false;

    RRDDIM *rd = rrddim_acquired_to_rrddim(bdm->rda);

    size_t success = 0;
    for (size_t tier = 1; tier < storage_tiers; tier++)
        if(backfill_tier_from_smaller_tiers(rd, tier, now_realtime_sec()))
            success++;

    if(success > 0)
        rrddim_option_set(rd, RRDDIM_OPTION_BACKFILLED_HIGH_TIERS);

    return success > 0;
}

static void backfill_dim_work_free(bool successful, struct backfill_dim_work *bdm) {
    struct backfill_request *br = bdm->br;

    if(successful)
        __atomic_add_fetch(&br->successful, 1, __ATOMIC_RELAXED);
    else
        __atomic_add_fetch(&br->failed, 1, __ATOMIC_RELAXED);

    uint32_t works = __atomic_sub_fetch(&br->works, 1, __ATOMIC_RELAXED);
    if(!works) {
        if(br->cb)
            br->cb(__atomic_load_n(&br->successful, __ATOMIC_RELAXED),
                   __atomic_load_n(&br->failed, __ATOMIC_RELAXED),
                   br->data);

        rrdset_acquired_release(br->rsa);
        freez(br);
    }

    rrddim_acquired_release(bdm->rda);
    freez(bdm);
}

void *backfill_worker_thread(void *ptr __maybe_unused) {
    size_t job_id = 0;
    while(!nd_thread_signaled_to_cancel() && service_running(SERVICE_COLLECTORS|SERVICE_STREAMING)) {
        spinlock_lock(&backfill_globals.spinlock);
        Word_t idx = 0;
        struct backfill_dim_work *bdm = BACKFILL_FIRST(&backfill_globals.queue, &idx);
        if(bdm)
            BACKFILL_DEL(&backfill_globals.queue, idx);
        spinlock_unlock(&backfill_globals.spinlock);

        if(bdm) {
            bool success = backfill_execute(bdm);
            backfill_dim_work_free(success, bdm);
            continue;
        }

        job_id = completion_wait_for_a_job_with_timeout(&backfill_globals.completion, job_id, 1000);
    }

    return NULL;
}

void *backfill_thread(void *ptr) {
    struct netdata_static_thread *static_thread = ptr;
    if(!static_thread) return NULL;

    nd_thread_tag_set("BACKFILL[0]");

    completion_init(&backfill_globals.completion);
    BACKFILL_INIT(&backfill_globals.queue);

    spinlock_lock(&backfill_globals.spinlock);
    backfill_globals.running = true;
    spinlock_unlock(&backfill_globals.spinlock);

    size_t threads = get_netdata_cpus();
    if(threads < 2) threads = 2;
    if(threads > 256) threads = 256;
    ND_THREAD *th[threads - 1];

    for(size_t t = 0; t < threads - 1 ;t++) {
        char tag[15];
        snprintfz(tag, sizeof(tag), "BACKFILL[%zu]", t + 1);
        th[t] = nd_thread_create(tag, NETDATA_THREAD_OPTION_JOINABLE, backfill_worker_thread, NULL);
    }

    backfill_worker_thread(NULL);
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    for(size_t t = 0; t < threads - 1 ;t++) {
        nd_thread_signal_cancel(th[t]);
        nd_thread_join(th[t]);
    }

    // cleanup
    spinlock_lock(&backfill_globals.spinlock);
    backfill_globals.running = false;
    Word_t idx = 0;
    for(struct backfill_dim_work *bdm = BACKFILL_FIRST(&backfill_globals.queue, &idx);
         bdm;
         bdm = BACKFILL_NEXT(&backfill_globals.queue, &idx)) {
        backfill_dim_work_free(false, bdm);
    }
    spinlock_unlock(&backfill_globals.spinlock);

    completion_destroy(&backfill_globals.completion);

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;

    return NULL;
}
