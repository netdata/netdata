// SPDX-License-Identifier: GPL-3.0-or-later

#include "jsonwrap.h"
#include "jsonwrap-internal.h"


ALWAYS_INLINE
size_t rrdr_dimension_names(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    const size_t dimensions = r->d;
    size_t c, i;

    buffer_json_member_add_array(wb, key);
    for(c = 0, i = 0; c < dimensions ; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        buffer_json_add_array_item_string(wb, string2str(r->dn[c]));
        i++;
    }
    buffer_json_array_close(wb);

    return i;
}

ALWAYS_INLINE
size_t rrdr_dimension_ids(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    const size_t dimensions = r->d;
    size_t c, i;

    buffer_json_member_add_array(wb, key);
    for(c = 0, i = 0; c < dimensions ; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        buffer_json_add_array_item_string(wb, string2str(r->di[c]));
        i++;
    }
    buffer_json_array_close(wb);

    return i;
}

ALWAYS_INLINE
void query_target_functions(BUFFER *wb, const char *key, RRDR *r) {
    QUERY_TARGET *qt = r->internal.qt;
    const long query_used = qt->query.used;

    DICTIONARY *funcs = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE);
    RRDINSTANCE_ACQUIRED *ria = NULL;
    for (long c = 0; c < query_used ; c++) {
        QUERY_METRIC *qm = query_metric(qt, c);
        QUERY_INSTANCE *qi = query_instance(qt, qm->link.query_instance_id);
        if(qi->ria == ria)
            continue;

        ria = qi->ria;
        chart_functions_to_dict(rrdinstance_acquired_functions(ria), funcs, NULL, 0);
    }

    buffer_json_member_add_array(wb, key);
    void *t; (void)t;
    dfe_start_read(funcs, t)
        buffer_json_add_array_item_string(wb, t_dfe.name);
    dfe_done(t);
    dictionary_destroy(funcs);
    buffer_json_array_close(wb);
}

ALWAYS_INLINE
void query_target_total_counts(BUFFER *wb, const char *key, struct summary_total_counts *totals) {
    if(!totals->selected && !totals->queried && !totals->failed && !totals->excluded)
        return;

    buffer_json_member_add_object(wb, key);

    if(totals->selected)
        buffer_json_member_add_uint64(wb, JSKEY(selected), totals->selected);

    if(totals->excluded)
        buffer_json_member_add_uint64(wb, JSKEY(excluded), totals->excluded);

    if(totals->queried)
        buffer_json_member_add_uint64(wb, JSKEY(queried), totals->queried);

    if(totals->failed)
        buffer_json_member_add_uint64(wb, JSKEY(failed), totals->failed);

    buffer_json_object_close(wb);
}

ALWAYS_INLINE
void query_target_metric_counts(BUFFER *wb, QUERY_METRICS_COUNTS *metrics) {
    if(!metrics->selected && !metrics->queried && !metrics->failed && !metrics->excluded)
        return;

    buffer_json_member_add_object(wb, JSKEY(dimensions));

    if(metrics->selected)
        buffer_json_member_add_uint64(wb, JSKEY(selected), metrics->selected);

    if(metrics->excluded)
        buffer_json_member_add_uint64(wb, JSKEY(excluded), metrics->excluded);

    if(metrics->queried)
        buffer_json_member_add_uint64(wb, JSKEY(queried), metrics->queried);

    if(metrics->failed)
        buffer_json_member_add_uint64(wb, JSKEY(failed), metrics->failed);

    buffer_json_object_close(wb);
}

ALWAYS_INLINE
void query_target_instance_counts(BUFFER *wb, QUERY_INSTANCES_COUNTS *instances) {
    if(!instances->selected && !instances->queried && !instances->failed && !instances->excluded)
        return;

    buffer_json_member_add_object(wb, JSKEY(instances));

    if(instances->selected)
        buffer_json_member_add_uint64(wb, JSKEY(selected), instances->selected);

    if(instances->excluded)
        buffer_json_member_add_uint64(wb, JSKEY(excluded), instances->excluded);

    if(instances->queried)
        buffer_json_member_add_uint64(wb, JSKEY(queried), instances->queried);

    if(instances->failed)
        buffer_json_member_add_uint64(wb, JSKEY(failed), instances->failed);

    buffer_json_object_close(wb);
}

ALWAYS_INLINE
void query_target_alerts_counts(BUFFER *wb, QUERY_ALERTS_COUNTS *alerts, const char *name, bool array) {
    if(!alerts->clear && !alerts->other && !alerts->critical && !alerts->warning)
        return;

    if(array)
        buffer_json_add_array_item_object(wb);
    else
        buffer_json_member_add_object(wb, JSKEY(alerts));

    if(name)
        buffer_json_member_add_string(wb, JSKEY(name), name);

    if(alerts->clear)
        buffer_json_member_add_uint64(wb, JSKEY(clear), alerts->clear);

    if(alerts->warning)
        buffer_json_member_add_uint64(wb, JSKEY(warning), alerts->warning);

    if(alerts->critical)
        buffer_json_member_add_uint64(wb, JSKEY(critical), alerts->critical);

    if(alerts->other)
        buffer_json_member_add_uint64(wb, JSKEY(other), alerts->other);

    buffer_json_object_close(wb);
}

ALWAYS_INLINE
void query_target_points_statistics(BUFFER *wb, QUERY_TARGET *qt, STORAGE_POINT *sp) {
    if(!sp->count)
        return;

    buffer_json_member_add_object(wb, JSKEY(statistics));

    buffer_json_member_add_double(wb, "min", sp->min);
    buffer_json_member_add_double(wb, "max", sp->max);

    if(query_target_aggregatable(qt)) {
        buffer_json_member_add_uint64(wb, JSKEY(count), sp->count);

        buffer_json_member_add_double(wb, "sum", sp->sum);
        buffer_json_member_add_double(wb, JSKEY(volume), sp->sum * (NETDATA_DOUBLE) query_view_update_every(qt));

        buffer_json_member_add_uint64(wb, JSKEY(anomaly_count), sp->anomaly_count);
    }
    else {
        NETDATA_DOUBLE avg = (sp->count) ? sp->sum / (NETDATA_DOUBLE)sp->count : 0.0;
        buffer_json_member_add_double(wb, "avg", avg);

        NETDATA_DOUBLE arp = storage_point_anomaly_rate(*sp);
        buffer_json_member_add_double(wb, JSKEY(anomaly_rate), arp);

        NETDATA_DOUBLE con = (qt->query_points.sum > 0.0) ? sp->sum * 100.0 / qt->query_points.sum : 0.0;
        buffer_json_member_add_double(wb, JSKEY(contribution), con);
    }
    buffer_json_object_close(wb);
}

ALWAYS_INLINE
void aggregate_metrics_counts(QUERY_METRICS_COUNTS *dst, const QUERY_METRICS_COUNTS *src) {
    dst->selected += src->selected;
    dst->excluded += src->excluded;
    dst->queried += src->queried;
    dst->failed += src->failed;
}

ALWAYS_INLINE
void aggregate_instances_counts(QUERY_INSTANCES_COUNTS *dst, const QUERY_INSTANCES_COUNTS *src) {
    dst->selected += src->selected;
    dst->excluded += src->excluded;
    dst->queried += src->queried;
    dst->failed += src->failed;
}

ALWAYS_INLINE
void aggregate_alerts_counts(QUERY_ALERTS_COUNTS *dst, const QUERY_ALERTS_COUNTS *src) {
    dst->clear += src->clear;
    dst->warning += src->warning;
    dst->critical += src->critical;
    dst->other += src->other;
}

ALWAYS_INLINE
void aggregate_into_summary_totals(struct summary_total_counts *totals, QUERY_METRICS_COUNTS *metrics) {
    if(unlikely(!totals || !metrics))
        return;

    if(metrics->selected) {
        totals->selected++;

        if(metrics->queried)
            totals->queried++;

        else if(metrics->failed)
            totals->failed++;
    }
    else
        totals->excluded++;
}
