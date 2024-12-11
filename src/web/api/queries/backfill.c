// SPDX-License-Identifier: GPL-3.0-or-later

#include "backfill.h"

struct backfill_request {
    ND_UUID machine_guid;
    STRING *rrdset;
    uint32_t stream_receiver_state_id;
    backfill_callback_t cb;
    void *data;
};

DEFINE_JUDYL_TYPED(BACKFILL, struct backfill_request *);

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
    bool rc = true;
    spinlock_lock(&backfill_globals.spinlock);
    if(backfill_globals.running) {
        struct backfill_request *br = callocz(1, sizeof(*br));
        br->machine_guid = st->rrdhost->host_id;
        br->rrdset = string_dup(st->id);
        br->stream_receiver_state_id = __atomic_load_n(&st->rrdhost->stream.rcv.status.state_id, __ATOMIC_RELAXED);
        br->cb = cb;
        br->data = data;
        BACKFILL_SET(&backfill_globals.queue, backfill_globals.id++, br);
    }
    else
        rc = false;
    spinlock_unlock(&backfill_globals.spinlock);

    if(rc)
        completion_mark_complete_a_job(&backfill_globals.completion);

    return rc;
}

bool backfill_execute(struct backfill_request *br) {
    char uuid_str[UUID_STR_LEN];
    nd_uuid_unparse_lower(br->machine_guid.uuid, uuid_str);

    RRDHOST_ACQUIRED *rha = rrdhost_find_and_acquire(uuid_str);
    if(!rha) return false;

    RRDHOST *host = rrdhost_acquired_to_rrdhost(rha);
    if(br->stream_receiver_state_id != __atomic_load_n(&host->stream.rcv.status.state_id, __ATOMIC_RELAXED)) {
        rrdhost_acquired_release(rha);
        return false;
    }

    RRDSET_ACQUIRED *rsa = rrdset_find_and_acquire(host, string2str(br->rrdset));
    if(!rsa) {
        rrdhost_acquired_release(rha);
        return false;
    }

    RRDSET *st = rrdset_acquired_to_rrdset(rsa);
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if(!rrddim_option_check(rd, RRDDIM_OPTION_BACKFILLED_HIGH_TIERS)) {
            for (size_t tier = 1; tier < storage_tiers; tier++)
                backfill_tier_from_smaller_tiers(rd, tier, now_realtime_sec());
        }
        rrddim_option_set(rd, RRDDIM_OPTION_BACKFILLED_HIGH_TIERS);
    }
    rrddim_foreach_done(rd);

    rrdset_acquired_release(rsa);
    rrdhost_acquired_release(rha);
    return true;
}

static void backfill_request_free(bool successful, struct backfill_request *br) {
    if(br->cb)
        br->cb(successful, br->data);

    string_freez(br->rrdset);
    freez(br);
}

void *backfill_worker_thread(void *ptr __maybe_unused) {
    size_t job_id = 0;
    while(!nd_thread_signaled_to_cancel() && service_running(SERVICE_COLLECTORS|SERVICE_STREAMING)) {
        spinlock_lock(&backfill_globals.spinlock);
        Word_t idx = 0;
        struct backfill_request *br = BACKFILL_FIRST(&backfill_globals.queue, &idx);
        if(br)
            BACKFILL_DEL(&backfill_globals.queue, idx);
        spinlock_unlock(&backfill_globals.spinlock);

        if(br) {
            bool success = backfill_execute(br);
            backfill_request_free(success, br);
            continue;
        }

        job_id = completion_wait_for_a_job_with_timeout(&backfill_globals.completion, job_id, 100000);
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
    for(struct backfill_request *br = BACKFILL_FIRST(&backfill_globals.queue, &idx);
         br;
         br = BACKFILL_NEXT(&backfill_globals.queue, &idx)) {
        backfill_request_free(false, br);
    }
    spinlock_unlock(&backfill_globals.spinlock);

    completion_destroy(&backfill_globals.completion);

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;

    return NULL;
}
