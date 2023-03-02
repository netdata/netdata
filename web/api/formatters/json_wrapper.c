// SPDX-License-Identifier: GPL-3.0-or-later

#include "json_wrapper.h"

static void jsonwrap_query_metric_plan(BUFFER *wb, QUERY_METRIC *qm) {
    buffer_json_member_add_array(wb, "plans");
    for (size_t p = 0; p < qm->plan.used; p++) {
        QUERY_PLAN_ENTRY *qp = &qm->plan.array[p];

        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_uint64(wb, "tr", qp->tier);
        buffer_json_member_add_time_t(wb, "af", qp->after);
        buffer_json_member_add_time_t(wb, "bf", qp->before);
        buffer_json_object_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "tiers");
    for (size_t tier = 0; tier < storage_tiers; tier++) {
        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_uint64(wb, "tr", tier);
        buffer_json_member_add_time_t(wb, "fe", qm->tiers[tier].db_first_time_s);
        buffer_json_member_add_time_t(wb, "le", qm->tiers[tier].db_last_time_s);
        buffer_json_member_add_int64(wb, "wg", qm->tiers[tier].weight);
        buffer_json_object_close(wb);
    }
    buffer_json_array_close(wb);
}

void jsonwrap_query_plan(RRDR *r, BUFFER *wb) {
    QUERY_TARGET *qt = r->internal.qt;

    buffer_json_member_add_object(wb, "query_plan");
    for(size_t m = 0; m < qt->query.used; m++) {
        QUERY_METRIC *qm = query_metric(qt, m);
        buffer_json_member_add_object(wb, query_metric_id(qt, qm));
        jsonwrap_query_metric_plan(wb, qm);
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static inline size_t rrdr_dimension_names(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
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

static inline size_t rrdr_dimension_ids(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
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

static inline long jsonwrap_v1_chart_ids(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    QUERY_TARGET *qt = r->internal.qt;
    const long query_used = qt->query.used;
    long c, i;

    buffer_json_member_add_array(wb, key);
    for (c = 0, i = 0; c < query_used; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        QUERY_METRIC *qm = query_metric(qt, c);
        QUERY_INSTANCE *qi = query_instance(qt, qm->link.query_instance_id);
        buffer_json_add_array_item_string(wb, rrdinstance_acquired_id(qi->ria));
        i++;
    }
    buffer_json_array_close(wb);

    return i;
}

struct summary_total_counts {
    size_t selected;
    size_t excluded;
    size_t queried;
    size_t failed;
};

static inline void aggregate_into_summary_totals(struct summary_total_counts *totals, struct query_metrics_counts *metrics) {
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

static inline void query_target_total_counts(BUFFER *wb, const char *key, struct summary_total_counts *totals) {
    if(!totals->selected && !totals->queried && !totals->failed && !totals->excluded)
        return;

    buffer_json_member_add_object(wb, key);

    if(totals->selected)
        buffer_json_member_add_uint64(wb, "sl", totals->selected);

    if(totals->excluded)
        buffer_json_member_add_uint64(wb, "ex", totals->excluded);

    if(totals->queried)
        buffer_json_member_add_uint64(wb, "qr", totals->queried);

    if(totals->failed)
        buffer_json_member_add_uint64(wb, "fl", totals->failed);

    buffer_json_object_close(wb);
}

static inline void query_target_metric_counts(BUFFER *wb, struct query_metrics_counts *metrics) {
    if(!metrics->selected && !metrics->queried && !metrics->failed && !metrics->excluded)
        return;

    buffer_json_member_add_object(wb, "ds");

    if(metrics->selected)
        buffer_json_member_add_uint64(wb, "sl", metrics->selected);

    if(metrics->excluded)
        buffer_json_member_add_uint64(wb, "ex", metrics->excluded);

    if(metrics->queried)
        buffer_json_member_add_uint64(wb, "qr", metrics->queried);

    if(metrics->failed)
        buffer_json_member_add_uint64(wb, "fl", metrics->failed);

    buffer_json_object_close(wb);
}

static inline void query_target_instance_counts(BUFFER *wb, struct query_instances_counts *instances) {
    if(!instances->selected && !instances->queried && !instances->failed && !instances->excluded)
        return;

    buffer_json_member_add_object(wb, "is");

    if(instances->selected)
        buffer_json_member_add_uint64(wb, "sl", instances->selected);

    if(instances->excluded)
        buffer_json_member_add_uint64(wb, "ex", instances->excluded);

    if(instances->queried)
        buffer_json_member_add_uint64(wb, "qr", instances->queried);

    if(instances->failed)
        buffer_json_member_add_uint64(wb, "fl", instances->failed);

    buffer_json_object_close(wb);
}

static inline void query_target_alerts_counts(BUFFER *wb, struct query_alerts_counts *alerts, const char *name, bool array) {
    if(!alerts->clear && !alerts->other && !alerts->critical && !alerts->warning)
        return;

    if(array)
        buffer_json_add_array_item_object(wb);
    else
        buffer_json_member_add_object(wb, "al");

    if(name)
        buffer_json_member_add_string(wb, "nm", name);

    if(alerts->clear)
        buffer_json_member_add_uint64(wb, "cl", alerts->clear);

    if(alerts->warning)
        buffer_json_member_add_uint64(wb, "wr", alerts->warning);

    if(alerts->critical)
        buffer_json_member_add_uint64(wb, "cr", alerts->critical);

    if(alerts->other)
        buffer_json_member_add_uint64(wb, "ot", alerts->other);

    buffer_json_object_close(wb);
}

static inline void query_target_data_statistics(BUFFER *wb, QUERY_TARGET *qt, struct query_data_statistics *d) {
    if(!d->group_points)
        return;

    buffer_json_member_add_object(wb, "sts");
    if(qt->request.group_by_aggregate_function == RRDR_GROUP_BY_FUNCTION_SUM_COUNT) {
        buffer_json_member_add_uint64(wb, "cnt", d->group_points);

        if(d->sum != 0.0)
            buffer_json_member_add_double(wb, "sum", d->sum);

        if(d->volume != 0.0)
            buffer_json_member_add_double(wb, "vol", d->volume);

        if(d->anomaly_sum != 0.0)
            buffer_json_member_add_double(wb, "ars", d->anomaly_sum);
    }
    else {
//        buffer_json_member_add_double(wb, "min", d->min);
//        buffer_json_member_add_double(wb, "max", d->max);

        NETDATA_DOUBLE avg = (d->group_points) ? d->sum / (NETDATA_DOUBLE)d->group_points : 0.0;
        if(avg != 0.0)
            buffer_json_member_add_double(wb, "avg", avg);

        NETDATA_DOUBLE arp = (d->group_points) ? d->anomaly_sum / (NETDATA_DOUBLE)d->group_points : 0.0;
        if(arp != 0.0)
            buffer_json_member_add_double(wb, "arp", arp);

        NETDATA_DOUBLE con = (qt->query_stats.volume > 0) ? d->volume * 100.0 / qt->query_stats.volume : 0.0;
        if(con != 0.0)
            buffer_json_member_add_double(wb, "con", con);
    }
    buffer_json_object_close(wb);
}

static void query_target_summary_nodes_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key, struct summary_total_counts *totals) {
    buffer_json_member_add_array(wb, key);
    for (size_t c = 0; c < qt->nodes.used; c++) {
        QUERY_NODE *qn = query_node(qt, c);
        RRDHOST *host = qn->rrdhost;
        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_uint64(wb, "ni", qn->slot);
        buffer_json_member_add_string(wb, "mg", host->machine_guid);
        if(qn->node_id[0])
            buffer_json_member_add_string(wb, "nd", qn->node_id);
        buffer_json_member_add_string(wb, "nm", rrdhost_hostname(host));
        query_target_instance_counts(wb, &qn->instances);
        query_target_metric_counts(wb, &qn->metrics);
        query_target_alerts_counts(wb, &qn->alerts, NULL, false);
        query_target_data_statistics(wb, qt, &qn->query_stats);
        buffer_json_object_close(wb);

        aggregate_into_summary_totals(totals, &qn->metrics);
    }
    buffer_json_array_close(wb);
}

static size_t query_target_summary_contexts_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key, struct summary_total_counts *totals) {
    buffer_json_member_add_array(wb, key);
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);

    struct {
        struct query_data_statistics query_stats;
        struct query_instances_counts instances;
        struct query_metrics_counts metrics;
        struct query_alerts_counts alerts;
    } x = { 0 }, *z;

    for (long c = 0; c < (long) qt->contexts.used; c++) {
        QUERY_CONTEXT *qc = query_context(qt, c);

        z = dictionary_set(dict, rrdcontext_acquired_id(qc->rca), &x, sizeof(x));

        z->instances.selected += qc->instances.selected;
        z->instances.excluded += qc->instances.selected;
        z->instances.queried += qc->instances.queried;
        z->instances.failed += qc->instances.failed;

        z->metrics.selected += qc->metrics.selected;
        z->metrics.excluded += qc->metrics.excluded;
        z->metrics.queried += qc->metrics.queried;
        z->metrics.failed += qc->metrics.failed;

        z->alerts.clear += qc->alerts.clear;
        z->alerts.warning += qc->alerts.warning;
        z->alerts.critical += qc->alerts.critical;

        query_target_merge_data_statistics(&z->query_stats, &qc->query_stats);
    }

    size_t unique_contexts = dictionary_entries(dict);
    dfe_start_read(dict, z) {
        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_string(wb, "id", z_dfe.name);
        query_target_instance_counts(wb, &z->instances);
        query_target_metric_counts(wb, &z->metrics);
        query_target_alerts_counts(wb, &z->alerts, NULL, false);
        query_target_data_statistics(wb, qt, &z->query_stats);
        buffer_json_object_close(wb);

        aggregate_into_summary_totals(totals, &z->metrics);
    }
    dfe_done(z);
    buffer_json_array_close(wb);
    dictionary_destroy(dict);

    return unique_contexts;
}

static void query_target_summary_instances_v1(BUFFER *wb, QUERY_TARGET *qt, const char *key) {
    char name[RRD_ID_LENGTH_MAX * 2 + 2];

    buffer_json_member_add_array(wb, key);
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
    for (long c = 0; c < (long) qt->instances.used; c++) {
        QUERY_INSTANCE *qi = query_instance(qt, c);

        snprintfz(name, RRD_ID_LENGTH_MAX * 2 + 1, "%s:%s",
                  rrdinstance_acquired_id(qi->ria),
                  rrdinstance_acquired_name(qi->ria));

        bool existing = 0;
        bool *set = dictionary_set(dict, name, &existing, sizeof(bool));
        if (!*set) {
            *set = true;
            buffer_json_add_array_item_array(wb);
            buffer_json_add_array_item_string(wb, rrdinstance_acquired_id(qi->ria));
            buffer_json_add_array_item_string(wb, rrdinstance_acquired_name(qi->ria));
            buffer_json_array_close(wb);
        }
    }
    dictionary_destroy(dict);
    buffer_json_array_close(wb);
}

static void query_target_summary_instances_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key, struct summary_total_counts *totals) {
    buffer_json_member_add_array(wb, key);
    for (long c = 0; c < (long) qt->instances.used; c++) {
        QUERY_INSTANCE *qi = query_instance(qt, c);
//        QUERY_HOST *qh = query_host(qt, qi->query_host_id);

        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_string(wb, "id", rrdinstance_acquired_id(qi->ria));

        if(!rrdinstance_acquired_id_and_name_are_same(qi->ria))
            buffer_json_member_add_string(wb, "nm", rrdinstance_acquired_name(qi->ria));

        buffer_json_member_add_uint64(wb, "ni", qi->query_host_id);
//        buffer_json_member_add_string(wb, "id", string2str(qi->id_fqdn));
//        buffer_json_member_add_string(wb, "nm", string2str(qi->name_fqdn));
//        buffer_json_member_add_string(wb, "lc", rrdinstance_acquired_name(qi->ria));
//        buffer_json_member_add_string(wb, "mg", qh->host->machine_guid);
//        if(qh->node_id[0])
//            buffer_json_member_add_string(wb, "nd", qh->node_id);
        query_target_metric_counts(wb, &qi->metrics);
        query_target_alerts_counts(wb, &qi->alerts, NULL, false);
        query_target_data_statistics(wb, qt, &qi->query_stats);
        buffer_json_object_close(wb);

        aggregate_into_summary_totals(totals, &qi->metrics);
    }
    buffer_json_array_close(wb);
}

static void query_target_summary_dimensions_v12(BUFFER *wb, QUERY_TARGET *qt, const char *key, bool v2, struct summary_total_counts *totals) {
    char name[RRD_ID_LENGTH_MAX * 2 + 2];

    buffer_json_member_add_array(wb, key);
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
    struct {
        const char *id;
        const char *name;
        struct query_data_statistics query_stats;
        struct query_metrics_counts metrics;
    } *z;
    size_t q = 0;
    for (long c = 0; c < (long) qt->dimensions.used; c++) {
        QUERY_DIMENSION * qd = query_dimension(qt, c);
        RRDMETRIC_ACQUIRED *rma = qd->rma;

        QUERY_METRIC *qm = NULL;
        for( ; q < qt->query.used ;q++) {
            QUERY_METRIC *tqm = query_metric(qt, q);
            QUERY_DIMENSION *tqd = query_dimension(qt, tqm->link.query_dimension_id);
            if(tqd->rma != rma) break;
            qm = tqm;
        }

        snprintfz(name, RRD_ID_LENGTH_MAX * 2 + 1, "%s:%s",
                  rrdmetric_acquired_id(rma),
                  rrdmetric_acquired_name(rma));

        z = dictionary_set(dict, name, NULL, sizeof(*z));
        if(!z->id)
            z->id = rrdmetric_acquired_id(rma);
        if(!z->name)
            z->name = rrdmetric_acquired_name(rma);

        if(qm) {
            z->metrics.selected += (qm->status & RRDR_DIMENSION_SELECTED) ? 1 : 0;
            z->metrics.failed += (qm->status & RRDR_DIMENSION_FAILED) ? 1 : 0;

            if(qm->status & RRDR_DIMENSION_QUERIED) {
                z->metrics.queried++;
                query_target_merge_data_statistics(&z->query_stats, &qm->query_stats);
            }
        }
        else
            z->metrics.excluded++;
    }
    dfe_start_read(dict, z) {
                if(v2) {
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "id", z->id);
                    buffer_json_member_add_string(wb, "nm", z->name);
                    query_target_metric_counts(wb, &z->metrics);
                    query_target_data_statistics(wb, qt, &z->query_stats);
                    buffer_json_object_close(wb);

                    aggregate_into_summary_totals(totals, &z->metrics);
                }
                else {
                    buffer_json_add_array_item_array(wb);
                    buffer_json_add_array_item_string(wb, z->id);
                    buffer_json_add_array_item_string(wb, z->name);
                    buffer_json_array_close(wb);
                }
            }
    dfe_done(z);
    dictionary_destroy(dict);
    buffer_json_array_close(wb);
}

struct rrdlabels_formatting_v2 {
    DICTIONARY *keys;
    QUERY_INSTANCE *qi;
    bool v2;
};

struct rrdlabels_keys_dict_entry {
    const char *name;
    DICTIONARY *values;
    struct query_data_statistics query_stats;
    struct query_metrics_counts metrics;
};

struct rrdlabels_key_value_dict_entry {
    const char *key;
    const char *value;
    struct query_data_statistics query_stats;
    struct query_metrics_counts metrics;
};

static int rrdlabels_formatting_v2(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    struct rrdlabels_formatting_v2 *t = data;

    struct rrdlabels_keys_dict_entry k = {
            .name = name,
            .values = NULL,
            .metrics = (struct query_metrics_counts){ 0 },
    }, *d = dictionary_set(t->keys, name, &k, sizeof(k));

    if(!d->values)
        d->values = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);

    char n[RRD_ID_LENGTH_MAX * 2 + 2];
    snprintfz(n, RRD_ID_LENGTH_MAX * 2, "%s:%s", name, value);

    struct rrdlabels_key_value_dict_entry x = {
            .key = name,
            .value = value,
            .query_stats = (struct query_data_statistics) { 0 },
            .metrics = (struct query_metrics_counts){ 0 },
    }, *z = dictionary_set(d->values, n, &x, sizeof(x));

    if(t->v2) {
        QUERY_INSTANCE *qi = t->qi;

        z->metrics.selected += qi->metrics.selected;
        z->metrics.excluded += qi->metrics.excluded;
        z->metrics.queried += qi->metrics.queried;
        z->metrics.failed += qi->metrics.failed;

        d->metrics.selected += qi->metrics.selected;
        d->metrics.excluded += qi->metrics.excluded;
        d->metrics.queried += qi->metrics.queried;
        d->metrics.failed += qi->metrics.failed;

        query_target_merge_data_statistics(&z->query_stats, &qi->query_stats);
        query_target_merge_data_statistics(&d->query_stats, &qi->query_stats);
    }

    return 1;
}

static void query_target_summary_labels_v12(BUFFER *wb, QUERY_TARGET *qt, const char *key, bool v2, struct summary_total_counts *key_totals, struct summary_total_counts *value_totals) {
    buffer_json_member_add_array(wb, key);
    struct rrdlabels_formatting_v2 t = {
            .keys = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE),
            .v2 = v2,
    };
    for (long c = 0; c < (long) qt->instances.used; c++) {
        QUERY_INSTANCE *qi = query_instance(qt, c);
        RRDINSTANCE_ACQUIRED *ria = qi->ria;
        t.qi = qi;
        rrdlabels_walkthrough_read(rrdinstance_acquired_labels(ria), rrdlabels_formatting_v2, &t);
    }
    struct rrdlabels_keys_dict_entry *d;
    dfe_start_read(t.keys, d) {
                if(v2) {
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "id", d_dfe.name);
                    query_target_metric_counts(wb, &d->metrics);
                    query_target_data_statistics(wb, qt, &d->query_stats);
                    aggregate_into_summary_totals(key_totals, &d->metrics);
                    buffer_json_member_add_array(wb, "vl");
                }
                struct rrdlabels_key_value_dict_entry *z;
                dfe_start_read(d->values, z){
                            if (v2) {
                                buffer_json_add_array_item_object(wb);
                                buffer_json_member_add_string(wb, "id", z->value);
                                query_target_metric_counts(wb, &z->metrics);
                                query_target_data_statistics(wb, qt, &z->query_stats);
                                buffer_json_object_close(wb);
                                aggregate_into_summary_totals(value_totals, &z->metrics);
                            } else {
                                buffer_json_add_array_item_array(wb);
                                buffer_json_add_array_item_string(wb, z->key);
                                buffer_json_add_array_item_string(wb, z->value);
                                buffer_json_array_close(wb);
                            }
                        }
                dfe_done(z);
                dictionary_destroy(d->values);
                if(v2) {
                    buffer_json_array_close(wb);
                    buffer_json_object_close(wb);
                }
            }
    dfe_done(d);
    dictionary_destroy(t.keys);
    buffer_json_array_close(wb);
}

static void query_target_summary_alerts_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key) {
    buffer_json_member_add_array(wb, key);
    struct query_alerts_counts x = { 0 }, *z;

    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
    for (long c = 0; c < (long) qt->instances.used; c++) {
        QUERY_INSTANCE *qi = query_instance(qt, c);
        RRDSET *st = rrdinstance_acquired_rrdset(qi->ria);
        if (st) {
            netdata_rwlock_rdlock(&st->alerts.rwlock);
            if (st->alerts.base) {
                for (RRDCALC *rc = st->alerts.base; rc; rc = rc->next) {
                    z = dictionary_set(dict, string2str(rc->name), &x, sizeof(x));

                    switch(rc->status) {
                        case RRDCALC_STATUS_CLEAR:
                            z->clear++;
                            break;

                        case RRDCALC_STATUS_WARNING:
                            z->warning++;
                            break;

                        case RRDCALC_STATUS_CRITICAL:
                            z->critical++;
                            break;

                        default:
                        case RRDCALC_STATUS_UNINITIALIZED:
                        case RRDCALC_STATUS_UNDEFINED:
                        case RRDCALC_STATUS_REMOVED:
                            z->other++;
                            break;
                    }
                }
            }
            netdata_rwlock_unlock(&st->alerts.rwlock);
        }
    }
    dfe_start_read(dict, z)
            query_target_alerts_counts(wb, z, z_dfe.name, true);
    dfe_done(z);
    dictionary_destroy(dict);
    buffer_json_array_close(wb); // alerts
}

static inline void query_target_functions(BUFFER *wb, const char *key, RRDR *r) {
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
        chart_functions_to_dict(rrdinstance_acquired_functions(ria), funcs);
    }

    buffer_json_member_add_array(wb, key);
    void *t; (void)t;
    dfe_start_read(funcs, t)
        buffer_json_add_array_item_string(wb, t_dfe.name);
    dfe_done(t);
    dictionary_destroy(funcs);
    buffer_json_array_close(wb);
}

static inline long query_target_chart_labels_filter_v1(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    QUERY_TARGET *qt = r->internal.qt;
    const long query_used = qt->query.used;
    long c, i = 0;

    buffer_json_member_add_object(wb, key);

    SIMPLE_PATTERN *pattern = qt->instances.chart_label_key_pattern;
    char *label_key = NULL;
    while (pattern && (label_key = simple_pattern_iterate(&pattern))) {
        buffer_json_member_add_array(wb, label_key);

        for (c = 0, i = 0; c < query_used; c++) {
            if(!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            QUERY_METRIC *qm = query_metric(qt, c);
            QUERY_INSTANCE *qi = query_instance(qt, qm->link.query_instance_id);
            rrdlabels_value_to_buffer_array_item_or_null(rrdinstance_acquired_labels(qi->ria), wb, label_key);
            i++;
        }
        buffer_json_array_close(wb);
    }

    buffer_json_object_close(wb);

    return i;
}

static inline long query_target_metrics_latest_values(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    QUERY_TARGET *qt = r->internal.qt;
    const long query_used = qt->query.used;
    long c, i;

    buffer_json_member_add_array(wb, key);

    for(c = 0, i = 0; c < query_used ;c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        QUERY_METRIC *qm = query_metric(qt, c);
        QUERY_DIMENSION *qd = query_dimension(qt, qm->link.query_dimension_id);
        buffer_json_add_array_item_double(wb, rrdmetric_acquired_last_stored_value(qd->rma));
        i++;
    }

    buffer_json_array_close(wb);

    return i;
}

static inline size_t rrdr_latest_values(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    size_t c, i;

    buffer_json_member_add_array(wb, key);

    NETDATA_DOUBLE total = 1;

    if(unlikely(options & RRDR_OPTION_PERCENTAGE)) {
        total = 0;
        for(c = 0; c < r->d ; c++) {
            if(unlikely(!(r->od[c] & RRDR_DIMENSION_QUERIED))) continue;

            NETDATA_DOUBLE *cn = &r->v[ (rrdr_rows(r) - 1) * r->d ];
            NETDATA_DOUBLE n = cn[c];

            if(likely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
                n = -n;

            total += n;
        }
        // prevent a division by zero
        if(total == 0) total = 1;
    }

    for(c = 0, i = 0; c < r->d ; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        i++;

        NETDATA_DOUBLE *cn = &r->v[ (rrdr_rows(r) - 1) * r->d ];
        RRDR_VALUE_FLAGS *co = &r->o[ (rrdr_rows(r) - 1) * r->d ];
        NETDATA_DOUBLE n = cn[c];

        if(co[c] & RRDR_VALUE_EMPTY) {
            if(options & RRDR_OPTION_NULL2ZERO)
                buffer_json_add_array_item_double(wb, 0.0);
            else
                buffer_json_add_array_item_double(wb, NAN);
        }
        else {
            if(unlikely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
                n = -n;

            if(unlikely(options & RRDR_OPTION_PERCENTAGE))
                n = n * 100 / total;

            buffer_json_add_array_item_double(wb, n);
        }
    }

    buffer_json_array_close(wb);

    return i;
}

void rrdr_json_wrapper_begin(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format, RRDR_OPTIONS options,
                             RRDR_TIME_GROUPING group_method)
{
    QUERY_TARGET *qt = r->internal.qt;

    long rows = rrdr_rows(r);

    char kq[2] = "",                    // key quote
         sq[2] = "";                    // string quote

    if( options & RRDR_OPTION_GOOGLE_JSON ) {
        kq[0] = '\0';
        sq[0] = '\'';
    }
    else {
        kq[0] = '"';
        sq[0] = '"';
    }

    buffer_json_initialize(wb, kq, sq, 0, true, options & RRDR_OPTION_MINIFY);

    buffer_json_member_add_uint64(wb, "api", 1);
    buffer_json_member_add_string(wb, "id", qt->id);
    buffer_json_member_add_string(wb, "name", qt->id);
    buffer_json_member_add_time_t(wb, "view_update_every", r->view.update_every);
    buffer_json_member_add_time_t(wb, "update_every", qt->db.minimum_latest_update_every_s);
    buffer_json_member_add_time_t(wb, "first_entry", qt->db.first_time_s);
    buffer_json_member_add_time_t(wb, "last_entry", qt->db.last_time_s);
    buffer_json_member_add_time_t(wb, "after", r->view.after);
    buffer_json_member_add_time_t(wb, "before", r->view.before);
    buffer_json_member_add_string(wb, "group", time_grouping_tostring(group_method));
    web_client_api_request_v1_data_options_to_buffer_json_array(wb, "options", r->view.options);

    if(!rrdr_dimension_names(wb, "dimension_names", r, options))
        rows = 0;

    if(!rrdr_dimension_ids(wb, "dimension_ids", r, options))
        rows = 0;

    if (r->view.options & RRDR_OPTION_ALL_DIMENSIONS) {
        query_target_summary_instances_v1(wb, qt, "full_chart_list");
        query_target_summary_dimensions_v12(wb, qt, "full_dimension_list", false, NULL);
        query_target_summary_labels_v12(wb, qt, "full_chart_labels", false, NULL, NULL);
    }

    query_target_functions(wb, "functions", r);

    if (!qt->request.st && !jsonwrap_v1_chart_ids(wb, "chart_ids", r, options))
        rows = 0;

    if (qt->instances.chart_label_key_pattern && !query_target_chart_labels_filter_v1(wb, "chart_labels", r, options))
        rows = 0;

    if(!query_target_metrics_latest_values(wb, "latest_values", r, options))
        rows = 0;

    size_t dimensions = rrdr_latest_values(wb, "view_latest_values", r, options);
    if(!dimensions)
        rows = 0;

    buffer_json_member_add_uint64(wb, "dimensions", dimensions);
    buffer_json_member_add_uint64(wb, "points", rows);
    buffer_json_member_add_string(wb, "format", rrdr_format_to_string(format));

    buffer_json_member_add_array(wb, "db_points_per_tier");
    for(size_t tier = 0; tier < storage_tiers ; tier++)
        buffer_json_add_array_item_uint64(wb, qt->db.tiers[tier].points);
    buffer_json_array_close(wb);

    if(options & RRDR_OPTION_SHOW_PLAN)
        jsonwrap_query_plan(r, wb);
}

static void rrdset_rrdcalc_entries_v2(BUFFER *wb, RRDINSTANCE_ACQUIRED *ria) {
    RRDSET *st = rrdinstance_acquired_rrdset(ria);
    if(st) {
        netdata_rwlock_rdlock(&st->alerts.rwlock);
        if(st->alerts.base) {
            buffer_json_member_add_object(wb, "alerts");
            for(RRDCALC *rc = st->alerts.base; rc ;rc = rc->next) {
                if(rc->status < RRDCALC_STATUS_CLEAR)
                    continue;

                buffer_json_member_add_object(wb, string2str(rc->name));
                buffer_json_member_add_string(wb, "st", rrdcalc_status2string(rc->status));
                buffer_json_member_add_double(wb, "vl", rc->value);
                buffer_json_member_add_string(wb, "un", string2str(rc->units));
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);
        }
        netdata_rwlock_unlock(&st->alerts.rwlock);
    }
}

static void query_target_combined_units_v2(BUFFER *wb, QUERY_TARGET *qt, size_t contexts) {
    if(query_target_has_percentage_units(qt)) {
        buffer_json_member_add_string(wb, "units", "%");
    }
    else if(contexts == 1) {
        buffer_json_member_add_string(wb, "units", rrdcontext_acquired_units(qt->contexts.array[0].rca));
    }
    else if(contexts > 1) {
        DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
        for(size_t c = 0; c < qt->contexts.used ;c++)
            dictionary_set(dict, rrdcontext_acquired_units(qt->contexts.array[c].rca), NULL, 0);

        if(dictionary_entries(dict) == 1)
            buffer_json_member_add_string(wb, "units", rrdcontext_acquired_units(qt->contexts.array[0].rca));
        else {
            buffer_json_member_add_array(wb, "units");
            const char *s;
            dfe_start_read(dict, s)
                    buffer_json_add_array_item_string(wb, s_dfe.name);
            dfe_done(s);
            buffer_json_array_close(wb);
        }
        dictionary_destroy(dict);
    }
}

static void query_target_combined_chart_type(BUFFER *wb, QUERY_TARGET *qt, size_t contexts) {
    if(contexts >= 1)
        buffer_json_member_add_string(wb, "chart_type", rrdset_type_name(rrdcontext_acquired_chart_type(qt->contexts.array[0].rca)));
}

static void rrdr_dimension_units_array_v2(BUFFER *wb, RRDR *r, RRDR_OPTIONS options) {
    if(!r->du)
        return;

    bool percentage = query_target_has_percentage_units(r->internal.qt);

    buffer_json_member_add_array(wb, "units");
    for(size_t c = 0; c < r->d ; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        if(percentage)
            buffer_json_add_array_item_string(wb, "%");
        else
            buffer_json_add_array_item_string(wb, string2str(r->du[c]));
    }
    buffer_json_array_close(wb);
}

static void rrdr_dimension_priority_array(BUFFER *wb, RRDR *r, RRDR_OPTIONS options) {
    if(!r->dp)
        return;

    buffer_json_member_add_array(wb, "priorities");
    for(size_t c = 0; c < r->d ; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        buffer_json_add_array_item_uint64(wb, r->dp[c]);
    }
    buffer_json_array_close(wb);
}

static void rrdr_dimension_grouped_array(BUFFER *wb, RRDR *r, RRDR_OPTIONS options) {
    if(!r->dgbc)
        return;

    buffer_json_member_add_array(wb, "grouped");
    for(size_t c = 0; c < r->d ;c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        buffer_json_add_array_item_uint64(wb, r->dgbc[c]);
    }
    buffer_json_array_close(wb);
}

static void query_target_title(BUFFER *wb, QUERY_TARGET *qt, size_t contexts) {
    if(contexts == 1) {
        buffer_json_member_add_string(wb, "title", rrdcontext_acquired_title(qt->contexts.array[0].rca));
    }
    else if(contexts > 1) {
        BUFFER *t = buffer_create(0, NULL);
        DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);

        buffer_strcat(t, "Chart for contexts: ");

        size_t added = 0;
        for(size_t c = 0; c < qt->contexts.used ;c++) {
            bool old = false;
            bool *set = dictionary_set(dict, rrdcontext_acquired_id(qt->contexts.array[c].rca), &old, sizeof(old));
            if(!*set) {
                *set = true;
                if(added)
                    buffer_fast_strcat(t, ", ", 2);

                buffer_strcat(t, rrdcontext_acquired_id(qt->contexts.array[c].rca));
                added++;
            }
        }
        buffer_json_member_add_string(wb, "title", buffer_tostring(t));
        dictionary_destroy(dict);
        buffer_free(t);
    }
}

static void query_target_detailed_objects_tree(BUFFER *wb, RRDR *r, RRDR_OPTIONS options) {
    QUERY_TARGET *qt = r->internal.qt;
    buffer_json_member_add_object(wb, "nodes");

    time_t now_s = now_realtime_sec();
    RRDHOST *last_host = NULL;
    RRDCONTEXT_ACQUIRED *last_rca = NULL;
    RRDINSTANCE_ACQUIRED *last_ria = NULL;

    size_t h = 0, c = 0, i = 0, m = 0, q = 0;
    for(; h < qt->nodes.used ; h++) {
        QUERY_NODE *qn = query_node(qt, h);
        RRDHOST *host = qn->rrdhost;

        for( ;c < qt->contexts.used ;c++) {
            QUERY_CONTEXT *qc = query_context(qt, c);
            RRDCONTEXT_ACQUIRED *rca = qc->rca;
            if(!rrdcontext_acquired_belongs_to_host(rca, host)) break;

            for( ;i < qt->instances.used ;i++) {
                QUERY_INSTANCE *qi = query_instance(qt, i);
                RRDINSTANCE_ACQUIRED *ria = qi->ria;
                if(!rrdinstance_acquired_belongs_to_context(ria, rca)) break;

                for( ; m < qt->dimensions.used ; m++) {
                    QUERY_DIMENSION *qd = query_dimension(qt, m);
                    RRDMETRIC_ACQUIRED *rma = qd->rma;
                    if(!rrdmetric_acquired_belongs_to_instance(rma, ria)) break;

                    QUERY_METRIC *qm = NULL;
                    bool queried = false;
                    for( ; q < qt->query.used ;q++) {
                        QUERY_METRIC *tqm = query_metric(qt, q);
                        QUERY_DIMENSION *tqd = query_dimension(qt, tqm->link.query_dimension_id);
                        if(tqd->rma != rma) break;

                        queried = tqm->status & RRDR_DIMENSION_QUERIED;
                        qm = tqm;
                    }

                    if(!queried & !(options & RRDR_OPTION_ALL_DIMENSIONS))
                        continue;

                    if(host != last_host) {
                        if(last_host) {
                            if(last_rca) {
                                if(last_ria) {
                                    buffer_json_object_close(wb); // dimensions
                                    buffer_json_object_close(wb); // instance
                                    last_ria = NULL;
                                }
                                buffer_json_object_close(wb); // instances
                                buffer_json_object_close(wb); // context
                                last_rca = NULL;
                            }
                            buffer_json_object_close(wb); // contexts
                            buffer_json_object_close(wb); // host
                            last_host = NULL;
                        }

                        buffer_json_member_add_object(wb, host->machine_guid);
                        if(qn->node_id[0])
                            buffer_json_member_add_string(wb, "nd", qn->node_id);
                        buffer_json_member_add_uint64(wb, "ni", qn->slot);
                        buffer_json_member_add_string(wb, "nm", rrdhost_hostname(host));
                        buffer_json_member_add_object(wb, "contexts");

                        last_host = host;
                    }

                    if(rca != last_rca) {
                        if(last_rca) {
                            if(last_ria) {
                                buffer_json_object_close(wb); // dimensions
                                buffer_json_object_close(wb); // instance
                                last_ria = NULL;
                            }
                            buffer_json_object_close(wb); // instances
                            buffer_json_object_close(wb); // context
                            last_rca = NULL;
                        }

                        buffer_json_member_add_object(wb, rrdcontext_acquired_id(rca));
                        buffer_json_member_add_object(wb, "instances");

                        last_rca = rca;
                    }

                    if(ria != last_ria) {
                        if(last_ria) {
                            buffer_json_object_close(wb); // dimensions
                            buffer_json_object_close(wb); // instance
                            last_ria = NULL;
                        }

                        buffer_json_member_add_object(wb, rrdinstance_acquired_id(ria));
                        buffer_json_member_add_string(wb, "nm", rrdinstance_acquired_name(ria));
                        buffer_json_member_add_time_t(wb, "ue", rrdinstance_acquired_update_every(ria));
                        DICTIONARY *labels = rrdinstance_acquired_labels(ria);
                        if(labels) {
                            buffer_json_member_add_object(wb, "labels");
                            rrdlabels_to_buffer_json_members(labels, wb);
                            buffer_json_object_close(wb);
                        }
                        rrdset_rrdcalc_entries_v2(wb, ria);
                        buffer_json_member_add_object(wb, "dimensions");

                        last_ria = ria;
                    }

                    buffer_json_member_add_object(wb, rrdmetric_acquired_id(rma));
                    {
                        buffer_json_member_add_string(wb, "nm", rrdmetric_acquired_name(rma));
                        buffer_json_member_add_uint64(wb, "qr", queried ? 1 : 0);
                        time_t first_entry_s = rrdmetric_acquired_first_entry(rma);
                        time_t last_entry_s = rrdmetric_acquired_last_entry(rma);
                        buffer_json_member_add_time_t(wb, "fe", first_entry_s);
                        buffer_json_member_add_time_t(wb, "le", last_entry_s ? last_entry_s : now_s);

                        if(qm) {
                            if(qm->status & RRDR_DIMENSION_GROUPED) {
                                // buffer_json_member_add_string(wb, "grouped_as_id", string2str(qm->grouped_as.id));
                                buffer_json_member_add_string(wb, "as", string2str(qm->grouped_as.name));
                            }

                            query_target_data_statistics(wb, qt, &qm->query_stats);

                            if(options & RRDR_OPTION_SHOW_PLAN)
                                jsonwrap_query_metric_plan(wb, qm);
                        }
                    }
                    buffer_json_object_close(wb); // metric
                }
            }
        }
    }

    if(last_host) {
        if(last_rca) {
            if(last_ria) {
                buffer_json_object_close(wb); // dimensions
                buffer_json_object_close(wb); // instance
                last_ria = NULL;
            }
            buffer_json_object_close(wb); // instances
            buffer_json_object_close(wb); // context
            last_rca = NULL;
        }
        buffer_json_object_close(wb); // contexts
        buffer_json_object_close(wb); // host
        last_host = NULL;
    }
    buffer_json_object_close(wb); // hosts
}

void rrdr_json_wrapper_begin2(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format, RRDR_OPTIONS options,
                             RRDR_TIME_GROUPING group_method)
{
    QUERY_TARGET *qt = r->internal.qt;

    long rows = rrdr_rows(r);

    char kq[2] = "\"",                    // key quote
         sq[2] = "\"";                    // string quote

    if(unlikely(options & RRDR_OPTION_GOOGLE_JSON)) {
        kq[0] = '\0';
        sq[0] = '\'';
    }

    buffer_json_initialize(wb, kq, sq, 0, true, options & RRDR_OPTION_MINIFY);

    buffer_json_member_add_uint64(wb, "api", 2);

    if(options & RRDR_OPTION_DEBUG) {
        buffer_json_member_add_string(wb, "id", qt->id);
        buffer_json_member_add_object(wb, "request");
        {
            buffer_json_member_add_string(wb, "format", rrdr_format_to_string(qt->request.format));
            web_client_api_request_v1_data_options_to_buffer_json_array(wb, "options", qt->request.options);

            buffer_json_member_add_object(wb, "scope");
            buffer_json_member_add_string(wb, "scope_nodes", qt->request.scope_nodes);
            buffer_json_member_add_string(wb, "scope_contexts", qt->request.scope_contexts);
            buffer_json_object_close(wb); // scope

            buffer_json_member_add_object(wb, "selectors");
            if (qt->request.host)
                buffer_json_member_add_string(wb, "nodes", rrdhost_hostname(qt->request.host));
            else
                buffer_json_member_add_string(wb, "nodes", qt->request.nodes);
            buffer_json_member_add_string(wb, "contexts", qt->request.contexts);
            buffer_json_member_add_string(wb, "instances", qt->request.instances);
            buffer_json_member_add_string(wb, "dimensions", qt->request.dimensions);
            buffer_json_member_add_string(wb, "labels", qt->request.labels);
            buffer_json_member_add_string(wb, "alerts", qt->request.alerts);
            buffer_json_object_close(wb); // selectors

            buffer_json_member_add_object(wb, "window");
            buffer_json_member_add_time_t(wb, "after", qt->request.after);
            buffer_json_member_add_time_t(wb, "before", qt->request.before);
            buffer_json_member_add_uint64(wb, "points", qt->request.points);
            if (qt->request.options & RRDR_OPTION_SELECTED_TIER)
                buffer_json_member_add_uint64(wb, "tier", qt->request.tier);
            else
                buffer_json_member_add_string(wb, "tier", NULL);
            buffer_json_object_close(wb); // window

            buffer_json_member_add_object(wb, "aggregations");
            {
                buffer_json_member_add_object(wb, "time");
                buffer_json_member_add_string(wb, "time_group", time_grouping_tostring(qt->request.time_group_method));
                buffer_json_member_add_string(wb, "time_group_options", qt->request.time_group_options);
                if (qt->request.resampling_time > 0)
                    buffer_json_member_add_time_t(wb, "time_resampling", qt->request.resampling_time);
                else
                    buffer_json_member_add_string(wb, "time_resampling", NULL);
                buffer_json_object_close(wb); // time

                buffer_json_member_add_object(wb, "metrics");

                buffer_json_member_add_array(wb, "group_by");
                buffer_json_group_by_to_array(wb, qt->request.group_by);
                buffer_json_array_close(wb);

                buffer_json_member_add_array(wb, "group_by_label");
                for(size_t l = 0; l < qt->group_by.used ;l++)
                    buffer_json_add_array_item_string(wb, qt->group_by.label_keys[l]);
                buffer_json_array_close(wb);

                buffer_json_member_add_string(wb, "aggregation",
                                              group_by_aggregate_function_to_string(
                                                      qt->request.group_by_aggregate_function));
                buffer_json_object_close(wb); // dimensions
            }
            buffer_json_object_close(wb); // aggregations

            buffer_json_member_add_uint64(wb, "timeout", qt->request.timeout);
        }
        buffer_json_object_close(wb); // request
    }

    buffer_json_member_add_object(wb, "versions");
    buffer_json_member_add_uint64(wb, "contexts_hard_hash", qt->versions.contexts_hard_hash);
    buffer_json_member_add_uint64(wb, "contexts_soft_hash", qt->versions.contexts_soft_hash);
    buffer_json_object_close(wb);

    size_t contexts;
    buffer_json_member_add_object(wb, "summary");
    struct summary_total_counts
            nodes_totals = { 0 },
            contexts_totals = { 0 },
            instances_totals = { 0 },
            metrics_totals = { 0 },
            label_key_totals = { 0 },
            label_key_value_totals = { 0 };
    {
        query_target_summary_nodes_v2(wb, qt, "nodes", &nodes_totals);
        contexts = query_target_summary_contexts_v2(wb, qt, "contexts", &contexts_totals);
        query_target_summary_instances_v2(wb, qt, "instances", &instances_totals);
        query_target_summary_dimensions_v12(wb, qt, "dimensions", true, &metrics_totals);
        query_target_summary_labels_v12(wb, qt, "labels", true, &label_key_totals, &label_key_value_totals);
        query_target_summary_alerts_v2(wb, qt, "alerts");
    }
    buffer_json_object_close(wb); // summary

    buffer_json_member_add_object(wb, "totals");
    query_target_total_counts(wb, "nodes", &nodes_totals);
    query_target_total_counts(wb, "contexts", &contexts_totals);
    query_target_total_counts(wb, "instances", &instances_totals);
    query_target_total_counts(wb, "dimensions", &metrics_totals);
    query_target_total_counts(wb, "label_keys", &label_key_totals);
    query_target_total_counts(wb, "label_key_values", &label_key_value_totals);
    buffer_json_object_close(wb); // totals

    if(options & RRDR_OPTION_SHOW_DETAILS) {
        buffer_json_member_add_object(wb, "detailed");
        query_target_detailed_objects_tree(wb, r, options);
        buffer_json_object_close(wb); // detailed
    }

    query_target_functions(wb, "functions", r);

    buffer_json_member_add_object(wb, "db");
    {
        buffer_json_member_add_uint64(wb, "tiers", storage_tiers);
        buffer_json_member_add_time_t(wb, "update_every", qt->db.minimum_latest_update_every_s);
        buffer_json_member_add_time_t(wb, "first_entry", qt->db.first_time_s);
        buffer_json_member_add_time_t(wb, "last_entry", qt->db.last_time_s);

        buffer_json_member_add_array(wb, "tiers");
        for(size_t tier = 0; tier < storage_tiers ; tier++) {
            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_uint64(wb, "tier", tier);
            buffer_json_member_add_uint64(wb, "queries", qt->db.tiers[tier].queries);
            buffer_json_member_add_uint64(wb, "points", qt->db.tiers[tier].points);
            buffer_json_member_add_time_t(wb, "update_every", qt->db.tiers[tier].update_every);
            buffer_json_member_add_time_t(wb, "first_entry", qt->db.tiers[tier].retention.first_time_s);
            buffer_json_member_add_time_t(wb, "last_entry", qt->db.tiers[tier].retention.last_time_s);
            buffer_json_object_close(wb);
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "view");
    {
        query_target_title(wb, qt, contexts);
        buffer_json_member_add_string(wb, "format", rrdr_format_to_string(format));
        web_client_api_request_v1_data_options_to_buffer_json_array(wb, "options", r->view.options);
        buffer_json_member_add_string(wb, "time_group", time_grouping_tostring(group_method));
        buffer_json_member_add_time_t(wb, "update_every", r->view.update_every);
        buffer_json_member_add_time_t(wb, "after", r->view.after);
        buffer_json_member_add_time_t(wb, "before", r->view.before);
        buffer_json_member_add_uint64(wb, "points", rows);
        query_target_combined_units_v2(wb, qt, contexts);
        query_target_combined_chart_type(wb, qt, contexts);
        buffer_json_member_add_object(wb, "dimensions");
        {
            rrdr_dimension_ids(wb, "ids", r, options);
            rrdr_dimension_names(wb, "names", r, options);
            rrdr_dimension_units_array_v2(wb, r, options);
            rrdr_dimension_priority_array(wb, r, options);
            rrdr_dimension_grouped_array(wb, r, options);
            size_t dims = rrdr_latest_values(wb, "view_latest_values", r, options);
            buffer_json_member_add_uint64(wb, "count", dims);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static void annotations_for_value_flags(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format __maybe_unused, RRDR_OPTIONS options, RRDR_VALUE_FLAGS flags, const char *type) {
    const size_t dims = r->d, rows = r->rows;
    size_t next_d_idx = 0;
    for(size_t d = 0; d < dims ; d++) {
        if(!rrdr_dimension_should_be_exposed(r->od[d], options))
            continue;

        size_t d_idx = next_d_idx++;

        size_t t = 0;
        while(t < rows) {

            // find the beginning
            time_t started = 0;
            for(; t < rows ;t++) {
                RRDR_VALUE_FLAGS o = r->o[t * r->d + d];
                if(o & flags) {
                    started = r->t[t];
                    break;
                }
            }

            if(started) {
                time_t ended = 0;
                for(; t < rows ;t++) {
                    RRDR_VALUE_FLAGS o = r->o[t * r->d + d];
                    if(!(o & flags)) {
                        ended = r->t[t];
                        break;
                    }
                }

                if(!ended)
                    ended = r->t[rows - 1];

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "t", type);
                // buffer_json_member_add_string(wb, "d", string2str(r->dn[d]));
                buffer_json_member_add_uint64(wb, "d", d_idx);
                if(started == ended) {
                    if(options & RRDR_OPTION_MILLISECONDS)
                        buffer_json_member_add_time_t2ms(wb, "x", started);
                    else
                        buffer_json_member_add_time_t(wb, "x", started);
                }
                else {
                    buffer_json_member_add_array(wb, "x");
                    if(options & RRDR_OPTION_MILLISECONDS) {
                        buffer_json_add_array_item_time_t2ms(wb, started);
                        buffer_json_add_array_item_time_t2ms(wb, ended);
                    }
                    else {
                        buffer_json_add_array_item_time_t(wb, started);
                        buffer_json_add_array_item_time_t(wb, ended);
                    }
                    buffer_json_array_close(wb);
                }
                buffer_json_object_close(wb);
            }
        }
    }
}

void rrdr_json_wrapper_annotations(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format __maybe_unused, RRDR_OPTIONS options) {
    buffer_json_member_add_array(wb, "annotations");

    annotations_for_value_flags(r, wb, format, options, RRDR_VALUE_EMPTY, "G"); // Gap
    annotations_for_value_flags(r, wb, format, options, RRDR_VALUE_RESET, "O"); // Overflow
    annotations_for_value_flags(r, wb, format, options, RRDR_VALUE_PARTIAL, "P"); // Partial

    buffer_json_array_close(wb); // annotations
}

void rrdr_json_wrapper_end(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format __maybe_unused, RRDR_OPTIONS options __maybe_unused) {
    QUERY_TARGET *qt = r->internal.qt;

    buffer_json_member_add_double(wb, "min", r->view.min);
    buffer_json_member_add_double(wb, "max", r->view.max);

    qt->timings.finished_ut = now_monotonic_usec();
    buffer_json_member_add_object(wb, "timings");
    buffer_json_member_add_double(wb, "prep_ms", (NETDATA_DOUBLE)(qt->timings.preprocessed_ut - qt->timings.received_ut) / USEC_PER_MS);
    buffer_json_member_add_double(wb, "query_ms", (NETDATA_DOUBLE)(qt->timings.executed_ut - qt->timings.preprocessed_ut) / USEC_PER_MS);
    buffer_json_member_add_double(wb, "group_by_ms", (NETDATA_DOUBLE)(qt->timings.group_by_ut - qt->timings.executed_ut) / USEC_PER_MS);
    buffer_json_member_add_double(wb, "output_ms", (NETDATA_DOUBLE)(qt->timings.finished_ut - qt->timings.group_by_ut) / USEC_PER_MS);
    buffer_json_member_add_double(wb, "total_ms", (NETDATA_DOUBLE)(qt->timings.finished_ut - qt->timings.received_ut) / USEC_PER_MS);
    buffer_json_object_close(wb);

    buffer_json_finalize(wb);
}
