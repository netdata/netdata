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

// Locks in the contract that pgc_open_cache_to_journal_v2() depends on when it
// casts a PGC page's bare metric_id back to a METRIC *.
//
// PGC pages store metric_id = (Word_t)metric WITHOUT holding an MRG reference,
// so that pointer can outlive the METRIC. Two things then matter:
//
//   1. mrg_metric_dup() CANNOT reject such a pointer. ARAL writes its free-list
//      header over the first 16 bytes of a freed element, which on 64bit
//      overlays metric->uuid and metric->refcount with the halves of the
//      free-list 'next' pointer. With 'next' NULL the refcount reads 0, which
//      is a legal unreferenced-but-alive value, so the acquire succeeds.
//
//   2. Because it succeeds, a NULL uuid is not a safe signal to act on: it is
//      still the result of dereferencing a dead pointer. Production code must
//      not dereference such a pointer at all; it carries the UUIDMAP_ID and
//      resolves that through the MRG index instead.
//
// The guard below is keyed to FSANITIZE_ADDRESS, which is what makes ARAL fall
// back to plain malloc/free (see aral_mallocz_internal() / aral_freez_internal()
// in src/libnetdata/aral/aral.c). ONLY in that configuration is the access below
// a real use-after-free, and only then must the test be skipped.
//
// -DENABLE_ADDRESS_SANITIZER defines FSANITIZE_ADDRESS itself
// (packaging/cmake/Modules/NetdataCompilerFlags.cmake), so an ASan build skips
// this test through the guard below; ARAL without its pooling is what ASan needs
// to see the free at all.
//
// It is deliberately NOT keyed to __SANITIZE_ADDRESS__, which only says the
// translation unit is instrumented. Instrumentation without the ARAL bypass
// leaves the element inside a live ARAL page, so the access below is ordinary
// in-bounds memory that ASan has nothing to say about, and skipping on it would
// drop this coverage for no reason.
static int mrg_stale_metric_pointer_unittest(void) {
#if defined(FSANITIZE_ADDRESS)
    fprintf(stderr, "\nTesting stale METRIC pointer contract... SKIPPED (FSANITIZE_ADDRESS)\n");
    return 0;
#else
    fprintf(stderr, "\nTesting stale METRIC pointer contract (deterministic)...\n");
    int errors = 0;

    MRG *mrg = mrg_create_for_unittest();

    nd_uuid_t victim;
    uuid_generate_random(victim);

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
        (void)mrg_destroy(mrg);
        return 1;
    }

    // Keep the raw pointer exactly the way a PGC page keeps page->metric_id:
    // as a bare value, with no reference behind it.
    METRIC *stale = (METRIC *)(uintptr_t)mrg_metric_id(mrg, metric);

    // Delete it. From here on `stale` points into the MRG partition ARAL's
    // free list - this is the situation a page with a stale metric_id is in.
    if(!mrg_metric_release_and_delete(mrg, metric)) {
        fprintf(stderr, "ERROR: the victim metric was not deleted\n");
        (void)mrg_destroy(mrg);
        return 1;
    }

    // Snapshot the overlaid bytes so we can put them back: the acquire below
    // mutates the free-list 'next' pointer this slot now holds.
    REFCOUNT refcount_before = __atomic_load_n(&stale->refcount, __ATOMIC_RELAXED);

    METRIC *resurrected = mrg_metric_dup(mrg, stale);
    if(!resurrected) {
        // The acquire refused the freed slot. That is strictly better than what
        // we expect, and it means the hazard below cannot arise in this build.
        // Not an error - just record that the premise did not apply here.
        fprintf(stderr, "  note: acquire refused the freed slot (refcount read %d)\n",
                refcount_before);
    }
    else {
        // THE assertion: an acquire that succeeded on a dead pointer must not
        // also produce a resolvable uuid. This is a premise test only: neither
        // outcome makes dereferencing an unowned pointer valid.
        nd_uuid_t *uuid = mrg_metric_uuid(mrg, resurrected);
        if(uuid) {
            char b[UUID_STR_LEN] = "?";
            uuid_unparse_lower(*uuid, b);
            fprintf(stderr,
                    "ERROR: a freed METRIC was re-acquired AND resolved to uuid %s - "
                    "the stale-pointer contract is broken\n", b);
            errors++;
        }

        // Undo the acquire by hand. mrg_metric_release() must NOT be used here:
        // it is exactly the double free this test exists to describe. Restore
        // the bytes the acquire changed, and the stats it bumped, so the ARAL
        // free list and the MRG counters are consistent for mrg_destroy().
        __atomic_store_n(&stale->refcount, refcount_before, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&mrg->index[stale->partition].stats.current_references, 1, __ATOMIC_RELAXED);
        if(refcount_before == 0)
            __atomic_sub_fetch(&mrg->index[stale->partition].stats.entries_acquired, 1, __ATOMIC_RELAXED);
    }

    size_t referenced = mrg_destroy(mrg);
    if(referenced) {
        fprintf(stderr, "ERROR: %zu metrics still referenced - the MRG was not destroyed\n", referenced);
        errors++;
    }

    if(errors)
        fprintf(stderr, "Stale METRIC pointer test: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "Stale METRIC pointer test: OK\n");

    return errors;
#endif
}

// Regression test for the jv2 stale-metric double free.
//
// Guards the !uuid branch of pgc_open_cache_to_journal_v2(): it must skip the
// page WITHOUT releasing the metric. Releasing a stale METRIC takes the bogus
// refcount it read out of the ARAL free list down to DELETED and aral_freez()es
// an already-free element - the 'ARAL: mrg[N] double free' fatal seen in the
// field.
//
// mrg_stale_metric_pointer_unittest() above establishes WHY a stale pointer
// survives the acquire. This test drives the real function and asserts the
// behaviour, so it fails against the pre-fix code instead of passing anyway.
//
// THE assertion is that the indexer leaves the freed slot COMPLETELY UNTOUCHED.
// Anything weaker is not enough: an earlier version of this fix skipped the page
// without RELEASING the metric, which stopped the double free but left the bogus
// acquire's refcount CAS in place - and on a freed slot refcount is the high half
// of ARAL's free-list 'next' pointer, so the free list stayed corrupted and the
// next allocation from that page returned a wild pointer. A test that only
// checked "was not released" passed that broken change.
//
// To keep every outcome an integer comparison rather than a crash, we plant
// refcount = PLANTED (> 1) instead of the 0 that a NULL free-list 'next'
// naturally produces. Untouched => PLANTED. Acquired and left outstanding =>
// PLANTED+1. Acquired then released => PLANTED again, which refcount alone
// cannot distinguish from correct - so we also assert on uuid and on the MRG
// reference counter.
//
// Same guard rationale as mrg_stale_metric_pointer_unittest() above: keyed to
// FSANITIZE_ADDRESS because that is what bypasses ARAL, not to the presence of
// -fsanitize=address.
static bool jv2_stale_metric_migrate_cb(
    Word_t section __maybe_unused, unsigned datafile_fileno __maybe_unused,
    uint8_t type __maybe_unused, Pvoid_t JudyL_metrics __maybe_unused,
    Pvoid_t JudyL_extents_pos __maybe_unused, size_t count_of_unique_extents __maybe_unused,
    size_t count_of_unique_metrics __maybe_unused, size_t count_of_unique_pages __maybe_unused,
    void *data __maybe_unused) {
    // false => the caller unwinds the pages without trying to write a journal
    return false;
}

static void jv2_stale_metric_free_clean_cb(PGC *cache __maybe_unused, PGC_ENTRY entry __maybe_unused) { ; }
static void jv2_stale_metric_save_dirty_cb(
    PGC *cache __maybe_unused, PGC_ENTRY *entries_array __maybe_unused,
    PGC_PAGE **pages_array __maybe_unused, size_t entries __maybe_unused) { ; }

static int mrg_jv2_stale_metric_not_dereferenced_unittest(void) {
#if defined(FSANITIZE_ADDRESS)
    fprintf(stderr, "\nTesting jv2 does not dereference a stale METRIC... SKIPPED (FSANITIZE_ADDRESS)\n");
    return 0;
#else
    fprintf(stderr, "\nTesting jv2 does not dereference a stale METRIC (deterministic)...\n");
    int errors = 0;

    enum {
        TEST_FILENO = 7,
        PLANTED     = 5,        // > 1, so a release cannot reach aral_freez()
    };
    const Word_t section = (Word_t)&test_ctx_0;

    MRG *mrg = mrg_create_for_unittest();

    // pgc_open_cache_to_journal_v2() resolves metrics through the global
    // main_mrg, so point it at our test MRG for the duration.
    MRG *saved_main_mrg = main_mrg;
    main_mrg = mrg;

    PGC *cache = pgc_create(
        "jv2-stale-metric-test",
        32 * 1024 * 1024, jv2_stale_metric_free_clean_cb,
        64, NULL, jv2_stale_metric_save_dirty_cb,
        10, 10, 1000, 10,
        PGC_OPTIONS_DEFAULT, 1, sizeof(struct extent_io_data));

    // 1. a real metric, with no retention so it is deletable on release
    nd_uuid_t victim;
    uuid_generate_random(victim);
    bool added = false;
    MRG_ENTRY entry = {
        .uuid = &victim,
        .section = section,
        .first_time_s = 0,
        .last_time_s = 0,
        .latest_update_every_s = 0,
    };
    METRIC *metric = mrg_metric_add_and_acquire(mrg, entry, &added);
    if(!metric) {
        fprintf(stderr, "ERROR: cannot add the victim metric\n");
        pgc_destroy(cache, false);
        main_mrg = saved_main_mrg;
        (void)mrg_destroy(mrg);
        return 1;
    }

    METRIC *stale = (METRIC *)(uintptr_t)mrg_metric_id(mrg, metric);

    // The id the open-cache page will carry. Captured while the metric is still
    // alive, exactly as pgc_open_add_hot_page() does in production. After the
    // delete below it no longer resolves, which is what the indexer must detect.
    const UUIDMAP_ID stale_uuid_id = mrg_metric_uuidmap_id(mrg, metric);
    const uint8_t stale_partition = stale->partition;   // survives the free (offset > 16)

    // 2. delete it - `stale` now points into the partition ARAL's free list,
    //    exactly like a PGC page holding a metric_id whose METRIC has gone.
    if(!mrg_metric_release_and_delete(mrg, metric)) {
        fprintf(stderr, "ERROR: the victim metric was not deleted\n");
        pgc_destroy(cache, false);
        main_mrg = saved_main_mrg;
        (void)mrg_destroy(mrg);
        return 1;
    }

    // 3. plant the overlaid bytes. uuid 0 makes mrg_metric_uuid() return NULL,
    //    which is what a freed slot with a NULL free-list 'next' produces.
    //    refcount PLANTED keeps a release away from the deletion path so the
    //    pre-fix behaviour is measurable instead of fatal.
    //
    //    uuid and refcount together ARE this slot's free-list 'next' pointer.
    //    Snapshot BOTH before overwriting and restore exactly these values at
    //    teardown. Restoring zeros instead would be correct only while this
    //    slot happens to be the tail of its incoming list; on any other slot it
    //    would cut every entry behind it out of the free list.
    const UUIDMAP_ID overlay_uuid_before = __atomic_load_n(&stale->uuid, __ATOMIC_RELAXED);
    const REFCOUNT overlay_refcount_before = __atomic_load_n(&stale->refcount, __ATOMIC_RELAXED);

    __atomic_store_n(&stale->uuid, 0, __ATOMIC_RELAXED);
    __atomic_store_n(&stale->refcount, (REFCOUNT)PLANTED, __ATOMIC_RELAXED);

    // 4. a hot page in the section/datafile the indexer will walk, carrying the
    //    stale pointer as its metric_id - the production layout.
    struct extent_io_data xio = {
        .fileno = TEST_FILENO,
        .block = 4096,
        .bytes = 4096,
        .uuid_id = stale_uuid_id,
    };
    bool page_added = false;
    PGC_PAGE *page = pgc_page_add_and_acquire(cache, (PGC_ENTRY){
        .section = section,
        .metric_id = (Word_t)stale,
        .start_time_s = 100,
        .end_time_s = 200,
        .size = 4096,
        .data = NULL,
        .update_every_s = 1,
        .hot = true,
        .custom_data = (uint8_t *)&xio,
    }, &page_added);

    if(!page || !page_added) {
        fprintf(stderr, "ERROR: cannot add the hot page carrying the stale metric_id\n");
        if(page) pgc_page_release(cache, page);
        pgc_destroy(cache, false);
        main_mrg = saved_main_mrg;
        (void)mrg_destroy(mrg);
        return 1;
    }

    // 5. run the indexer over it.
    //    Snapshot the partition's search-miss counter first: a correct indexer
    //    resolves the metric through the MRG index, so a dead id MUST show up
    //    here as a miss. A version that went back to dereferencing
    //    page->metric_id would never perform a lookup at all, and this counter
    //    would not move. That is what makes this test discriminate - the
    //    slot-unchanged assertions below cannot, because an acquire followed by
    //    a release restores every byte it touched.
    const size_t misses_before =
        __atomic_load_n(&mrg->index[stale_partition].stats.search_misses, __ATOMIC_RELAXED);

    pgc_open_cache_to_journal_v2(cache, section, TEST_FILENO, 1,
                                 jv2_stale_metric_migrate_cb, NULL, true);

    const size_t misses_after =
        __atomic_load_n(&mrg->index[stale_partition].stats.search_misses, __ATOMIC_RELAXED);

    // 6. THE assertion: the freed slot must be bit-for-bit unchanged, and the
    //    MRG must not have recorded a reference against it.
    REFCOUNT after_refcount = __atomic_load_n(&stale->refcount, __ATOMIC_RELAXED);
    UUIDMAP_ID after_uuid = __atomic_load_n(&stale->uuid, __ATOMIC_RELAXED);

    if(after_refcount != (REFCOUNT)PLANTED) {
        fprintf(stderr,
                "ERROR: pgc_open_cache_to_journal_v2() mutated a freed slot's "
                "refcount (%d -> %d). refcount is the high half of ARAL's "
                "free-list 'next' pointer, so this corrupts the free list\n",
                (int)PLANTED, (int)after_refcount);
        errors++;
    }

    if(after_uuid != 0) {
        fprintf(stderr,
                "ERROR: pgc_open_cache_to_journal_v2() mutated a freed slot's "
                "uuid (0 -> %" PRIu32 ") - the low half of the same free-list pointer\n",
                after_uuid);
        errors++;
    }

    if(__atomic_load_n(&mrg->index[stale_partition].stats.current_references,
                       __ATOMIC_RELAXED) != 0) {
        fprintf(stderr,
                "ERROR: the indexer took an MRG reference on a dead metric "
                "(current_references != 0)\n");
        errors++;
    }

    if(misses_after == misses_before) {
        fprintf(stderr,
                "ERROR: the indexer did not look the metric up in the MRG index "
                "(search_misses unchanged at %zu) - it must resolve the page's "
                "uuid_id, never dereference page->metric_id\n",
                misses_after);
        errors++;
    }

    // 7. tear down without touching the stale slot through the MRG API.
    pgc_page_release(cache, page);
    pgc_destroy(cache, false);

    // Restore the overlay bytes THIS TEST planted. A correct indexer changes
    // nothing here, so there is nothing of its doing to undo - and no MRG stats
    // compensation, because it never acquires the dead metric. The assertion
    // above is what proves that.
    //
    // Restore the SNAPSHOT, not zeros: these two fields are the slot's
    // free-list 'next' pointer, and zeroing a non-NULL one would cut every slot
    // behind it out of the list.
    __atomic_store_n(&stale->refcount, overlay_refcount_before, __ATOMIC_RELAXED);
    __atomic_store_n(&stale->uuid, overlay_uuid_before, __ATOMIC_RELAXED);

    main_mrg = saved_main_mrg;

    size_t referenced = mrg_destroy(mrg);
    if(referenced) {
        fprintf(stderr, "ERROR: %zu metrics still referenced - the MRG was not destroyed\n", referenced);
        errors++;
    }

    if(errors)
        fprintf(stderr, "jv2 stale METRIC test: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "jv2 stale METRIC test: OK\n");

    return errors;
#endif
}

// Captures what the indexer grouped, so the test can assert on it.
struct jv2_group_capture {
    size_t metrics;
    size_t pages;
    size_t pages_in_judy;       // pages actually reachable in the per-metric JudyLs
    bool number_of_pages_ok;    // every mi->number_of_pages matched its JudyL count
    bool called;
    bool return_success;        // what the callback reports to the indexer

    size_t extents;             // extents the indexer reports to the writer
    bool writer_invariants_ok;  // what journalfile_migrate_to_v2_callback() would need

    // the single indexed page, when there is exactly one
    time_t only_end_time_s;
    size_t only_page_length;
    uint32_t only_update_every_s;
    uint32_t only_extent_index;
};

static bool jv2_group_capture_cb(
    Word_t section __maybe_unused, unsigned datafile_fileno __maybe_unused,
    uint8_t type __maybe_unused, Pvoid_t JudyL_metrics,
    Pvoid_t JudyL_extents_pos, size_t count_of_unique_extents,
    size_t count_of_unique_metrics, size_t count_of_unique_pages,
    void *data) {
    struct jv2_group_capture *c = data;
    c->metrics = count_of_unique_metrics;
    c->pages = count_of_unique_pages;
    c->extents = count_of_unique_extents;
    c->called = true;

    // Re-derive what journalfile_migrate_to_v2_callback() relies on: it writes a
    // metric's page descriptors by WALKING JudyL_pages_by_start_time, but advances
    // pages_offset by mi->number_of_pages. If those two disagree its offset check
    // fails and the entire migration is discarded, so assert the invariant here
    // rather than only counting pages.
    c->number_of_pages_ok = true;
    c->writer_invariants_ok = true;
    c->pages_in_judy = 0;

    // The real writer sizes the file from count_of_unique_pages /
    // count_of_unique_extents, but then addresses it through per-metric and
    // per-extent numbers. Check the two things it would trip on, so a stubbed
    // callback still catches a miscounted migration:
    //
    //   1. sum(mi->number_of_pages) == count_of_unique_pages, because pages_offset
    //      is advanced per metric while the file was sized from the total.
    //   2. extent indices are exactly 0..count_of_unique_extents-1, because
    //      journalfile_v2_write_extent_list() writes at j2_extent_base[ei->index]
    //      and returns base + count.
    size_t total_pages_claimed = 0;

    Pvoid_t seen_extent_index = NULL;
    size_t extents_walked = 0;
    Pvoid_t *ext_pptr;
    bool ext_first = true;
    Word_t ext_block = 0;
    while((ext_pptr = JudyLFirstThenNext(JudyL_extents_pos, &ext_block, &ext_first))) {
        struct jv2_extents_info *ei = *ext_pptr;
        extents_walked++;

        if(ei->index >= count_of_unique_extents) {
            fprintf(stderr, "ERROR: extent index %u is out of range for %zu extents\n",
                    ei->index, count_of_unique_extents);
            c->writer_invariants_ok = false;
            continue;
        }

        Pvoid_t *seen = JudyLIns(&seen_extent_index, (Word_t)ei->index, PJE0);
        if(!seen || seen == PJERR) {
            fprintf(stderr, "ERROR: cannot record extent index %u\n", ei->index);
            c->writer_invariants_ok = false;
        }
        else if(*seen) {
            fprintf(stderr, "ERROR: extent index %u is used twice\n", ei->index);
            c->writer_invariants_ok = false;
        }
        else
            *seen = (void *)ei;
    }
    JudyLFreeArray(&seen_extent_index, PJE0);

    if(extents_walked != count_of_unique_extents) {
        fprintf(stderr, "ERROR: %zu extents are reachable but %zu were reported\n",
                extents_walked, count_of_unique_extents);
        c->writer_invariants_ok = false;
    }

    Pvoid_t *mi_pptr;
    bool mi_first = true;
    Word_t uuid_id = 0;
    while((mi_pptr = JudyLFirstThenNext(JudyL_metrics, &uuid_id, &mi_first))) {
        struct jv2_metrics_info *mi = *mi_pptr;

        size_t counted = 0;
        Pvoid_t *pi_pptr;
        bool pi_first = true;
        Word_t start_time = 0;
        while((pi_pptr = JudyLFirstThenNext(mi->JudyL_pages_by_start_time, &start_time, &pi_first)))
            counted++;

        c->pages_in_judy += counted;
        total_pages_claimed += mi->number_of_pages;

        if(counted != mi->number_of_pages) {
            fprintf(stderr, "ERROR: uuid id %" PRIu32 " claims %zu pages but its JudyL holds %zu\n",
                    (UUIDMAP_ID)uuid_id, (size_t)mi->number_of_pages, counted);
            c->number_of_pages_ok = false;
        }
    }

    if(total_pages_claimed != count_of_unique_pages) {
        fprintf(stderr, "ERROR: metrics claim %zu pages in total but %zu were reported - "
                        "the writer's pages_offset would not land on its own trailer\n",
                total_pages_claimed, count_of_unique_pages);
        c->writer_invariants_ok = false;
    }

    if(c->pages_in_judy == 1) {
        // record the surviving descriptor, so a caller that planted two DIFFERENT
        // pages can assert WHICH one was kept
        Word_t first_uuid_id = 0;
        Pvoid_t *only_mi_pptr = JudyLFirst(JudyL_metrics, &first_uuid_id, PJE0);
        if(only_mi_pptr) {
            struct jv2_metrics_info *only_mi = *only_mi_pptr;

            Word_t first_start_time = 0;
            Pvoid_t *only_pi_pptr = JudyLFirst(only_mi->JudyL_pages_by_start_time, &first_start_time, PJE0);
            if(only_pi_pptr) {
                struct jv2_page_info *only_pi = *only_pi_pptr;
                c->only_end_time_s = only_pi->end_time_s;
                c->only_page_length = only_pi->page_length;
                c->only_update_every_s = only_pi->update_every_s;
                c->only_extent_index = only_pi->extent_index;
            }
        }
    }

    return c->return_success;
}

// The state a metric deletion + recreation leaves behind: two DIFFERENT METRIC
// pointers for ONE uuidmap id.
//
// `pinned` emulates another tier's METRIC holding the uuid. Without it, deleting the
// first metric would drop the uuidmap entry and the re-created metric would get a
// DIFFERENT id, which is not the case under test.
//
// `decoy` consumes the ARAL slot the delete just freed, so the re-created metric is
// forced onto a different address - otherwise ARAL hands the same slot straight back
// and there is no "two pointers, one uuid" case at all. It shares the victim's uuid
// partition (last byte) so it allocates from the same partition ARAL, and it is held
// until teardown so the slot is not released again.
//
// Returns the number of errors. On success (0) the caller owns `pinned` (a uuidmap
// reference) plus one MRG reference on each of `live` and `decoy`; on failure this
// releases everything it took, so the caller has nothing to undo. A premise that does
// not hold is REPORTED, never silently passed.
struct jv2_two_pointers {
    UUIDMAP_ID pinned;
    UUIDMAP_ID shared_id;
    METRIC *stale_ptr;      // the freed first metric's address - never dereferenced
    METRIC *live;
    METRIC *decoy;
};

// Releases everything jv2_make_two_pointers_one_uuid() took and zeroes the struct,
// so the same struct can never be released twice - neither by a failure path inside
// the maker, nor by the caller's cleanup afterwards.
static void jv2_two_pointers_undo(MRG *mrg, struct jv2_two_pointers *tp) {
    if(tp->live) {
        mrg_metric_release_and_delete(mrg, tp->live);
        tp->live = NULL;
    }
    if(tp->decoy) {
        mrg_metric_release_and_delete(mrg, tp->decoy);
        tp->decoy = NULL;
    }
    if(tp->pinned) {
        uuidmap_free(tp->pinned);
        tp->pinned = 0;
    }
    tp->shared_id = 0;
    tp->stale_ptr = NULL;
}

static int jv2_make_two_pointers_one_uuid(MRG *mrg, Word_t section, struct jv2_two_pointers *out) {
    memset(out, 0, sizeof(*out));

    nd_uuid_t shared;
    uuid_generate_random(shared);

    out->pinned = uuidmap_create(shared);

    MRG_ENTRY entry = {
        .uuid = &shared,
        .section = section,
        .first_time_s = 0,          // no retention, so it is deletable on release
        .last_time_s = 0,
        .latest_update_every_s = 0,
    };

    bool added = false;
    METRIC *first = mrg_metric_add_and_acquire(mrg, entry, &added);
    if(!first) {
        fprintf(stderr, "ERROR: cannot add the first metric\n");
        jv2_two_pointers_undo(mrg, out);
        return 1;
    }
    out->stale_ptr = (METRIC *)(uintptr_t)mrg_metric_id(mrg, first);

    // delete it; `pinned` keeps the uuid (and therefore the id) alive
    if(!mrg_metric_release_and_delete(mrg, first)) {
        // the reference is dropped either way; false only means it was retained
        fprintf(stderr, "ERROR: the first metric was not deleted\n");
        jv2_two_pointers_undo(mrg, out);
        return 1;
    }

    nd_uuid_t decoy_uuid;
    uuid_generate_random(decoy_uuid);
    decoy_uuid[15] = shared[15];
    bool decoy_added = false;
    MRG_ENTRY decoy_entry = {
        .uuid = &decoy_uuid,
        .section = section,
        .first_time_s = 2,          // retention, so it is not deletable on release
        .last_time_s = 3,
        .latest_update_every_s = 1,
    };
    out->decoy = mrg_metric_add_and_acquire(mrg, decoy_entry, &decoy_added);
    if(!out->decoy) {
        fprintf(stderr, "ERROR: cannot add the decoy metric\n");
        jv2_two_pointers_undo(mrg, out);
        return 1;
    }

    // re-create for the same uuid -> new pointer, same id
    added = false;
    out->live = mrg_metric_add_and_acquire(mrg, entry, &added);
    if(!out->live || !added) {
        fprintf(stderr, "ERROR: cannot re-create the metric for the same uuid\n");
        jv2_two_pointers_undo(mrg, out);
        return 1;
    }
    out->shared_id = mrg_metric_uuidmap_id(mrg, out->live);

    METRIC *new_ptr = (METRIC *)(uintptr_t)mrg_metric_id(mrg, out->live);
    if(new_ptr == out->stale_ptr) {
        // ARAL handed back the same slot, so there is no "two pointers, one uuid"
        // situation. Not a product defect - an inconclusive run, and better reported
        // than silently passing.
        fprintf(stderr, "ERROR: re-created metric reused the same address; "
                        "the case under test cannot be formed\n");
        jv2_two_pointers_undo(mrg, out);
        return 1;
    }

    if(out->shared_id != out->pinned) {
        fprintf(stderr, "ERROR: re-created metric got uuid id %" PRIu32 ", expected the "
                        "pinned %" PRIu32 " - the premise does not hold\n",
                out->shared_id, out->pinned);
        jv2_two_pointers_undo(mrg, out);
        return 1;
    }

    return 0;
}

// Two open-cache pages for the SAME uuid but with DIFFERENT page->metric_id
// values must be indexed as ONE metric.
//
// This happens in production when a metric is deleted and re-created for the
// same uuid: the new METRIC gets a new pointer, while older open-cache pages
// still carry the old one. The uuidmap id survives the deletion whenever
// something else still references that uuid - another tier's METRIC for the same
// dimension, for instance - so both pages legitimately carry the same uuid_id.
//
// It matters because journal v2 wants one entry per uuid:
// journalfile_migrate_to_v2_callback() builds and sorts a uuid list, and its
// readers bsearch() by uuid. Two entries for one uuid means one group's pages
// are unreachable in the written journal.
//
// Keying JudyL_metrics by page->metric_id produces two groups here. Keying it by
// the page's uuid_id produces one.
//
// Note this only became reachable once metrics were resolved by id: while the
// indexer dereferenced page->metric_id, the older page failed to resolve and was
// skipped, so the duplicate could not form.
// start_old / start_new are the two pages' start times, and expected_pages is how
// many pages the indexer should end up with. Distinct start times => both pages
// are indexed. IDENTICAL start times => the two pages collide inside the single
// uuid group, and exactly one of them must be kept.
static int mrg_jv2_same_uuid_grouped_once_check(
    const char *label, time_t start_old, time_t start_new,
    time_t end_old, time_t end_new, size_t expected_pages,
    time_t expected_end_time_s, size_t expected_extents, bool callback_success) {
    fprintf(stderr, "\nTesting jv2 groups one uuid once, %s (deterministic)...\n", label);
    int errors = 0;

    enum { TEST_FILENO = 9 };
    const Word_t section = (Word_t)&test_ctx_0;

    MRG *mrg = mrg_create_for_unittest();
    MRG *saved_main_mrg = main_mrg;
    main_mrg = mrg;

    PGC *cache = pgc_create(
        "jv2-same-uuid-test",
        32 * 1024 * 1024, jv2_stale_metric_free_clean_cb,
        64, NULL, jv2_stale_metric_save_dirty_cb,
        10, 10, 1000, 10,
        PGC_OPTIONS_DEFAULT, 1, sizeof(struct extent_io_data));

    // declared before the goto below, so jumping to cleanup cannot skip an
    // initialization and leave the teardown reading indeterminate values
    struct jv2_two_pointers tp = { 0 };
    METRIC *old_ptr = NULL, *new_ptr = NULL, *second = NULL, *decoy = NULL;
    UUIDMAP_ID shared_id = 0, pinned = 0;

    if(jv2_make_two_pointers_one_uuid(mrg, section, &tp)) {
        // the helper released everything it took, so there is nothing to undo
        errors++;
        goto cleanup_early;
    }
    old_ptr = tp.stale_ptr;
    new_ptr = (METRIC *)(uintptr_t)mrg_metric_id(mrg, tp.live);
    shared_id = tp.shared_id;
    pinned = tp.pinned;
    second = tp.live;
    decoy = tp.decoy;

    // two hot pages, same section and datafile, same uuid_id, DIFFERENT
    // metric_id; the start times decide whether they collide
    struct extent_io_data xio_old = {
        .fileno = TEST_FILENO, .block = 4096, .bytes = 4096, .uuid_id = shared_id,
    };
    struct extent_io_data xio_new = {
        .fileno = TEST_FILENO, .block = 8192, .bytes = 4096, .uuid_id = shared_id,
    };

    bool a1 = false, a2 = false;
    PGC_PAGE *p_old = pgc_page_add_and_acquire(cache, (PGC_ENTRY){
        .section = section, .metric_id = (Word_t)old_ptr,
        .start_time_s = start_old, .end_time_s = end_old, .size = 4096, .data = NULL,
        .update_every_s = 1, .hot = true, .custom_data = (uint8_t *)&xio_old,
    }, &a1);
    PGC_PAGE *p_new = pgc_page_add_and_acquire(cache, (PGC_ENTRY){
        .section = section, .metric_id = (Word_t)new_ptr,
        .start_time_s = start_new, .end_time_s = end_new, .size = 4096, .data = NULL,
        .update_every_s = 1, .hot = true, .custom_data = (uint8_t *)&xio_new,
    }, &a2);

    if(!p_old || !p_new || !a1 || !a2) {
        fprintf(stderr, "ERROR: cannot add the two hot pages\n");
        errors++;
    }
    else {
        struct jv2_group_capture cap = { 0 };
        cap.return_success = callback_success;
        pgc_open_cache_to_journal_v2(cache, section, TEST_FILENO, 1,
                                     jv2_group_capture_cb, &cap, true);

        if(!cap.called) {
            fprintf(stderr, "ERROR: the migrate callback was never invoked\n");
            errors++;
        }
        else {
            // THE assertion.
            if(cap.metrics != 1) {
                fprintf(stderr,
                        "ERROR: the indexer grouped one uuid into %zu metric "
                        "entries (expected 1) - journal v2 wants one entry per "
                        "uuid, so the extra group's pages become unreachable\n",
                        cap.metrics);
                errors++;
            }
            if(cap.pages != expected_pages) {
                fprintf(stderr,
                        "ERROR: the indexer saw %zu pages (expected %zu)\n",
                        cap.pages, expected_pages);
                errors++;
            }
            if(cap.pages_in_judy != expected_pages) {
                fprintf(stderr,
                        "ERROR: %zu pages are reachable in the per-metric JudyLs "
                        "(expected %zu)\n",
                        cap.pages_in_judy, expected_pages);
                errors++;
            }
            if(!cap.number_of_pages_ok) {
                fprintf(stderr,
                        "ERROR: mi->number_of_pages disagrees with the pages actually "
                        "indexed - journalfile_migrate_to_v2_callback() would discard "
                        "the whole migration\n");
                errors++;
            }
            if(callback_success) {
                // THE guard for the dropped page. Publishing the journal makes
                // every page this datafile owns clean; a page the indexer dropped
                // but forgot to dispose of stays hot, keeps its
                // DATAFILE_ACQUIRE_OPEN_CACHE reference, and its datafile can then
                // never be deleted (datafile.c refuses deletion while lockers
                // remain).
                size_t still_hot = pgc_hot_and_dirty_entries(cache);
                if(still_hot != 0) {
                    fprintf(stderr,
                            "ERROR: %zu pages are still hot after the journal was "
                            "published (expected 0) - a dropped page pins its "
                            "datafile forever\n",
                            still_hot);
                    errors++;
                }
            }
            if(!cap.writer_invariants_ok) {
                fprintf(stderr,
                        "ERROR: the indexer's output would break "
                        "journalfile_migrate_to_v2_callback()\n");
                errors++;
            }
            if(cap.extents != expected_extents) {
                // A page that never entered the index must not leave an extent
                // behind: journalfile_v2_write_extent_list() emits one
                // journal_extent_list per extent and the file is sized from this
                // count, so an empty extent inflates the journal and
                // stats->extents_pages.
                fprintf(stderr,
                        "ERROR: the indexer reported %zu extents (expected %zu)\n",
                        cap.extents, expected_extents);
                errors++;
            }
            if(cap.pages_in_judy == 1 && cap.only_extent_index >= cap.extents) {
                // extent indices are array positions in the written journal
                fprintf(stderr,
                        "ERROR: the surviving page points at extent index %u, but only "
                        "%zu extents are written - compaction left a stale index\n",
                        cap.only_extent_index, cap.extents);
                errors++;
            }
            if(expected_end_time_s && cap.only_end_time_s != expected_end_time_s) {
                fprintf(stderr,
                        "ERROR: the indexer kept the page ending at %ld (expected %ld) - "
                        "the surviving page must be chosen deterministically, not by the "
                        "order the hot queue happens to yield\n",
                        (long)cap.only_end_time_s, (long)expected_end_time_s);
                errors++;
            }
        }
    }

    if(p_old) pgc_page_release(cache, p_old);
    if(p_new) pgc_page_release(cache, p_new);

    // reached only on the success path; the goto above jumps straight to cleanup_early
    if(second) mrg_metric_release_and_delete(mrg, second);
    if(decoy) mrg_metric_release_and_delete(mrg, decoy);

cleanup_early:
    pgc_destroy(cache, false);
    if(pinned) uuidmap_free(pinned);
    main_mrg = saved_main_mrg;

    size_t referenced = mrg_destroy(mrg);
    if(referenced) {
        fprintf(stderr, "ERROR: %zu metrics still referenced - the MRG was not destroyed\n", referenced);
        errors++;
    }

    if(errors)
        fprintf(stderr, "jv2 same-uuid grouping test (%s): %d ERROR(S)\n", label, errors);
    else
        fprintf(stderr, "jv2 same-uuid grouping test (%s): OK\n", label);

    return errors;
}

// The mkdtemp() template appended to the temporary directory.
#define JV2_TMPDIR_TEMPLATE "/netdata-jv2-writer-XXXXXX"

// Longest name journalfile_v2_generate_path() appends to dbfiles_path:
// "/" WALFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL WALFILE_EXTENSION_V2 is ~31 bytes.
// Round up generously - being wrong here means a silently truncated journal path.
#define JV2_JOURNAL_NAME_MAX 64

// Is this directory safe to build the test's temporary tree in?
//
// TMPDIR comes from the environment, so it is not trusted. The value ends up in
// mkdtemp() and then in ctx.config.dbfiles_path, where journalfile_v2_generate_path()
// snprintfz()es a filename onto it - so a bad value means creating directories in an
// unexpected place, or a truncated path that the unlink() in teardown would then miss.
//
// Checks, in order:
//   - non-empty, and absolute, so nothing is ever created relative to the cwd;
//   - no control characters, so a mangled environment cannot produce odd filenames;
//   - no ".." anywhere, so the path cannot be walked outside the directory it names;
//   - an existing directory we may create in (W_OK|X_OK), rather than a path we would
//     only discover was unusable after mkdtemp(). Symlinks are followed on purpose:
//     /tmp itself is a symlink on macOS, so rejecting them would reject the fallback;
//   - short enough that the template AND the journal filename still fit in every
//     buffer downstream.
//
// Note this is a TEST-side guard, not a security boundary: whoever sets TMPDIR already
// runs this binary. It exists so a hostile-looking or simply broken TMPDIR degrades to
// /tmp instead of producing confusing failures or stray files.
static bool jv2_tmpdir_is_usable(const char *dir, size_t dst_size) {
    if(!dir || !*dir)
        return false;

    if(dir[0] != '/')
        return false;

    // the filesystem root is not a temporary directory, however writable it is
    if(!dir[strspn(dir, "/")])
        return false;

    const size_t len = strlen(dir);
    const size_t needed = len + sizeof(JV2_TMPDIR_TEMPLATE) + JV2_JOURNAL_NAME_MAX;
    if(needed > dst_size || needed > RRDENG_PATH_MAX || needed > FILENAME_MAX)
        return false;

    for(size_t i = 0; i < len; i++) {
        unsigned char c = (unsigned char)dir[i];
        if(c < 0x20 || c == 0x7f)
            return false;
    }

    if(strstr(dir, ".."))
        return false;

    struct stat st;
    if(stat(dir, &st) != 0 || !S_ISDIR(st.st_mode))
        return false;

    if(access(dir, W_OK | X_OK) != 0)
        return false;

    return true;
}

// Create the test's temporary directory under a sanitized TMPDIR, falling back to
// /tmp. Returns false only if neither can be used, in which case the caller skips
// rather than fails - an unusable /tmp is an environment problem, not a defect in the
// code under test.
static bool jv2_make_tmpdir(char *dst, size_t dst_size) {
    const char *base = getenv("TMPDIR");

    if(!jv2_tmpdir_is_usable(base, dst_size)) {
        if(base && *base)
            fprintf(stderr, "TMPDIR is not usable for this test; falling back to /tmp\n");
        base = "/tmp";

        if(!jv2_tmpdir_is_usable(base, dst_size))
            return false;
    }

    // trim trailing slashes so we never build a path containing "//"
    size_t len = strlen(base);
    while(len > 1 && base[len - 1] == '/')
        len--;

    snprintfz(dst, dst_size, "%.*s" JV2_TMPDIR_TEMPLATE, (int)len, base);

    return mkdtemp(dst) != NULL;
}

// Drives the REAL writer end to end: journalfile_migrate_to_v2_callback() computes the
// file size, writes the extent/metric/page structures, checksums them, and activates
// the result through journalfile_v2_data_set(). We then read the SERIALIZED bytes back.
//
// The stub-callback cases above prove the indexer's in-memory output. This proves the
// bytes it produces are the ones the reader expects - the offset arithmetic in
// journalfile.c is driven by counters this change rewrote (mi->number_of_pages and the
// extent indices), and a stub callback cannot exercise any of it.
//
// The collision is the interesting input: two pages of one uuid at one start time on
// two different extents, so the writer must emit exactly one metric, one page and one
// extent, with the surviving page's descriptor pointing at a valid extent.
static int mrg_jv2_real_writer_unittest(void) {
    fprintf(stderr, "\nTesting jv2 real writer publishes and reloads (deterministic)...\n");
    int errors = 0;

    enum { TEST_FILENO = 11 };
    enum { START_TIME = 100, END_SHORT = 200, END_LONG = 400 };

    char dbpath[FILENAME_MAX + 1];
    if(!jv2_make_tmpdir(dbpath, sizeof(dbpath))) {
        // Neither TMPDIR nor /tmp is usable. That is an environment problem, not a defect in
        // the code under test, and jv2_make_tmpdir() documents that its caller skips on it -
        // so report the skip and succeed rather than failing the whole MRG unit test.
        fprintf(stderr, "SKIPPED: no usable temporary directory for the journal\n");
        return 0;
    }

    // A zeroed fixture is NOT enough: activation validates the datafile and takes
    // locks that must be constructed, not merely zeroed. Mirror what production does.
    //
    //   - initialize_single_ctx() (rrdengineapi.c) builds the two ctx locks. It is
    //     static, so they are constructed here: njfv2idx.spinlock is taken by
    //     njfv2idx_add(), which journalfile_v2_data_set() calls on activation.
    //   - datafile_alloc_and_init() (datafile.c) stamps DATAFILE_MAGIC and builds the
    //     datafile locks. It is static AND asserts tier == 1, so it is replicated.
    //     Without the magic, datafile_ctx() fatals with "invalid magic" as soon as the
    //     writer resolves the ctx from the datafile.
    //   - journalfile_alloc_and_init() IS exported, so it is used rather than
    //     hand-rolled: it builds data_spinlock (taken by journalfile_v2_data_set()),
    //     builds unsafe.spinlock (read by journalfile_current_size()), sets
    //     mmap.fd = -1, and links itself to the datafile.
    struct rrdengine_instance ctx;
    memset(&ctx, 0, sizeof(ctx));
    ctx.config.tier = 0;
    strncpyz(ctx.config.dbfiles_path, dbpath, sizeof(ctx.config.dbfiles_path) - 1);
    fatal_assert(0 == netdata_rwlock_init(&ctx.datafiles.rwlock));
    rw_spinlock_init(&ctx.njfv2idx.spinlock);

    struct rrdengine_datafile datafile;
    memset(&datafile, 0, sizeof(datafile));
    datafile.tier = 1;              // datafile_alloc_and_init() asserts exactly this
    datafile.fileno = TEST_FILENO;
    datafile.ctx = &ctx;
    datafile.magic1 = datafile.magic2 = DATAFILE_MAGIC;
    datafile.users.available = true;
    fatal_assert(0 == netdata_rwlock_init(&datafile.extent_rwlock));
    spinlock_tracked_init(&datafile.users.spinlock);
    spinlock_init(&datafile.writers.spinlock);
    rw_spinlock_init(&datafile.extent_epdl.spinlock);

    struct rrdengine_journalfile *journalfile = journalfile_alloc_and_init(&datafile);

    const Word_t section = (Word_t)&ctx;

    MRG *mrg = mrg_create_for_unittest();
    MRG *saved_main_mrg = main_mrg;
    main_mrg = mrg;

    PGC *cache = pgc_create(
        "jv2-real-writer-test",
        32 * 1024 * 1024, jv2_stale_metric_free_clean_cb,
        64, NULL, jv2_stale_metric_save_dirty_cb,
        10, 10, 1000, 10,
        PGC_OPTIONS_DEFAULT, 1, sizeof(struct extent_io_data));

    // everything the cleanup path can touch, and everything the gotos below jump
    // over, is declared and initialized up front
    struct jv2_two_pointers tp = { 0 };
    PGC_PAGE *p_old = NULL, *p_new = NULL;
    bool published = false;
    bool a1 = false, a2 = false;
    struct extent_io_data xio_old = { 0 }, xio_new = { 0 };

    if(jv2_make_two_pointers_one_uuid(mrg, section, &tp)) {
        errors++;
        goto cleanup;
    }

    // two different extents, so the rejected page's extent must not be serialized
    xio_old.fileno = TEST_FILENO; xio_old.block = 4096;
    xio_old.bytes = 4096; xio_old.uuid_id = tp.shared_id;

    xio_new.fileno = TEST_FILENO; xio_new.block = 8192;
    xio_new.bytes = 4096; xio_new.uuid_id = tp.shared_id;
    p_old = pgc_page_add_and_acquire(cache, (PGC_ENTRY){
        .section = section, .metric_id = (Word_t)tp.stale_ptr,
        .start_time_s = START_TIME, .end_time_s = END_SHORT, .size = 4096, .data = &datafile,
        .update_every_s = 1, .hot = true, .custom_data = (uint8_t *)&xio_old,
    }, &a1);
    p_new = pgc_page_add_and_acquire(cache, (PGC_ENTRY){
        .section = section, .metric_id = (Word_t)mrg_metric_id(mrg, tp.live),
        .start_time_s = START_TIME, .end_time_s = END_LONG, .size = 4096, .data = &datafile,
        .update_every_s = 1, .hot = true, .custom_data = (uint8_t *)&xio_new,
    }, &a2);

    if(!p_old || !p_new || !a1 || !a2) {
        fprintf(stderr, "ERROR: cannot add the two hot pages\n");
        errors++;
        goto cleanup;
    }

    pgc_open_cache_to_journal_v2(cache, section, TEST_FILENO, 1,
                                 journalfile_migrate_to_v2_callback, journalfile, true);

    if(!journalfile_v2_data_available(journalfile)) {
        fprintf(stderr, "ERROR: the writer did not activate a v2 journal\n");
        errors++;
        goto cleanup;
    }
    published = true;

    // Read the serialized structures back through the same mmap the reader uses.
    {
        struct journal_v2_header *j2 = journalfile_v2_data_acquire(journalfile, NULL, START_TIME, END_LONG);
        if(!j2) {
            fprintf(stderr, "ERROR: cannot acquire the published v2 journal data\n");
            errors++;
        }
        else {
            if(j2->magic != JOURVAL_V2_MAGIC) {
                fprintf(stderr, "ERROR: published journal has magic 0x%x\n", j2->magic);
                errors++;
            }
            if(j2->metric_count != 1) {
                fprintf(stderr, "ERROR: published journal has %u metrics (expected 1)\n",
                        j2->metric_count);
                errors++;
            }
            if(j2->page_count != 1) {
                fprintf(stderr, "ERROR: published journal has %u pages (expected 1) - the "
                                "colliding duplicate must not be serialized\n", j2->page_count);
                errors++;
            }
            if(j2->extent_count != 1) {
                fprintf(stderr, "ERROR: published journal has %u extents (expected 1) - the "
                                "rejected page's extent must not be serialized\n",
                        j2->extent_count);
                errors++;
            }

            if(!errors) {
                // the single metric's single page descriptor must be the SURVIVOR
                struct journal_metric_list *metric_list =
                    (void *)((uint8_t *)j2 + j2->metric_offset);
                time_t journal_start_s = (time_t)(j2->start_time_ut / USEC_PER_SEC);

                struct journal_page_header *page_header =
                    (void *)((uint8_t *)j2 + metric_list[0].page_offset);
                struct journal_page_list *page_list =
                    (void *)((uint8_t *)page_header + sizeof(*page_header));

                // delta_end_s is what the reader turns back into the page end
                time_t got_end_s = journal_start_s + (time_t)page_list[0].delta_end_s;

                if(page_header->entries != 1) {
                    fprintf(stderr, "ERROR: the metric's page header claims %u entries "
                                    "(expected 1)\n", page_header->entries);
                    errors++;
                }
                if(got_end_s != END_LONG) {
                    fprintf(stderr, "ERROR: the serialized page ends at %ld (expected %d) - "
                                    "the writer kept the wrong duplicate\n",
                            (long)got_end_s, END_LONG);
                    errors++;
                }
                if(page_list[0].extent_index >= j2->extent_count) {
                    fprintf(stderr, "ERROR: the serialized page points at extent %u of %u\n",
                            page_list[0].extent_index, j2->extent_count);
                    errors++;
                }
            }

            journalfile_v2_data_release(journalfile);
        }
    }

cleanup:
    if(p_old) pgc_page_release(cache, p_old);
    if(p_new) pgc_page_release(cache, p_new);

    if(tp.live) mrg_metric_release_and_delete(mrg, tp.live);
    tp.live = NULL;
    if(tp.decoy) mrg_metric_release_and_delete(mrg, tp.decoy);
    tp.decoy = NULL;

    pgc_destroy(cache, false);
    jv2_two_pointers_undo(mrg, &tp);
    main_mrg = saved_main_mrg;

    size_t referenced = mrg_destroy(mrg);
    if(referenced) {
        fprintf(stderr, "ERROR: %zu metrics still referenced - the MRG was not destroyed\n",
                referenced);
        errors++;
    }

    // journalfile_close() removes the datafile from ctx.njfv2idx and unmaps the file;
    // only valid once activation succeeded, otherwise it would close an unset uv_file.
    if(published)
        journalfile_close(journalfile, &datafile);
    freez(journalfile);

    netdata_rwlock_destroy(&datafile.extent_rwlock);
    netdata_rwlock_destroy(&ctx.datafiles.rwlock);

    {
        char path[FILENAME_MAX + 1];
        journalfile_v2_generate_path(&datafile, path, sizeof(path));
        unlink(path);
        rmdir(dbpath);
    }

    if(errors)
        fprintf(stderr, "jv2 real writer test: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "jv2 real writer test: OK\n");

    return errors;
}

static int mrg_jv2_same_uuid_grouped_once_unittest(void) {
    // distinct start times: one group, both pages kept
    int errors = mrg_jv2_same_uuid_grouped_once_check(
        "distinct start times", 100, 300, 200, 400, 2, 0, 2, false);

    // Identical start times: the two pages of the same uuid collide inside the
    // group. Before this was handled, the second page hit
    // internal_fatal("Page is already in JudyL metric pages") and, with internal
    // checks off, still counted itself into mi->number_of_pages while never
    // entering the JudyL - which made the journal v2 writer discard the migration.
    //
    // The two pages are NOT interchangeable, so assert WHICH one survives: the one
    // covering more time.
    //
    // Both cases plant p_old first and p_new second, and the hot queue is walked in
    // insertion order, so the page VISITED first is always the same. What the two
    // cases swap is which page carries the larger end_time_s - that is what makes
    // them discriminating: a comparator degenerating to "first visited always wins"
    // fails one case, "last visited always wins" fails the other, and an inverted
    // comparison fails both.
    //
    // Swapping it also drives both sides of the extent bookkeeping, because the two
    // pages sit on DIFFERENT extent blocks and exactly one extent must be reported
    // either way. When the larger page comes second it REPLACES an already indexed
    // page, so the extent already created for the loser has to be dropped and the
    // survivors renumbered; when it comes first the loser is rejected before it ever
    // creates an extent.
    errors += mrg_jv2_same_uuid_grouped_once_check(
        "colliding start times, longer page second", 100, 100, 200, 400, 1, 400, 1, false);
    errors += mrg_jv2_same_uuid_grouped_once_check(
        "colliding start times, longer page first", 100, 100, 400, 200, 1, 400, 1, false);

    // Same collision, but the callback now reports success, so the indexer really
    // publishes the journal and has to dispose of the page it dropped. A dropped
    // page left hot would hold its DATAFILE_ACQUIRE_OPEN_CACHE reference forever
    // and its datafile could never be deleted.
    errors += mrg_jv2_same_uuid_grouped_once_check(
        "colliding start times, journal published", 100, 100, 200, 400, 1, 400, 1, true);

    return errors;
}

int mrg_unittest(void) {
    int errors = dbengine_accounting_helpers_unittest();
    errors += mrg_destroy_referenced_metric_unittest();
    errors += mrg_stale_peeked_id_unittest();
    errors += mrg_stale_metric_pointer_unittest();
    errors += mrg_jv2_stale_metric_not_dereferenced_unittest();
    errors += mrg_jv2_same_uuid_grouped_once_unittest();
    errors += mrg_jv2_real_writer_unittest();
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
