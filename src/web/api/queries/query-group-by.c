// SPDX-License-Identifier: GPL-3.0-or-later

#include "query-internal.h"

RRDR_GROUP_BY group_by_parse(const char *group_by_txt) {
    char src[strlen(group_by_txt) + 1];
    strcatz(src, 0, group_by_txt, sizeof(src));
    char *s = src;

    RRDR_GROUP_BY group_by = RRDR_GROUP_BY_NONE;

    while(s) {
        char *key = strsep_skip_consecutive_separators(&s, ",| ");
        if (!key || !*key) continue;

        if (strcmp(key, "selected") == 0)
            group_by |= RRDR_GROUP_BY_SELECTED;

        if (strcmp(key, "dimension") == 0)
            group_by |= RRDR_GROUP_BY_DIMENSION;

        if (strcmp(key, "instance") == 0)
            group_by |= RRDR_GROUP_BY_INSTANCE;

        if (strcmp(key, "percentage-of-instance") == 0)
            group_by |= RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE;

        if (strcmp(key, "label") == 0)
            group_by |= RRDR_GROUP_BY_LABEL;

        if (strcmp(key, "node") == 0)
            group_by |= RRDR_GROUP_BY_NODE;

        if (strcmp(key, "context") == 0)
            group_by |= RRDR_GROUP_BY_CONTEXT;

        if (strcmp(key, "units") == 0)
            group_by |= RRDR_GROUP_BY_UNITS;
    }

    if((group_by & RRDR_GROUP_BY_SELECTED) && (group_by & ~RRDR_GROUP_BY_SELECTED)) {
        internal_error(true, "group-by given by query has 'selected' together with more groupings");
        group_by = RRDR_GROUP_BY_SELECTED; // remove all other groupings
    }

    if(group_by & RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE)
        group_by = RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE; // remove all other groupings

    return group_by;
}

void buffer_json_group_by_to_array(BUFFER *wb, RRDR_GROUP_BY group_by) {
    if(group_by == RRDR_GROUP_BY_NONE)
        buffer_json_add_array_item_string(wb, "none");
    else {
        if (group_by & RRDR_GROUP_BY_DIMENSION)
            buffer_json_add_array_item_string(wb, "dimension");

        if (group_by & RRDR_GROUP_BY_INSTANCE)
            buffer_json_add_array_item_string(wb, "instance");

        if (group_by & RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE)
            buffer_json_add_array_item_string(wb, "percentage-of-instance");

        if (group_by & RRDR_GROUP_BY_LABEL)
            buffer_json_add_array_item_string(wb, "label");

        if (group_by & RRDR_GROUP_BY_NODE)
            buffer_json_add_array_item_string(wb, "node");

        if (group_by & RRDR_GROUP_BY_CONTEXT)
            buffer_json_add_array_item_string(wb, "context");

        if (group_by & RRDR_GROUP_BY_UNITS)
            buffer_json_add_array_item_string(wb, "units");

        if (group_by & RRDR_GROUP_BY_SELECTED)
            buffer_json_add_array_item_string(wb, "selected");
    }
}

RRDR_GROUP_BY_FUNCTION group_by_aggregate_function_parse(const char *s) {
    if(strcmp(s, "average") == 0)
        return RRDR_GROUP_BY_FUNCTION_AVERAGE;

    if(strcmp(s, "avg") == 0)
        return RRDR_GROUP_BY_FUNCTION_AVERAGE;

    if(strcmp(s, "min") == 0)
        return RRDR_GROUP_BY_FUNCTION_MIN;

    if(strcmp(s, "max") == 0)
        return RRDR_GROUP_BY_FUNCTION_MAX;

    if(strcmp(s, "sum") == 0)
        return RRDR_GROUP_BY_FUNCTION_SUM;

    if(strcmp(s, "percentage") == 0)
        return RRDR_GROUP_BY_FUNCTION_PERCENTAGE;

    if(strcmp(s, "extremes") == 0)
        return RRDR_GROUP_BY_FUNCTION_EXTREMES;

    return RRDR_GROUP_BY_FUNCTION_AVERAGE;
}

const char *group_by_aggregate_function_to_string(RRDR_GROUP_BY_FUNCTION group_by_function) {
    switch(group_by_function) {
        default:
        case RRDR_GROUP_BY_FUNCTION_AVERAGE:
            return "average";

        case RRDR_GROUP_BY_FUNCTION_MIN:
            return "min";

        case RRDR_GROUP_BY_FUNCTION_MAX:
            return "max";

        case RRDR_GROUP_BY_FUNCTION_SUM:
            return "sum";

        case RRDR_GROUP_BY_FUNCTION_PERCENTAGE:
            return "percentage";

        case RRDR_GROUP_BY_FUNCTION_EXTREMES:
            return "extremes";
    }
}

// ----------------------------------------------------------------------------
// group by

struct group_by_label_key {
    DICTIONARY *values;
};

static void group_by_label_key_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    // add the key to our r->label_keys global keys dictionary
    DICTIONARY *label_keys = data;
    dictionary_set(label_keys, dictionary_acquired_item_name(item), NULL, 0);

    // create a dictionary for the values of this key
    struct group_by_label_key *k = value;
    k->values = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, 0);
}

static void group_by_label_key_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct group_by_label_key *k = value;
    dictionary_destroy(k->values);
}

static int rrdlabels_traversal_cb_to_group_by_label_key(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    DICTIONARY *dl = data;
    struct group_by_label_key *k = dictionary_set(dl, name, NULL, sizeof(struct group_by_label_key));
    dictionary_set(k->values, value, NULL, 0);
    return 1;
}

void rrdr_json_group_by_labels(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    if(!r->label_keys || !r->dl)
        return;

    buffer_json_member_add_object(wb, key);

    void *t;
    dfe_start_read(r->label_keys, t) {
        buffer_json_member_add_array(wb, t_dfe.name);

        for(size_t d = 0; d < r->d ;d++) {
            if(!rrdr_dimension_should_be_exposed(r->od[d], options))
                continue;

            struct group_by_label_key *k = dictionary_get(r->dl[d], t_dfe.name);
            if(k) {
                buffer_json_add_array_item_array(wb);
                void *tt;
                dfe_start_read(k->values, tt) {
                    buffer_json_add_array_item_string(wb, tt_dfe.name);
                }
                dfe_done(tt);
                buffer_json_array_close(wb);
            }
            else
                buffer_json_add_array_item_string(wb, NULL);
        }

        buffer_json_array_close(wb);
    }
    dfe_done(t);

    buffer_json_object_close(wb); // key
}

static void rrd2rrdr_set_timestamps(RRDR *r) {
    QUERY_TARGET *qt = r->internal.qt;

    internal_fatal(qt->window.points != r->n, "QUERY: mismatch to the number of points in qt and r");

    r->view.group = qt->window.group;
    r->view.update_every = (int) query_view_update_every(qt);
    r->view.before = qt->window.before;
    r->view.after = qt->window.after;

    r->time_grouping.points_wanted = qt->window.points;
    r->time_grouping.resampling_group = qt->window.resampling_group;
    r->time_grouping.resampling_divisor = qt->window.resampling_divisor;

    r->rows = qt->window.points;

    size_t points_wanted = qt->window.points;
    time_t after_wanted = qt->window.after;
    time_t before_wanted = qt->window.before; (void)before_wanted;

    time_t view_update_every = r->view.update_every;
    time_t query_granularity = (time_t)(r->view.update_every / r->view.group);

    size_t rrdr_line = 0;
    time_t first_point_end_time = after_wanted + view_update_every - query_granularity;
    time_t now_end_time = first_point_end_time;

    while (rrdr_line < points_wanted) {
        r->t[rrdr_line++] = now_end_time;
        now_end_time += view_update_every;
    }

    internal_fatal(r->t[0] != first_point_end_time, "QUERY: wrong first timestamp in the query");
    internal_error(r->t[points_wanted - 1] != before_wanted,
                   "QUERY: wrong last timestamp in the query, expected %ld, found %ld",
                   before_wanted, r->t[points_wanted - 1]);
}

static void query_group_by_make_dimension_key(BUFFER *key, RRDR_GROUP_BY group_by, size_t group_by_id, QUERY_TARGET *qt, QUERY_NODE *qn, QUERY_CONTEXT *qc, QUERY_INSTANCE *qi, QUERY_DIMENSION *qd __maybe_unused, QUERY_METRIC *qm, bool query_has_percentage_of_group) {
    buffer_flush(key);
    if(unlikely(!query_has_percentage_of_group && qm->status & RRDR_DIMENSION_HIDDEN)) {
        buffer_strcat(key, "__hidden_dimensions__");
    }
    else if(unlikely(group_by & RRDR_GROUP_BY_SELECTED)) {
        buffer_strcat(key, "selected");
    }
    else {
        if (group_by & RRDR_GROUP_BY_DIMENSION) {
            buffer_fast_strcat(key, "|", 1);
            buffer_strcat(key, query_metric_name(qt, qm));
        }

        if (group_by & (RRDR_GROUP_BY_INSTANCE|RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE)) {
            buffer_fast_strcat(key, "|", 1);
            buffer_strcat(key, string2str(query_instance_id_fqdn(qi, qt->request.version)));
        }

        if (group_by & RRDR_GROUP_BY_LABEL) {
            RRDLABELS *labels = rrdinstance_acquired_labels(qi->ria);
            for (size_t l = 0; l < qt->group_by[group_by_id].used; l++) {
                buffer_fast_strcat(key, "|", 1);
                rrdlabels_get_value_to_buffer_or_unset(labels, key, qt->group_by[group_by_id].label_keys[l], "[unset]");
            }
        }

        if (group_by & RRDR_GROUP_BY_NODE) {
            buffer_fast_strcat(key, "|", 1);
            buffer_strcat(key, qn->rrdhost->machine_guid);
        }

        if (group_by & RRDR_GROUP_BY_CONTEXT) {
            buffer_fast_strcat(key, "|", 1);
            buffer_strcat(key, rrdcontext_acquired_id(qc->rca));
        }

        if (group_by & RRDR_GROUP_BY_UNITS) {
            buffer_fast_strcat(key, "|", 1);
            buffer_strcat(key, query_target_has_percentage_units(qt) ? "%" : rrdinstance_acquired_units(qi->ria));
        }
    }
}

static void query_group_by_make_dimension_id(BUFFER *key, RRDR_GROUP_BY group_by, size_t group_by_id, QUERY_TARGET *qt, QUERY_NODE *qn, QUERY_CONTEXT *qc, QUERY_INSTANCE *qi, QUERY_DIMENSION *qd __maybe_unused, QUERY_METRIC *qm, bool query_has_percentage_of_group) {
    buffer_flush(key);
    if(unlikely(!query_has_percentage_of_group && qm->status & RRDR_DIMENSION_HIDDEN)) {
        buffer_strcat(key, "__hidden_dimensions__");
    }
    else if(unlikely(group_by & RRDR_GROUP_BY_SELECTED)) {
        buffer_strcat(key, "selected");
    }
    else {
        if (group_by & RRDR_GROUP_BY_DIMENSION) {
            buffer_strcat(key, query_metric_name(qt, qm));
        }

        if (group_by & (RRDR_GROUP_BY_INSTANCE|RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE)) {
            if (buffer_strlen(key) != 0)
                buffer_fast_strcat(key, ",", 1);

            if (group_by & RRDR_GROUP_BY_NODE)
                buffer_strcat(key, rrdinstance_acquired_id(qi->ria));
            else
                buffer_strcat(key, string2str(query_instance_id_fqdn(qi, qt->request.version)));
        }

        if (group_by & RRDR_GROUP_BY_LABEL) {
            RRDLABELS *labels = rrdinstance_acquired_labels(qi->ria);
            for (size_t l = 0; l < qt->group_by[group_by_id].used; l++) {
                if (buffer_strlen(key) != 0)
                    buffer_fast_strcat(key, ",", 1);
                rrdlabels_get_value_to_buffer_or_unset(labels, key, qt->group_by[group_by_id].label_keys[l], "[unset]");
            }
        }

        if (group_by & RRDR_GROUP_BY_NODE) {
            if (buffer_strlen(key) != 0)
                buffer_fast_strcat(key, ",", 1);

            buffer_strcat(key, qn->rrdhost->machine_guid);
        }

        if (group_by & RRDR_GROUP_BY_CONTEXT) {
            if (buffer_strlen(key) != 0)
                buffer_fast_strcat(key, ",", 1);

            buffer_strcat(key, rrdcontext_acquired_id(qc->rca));
        }

        if (group_by & RRDR_GROUP_BY_UNITS) {
            if (buffer_strlen(key) != 0)
                buffer_fast_strcat(key, ",", 1);

            buffer_strcat(key, query_target_has_percentage_units(qt) ? "%" : rrdinstance_acquired_units(qi->ria));
        }
    }
}

static void query_group_by_make_dimension_name(BUFFER *key, RRDR_GROUP_BY group_by, size_t group_by_id, QUERY_TARGET *qt, QUERY_NODE *qn, QUERY_CONTEXT *qc, QUERY_INSTANCE *qi, QUERY_DIMENSION *qd __maybe_unused, QUERY_METRIC *qm, bool query_has_percentage_of_group) {
    buffer_flush(key);
    if(unlikely(!query_has_percentage_of_group && qm->status & RRDR_DIMENSION_HIDDEN)) {
        buffer_strcat(key, "__hidden_dimensions__");
    }
    else if(unlikely(group_by & RRDR_GROUP_BY_SELECTED)) {
        buffer_strcat(key, "selected");
    }
    else {
        if (group_by & RRDR_GROUP_BY_DIMENSION) {
            buffer_strcat(key, query_metric_name(qt, qm));
        }

        if (group_by & (RRDR_GROUP_BY_INSTANCE|RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE)) {
            if (buffer_strlen(key) != 0)
                buffer_fast_strcat(key, ",", 1);

            if (group_by & RRDR_GROUP_BY_NODE)
                buffer_strcat(key, rrdinstance_acquired_name(qi->ria));
            else
                buffer_strcat(key, string2str(query_instance_name_fqdn(qi, qt->request.version)));
        }

        if (group_by & RRDR_GROUP_BY_LABEL) {
            RRDLABELS *labels = rrdinstance_acquired_labels(qi->ria);
            for (size_t l = 0; l < qt->group_by[group_by_id].used; l++) {
                if (buffer_strlen(key) != 0)
                    buffer_fast_strcat(key, ",", 1);
                rrdlabels_get_value_to_buffer_or_unset(labels, key, qt->group_by[group_by_id].label_keys[l], "[unset]");
            }
        }

        if (group_by & RRDR_GROUP_BY_NODE) {
            if (buffer_strlen(key) != 0)
                buffer_fast_strcat(key, ",", 1);

            buffer_strcat(key, rrdhost_hostname(qn->rrdhost));
        }

        if (group_by & RRDR_GROUP_BY_CONTEXT) {
            if (buffer_strlen(key) != 0)
                buffer_fast_strcat(key, ",", 1);

            buffer_strcat(key, rrdcontext_acquired_id(qc->rca));
        }

        if (group_by & RRDR_GROUP_BY_UNITS) {
            if (buffer_strlen(key) != 0)
                buffer_fast_strcat(key, ",", 1);

            buffer_strcat(key, query_target_has_percentage_units(qt) ? "%" : rrdinstance_acquired_units(qi->ria));
        }
    }
}

struct rrdr_group_by_entry {
    size_t priority;
    size_t count;
    STRING *id;
    STRING *name;
    STRING *units;
    RRDR_DIMENSION_FLAGS od;
    DICTIONARY *dl;
};

RRDR *rrd2rrdr_group_by_initialize(ONEWAYALLOC *owa, QUERY_TARGET *qt) {
    RRDR *r_tmp = NULL;
    RRDR_OPTIONS options = qt->window.options;

    if(qt->request.version < 2) {
        // v1 query
        RRDR *r = rrdr_create(owa, qt, qt->query.used, qt->window.points);
        if(unlikely(!r)) {
            internal_error(true, "QUERY: cannot create RRDR for %s, after=%ld, before=%ld, dimensions=%u, points=%zu",
                           qt->id, qt->window.after, qt->window.before, qt->query.used, qt->window.points);
            return NULL;
        }
        r->group_by.r = NULL;

        for(size_t d = 0; d < qt->query.used ; d++) {
            QUERY_METRIC *qm = query_metric(qt, d);
            QUERY_DIMENSION *qd = query_dimension(qt, qm->link.query_dimension_id);
            r->di[d] = rrdmetric_acquired_id_dup(qd->rma);
            r->dn[d] = rrdmetric_acquired_name_dup(qd->rma);
        }

        rrd2rrdr_set_timestamps(r);
        return r;
    }
    // v2 query

    // parse all the group-by label keys
    for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++) {
        if (qt->request.group_by[g].group_by & RRDR_GROUP_BY_LABEL &&
            qt->request.group_by[g].group_by_label && *qt->request.group_by[g].group_by_label)
            qt->group_by[g].used = quoted_strings_splitter_query_group_by_label(
                qt->request.group_by[g].group_by_label, qt->group_by[g].label_keys,
                GROUP_BY_MAX_LABEL_KEYS);

        if (!qt->group_by[g].used)
            qt->request.group_by[g].group_by &= ~RRDR_GROUP_BY_LABEL;
    }

    // make sure there are valid group-by methods
    for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++) {
        if(!(qt->request.group_by[g].group_by & SUPPORTED_GROUP_BY_METHODS))
            qt->request.group_by[g].group_by = (g == 0) ? RRDR_GROUP_BY_DIMENSION : RRDR_GROUP_BY_NONE;
    }

    bool query_has_percentage_of_group = query_target_has_percentage_of_group(qt);

    // merge all group-by options to upper levels,
    // so that the top level has all the groupings of the inner levels,
    // and each subsequent level has all the groupings of its inner levels.
    for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES - 1 ;g++) {
        if(qt->request.group_by[g].group_by == RRDR_GROUP_BY_NONE)
            continue;

        if(qt->request.group_by[g].group_by == RRDR_GROUP_BY_SELECTED) {
            for (size_t r = g + 1; r < MAX_QUERY_GROUP_BY_PASSES; r++)
                qt->request.group_by[r].group_by = RRDR_GROUP_BY_NONE;
        }
        else {
            for (size_t r = g + 1; r < MAX_QUERY_GROUP_BY_PASSES; r++) {
                if (qt->request.group_by[r].group_by == RRDR_GROUP_BY_NONE)
                    continue;

                if (qt->request.group_by[r].group_by != RRDR_GROUP_BY_SELECTED) {
                    if(qt->request.group_by[r].group_by & RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE)
                        qt->request.group_by[g].group_by |= RRDR_GROUP_BY_INSTANCE;
                    else
                        qt->request.group_by[g].group_by |= qt->request.group_by[r].group_by;

                    if(qt->request.group_by[r].group_by & RRDR_GROUP_BY_LABEL) {
                        for (size_t lr = 0; lr < qt->group_by[r].used; lr++) {
                            bool found = false;
                            for (size_t lg = 0; lg < qt->group_by[g].used; lg++) {
                                if (strcmp(qt->group_by[g].label_keys[lg], qt->group_by[r].label_keys[lr]) == 0) {
                                    found = true;
                                    break;
                                }
                            }

                            if (!found && qt->group_by[g].used < GROUP_BY_MAX_LABEL_KEYS * MAX_QUERY_GROUP_BY_PASSES)
                                qt->group_by[g].label_keys[qt->group_by[g].used++] = qt->group_by[r].label_keys[lr];
                        }
                    }
                }
            }
        }
    }

    int added = 0;
    RRDR *first_r = NULL, *last_r = NULL;
    BUFFER *key = buffer_create(0, NULL);
    struct rrdr_group_by_entry *entries = onewayalloc_mallocz(owa, qt->query.used * sizeof(struct rrdr_group_by_entry));
    DICTIONARY *groups = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
    DICTIONARY *label_keys = NULL;

    for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++) {
        RRDR_GROUP_BY group_by = qt->request.group_by[g].group_by;
        RRDR_GROUP_BY_FUNCTION aggregation_method = qt->request.group_by[g].aggregation;

        if(group_by == RRDR_GROUP_BY_NONE)
            break;

        memset(entries, 0, qt->query.used * sizeof(struct rrdr_group_by_entry));
        dictionary_flush(groups);
        added = 0;

        size_t hidden_dimensions = 0;
        bool final_grouping = (g == MAX_QUERY_GROUP_BY_PASSES - 1 || qt->request.group_by[g + 1].group_by == RRDR_GROUP_BY_NONE) ? true : false;

        if (final_grouping && (options & RRDR_OPTION_GROUP_BY_LABELS))
            label_keys = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, 0);

        QUERY_INSTANCE *last_qi = NULL;
        size_t priority = 0;
        time_t update_every_max = 0;
        for (size_t d = 0; d < qt->query.used; d++) {
            QUERY_METRIC *qm = query_metric(qt, d);
            QUERY_DIMENSION *qd = query_dimension(qt, qm->link.query_dimension_id);
            QUERY_INSTANCE *qi = query_instance(qt, qm->link.query_instance_id);
            QUERY_CONTEXT *qc = query_context(qt, qm->link.query_context_id);
            QUERY_NODE *qn = query_node(qt, qm->link.query_node_id);

            if (qi != last_qi) {
                last_qi = qi;

                time_t update_every = rrdinstance_acquired_update_every(qi->ria);
                if (update_every > update_every_max)
                    update_every_max = update_every;
            }

            priority = qd->priority;

            if(qm->status & RRDR_DIMENSION_HIDDEN)
                hidden_dimensions++;

            // --------------------------------------------------------------------
            // generate the group by key

            query_group_by_make_dimension_key(key, group_by, g, qt, qn, qc, qi, qd, qm, query_has_percentage_of_group);

            // lookup the key in the dictionary

            int pos = -1;
            int *set = dictionary_set(groups, buffer_tostring(key), &pos, sizeof(pos));
            if (*set == -1) {
                // the key just added to the dictionary

                *set = pos = added++;

                // ----------------------------------------------------------------
                // generate the dimension id

                query_group_by_make_dimension_id(key, group_by, g, qt, qn, qc, qi, qd, qm, query_has_percentage_of_group);
                entries[pos].id = string_strdupz(buffer_tostring(key));

                // ----------------------------------------------------------------
                // generate the dimension name

                query_group_by_make_dimension_name(key, group_by, g, qt, qn, qc, qi, qd, qm, query_has_percentage_of_group);
                entries[pos].name = string_strdupz(buffer_tostring(key));

                // add the rest of the info
                entries[pos].units = rrdinstance_acquired_units_dup(qi->ria);
                entries[pos].priority = priority;

                if (label_keys) {
                    entries[pos].dl = dictionary_create_advanced(
                        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_FIXED_SIZE | DICT_OPTION_DONT_OVERWRITE_VALUE,
                        NULL, sizeof(struct group_by_label_key));
                    dictionary_register_insert_callback(entries[pos].dl, group_by_label_key_insert_cb, label_keys);
                    dictionary_register_delete_callback(entries[pos].dl, group_by_label_key_delete_cb, label_keys);
                }
            } else {
                // the key found in the dictionary
                pos = *set;
            }

            entries[pos].count++;

            if (unlikely(priority < entries[pos].priority))
                entries[pos].priority = priority;

            if(g > 0)
                last_r->dgbs[qm->grouped_as.slot] = pos;
            else
                qm->grouped_as.first_slot = pos;

            qm->grouped_as.slot = pos;
            qm->grouped_as.id = entries[pos].id;
            qm->grouped_as.name = entries[pos].name;
            qm->grouped_as.units = entries[pos].units;

            // copy the dimension flags decided by the query target
            // we need this, because if a dimension is explicitly selected
            // the query target adds to it the non-zero flag
            qm->status |= RRDR_DIMENSION_GROUPED;

            if(query_has_percentage_of_group)
                // when the query has percentage of group
                // there will be no hidden dimensions in the final query,
                // so we have to remove the hidden flag from all dimensions
                entries[pos].od |= qm->status & ~RRDR_DIMENSION_HIDDEN;
            else
                entries[pos].od |= qm->status;

            if (entries[pos].dl)
                rrdlabels_walkthrough_read(rrdinstance_acquired_labels(qi->ria),
                                           rrdlabels_traversal_cb_to_group_by_label_key, entries[pos].dl);
        }

        RRDR *r = rrdr_create(owa, qt, added, qt->window.points);
        if (!r) {
            internal_error(true,
                           "QUERY: cannot create group by RRDR for %s, after=%ld, before=%ld, dimensions=%d, points=%zu",
                           qt->id, qt->window.after, qt->window.before, added, qt->window.points);
            goto cleanup;
        }
        // prevent double free at cleanup in case of error
        added = 0;

        // link this RRDR
        if(!last_r)
            first_r = last_r = r;
        else
            last_r->group_by.r = r;

        last_r = r;

        rrd2rrdr_set_timestamps(r);

        if(r->d) {
            r->dp = onewayalloc_callocz(owa, r->d, sizeof(*r->dp));
            r->dview = onewayalloc_callocz(owa, r->d, sizeof(*r->dview));
            r->dgbc = onewayalloc_callocz(owa, r->d, sizeof(*r->dgbc));
            r->dqp = onewayalloc_callocz(owa, r->d, sizeof(STORAGE_POINT));

            if(!final_grouping)
                // this is where we are going to store the slot in the next RRDR
                // that we are going to group by the dimension of this RRDR
                r->dgbs = onewayalloc_callocz(owa, r->d, sizeof(*r->dgbs));

            if (label_keys) {
                r->dl = onewayalloc_callocz(owa, r->d, sizeof(DICTIONARY *));
                r->label_keys = label_keys;
                label_keys = NULL;
            }

            if(r->n) {
                r->gbc = onewayalloc_callocz(owa, r->n * r->d, sizeof(*r->gbc));

                if(hidden_dimensions && ((group_by & RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE) || (aggregation_method == RRDR_GROUP_BY_FUNCTION_PERCENTAGE)))
                    // this is where we are going to group the hidden dimensions
                    r->vh = onewayalloc_mallocz(owa, r->n * r->d * sizeof(*r->vh));
            }
        }

        // zero r (dimension options, names, and ids)
        // this is required, because group-by may lead to empty dimensions
        for (size_t d = 0; d < r->d; d++) {
            r->di[d] = entries[d].id;
            r->dn[d] = entries[d].name;

            r->od[d] = entries[d].od;
            r->du[d] = entries[d].units;
            r->dp[d] = entries[d].priority;
            r->dgbc[d] = entries[d].count;

            if (r->dl)
                r->dl[d] = entries[d].dl;
        }

        // initialize partial trimming
        r->partial_data_trimming.max_update_every = update_every_max * 2;
        r->partial_data_trimming.expected_after =
            (!query_target_aggregatable(qt) &&
             qt->window.before >= qt->window.now - r->partial_data_trimming.max_update_every) ?
                qt->window.before - r->partial_data_trimming.max_update_every :
                qt->window.before;
        r->partial_data_trimming.trimmed_after = qt->window.before;

        // make all values empty
        if(r->n && r->d) {
            for (size_t i = 0; i != r->n; i++) {
                NETDATA_DOUBLE *cn = &r->v[i * r->d];
                RRDR_VALUE_FLAGS *co = &r->o[i * r->d];
                NETDATA_DOUBLE *ar = &r->ar[i * r->d];
                NETDATA_DOUBLE *vh = r->vh ? &r->vh[i * r->d] : NULL;

                for (size_t d = 0; d < r->d; d++) {
                    cn[d] = NAN;
                    ar[d] = 0.0;
                    co[d] = RRDR_VALUE_EMPTY;

                    if (vh)
                        vh[d] = NAN;
                }
            }
        }
    }

    if(!first_r || !last_r)
        goto cleanup;

    r_tmp = rrdr_create(owa, qt, 1, qt->window.points);
    if (!r_tmp) {
        internal_error(true,
                       "QUERY: cannot create group by temporary RRDR for %s, after=%ld, before=%ld, dimensions=%d, points=%zu",
                       qt->id, qt->window.after, qt->window.before, 1, qt->window.points);
        goto cleanup;
    }
    rrd2rrdr_set_timestamps(r_tmp);
    r_tmp->group_by.r = first_r;

cleanup:
    if(!first_r || !last_r || !r_tmp) {
        if(r_tmp) {
            r_tmp->group_by.r = NULL;
            rrdr_free(owa, r_tmp);
        }

        if(first_r) {
            RRDR *r = first_r;
            while (r) {
                r_tmp = r->group_by.r;
                r->group_by.r = NULL;
                rrdr_free(owa, r);
                r = r_tmp;
            }
        }

        if(entries && added) {
            for (int d = 0; d < added; d++) {
                string_freez(entries[d].id);
                string_freez(entries[d].name);
                string_freez(entries[d].units);
                dictionary_destroy(entries[d].dl);
            }
        }
        dictionary_destroy(label_keys);

        first_r = last_r = r_tmp = NULL;
    }

    buffer_free(key);
    onewayalloc_freez(owa, entries);
    dictionary_destroy(groups);

    return r_tmp;
}

void rrd2rrdr_group_by_add_metric(RRDR *r_dst, size_t d_dst, RRDR *r_tmp, size_t d_tmp,
                                         RRDR_GROUP_BY_FUNCTION group_by_aggregate_function,
                                         STORAGE_POINT *query_points, size_t pass __maybe_unused) {
    if(!r_tmp || r_dst == r_tmp || !(r_tmp->od[d_tmp] & RRDR_DIMENSION_QUERIED))
        return;

    internal_fatal(r_dst->n != r_tmp->n, "QUERY: group-by source and destination do not have the same number of rows");
    internal_fatal(d_dst >= r_dst->d, "QUERY: group-by destination dimension number exceeds destination RRDR size");
    internal_fatal(d_tmp >= r_tmp->d, "QUERY: group-by source dimension number exceeds source RRDR size");
    internal_fatal(!r_dst->dqp, "QUERY: group-by destination is not properly prepared (missing dqp array)");
    internal_fatal(!r_dst->gbc, "QUERY: group-by destination is not properly prepared (missing gbc array)");

    bool hidden_dimension_on_percentage_of_group = (r_tmp->od[d_tmp] & RRDR_DIMENSION_HIDDEN) && r_dst->vh;

    if(!hidden_dimension_on_percentage_of_group) {
        r_dst->od[d_dst] |= r_tmp->od[d_tmp];
        storage_point_merge_to(r_dst->dqp[d_dst], *query_points);
    }

    // do the group_by
    for(size_t i = 0; i != rrdr_rows(r_tmp) ; i++) {

        size_t idx_tmp = i * r_tmp->d + d_tmp;
        NETDATA_DOUBLE n_tmp = r_tmp->v[ idx_tmp ];
        RRDR_VALUE_FLAGS o_tmp = r_tmp->o[ idx_tmp ];
        NETDATA_DOUBLE ar_tmp = r_tmp->ar[ idx_tmp ];

        if(o_tmp & RRDR_VALUE_EMPTY)
            continue;

        size_t idx_dst = i * r_dst->d + d_dst;
        NETDATA_DOUBLE *cn = (hidden_dimension_on_percentage_of_group) ? &r_dst->vh[ idx_dst ] : &r_dst->v[ idx_dst ];
        RRDR_VALUE_FLAGS *co = &r_dst->o[ idx_dst ];
        NETDATA_DOUBLE *ar = &r_dst->ar[ idx_dst ];
        uint32_t *gbc = &r_dst->gbc[ idx_dst ];

        switch(group_by_aggregate_function) {
            default:
            case RRDR_GROUP_BY_FUNCTION_AVERAGE:
            case RRDR_GROUP_BY_FUNCTION_SUM:
            case RRDR_GROUP_BY_FUNCTION_PERCENTAGE:
                if(isnan(*cn))
                    *cn = n_tmp;
                else
                    *cn += n_tmp;
                break;

            case RRDR_GROUP_BY_FUNCTION_MIN:
                if(isnan(*cn) || n_tmp < *cn)
                    *cn = n_tmp;
                break;

            case RRDR_GROUP_BY_FUNCTION_MAX:
                if(isnan(*cn) || n_tmp > *cn)
                    *cn = n_tmp;
                break;

            case RRDR_GROUP_BY_FUNCTION_EXTREMES:
                // For extremes, we need to keep track of the value with the maximum absolute value
                if(isnan(*cn) || fabsndd(n_tmp) > fabsndd(*cn))
                    *cn = n_tmp;
                break;
        }

        if(!hidden_dimension_on_percentage_of_group) {
            *co &= ~RRDR_VALUE_EMPTY;
            *co |= (o_tmp & (RRDR_VALUE_RESET | RRDR_VALUE_PARTIAL));
            *ar += ar_tmp;
            (*gbc)++;
        }
    }
}

void rrdr2rrdr_group_by_partial_trimming(RRDR *r) {
    time_t trimmable_after = r->partial_data_trimming.expected_after;

    // find the point just before the trimmable ones
    ssize_t i = (ssize_t)r->n - 1;
    for( ; i >= 0 ;i--) {
        if (r->t[i] < trimmable_after)
            break;
    }

    if(unlikely(i < 0))
        return;

    // internal_error(true, "Found trimmable index %zd (from 0 to %zu)", i, r->n - 1);

    size_t last_row_gbc = 0;
    for (; i < (ssize_t)r->n; i++) {
        size_t row_gbc = 0;
        for (size_t d = 0; d < r->d; d++) {
            if (unlikely(!(r->od[d] & RRDR_DIMENSION_QUERIED)))
                continue;

            row_gbc += r->gbc[ i * r->d + d ];
        }

        // internal_error(true, "GBC of index %zd is %zu", i, row_gbc);

        if (unlikely(r->t[i] >= trimmable_after && (row_gbc < last_row_gbc || !row_gbc))) {
            // discard the rest of the points
            // internal_error(true, "Discarding points %zd to %zu", i, r->n - 1);
            r->partial_data_trimming.trimmed_after = r->t[i];
            r->rows = i;
            break;
        }
        else
            last_row_gbc = row_gbc;
    }
}

void rrdr2rrdr_group_by_calculate_percentage_of_group(RRDR *r) {
    if(!r->vh)
        return;

    if(query_target_aggregatable(r->internal.qt) && query_has_group_by_aggregation_percentage(r->internal.qt))
        return;

    for(size_t i = 0; i < r->n ;i++) {
        NETDATA_DOUBLE *cn = &r->v[ i * r->d ];
        NETDATA_DOUBLE *ch = &r->vh[ i * r->d ];

        for(size_t d = 0; d < r->d ;d++) {
            NETDATA_DOUBLE n = cn[d];
            NETDATA_DOUBLE h = ch[d];

            if(isnan(n))
                cn[d] = 0.0;

            else if(isnan(h))
                cn[d] = 100.0;

            else
                cn[d] = n * 100.0 / (n + h);
        }
    }
}


void rrd2rrdr_convert_values_to_percentage_of_total(RRDR *r) {
    if(!(r->internal.qt->window.options & RRDR_OPTION_PERCENTAGE) || query_target_aggregatable(r->internal.qt))
        return;

    size_t global_min_max_values = 0;
    NETDATA_DOUBLE global_min = NAN, global_max = NAN;

    for(size_t i = 0; i != r->n ;i++) {
        NETDATA_DOUBLE *cn = &r->v[ i * r->d ];
        RRDR_VALUE_FLAGS *co = &r->o[ i * r->d ];

        NETDATA_DOUBLE total = 0;
        for (size_t d = 0; d < r->d; d++) {
            if (unlikely(!(r->od[d] & RRDR_DIMENSION_QUERIED)))
                continue;

            if(co[d] & RRDR_VALUE_EMPTY)
                continue;

            total += cn[d];
        }

        if(total == 0.0)
            total = 1.0;

        for (size_t d = 0; d < r->d; d++) {
            if (unlikely(!(r->od[d] & RRDR_DIMENSION_QUERIED)))
                continue;

            if(co[d] & RRDR_VALUE_EMPTY)
                continue;

            NETDATA_DOUBLE n = cn[d];
            n = cn[d] = n * 100.0 / total;

            if(unlikely(!global_min_max_values++))
                global_min = global_max = n;
            else {
                if(n < global_min)
                    global_min = n;
                if(n > global_max)
                    global_max = n;
            }
        }
    }

    r->view.min = global_min;
    r->view.max = global_max;

    if(!r->dview)
        // v1 query
        return;

    // v2 query

    for (size_t d = 0; d < r->d; d++) {
        if (unlikely(!(r->od[d] & RRDR_DIMENSION_QUERIED)))
            continue;

        size_t count = 0;
        NETDATA_DOUBLE min = 0.0, max = 0.0, sum = 0.0, ars = 0.0;
        for(size_t i = 0; i != r->rows ;i++) { // we use r->rows to respect trimming
            size_t idx = i * r->d + d;

            RRDR_VALUE_FLAGS o = r->o[ idx ];

            if (o & RRDR_VALUE_EMPTY)
                continue;

            NETDATA_DOUBLE ar = r->ar[ idx ];
            ars += ar;

            NETDATA_DOUBLE n = r->v[ idx ];
            sum += n;

            if(!count++)
                min = max = n;
            else {
                if(n < min)
                    min = n;
                if(n > max)
                    max = n;
            }
        }

        r->dview[d] = (STORAGE_POINT) {
            .sum = sum,
            .count = count,
            .min = min,
            .max = max,
            .anomaly_count = (size_t)(ars * (NETDATA_DOUBLE)count),
        };
    }
}

RRDR *rrd2rrdr_group_by_finalize(RRDR *r_tmp) {
    QUERY_TARGET *qt = r_tmp->internal.qt;

    if(!r_tmp->group_by.r) {
        // v1 query
        rrd2rrdr_convert_values_to_percentage_of_total(r_tmp);
        return r_tmp;
    }
    // v2 query

    // do the additional passes on RRDRs
    RRDR *last_r = r_tmp->group_by.r;
    rrdr2rrdr_group_by_calculate_percentage_of_group(last_r);

    RRDR *r = last_r->group_by.r;
    size_t pass = 0;
    while(r) {
        pass++;
        for(size_t d = 0; d < last_r->d ;d++) {
            rrd2rrdr_group_by_add_metric(r, last_r->dgbs[d], last_r, d,
                                         qt->request.group_by[pass].aggregation,
                                         &last_r->dqp[d], pass);
        }
        rrdr2rrdr_group_by_calculate_percentage_of_group(r);

        last_r = r;
        r = last_r->group_by.r;
    }

    // free all RRDRs except the last one
    r = r_tmp;
    while(r != last_r) {
        r_tmp = r->group_by.r;
        r->group_by.r = NULL;
        rrdr_free(r->internal.owa, r);
        r = r_tmp;
    }
    r = last_r;

    // find the final aggregation
    RRDR_GROUP_BY_FUNCTION aggregation = qt->request.group_by[0].aggregation;
    for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++)
        if(qt->request.group_by[g].group_by != RRDR_GROUP_BY_NONE)
            aggregation = qt->request.group_by[g].aggregation;

    if(!query_target_aggregatable(qt) && r->partial_data_trimming.expected_after < qt->window.before)
        rrdr2rrdr_group_by_partial_trimming(r);

    // apply averaging, remove RRDR_VALUE_EMPTY, find the non-zero dimensions, min and max
    size_t global_min_max_values = 0;
    size_t dimensions_nonzero = 0;
    NETDATA_DOUBLE global_min = NAN, global_max = NAN;
    for (size_t d = 0; d < r->d; d++) {
        if (unlikely(!(r->od[d] & RRDR_DIMENSION_QUERIED)))
            continue;

        size_t points_nonzero = 0;
        NETDATA_DOUBLE min = 0, max = 0, sum = 0, ars = 0;
        size_t count = 0;

        for(size_t i = 0; i != r->n ;i++) {
            size_t idx = i * r->d + d;

            NETDATA_DOUBLE *cn = &r->v[ idx ];
            RRDR_VALUE_FLAGS *co = &r->o[ idx ];
            NETDATA_DOUBLE *ar = &r->ar[ idx ];
            uint32_t gbc = r->gbc[ idx ];

            if(likely(gbc)) {
                *co &= ~RRDR_VALUE_EMPTY;

                if(gbc != r->dgbc[d])
                    *co |= RRDR_VALUE_PARTIAL;

                NETDATA_DOUBLE n;

                sum += *cn;
                ars += *ar;

                if(aggregation == RRDR_GROUP_BY_FUNCTION_AVERAGE && !query_target_aggregatable(qt))
                    n = (*cn /= gbc);
                else
                    n = *cn;

                if(!query_target_aggregatable(qt))
                    *ar /= gbc;

                if(islessgreater(n, 0.0))
                    points_nonzero++;

                if(unlikely(!count))
                    min = max = n;
                else {
                    if(n < min)
                        min = n;

                    if(n > max)
                        max = n;
                }

                if(unlikely(!global_min_max_values++))
                    global_min = global_max = n;
                else {
                    if(n < global_min)
                        global_min = n;

                    if(n > global_max)
                        global_max = n;
                }

                count += gbc;
            }
        }

        if(points_nonzero) {
            r->od[d] |= RRDR_DIMENSION_NONZERO;
            dimensions_nonzero++;
        }

        r->dview[d] = (STORAGE_POINT) {
            .sum = sum,
            .count = count,
            .min = min,
            .max = max,
            .anomaly_count = (size_t)(ars * RRDR_DVIEW_ANOMALY_COUNT_MULTIPLIER / 100.0),
        };
    }

    r->view.min = global_min;
    r->view.max = global_max;

    if(!dimensions_nonzero && (qt->window.options & RRDR_OPTION_NONZERO)) {
        // all dimensions are zero
        // remove the nonzero option
        qt->window.options &= ~RRDR_OPTION_NONZERO;
    }

    rrd2rrdr_convert_values_to_percentage_of_total(r);

    // update query instance counts in query host and query context
    {
        size_t h = 0, c = 0, i = 0;
        for(; h < qt->nodes.used ; h++) {
            QUERY_NODE *qn = &qt->nodes.array[h];

            for(; c < qt->contexts.used ;c++) {
                QUERY_CONTEXT *qc = &qt->contexts.array[c];

                if(!rrdcontext_acquired_belongs_to_host(qc->rca, qn->rrdhost))
                    break;

                for(; i < qt->instances.used ;i++) {
                    QUERY_INSTANCE *qi = &qt->instances.array[i];

                    if(!rrdinstance_acquired_belongs_to_context(qi->ria, qc->rca))
                        break;

                    if(qi->metrics.queried) {
                        qc->instances.queried++;
                        qn->instances.queried++;
                    }
                    else if(qi->metrics.failed) {
                        qc->instances.failed++;
                        qn->instances.failed++;
                    }
                }
            }
        }
    }

    return r;
}

static int compare_contributions(const void *a, const void *b) {
    const struct { size_t dim_idx; NETDATA_DOUBLE contribution; } *da = a;
    const struct { size_t dim_idx; NETDATA_DOUBLE contribution; } *db = b;

    if (da->contribution > db->contribution) return -1;
    if (da->contribution < db->contribution) return 1;
    return 0;
}

RRDR *rrd2rrdr_cardinality_limit(RRDR *r) {
    QUERY_TARGET *qt = r->internal.qt;
    
    if(!qt || qt->request.cardinality_limit == 0 || r->d <= qt->request.cardinality_limit)
        return r;
        
    ONEWAYALLOC *owa = r->internal.owa;
    
    // Calculate contribution of each dimension using dview statistics (sum of values)
    NETDATA_DOUBLE *contributions = onewayalloc_mallocz(owa, r->d * sizeof(NETDATA_DOUBLE));
    
    // Count queried dimensions and get their contributions from dview
    size_t queried_count = 0;
    for (size_t d = 0; d < r->d; d++) {
        contributions[d] = 0.0;
        
        if (!(r->od[d] & RRDR_DIMENSION_QUERIED))
            continue;
            
        queried_count++;
        
        // Use the sum from dview if available, otherwise fall back to manual calculation
        if(r->dview && !isnan(r->dview[d].sum)) {
            contributions[d] = fabsndd(r->dview[d].sum);
        } else {
            // Fallback: calculate manually from values
            for(size_t i = 0; i < r->rows; i++) {
                size_t idx = i * r->d + d;
                
                if(r->o[idx] & RRDR_VALUE_EMPTY)
                    continue;
                    
                NETDATA_DOUBLE value = r->v[idx];
                if(!isnan(value))
                    contributions[d] += fabsndd(value);
            }
        }
    }
    
    // If we don't need to reduce, return original
    if(queried_count <= qt->request.cardinality_limit) {
        onewayalloc_freez(owa, contributions);
        return r;
    }
    
    // Create array of dimension indices sorted by contribution (descending)
    struct {
        size_t dim_idx;
        NETDATA_DOUBLE contribution;
    } *sorted_dims = onewayalloc_mallocz(owa, queried_count * sizeof(*sorted_dims));
    
    size_t sorted_idx = 0;
    for (size_t d = 0; d < r->d; d++) {
        if (r->od[d] & RRDR_DIMENSION_QUERIED) {
            sorted_dims[sorted_idx].dim_idx = d;
            sorted_dims[sorted_idx].contribution = contributions[d];
            sorted_idx++;
        }
    }
    
    // Sort by contribution (descending)
    qsort(sorted_dims, queried_count, sizeof(*sorted_dims), compare_contributions);
    
    // Create new RRDR with limited dimensions
    size_t new_d = qt->request.cardinality_limit;
    size_t remaining_count = queried_count - (qt->request.cardinality_limit - 1);
    if(remaining_count > 0)
        new_d = qt->request.cardinality_limit; // Keep one slot for "remaining N dimensions"
    else
        new_d = queried_count; // No remaining dimensions needed
        
    RRDR *new_r = rrdr_create(owa, qt, new_d, r->n);
    if (!new_r) {
        internal_error(true, "QUERY: cannot create cardinality limited RRDR");
        onewayalloc_freez(owa, contributions);
        onewayalloc_freez(owa, sorted_dims);
        return r;
    }
    
    // Copy basic metadata from original RRDR
    new_r->view = r->view;
    new_r->time_grouping = r->time_grouping;
    new_r->partial_data_trimming = r->partial_data_trimming;
    new_r->rows = r->rows;
    
    // Copy timestamps
    memcpy(new_r->t, r->t, r->n * sizeof(time_t));
    
    // Setup arrays for new RRDR
    if(new_r->d) {
        new_r->dp = onewayalloc_callocz(owa, new_r->d, sizeof(*new_r->dp));
        new_r->dview = onewayalloc_callocz(owa, new_r->d, sizeof(*new_r->dview));
        
        if(new_r->n) {
            // Initialize all values as empty
            for (size_t i = 0; i < new_r->n; i++) {
                for (size_t d = 0; d < new_r->d; d++) {
                    size_t idx = i * new_r->d + d;
                    new_r->v[idx] = NAN;
                    new_r->ar[idx] = 0.0;
                    new_r->o[idx] = RRDR_VALUE_EMPTY;
                }
            }
        }
    }
    
    // Copy top dimensions
    size_t kept_dimensions = (remaining_count > 0) ? qt->request.cardinality_limit - 1 : queried_count;
    
    for (size_t i = 0; i < kept_dimensions; i++) {
        size_t src_d = sorted_dims[i].dim_idx;
        
        // Copy metadata
        new_r->di[i] = string_dup(r->di[src_d]);
        new_r->dn[i] = string_dup(r->dn[src_d]);
        new_r->od[i] = r->od[src_d];
        new_r->du[i] = string_dup(r->du[src_d]);
        new_r->dp[i] = r->dp[src_d];
        
        // Copy data
        for (size_t row = 0; row < r->rows; row++) {
            size_t src_idx = row * r->d + src_d;
            size_t dst_idx = row * new_r->d + i;
            
            new_r->v[dst_idx] = r->v[src_idx];
            new_r->ar[dst_idx] = r->ar[src_idx];
            new_r->o[dst_idx] = r->o[src_idx];
        }
        
        // Copy dview stats
        if(r->dview)
            new_r->dview[i] = r->dview[src_d];
    }
    
    // Create "remaining N dimensions" if needed
    if (remaining_count > 0) {
        size_t remaining_idx = kept_dimensions;
        
        char remaining_name[256];
        snprintfz(remaining_name, sizeof(remaining_name), "remaining %zu dimension%s", 
                 remaining_count, remaining_count == 1 ? "" : "s");
        
        new_r->di[remaining_idx] = string_strdupz(remaining_name);
        new_r->dn[remaining_idx] = string_strdupz(remaining_name);
        new_r->od[remaining_idx] = RRDR_DIMENSION_QUERIED | RRDR_DIMENSION_NONZERO;
        
        // Use the units from the first remaining dimension
        if(kept_dimensions < queried_count) {
            size_t first_remaining_d = sorted_dims[kept_dimensions].dim_idx;
            new_r->du[remaining_idx] = string_dup(r->du[first_remaining_d]);
            new_r->dp[remaining_idx] = r->dp[first_remaining_d];
        }
        
        // Aggregate remaining dimensions
        NETDATA_DOUBLE sum = 0.0, min = NAN, max = NAN, ars = 0.0;
        size_t count = 0;
        
        for (size_t row = 0; row < r->rows; row++) {
            size_t dst_idx = row * new_r->d + remaining_idx;
            NETDATA_DOUBLE aggregated_value = 0.0;
            NETDATA_DOUBLE aggregated_ar = 0.0;
            RRDR_VALUE_FLAGS aggregated_flags = RRDR_VALUE_NOTHING;
            bool has_values = false;
            
            for (size_t i = kept_dimensions; i < queried_count; i++) {
                size_t src_d = sorted_dims[i].dim_idx;
                size_t src_idx = row * r->d + src_d;
                
                if(!(r->o[src_idx] & RRDR_VALUE_EMPTY)) {
                    NETDATA_DOUBLE value = r->v[src_idx];
                    if(!isnan(value)) {
                        aggregated_value += value;
                        aggregated_ar += r->ar[src_idx];
                        aggregated_flags |= (r->o[src_idx] & (RRDR_VALUE_RESET | RRDR_VALUE_PARTIAL));
                        has_values = true;
                    }
                }
            }
            
            if(has_values) {
                new_r->v[dst_idx] = aggregated_value;
                new_r->ar[dst_idx] = aggregated_ar;
                new_r->o[dst_idx] = aggregated_flags & ~RRDR_VALUE_EMPTY;
                
                // Update statistics for dview
                sum += aggregated_value;
                ars += aggregated_ar;
                if(count == 0) {
                    min = max = aggregated_value;
                } else {
                    if(aggregated_value < min) min = aggregated_value;
                    if(aggregated_value > max) max = aggregated_value;
                }
                count++;
            } else {
                new_r->v[dst_idx] = NAN;
                new_r->ar[dst_idx] = 0.0;
                new_r->o[dst_idx] = RRDR_VALUE_EMPTY;
            }
        }
        
        // Set dview for remaining dimension
        if(new_r->dview) {
            new_r->dview[remaining_idx] = (STORAGE_POINT) {
                .sum = sum,
                .count = count,
                .min = min,
                .max = max,
                .anomaly_count = (size_t)(ars * RRDR_DVIEW_ANOMALY_COUNT_MULTIPLIER / 100.0),
            };
        }
    }
    
    // Cleanup
    onewayalloc_freez(owa, contributions);
    onewayalloc_freez(owa, sorted_dims);
    
    // Free the original RRDR 
    rrdr_free(owa, r);
    
    return new_r;
}
