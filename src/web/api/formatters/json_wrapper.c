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
    for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
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

static inline void aggregate_into_summary_totals(struct summary_total_counts *totals, QUERY_METRICS_COUNTS *metrics) {
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

static inline void query_target_metric_counts(BUFFER *wb, QUERY_METRICS_COUNTS *metrics) {
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

static inline void query_target_instance_counts(BUFFER *wb, QUERY_INSTANCES_COUNTS *instances) {
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

static inline void query_target_alerts_counts(BUFFER *wb, QUERY_ALERTS_COUNTS *alerts, const char *name, bool array) {
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

static inline void query_target_points_statistics(BUFFER *wb, QUERY_TARGET *qt, STORAGE_POINT *sp) {
    if(!sp->count)
        return;

    buffer_json_member_add_object(wb, "sts");

    buffer_json_member_add_double(wb, "min", sp->min);
    buffer_json_member_add_double(wb, "max", sp->max);

    if(query_target_aggregatable(qt)) {
        buffer_json_member_add_uint64(wb, "cnt", sp->count);

        if(sp->sum != 0.0) {
            buffer_json_member_add_double(wb, "sum", sp->sum);
            buffer_json_member_add_double(wb, "vol", sp->sum * (NETDATA_DOUBLE) query_view_update_every(qt));
        }

        if(sp->anomaly_count != 0)
            buffer_json_member_add_uint64(wb, "arc", sp->anomaly_count);
    }
    else {
        NETDATA_DOUBLE avg = (sp->count) ? sp->sum / (NETDATA_DOUBLE)sp->count : 0.0;
        if(avg != 0.0)
            buffer_json_member_add_double(wb, "avg", avg);

        NETDATA_DOUBLE arp = storage_point_anomaly_rate(*sp);
        if(arp != 0.0)
            buffer_json_member_add_double(wb, "arp", arp);

        NETDATA_DOUBLE con = (qt->query_points.sum > 0.0) ? sp->sum * 100.0 / qt->query_points.sum : 0.0;
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
        buffer_json_node_add_v2(wb, host, qn->slot, qn->duration_ut, true);
        query_target_instance_counts(wb, &qn->instances);
        query_target_metric_counts(wb, &qn->metrics);
        query_target_alerts_counts(wb, &qn->alerts, NULL, false);
        query_target_points_statistics(wb, qt, &qn->query_points);
        buffer_json_object_close(wb);

        aggregate_into_summary_totals(totals, &qn->metrics);
    }
    buffer_json_array_close(wb);
}

static size_t query_target_summary_contexts_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key, struct summary_total_counts *totals) {
    buffer_json_member_add_array(wb, key);
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);

    struct {
        STORAGE_POINT query_points;
        QUERY_INSTANCES_COUNTS instances;
        QUERY_METRICS_COUNTS metrics;
        QUERY_ALERTS_COUNTS alerts;
    } *z;

    for (long c = 0; c < (long) qt->contexts.used; c++) {
        QUERY_CONTEXT *qc = query_context(qt, c);

        z = dictionary_set(dict, rrdcontext_acquired_id(qc->rca), NULL, sizeof(*z));

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

        storage_point_merge_to(z->query_points, qc->query_points);
    }

    size_t unique_contexts = dictionary_entries(dict);
    dfe_start_read(dict, z) {
        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_string(wb, "id", z_dfe.name);
        query_target_instance_counts(wb, &z->instances);
        query_target_metric_counts(wb, &z->metrics);
        query_target_alerts_counts(wb, &z->alerts, NULL, false);
                query_target_points_statistics(wb, qt, &z->query_points);
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

        bool *set = dictionary_set(dict, name, NULL, sizeof(*set));
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
        query_target_points_statistics(wb, qt, &qi->query_points);
        buffer_json_object_close(wb);

        aggregate_into_summary_totals(totals, &qi->metrics);
    }
    buffer_json_array_close(wb);
}

struct dimensions_sorted_walkthrough_data {
    BUFFER *wb;
    struct summary_total_counts *totals;
    QUERY_TARGET *qt;
};

struct dimensions_sorted_entry {
    const char *id;
    const char *name;
    STORAGE_POINT query_points;
    QUERY_METRICS_COUNTS metrics;
    uint32_t priority;
};

static int dimensions_sorted_walktrhough_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    struct dimensions_sorted_walkthrough_data *sdwd = data;
    BUFFER *wb = sdwd->wb;
    struct summary_total_counts *totals = sdwd->totals;
    QUERY_TARGET *qt = sdwd->qt;
    struct dimensions_sorted_entry *z = value;

    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "id", z->id);
    if (z->id != z->name && z->name)
        buffer_json_member_add_string(wb, "nm", z->name);

    query_target_metric_counts(wb, &z->metrics);
    query_target_points_statistics(wb, qt, &z->query_points);
    buffer_json_member_add_uint64(wb, "pri", z->priority);
    buffer_json_object_close(wb);

    aggregate_into_summary_totals(totals, &z->metrics);

    return 1;
}

int dimensions_sorted_compar(const DICTIONARY_ITEM **item1, const DICTIONARY_ITEM **item2) {
    struct dimensions_sorted_entry *z1 = dictionary_acquired_item_value(*item1);
    struct dimensions_sorted_entry *z2 = dictionary_acquired_item_value(*item2);

    if(z1->priority == z2->priority)
        return strcmp(dictionary_acquired_item_name(*item1), dictionary_acquired_item_name(*item2));
    else if(z1->priority < z2->priority)
        return -1;
    else
        return 1;
}

static void query_target_summary_dimensions_v12(BUFFER *wb, QUERY_TARGET *qt, const char *key, bool v2, struct summary_total_counts *totals) {
    char buf[RRD_ID_LENGTH_MAX * 2 + 2];

    buffer_json_member_add_array(wb, key);
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
    struct dimensions_sorted_entry *z;
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

        const char *k, *id, *name;

        if(v2) {
            k = rrdmetric_acquired_name(rma);
            id = k;
            name = k;
        }
        else {
            snprintfz(buf, RRD_ID_LENGTH_MAX * 2 + 1, "%s:%s",
                      rrdmetric_acquired_id(rma),
                      rrdmetric_acquired_name(rma));
            k = buf;
            id = rrdmetric_acquired_id(rma);
            name = rrdmetric_acquired_name(rma);
        }

        z = dictionary_set(dict, k, NULL, sizeof(*z));
        if(!z->id) {
            z->id = id;
            z->name = name;
            z->priority = qd->priority;
        }
        else {
            if(qd->priority < z->priority)
                z->priority = qd->priority;
        }

        if(qm) {
            z->metrics.selected += (qm->status & RRDR_DIMENSION_SELECTED) ? 1 : 0;
            z->metrics.failed += (qm->status & RRDR_DIMENSION_FAILED) ? 1 : 0;

            if(qm->status & RRDR_DIMENSION_QUERIED) {
                z->metrics.queried++;
                storage_point_merge_to(z->query_points, qm->query_points);
            }
        }
        else
            z->metrics.excluded++;
    }

    if(v2) {
        struct dimensions_sorted_walkthrough_data t = {
                .wb = wb,
                .totals = totals,
                .qt = qt,
        };
        dictionary_sorted_walkthrough_rw(dict, DICTIONARY_LOCK_READ, dimensions_sorted_walktrhough_cb,
                                         &t, dimensions_sorted_compar);
    }
    else {
        // v1
        dfe_start_read(dict, z) {
                buffer_json_add_array_item_array(wb);
                buffer_json_add_array_item_string(wb, z->id);
                buffer_json_add_array_item_string(wb, z->name);
                buffer_json_array_close(wb);
        }
        dfe_done(z);
    }
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
    STORAGE_POINT query_points;
    QUERY_METRICS_COUNTS metrics;
};

struct rrdlabels_key_value_dict_entry {
    const char *key;
    const char *value;
    STORAGE_POINT query_points;
    QUERY_METRICS_COUNTS metrics;
};

static int rrdlabels_formatting_v2(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    struct rrdlabels_formatting_v2 *t = data;

    struct rrdlabels_keys_dict_entry *d = dictionary_set(t->keys, name, NULL, sizeof(*d));
    if(!d->values) {
        d->name = name;
        d->values = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
    }

    char n[RRD_ID_LENGTH_MAX * 2 + 2];
    snprintfz(n, RRD_ID_LENGTH_MAX * 2, "%s:%s", name, value);

    struct rrdlabels_key_value_dict_entry *z = dictionary_set(d->values, n, NULL, sizeof(*z));
    if(!z->key) {
        z->key = name;
        z->value = value;
    }

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

        storage_point_merge_to(z->query_points, qi->query_points);
        storage_point_merge_to(d->query_points, qi->query_points);
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
                    query_target_points_statistics(wb, qt, &d->query_points);
                    aggregate_into_summary_totals(key_totals, &d->metrics);
                    buffer_json_member_add_array(wb, "vl");
                }
                struct rrdlabels_key_value_dict_entry *z;
                dfe_start_read(d->values, z){
                            if (v2) {
                                buffer_json_add_array_item_object(wb);
                                buffer_json_member_add_string(wb, "id", z->value);
                                query_target_metric_counts(wb, &z->metrics);
                                query_target_points_statistics(wb, qt, &z->query_points);
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
    QUERY_ALERTS_COUNTS *z;

    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
    for (long c = 0; c < (long) qt->instances.used; c++) {
        QUERY_INSTANCE *qi = query_instance(qt, c);
        RRDSET *st = rrdinstance_acquired_rrdset(qi->ria);
        if (st) {
            rw_spinlock_read_lock(&st->alerts.spinlock);
            if (st->alerts.base) {
                for (RRDCALC *rc = st->alerts.base; rc; rc = rc->next) {
                    z = dictionary_set(dict, string2str(rc->config.name), NULL, sizeof(*z));

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
            rw_spinlock_read_unlock(&st->alerts.spinlock);
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

static inline size_t rrdr_dimension_view_latest_values(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    buffer_json_member_add_array(wb, key);

    size_t c, i;
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
        else
            buffer_json_add_array_item_double(wb, n);
    }

    buffer_json_array_close(wb);

    return i;
}

static inline void rrdr_dimension_query_points_statistics(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options, bool dview) {
    STORAGE_POINT *sp = (dview) ? r->dview : r->dqp;
    NETDATA_DOUBLE anomaly_rate_multiplier = (dview) ? RRDR_DVIEW_ANOMALY_COUNT_MULTIPLIER : 1.0;

    if(unlikely(!sp))
        return;

    if(key)
        buffer_json_member_add_object(wb, key);

    buffer_json_member_add_array(wb, "min");
    for(size_t c = 0; c < r->d ; c++) {
        if (!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        buffer_json_add_array_item_double(wb, sp[c].min);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "max");
    for(size_t c = 0; c < r->d ; c++) {
        if (!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        buffer_json_add_array_item_double(wb, sp[c].max);
    }
    buffer_json_array_close(wb);

    if(options & RRDR_OPTION_RETURN_RAW) {
        buffer_json_member_add_array(wb, "sum");
        for(size_t c = 0; c < r->d ; c++) {
            if (!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            buffer_json_add_array_item_double(wb, sp[c].sum);
        }
        buffer_json_array_close(wb);

        buffer_json_member_add_array(wb, "cnt");
        for(size_t c = 0; c < r->d ; c++) {
            if (!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            buffer_json_add_array_item_uint64(wb, sp[c].count);
        }
        buffer_json_array_close(wb);

        buffer_json_member_add_array(wb, "arc");
        for(size_t c = 0; c < r->d ; c++) {
            if (!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            buffer_json_add_array_item_uint64(wb, storage_point_anomaly_rate(sp[c]) / anomaly_rate_multiplier / 100.0 * sp[c].count);
        }
        buffer_json_array_close(wb);
    }
    else {
        NETDATA_DOUBLE sum = 0.0;
        for(size_t c = 0; c < r->d ; c++) {
            if(!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            sum += ABS(sp[c].sum);
        }

        buffer_json_member_add_array(wb, "avg");
        for(size_t c = 0; c < r->d ; c++) {
            if (!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            buffer_json_add_array_item_double(wb, storage_point_average_value(sp[c]));
        }
        buffer_json_array_close(wb);

        buffer_json_member_add_array(wb, "arp");
        for(size_t c = 0; c < r->d ; c++) {
            if (!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            buffer_json_add_array_item_double(wb, storage_point_anomaly_rate(sp[c]) / anomaly_rate_multiplier);
        }
        buffer_json_array_close(wb);

        buffer_json_member_add_array(wb, "con");
        for(size_t c = 0; c < r->d ; c++) {
            if (!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            NETDATA_DOUBLE con = (sum > 0.0) ? ABS(sp[c].sum) * 100.0 / sum : 0.0;
            buffer_json_add_array_item_double(wb, con);
        }
        buffer_json_array_close(wb);
    }

    if(key)
        buffer_json_object_close(wb);
}

void rrdr_json_wrapper_begin(RRDR *r, BUFFER *wb) {
    QUERY_TARGET *qt = r->internal.qt;
    DATASOURCE_FORMAT format = qt->request.format;
    RRDR_OPTIONS options = qt->window.options;

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

    buffer_json_initialize(
        wb, kq, sq, 0, true, (options & RRDR_OPTION_MINIFY) ? BUFFER_JSON_OPTIONS_MINIFY : BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_uint64(wb, "api", 1);
    buffer_json_member_add_string(wb, "id", qt->id);
    buffer_json_member_add_string(wb, "name", qt->id);
    buffer_json_member_add_time_t(wb, "view_update_every", r->view.update_every);
    buffer_json_member_add_time_t(wb, "update_every", qt->db.minimum_latest_update_every_s);
    buffer_json_member_add_time_t(wb, "first_entry", qt->db.first_time_s);
    buffer_json_member_add_time_t(wb, "last_entry", qt->db.last_time_s);
    buffer_json_member_add_time_t(wb, "after", r->view.after);
    buffer_json_member_add_time_t(wb, "before", r->view.before);
    buffer_json_member_add_string(wb, "group", time_grouping_tostring(qt->request.time_group_method));
    rrdr_options_to_buffer_json_array(wb, "options", options);

    if(!rrdr_dimension_names(wb, "dimension_names", r, options))
        rows = 0;

    if(!rrdr_dimension_ids(wb, "dimension_ids", r, options))
        rows = 0;

    if (options & RRDR_OPTION_ALL_DIMENSIONS) {
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

    size_t dimensions = rrdr_dimension_view_latest_values(wb, "view_latest_values", r, options);
    if(!dimensions)
        rows = 0;

    buffer_json_member_add_uint64(wb, "dimensions", dimensions);
    buffer_json_member_add_uint64(wb, "points", rows);
    buffer_json_member_add_string(wb, "format", rrdr_format_to_string(format));

    buffer_json_member_add_array(wb, "db_points_per_tier");
    for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
        buffer_json_add_array_item_uint64(wb, qt->db.tiers[tier].points);
    buffer_json_array_close(wb);

    if(options & RRDR_OPTION_DEBUG)
        jsonwrap_query_plan(r, wb);
}

static void rrdset_rrdcalc_entries_v2(BUFFER *wb, RRDINSTANCE_ACQUIRED *ria) {
    RRDSET *st = rrdinstance_acquired_rrdset(ria);
    if(st) {
        rw_spinlock_read_lock(&st->alerts.spinlock);
        if(st->alerts.base) {
            buffer_json_member_add_object(wb, "alerts");
            for(RRDCALC *rc = st->alerts.base; rc ;rc = rc->next) {
                if(rc->status < RRDCALC_STATUS_CLEAR)
                    continue;

                buffer_json_member_add_object(wb, string2str(rc->config.name));
                buffer_json_member_add_string(wb, "st", rrdcalc_status2string(rc->status));
                buffer_json_member_add_double(wb, "vl", rc->value);
                buffer_json_member_add_string(wb, "un", string2str(rc->config.units));
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);
        }
        rw_spinlock_read_unlock(&st->alerts.spinlock);
    }
}

static void query_target_combined_units_v2(BUFFER *wb, QUERY_TARGET *qt, size_t contexts, bool ignore_percentage) {
    if(!ignore_percentage && query_target_has_percentage_units(qt)) {
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

static void rrdr_grouped_by_array_v2(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options __maybe_unused) {
    QUERY_TARGET *qt = r->internal.qt;

    buffer_json_member_add_array(wb, key);

    // find the deeper group-by
    ssize_t g = 0;
    for(g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++) {
        if(qt->request.group_by[g].group_by == RRDR_GROUP_BY_NONE)
            break;
    }

    if(g > 0)
        g--;

    RRDR_GROUP_BY group_by = qt->request.group_by[g].group_by;

    if(group_by & RRDR_GROUP_BY_SELECTED)
        buffer_json_add_array_item_string(wb, "selected");

    else if(group_by & RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE)
        buffer_json_add_array_item_string(wb, "percentage-of-instance");

    else {

        if(group_by & RRDR_GROUP_BY_DIMENSION)
            buffer_json_add_array_item_string(wb, "dimension");

        if(group_by & RRDR_GROUP_BY_INSTANCE)
            buffer_json_add_array_item_string(wb, "instance");

        if(group_by & RRDR_GROUP_BY_LABEL) {
            BUFFER *b = buffer_create(0, NULL);
            for (size_t l = 0; l < qt->group_by[g].used; l++) {
                buffer_flush(b);
                buffer_fast_strcat(b, "label:", 6);
                buffer_strcat(b, qt->group_by[g].label_keys[l]);
                buffer_json_add_array_item_string(wb, buffer_tostring(b));
            }
            buffer_free(b);
        }

        if(group_by & RRDR_GROUP_BY_NODE)
            buffer_json_add_array_item_string(wb, "node");

        if(group_by & RRDR_GROUP_BY_CONTEXT)
            buffer_json_add_array_item_string(wb, "context");

        if(group_by & RRDR_GROUP_BY_UNITS)
            buffer_json_add_array_item_string(wb, "units");
    }

    buffer_json_array_close(wb); // group_by_order
}

static void rrdr_dimension_units_array_v2(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options, bool ignore_percentage) {
    if(!r->du)
        return;

    bool percentage = !ignore_percentage && query_target_has_percentage_units(r->internal.qt);

    buffer_json_member_add_array(wb, key);
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

static void rrdr_dimension_priority_array_v2(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    if(!r->dp)
        return;

    buffer_json_member_add_array(wb, key);
    for(size_t c = 0; c < r->d ; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        buffer_json_add_array_item_uint64(wb, r->dp[c]);
    }
    buffer_json_array_close(wb);
}

static void rrdr_dimension_aggregated_array_v2(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    if(!r->dgbc)
        return;

    buffer_json_member_add_array(wb, key);
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
            bool *set = dictionary_set(dict, rrdcontext_acquired_id(qt->contexts.array[c].rca), NULL, sizeof(*set));
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
                        RRDLABELS *labels = rrdinstance_acquired_labels(ria);
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

                            query_target_points_statistics(wb, qt, &qm->query_points);

                            if(options & RRDR_OPTION_DEBUG)
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

void version_hashes_api_v2(BUFFER *wb, struct query_versions *versions) {
    buffer_json_member_add_object(wb, "versions");
    buffer_json_member_add_uint64(wb, "routing_hard_hash", 1);
    buffer_json_member_add_uint64(wb, "nodes_hard_hash", dictionary_version(rrdhost_root_index));
    buffer_json_member_add_uint64(wb, "contexts_hard_hash", versions->contexts_hard_hash);
    buffer_json_member_add_uint64(wb, "contexts_soft_hash", versions->contexts_soft_hash);
    buffer_json_member_add_uint64(wb, "alerts_hard_hash", versions->alerts_hard_hash);
    buffer_json_member_add_uint64(wb, "alerts_soft_hash", versions->alerts_soft_hash);
    buffer_json_object_close(wb);
}

void rrdr_json_wrapper_begin2(RRDR *r, BUFFER *wb) {
    QUERY_TARGET *qt = r->internal.qt;
    RRDR_OPTIONS options = qt->window.options;

    char kq[2] = "\"",                    // key quote
         sq[2] = "\"";                    // string quote

    if(unlikely(options & RRDR_OPTION_GOOGLE_JSON)) {
        kq[0] = '\0';
        sq[0] = '\'';
    }

    buffer_json_initialize(
        wb, kq, sq, 0, true, (options & RRDR_OPTION_MINIFY) ? BUFFER_JSON_OPTIONS_MINIFY : BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_uint64(wb, "api", 2);

    if(options & RRDR_OPTION_DEBUG) {
        buffer_json_member_add_string(wb, "id", qt->id);
        buffer_json_member_add_object(wb, "request");
        {
            buffer_json_member_add_string(wb, "format", rrdr_format_to_string(qt->request.format));
            rrdr_options_to_buffer_json_array(wb, "options", qt->request.options);

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

                buffer_json_member_add_array(wb, "metrics");
                for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++) {
                    if(qt->request.group_by[g].group_by == RRDR_GROUP_BY_NONE)
                        break;

                    buffer_json_add_array_item_object(wb);
                    {
                        buffer_json_member_add_array(wb, "group_by");
                        buffer_json_group_by_to_array(wb, qt->request.group_by[g].group_by);
                        buffer_json_array_close(wb);

                        buffer_json_member_add_array(wb, "group_by_label");
                        for (size_t l = 0; l < qt->group_by[g].used; l++)
                            buffer_json_add_array_item_string(wb, qt->group_by[g].label_keys[l]);
                        buffer_json_array_close(wb);

                        buffer_json_member_add_string(
                                wb, "aggregation",group_by_aggregate_function_to_string(qt->request.group_by[g].aggregation));
                    }
                    buffer_json_object_close(wb);
                }
                buffer_json_array_close(wb); // group_by
            }
            buffer_json_object_close(wb); // aggregations

            buffer_json_member_add_uint64(wb, "timeout", qt->request.timeout_ms);
        }
        buffer_json_object_close(wb); // request
    }

    version_hashes_api_v2(wb, &qt->versions);

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
        r->internal.contexts = query_target_summary_contexts_v2(wb, qt, "contexts", &contexts_totals);
        query_target_summary_instances_v2(wb, qt, "instances", &instances_totals);
        query_target_summary_dimensions_v12(wb, qt, "dimensions", true, &metrics_totals);
        query_target_summary_labels_v12(wb, qt, "labels", true, &label_key_totals, &label_key_value_totals);
        query_target_summary_alerts_v2(wb, qt, "alerts");
    }
    if(query_target_aggregatable(qt)) {
        buffer_json_member_add_object(wb, "globals");
        query_target_points_statistics(wb, qt, &qt->query_points);
        buffer_json_object_close(wb); // globals
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
}

//static void annotations_range_for_value_flags(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format __maybe_unused, RRDR_OPTIONS options, RRDR_VALUE_FLAGS flags, const char *type) {
//    const size_t dims = r->d, rows = r->rows;
//    size_t next_d_idx = 0;
//    for(size_t d = 0; d < dims ; d++) {
//        if(!rrdr_dimension_should_be_exposed(r->od[d], options))
//            continue;
//
//        size_t d_idx = next_d_idx++;
//
//        size_t t = 0;
//        while(t < rows) {
//
//            // find the beginning
//            time_t started = 0;
//            for(; t < rows ;t++) {
//                RRDR_VALUE_FLAGS o = r->o[t * r->d + d];
//                if(o & flags) {
//                    started = r->t[t];
//                    break;
//                }
//            }
//
//            if(started) {
//                time_t ended = 0;
//                for(; t < rows ;t++) {
//                    RRDR_VALUE_FLAGS o = r->o[t * r->d + d];
//                    if(!(o & flags)) {
//                        ended = r->t[t];
//                        break;
//                    }
//                }
//
//                if(!ended)
//                    ended = r->t[rows - 1];
//
//                buffer_json_add_array_item_object(wb);
//                buffer_json_member_add_string(wb, "t", type);
//                // buffer_json_member_add_string(wb, "d", string2str(r->dn[d]));
//                buffer_json_member_add_uint64(wb, "d", d_idx);
//                if(started == ended) {
//                    if(options & RRDR_OPTION_MILLISECONDS)
//                        buffer_json_member_add_time_t2ms(wb, "x", started);
//                    else
//                        buffer_json_member_add_time_t(wb, "x", started);
//                }
//                else {
//                    buffer_json_member_add_array(wb, "x");
//                    if(options & RRDR_OPTION_MILLISECONDS) {
//                        buffer_json_add_array_item_time_t2ms(wb, started);
//                        buffer_json_add_array_item_time_t2ms(wb, ended);
//                    }
//                    else {
//                        buffer_json_add_array_item_time_t(wb, started);
//                        buffer_json_add_array_item_time_t(wb, ended);
//                    }
//                    buffer_json_array_close(wb);
//                }
//                buffer_json_object_close(wb);
//            }
//        }
//    }
//}
//
//void rrdr_json_wrapper_annotations(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format __maybe_unused, RRDR_OPTIONS options) {
//    buffer_json_member_add_array(wb, "annotations");
//
//    annotations_range_for_value_flags(r, wb, format, options, RRDR_VALUE_EMPTY, "G"); // Gap
//    annotations_range_for_value_flags(r, wb, format, options, RRDR_VALUE_RESET, "O"); // Overflow
//    annotations_range_for_value_flags(r, wb, format, options, RRDR_VALUE_PARTIAL, "P"); // Partial
//
//    buffer_json_array_close(wb); // annotations
//}

void rrdr_json_wrapper_end(RRDR *r, BUFFER *wb) {
    buffer_json_member_add_double(wb, "min", r->view.min);
    buffer_json_member_add_double(wb, "max", r->view.max);

    buffer_json_query_timings(wb, "timings", &r->internal.qt->timings);
    buffer_json_finalize(wb);
}

void rrdr_json_wrapper_end2(RRDR *r, BUFFER *wb) {
    QUERY_TARGET *qt = r->internal.qt;
    DATASOURCE_FORMAT format = qt->request.format;
    RRDR_OPTIONS options = qt->window.options;

    buffer_json_member_add_object(wb, "db");
    {
        buffer_json_member_add_uint64(wb, "tiers", nd_profile.storage_tiers);
        buffer_json_member_add_time_t(wb, "update_every", qt->db.minimum_latest_update_every_s);
        buffer_json_member_add_time_t(wb, "first_entry", qt->db.first_time_s);
        buffer_json_member_add_time_t(wb, "last_entry", qt->db.last_time_s);

        query_target_combined_units_v2(wb, qt, r->internal.contexts, true);
        buffer_json_member_add_object(wb, "dimensions");
        {
            rrdr_dimension_ids(wb, "ids", r, options);
            rrdr_dimension_units_array_v2(wb, "units", r, options, true);
            rrdr_dimension_query_points_statistics(wb, "sts", r, options, false);
        }
        buffer_json_object_close(wb); // dimensions

        buffer_json_member_add_array(wb, "per_tier");
        for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
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
        query_target_title(wb, qt, r->internal.contexts);
        buffer_json_member_add_time_t(wb, "update_every", r->view.update_every);
        buffer_json_member_add_time_t(wb, "after", r->view.after);
        buffer_json_member_add_time_t(wb, "before", r->view.before);

        if(options & RRDR_OPTION_DEBUG) {
            buffer_json_member_add_string(wb, "format", rrdr_format_to_string(format));
            rrdr_options_to_buffer_json_array(wb, "options", options);
            buffer_json_member_add_string(wb, "time_group", time_grouping_tostring(qt->request.time_group_method));
        }

        if(options & RRDR_OPTION_DEBUG) {
            buffer_json_member_add_object(wb, "partial_data_trimming");
            buffer_json_member_add_time_t(wb, "max_update_every", r->partial_data_trimming.max_update_every);
            buffer_json_member_add_time_t(wb, "expected_after", r->partial_data_trimming.expected_after);
            buffer_json_member_add_time_t(wb, "trimmed_after", r->partial_data_trimming.trimmed_after);
            buffer_json_object_close(wb);
        }

        if(options & RRDR_OPTION_RETURN_RAW)
            buffer_json_member_add_uint64(wb, "points", rrdr_rows(r));

        query_target_combined_units_v2(wb, qt, r->internal.contexts, false);
        query_target_combined_chart_type(wb, qt, r->internal.contexts);
        buffer_json_member_add_object(wb, "dimensions");
        {
            rrdr_grouped_by_array_v2(wb, "grouped_by", r, options);
            rrdr_dimension_ids(wb, "ids", r, options);
            rrdr_dimension_names(wb, "names", r, options);
            rrdr_dimension_units_array_v2(wb, "units", r, options, false);
            rrdr_dimension_priority_array_v2(wb, "priorities", r, options);
            rrdr_dimension_aggregated_array_v2(wb, "aggregated", r, options);
            rrdr_dimension_query_points_statistics(wb, "sts", r, options, true);
            rrdr_json_group_by_labels(wb, "labels", r, options);
        }
        buffer_json_object_close(wb); // dimensions
        buffer_json_member_add_double(wb, "min", r->view.min);
        buffer_json_member_add_double(wb, "max", r->view.max);
    }
    buffer_json_object_close(wb); // view

    buffer_json_agents_v2(wb, &r->internal.qt->timings, 0, false, true);
    buffer_json_cloud_timings(wb, "timings", &r->internal.qt->timings);
    buffer_json_finalize(wb);
}
