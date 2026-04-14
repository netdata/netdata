// SPDX-License-Identifier: GPL-3.0-or-later

#include "query-internal.h"

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

