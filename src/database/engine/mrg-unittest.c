// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-internals.h"
#include "rrdengine.h"

// Global dummy rrdengine_instances for tests
static struct rrdengine_instance test_ctx_0 = {0};
static struct rrdengine_instance test_ctx_1 = {0};
static struct rrdengine_instance test_ctx_tier[4] = { 0 }; // For stress test tiers

static int mrg_allocation_alignment_unittest(MRG *mrg) {
    if((uintptr_t)mrg % _Alignof(MRG) == 0)
        return 0;

    fprintf(stderr,
            "DBENGINE METRIC: MRG allocation is not aligned to %zu bytes\n",
            (size_t)_Alignof(MRG));
    return 1;
}

struct mrg_stress_entry {
    nd_uuid_t uuid;
    time_t after;
    time_t before;
};

struct mrg_stress {
    MRG *mrg;
    bool stop;
    size_t entries;
    struct mrg_stress_entry *array;
    size_t updates;
};

static void mrg_stress(void *ptr) {
    struct mrg_stress *t = ptr;
    MRG *mrg = t->mrg;

    ssize_t start = 0;
    ssize_t end = (ssize_t)t->entries;
    ssize_t step = 1;

    if(gettid_cached() % 2) {
        start = (ssize_t)t->entries - 1;
        end = -1;
        step = -1;
    }

    while(!__atomic_load_n(&t->stop, __ATOMIC_RELAXED) && !nd_thread_signaled_to_cancel()) {
        for (ssize_t i = start; i != end; i += step) {
            struct mrg_stress_entry *e = &t->array[i];

            time_t after = __atomic_sub_fetch(&e->after, 1, __ATOMIC_RELAXED);
            time_t before = __atomic_add_fetch(&e->before, 1, __ATOMIC_RELAXED);

            mrg_update_metric_retention_and_granularity_by_uuid(
                mrg, (Word_t)&test_ctx_0, &e->uuid, after, before, 1, before, NULL);

            __atomic_add_fetch(&t->updates, 1, __ATOMIC_RELAXED);
        }
    }
}

static int mrg_unittest_expect_samples_delta(
    struct rrdengine_instance *ctx,
    time_t first_time_s,
    time_t last_time_s,
    uint32_t update_every_s,
    bool expected_rc,
    uint64_t expected_samples,
    const char *name)
{
    uint64_t samples = UINT64_MAX;
    bool rc = rrdeng_retention_samples_delta(ctx, first_time_s, last_time_s, update_every_s, name, &samples);

    if(rc == expected_rc && samples == expected_samples)
        return 0;

    fprintf(stderr,
            "DBENGINE METRIC: samples delta test '%s' failed, expected rc=%s samples=%"PRIu64
            ", got rc=%s samples=%"PRIu64"\n",
            name,
            expected_rc ? "true" : "false",
            expected_samples,
            rc ? "true" : "false",
            samples);
    return 1;
}

static int mrg_unittest_expect_counter_sub(
    struct rrdengine_instance *ctx,
    uint64_t initial,
    uint64_t decrement,
    bool expected_rc,
    uint64_t expected_value,
    const char *name)
{
    uint64_t counter = initial;
    bool rc = rrdeng_atomic_uint64_sub_saturating(ctx, &counter, decrement, "unittest", name);

    if(rc == expected_rc && counter == expected_value)
        return 0;

    fprintf(stderr,
            "DBENGINE METRIC: counter subtraction test '%s' failed, expected rc=%s value=%"PRIu64
            ", got rc=%s value=%"PRIu64"\n",
            name,
            expected_rc ? "true" : "false",
            expected_value,
            rc ? "true" : "false",
            counter);
    return 1;
}

static int dbengine_accounting_helpers_unittest(void) {
    int errors = 0;
    struct rrdengine_instance ctx = {0};
    ctx.config.tier = 1;

    errors += mrg_unittest_expect_samples_delta(&ctx, 10, 70, 10, true, 6, "normal positive interval");
    errors += mrg_unittest_expect_samples_delta(&ctx, 10, 10, 10, false, 0, "equal timestamps");
    errors += mrg_unittest_expect_samples_delta(&ctx, 0, 10, 10, false, 0, "missing first timestamp");
    errors += mrg_unittest_expect_samples_delta(&ctx, 10, 70, 0, false, 0, "missing update every");

#ifndef NETDATA_INTERNAL_CHECKS
    errors += mrg_unittest_expect_samples_delta(&ctx, 70, 10, 10, false, 0, "reversed interval");
#endif

    errors += mrg_unittest_expect_counter_sub(&ctx, 9, 4, true, 5, "normal decrement");
    errors += mrg_unittest_expect_counter_sub(&ctx, 9, 0, true, 9, "zero decrement");

#ifndef NETDATA_INTERNAL_CHECKS
    errors += mrg_unittest_expect_counter_sub(&ctx, 3, 9, false, 0, "underflow saturation");
#endif

    ctx.atomic.metrics = 11;
    ctx.atomic.samples = 22;
    rrdeng_reset_accounting_if_fresh(&ctx, false);
    if(ctx.atomic.metrics != 11 || ctx.atomic.samples != 22) {
        fprintf(stderr,
                "DBENGINE METRIC: global-context accounting reset policy failed, expected 11/22 got %"PRIu64"/%"PRIu64"\n",
                ctx.atomic.metrics,
                ctx.atomic.samples);
        errors++;
    }

    rrdeng_reset_accounting_if_fresh(&ctx, true);
    if(ctx.atomic.metrics != 0 || ctx.atomic.samples != 0) {
        fprintf(stderr,
                "DBENGINE METRIC: fresh-context accounting reset policy failed, expected 0/0 got %"PRIu64"/%"PRIu64"\n",
                ctx.atomic.metrics,
                ctx.atomic.samples);
        errors++;
    }

    return errors;
}

#ifndef NETDATA_INTERNAL_CHECKS
static int mrg_metrics_delete_underflow_unittest(MRG *mrg) {
    nd_uuid_t uuid;
    uuid_generate(uuid);

    MRG_ENTRY entry = {
        .uuid = &uuid,
        .section = (Word_t)&test_ctx_0,
        .first_time_s = 0,
        .last_time_s = 0,
        .latest_update_every_s = 1,
    };

    bool added = false;
    METRIC *metric = mrg_metric_add_and_acquire(mrg, entry, &added);
    if(!metric || !added) {
        fprintf(stderr, "DBENGINE METRIC: failed to add no-retention metric for delete-underflow test\n");
        return 1;
    }

    __atomic_store_n(&test_ctx_0.atomic.metrics, 0, __ATOMIC_RELAXED);

    bool deleted = mrg_metric_release_and_delete(mrg, metric);
    uint64_t metrics = __atomic_load_n(&test_ctx_0.atomic.metrics, __ATOMIC_RELAXED);

    if(!deleted || metrics != 0) {
        fprintf(stderr,
                "DBENGINE METRIC: delete-underflow test failed, expected deleted=true metrics=0, got deleted=%s metrics=%"PRIu64"\n",
                deleted ? "true" : "false",
                metrics);
        return 1;
    }

    return 0;
}
#endif

static int mrg_destroy_referenced_metric_unittest(void) {
    int errors = 0;

    MRG *mrg = mrg_create_for_unittest();
    errors += mrg_allocation_alignment_unittest(mrg);
    nd_uuid_t uuid;
    uuid_generate(uuid);

    MRG_ENTRY entry = {
        .uuid = &uuid,
        .section = (Word_t)&test_ctx_0,
        .first_time_s = 0,
        .last_time_s = 0,
        .latest_update_every_s = 1,
    };

    bool added = false;
    METRIC *metric = mrg_metric_add_and_acquire(mrg, entry, &added);
    if(!metric || !added) {
        fprintf(stderr, "DBENGINE METRIC: failed to add metric for referenced-destroy test\n");
        mrg_destroy(mrg);
        return 1;
    }

    size_t referenced = mrg_destroy(mrg);
    if(referenced != 1) {
        fprintf(stderr,
                "DBENGINE METRIC: referenced-destroy test failed, expected 1 referenced metric, got %zu\n",
                referenced);
        errors++;
    }

    if(!mrg_metric_release(mrg, metric)) {
        fprintf(stderr, "DBENGINE METRIC: referenced-destroy release did not delete metric\n");
        errors++;
    }

    referenced = mrg_destroy(mrg);
    if(referenced != 0) {
        fprintf(stderr,
                "DBENGINE METRIC: referenced-destroy cleanup failed, expected 0 referenced metrics, got %zu\n",
                referenced);
        errors++;
    }

    return errors;
}

static int mrg_entries_acquired_counter_unittest(MRG *mrg) {
    int errors = 0;

    nd_uuid_t uuid;
    uuid_generate(uuid);

    struct mrg_statistics baseline;
    mrg_get_statistics(mrg, &baseline);

    MRG_ENTRY entry = {
        .uuid = &uuid,
        .section = (Word_t)&test_ctx_0,
        .first_time_s = 10,
        .last_time_s = 20,
        .latest_update_every_s = 1,
    };

    bool added = false;
    METRIC *metric = mrg_metric_add_and_acquire(mrg, entry, &added);
    if(!metric || !added) {
        fprintf(stderr, "DBENGINE METRIC: failed to add retained metric for acquired-counter test\n");
        return 1;
    }

    struct mrg_statistics stats;
    mrg_get_statistics(mrg, &stats);
    if(stats.entries != baseline.entries + 1 ||
       stats.entries_acquired != baseline.entries_acquired + 1 ||
       stats.current_references != baseline.current_references + 1) {
        fprintf(stderr,
                "DBENGINE METRIC: acquired-counter add failed, expected entries/acquired/references %zu/%zd/%zd, got %zu/%zd/%zd\n",
                baseline.entries + 1,
                baseline.entries_acquired + 1,
                baseline.current_references + 1,
                stats.entries,
                stats.entries_acquired,
                stats.current_references);
        errors++;
    }

    bool deleted = mrg_metric_release(mrg, metric);
    if(deleted) {
        fprintf(stderr, "DBENGINE METRIC: acquired-counter retained metric was deleted on release\n");
        errors++;
    }

    mrg_get_statistics(mrg, &stats);
    if(stats.entries_acquired != baseline.entries_acquired ||
       stats.current_references != baseline.current_references) {
        fprintf(stderr,
                "DBENGINE METRIC: acquired-counter release failed, expected acquired/references %zd/%zd, got %zd/%zd\n",
                baseline.entries_acquired,
                baseline.current_references,
                stats.entries_acquired,
                stats.current_references);
        errors++;
    }

    metric = mrg_metric_get_and_acquire_by_uuid(mrg, &uuid, entry.section);
    if(!metric) {
        fprintf(stderr, "DBENGINE METRIC: acquired-counter retained metric not found after release\n");
        return errors + 1;
    }

    mrg_get_statistics(mrg, &stats);
    if(stats.entries_acquired != baseline.entries_acquired + 1 ||
       stats.current_references != baseline.current_references + 1) {
        fprintf(stderr,
                "DBENGINE METRIC: acquired-counter reacquire failed, expected acquired/references %zd/%zd, got %zd/%zd\n",
                baseline.entries_acquired + 1,
                baseline.current_references + 1,
                stats.entries_acquired,
                stats.current_references);
        errors++;
    }

    deleted = mrg_metric_release(mrg, metric);
    if(deleted) {
        fprintf(stderr, "DBENGINE METRIC: acquired-counter reacquired retained metric was deleted on release\n");
        errors++;
    }

    mrg_get_statistics(mrg, &stats);
    if(stats.entries_acquired != baseline.entries_acquired ||
       stats.current_references != baseline.current_references) {
        fprintf(stderr,
                "DBENGINE METRIC: acquired-counter second release failed, expected acquired/references %zd/%zd, got %zd/%zd\n",
                baseline.entries_acquired,
                baseline.current_references,
                stats.entries_acquired,
                stats.current_references);
        errors++;
    }

    metric = mrg_metric_get_and_acquire_by_uuid(mrg, &uuid, entry.section);
    if(!metric) {
        fprintf(stderr, "DBENGINE METRIC: acquired-counter retained metric not found for cleanup\n");
        return errors + 1;
    }

    mrg_metric_clear_retention(mrg, metric);
    deleted = mrg_metric_release_and_delete(mrg, metric);
    if(!deleted) {
        fprintf(stderr, "DBENGINE METRIC: acquired-counter cleanup did not delete metric\n");
        errors++;
    }

    mrg_get_statistics(mrg, &stats);
    if(stats.entries != baseline.entries ||
       stats.entries_acquired != baseline.entries_acquired ||
       stats.current_references != baseline.current_references) {
        fprintf(stderr,
                "DBENGINE METRIC: acquired-counter cleanup failed, expected entries/acquired/references %zu/%zd/%zd, got %zu/%zd/%zd\n",
                baseline.entries,
                baseline.entries_acquired,
                baseline.current_references,
                stats.entries,
                stats.entries_acquired,
                stats.current_references);
        errors++;
    }

    return errors;
}

// ----------------------------------------------------------------------------
// Concurrent lookup-vs-delete on the uuid path.
//
// mrg_metric_get_and_acquire_by_uuid() resolves the uuid with uuidmap_peek_id(),
// which returns an id it holds NO reference on. That is only safe because every
// METRIC owns a uuidmap reference for its whole lifetime, so an id that resolves
// to a metric is necessarily still alive, and because ids are unique for the
// lifetime of the uuidmap, so a stale id can only miss -- never alias a different
// uuid. (uuidmap_destroy() restarts the sequence, but it cannot succeed while a
// METRIC holds a reference.)
//
// This exercises exactly that window: readers look metrics up by uuid while
// writers add and delete metrics for the same uuid pool. Any mismatch between the
// uuid asked for and the uuid of the metric returned means the id aliased, which
// is the failure mode the contract forbids.

#define MRG_UUID_RACE_ENTRIES 512
#define MRG_UUID_RACE_SECS 3
#define MRG_UUID_RACE_READERS 4
#define MRG_UUID_RACE_WRITERS 4

struct mrg_uuid_race {
    MRG *mrg;
    nd_uuid_t *uuids;
    size_t entries;
    size_t churn_from;      // uuids below this index are stable, above they churn
    bool stop;
    // counted per region: an aggregate hit counter cannot distinguish "a reader
    // queried a churning uuid while it was being deleted" from "a reader only
    // ever hit the stable half", which is the whole point of the test
    size_t stable_hits;
    size_t stable_misses;
    size_t churn_hits;
    size_t churn_misses;
    size_t mismatches;
    size_t deletes;        // deletions actually performed, by whichever side won
};

struct mrg_uuid_race_writer_ctx {
    struct mrg_uuid_race *t;
    size_t from, to;            // this writer's exclusive slice of the churn region
};

static void mrg_uuid_race_writer(void *ptr) {
    struct mrg_uuid_race_writer_ctx *w = ptr;
    struct mrg_uuid_race *t = w->t;

    size_t span = w->to - w->from;
    METRIC **held = callocz(span, sizeof(METRIC *));
    size_t i = 0;

    // Each writer owns its slice exclusively, and keeps every metric it creates
    // alive until its next sweep over that slot. Without that window the
    // add/delete pair is back-to-back and readers can only ever miss - the churn
    // region then contributes nothing and the race is not actually exercised.
    while(!__atomic_load_n(&t->stop, __ATOMIC_RELAXED)) {
        i = (i + 1) % span;
        size_t idx = w->from + i;

        if(held[i]) {
            // it has been visible to readers for a full sweep - retire it now
            if(mrg_metric_release_and_delete(t->mrg, held[i]))
                __atomic_add_fetch(&t->deletes, 1, __ATOMIC_RELAXED);
            held[i] = NULL;
        }

        bool added = false;
        // no retention on purpose: metric_release() only deletes a metric that
        // has none (acquired_metric_has_retention()), and this test needs the
        // delete path to actually run
        MRG_ENTRY entry = {
            .uuid = &t->uuids[idx],
            .section = (Word_t)&test_ctx_0,
            .first_time_s = 0,
            .last_time_s = 0,
            .latest_update_every_s = 0,
        };
        held[i] = mrg_metric_add_and_acquire(t->mrg, entry, &added);
    }

    for(size_t k = 0; k < span ;k++) {
        if(held[k] && mrg_metric_release_and_delete(t->mrg, held[k]))
            __atomic_add_fetch(&t->deletes, 1, __ATOMIC_RELAXED);
    }
    freez(held);
}

static void mrg_uuid_race_reader(void *ptr) {
    struct mrg_uuid_race *t = ptr;
    size_t i = 0;

    while(!__atomic_load_n(&t->stop, __ATOMIC_RELAXED)) {
        i = (i + 1) % t->entries;

        bool churned = (i >= t->churn_from);

        METRIC *metric = mrg_metric_get_and_acquire_by_uuid(t->mrg, &t->uuids[i], (Word_t)&test_ctx_0);
        if(!metric) {
            // a miss is legitimate in the churn region - the writers delete constantly.
            // In the stable region it is a defect: those metrics are never deleted.
            __atomic_add_fetch(churned ? &t->churn_misses : &t->stable_misses, 1, __ATOMIC_RELAXED);
            continue;
        }

        // we hold the metric, so its uuidmap entry cannot go away underneath us
        nd_uuid_t *got = mrg_metric_uuid(t->mrg, metric);
        if(!got || uuid_compare(*got, t->uuids[i]) != 0)
            __atomic_add_fetch(&t->mismatches, 1, __ATOMIC_RELAXED);
        else
            __atomic_add_fetch(churned ? &t->churn_hits : &t->stable_hits, 1, __ATOMIC_RELAXED);

        // If the writer retired this metric while we held it, the writer's
        // release could not delete it and OUR release performs the deletion.
        // Counting only the writer side would undercount, and could report zero
        // deletions - a false failure - in a run where readers always won.
        if(mrg_metric_release(t->mrg, metric))
            __atomic_add_fetch(&t->deletes, 1, __ATOMIC_RELAXED);
    }
}

static int mrg_uuid_lookup_delete_race_unittest(void) {
    fprintf(stderr, "\nTesting concurrent MRG lookup-by-uuid vs delete (%d readers, %d writers, %ds)...\n",
            MRG_UUID_RACE_READERS, MRG_UUID_RACE_WRITERS, MRG_UUID_RACE_SECS);

    int errors = 0;

    // isolated MRG so the churn cannot disturb the accounting the other tests check
    MRG *mrg = mrg_create_for_unittest();

    struct mrg_uuid_race t = {
        .mrg = mrg,
        .entries = MRG_UUID_RACE_ENTRIES,
        .uuids = callocz(MRG_UUID_RACE_ENTRIES, sizeof(nd_uuid_t)),
    };
    for(size_t i = 0; i < t.entries ;i++)
        uuid_generate_random(t.uuids[i]);

    // Seed the lower half with metrics that HAVE retention. metric_release()
    // refuses to delete those, so they stay put and give the readers something
    // they must reliably find; the upper half is left to the writers to churn.
    // Without both halves the test proves nothing: all-stable means the delete
    // path never runs, all-churn means the lookup path never succeeds.
    t.churn_from = t.entries / 2;
    for(size_t i = 0; i < t.churn_from ;i++) {
        bool added = false;
        MRG_ENTRY entry = {
            .uuid = &t.uuids[i],
            .section = (Word_t)&test_ctx_0,
            .first_time_s = 2,
            .last_time_s = 3,
            .latest_update_every_s = 1,
        };
        METRIC *m = mrg_metric_add_and_acquire(mrg, entry, &added);
        if(m) mrg_metric_release(mrg, m);
    }

    ND_THREAD *th[MRG_UUID_RACE_READERS + MRG_UUID_RACE_WRITERS];
    struct mrg_uuid_race_writer_ctx wctx[MRG_UUID_RACE_WRITERS];
    size_t n = 0;
    size_t churn_span = t.entries - t.churn_from;
    for(size_t i = 0; i < MRG_UUID_RACE_WRITERS ;i++) {
        char buf[15 + 1];
        snprintfz(buf, sizeof(buf) - 1, "MRGRW[%zu]", i);
        wctx[i].t = &t;
        wctx[i].from = t.churn_from + (churn_span * i) / MRG_UUID_RACE_WRITERS;
        wctx[i].to   = t.churn_from + (churn_span * (i + 1)) / MRG_UUID_RACE_WRITERS;
        th[n++] = nd_thread_create(buf, NETDATA_THREAD_OPTION_DONT_LOG, mrg_uuid_race_writer, &wctx[i]);
    }
    for(size_t i = 0; i < MRG_UUID_RACE_READERS ;i++) {
        char buf[15 + 1];
        snprintfz(buf, sizeof(buf) - 1, "MRGRD[%zu]", i);
        th[n++] = nd_thread_create(buf, NETDATA_THREAD_OPTION_DONT_LOG, mrg_uuid_race_reader, &t);
    }

    sleep_usec(MRG_UUID_RACE_SECS * USEC_PER_SEC);
    __atomic_store_n(&t.stop, true, __ATOMIC_RELAXED);

    for(size_t i = 0; i < n ;i++)
        nd_thread_join(th[i]);

    fprintf(stderr, "  stable: %zu hit / %zu miss   churn: %zu hit / %zu miss   deletes: %zu   MISMATCHES: %zu\n",
            t.stable_hits, t.stable_misses, t.churn_hits, t.churn_misses, t.deletes, t.mismatches);

    if(t.mismatches) {
        fprintf(stderr, "ERROR: %zu lookups returned a metric for the wrong UUID\n", t.mismatches);
        errors++;
    }

    // stable metrics are never deleted, so they must always be findable
    if(t.stable_misses) {
        fprintf(stderr, "ERROR: %zu lookups missed a metric that is never deleted\n", t.stable_misses);
        errors++;
    }
    if(!t.stable_hits) {
        fprintf(stderr, "ERROR: no stable lookup succeeded - the readers did not run\n");
        errors++;
    }

    // The real requirement: readers must have queried CHURNING uuids and seen
    // them both present and absent. Hits alone could all come from the stable
    // half, and deletes alone say nothing about what the readers were doing --
    // neither would prove a lookup overlapped an in-flight deletion.
    if(!t.churn_hits) {
        fprintf(stderr, "ERROR: no churn-region hit - readers never caught a churning metric alive\n");
        errors++;
    }
    if(!t.churn_misses) {
        fprintf(stderr, "ERROR: no churn-region miss - readers never caught a churning metric deleted\n");
        errors++;
    }
    if(!t.deletes) {
        fprintf(stderr, "ERROR: no deletion happened - the race was never exercised\n");
        errors++;
    }

    freez(t.uuids);

    size_t referenced = mrg_destroy(mrg);
    if(referenced) {
        fprintf(stderr, "ERROR: %zu metrics still referenced after the race (leaked reference)\n", referenced);
        errors++;
    }

    if(errors)
        fprintf(stderr, "MRG uuid lookup/delete race test: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "MRG uuid lookup/delete race test: OK\n");

    return errors;
}

// Deterministic proof of the property the race test can only sample: an id
// borrowed by uuidmap_peek_id() and then invalidated must MISS, never alias.
//
// This reproduces the exact handoff mrg_metric_get_and_acquire_by_uuid() performs
// -- peek the id, then use it as a bare MRG key -- but drives the invalidation by
// hand instead of hoping threads interleave, so it cannot pass vacuously.
static int mrg_stale_peeked_id_unittest(void) {
    fprintf(stderr, "\nTesting stale peeked id cannot alias (deterministic)...\n");
    int errors = 0;

    MRG *mrg = mrg_create_for_unittest();

    nd_uuid_t victim;
    uuid_generate_random(victim);

    // 1. a metric exists for `victim`, so its uuid is in the uuidmap
    bool added = false;
    MRG_ENTRY entry = {
        .uuid = &victim,
        .section = (Word_t)&test_ctx_0,
        .first_time_s = 0,          // no retention, so it is deletable on release
        .last_time_s = 0,
        .latest_update_every_s = 0,
    };
    METRIC *metric = mrg_metric_add_and_acquire(mrg, entry, &added);
    if(!metric) {
        fprintf(stderr, "ERROR: cannot add the victim metric\n");
        mrg_destroy(mrg);
        return 1;
    }

    // 2. borrow the id exactly as the production lookup does - no reference taken
    UUIDMAP_ID borrowed = uuidmap_peek_id(victim);
    if(!borrowed) {
        // Without a borrowed id there is nothing that can go stale, and the
        // assertion below would look up id 0 and pass vacuously. Stop here.
        fprintf(stderr, "ERROR: peek could not resolve a uuid that has a metric\n");
        (void) mrg_metric_release_and_delete(mrg, metric);
        (void)mrg_destroy(mrg);
        fprintf(stderr, "Stale peeked id test: 1 ERROR(S)\n");
        return 1;
    }

    // 3. delete the metric. metric_release() also drops the METRIC's uuidmap
    //    reference, so the uuidmap entry goes away and `borrowed` becomes stale.
    if(!mrg_metric_release_and_delete(mrg, metric)) {
        fprintf(stderr, "ERROR: the victim metric was not deleted\n");
        errors++;
    }

    if(uuidmap_peek_id(victim) != 0) {
        fprintf(stderr, "ERROR: peek still resolves a uuid whose only metric was deleted\n");
        errors++;
    }

    // 4. churn plenty of new metrics through the same section. If ids were ever
    //    recycled, one of these would land on `borrowed`.
    enum { DECOYS = 4096 };
    for(size_t i = 0; i < DECOYS ;i++) {
        nd_uuid_t decoy;
        uuid_generate_random(decoy);
        decoy[15] = victim[15];     // same partition => same id sequence
        bool decoy_added = false;
        MRG_ENTRY d = {
            .uuid = &decoy,
            .section = (Word_t)&test_ctx_0,
            .first_time_s = 2,
            .last_time_s = 3,
            .latest_update_every_s = 1,
        };
        METRIC *dm = mrg_metric_add_and_acquire(mrg, d, &decoy_added);
        if(dm) mrg_metric_release(mrg, dm);
    }

    // 5. THE assertion: the stale id must not resolve to anything at all.
    //    A non-NULL result here is the aliasing bug the contract forbids.
    METRIC *ghost = mrg_metric_get_and_acquire_by_id(mrg, borrowed, (Word_t)&test_ctx_0);
    if(ghost) {
        nd_uuid_t *ghost_uuid = mrg_metric_uuid(mrg, ghost);
        char b[UUID_STR_LEN] = "?";
        if(ghost_uuid) uuid_unparse_lower(*ghost_uuid, b);
        fprintf(stderr, "ERROR: stale borrowed id %u resolved to a metric (uuid %s) - it aliased\n",
                borrowed, b);
        errors++;
        mrg_metric_release(mrg, ghost);
    }

    // A non-zero result means metrics were still referenced and the MRG was left
    // alive - i.e. this test leaked a reference. Ignoring it would let a cleanup
    // regression pass silently.
    size_t referenced = mrg_destroy(mrg);
    if(referenced) {
        fprintf(stderr, "ERROR: %zu metrics still referenced - the MRG was not destroyed\n", referenced);
        errors++;
    }

    if(errors)
        fprintf(stderr, "Stale peeked id test: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "Stale peeked id test: OK\n");

    return errors;
}

int mrg_unittest(void) {
    int errors = dbengine_accounting_helpers_unittest();
    errors += mrg_destroy_referenced_metric_unittest();
    errors += mrg_stale_peeked_id_unittest();
    errors += mrg_uuid_lookup_delete_race_unittest();

    // Use mrg_create_for_unittest to avoid pre-loaded metrics that block deletion
    MRG *mrg = mrg_create_for_unittest();
    errors += mrg_allocation_alignment_unittest(mrg);

#ifndef NETDATA_INTERNAL_CHECKS
    errors += mrg_metrics_delete_underflow_unittest(mrg);
#endif
    errors += mrg_entries_acquired_counter_unittest(mrg);

    if(errors) {
        mrg_destroy(mrg);
        return errors;
    }

    METRIC *m1_t0, *m2_t0, *m3_t0, *m4_t0;
    METRIC *m1_t1, *m2_t1, *m3_t1, *m4_t1;
    bool ret;

    nd_uuid_t test_uuid;
    uuid_generate(test_uuid);
    MRG_ENTRY entry = {
        .uuid = &test_uuid,
        .section = (Word_t)&test_ctx_0,
        .first_time_s = 2,
        .last_time_s = 3,
        .latest_update_every_s = 4,
    };
    m1_t0 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(!ret)
        fatal("DBENGINE METRIC: failed to add metric");

    // add the same metric again
    m2_t0 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(m2_t0 != m1_t0)
        fatal("DBENGINE METRIC: adding the same metric twice, does not return the same pointer");
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice");

    m3_t0 = mrg_metric_get_and_acquire_by_uuid(mrg, entry.uuid, entry.section);
    if(m3_t0 != m1_t0)
        fatal("DBENGINE METRIC: cannot find the metric added");

    // add the same metric again
    m4_t0 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(m4_t0 != m1_t0)
        fatal("DBENGINE METRIC: adding the same metric twice, does not return the same pointer");
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice");

    // add the same metric in another section
    entry.section = (Word_t)&test_ctx_1;
    m1_t1 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(!ret)
        fatal("DBENGINE METRIC: failed to add metric in section %zu", (size_t)entry.section);

    // add the same metric again
    m2_t1 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(m2_t1 != m1_t1)
        fatal("DBENGINE METRIC: adding the same metric twice (section %zu), does not return the same pointer", (size_t)entry.section);
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice in (section 0)");

    m3_t1 = mrg_metric_get_and_acquire_by_uuid(mrg, entry.uuid, entry.section);
    if(m3_t1 != m1_t1)
        fatal("DBENGINE METRIC: cannot find the metric added (section %zu)", (size_t)entry.section);

    // add the same metric again in section 1
    m4_t1 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(m4_t1 != m1_t1)
        fatal("DBENGINE METRIC: adding the same metric twice (section %zu), does not return the same pointer", (size_t)entry.section);
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice in (section %zu)", (size_t)entry.section);

    // Release all references to these initial test metrics
    mrg_metric_release(mrg, m2_t0);
    mrg_metric_release(mrg, m3_t0);
    mrg_metric_release(mrg, m4_t0);
    mrg_metric_release(mrg, m1_t0);

    mrg_metric_release(mrg, m2_t1);
    mrg_metric_release(mrg, m3_t1);
    mrg_metric_release(mrg, m4_t1);
    mrg_metric_release(mrg, m1_t1);

    size_t entries = 100000;  // Reduced from 1M to make deletion test feasible
    size_t threads = _countof(mrg->index) / 3 + 1;
    size_t tiers = 3;
    size_t run_for_secs = 5;
    fprintf(stderr, "preparing stress test of %zu entries...\n", entries);
    struct mrg_stress t = {
        .mrg = mrg,
        .entries = entries,
        .array = callocz(entries, sizeof(struct mrg_stress_entry)),
    };

    time_t now = max_acceptable_collected_time();
    for(size_t i = 0; i < entries ;i++) {
        uuid_generate_random(t.array[i].uuid);
        t.array[i].after = now / 3;
        t.array[i].before = now / 2;
    }
    fprintf(stderr, "stress test is populating MRG with 3 tiers...\n");
    for(size_t i = 0; i < entries ;i++) {
        struct mrg_stress_entry *e = &t.array[i];
        for(size_t tier = 1; tier <= tiers ;tier++) {
            mrg_update_metric_retention_and_granularity_by_uuid(
                mrg, (Word_t)&test_ctx_tier[tier],
                &e->uuid,
                e->after,
                e->before,
                1,
                e->before, NULL);
        }
    }
    fprintf(stderr, "stress test ready to run...\n");

    usec_t started_ut = now_monotonic_usec();

    ND_THREAD *th[threads];
    for(size_t i = 0; i < threads ; i++) {
        char buf[15 + 1];
        snprintfz(buf, sizeof(buf) - 1, "TH[%zu]", i);
        th[i] = nd_thread_create(buf, NETDATA_THREAD_OPTION_DONT_LOG, mrg_stress, &t);
    }

    sleep_usec(run_for_secs * USEC_PER_SEC);
    __atomic_store_n(&t.stop, true, __ATOMIC_RELAXED);

    for(size_t i = 0; i < threads ; i++)
        nd_thread_signal_cancel(th[i]);

    for(size_t i = 0; i < threads ; i++)
        nd_thread_join(th[i]);

    usec_t ended_ut = now_monotonic_usec();

    struct mrg_statistics stats;
    mrg_get_statistics(mrg, &stats);

    fprintf(stderr, "DBENGINE METRIC: did %zu additions, %zu duplicate additions, "
                     "%zu deletions, %zu wrong deletions, "
                     "%zu successful searches, %zu wrong searches, "
                     "in %"PRIu64" usecs\n",
                     stats.additions, stats.additions_duplicate,
                     stats.deletions, stats.delete_misses,
                     stats.search_hits, stats.search_misses,
                     ended_ut - started_ut);

    fprintf(stderr, "DBENGINE METRIC: updates performance: %0.2fk/sec total, %0.2fk/sec/thread\n",
                     (double)t.updates / (double)((ended_ut - started_ut) / USEC_PER_SEC) / 1000.0,
                     (double)t.updates / (double)((ended_ut - started_ut) / USEC_PER_SEC) / 1000.0 / threads);

    fprintf(stderr, "DBENGINE METRIC: addition rate: %0.2fk/sec, search rate: %0.2fk/sec, deletion rate: %zu/%zu attempted\n",
                     (double)stats.additions / (double)((ended_ut - started_ut) / USEC_PER_SEC) / 1000.0,
                     (double)(stats.search_hits + stats.search_misses) / (double)((ended_ut - started_ut) / USEC_PER_SEC) / 1000.0,
                     stats.deletions, stats.deletions + stats.delete_misses);

    // Phase 3: Measure final statistics
    struct mrg_statistics final_stats;
    mrg_get_statistics(mrg, &final_stats);
    fprintf(stderr, "DBENGINE METRIC: final MRG state - %zu entries, %zd acquired\n",
                     final_stats.entries, final_stats.entries_acquired);

    freez(t.array);

    // Destroy MRG (will handle cleanup of any remaining metrics)
    size_t leaked = mrg_destroy(mrg);
    if(leaked > 0) {
        fprintf(stderr, "DBENGINE METRIC: warning - %zu metrics still referenced during destroy\n", leaked);
    } else {
        fprintf(stderr, "DBENGINE METRIC: all metrics properly cleaned up\n");
    }

    fprintf(stderr, "DBENGINE METRIC: all tests passed!\n");

    return 0;
}

// ============================================================================
// MRG Retention Benchmark
// ============================================================================

#define MRG_BENCH_MAX_THREADS 64
#define MRG_BENCH_TEST_DURATION_SEC 1
#define MRG_BENCH_STOP_SIGNAL UINT64_MAX
#define MRG_BENCH_MAX_CONFIGS 12

typedef struct {
    uint64_t operations;
    uint64_t violations;        // consistency check failures
    usec_t test_time;
    volatile int ready;
} mrg_bench_thread_stats_t;

typedef struct {
    netdata_cond_t cond;
    netdata_mutex_t cond_mutex;
    uint64_t run_flag;  // accessed by multiple threads via __atomic builtins
} mrg_bench_thread_control_t;

typedef enum {
    MRG_BENCH_READER,
    MRG_BENCH_WRITER
} mrg_bench_thread_type_t;

typedef struct {
    int thread_id;
    mrg_bench_thread_type_t type;
    MRG *mrg;
    METRIC *metric;
    mrg_bench_thread_stats_t *stats;
    mrg_bench_thread_control_t *control;
    ND_THREAD *thread;
} mrg_bench_thread_context_t;

typedef struct {
    double reader_ops_per_sec[MRG_BENCH_MAX_CONFIGS];
    double writer_ops_per_sec[MRG_BENCH_MAX_CONFIGS];
    uint64_t total_violations[MRG_BENCH_MAX_CONFIGS];
    int readers[MRG_BENCH_MAX_CONFIGS];
    int writers[MRG_BENCH_MAX_CONFIGS];
    int config_count;
} mrg_bench_summary_stats_t;

static void mrg_bench_wait_for_start(netdata_cond_t *cond, netdata_mutex_t *mutex, uint64_t *flag) {
    netdata_mutex_lock(mutex);
    while (__atomic_load_n(flag, __ATOMIC_RELAXED) == 0)
        netdata_cond_wait(cond, mutex);
    netdata_mutex_unlock(mutex);
}

static void mrg_bench_thread(void *arg) {
    mrg_bench_thread_context_t *ctx = (mrg_bench_thread_context_t *)arg;
    mrg_bench_thread_control_t *tc = ctx->control;
    MRG *mrg = ctx->mrg;
    METRIC *metric = ctx->metric;

    while(1) {
        mrg_bench_wait_for_start(&tc->cond, &tc->cond_mutex, &tc->run_flag);

        if(__atomic_load_n(&tc->run_flag, __ATOMIC_RELAXED) == MRG_BENCH_STOP_SIGNAL)
            break;

        usec_t start = now_monotonic_high_precision_usec();
        uint64_t operations = 0;
        uint64_t violations = 0;

        if(ctx->type == MRG_BENCH_WRITER) {
            // Writer: expand retention with incrementing timestamps
            time_t seq = 1000;
            while(__atomic_load_n(&tc->run_flag, __ATOMIC_RELAXED)) {
                seq++;
                // Expand retention with incrementing first and last time
                mrg_metric_expand_retention(mrg, metric, seq - 100, seq, 10);
                operations++;
            }
        }
        else {
            // Reader: read retention and check consistency invariant
            while(__atomic_load_n(&tc->run_flag, __ATOMIC_RELAXED)) {
                time_t first_time_s, last_time_s;
                uint32_t update_every_s;
                mrg_metric_get_retention(mrg, metric, &first_time_s, &last_time_s, &update_every_s);

                // Consistency check: first_time_s <= last_time_s
                if(unlikely(first_time_s > 0 && last_time_s > 0 && first_time_s > last_time_s))
                    violations++;

                operations++;
            }
        }

        usec_t test_time = now_monotonic_high_precision_usec() - start;
        __atomic_store_n(&ctx->stats[ctx->thread_id].test_time, test_time, __ATOMIC_RELEASE);
        __atomic_store_n(&ctx->stats[ctx->thread_id].operations, operations, __ATOMIC_RELEASE);
        __atomic_store_n(&ctx->stats[ctx->thread_id].violations, violations, __ATOMIC_RELEASE);
        __atomic_store_n(&ctx->stats[ctx->thread_id].ready, 1, __ATOMIC_RELEASE);
    }
}

static void mrg_bench_print_thread_stats(const char *test_name, int readers, int writers,
                                          mrg_bench_thread_context_t *contexts,
                                          mrg_bench_thread_stats_t *stats,
                                          mrg_bench_summary_stats_t *summary, int config_idx) {
    fprintf(stderr, "\n%-20s (readers: %d, writers: %d)\n", test_name, readers, writers);
    fprintf(stderr, "%4s %8s %12s %12s %12s %12s\n",
            "THR", "TYPE", "OPS", "OPS/SEC", "VIOLATIONS", "TIME (ms)");

    double reader_ops_per_sec = 0;
    double writer_ops_per_sec = 0;
    uint64_t total_violations = 0;

    for(int i = 0; i < readers + writers; i++) {
        uint64_t ops = __atomic_load_n(&stats[i].operations, __ATOMIC_RELAXED);
        uint64_t viol = __atomic_load_n(&stats[i].violations, __ATOMIC_RELAXED);
        usec_t time = __atomic_load_n(&stats[i].test_time, __ATOMIC_RELAXED);
        double ops_per_sec = time > 0 ? (double)ops * USEC_PER_SEC / time : 0.0;

        fprintf(stderr, "%4d %8s %12"PRIu64" %12.0f %12"PRIu64" %12.2f\n",
                i,
                contexts[i].type == MRG_BENCH_READER ? "READER" : "WRITER",
                ops, ops_per_sec, viol, (double)time / 1000.0);

        total_violations += viol;

        if(contexts[i].type == MRG_BENCH_READER)
            reader_ops_per_sec += ops_per_sec;
        else
            writer_ops_per_sec += ops_per_sec;
    }

    if(total_violations > 0) {
        fprintf(stderr, "\nFATAL ERROR: Detected %"PRIu64" consistency violations (torn reads)!\n",
                total_violations);
        fflush(stderr);
        _exit(1);
    }

    summary->reader_ops_per_sec[config_idx] = reader_ops_per_sec;
    summary->writer_ops_per_sec[config_idx] = writer_ops_per_sec;
    summary->total_violations[config_idx] = total_violations;
    summary->readers[config_idx] = readers;
    summary->writers[config_idx] = writers;
}

static void mrg_bench_run_test(const char *name, int readers, int writers,
                                mrg_bench_thread_context_t *contexts,
                                mrg_bench_thread_stats_t *stats,
                                mrg_bench_thread_control_t *controls,
                                mrg_bench_summary_stats_t *summary, int config_idx) {
    int total_threads = readers + writers;

    fprintf(stderr, "\nRunning test: %s with %d readers and %d writers...\n",
            name, readers, writers);

    // Reset
    memset(stats, 0, total_threads * sizeof(mrg_bench_thread_stats_t));

    // Signal threads to start
    for(int i = 0; i < total_threads; i++) {
        netdata_mutex_lock(&controls[i].cond_mutex);
        __atomic_store_n(&controls[i].run_flag, 1, __ATOMIC_RELAXED);
        netdata_cond_signal(&controls[i].cond);
        netdata_mutex_unlock(&controls[i].cond_mutex);
    }

    sleep_usec(MRG_BENCH_TEST_DURATION_SEC * USEC_PER_SEC);

    // Signal stop
    for(int i = 0; i < total_threads; i++)
        __atomic_store_n(&controls[i].run_flag, 0, __ATOMIC_RELEASE);

    // Wait for results
    for(int i = 0; i < total_threads; i++) {
        while(!__atomic_load_n(&stats[i].ready, __ATOMIC_ACQUIRE))
            sleep_usec(10);
    }

    mrg_bench_print_thread_stats(name, readers, writers, contexts, stats, summary, config_idx);
}

static void mrg_bench_print_summary(const mrg_bench_summary_stats_t *summary) {
    fprintf(stderr, "\n=== MRG Retention Benchmark Summary (Million ops/sec) ===\n\n");
    fprintf(stderr, "%-8s %-8s %16s %16s\n",
            "Readers", "Writers", "Reader Ops/s", "Writer Ops/s");
    fprintf(stderr, "----------------------------------------------------------------------\n");

    for(int config = 0; config < summary->config_count; config++) {
        double reader_ops = summary->reader_ops_per_sec[config];
        double writer_ops = summary->writer_ops_per_sec[config];

        fprintf(stderr, "%-8d %-8d %16.2f %16.2f\n",
                summary->readers[config],
                summary->writers[config],
                reader_ops / 1000000.0,
                writer_ops / 1000000.0);
    }
    fprintf(stderr, "\n");
}

int mrg_retention_benchmark(void) {
    mrg_bench_summary_stats_t summary = {0};

    // Use mrg_create_for_unittest() to avoid loading from database
    MRG *mrg = mrg_create_for_unittest();
    nd_uuid_t test_uuid;
    uuid_generate(test_uuid);

    MRG_ENTRY entry = {
        .uuid = &test_uuid,
        .section = (Word_t)&test_ctx_0,
        .first_time_s = 1000,
        .last_time_s = 2000,
        .latest_update_every_s = 10,
    };
    bool added;
    METRIC *metric = mrg_metric_add_and_acquire(mrg, entry, &added);

    if(!added) {
        fatal("DBENGINE METRIC: failed to add metric for benchmark");
    }

    mrg_bench_thread_stats_t stats[MRG_BENCH_MAX_THREADS];
    mrg_bench_thread_control_t controls[MRG_BENCH_MAX_THREADS];
    mrg_bench_thread_context_t contexts[MRG_BENCH_MAX_THREADS];

    fprintf(stderr, "\nStarting MRG retention benchmark...\n");
    fprintf(stderr, "Creating threads...\n");

    // Initialize per-thread controls
    for(int i = 0; i < MRG_BENCH_MAX_THREADS; i++) {
        netdata_cond_init(&controls[i].cond);
        netdata_mutex_init(&controls[i].cond_mutex);
        __atomic_store_n(&controls[i].run_flag, 0, __ATOMIC_RELAXED);
    }

    // Create threads
    for(int i = 0; i < MRG_BENCH_MAX_THREADS; i++) {
        char thr_name[32];
        snprintf(thr_name, sizeof(thr_name), "mrgbench%d", i);

        contexts[i] = (mrg_bench_thread_context_t){
            .thread_id = i,
            .type = MRG_BENCH_READER,
            .mrg = mrg,
            .metric = metric,
            .stats = stats,
            .control = &controls[i],
        };
        contexts[i].thread =
            nd_thread_create(thr_name, NETDATA_THREAD_OPTION_DONT_LOG, mrg_bench_thread, &contexts[i]);
    }

    // Test configurations: [readers, writers]
    int configs[][2] = {
        {1, 0},   // Single reader (no contention baseline)
        {0, 1},   // Single writer (write throughput baseline)
        {1, 1},   // 1 reader + 1 writer
        {2, 1},   // 2 readers + 1 writer (typical seqlock sweet spot)
        {4, 1},   // 4 readers + 1 writer
        {8, 1},   // 8 readers + 1 writer (high read contention)
        {16, 1},  // 16 readers + 1 writer
    };

    const int num_configs = sizeof(configs) / sizeof(configs[0]);
    summary.config_count = num_configs;

    // Warm up
    sleep_usec(100000);

    for(int i = 0; i < num_configs; i++) {
        int readers = configs[i][0];
        int writers = configs[i][1];
        int total = readers + writers;

        // Assign reader/writer roles
        int thread_idx = 0;
        for(int r = 0; r < readers; r++) {
            contexts[thread_idx].type = MRG_BENCH_READER;
            thread_idx++;
        }
        for(int w = 0; w < writers; w++) {
            contexts[thread_idx].type = MRG_BENCH_WRITER;
            thread_idx++;
        }

        // Reset roles for unused threads
        for(int j = total; j < MRG_BENCH_MAX_THREADS; j++) {
            contexts[j].type = MRG_BENCH_READER;
        }

        char test_name[64];
        snprintf(test_name, sizeof(test_name), "mrg_retention %dR/%dW", readers, writers);
        mrg_bench_run_test(test_name, readers, writers, contexts, stats, controls, &summary, i);
    }

    mrg_bench_print_summary(&summary);

    // Stop all threads
    fprintf(stderr, "Stopping threads...\n");
    for(int i = 0; i < MRG_BENCH_MAX_THREADS; i++) {
        netdata_mutex_lock(&controls[i].cond_mutex);
        __atomic_store_n(&controls[i].run_flag, MRG_BENCH_STOP_SIGNAL, __ATOMIC_RELAXED);
        netdata_cond_signal(&controls[i].cond);
        netdata_mutex_unlock(&controls[i].cond_mutex);
    }

    fprintf(stderr, "Waiting for threads to exit...\n");
    for(int i = 0; i < MRG_BENCH_MAX_THREADS; i++) {
        nd_thread_join(contexts[i].thread);
    }

    // Cleanup
    for(int i = 0; i < MRG_BENCH_MAX_THREADS; i++) {
        netdata_cond_destroy(&controls[i].cond);
        netdata_mutex_destroy(&controls[i].cond_mutex);
    }

    mrg_metric_release(mrg, metric);
    mrg_destroy(mrg);

    fprintf(stderr, "All benchmark tests passed.\n");
    return 0;
}
