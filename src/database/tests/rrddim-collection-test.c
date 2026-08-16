// SPDX-License-Identifier: GPL-3.0-or-later

#define rrdeng_store_metric_next collection_test_dbengine_store
#define rrddim_collect_store_metric collection_test_ram_store
#define rrdeng_metric_oldest_time collection_test_oldest_time
#define rrddim_query_oldest_time_s collection_test_oldest_time
#define rrdeng_metric_latest_time collection_test_latest_time
#define rrddim_query_latest_time_s collection_test_latest_time
#define rrdeng_load_metric_init collection_test_query_init
#define rrddim_query_init collection_test_query_init
#define rrdeng_load_metric_next collection_test_query_next
#define rrddim_query_next_metric collection_test_query_next
#define rrdeng_load_metric_is_finished collection_test_query_is_finished
#define rrddim_query_is_finished collection_test_query_is_finished
#define rrdeng_load_metric_finalize collection_test_query_finalize
#define rrddim_query_finalize collection_test_query_finalize
#define rrdcontext_collected_rrddim collection_test_context_collected
#include "../rrddim-collection.c"
#include "../rrddim-backfill.c"
#undef rrdcontext_collected_rrddim
#undef rrddim_query_finalize
#undef rrdeng_load_metric_finalize
#undef rrddim_query_is_finished
#undef rrdeng_load_metric_is_finished
#undef rrddim_query_next_metric
#undef rrdeng_load_metric_next
#undef rrddim_query_init
#undef rrdeng_load_metric_init
#undef rrddim_query_latest_time_s
#undef rrdeng_metric_latest_time
#undef rrddim_query_oldest_time_s
#undef rrdeng_metric_oldest_time
#undef rrddim_collect_store_metric
#undef rrdeng_store_metric_next

#define TEST_EXPECT(condition) do { \
    if(!(condition)) { \
        fprintf(stderr, "%s:%d: %s\n", __FUNCTION__, __LINE__, #condition); \
        errors++; \
    } \
} while(0)

struct nd_profile_t nd_profile = { 0 };
RRD_BACKFILL default_backfill = RRD_BACKFILL_FULL;
__thread size_t rrdset_done_statistics_points_stored_per_tier[RRD_STORAGE_TIERS];

struct captured_store {
    usec_t point_in_time_ut;
    NETDATA_DOUBLE n;
    NETDATA_DOUBLE min;
    NETDATA_DOUBLE max;
    uint16_t count;
    uint16_t anomaly_count;
    SN_FLAGS flags;
};

static struct captured_store captured_stores[16];
static size_t captured_store_count;
static size_t context_collected_count;
static size_t backfill_started_count;
static size_t backfill_finished_count;
static size_t backfill_completed_count;
static size_t backfill_points_read;
static size_t collection_completed_count;

static void capture_store(
    usec_t point_in_time_ut,
    NETDATA_DOUBLE n,
    NETDATA_DOUBLE min,
    NETDATA_DOUBLE max,
    uint16_t count,
    uint16_t anomaly_count,
    SN_FLAGS flags) {
    if(captured_store_count >= _countof(captured_stores))
        fatal("too many captured stores");

    captured_stores[captured_store_count++] = (struct captured_store){
        .point_in_time_ut = point_in_time_ut,
        .n = n,
        .min = min,
        .max = max,
        .count = count,
        .anomaly_count = anomaly_count,
        .flags = flags,
    };
}

void collection_test_dbengine_store(
    STORAGE_COLLECT_HANDLE *sch __maybe_unused,
    usec_t point_in_time_ut,
    NETDATA_DOUBLE n,
    NETDATA_DOUBLE min,
    NETDATA_DOUBLE max,
    uint16_t count,
    uint16_t anomaly_count,
    SN_FLAGS flags) {
    capture_store(point_in_time_ut, n, min, max, count, anomaly_count, flags);
}

void collection_test_ram_store(
    STORAGE_COLLECT_HANDLE *sch __maybe_unused,
    usec_t point_in_time_ut,
    NETDATA_DOUBLE n,
    NETDATA_DOUBLE min,
    NETDATA_DOUBLE max,
    uint16_t count,
    uint16_t anomaly_count,
    SN_FLAGS flags) {
    capture_store(point_in_time_ut, n, min, max, count, anomaly_count, flags);
}

void collection_test_context_collected(RRDDIM *rd __maybe_unused) {
    context_collected_count++;
}

void stream_control_backfill_query_started(void) {
    backfill_started_count++;
}

void stream_control_backfill_query_finished(void) {
    backfill_finished_count++;
}

void pulse_queries_backfill_query_completed(size_t points_read) {
    backfill_completed_count++;
    backfill_points_read += points_read;
}

void pulse_queries_rrdset_collection_completed(size_t *points_read_per_tier_array __maybe_unused) {
    collection_completed_count++;
}

struct collection_test_metric {
    const STORAGE_POINT *points;
    size_t points_count;
    size_t next_point;
    time_t oldest_time_s;
    time_t latest_time_s;
};

time_t collection_test_oldest_time(STORAGE_METRIC_HANDLE *smh) {
    return ((struct collection_test_metric *)smh)->oldest_time_s;
}

time_t collection_test_latest_time(STORAGE_METRIC_HANDLE *smh) {
    return ((struct collection_test_metric *)smh)->latest_time_s;
}

void collection_test_query_init(
    STORAGE_METRIC_HANDLE *smh,
    struct storage_engine_query_handle *seqh,
    time_t start_time_s,
    time_t end_time_s,
    STORAGE_PRIORITY priority) {
    struct collection_test_metric *metric = (struct collection_test_metric *)smh;
    metric->next_point = 0;
    *seqh = (struct storage_engine_query_handle){
        .start_time_s = start_time_s,
        .end_time_s = end_time_s,
        .priority = priority,
#ifdef ENABLE_DBENGINE
        .seb = STORAGE_ENGINE_BACKEND_DBENGINE,
#else
        .seb = STORAGE_ENGINE_BACKEND_RRDDIM,
#endif
        .handle = (STORAGE_QUERY_HANDLE *)metric,
    };
}

STORAGE_POINT collection_test_query_next(struct storage_engine_query_handle *seqh) {
    struct collection_test_metric *metric = (struct collection_test_metric *)seqh->handle;
    if(metric->next_point >= metric->points_count)
        return STORAGE_POINT_UNSET;

    return metric->points[metric->next_point++];
}

int collection_test_query_is_finished(struct storage_engine_query_handle *seqh) {
    struct collection_test_metric *metric = (struct collection_test_metric *)seqh->handle;
    return metric->next_point >= metric->points_count;
}

void collection_test_query_finalize(struct storage_engine_query_handle *seqh __maybe_unused) {
    // The mock query borrows fixture storage and owns no resources.
}

static STORAGE_POINT numeric_point(
    time_t start_time_s,
    time_t end_time_s,
    NETDATA_DOUBLE min,
    NETDATA_DOUBLE max,
    NETDATA_DOUBLE sum,
    uint32_t count,
    uint32_t gap_count,
    uint32_t anomaly_count,
    SN_FLAGS flags) {
    return (STORAGE_POINT){
        .min = min,
        .max = max,
        .sum = sum,
        .start_time_s = start_time_s,
        .end_time_s = end_time_s,
        .count = count,
        .anomaly_count = anomaly_count,
        .flags = flags,
        .gap_count = gap_count,
    };
}

static STORAGE_POINT gap_point(time_t start_time_s, time_t end_time_s, NETDATA_DOUBLE payload, uint32_t gaps) {
    return (STORAGE_POINT){
        .min = payload,
        .max = payload,
        .sum = payload,
        .start_time_s = start_time_s,
        .end_time_s = end_time_s,
        .count = 0,
        .anomaly_count = 0,
        .flags = SN_FLAG_RESET,
        .gap_count = gaps,
    };
}

static bool point_is_fully_unset(STORAGE_POINT sp) {
    return storage_point_is_unset(sp) &&
        isnan(sp.min) && isnan(sp.max) && isnan(sp.sum) &&
        sp.start_time_s == 0 && sp.end_time_s == 0 &&
        sp.count == 0 && sp.gap_count == 0 && sp.anomaly_count == 0 &&
        sp.flags == SN_FLAG_NONE;
}

static size_t test_completed_point_persistence(void) {
    size_t errors = 0;
    STORAGE_COLLECT_HANDLE sch = { .seb = STORAGE_ENGINE_BACKEND_RRDDIM };
    struct rrddim_tier tier = { .sch = &sch };

    const STORAGE_POINT cases[] = {
        numeric_point(100, 160, 1, 3, 6, 3, 0, 1, SN_FLAG_RESET),
        numeric_point(160, 220, 2, 4, 6, 2, 3, 1, SN_DEFAULT_FLAGS),
        gap_point(220, 280, NAN, 60),
        gap_point(280, 340, INFINITY, 60),
        gap_point(340, 400, -INFINITY, 60),
    };

    captured_store_count = 0;
    memset(rrdset_done_statistics_points_stored_per_tier, 0,
           sizeof(rrdset_done_statistics_points_stored_per_tier));

    for(size_t i = 0; i < _countof(cases); i++) {
        tier.last_completed_point = cases[i];
        store_metric_at_tier_flush_last_completed(NULL, 1, &tier);
        TEST_EXPECT(point_is_fully_unset(tier.last_completed_point));
    }

    TEST_EXPECT(captured_store_count == _countof(cases));
    TEST_EXPECT(rrdset_done_statistics_points_stored_per_tier[1] == _countof(cases));

    for(size_t i = 0; i < 2 && i < captured_store_count; i++) {
        TEST_EXPECT(captured_stores[i].point_in_time_ut == (usec_t)cases[i].end_time_s * USEC_PER_SEC);
        TEST_EXPECT(captured_stores[i].n == cases[i].sum);
        TEST_EXPECT(captured_stores[i].min == cases[i].min);
        TEST_EXPECT(captured_stores[i].max == cases[i].max);
        TEST_EXPECT(captured_stores[i].count == cases[i].count);
        TEST_EXPECT(captured_stores[i].anomaly_count == cases[i].anomaly_count);
        TEST_EXPECT(captured_stores[i].flags == cases[i].flags);
    }

    for(size_t i = 2; i < _countof(cases) && i < captured_store_count; i++) {
        TEST_EXPECT(captured_stores[i].point_in_time_ut == (usec_t)cases[i].end_time_s * USEC_PER_SEC);
        TEST_EXPECT(isnan(captured_stores[i].n));
        TEST_EXPECT(isnan(captured_stores[i].min));
        TEST_EXPECT(isnan(captured_stores[i].max));
        TEST_EXPECT(captured_stores[i].count == 0);
        TEST_EXPECT(captured_stores[i].anomaly_count == 0);
        TEST_EXPECT(captured_stores[i].flags == SN_FLAG_NONE);
    }

    return errors;
}

static size_t test_one_tier_fast_path(void) {
    size_t errors = 0;
    STORAGE_COLLECT_HANDLE tier0_sch = { .seb = STORAGE_ENGINE_BACKEND_RRDDIM };
    STORAGE_COLLECT_HANDLE tier1_sch = { .seb = STORAGE_ENGINE_BACKEND_RRDDIM };
    RRDSET st = { .update_every = 1 };
    RRDDIM *rd = callocz(1, sizeof(*rd) + 2 * sizeof(struct rrddim_tier));
    rd->id = string_strdupz("collection-one-tier");
    rd->rrdset = &st;
    rd->tiers[0].sch = &tier0_sch;
    rd->tiers[1].sch = &tier1_sch;
    rd->tiers[1].smh = (STORAGE_METRIC_HANDLE *)(uintptr_t)1;
    rd->tiers[1].tier_grouping = 60;
    rd->tiers[1].last_completed_point_flush_modulo = 1;
    storage_point_unset(rd->tiers[1].virtual_point);
    storage_point_unset(rd->tiers[1].last_completed_point);
    rrddim_option_set(rd, RRDDIM_OPTION_BACKFILLED_HIGH_TIERS);

    nd_profile.storage_tiers = 1;
    captured_store_count = 0;
    context_collected_count = 0;
    rrddim_store_metric(rd, 200 * USEC_PER_SEC, 42, SN_DEFAULT_FLAGS);

    TEST_EXPECT(captured_store_count == 1);
    if(captured_store_count == 1) {
        TEST_EXPECT(captured_stores[0].point_in_time_ut == 200 * USEC_PER_SEC);
        TEST_EXPECT(captured_stores[0].n == 42);
        TEST_EXPECT(captured_stores[0].count == 1);
    }
    TEST_EXPECT(context_collected_count == 1);
    TEST_EXPECT(point_is_fully_unset(rd->tiers[1].virtual_point));
    TEST_EXPECT(point_is_fully_unset(rd->tiers[1].last_completed_point));
    TEST_EXPECT(rd->tiers[1].next_point_end_time_s == 0);

    string_freez(rd->id);
    freez(rd);
    return errors;
}

static size_t test_ordinary_higher_tier_rollup(void) {
    size_t errors = 0;
    STORAGE_COLLECT_HANDLE tier0_sch = { .seb = STORAGE_ENGINE_BACKEND_RRDDIM };
    STORAGE_COLLECT_HANDLE tier1_sch = { .seb = STORAGE_ENGINE_BACKEND_RRDDIM };
    RRDSET st = { .update_every = 10 };
    RRDDIM *rd = callocz(1, sizeof(*rd) + 2 * sizeof(struct rrddim_tier));
    rd->id = string_strdupz("collection-higher-tier");
    rd->rrdset = &st;
    rd->tiers[0].sch = &tier0_sch;
    rd->tiers[1].sch = &tier1_sch;
    rd->tiers[1].smh = (STORAGE_METRIC_HANDLE *)(uintptr_t)1;
    rd->tiers[1].tier_grouping = 3;
    rd->tiers[1].last_completed_point_flush_modulo = 1;
    storage_point_unset(rd->tiers[1].virtual_point);
    storage_point_unset(rd->tiers[1].last_completed_point);
    rrddim_option_set(rd, RRDDIM_OPTION_BACKFILLED_HIGH_TIERS);

    nd_profile.storage_tiers = 2;
    captured_store_count = 0;
    context_collected_count = 0;

    rrddim_store_metric(rd, 70 * USEC_PER_SEC, INFINITY, SN_FLAG_RESET);
    TEST_EXPECT(storage_point_is_gap(rd->tiers[1].virtual_point));
    TEST_EXPECT(isnan(rd->tiers[1].virtual_point.min));
    TEST_EXPECT(isnan(rd->tiers[1].virtual_point.max));
    TEST_EXPECT(isnan(rd->tiers[1].virtual_point.sum));
    TEST_EXPECT(rd->tiers[1].virtual_point.flags == SN_FLAG_NONE);
    rrddim_store_metric(rd, 80 * USEC_PER_SEC, NAN, SN_FLAG_NONE);
    rrddim_store_metric(rd, 90 * USEC_PER_SEC, -INFINITY, SN_FLAG_NONE);

    struct rrddim_tier *t = &rd->tiers[1];
    TEST_EXPECT(t->next_point_end_time_s == 90);
    TEST_EXPECT(storage_point_is_gap(t->virtual_point));
    TEST_EXPECT(t->virtual_point.start_time_s == 60 && t->virtual_point.end_time_s == 90);
    TEST_EXPECT(isnan(t->virtual_point.min));
    TEST_EXPECT(isnan(t->virtual_point.max));
    TEST_EXPECT(isnan(t->virtual_point.sum));
    TEST_EXPECT(t->virtual_point.count == 0 && t->virtual_point.gap_count == 3);
    TEST_EXPECT(t->virtual_point.anomaly_count == 0 && t->virtual_point.flags == SN_FLAG_NONE);

    rrddim_store_metric(rd, 100 * USEC_PER_SEC, 4, SN_DEFAULT_FLAGS);
    TEST_EXPECT(t->next_point_end_time_s == 120);
    TEST_EXPECT(storage_point_is_gap(t->last_completed_point));
    TEST_EXPECT(t->last_completed_point.start_time_s == 60 && t->last_completed_point.end_time_s == 90);
    TEST_EXPECT(isnan(t->last_completed_point.min));
    TEST_EXPECT(isnan(t->last_completed_point.max));
    TEST_EXPECT(isnan(t->last_completed_point.sum));
    TEST_EXPECT(t->last_completed_point.count == 0 && t->last_completed_point.gap_count == 3);
    TEST_EXPECT(t->last_completed_point.anomaly_count == 0 && t->last_completed_point.flags == SN_FLAG_NONE);
    TEST_EXPECT(storage_point_is_complete(t->virtual_point));
    TEST_EXPECT(t->virtual_point.start_time_s == 90 && t->virtual_point.end_time_s == 100);
    TEST_EXPECT(t->virtual_point.min == 4 && t->virtual_point.max == 4 && t->virtual_point.sum == 4);
    TEST_EXPECT(t->virtual_point.count == 1 && t->virtual_point.gap_count == 0);
    TEST_EXPECT(t->virtual_point.anomaly_count == 0 && t->virtual_point.flags == SN_DEFAULT_FLAGS);

    store_metric_at_tier_flush_last_completed(rd, 1, t);
    TEST_EXPECT(captured_store_count == 5);
    if(captured_store_count == 5) {
        const struct captured_store *gap = &captured_stores[4];
        TEST_EXPECT(gap->point_in_time_ut == 90 * USEC_PER_SEC);
        TEST_EXPECT(isnan(gap->n) && isnan(gap->min) && isnan(gap->max));
        TEST_EXPECT(gap->count == 0 && gap->anomaly_count == 0 && gap->flags == SN_FLAG_NONE);
    }
    TEST_EXPECT(point_is_fully_unset(t->last_completed_point));

    rrddim_store_metric(rd, 110 * USEC_PER_SEC, 6, SN_FLAG_RESET);
    rrddim_store_metric(rd, 120 * USEC_PER_SEC, NAN, SN_FLAG_NONE);
    rrddim_store_metric(rd, 130 * USEC_PER_SEC, 8, SN_DEFAULT_FLAGS);

    TEST_EXPECT(t->next_point_end_time_s == 150);
    TEST_EXPECT(storage_point_is_partial(t->last_completed_point));
    TEST_EXPECT(t->last_completed_point.start_time_s == 90 && t->last_completed_point.end_time_s == 120);
    TEST_EXPECT(t->last_completed_point.min == 4 && t->last_completed_point.max == 6 &&
                t->last_completed_point.sum == 10);
    TEST_EXPECT(t->last_completed_point.count == 2 && t->last_completed_point.gap_count == 1);
    TEST_EXPECT(t->last_completed_point.anomaly_count == 1 &&
                t->last_completed_point.flags == (SN_DEFAULT_FLAGS | SN_FLAG_RESET));
    TEST_EXPECT(storage_point_is_complete(t->virtual_point));
    TEST_EXPECT(t->virtual_point.start_time_s == 120 && t->virtual_point.end_time_s == 130);
    TEST_EXPECT(t->virtual_point.min == 8 && t->virtual_point.max == 8 && t->virtual_point.sum == 8);
    TEST_EXPECT(t->virtual_point.count == 1 && t->virtual_point.gap_count == 0);
    TEST_EXPECT(t->virtual_point.anomaly_count == 0 && t->virtual_point.flags == SN_DEFAULT_FLAGS);

    store_metric_at_tier_flush_last_completed(rd, 1, t);
    TEST_EXPECT(captured_store_count == 9);
    if(captured_store_count == 9) {
        const struct captured_store *partial = &captured_stores[8];
        TEST_EXPECT(partial->point_in_time_ut == 120 * USEC_PER_SEC);
        TEST_EXPECT(partial->n == 10 && partial->min == 4 && partial->max == 6);
        TEST_EXPECT(partial->count == 2 && partial->anomaly_count == 1 &&
                    partial->flags == (SN_DEFAULT_FLAGS | SN_FLAG_RESET));
    }
    TEST_EXPECT(point_is_fully_unset(t->last_completed_point));
    TEST_EXPECT(context_collected_count == 7);

    string_freez(rd->id);
    freez(rd);
    return errors;
}

#ifdef ENABLE_DBENGINE
static size_t test_backfill_gap_composition(void) {
    size_t errors = 0;
    const time_t t = 600;
    const STORAGE_POINT source_points[] = {
        numeric_point(t, t + 10, 1, 1, 1, 1, 0, 0, SN_DEFAULT_FLAGS),
        numeric_point(t + 10, t + 20, 2, 2, 2, 1, 0, 0, SN_DEFAULT_FLAGS),
        numeric_point(t + 20, t + 21, 3, 3, 3, 1, 0, 0, SN_DEFAULT_FLAGS),
        gap_point(t + 21, t + 22, NAN, 1),
        numeric_point(t + 22, t + 23, 4, 4, 4, 1, 0, 0, SN_DEFAULT_FLAGS),
    };
    struct collection_test_metric source = {
        .points = source_points,
        .points_count = _countof(source_points),
        .oldest_time_s = t + 10,
        .latest_time_s = t + 23,
    };
    struct collection_test_metric target = { .oldest_time_s = 0, .latest_time_s = 0 };
    RRDSET st = { .update_every = 1 };
    RRDDIM *rd = callocz(1, sizeof(*rd) + 2 * sizeof(struct rrddim_tier));
    rd->rrdset = &st;
    rd->tiers[0].seb = STORAGE_ENGINE_BACKEND_DBENGINE;
    rd->tiers[0].smh = (STORAGE_METRIC_HANDLE *)&source;
    rd->tiers[1].seb = STORAGE_ENGINE_BACKEND_DBENGINE;
    rd->tiers[1].smh = (STORAGE_METRIC_HANDLE *)&target;
    rd->tiers[1].tier_grouping = 60;
    rd->tiers[1].last_completed_point_flush_modulo = 1;
    storage_point_unset(rd->tiers[1].virtual_point);
    storage_point_unset(rd->tiers[1].last_completed_point);

    nd_profile.storage_tiers = 2;
    default_backfill = RRD_BACKFILL_FULL;
    backfill_started_count = 0;
    backfill_finished_count = 0;
    backfill_completed_count = 0;
    backfill_points_read = 0;
    collection_completed_count = 0;

    bool backfilled = backfill_tier_from_smaller_tiers(rd, 1, t + 120);
    STORAGE_POINT sp = rd->tiers[1].virtual_point;
    TEST_EXPECT(backfilled);
    TEST_EXPECT(sp.start_time_s == t && sp.end_time_s == t + 23);
    TEST_EXPECT(sp.min == 1 && sp.max == 4 && sp.sum == 10);
    TEST_EXPECT(sp.count == 4 && sp.gap_count == 1 && sp.anomaly_count == 0);
    TEST_EXPECT(sp.flags == SN_DEFAULT_FLAGS);
    TEST_EXPECT(storage_point_is_partial(sp));
    TEST_EXPECT(storage_point_slots(sp) == 5);
    TEST_EXPECT(rd->tiers[1].next_point_end_time_s == t + 60);
    TEST_EXPECT(backfill_started_count == 1 && backfill_finished_count == 1);
    TEST_EXPECT(backfill_completed_count == 1 && backfill_points_read == _countof(source_points));
    TEST_EXPECT(collection_completed_count == 1);

    freez(rd);
    return errors;
}
#endif

int main(void) {
    size_t errors = 0;
    errors += test_completed_point_persistence();
    errors += test_one_tier_fast_path();
    errors += test_ordinary_higher_tier_rollup();
#ifdef ENABLE_DBENGINE
    errors += test_backfill_gap_composition();
#endif
    return errors ? 1 : 0;
}
