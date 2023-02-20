// SPDX-License-Identifier: GPL-3.0-or-later

#include "json_wrapper.h"

static void jsonwrap_query_metric_plan(BUFFER *wb, QUERY_METRIC *qm) {
    buffer_json_member_add_array(wb, "plans");
    for (size_t p = 0; p < qm->plan.used; p++) {
        QUERY_PLAN_ENTRY *qp = &qm->plan.array[p];

        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_uint64(wb, "tier", qp->tier);
        buffer_json_member_add_time_t(wb, "after", qp->after);
        buffer_json_member_add_time_t(wb, "before", qp->before);
        buffer_json_object_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "tiers");
    for (size_t tier = 0; tier < storage_tiers; tier++) {
        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_uint64(wb, "tier", tier);
        buffer_json_member_add_time_t(wb, "first_entry", qm->tiers[tier].db_first_time_s);
        buffer_json_member_add_time_t(wb, "last_entry", qm->tiers[tier].db_last_time_s);
        buffer_json_member_add_int64(wb, "weight", qm->tiers[tier].weight);
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
        buffer_json_add_array_item_string(wb, string2str(qi->id_fqdn));
        i++;
    }
    buffer_json_array_close(wb);

    return i;
}

struct rrdlabels_formatting_v2 {
    DICTIONARY *dict;
    QUERY_INSTANCE *qi;
};

struct rrdlabels_dict_entry {
    const char *name;
    const char *value;
    size_t selected;
    size_t excluded;
    size_t queried;
    size_t failed;
};

static int rrdlabels_formatting_v2(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    struct rrdlabels_formatting_v2 *t = data;
    DICTIONARY *dict = t->dict;

    char n[RRD_ID_LENGTH_MAX * 2 + 2];
    snprintfz(n, RRD_ID_LENGTH_MAX * 2, "%s:%s", name, value);

    struct rrdlabels_dict_entry x = {
        .name = name,
        .value = value,
        .selected = 0,
        .excluded = 0,
        .queried = 0,
        .failed = 0,
    };
    struct rrdlabels_dict_entry *z = dictionary_set(dict, n, &x, sizeof(x));
    z->selected += t->qi->selected;
    z->excluded += t->qi->excluded;
    z->queried += t->qi->queried;
    z->failed += t->qi->failed;

    return 1;
}

static inline void query_target_metric_count(BUFFER *wb, size_t selected, size_t excluded, size_t queried, size_t failed) {
    buffer_json_member_add_object(wb, "ds");
    buffer_json_member_add_uint64(wb, "sl", selected);
    buffer_json_member_add_uint64(wb, "ex", excluded);
    buffer_json_member_add_uint64(wb, "qr", queried);
    buffer_json_member_add_uint64(wb, "fl", failed);
    buffer_json_object_close(wb);
}

static inline void query_target_value_stats(BUFFER *wb, NETDATA_DOUBLE min, NETDATA_DOUBLE max, NETDATA_DOUBLE sum, NETDATA_DOUBLE avg, NETDATA_DOUBLE vol) {
    buffer_json_member_add_object(wb, "sts");
    buffer_json_member_add_double(wb, "min", min);
    buffer_json_member_add_double(wb, "max", max);
    buffer_json_member_add_double(wb, "sum", sum);
    buffer_json_member_add_double(wb, "avg", avg);
    buffer_json_member_add_double(wb, "vol", vol);
    buffer_json_object_close(wb);
}

static inline void query_target_hosts_instances_labels_dimensions(
        BUFFER *wb, RRDR *r,
        const char *key_hosts,
        const char *key_dimensions,
        const char *key_instances,
        const char *key_labels,
        const char *key_alerts,
        bool v2) {
    QUERY_TARGET *qt = r->internal.qt;

    char name[RRD_ID_LENGTH_MAX * 2 + 2];

    if(key_hosts) {
        buffer_json_member_add_array(wb, key_hosts);
        for (long c = 0; c < (long) qt->hosts.used; c++) {
            QUERY_HOST *qh = query_host(qt, c);
            RRDHOST *host = qh->host;
            if(v2) {
                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "mg", host->machine_guid);
                buffer_json_member_add_uuid(wb, "nd", host->node_id);
                buffer_json_member_add_string(wb, "hn", rrdhost_hostname(host));
                query_target_metric_count(wb, qh->selected, qh->excluded, qh->queried, qh->failed);
                buffer_json_object_close(wb);
            }
            else {
                buffer_json_add_array_item_array(wb);
                buffer_json_add_array_item_string(wb, host->machine_guid);
                buffer_json_add_array_item_string(wb, rrdhost_hostname(host));
                buffer_json_array_close(wb);
            }
        }
        buffer_json_array_close(wb);
    }

    if(key_instances) {
        buffer_json_member_add_array(wb, key_instances);
        DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
        for (long c = 0; c < (long) qt->instances.used; c++) {
            QUERY_INSTANCE *qi = query_instance(qt, c);

            snprintfz(name, RRD_ID_LENGTH_MAX * 2 + 1, "%s:%s",
                      string2str(qi->id_fqdn),
                      string2str(qi->name_fqdn));

            bool existing = 0;
            bool *set = dictionary_set(dict, name, &existing, sizeof(bool));
            if (!*set) {
                *set = true;
                if(v2) {
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "id", string2str(qi->id_fqdn));
                    buffer_json_member_add_string(wb, "nm", string2str(qi->name_fqdn));
                    buffer_json_member_add_string(wb, "lc", rrdinstance_acquired_name(qi->ria));
                    query_target_metric_count(wb, qi->selected, qi->excluded, qi->queried, qi->failed);
                    buffer_json_object_close(wb);
                }
                else {
                    buffer_json_add_array_item_array(wb);
                    buffer_json_add_array_item_string(wb, string2str(qi->id_fqdn));
                    buffer_json_add_array_item_string(wb, string2str(qi->name_fqdn));
                    buffer_json_array_close(wb);
                }
            }
        }
        dictionary_destroy(dict);
        buffer_json_array_close(wb);
    }

    if(key_dimensions) {
        buffer_json_member_add_array(wb, key_dimensions);
        DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
        struct {
            const char *id;
            const char *name;
            size_t selected;
            size_t queried;
            size_t excluded;
            size_t failed;
        } x, *z;
        size_t q = 0;
        for (long c = 0; c < (long) qt->dimensions.used; c++) {
            QUERY_DIMENSION * qd = query_dimension(qt, c);
            RRDMETRIC_ACQUIRED *rma = qd->rma;

            RRDR_DIMENSION_FLAGS options = RRDR_DIMENSION_DEFAULT;
            bool found = false;
            for( ; q < qt->query.used ;q++) {
                QUERY_METRIC *tqm = query_metric(qt, q);
                QUERY_DIMENSION *tqd = query_dimension(qt, tqm->link.query_dimension_id);
                if(tqd->rma != rma) break;
                options = tqm->query.options;
                found = true;
            }

            snprintfz(name, RRD_ID_LENGTH_MAX * 2 + 1, "%s:%s",
                      rrdmetric_acquired_id(rma),
                      rrdmetric_acquired_name(rma));

            x.id = rrdmetric_acquired_id(rma);
            x.name = rrdmetric_acquired_name(rma);
            x.selected = 0;
            x.excluded = 0;
            x.queried = 0;
            x.failed = 0;

            z = dictionary_set(dict, name, &x, sizeof(x));
            z->selected += (options & RRDR_DIMENSION_SELECTED) ? 1 : 0;
            z->excluded += (!found) ? 1 : 0;
            z->queried += (options & RRDR_DIMENSION_QUERIED) ? 1 : 0;
            z->failed += (options & RRDR_DIMENSION_FAILED) ? 1 : 0;
        }
        dfe_start_read(dict, z) {
                    if(v2) {
                        buffer_json_add_array_item_object(wb);
                        buffer_json_member_add_string(wb, "id", z->id);
                        buffer_json_member_add_string(wb, "nm", z->name);
                        query_target_metric_count(wb, z->selected, z->excluded, z->queried, z->failed);
                        buffer_json_object_close(wb);
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

    if(key_labels) {
        buffer_json_member_add_array(wb, key_labels);
        struct rrdlabels_formatting_v2 t = {
                .dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE),
        };
        for (long c = 0; c < (long) qt->instances.used; c++) {
            QUERY_INSTANCE *qi = query_instance(qt, c);
            RRDINSTANCE_ACQUIRED *ria = qi->ria;
            t.qi = qi;
            rrdlabels_walkthrough_read(rrdinstance_acquired_labels(ria), rrdlabels_formatting_v2, &t);
        }
        struct rrdlabels_dict_entry *z;
        dfe_start_read(t.dict, z) {
                    if(v2) {
                        buffer_json_add_array_item_object(wb);
                        buffer_json_member_add_string(wb, "nm", z->name);
                        buffer_json_member_add_string(wb, "vl", z->value);
                        query_target_metric_count(wb, z->selected, z->excluded, z->queried, z->failed);
                        buffer_json_object_close(wb);
                    }
                    else {
                        buffer_json_add_array_item_array(wb);
                        buffer_json_add_array_item_string(wb, z->name);
                        buffer_json_add_array_item_string(wb, z->value);
                        buffer_json_array_close(wb);
                    }
        }
        dfe_done(z);
        dictionary_destroy(t.dict);
        buffer_json_array_close(wb);
    }

    if(key_alerts) {
        buffer_json_member_add_array(wb, key_alerts);
        for (long c = 0; c < (long) qt->instances.used; c++) {
            QUERY_INSTANCE *qi = query_instance(qt, c);
            RRDSET *st = rrdinstance_acquired_rrdset(qi->ria);
            if (st) {
                netdata_rwlock_rdlock(&st->alerts.rwlock);
                if (st->alerts.base) {
                    char id[RRD_ID_LENGTH_MAX + 1];
                    for (RRDCALC *rc = st->alerts.base; rc; rc = rc->next) {
//                        if(rc->status < RRDCALC_STATUS_CLEAR)
//                            continue;

                        snprintfz(id, RRD_ID_LENGTH_MAX, "%s:%s", string2str(rc->name), string2str(qi->id_fqdn));
                        snprintfz(name, RRD_ID_LENGTH_MAX, "%s:%s", string2str(rc->name), string2str(qi->name_fqdn));
                        buffer_json_add_array_item_object(wb);
                        buffer_json_member_add_string(wb, "id", id);
                        buffer_json_member_add_string(wb, "nm", name);
                        buffer_json_member_add_string(wb, "lc", string2str(rc->name));
                        buffer_json_member_add_string(wb, "st", rrdcalc_status2string(rc->status));
                        buffer_json_member_add_double(wb, "vl", rc->value);
                        buffer_json_object_close(wb);
                    }
                }
                netdata_rwlock_unlock(&st->alerts.rwlock);
            }
        }
        buffer_json_array_close(wb);
    }
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

static inline long query_target_chart_labels_filter(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
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

void rrdr_json_wrapper_begin(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format, RRDR_OPTIONS options, bool string_value,
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

    buffer_json_initialize(wb, kq, sq, 0, true);

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

    if (r->view.options & RRDR_OPTION_ALL_DIMENSIONS)
        query_target_hosts_instances_labels_dimensions(
                wb, r, NULL, "full_dimension_list", "full_chart_list",
                "full_chart_labels", NULL, false);

    query_target_functions(wb, "functions", r);

    if (!qt->request.st && !jsonwrap_v1_chart_ids(wb, "chart_ids", r, options))
        rows = 0;

    if (qt->instances.chart_label_key_pattern && !query_target_chart_labels_filter(wb, "chart_labels", r, options))
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
        buffer_json_add_array_item_uint64(wb, r->stats.tier_points_read[tier]);
    buffer_json_array_close(wb);

    if(options & RRDR_OPTION_SHOW_PLAN)
        jsonwrap_query_plan(r, wb);

    buffer_sprintf(wb, ",\n    %sresult%s:", kq, kq);
    if(string_value) buffer_strcat(wb, sq);
}

static void rrdset_rrdcalc_entries(BUFFER *wb, RRDINSTANCE_ACQUIRED *ria) {
    RRDSET *st = rrdinstance_acquired_rrdset(ria);
    if(st) {
        netdata_rwlock_rdlock(&st->alerts.rwlock);
        if(st->alerts.base) {
            buffer_json_member_add_object(wb, "alerts");
            for(RRDCALC *rc = st->alerts.base; rc ;rc = rc->next) {
                buffer_json_member_add_object(wb, string2str(rc->name));
                buffer_json_member_add_string(wb, "status", rrdcalc_status2string(rc->status));
                buffer_json_member_add_double(wb, "value", rc->value);
                buffer_json_object_close(wb);
            }
            buffer_json_object_close(wb);
        }
        netdata_rwlock_unlock(&st->alerts.rwlock);
    }
}

static void query_target_units(BUFFER *wb, QUERY_TARGET *qt) {
    if(qt->contexts.used == 1) {
        buffer_json_member_add_string(wb, "units", rrdcontext_acquired_units(qt->contexts.array[0].rca));
    }
    else if(qt->contexts.used > 1) {
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

void rrdr_json_wrapper_begin2(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format, RRDR_OPTIONS options, bool string_value,
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

    buffer_json_initialize(wb, kq, sq, 0, true);

    buffer_json_member_add_uint64(wb, "api", 2);
    buffer_json_member_add_string(wb, "id", qt->id);

    buffer_json_member_add_object(wb, "request");
    {
        buffer_json_member_add_string(wb, "format", rrdr_format_to_string(qt->request.format));
        web_client_api_request_v1_data_options_to_buffer_json_array(wb, "options", qt->request.options);

        buffer_json_member_add_object(wb, "selectors");
        if(qt->request.host)
            buffer_json_member_add_string(wb, "host", rrdhost_hostname(qt->request.host));
        else
            buffer_json_member_add_string(wb, "hosts", qt->request.hosts);
        buffer_json_member_add_string(wb, "contexts", qt->request.contexts);
        buffer_json_member_add_string(wb, "instances", qt->request.charts);
        buffer_json_member_add_string(wb, "dimensions", qt->request.dimensions);
        buffer_json_member_add_string(wb, "labels", qt->request.charts_labels_filter);
        buffer_json_object_close(wb); // selectors

        buffer_json_member_add_object(wb, "window");
        buffer_json_member_add_time_t(wb, "after", qt->request.after);
        buffer_json_member_add_time_t(wb, "before", qt->request.before);
        buffer_json_member_add_uint64(wb, "points", qt->request.points);
        if(qt->request.options & RRDR_OPTION_SELECTED_TIER)
            buffer_json_member_add_uint64(wb, "tier", qt->request.tier);
        else
            buffer_json_member_add_string(wb, "tier", NULL);
        buffer_json_object_close(wb); // window

        buffer_json_member_add_object(wb, "aggregations");
        {
            buffer_json_member_add_object(wb, "time");
            buffer_json_member_add_string(wb, "time_group", time_grouping_tostring(qt->request.time_group_method));
            buffer_json_member_add_string(wb, "time_group_options", qt->request.time_group_options);
            if(qt->request.resampling_time > 0)
                buffer_json_member_add_time_t(wb, "time_resampling", qt->request.resampling_time);
            else
                buffer_json_member_add_string(wb, "time_resampling", NULL);
            buffer_json_object_close(wb); // time

            buffer_json_member_add_object(wb, "metrics");
            buffer_json_member_add_string(wb, "group_by", group_by_to_string(qt->request.group_by));
            buffer_json_member_add_string(wb, "group_by_key", qt->request.group_by_key);
            buffer_json_member_add_string(wb, "group_by_aggregate",
                                          group_by_aggregate_function_to_string(qt->request.group_by_aggregate_function));
            buffer_json_object_close(wb); // dimensions
        }
        buffer_json_object_close(wb); // aggregations

        buffer_json_member_add_uint64(wb, "timeout", qt->request.timeout);
    }
    buffer_json_object_close(wb); // request

    buffer_json_member_add_object(wb, "metadata");
    {
        query_target_functions(wb, "functions", r);

        buffer_json_member_add_object(wb, "summary");
        query_target_hosts_instances_labels_dimensions(
                wb, r, "hosts", "dimensions", "instances", "labels", "alerts", true);
        buffer_json_object_close(wb); // aggregated

        buffer_json_member_add_object(wb, "detailed");
        buffer_json_member_add_object(wb, "hosts");

        time_t now_s = now_realtime_sec();
        RRDHOST *last_host = NULL;
        RRDCONTEXT_ACQUIRED *last_rca = NULL;
        RRDINSTANCE_ACQUIRED *last_ria = NULL;

        size_t h = 0, c = 0, i = 0, m = 0, q = 0;
        for(; h < qt->hosts.used ; h++) {
            QUERY_HOST *qh = query_host(qt, h);
            RRDHOST *host = qh->host;

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

                            queried = tqm->query.options & RRDR_DIMENSION_QUERIED;
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
                            buffer_json_member_add_uint64(wb, "idx", qh->slot);
                            buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(host));
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
                            buffer_json_member_add_uint64(wb, "idx", qi->slot);
                            buffer_json_member_add_string(wb, "name", rrdinstance_acquired_name(ria));
                            buffer_json_member_add_time_t(wb, "update_every", rrdinstance_acquired_update_every(ria));
                            DICTIONARY *labels = rrdinstance_acquired_labels(ria);
                            if(labels) {
                                buffer_json_member_add_object(wb, "labels");
                                rrdlabels_to_buffer_json_members(labels, wb);
                                buffer_json_object_close(wb);
                            }
                            rrdset_rrdcalc_entries(wb, ria);
                            buffer_json_member_add_object(wb, "dimensions");

                            last_ria = ria;
                        }

                        buffer_json_member_add_object(wb, rrdmetric_acquired_id(rma));
                        {
                            buffer_json_member_add_string(wb, "name", rrdmetric_acquired_name(rma));
                            buffer_json_member_add_boolean(wb, "queried", queried);
                            time_t first_entry_s = rrdmetric_acquired_first_entry(rma);
                            time_t last_entry_s = rrdmetric_acquired_last_entry(rma);
                            buffer_json_member_add_time_t(wb, "first_entry", first_entry_s);
                            buffer_json_member_add_time_t(wb, "last_entry", last_entry_s ? last_entry_s : now_s);

                            if(qm) {
                                query_target_value_stats(wb, qm->query.min, qm->query.max, qm->query.sum, qm->query.average, qm->query.volume);

                                if(qm->query.options & RRDR_DIMENSION_GROUPED) {
                                    // buffer_json_member_add_string(wb, "grouped_as_id", string2str(qm->grouped_as.id));
                                    buffer_json_member_add_string(wb, "grouped_as", string2str(qm->grouped_as.name));
                                }

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
        buffer_json_object_close(wb); // detailed
    }
    buffer_json_object_close(wb); // metadata

    buffer_json_member_add_object(wb, "db");
    {
        buffer_json_member_add_time_t(wb, "update_every", qt->db.minimum_latest_update_every_s);
        buffer_json_member_add_time_t(wb, "first_entry", qt->db.first_time_s);
        buffer_json_member_add_time_t(wb, "last_entry", qt->db.last_time_s);
        buffer_json_member_add_array(wb, "points_per_tier");
        for(size_t tier = 0; tier < storage_tiers ; tier++)
            buffer_json_add_array_item_uint64(wb, r->stats.tier_points_read[tier]);
        buffer_json_array_close(wb);

    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "view");
    {
        buffer_json_member_add_string(wb, "format", rrdr_format_to_string(format));
        web_client_api_request_v1_data_options_to_buffer_json_array(wb, "options", r->view.options);
        buffer_json_member_add_string(wb, "time_group", time_grouping_tostring(group_method));
        buffer_json_member_add_time_t(wb, "update_every", r->view.update_every);
        buffer_json_member_add_time_t(wb, "after", r->view.after);
        buffer_json_member_add_time_t(wb, "before", r->view.before);
        buffer_json_member_add_uint64(wb, "points", rows);
        query_target_units(wb, qt);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "dimensions");
    {
        rrdr_dimension_ids(wb, "ids", r, options);
        rrdr_dimension_names(wb, "names", r, options);
        size_t dims = rrdr_latest_values(wb, "view_latest_values", r, options);
        buffer_json_member_add_uint64(wb, "count", dims);
    }
    buffer_json_object_close(wb);

    buffer_sprintf(wb, ",\n    %sresult%s:", kq, kq);
    if(string_value) buffer_strcat(wb, sq);
}

void rrdr_json_wrapper_anomaly_rates(RRDR *r __maybe_unused, BUFFER *wb, DATASOURCE_FORMAT format __maybe_unused, RRDR_OPTIONS options, bool string_value) {

    char kq[2] = "",                    // key quote
        sq[2] = "";                     // string quote

    if( options & RRDR_OPTION_GOOGLE_JSON ) {
        kq[0] = '\0';
        sq[0] = '\'';
    }
    else {
        kq[0] = '"';
        sq[0] = '"';
    }

    if(string_value) buffer_strcat(wb, sq);

    buffer_sprintf(wb, ",\n    %sanomaly_rates%s: ", kq, kq);
}

void rrdr_json_wrapper_group_by_count(RRDR *r __maybe_unused, BUFFER *wb, DATASOURCE_FORMAT format __maybe_unused, RRDR_OPTIONS options, bool string_value) {

    char kq[2] = "",                    // key quote
    sq[2] = "";                     // string quote

    if( options & RRDR_OPTION_GOOGLE_JSON ) {
        kq[0] = '\0';
        sq[0] = '\'';
    }
    else {
        kq[0] = '"';
        sq[0] = '"';
    }

    if(string_value) buffer_strcat(wb, sq);

    buffer_sprintf(wb, ",\n    %sgroup_by_count%s: ", kq, kq);
}

void rrdr_json_wrapper_end(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format __maybe_unused, RRDR_OPTIONS options, bool string_value) {
    QUERY_TARGET *qt = r->internal.qt;

    char sq[2] = "";                     // string quote

    if( options & RRDR_OPTION_GOOGLE_JSON ) {
        sq[0] = '\'';
    }
    else {
        sq[0] = '"';
    }

    if(string_value) buffer_strcat(wb, sq);

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
