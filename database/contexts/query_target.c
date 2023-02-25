// SPDX-License-Identifier: GPL-3.0-or-later

#include "internal.h"

// ----------------------------------------------------------------------------
// query API

typedef struct query_target_locals {
    time_t start_s;

    QUERY_TARGET *qt;

    RRDSET *st;

    const char *scope_hosts;
    const char *scope_contexts;

    const char *hosts;
    const char *contexts;
    const char *charts;
    const char *dimensions;
    const char *chart_label_key;
    const char *labels;
    const char *alerts;

    long long after;
    long long before;
    bool match_ids;
    bool match_names;

    size_t metrics_skipped_due_to_not_matching_timeframe;

    char host_uuid_buffer[UUID_STR_LEN];
    QUERY_HOST *qh; // temp to pass on callbacks, ignore otherwise - no need to free
} QUERY_TARGET_LOCALS;

static __thread QUERY_TARGET thread_query_target = {};
void query_target_release(QUERY_TARGET *qt) {
    if(unlikely(!qt || !qt->used)) return;

    simple_pattern_free(qt->hosts.scope_pattern);
    qt->hosts.scope_pattern = NULL;

    simple_pattern_free(qt->hosts.pattern);
    qt->hosts.pattern = NULL;

    simple_pattern_free(qt->contexts.scope_pattern);
    qt->contexts.scope_pattern = NULL;

    simple_pattern_free(qt->contexts.pattern);
    qt->contexts.pattern = NULL;

    simple_pattern_free(qt->instances.pattern);
    qt->instances.pattern = NULL;

    simple_pattern_free(qt->instances.chart_label_key_pattern);
    qt->instances.chart_label_key_pattern = NULL;

    simple_pattern_free(qt->instances.labels_pattern);
    qt->instances.labels_pattern = NULL;

    simple_pattern_free(qt->query.pattern);
    qt->query.pattern = NULL;

    // release the query
    for(size_t i = 0, used = qt->query.used; i < used ;i++) {
        QUERY_METRIC *qm = query_metric(qt, i);

        // reset the plans
        for(size_t p = 0; p < qm->plan.used; p++) {
            internal_fatal(qm->plan.array[p].initialized &&
                           !qm->plan.array[p].finalized,
                           "QUERY: left-over initialized plan");

            qm->plan.array[p].initialized = false;
            qm->plan.array[p].finalized = false;
        }
        qm->plan.used = 0;

        // reset the tiers
        for(size_t tier = 0; tier < storage_tiers ;tier++) {
            if(qm->tiers[tier].db_metric_handle) {
                STORAGE_ENGINE *eng = qm->tiers[tier].eng;
                eng->api.metric_release(qm->tiers[tier].db_metric_handle);
                qm->tiers[tier].db_metric_handle = NULL;
                qm->tiers[tier].eng = NULL;
            }
        }
    }
    qt->query.used = 0;

    // release the dimensions
    for(size_t i = 0, used = qt->dimensions.used; i < used ; i++) {
        QUERY_DIMENSION *qd = query_dimension(qt, i);
        rrdmetric_release(qd->rma);
        qd->rma = NULL;
    }
    qt->dimensions.used = 0;

    // release the instances
    for(size_t i = 0, used = qt->instances.used; i < used ;i++) {
        QUERY_INSTANCE *qi = &qt->instances.array[i];

        rrdinstance_release(qi->ria);
        qi->ria = NULL;

        string_freez(qi->id_fqdn);
        qi->id_fqdn = NULL;

        string_freez(qi->name_fqdn);
        qi->name_fqdn = NULL;
    }
    qt->instances.used = 0;

    // release the contexts
    for(size_t i = 0, used = qt->contexts.used; i < used ;i++) {
        QUERY_CONTEXT *qc = query_context(qt, i);
        rrdcontext_release(qc->rca);
        qc->rca = NULL;
    }
    qt->contexts.used = 0;

    // release the hosts
    for(size_t i = 0, used = qt->hosts.used; i < used ;i++) {
        QUERY_HOST *qh = query_host(qt, i);
        qh->host = NULL;
    }
    qt->hosts.used = 0;

    qt->db.minimum_latest_update_every_s = 0;
    qt->db.first_time_s = 0;
    qt->db.last_time_s = 0;

    qt->group_by.used = 0;

    qt->id[0] = '\0';

    qt->used = false;
}
void query_target_free(void) {
    QUERY_TARGET *qt = &thread_query_target;

    if(qt->used)
        query_target_release(qt);

    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->query.size * sizeof(QUERY_METRIC), __ATOMIC_RELAXED);
    freez(qt->query.array);
    qt->query.array = NULL;
    qt->query.size = 0;

    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->dimensions.size * sizeof(RRDMETRIC_ACQUIRED *), __ATOMIC_RELAXED);
    freez(qt->dimensions.array);
    qt->dimensions.array = NULL;
    qt->dimensions.size = 0;

    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->instances.size * sizeof(RRDINSTANCE_ACQUIRED *), __ATOMIC_RELAXED);
    freez(qt->instances.array);
    qt->instances.array = NULL;
    qt->instances.size = 0;

    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->contexts.size * sizeof(RRDCONTEXT_ACQUIRED *), __ATOMIC_RELAXED);
    freez(qt->contexts.array);
    qt->contexts.array = NULL;
    qt->contexts.size = 0;

    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->hosts.size * sizeof(RRDHOST *), __ATOMIC_RELAXED);
    freez(qt->hosts.array);
    qt->hosts.array = NULL;
    qt->hosts.size = 0;
}

#define query_target_retention_matches_query(qt, first_entry_s, last_entry_s, update_every_s) \
    (((first_entry_s) - ((update_every_s) * 2) <= (qt)->window.before) &&                     \
     ((last_entry_s)  + ((update_every_s) * 2) >= (qt)->window.after))

static bool query_target_add_metric(QUERY_TARGET_LOCALS *qtl, QUERY_HOST *qh, QUERY_CONTEXT *qc,
                                    QUERY_INSTANCE *qi, QUERY_DIMENSION *qd) {
    QUERY_TARGET *qt = qtl->qt;
    RRDMETRIC *rm = rrdmetric_acquired_value(qd->rma);
    RRDINSTANCE *ri = rm->ri;

    time_t common_first_time_s = 0;
    time_t common_last_time_s = 0;
    time_t common_update_every_s = 0;
    size_t tiers_added = 0;

    struct {
        STORAGE_ENGINE *eng;
        STORAGE_METRIC_HANDLE *db_metric_handle;
        time_t db_first_time_s;
        time_t db_last_time_s;
        time_t db_update_every_s;
    } tier_retention[storage_tiers];

    for (size_t tier = 0; tier < storage_tiers; tier++) {
        STORAGE_ENGINE *eng = qh->host->db[tier].eng;
        tier_retention[tier].eng = eng;
        tier_retention[tier].db_update_every_s = (time_t) (qh->host->db[tier].tier_grouping * ri->update_every_s);

        if(rm->rrddim && rm->rrddim->tiers[tier].db_metric_handle)
            tier_retention[tier].db_metric_handle = eng->api.metric_dup(rm->rrddim->tiers[tier].db_metric_handle);
        else
            tier_retention[tier].db_metric_handle = eng->api.metric_get(qh->host->db[tier].instance, &rm->uuid);

        if(tier_retention[tier].db_metric_handle) {
            tier_retention[tier].db_first_time_s = tier_retention[tier].eng->api.query_ops.oldest_time_s(tier_retention[tier].db_metric_handle);
            tier_retention[tier].db_last_time_s = tier_retention[tier].eng->api.query_ops.latest_time_s(tier_retention[tier].db_metric_handle);

            if(!common_first_time_s)
                common_first_time_s = tier_retention[tier].db_first_time_s;
            else if(tier_retention[tier].db_first_time_s)
                common_first_time_s = MIN(common_first_time_s, tier_retention[tier].db_first_time_s);

            if(!common_last_time_s)
                common_last_time_s = tier_retention[tier].db_last_time_s;
            else
                common_last_time_s = MAX(common_last_time_s, tier_retention[tier].db_last_time_s);

            if(!common_update_every_s)
                common_update_every_s = tier_retention[tier].db_update_every_s;
            else if(tier_retention[tier].db_update_every_s)
                common_update_every_s = MIN(common_update_every_s, tier_retention[tier].db_update_every_s);

            tiers_added++;
        }
        else {
            tier_retention[tier].db_first_time_s = 0;
            tier_retention[tier].db_last_time_s = 0;
            tier_retention[tier].db_update_every_s = 0;
        }
    }

    bool release_retention = true;
    bool timeframe_matches =
            (tiers_added &&
            query_target_retention_matches_query(qt, common_first_time_s, common_last_time_s, common_update_every_s))
            ? true : false;

    if(timeframe_matches) {
        RRDR_DIMENSION_FLAGS options = RRDR_DIMENSION_DEFAULT;

        if (rrd_flag_check(rm, RRD_FLAG_HIDDEN)
            || (rm->rrddim && rrddim_option_check(rm->rrddim, RRDDIM_OPTION_HIDDEN))) {
            options |= RRDR_DIMENSION_HIDDEN;
            options &= ~RRDR_DIMENSION_SELECTED;
        }

        if (qt->query.pattern) {
            // we have a dimensions pattern
            // lets see if this dimension is selected

            if ((qtl->match_ids   && simple_pattern_matches_string(qt->query.pattern, rm->id))
                || (qtl->match_names && rm->name != rm->id && simple_pattern_matches_string(qt->query.pattern, rm->name))
                    ) {
                // it matches the pattern
                options |= (RRDR_DIMENSION_SELECTED | RRDR_DIMENSION_NONZERO);
                options &= ~RRDR_DIMENSION_HIDDEN;
            }
            else {
                // it does not match the pattern
                options |= RRDR_DIMENSION_HIDDEN;
                options &= ~RRDR_DIMENSION_SELECTED;
            }
        }
        else {
            // we don't have a dimensions pattern
            // so this is a selected dimension
            // if it is not hidden
            if(!(options & RRDR_DIMENSION_HIDDEN))
                options |= RRDR_DIMENSION_SELECTED;
        }

        if((options & RRDR_DIMENSION_HIDDEN) && (options & RRDR_DIMENSION_SELECTED))
            options &= ~RRDR_DIMENSION_HIDDEN;

        if(!(options & RRDR_DIMENSION_HIDDEN) || (qt->request.options & RRDR_OPTION_PERCENTAGE)) {
            // we have a non-hidden dimension
            // let's add it to the query metrics

            if(ri->rrdset)
                ri->rrdset->last_accessed_time_s = qtl->start_s;

            if (qt->query.used == qt->query.size) {
                size_t old_mem = qt->query.size * sizeof(*qt->query.array);
                qt->query.size = (qt->query.size) ? qt->query.size * 2 : 1;
                size_t new_mem = qt->query.size * sizeof(*qt->query.array);
                qt->query.array = reallocz(qt->query.array, new_mem);

                __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
            }
            QUERY_METRIC *qm = &qt->query.array[qt->query.used++];
            memset(qm, 0, sizeof(*qm));

            qm->status = options;

            qm->link.query_host_id = qh->slot;
            qm->link.query_context_id = qc->slot;
            qm->link.query_instance_id = qi->slot;
            qm->link.query_dimension_id = qd->slot;

            if (!qt->db.first_time_s || common_first_time_s < qt->db.first_time_s)
                qt->db.first_time_s = common_first_time_s;

            if (!qt->db.last_time_s || common_last_time_s > qt->db.last_time_s)
                qt->db.last_time_s = common_last_time_s;

            for (size_t tier = 0; tier < storage_tiers; tier++) {
                qm->tiers[tier].eng = tier_retention[tier].eng;
                qm->tiers[tier].db_metric_handle = tier_retention[tier].db_metric_handle;
                qm->tiers[tier].db_first_time_s = tier_retention[tier].db_first_time_s;
                qm->tiers[tier].db_last_time_s = tier_retention[tier].db_last_time_s;
                qm->tiers[tier].db_update_every_s = tier_retention[tier].db_update_every_s;
            }

            release_retention = false;

            qi->metrics.selected++;
            qc->metrics.selected++;
            qh->metrics.selected++;
        }
        else {
            qi->metrics.excluded++;
            qc->metrics.excluded++;
            qh->metrics.excluded++;

            qd->status |= QUERY_STATUS_DIMENSION_HIDDEN;
        }
    }
    else {
        qi->metrics.excluded++;
        qc->metrics.excluded++;
        qh->metrics.excluded++;

        qd->status |= QUERY_STATUS_DIMENSION_NODATA;
        qtl->metrics_skipped_due_to_not_matching_timeframe++;
    }

    if(release_retention) {
        // cleanup anything we allocated to the retention we will not use
        for(size_t tier = 0; tier < storage_tiers ;tier++) {
            if (tier_retention[tier].db_metric_handle)
                tier_retention[tier].eng->api.metric_release(tier_retention[tier].db_metric_handle);
        }

        return false;
    }

    return true;
}

static bool query_target_add_dimension(QUERY_TARGET_LOCALS *qtl, QUERY_HOST *qh, QUERY_CONTEXT *qc, QUERY_INSTANCE *qi,
                                       RRDMETRIC_ACQUIRED *rma, bool queryable_instance, size_t *metrics_added) {
    QUERY_TARGET *qt = qtl->qt;

    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    if(rrd_flag_is_deleted(rm))
        return false;

    if(qt->dimensions.used == qt->dimensions.size) {
        size_t old_mem = qt->dimensions.size * sizeof(*qt->dimensions.array);
        qt->dimensions.size = (qt->dimensions.size) ? qt->dimensions.size * 2 : 1;
        size_t new_mem = qt->dimensions.size * sizeof(*qt->dimensions.array);
        qt->dimensions.array = reallocz(qt->dimensions.array, new_mem);

        __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
    }
    QUERY_DIMENSION *qd = &qt->dimensions.array[qt->dimensions.used];
    memset(qd, 0, sizeof(*qd));

    qd->slot = qt->dimensions.used++;
    qd->rma = rrdmetric_acquired_dup(rma);
    qd->status = QUERY_STATUS_NONE;

    bool undo = false;
    if(!queryable_instance) {
        qi->metrics.excluded++;
        qc->metrics.excluded++;
        qh->metrics.excluded++;
        qd->status |= QUERY_STATUS_EXCLUDED;

        time_t first_time_s = rm->first_time_s;
        time_t last_time_s = rrd_flag_is_collected(rm) ? qtl->start_s : rm->last_time_s;
        time_t update_every_s = rm->ri->update_every_s;
        if(!query_target_retention_matches_query(qt, first_time_s, last_time_s, update_every_s))
            undo = true;
    }
    else {
        if(query_target_add_metric(qtl, qh, qc, qi, qd))
            (*metrics_added)++;
        else
            undo = true;
    }

    if(undo) {
        rrdmetric_release(qd->rma);
        qd->rma = NULL;
        qt->dimensions.used--;
        return false;
    }

    return true;
}

static inline STRING *rrdinstance_id_fqdn_v1(RRDINSTANCE_ACQUIRED *ria) {
    if(unlikely(!ria))
        return NULL;

    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return string_dup(ri->id);
}

static inline STRING *rrdinstance_name_fqdn_v1(RRDINSTANCE_ACQUIRED *ria) {
    if(unlikely(!ria))
        return NULL;

    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return string_dup(ri->name);
}

static inline STRING *rrdinstance_id_fqdn_v2(RRDINSTANCE_ACQUIRED *ria) {
    if(unlikely(!ria))
        return NULL;

    char buffer[RRD_ID_LENGTH_MAX + 1];

    RRDHOST *host = rrdinstance_acquired_rrdhost(ria);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s@%s", rrdinstance_acquired_id(ria), host->machine_guid);
    return string_strdupz(buffer);
}

static inline STRING *rrdinstance_name_fqdn_v2(RRDINSTANCE_ACQUIRED *ria) {
    if(unlikely(!ria))
        return NULL;

    char buffer[RRD_ID_LENGTH_MAX + 1];

    RRDHOST *host = rrdinstance_acquired_rrdhost(ria);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s@%s", rrdinstance_acquired_name(ria), rrdhost_hostname(host));
    return string_strdupz(buffer);
}

RRDSET *rrdinstance_acquired_rrdset(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return ri->rrdset;
}

const char *rrdcontext_acquired_units(RRDCONTEXT_ACQUIRED *rca) {
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    return string2str(rc->units);
}

RRDSET_TYPE rrdcontext_acquired_chart_type(RRDCONTEXT_ACQUIRED *rca) {
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    return rc->chart_type;
}

const char *rrdcontext_acquired_title(RRDCONTEXT_ACQUIRED *rca) {
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    return string2str(rc->title);
}

static void query_target_eval_instance_rrdcalc(QUERY_TARGET_LOCALS *qtl __maybe_unused,
                                               QUERY_HOST *qh, QUERY_CONTEXT *qc, QUERY_INSTANCE *qi) {
    RRDSET *st = rrdinstance_acquired_rrdset(qi->ria);
    if (st) {
        netdata_rwlock_rdlock(&st->alerts.rwlock);
        for (RRDCALC *rc = st->alerts.base; rc; rc = rc->next) {
            switch(rc->status) {
                case RRDCALC_STATUS_CLEAR:
                    qi->alerts.clear++;
                    qc->alerts.clear++;
                    qh->alerts.clear++;
                    break;

                case RRDCALC_STATUS_WARNING:
                    qi->alerts.warning++;
                    qc->alerts.warning++;
                    qh->alerts.warning++;
                    break;

                case RRDCALC_STATUS_CRITICAL:
                    qi->alerts.critical++;
                    qc->alerts.critical++;
                    qh->alerts.critical++;
                    break;

                default:
                case RRDCALC_STATUS_UNINITIALIZED:
                case RRDCALC_STATUS_UNDEFINED:
                case RRDCALC_STATUS_REMOVED:
                    qi->alerts.other++;
                    qc->alerts.other++;
                    qh->alerts.other++;
                    break;
            }
        }
        netdata_rwlock_unlock(&st->alerts.rwlock);
    }
}

static bool query_target_match_alert_pattern(QUERY_INSTANCE *qi, SIMPLE_PATTERN *pattern) {
    if(!pattern)
        return true;

    RRDSET *st = rrdinstance_acquired_rrdset(qi->ria);
    if (!st)
        return false;

    BUFFER *wb = NULL;
    bool matched = false;
    netdata_rwlock_rdlock(&st->alerts.rwlock);
    if (st->alerts.base) {
        for (RRDCALC *rc = st->alerts.base; rc; rc = rc->next) {
            if(simple_pattern_matches_string(pattern, rc->name)) {
                matched = true;
                break;
            }

            if(!wb)
                wb = buffer_create(0, NULL);
            else
                buffer_flush(wb);

            buffer_fast_strcat(wb, string2str(rc->name), string_strlen(rc->name));
            buffer_fast_strcat(wb, ":", 1);
            buffer_strcat(wb, rrdcalc_status2string(rc->status));

            if(simple_pattern_matches_buffer(pattern, wb)) {
                matched = true;
                break;
            }
        }
    }
    netdata_rwlock_unlock(&st->alerts.rwlock);

    buffer_free(wb);
    return matched;
}

static bool query_target_add_instance(QUERY_TARGET_LOCALS *qtl, QUERY_HOST *qh, QUERY_CONTEXT *qc,
                                      RRDINSTANCE_ACQUIRED *ria, bool queryable_instance, bool filter_instances) {
    QUERY_TARGET *qt = qtl->qt;

    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    if(rrd_flag_is_deleted(ri))
        return false;

    if(qt->instances.used == qt->instances.size) {
        size_t old_mem = qt->instances.size * sizeof(*qt->instances.array);
        qt->instances.size = (qt->instances.size) ? qt->instances.size * 2 : 1;
        size_t new_mem = qt->instances.size * sizeof(*qt->instances.array);
        qt->instances.array = reallocz(qt->instances.array, new_mem);

        __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
    }
    QUERY_INSTANCE *qi = &qt->instances.array[qt->instances.used];
    memset(qi, 0, sizeof(*qi));

    qi->slot = qt->instances.used;
    qt->instances.used++;
    qi->ria = rrdinstance_acquired_dup(ria);
    qi->query_host_id = qh->slot;

    if(qt->request.version <= 1) {
        qi->id_fqdn = rrdinstance_id_fqdn_v1(ria);
        qi->name_fqdn = rrdinstance_name_fqdn_v1(ria);
    }
    else {
        qi->id_fqdn = rrdinstance_id_fqdn_v2(ria);
        qi->name_fqdn = rrdinstance_name_fqdn_v2(ria);
    }

    if(qt->db.minimum_latest_update_every_s == 0 || ri->update_every_s < qt->db.minimum_latest_update_every_s)
        qt->db.minimum_latest_update_every_s = ri->update_every_s;

    if(queryable_instance && filter_instances) {
        queryable_instance = false;
        if(!qt->instances.pattern
           || (qtl->match_ids   && simple_pattern_matches_string(qt->instances.pattern, ri->id))
           || (qtl->match_names && ri->name != ri->id && simple_pattern_matches_string(qt->instances.pattern, ri->name))
           || (qtl->match_ids   && simple_pattern_matches_string(qt->instances.pattern, qi->id_fqdn))
           || (qtl->match_names && qi->name_fqdn != qi->id_fqdn && simple_pattern_matches_string(qt->instances.pattern, qi->name_fqdn))
                )
            queryable_instance = true;
    }

    if(queryable_instance) {
        if ((qt->instances.chart_label_key_pattern && !rrdlabels_match_simple_pattern_parsed(ri->rrdlabels,
                                                                                             qt->instances.chart_label_key_pattern,
                                                                                             '\0', NULL)) ||
            (qt->instances.labels_pattern && !rrdlabels_match_simple_pattern_parsed(ri->rrdlabels,
                                                                                    qt->instances.labels_pattern, ':',
                                                                                    NULL)))
            queryable_instance = false;
    }

    if(queryable_instance) {
        if(qt->instances.alerts_pattern && !query_target_match_alert_pattern(qi, qt->instances.alerts_pattern))
            queryable_instance = false;
    }

    if(queryable_instance && qt->request.version >= 2)
        query_target_eval_instance_rrdcalc(qtl, qh, qc, qi);

    size_t dimensions_added = 0, metrics_added = 0;

    if(unlikely(qt->request.rma)) {
        if(query_target_add_dimension(qtl, qh, qc, qi, qt->request.rma, queryable_instance, &metrics_added))
            dimensions_added++;
    }
    else {
        RRDMETRIC *rm;
        dfe_start_read(ri->rrdmetrics, rm) {
                    if(query_target_add_dimension(qtl, qh, qc, qi, (RRDMETRIC_ACQUIRED *) rm_dfe.item, queryable_instance, &metrics_added))
                        dimensions_added++;
                }
        dfe_done(rm);
    }

    if(!dimensions_added) {
        qt->instances.used--;
        rrdinstance_release(ria);
        qi->ria = NULL;

        string_freez(qi->id_fqdn);
        qi->id_fqdn = NULL;

        string_freez(qi->name_fqdn);
        qi->name_fqdn = NULL;
    }
    else {
        if(metrics_added) {
            qc->instances.selected++;
            qh->instances.selected++;
        }
        else {
            qc->instances.excluded++;
            qh->instances.excluded++;
        }
    }

    return true;
}

static bool query_target_add_context(void *data, RRDCONTEXT_ACQUIRED *rca, bool queryable_context) {
    QUERY_TARGET_LOCALS *qtl = data;
    QUERY_HOST *qh = qtl->qh;
    QUERY_TARGET *qt = qtl->qt;

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    if(rrd_flag_is_deleted(rc))
        return false;

    if(qt->contexts.used == qt->contexts.size) {
        size_t old_mem = qt->contexts.size * sizeof(*qt->contexts.array);
        qt->contexts.size = (qt->contexts.size) ? qt->contexts.size * 2 : 1;
        size_t new_mem = qt->contexts.size * sizeof(*qt->contexts.array);
        qt->contexts.array = reallocz(qt->contexts.array, new_mem);

        __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
    }
    QUERY_CONTEXT *qc = &qt->contexts.array[qt->contexts.used];
    memset(qc, 0, sizeof(*qc));
    qc->slot = qt->contexts.used++;
    qc->rca =  rrdcontext_acquired_dup(rca);

    size_t added = 0;
    if(unlikely(qt->request.ria)) {
        if(query_target_add_instance(qtl, qh, qc, qt->request.ria, queryable_context, false))
            added++;
    }
    else if(unlikely(qtl->st && qtl->st->rrdcontext == rca && qtl->st->rrdinstance)) {
        if(query_target_add_instance(qtl, qh, qc, qtl->st->rrdinstance, queryable_context, false))
            added++;
    }
    else {
        RRDINSTANCE *ri;
        dfe_start_read(rc->rrdinstances, ri) {
                    if(query_target_add_instance(qtl, qh, qc, (RRDINSTANCE_ACQUIRED *)ri_dfe.item, queryable_context, true))
                        added++;
                }
        dfe_done(ri);
    }

    if(!added) {
        qt->contexts.used--;
        rrdcontext_release(qc->rca);
    }

    return true;
}

static bool query_target_add_host(void *data, RRDHOST *host, bool queryable_host) {
    QUERY_TARGET_LOCALS *qtl = data;
    QUERY_TARGET *qt = qtl->qt;

    if(qt->hosts.used == qt->hosts.size) {
        size_t old_mem = qt->hosts.size * sizeof(*qt->hosts.array);
        qt->hosts.size = (qt->hosts.size) ? qt->hosts.size * 2 : 1;
        size_t new_mem = qt->hosts.size * sizeof(*qt->hosts.array);
        qt->hosts.array = reallocz(qt->hosts.array, new_mem);

        __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
    }
    QUERY_HOST *qh = &qt->hosts.array[qt->hosts.used];
    memset(qh, 0, sizeof(*qh));
    qh->slot = qt->hosts.used++;
    qh->host = host;

    if(host->node_id) {
        if(!qtl->host_uuid_buffer[0])
            uuid_unparse_lower(*host->node_id, qh->node_id);
        else
            memcpy(qh->node_id, qtl->host_uuid_buffer, sizeof(qh->node_id));
    }
    else
        qh->node_id[0] = '\0';

    // is the chart given valid?
    if(unlikely(qtl->st && (!qtl->st->rrdinstance || !qtl->st->rrdcontext))) {
        error("QUERY TARGET: RRDSET '%s' given, but it is not linked to rrdcontext structures. Linking it now.", rrdset_name(qtl->st));
        rrdinstance_from_rrdset(qtl->st);

        if(unlikely(qtl->st && (!qtl->st->rrdinstance || !qtl->st->rrdcontext))) {
            error("QUERY TARGET: RRDSET '%s' given, but failed to be linked to rrdcontext structures. Switching to context query.",
                  rrdset_name(qtl->st));

            if (!is_valid_sp(qtl->charts))
                qtl->charts = rrdset_name(qtl->st);

            qtl->st = NULL;
        }
    }

    qtl->qh = qh;

    size_t added = 0;
    if(unlikely(qt->request.rca)) {
        if(query_target_add_context(qtl, qt->request.rca, true))
            added++;
    }
    else if(unlikely(qtl->st)) {
        // single chart data queries
        if(query_target_add_context(qtl, qtl->st->rrdcontext, true))
            added++;
    }
    else {
        // context pattern queries
        added = query_scope_foreach_context(
                host, qtl->scope_contexts,
                qt->contexts.scope_pattern, qt->contexts.pattern,
                query_target_add_context, queryable_host, qtl);
    }

    if(!added) {
        qt->hosts.used--;
        return false;
    }

    return true;
}

void query_target_generate_name(QUERY_TARGET *qt) {
    char options_buffer[100 + 1];
    web_client_api_request_v1_data_options_to_string(options_buffer, 100, qt->request.options);

    char resampling_buffer[20 + 1] = "";
    if(qt->request.resampling_time > 1)
        snprintfz(resampling_buffer, 20, "/resampling:%lld", (long long)qt->request.resampling_time);

    char tier_buffer[20 + 1] = "";
    if(qt->request.options & RRDR_OPTION_SELECTED_TIER)
        snprintfz(tier_buffer, 20, "/tier:%zu", qt->request.tier);

    if(qt->request.st)
        snprintfz(qt->id, MAX_QUERY_TARGET_ID_LENGTH, "chart://hosts:%s/instance:%s/dimensions:%s/after:%lld/before:%lld/points:%zu/group:%s%s/options:%s%s%s"
                , rrdhost_hostname(qt->request.st->rrdhost)
                , rrdset_name(qt->request.st)
                , (qt->request.dimensions) ? qt->request.dimensions : "*"
                , (long long)qt->request.after
                , (long long)qt->request.before
                , qt->request.points
                , time_grouping_tostring(qt->request.time_group_method)
                , qt->request.time_group_options ? qt->request.time_group_options : ""
                , options_buffer
                , resampling_buffer
                , tier_buffer
        );
    else if(qt->request.host && qt->request.rca && qt->request.ria && qt->request.rma)
        snprintfz(qt->id, MAX_QUERY_TARGET_ID_LENGTH, "metric://hosts:%s/context:%s/instance:%s/dimension:%s/after:%lld/before:%lld/points:%zu/group:%s%s/options:%s%s%s"
                , rrdhost_hostname(qt->request.host)
                , rrdcontext_acquired_id(qt->request.rca)
                , rrdinstance_acquired_id(qt->request.ria)
                , rrdmetric_acquired_id(qt->request.rma)
                , (long long)qt->request.after
                , (long long)qt->request.before
                , qt->request.points
                , time_grouping_tostring(qt->request.time_group_method)
                , qt->request.time_group_options ? qt->request.time_group_options : ""
                , options_buffer
                , resampling_buffer
                , tier_buffer
        );
    else
        snprintfz(qt->id, MAX_QUERY_TARGET_ID_LENGTH, "context://hosts:%s/contexts:%s/instances:%s/dimensions:%s/after:%lld/before:%lld/points:%zu/group:%s%s/options:%s%s%s"
                , (qt->request.host) ? rrdhost_hostname(qt->request.host) : ((qt->request.nodes) ? qt->request.nodes : "*")
                , (qt->request.contexts) ? qt->request.contexts : "*"
                , (qt->request.instances) ? qt->request.instances : "*"
                , (qt->request.dimensions) ? qt->request.dimensions : "*"
                , (long long)qt->request.after
                , (long long)qt->request.before
                , qt->request.points
                , time_grouping_tostring(qt->request.time_group_method)
                , qt->request.time_group_options ? qt->request.time_group_options : ""
                , options_buffer
                , resampling_buffer
                , tier_buffer
        );

    json_fix_string(qt->id);
}

QUERY_TARGET *query_target_create(QUERY_TARGET_REQUEST *qtr) {
    if(!service_running(ABILITY_DATA_QUERIES))
        return NULL;

    QUERY_TARGET *qt = &thread_query_target;

    if(qt->used)
        fatal("QUERY TARGET: this query target is already used (%zu queries made with this QUERY_TARGET so far).", qt->queries);

    qt->used = true;
    qt->queries++;

    if(!qtr->received_ut)
        qtr->received_ut = now_monotonic_usec();

    qt->timings.received_ut = qtr->received_ut;

    if(qtr->nodes && !qtr->scope_nodes)
        qtr->scope_nodes = qtr->nodes;

    if(qtr->contexts && !qtr->scope_contexts)
        qtr->scope_contexts = qtr->contexts;

    memset(&qt->query_stats, 0, sizeof(qt->query_stats));

    // copy the request into query_thread_target
    qt->request = *qtr;

    query_target_generate_name(qt);
    qt->window.after = qt->request.after;
    qt->window.before = qt->request.before;
    rrdr_relative_window_to_absolute(&qt->window.after, &qt->window.before);

    // prepare our local variables - we need these across all these functions
    QUERY_TARGET_LOCALS qtl = {
            .qt = qt,
            .start_s = now_realtime_sec(),
            .st = qt->request.st,
            .scope_hosts = qt->request.scope_nodes,
            .scope_contexts = qt->request.scope_contexts,
            .hosts = qt->request.nodes,
            .contexts = qt->request.contexts,
            .charts = qt->request.instances,
            .dimensions = qt->request.dimensions,
            .chart_label_key = qt->request.chart_label_key,
            .labels = qt->request.labels,
            .alerts = qt->request.alerts,
    };

    RRDHOST *host = qt->request.host;

    qt->db.minimum_latest_update_every_s = 0; // it will be updated by query_target_add_query()

    // prepare all the patterns
    qt->hosts.scope_pattern = string_to_simple_pattern(qtl.scope_hosts);
    qt->hosts.pattern = string_to_simple_pattern(qtl.hosts);

    qt->contexts.pattern = string_to_simple_pattern(qtl.contexts);
    qt->contexts.scope_pattern = string_to_simple_pattern(qtl.scope_contexts);

    qt->instances.pattern = string_to_simple_pattern(qtl.charts);
    qt->query.pattern = string_to_simple_pattern(qtl.dimensions);
    qt->instances.chart_label_key_pattern = string_to_simple_pattern(qtl.chart_label_key);
    qt->instances.labels_pattern = string_to_simple_pattern(qtl.labels);
    qt->instances.alerts_pattern = string_to_simple_pattern(qtl.alerts);

    qtl.match_ids = qt->request.options & RRDR_OPTION_MATCH_IDS;
    qtl.match_names = qt->request.options & RRDR_OPTION_MATCH_NAMES;
    if(likely(!qtl.match_ids && !qtl.match_names))
        qtl.match_ids = qtl.match_names = true;

    // verify that the chart belongs to the host we are interested
    if(qtl.st) {
        if (!host) {
            // It is NULL, set it ourselves.
            host = qtl.st->rrdhost;
        }
        else if (unlikely(host != qtl.st->rrdhost)) {
            // Oops! A different host!
            error("QUERY TARGET: RRDSET '%s' given does not belong to host '%s'. Switching query host to '%s'",
                  rrdset_name(qtl.st), rrdhost_hostname(host), rrdhost_hostname(qtl.st->rrdhost));
            host = qtl.st->rrdhost;
        }
    }

    if(host) {
        // single host query
        qt->versions.contexts_hard_hash = dictionary_version(host->rrdctx.contexts);
        qt->versions.contexts_soft_hash = dictionary_version(host->rrdctx.hub_queue);
        query_target_add_host(&qtl, host, true);
        qtl.hosts = rrdhost_hostname(host);
    }
    else
        query_scope_foreach_host(qt->hosts.scope_pattern, qt->hosts.pattern, query_target_add_host, &qtl,
                                 &qt->versions.contexts_hard_hash, &qt->versions.contexts_soft_hash,
                                 qtl.host_uuid_buffer);

    // we need the available db retention for this call
    // so it has to be done last
    query_target_calculate_window(qt);

    qt->timings.preprocessed_ut = now_monotonic_usec();

    return qt;
}
