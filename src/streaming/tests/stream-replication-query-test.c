// SPDX-License-Identifier: GPL-3.0-or-later

#define rrdeng_load_metric_next stream_replication_test_next
#define rrdeng_load_metric_is_finished stream_replication_test_is_finished
#define rrdeng_load_align_to_optimal_before stream_replication_test_align_to_optimal_before
#include "../stream-replication-sender.c"
#undef rrdeng_load_align_to_optimal_before
#undef rrdeng_load_metric_is_finished
#undef rrdeng_load_metric_next

struct stream_replication_test_query {
    const STORAGE_POINT *points;
    size_t points_count;
    size_t next_point;
    time_t optimal_before;
};

static bool stream_replication_test_used_rrddim_backend;

STORAGE_POINT stream_replication_test_next(struct storage_engine_query_handle *seqh) {
    struct stream_replication_test_query *query = (struct stream_replication_test_query *)seqh->handle;
    if(query->next_point >= query->points_count)
        return STORAGE_POINT_UNSET;

    return query->points[query->next_point++];
}

int stream_replication_test_is_finished(struct storage_engine_query_handle *seqh) {
    struct stream_replication_test_query *query = (struct stream_replication_test_query *)seqh->handle;
    return query->next_point >= query->points_count;
}

time_t stream_replication_test_align_to_optimal_before(struct storage_engine_query_handle *seqh) {
    struct stream_replication_test_query *query = (struct stream_replication_test_query *)seqh->handle;
    return query->optimal_before;
}

STORAGE_POINT rrddim_query_next_metric(struct storage_engine_query_handle *seqh __maybe_unused) {
    stream_replication_test_used_rrddim_backend = true;
    return STORAGE_POINT_UNSET;
}

int rrddim_query_is_finished(struct storage_engine_query_handle *seqh __maybe_unused) {
    stream_replication_test_used_rrddim_backend = true;
    return 1;
}

time_t rrddim_query_align_to_optimal_before(struct storage_engine_query_handle *seqh) {
    stream_replication_test_used_rrddim_backend = true;
    return seqh->end_time_s;
}

static STORAGE_POINT stream_replication_test_point(
    time_t start_time_s, time_t end_time_s, NETDATA_DOUBLE value) {
    return (STORAGE_POINT){
        .min = value,
        .max = value,
        .sum = value,
        .start_time_s = start_time_s,
        .end_time_s = end_time_s,
        .count = 1,
        .anomaly_count = 0,
        .flags = SN_DEFAULT_FLAGS,
        .gap_count = 0,
    };
}

static STORAGE_POINT stream_replication_test_gap(time_t start_time_s, time_t end_time_s) {
    STORAGE_POINT sp;
    storage_point_empty(sp, start_time_s, end_time_s);
    return sp;
}

int main(void) {
    const STORAGE_POINT points_a[] = {
        stream_replication_test_point(100, 160, 1),
        stream_replication_test_point(160, 170, 2),
        stream_replication_test_point(170, 180, 3),
        stream_replication_test_point(180, 190, 4),
        stream_replication_test_point(190, 200, 5),
    };
    const STORAGE_POINT points_b[] = {
        stream_replication_test_point(100, 160, 11),
        stream_replication_test_point(160, 170, 12),
        stream_replication_test_gap(170, 180),
        stream_replication_test_point(180, 190, 14),
        stream_replication_test_point(190, 200, 15),
        stream_replication_test_point(200, 210, 16),
    };

    struct stream_replication_test_query query_a = {
        .points = points_a,
        .points_count = _countof(points_a),
        .optimal_before = 200,
    };
    struct stream_replication_test_query query_b = {
        .points = points_b,
        .points_count = _countof(points_b),
        .optimal_before = 210,
    };

    RRDSET st = { 0 };
    st.update_every = 10;
    st.last_updated.tv_sec = 300;

    RRDDIM *rd_a = callocz(1, sizeof(*rd_a));
    RRDDIM *rd_b = callocz(1, sizeof(*rd_b));
    rd_a->id = string_strdupz("a");
    rd_b->id = string_strdupz("b");

    struct replication_query *q = callocz(
        1, sizeof(*q) + 2 * sizeof(struct replication_dimension));
    q->st = &st;
    q->query.after = 100;
    q->query.before = 175;
    q->query.execute = true;
    q->query.enable_streaming = false;
    q->wall_clock_time = 1000;
    q->dimensions = 2;

    q->data[0].enabled = true;
    q->data[0].rd = rd_a;
    q->data[0].handle.seb = STORAGE_ENGINE_BACKEND_DBENGINE;
    q->data[0].handle.handle = (STORAGE_QUERY_HANDLE *)&query_a;

    q->data[1].enabled = true;
    q->data[1].rd = rd_b;
    q->data[1].handle.seb = STORAGE_ENGINE_BACKEND_DBENGINE;
    q->data[1].handle.handle = (STORAGE_QUERY_HANDLE *)&query_b;

    BUFFER *wb = buffer_create(0, NULL);
    bool finished_with_gap = replication_query_execute(wb, q, SIZE_MAX);

    static const char expected[] =
        "RBEGIN '' 100 160 1000\n"
        "RSET \"a\" 1 ''\n"
        "RSET \"b\" 11 ''\n"
        "RBEGIN '' 160 170 1000\n"
        "RSET \"a\" 2 ''\n"
        "RSET \"b\" 12 ''\n"
        "RBEGIN '' 170 180 1000\n"
        "RSET \"a\" 3 ''\n"
        "RBEGIN '' 180 190 1000\n"
        "RSET \"a\" 4 ''\n"
        "RSET \"b\" 14 ''\n"
        "RBEGIN '' 190 200 1000\n"
        "RSET \"a\" 5 ''\n"
        "RSET \"b\" 15 ''\n";

    int errors = 0;
    if(stream_replication_test_used_rrddim_backend || finished_with_gap ||
       q->query.after != 100 || q->query.before != 200 ||
       q->points_read != 10 || q->points_generated != 9 ||
       strcmp(buffer_tostring(wb), expected) != 0) {
        fprintf(stderr,
                "stream replication result: rrddim=%d gap=%d after=%" PRIdMAX " before=%" PRIdMAX
                " read=%zu generated=%zu\n"
                "expected:\n%sactual:\n%s",
                stream_replication_test_used_rrddim_backend, finished_with_gap,
                (intmax_t)q->query.after, (intmax_t)q->query.before,
                q->points_read, q->points_generated,
                expected, buffer_tostring(wb));
        errors++;
    }

    buffer_free(wb);
    freez(q);
    string_freez(rd_a->id);
    string_freez(rd_b->id);
    freez(rd_a);
    freez(rd_b);

    return errors ? 1 : 0;
}
