// SPDX-License-Identifier: GPL-3.0-or-later

#include "internal.h"

#define QUERY_TARGET_MAX_REALLOC_INCREASE 500
#define query_target_realloc_size(size, start) \
            (size) ? ((size) < QUERY_TARGET_MAX_REALLOC_INCREASE ? (size) * 2 : (size) + QUERY_TARGET_MAX_REALLOC_INCREASE) : (start)

static void query_metric_release(QUERY_TARGET *qt, QUERY_METRIC *qm);
static void query_dimension_release(QUERY_DIMENSION *qd);
static void query_instance_release(QUERY_INSTANCE *qi);
static void query_context_release(QUERY_CONTEXT *qc);
static void query_node_release(QUERY_NODE *qn);

static __thread QUERY_TARGET *thread_qt = NULL;
static struct {
    struct {
        SPINLOCK spinlock;
        size_t count;
        QUERY_TARGET *base;
    } available;

    struct {
        SPINLOCK spinlock;
        size_t count;
        QUERY_TARGET *base;
    } used;
} query_target_base = {
        .available = {
                .spinlock = SPINLOCK_INITIALIZER,
                .base = NULL,
                .count = 0,
        },
        .used = {
                .spinlock = SPINLOCK_INITIALIZER,
                .base = NULL,
                .count = 0,
        },
};

static void query_target_destroy(QUERY_TARGET *qt) {
    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->query.size * sizeof(*qt->query.array), __ATOMIC_RELAXED);
    freez(qt->query.array);

    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->dimensions.size * sizeof(*qt->dimensions.array), __ATOMIC_RELAXED);
    freez(qt->dimensions.array);

    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->instances.size * sizeof(*qt->instances.array), __ATOMIC_RELAXED);
    freez(qt->instances.array);

    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->contexts.size * sizeof(*qt->contexts.array), __ATOMIC_RELAXED);
    freez(qt->contexts.array);

    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->nodes.size * sizeof(*qt->nodes.array), __ATOMIC_RELAXED);
    freez(qt->nodes.array);

    freez(qt);
}

void query_target_release(QUERY_TARGET *qt) {
    if(unlikely(!qt)) return;

    internal_fatal(!qt->internal.used, "QUERY TARGET: qt to be released is not used");

    simple_pattern_free(qt->nodes.scope_pattern);
    qt->nodes.scope_pattern = NULL;

    simple_pattern_free(qt->nodes.pattern);
    qt->nodes.pattern = NULL;

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
        query_metric_release(qt, qm);
    }
    qt->query.used = 0;

    // release the dimensions
    for(size_t i = 0, used = qt->dimensions.used; i < used ; i++) {
        QUERY_DIMENSION *qd = query_dimension(qt, i);
        query_dimension_release(qd);
    }
    qt->dimensions.used = 0;

    // release the instances
    for(size_t i = 0, used = qt->instances.used; i < used ;i++) {
        QUERY_INSTANCE *qi = query_instance(qt, i);
        query_instance_release(qi);
    }
    qt->instances.used = 0;

    // release the contexts
    for(size_t i = 0, used = qt->contexts.used; i < used ;i++) {
        QUERY_CONTEXT *qc = query_context(qt, i);
        rrdcontext_release(qc->rca);
        qc->rca = NULL;
    }
    qt->contexts.used = 0;

    // release the nodes
    for(size_t i = 0, used = qt->nodes.used; i < used ; i++) {
        QUERY_NODE *qn = query_node(qt, i);
        query_node_release(qn);
    }
    qt->nodes.used = 0;

    qt->db.minimum_latest_update_every_s = 0;
    qt->db.first_time_s = 0;
    qt->db.last_time_s = 0;

    for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++)
        qt->group_by[g].used = 0;

    qt->id[0] = '\0';

    spinlock_lock(&query_target_base.used.spinlock);
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(query_target_base.used.base, qt, internal.prev, internal.next);
    query_target_base.used.count--;
    spinlock_unlock(&query_target_base.used.spinlock);

    qt->internal.used = false;
    thread_qt = NULL;

    if (qt->internal.queries > 1000) {
        query_target_destroy(qt);
    }
    else {
        spinlock_lock(&query_target_base.available.spinlock);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(query_target_base.available.base, qt, internal.prev, internal.next);
        query_target_base.available.count++;
        spinlock_unlock(&query_target_base.available.spinlock);
    }
}

static QUERY_TARGET *query_target_get(void) {
    spinlock_lock(&query_target_base.available.spinlock);
    QUERY_TARGET *qt = query_target_base.available.base;
    if (qt) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(query_target_base.available.base, qt, internal.prev, internal.next);
        query_target_base.available.count--;
    }
    spinlock_unlock(&query_target_base.available.spinlock);

    if(unlikely(!qt))
        qt = callocz(1, sizeof(*qt));

    spinlock_lock(&query_target_base.used.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(query_target_base.used.base, qt, internal.prev, internal.next);
    query_target_base.used.count++;
    spinlock_unlock(&query_target_base.used.spinlock);

    qt->internal.used = true;
    qt->internal.queries++;
    thread_qt = qt;

    return qt;
}

// this is used to release a query target from a cancelled thread
void query_target_free(void) {
    query_target_release(thread_qt);
}

// ----------------------------------------------------------------------------
// query API

typedef struct query_target_locals {
    time_t start_s;

    QUERY_TARGET *qt;

    RRDSET *st;

    const char *scope_nodes;
    const char *scope_contexts;

    const char *nodes;
    const char *contexts;
    const char *instances;
    const char *dimensions;
    const char *chart_label_key;
    const char *labels;
    const char *alerts;

    long long after;
    long long before;
    bool match_ids;
    bool match_names;

    size_t metrics_skipped_due_to_not_matching_timeframe;

    char host_node_id_str[UUID_STR_LEN];
    QUERY_NODE *qn; // temp to pass on callbacks, ignore otherwise - no need to free
} QUERY_TARGET_LOCALS;

struct storage *query_metric_storage_engine(QUERY_TARGET *qt, QUERY_METRIC *qm, size_t tier) {
    QUERY_NODE *qn = query_node(qt, qm->link.query_node_id);
    return qn->rrdhost->db[tier].eng;
}

static inline void query_metric_release(QUERY_TARGET *qt, QUERY_METRIC *qm) {
    qm->plan.used = 0;

    // reset the tiers
    for(size_t tier = 0; tier < nd_profile.storage_tiers;tier++) {
        if(qm->tiers[tier].smh) {
            STORAGE_ENGINE *eng = query_metric_storage_engine(qt, qm, tier);
            eng->api.metric_release(qm->tiers[tier].smh);
            qm->tiers[tier].smh = NULL;
        }
    }
}

static bool query_metric_add(QUERY_TARGET_LOCALS *qtl, QUERY_NODE *qn, QUERY_CONTEXT *qc,
                             QUERY_INSTANCE *qi, size_t qd_slot, RRDMETRIC *rm, RRDR_DIMENSION_FLAGS options) {
    QUERY_TARGET *qt = qtl->qt;
    RRDINSTANCE *ri = rm->ri;

    time_t common_first_time_s = 0;
    time_t common_last_time_s = 0;
    time_t common_update_every_s = 0;
    size_t tiers_added = 0;

    struct {
        STORAGE_ENGINE *eng;
        STORAGE_METRIC_HANDLE *smh;
        time_t db_first_time_s;
        time_t db_last_time_s;
        time_t db_update_every_s;
    } tier_retention[nd_profile.storage_tiers];

    for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
        STORAGE_ENGINE *eng = qn->rrdhost->db[tier].eng;
        tier_retention[tier].eng = eng;
        tier_retention[tier].db_update_every_s = (time_t) (qn->rrdhost->db[tier].tier_grouping * ri->update_every_s);

        if(rm->rrddim && rm->rrddim->tiers[tier].smh)
            tier_retention[tier].smh = eng->api.metric_dup(rm->rrddim->tiers[tier].smh);
        else
            tier_retention[tier].smh = eng->api.metric_get_by_id(qn->rrdhost->db[tier].si, rm->uuid);

        if(tier_retention[tier].smh) {
            tier_retention[tier].db_first_time_s = storage_engine_oldest_time_s(tier_retention[tier].eng->seb, tier_retention[tier].smh);
            tier_retention[tier].db_last_time_s = storage_engine_latest_time_s(tier_retention[tier].eng->seb, tier_retention[tier].smh);

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

    for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
        if(!qt->db.tiers[tier].update_every || (tier_retention[tier].db_update_every_s && tier_retention[tier].db_update_every_s < qt->db.tiers[tier].update_every))
            qt->db.tiers[tier].update_every = tier_retention[tier].db_update_every_s;

        if(!qt->db.tiers[tier].retention.first_time_s || (tier_retention[tier].db_first_time_s && tier_retention[tier].db_first_time_s < qt->db.tiers[tier].retention.first_time_s))
            qt->db.tiers[tier].retention.first_time_s = tier_retention[tier].db_first_time_s;

        if(!qt->db.tiers[tier].retention.last_time_s || (tier_retention[tier].db_last_time_s && tier_retention[tier].db_last_time_s > qt->db.tiers[tier].retention.last_time_s))
            qt->db.tiers[tier].retention.last_time_s = tier_retention[tier].db_last_time_s;
    }

    bool timeframe_matches =
            (tiers_added &&
             query_matches_retention(qt->window.after, qt->window.before, common_first_time_s, common_last_time_s, common_update_every_s))
            ? true : false;

    if(timeframe_matches) {
        if(ri->rrdset)
            ri->rrdset->last_accessed_time_s = qtl->start_s;

        if (qt->query.used == qt->query.size) {
            size_t old_mem = qt->query.size * sizeof(*qt->query.array);
            qt->query.size = query_target_realloc_size(qt->query.size, 4);
            size_t new_mem = qt->query.size * sizeof(*qt->query.array);
            qt->query.array = reallocz(qt->query.array, new_mem);

            __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
        }
        QUERY_METRIC *qm = &qt->query.array[qt->query.used++];
        memset(qm, 0, sizeof(*qm));

        qm->status = options;

        qm->link.query_node_id = qn->slot;
        qm->link.query_context_id = qc->slot;
        qm->link.query_instance_id = qi->slot;
        qm->link.query_dimension_id = qd_slot;

        if (!qt->db.first_time_s || common_first_time_s < qt->db.first_time_s)
            qt->db.first_time_s = common_first_time_s;

        if (!qt->db.last_time_s || common_last_time_s > qt->db.last_time_s)
            qt->db.last_time_s = common_last_time_s;

        for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
            internal_fatal(tier_retention[tier].eng != query_metric_storage_engine(qt, qm, tier), "QUERY TARGET: storage engine mismatch");
            qm->tiers[tier].smh = tier_retention[tier].smh;
            qm->tiers[tier].db_first_time_s = tier_retention[tier].db_first_time_s;
            qm->tiers[tier].db_last_time_s = tier_retention[tier].db_last_time_s;
            qm->tiers[tier].db_update_every_s = tier_retention[tier].db_update_every_s;
        }

        return true;
    }

    // cleanup anything we allocated to the retention we will not use
    for(size_t tier = 0; tier < nd_profile.storage_tiers;tier++) {
        if (tier_retention[tier].smh) {
            tier_retention[tier].eng->api.metric_release(tier_retention[tier].smh);
            tier_retention[tier].smh = NULL;
        }
    }

    return false;
}

static inline bool rrdmetric_retention_matches_query(QUERY_TARGET *qt, RRDMETRIC *rm, time_t now_s) {
    time_t first_time_s = rm->first_time_s;
    time_t last_time_s = rrd_flag_is_collected(rm) ? now_s : rm->last_time_s;
    time_t update_every_s = rm->ri->update_every_s;
    return query_matches_retention(qt->window.after, qt->window.before, first_time_s, last_time_s, update_every_s);
}

static inline void query_dimension_release(QUERY_DIMENSION *qd) {
    rrdmetric_release(qd->rma);
    qd->rma = NULL;
}

static QUERY_DIMENSION *query_dimension_allocate(QUERY_TARGET *qt, RRDMETRIC_ACQUIRED *rma, QUERY_STATUS status, size_t priority) {
    if(qt->dimensions.used == qt->dimensions.size) {
        size_t old_mem = qt->dimensions.size * sizeof(*qt->dimensions.array);
        qt->dimensions.size = query_target_realloc_size(qt->dimensions.size, 4);
        size_t new_mem = qt->dimensions.size * sizeof(*qt->dimensions.array);
        qt->dimensions.array = reallocz(qt->dimensions.array, new_mem);

        __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
    }
    QUERY_DIMENSION *qd = &qt->dimensions.array[qt->dimensions.used];
    memset(qd, 0, sizeof(*qd));

    qd->slot = qt->dimensions.used++;
    qd->rma = rrdmetric_acquired_dup(rma);
    qd->status = status;
    qd->priority = priority;

    return qd;
}

static bool query_dimension_add(QUERY_TARGET_LOCALS *qtl, QUERY_NODE *qn, QUERY_CONTEXT *qc, QUERY_INSTANCE *qi,
                                RRDMETRIC_ACQUIRED *rma, bool queryable_instance, size_t *metrics_added, size_t priority) {
    QUERY_TARGET *qt = qtl->qt;

    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    if(rrd_flag_is_deleted(rm))
        return false;

    QUERY_STATUS status = QUERY_STATUS_NONE;

    bool undo = false;
    if(!queryable_instance) {
        if(rrdmetric_retention_matches_query(qt, rm, qtl->start_s)) {
            qi->metrics.excluded++;
            qc->metrics.excluded++;
            qn->metrics.excluded++;
            status |= QUERY_STATUS_EXCLUDED;
        }
        else
            undo = true;
    }
    else {
        RRDR_DIMENSION_FLAGS options = RRDR_DIMENSION_DEFAULT;
        bool needed = false;

        if (qt->query.pattern) {
            // the user asked for specific dimensions

            SIMPLE_PATTERN_RESULT ret = SP_NOT_MATCHED;

            if(qtl->match_ids)
                ret = simple_pattern_matches_string_extract(qt->query.pattern, rm->id, NULL, 0);

            if(ret == SP_NOT_MATCHED && qtl->match_names && (rm->name != rm->id || !qtl->match_ids))
                ret = simple_pattern_matches_string_extract(qt->query.pattern, rm->name, NULL, 0);

            if(ret == SP_MATCHED_POSITIVE) {
                needed = true;
                options |= RRDR_DIMENSION_SELECTED | RRDR_DIMENSION_NONZERO;
            }
            else {
                // the user selection does not match this dimension
                // but, we may still need to query it

                if (query_target_needs_all_dimensions(qt)) {
                    // this is percentage calculation
                    // so, we need this dimension to calculate the percentage
                    needed = true;
                    options |= RRDR_DIMENSION_HIDDEN;
                }
                else {
                    // the user did not select this dimension
                    // and the calculation is not percentage
                    // so, no need to query it
                    ;
                }
            }
        }
        else {
            // we don't have a dimensions pattern
            // so this is a selected dimension
            // if it is not hidden

            if(rrd_flag_check(rm, RRD_FLAG_HIDDEN) || (rm->rrddim && rrddim_option_check(rm->rrddim, RRDDIM_OPTION_HIDDEN))) {
                // this is a hidden dimension
                // we don't need to query it
                status |= QUERY_STATUS_DIMENSION_HIDDEN;
                options |= RRDR_DIMENSION_HIDDEN;

                if (query_target_needs_all_dimensions(qt)) {
                    // this is percentage calculation
                    // so, we need this dimension to calculate the percentage
                    needed = true;
                }
            }
            else {
                // this is a not hidden dimension
                // and the user did not provide any selection for dimensions
                // so, we need to query it
                needed = true;
                options |= RRDR_DIMENSION_SELECTED;
            }
        }

        if (needed) {
            if(query_metric_add(qtl, qn, qc, qi, qt->dimensions.used, rm, options)) {
                (*metrics_added)++;

                qi->metrics.selected++;
                qc->metrics.selected++;
                qn->metrics.selected++;
            }
            else {
                undo = true;
                qtl->metrics_skipped_due_to_not_matching_timeframe++;
            }
        }
        else if(rrdmetric_retention_matches_query(qt, rm, qtl->start_s)) {
            qi->metrics.excluded++;
            qc->metrics.excluded++;
            qn->metrics.excluded++;
            status |= QUERY_STATUS_EXCLUDED;
        }
        else
            undo = true;
    }

    if(undo)
        return false;

    query_dimension_allocate(qt, rma, status, priority);
    return true;
}

static inline STRING *rrdinstance_create_id_fqdn_v1(RRDINSTANCE_ACQUIRED *ria) {
    if(unlikely(!ria))
        return NULL;

    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return string_dup(ri->id);
}

static inline STRING *rrdinstance_create_name_fqdn_v1(RRDINSTANCE_ACQUIRED *ria) {
    if(unlikely(!ria))
        return NULL;

    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return string_dup(ri->name);
}

static inline STRING *rrdinstance_create_id_fqdn_v2(RRDINSTANCE_ACQUIRED *ria) {
    if(unlikely(!ria))
        return NULL;

    char buffer[RRD_ID_LENGTH_MAX + 1];

    RRDHOST *host = rrdinstance_acquired_rrdhost(ria);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s@%s", rrdinstance_acquired_id(ria), host->machine_guid);
    return string_strdupz(buffer);
}

static inline STRING *rrdinstance_create_name_fqdn_v2(RRDINSTANCE_ACQUIRED *ria) {
    if(unlikely(!ria))
        return NULL;

    char buffer[RRD_ID_LENGTH_MAX + 1];

    RRDHOST *host = rrdinstance_acquired_rrdhost(ria);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s@%s", rrdinstance_acquired_name(ria), rrdhost_hostname(host));
    return string_strdupz(buffer);
}

inline STRING *query_instance_id_fqdn(QUERY_INSTANCE *qi, size_t version) {
    if(!qi->id_fqdn) {
        if (version <= 1)
            qi->id_fqdn = rrdinstance_create_id_fqdn_v1(qi->ria);
        else
            qi->id_fqdn = rrdinstance_create_id_fqdn_v2(qi->ria);
    }

    return qi->id_fqdn;
}

inline STRING *query_instance_name_fqdn(QUERY_INSTANCE *qi, size_t version) {
    if(!qi->name_fqdn) {
        if (version <= 1)
            qi->name_fqdn = rrdinstance_create_name_fqdn_v1(qi->ria);
        else
            qi->name_fqdn = rrdinstance_create_name_fqdn_v2(qi->ria);
    }

    return qi->name_fqdn;
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
                                               QUERY_NODE *qn, QUERY_CONTEXT *qc, QUERY_INSTANCE *qi) {
    RRDSET *st = rrdinstance_acquired_rrdset(qi->ria);
    if (st) {
        rw_spinlock_read_lock(&st->alerts.spinlock);
        for (RRDCALC *rc = st->alerts.base; rc; rc = rc->next) {
            switch(rc->status) {
                case RRDCALC_STATUS_CLEAR:
                    qi->alerts.clear++;
                    qc->alerts.clear++;
                    qn->alerts.clear++;
                    break;

                case RRDCALC_STATUS_WARNING:
                    qi->alerts.warning++;
                    qc->alerts.warning++;
                    qn->alerts.warning++;
                    break;

                case RRDCALC_STATUS_CRITICAL:
                    qi->alerts.critical++;
                    qc->alerts.critical++;
                    qn->alerts.critical++;
                    break;

                default:
                case RRDCALC_STATUS_UNINITIALIZED:
                case RRDCALC_STATUS_UNDEFINED:
                case RRDCALC_STATUS_REMOVED:
                    qi->alerts.other++;
                    qc->alerts.other++;
                    qn->alerts.other++;
                    break;
            }
        }
        rw_spinlock_read_unlock(&st->alerts.spinlock);
    }
}

static bool query_target_match_alert_pattern(RRDINSTANCE_ACQUIRED *ria, SIMPLE_PATTERN *pattern) {
    if(!pattern)
        return true;

    RRDSET *st = rrdinstance_acquired_rrdset(ria);
    if (!st)
        return false;

    BUFFER *wb = NULL;
    bool matched = false;
    rw_spinlock_read_lock(&st->alerts.spinlock);
    if (st->alerts.base) {
        for (RRDCALC *rc = st->alerts.base; rc; rc = rc->next) {
            SIMPLE_PATTERN_RESULT ret = simple_pattern_matches_string_extract(pattern, rc->config.name, NULL, 0);

            if(ret == SP_MATCHED_POSITIVE) {
                matched = true;
                break;
            }
            else if(ret == SP_MATCHED_NEGATIVE)
                break;

            if (!wb)
                wb = buffer_create(0, NULL);
            else
                buffer_flush(wb);

            buffer_fast_strcat(wb, string2str(rc->config.name), string_strlen(rc->config.name));
            buffer_fast_strcat(wb, ":", 1);
            buffer_strcat(wb, rrdcalc_status2string(rc->status));

            ret = simple_pattern_matches_buffer_extract(pattern, wb, NULL, 0);

            if(ret == SP_MATCHED_POSITIVE) {
                matched = true;
                break;
            }
            else if(ret == SP_MATCHED_NEGATIVE)
                break;
        }
    }
    rw_spinlock_read_unlock(&st->alerts.spinlock);

    buffer_free(wb);
    return matched;
}

static inline void query_instance_release(QUERY_INSTANCE *qi) {
    if(qi->ria) {
        rrdinstance_release(qi->ria);
        qi->ria = NULL;
    }

    string_freez(qi->id_fqdn);
    qi->id_fqdn = NULL;

    string_freez(qi->name_fqdn);
    qi->name_fqdn = NULL;
}

static inline QUERY_INSTANCE *query_instance_allocate(QUERY_TARGET *qt, RRDINSTANCE_ACQUIRED *ria, size_t qn_slot) {
    if(qt->instances.used == qt->instances.size) {
        size_t old_mem = qt->instances.size * sizeof(*qt->instances.array);
        qt->instances.size = query_target_realloc_size(qt->instances.size, 2);
        size_t new_mem = qt->instances.size * sizeof(*qt->instances.array);
        qt->instances.array = reallocz(qt->instances.array, new_mem);

        __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
    }
    QUERY_INSTANCE *qi = &qt->instances.array[qt->instances.used];
    memset(qi, 0, sizeof(*qi));

    qi->slot = qt->instances.used;
    qt->instances.used++;
    qi->ria = rrdinstance_acquired_dup(ria);
    qi->query_host_id = qn_slot;

    return qi;
}

static inline SIMPLE_PATTERN_RESULT query_instance_matches(QUERY_INSTANCE *qi,
                                             RRDINSTANCE *ri,
                                             SIMPLE_PATTERN *instances_sp,
                                             bool match_ids,
                                             bool match_names,
                                             size_t version,
                                             char *host_node_id_str) {
    SIMPLE_PATTERN_RESULT ret = SP_MATCHED_POSITIVE;

    if(instances_sp) {
        ret = SP_NOT_MATCHED;

        if(match_ids)
            ret = simple_pattern_matches_string_extract(instances_sp, ri->id, NULL, 0);
        if (ret == SP_NOT_MATCHED && match_names && (ri->name != ri->id || !match_ids))
            ret = simple_pattern_matches_string_extract(instances_sp, ri->name, NULL, 0);
        if (ret == SP_NOT_MATCHED && match_ids)
            ret = simple_pattern_matches_string_extract(instances_sp, query_instance_id_fqdn(qi, version), NULL, 0);
        if (ret == SP_NOT_MATCHED && match_names)
            ret = simple_pattern_matches_string_extract(instances_sp, query_instance_name_fqdn(qi, version), NULL, 0);

        if (ret == SP_NOT_MATCHED && match_ids && host_node_id_str[0]) {
            char buffer[RRD_ID_LENGTH_MAX + 1];
            snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s@%s", rrdinstance_acquired_id(qi->ria), host_node_id_str);
            ret = simple_pattern_matches_extract(instances_sp, buffer, NULL, 0);
        }
    }

    return ret;
}

static inline bool query_instance_matches_labels(
    RRDINSTANCE *ri,
    SIMPLE_PATTERN *chart_label_key_sp,
    SIMPLE_PATTERN *labels_sp)
{

    if (chart_label_key_sp && rrdlabels_match_simple_pattern_parsed(ri->rrdlabels, chart_label_key_sp, '\0', NULL) != SP_MATCHED_POSITIVE)
        return false;

    if (labels_sp) {
        struct pattern_array *pa = pattern_array_add_simple_pattern(NULL, labels_sp, ':');
        bool found = pattern_array_label_match(pa, ri->rrdlabels, ':', NULL);
        pattern_array_free(pa);
        return found;
    }

    return true;
}

static bool query_instance_add(QUERY_TARGET_LOCALS *qtl, QUERY_NODE *qn, QUERY_CONTEXT *qc,
                               RRDINSTANCE_ACQUIRED *ria, bool queryable_instance, bool filter_instances) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    if(rrd_flag_is_deleted(ri))
        return false;

    QUERY_TARGET *qt = qtl->qt;
    QUERY_INSTANCE *qi = query_instance_allocate(qt, ria, qn->slot);

    if(qt->db.minimum_latest_update_every_s == 0 || ri->update_every_s < qt->db.minimum_latest_update_every_s)
        qt->db.minimum_latest_update_every_s = ri->update_every_s;

    if(queryable_instance && filter_instances)
        queryable_instance = (SP_MATCHED_POSITIVE == query_instance_matches(
                qi, ri, qt->instances.pattern, qtl->match_ids, qtl->match_names, qt->request.version, qtl->host_node_id_str));

    if(queryable_instance)
        queryable_instance = query_instance_matches_labels(
            ri,
            qt->instances.chart_label_key_pattern,
            qt->instances.labels_pattern);

    if(queryable_instance) {
        if(qt->instances.alerts_pattern && !query_target_match_alert_pattern(ria, qt->instances.alerts_pattern))
            queryable_instance = false;
    }

    if(queryable_instance && qt->request.version >= 2)
        query_target_eval_instance_rrdcalc(qtl, qn, qc, qi);

    size_t dimensions_added = 0, metrics_added = 0, priority = 0;

    if(unlikely(qt->request.rma)) {
        if(query_dimension_add(qtl, qn, qc, qi, qt->request.rma, queryable_instance, &metrics_added, priority++))
            dimensions_added++;
    }
    else {
        RRDMETRIC *rm;
        dfe_start_read(ri->rrdmetrics, rm) {
            if(query_dimension_add(qtl, qn, qc, qi, (RRDMETRIC_ACQUIRED *) rm_dfe.item,
                                   queryable_instance, &metrics_added, priority++))
                dimensions_added++;
        }
        dfe_done(rm);
    }

    if(!dimensions_added) {
        qt->instances.used--;
        query_instance_release(qi);
        return false;
    }
    else {
        if(metrics_added) {
            qc->instances.selected++;
            qn->instances.selected++;
        }
        else {
            qc->instances.excluded++;
            qn->instances.excluded++;
        }
    }

    return true;
}

static inline void query_context_release(QUERY_CONTEXT *qc) {
    rrdcontext_release(qc->rca);
    qc->rca = NULL;
}

static inline QUERY_CONTEXT *query_context_allocate(QUERY_TARGET *qt, RRDCONTEXT_ACQUIRED *rca) {
    if(qt->contexts.used == qt->contexts.size) {
        size_t old_mem = qt->contexts.size * sizeof(*qt->contexts.array);
        qt->contexts.size = query_target_realloc_size(qt->contexts.size, 2);
        size_t new_mem = qt->contexts.size * sizeof(*qt->contexts.array);
        qt->contexts.array = reallocz(qt->contexts.array, new_mem);

        __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
    }
    QUERY_CONTEXT *qc = &qt->contexts.array[qt->contexts.used];
    memset(qc, 0, sizeof(*qc));
    qc->slot = qt->contexts.used++;
    qc->rca =  rrdcontext_acquired_dup(rca);

    return qc;
}

static ssize_t query_context_add(void *data, RRDCONTEXT_ACQUIRED *rca, bool queryable_context) {
    QUERY_TARGET_LOCALS *qtl = data;

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    if(rrd_flag_is_deleted(rc))
        return 0;

    QUERY_NODE *qn = qtl->qn;
    QUERY_TARGET *qt = qtl->qt;
    QUERY_CONTEXT *qc = query_context_allocate(qt, rca);

    ssize_t added = 0;
    if(unlikely(qt->request.ria)) {
        if(query_instance_add(qtl, qn, qc, qt->request.ria, queryable_context, false))
            added++;
    }
    else if(unlikely(qtl->st && qtl->st->rrdcontexts.rrdcontext == rca && qtl->st->rrdcontexts.rrdinstance)) {
        if(query_instance_add(qtl, qn, qc, qtl->st->rrdcontexts.rrdinstance, queryable_context, false))
            added++;
    }
    else {
        RRDINSTANCE *ri;
        dfe_start_read(rc->rrdinstances, ri) {
                    if(query_instance_add(qtl, qn, qc, (RRDINSTANCE_ACQUIRED *) ri_dfe.item, queryable_context, true))
                        added++;
                }
        dfe_done(ri);
    }

    if(!added) {
        query_context_release(qc);
        qt->contexts.used--;
        return 0;
    }

    return added;
}

static inline void query_node_release(QUERY_NODE *qn) {
    qn->rrdhost = NULL;
}

static inline QUERY_NODE *query_node_allocate(QUERY_TARGET *qt, RRDHOST *host) {
    if(qt->nodes.used == qt->nodes.size) {
        size_t old_mem = qt->nodes.size * sizeof(*qt->nodes.array);
        qt->nodes.size = query_target_realloc_size(qt->nodes.size, 2);
        size_t new_mem = qt->nodes.size * sizeof(*qt->nodes.array);
        qt->nodes.array = reallocz(qt->nodes.array, new_mem);

        __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
    }
    QUERY_NODE *qn = &qt->nodes.array[qt->nodes.used];
    memset(qn, 0, sizeof(*qn));

    qn->slot = qt->nodes.used++;
    qn->rrdhost = host;

    return qn;
}

static ssize_t query_node_add(void *data, RRDHOST *host, bool queryable_host) {
    QUERY_TARGET_LOCALS *qtl = data;
    QUERY_TARGET *qt = qtl->qt;
    QUERY_NODE *qn = query_node_allocate(qt, host);

    if(!UUIDiszero(host->node_id)) {
        if(!qtl->host_node_id_str[0])
            uuid_unparse_lower(host->node_id.uuid, qn->node_id);
        else
            memcpy(qn->node_id, qtl->host_node_id_str, sizeof(qn->node_id));
    }
    else
        qn->node_id[0] = '\0';

    // is the chart given valid?
    if(unlikely(qtl->st && (!qtl->st->rrdcontexts.rrdinstance || !qtl->st->rrdcontexts.rrdcontext))) {
        netdata_log_error("QUERY TARGET: RRDSET '%s' given, but it is not linked to rrdcontext structures. Linking it now.", rrdset_name(qtl->st));
        rrdinstance_from_rrdset(qtl->st);

        if(unlikely(qtl->st && (!qtl->st->rrdcontexts.rrdinstance || !qtl->st->rrdcontexts.rrdcontext))) {
            netdata_log_error("QUERY TARGET: RRDSET '%s' given, but failed to be linked to rrdcontext structures. Switching to context query.",
                              rrdset_name(qtl->st));

            if (!is_valid_sp(qtl->instances))
                qtl->instances = rrdset_name(qtl->st);

            qtl->st = NULL;
        }
    }

    qtl->qn = qn;

    ssize_t added = 0;
    if(unlikely(qt->request.rca)) {
        if(query_context_add(qtl, qt->request.rca, true))
            added++;
    }
    else if(unlikely(qtl->st)) {
        // single chart data queries
        if(query_context_add(qtl, qtl->st->rrdcontexts.rrdcontext, true))
            added++;
    }
    else {
        // context pattern queries
        added = query_scope_foreach_context(
                host, qtl->scope_contexts,
                qt->contexts.scope_pattern, qt->contexts.pattern,
                query_context_add, queryable_host, qtl);

        if(added < 0)
            added = 0;
    }

    qtl->qn = NULL;

    if(!added) {
        query_node_release(qn);
        qt->nodes.used--;
        return false;
    }

    return true;
}

void query_target_generate_name(QUERY_TARGET *qt) {
    char options_buffer[100 + 1];
    web_client_api_request_data_vX_options_to_string(options_buffer, 100, qt->request.options);

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
    else if(qt->request.version >= 2)
        snprintfz(qt->id, MAX_QUERY_TARGET_ID_LENGTH, "data_v2://scope_nodes:%s/scope_contexts:%s/nodes:%s/contexts:%s/instances:%s/labels:%s/dimensions:%s/after:%lld/before:%lld/points:%zu/time_group:%s%s/options:%s%s%s"
                , qt->request.scope_nodes ? qt->request.scope_nodes : "*"
                , qt->request.scope_contexts ? qt->request.scope_contexts : "*"
                , qt->request.nodes ? qt->request.nodes : "*"
                , (qt->request.contexts) ? qt->request.contexts : "*"
                , (qt->request.instances) ? qt->request.instances : "*"
                , (qt->request.labels) ? qt->request.labels : "*"
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
    //if(!service_running(ABILITY_DATA_QUERIES))
    //    return NULL;

    QUERY_TARGET *qt = query_target_get();

    if(!qtr->received_ut)
        qtr->received_ut = now_monotonic_usec();

    qt->timings.received_ut = qtr->received_ut;

    if(qtr->nodes && !qtr->scope_nodes)
        qtr->scope_nodes = qtr->nodes;

    if(qtr->contexts && !qtr->scope_contexts)
        qtr->scope_contexts = qtr->contexts;

    memset(&qt->db, 0, sizeof(qt->db));
    qt->query_points = STORAGE_POINT_UNSET;

    // copy the request into query_thread_target
    qt->request = *qtr;

    query_target_generate_name(qt);
    qt->window.after = qt->request.after;
    qt->window.before = qt->request.before;

    qt->window.options = qt->request.options;
    if(query_target_has_percentage_of_group(qt))
        qt->window.options &= ~RRDR_OPTION_PERCENTAGE;

    qt->internal.relative = rrdr_relative_window_to_absolute_query(&qt->window.after, &qt->window.before
                                                                   , &qt->window.now, unittest_running
                                                                  );

    // prepare our local variables - we need these across all these functions
    QUERY_TARGET_LOCALS qtl = {
            .qt = qt,
            .start_s = now_realtime_sec(),
            .st = qt->request.st,
            .scope_nodes = qt->request.scope_nodes,
            .scope_contexts = qt->request.scope_contexts,
            .nodes = qt->request.nodes,
            .contexts = qt->request.contexts,
            .instances = qt->request.instances,
            .dimensions = qt->request.dimensions,
            .chart_label_key = qt->request.chart_label_key,
            .labels = qt->request.labels,
            .alerts = qt->request.alerts,
    };

    RRDHOST *host = qt->request.host;

    // prepare all the patterns
    qt->nodes.scope_pattern = string_to_simple_pattern(qtl.scope_nodes);
    qt->nodes.pattern = string_to_simple_pattern(qtl.nodes);

    qt->contexts.pattern = string_to_simple_pattern(qtl.contexts);
    qt->contexts.scope_pattern = string_to_simple_pattern(qtl.scope_contexts);

    qt->instances.pattern = string_to_simple_pattern(qtl.instances);
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
            netdata_log_error("QUERY TARGET: RRDSET '%s' given does not belong to host '%s'. Switching query host to '%s'",
                  rrdset_name(qtl.st), rrdhost_hostname(host), rrdhost_hostname(qtl.st->rrdhost));
            host = qtl.st->rrdhost;
        }
    }

    if(host) {
        if(!UUIDiszero(host->node_id))
            uuid_unparse_lower(host->node_id.uuid, qtl.host_node_id_str);
        else
            qtl.host_node_id_str[0] = '\0';

        // single host query
        qt->versions.contexts_hard_hash = dictionary_version(host->rrdctx.contexts);
        qt->versions.contexts_soft_hash = dictionary_version(host->rrdctx.hub_queue);
        qt->versions.alerts_hard_hash = dictionary_version(host->rrdcalc_root_index);
        qt->versions.alerts_soft_hash = __atomic_load_n(&host->health_transitions, __ATOMIC_RELAXED);
        query_node_add(&qtl, host, true);
        qtl.nodes = rrdhost_hostname(host);
    }
    else
        query_scope_foreach_host(qt->nodes.scope_pattern, qt->nodes.pattern,
                                 query_node_add, &qtl,
                                 &qt->versions,
                                 qtl.host_node_id_str);

    // we need the available db retention for this call
    // so it has to be done last
    query_target_calculate_window(qt);

    qt->timings.preprocessed_ut = now_monotonic_usec();

    return qt;
}

ssize_t weights_foreach_rrdmetric_in_context(RRDCONTEXT_ACQUIRED *rca,
                                            SIMPLE_PATTERN *instances_sp,
                                            SIMPLE_PATTERN *chart_label_key_sp,
                                            SIMPLE_PATTERN *labels_sp,
                                            SIMPLE_PATTERN *alerts_sp,
                                            SIMPLE_PATTERN *dimensions_sp,
                                            bool match_ids, bool match_names,
                                            size_t version,
                                            weights_add_metric_t cb,
                                            void *data
                                           ) {
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    if(!rc || rrd_flag_is_deleted(rc))
        return 0;

    char host_node_id_str[UUID_STR_LEN] = "";

    bool proceed = true;

    ssize_t count = 0;
    RRDINSTANCE *ri;
    dfe_start_read(rc->rrdinstances, ri) {
                if(rrd_flag_is_deleted(ri))
                    continue;

                RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *) ri_dfe.item;

                if(instances_sp) {
                    QUERY_INSTANCE qi = { .ria = ria, };
                    SIMPLE_PATTERN_RESULT ret = query_instance_matches(&qi, ri, instances_sp, match_ids, match_names, version, host_node_id_str);
                    qi.ria = NULL;
                    query_instance_release(&qi);

                    if (ret != SP_MATCHED_POSITIVE)
                        continue;
                }

                if(!query_instance_matches_labels(ri, chart_label_key_sp, labels_sp))
                    continue;

                if(alerts_sp && !query_target_match_alert_pattern(ria, alerts_sp))
                    continue;

                dfe_unlock(ri);

                RRDMETRIC *rm;
                dfe_start_read(ri->rrdmetrics, rm) {
                            if(rrd_flag_is_deleted(rm))
                                continue;

                            if(dimensions_sp) {
                                SIMPLE_PATTERN_RESULT ret = SP_NOT_MATCHED;

                                if (match_ids)
                                    ret = simple_pattern_matches_string_extract(dimensions_sp, rm->id, NULL, 0);

                                if (ret == SP_NOT_MATCHED && match_names && (rm->name != rm->id || !match_ids))
                                    ret = simple_pattern_matches_string_extract(dimensions_sp, rm->name, NULL, 0);

                                if(ret != SP_MATCHED_POSITIVE)
                                    continue;
                            }

                            dfe_unlock(rm);

                            RRDMETRIC_ACQUIRED *rma = (RRDMETRIC_ACQUIRED *)rm_dfe.item;
                            ssize_t ret = cb(data, rc->rrdhost, rca, ria, rma);

                            if(ret < 0) {
                                proceed = false;
                                break;
                            }

                            count += ret;
                        }
                dfe_done(rm);

                if(unlikely(!proceed))
                    break;
            }
    dfe_done(ri);
    return count;
}
