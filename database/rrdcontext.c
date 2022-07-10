// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext.h"
#include "sqlite/sqlite_context.h"

typedef struct rrdmetric RRDMETRIC;
typedef struct rrdinstance RRDINSTANCE;
typedef struct rrdcontext RRDCONTEXT;
typedef struct rrdmetric_dictionary RRDMETRICS;

static void rrdinstance_updated(RRDINSTANCE *ri);
static void rrdcontext_updated(RRDCONTEXT *rc);

static inline RRDCONTEXT *rrdcontext_acquired_value(RRDHOST *host, RRDCONTEXT_ACQUIRED *item);

typedef enum {
    RRDMETRIC_FLAG_NONE     = 0,
    RRDMETRIC_FLAG_ARCHIVED = (1 << 0),
} RRDMETRIC_FLAGS;

struct rrdmetric {
    uuid_t uuid;

    STRING *id;
    STRING *name;

    int update_every;
    RRDDIM *rd;

    RRDMETRIC_FLAGS flags;

    RRDINSTANCE_ACQUIRED *rrdinstance;
};

typedef enum {
    RRDINSTANCE_FLAG_NONE       = 0,
    RRDINSTANCE_FLAG_DELETED    = (1 << 0),
    RRDINSTANCE_FLAG_COLLECTED  = (1 << 1),
    RRDINSTANCE_FLAG_UPDATED    = (1 << 2),
    RRDINSTANCE_FLAG_OWN_LABELS = (1 << 3),
} RRDINSTANCE_FLAGS;

struct rrdinstance {
    uuid_t uuid;

    STRING *id;
    STRING *name;
    STRING *title;
    STRING *units;
    size_t priority;

    int update_every;                   // data collection frequency
    RRDSET *rrdset;                     // pointer to RRDSET when collected, or NULL

    RRDINSTANCE_FLAGS flags;            // flags related to this instance
    RRDCONTEXT_ACQUIRED *rca;           // acquired rrdcontext item, or NULL
    RRDMETRICS *rrdmetrics;

    size_t rrdlabels_version;            // the version of rrdlabels the last time we checked
    DICTIONARY *rrdlabels;

    RRDHOST *rrdhost;
};

// ----------------------------------------------------------------------------
// RRDMETRIC

void rrdmetric_insert_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDINSTANCE *ri = (RRDINSTANCE *)data;
    RRDMETRIC *rm = (RRDMETRIC *)value;
    (void)ri;
    (void)rm;

    // TODO what other initializations needed?
}

void rrdmetric_delete_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDINSTANCE *ri = (RRDINSTANCE *)data;
    (void)ri;

    RRDMETRIC *rm = (RRDMETRIC *)value;
    string_freez(rm->id);
    string_freez(rm->name);
}

void rrdmetric_conflict_callback(const char *id, void *oldv, void *newv, void *data) {
    (void)id;
    RRDINSTANCE *ri = (RRDINSTANCE *)data;
    (void)ri;

    RRDMETRIC *rm_old = (RRDMETRIC *)oldv;
    RRDMETRIC *rm_new = (RRDMETRIC *)newv;

    // TODO what other needs to be done here?

    string_freez(rm_new->id);
    string_freez(rm_new->name);

    (void)rm_old;
}

void rrdmetrics_create(RRDINSTANCE *ri) {
    if(unlikely(!ri)) return;
    if(likely(ri->rrdmetrics)) return;

    DICTIONARY *dict = dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback(dict, rrdmetric_insert_callback, (void *)ri);
    dictionary_register_delete_callback(dict, rrdmetric_delete_callback, (void *)ri);
    dictionary_register_conflict_callback(dict, rrdmetric_conflict_callback, (void *)ri);
}

void rrdmetrics_destroy(RRDINSTANCE *ri) {
    if(unlikely(!ri || !ri->rrdmetrics)) return;
    dictionary_destroy((DICTIONARY *)ri->rrdmetrics);
    ri->rrdmetrics = NULL;
}

// ----------------------------------------------------------------------------
// RRDINSTANCE

static void rrdinstance_free(RRDINSTANCE *ri) {
    rrdmetrics_destroy(ri);

    if(ri->flags & RRDINSTANCE_FLAG_OWN_LABELS)
        dictionary_destroy(ri->rrdlabels);

    string_freez(ri->id);
    string_freez(ri->name);
    string_freez(ri->title);
    string_freez(ri->units);
}

void rrdinstance_insert_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    RRDINSTANCE *ri = (RRDINSTANCE *)value;

    ri->rrdhost = host;

    if(ri->rrdset && ri->rrdset->state) {
        ri->rrdlabels = ri->rrdset->state->chart_labels;
        if(ri->flags & RRDINSTANCE_FLAG_OWN_LABELS)
            ri->flags &= ~RRDINSTANCE_FLAG_OWN_LABELS;
    }
    else {
        ri->rrdlabels = rrdlabels_create();
        ri->flags |= RRDINSTANCE_FLAG_OWN_LABELS;
    }
    ri->rrdlabels_version = dictionary_stats_version(ri->rrdlabels);

    rrdmetrics_create(ri);

    ri->flags |= RRDINSTANCE_FLAG_UPDATED;
    rrdinstance_updated(ri);
}

void rrdinstance_delete_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data; (void)host;

    RRDINSTANCE *ri = (RRDINSTANCE *)value;
    rrdinstance_free(ri);
}

void rrdinstance_conflict_callback(const char *id, void *oldv, void *newv, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data; (void)host;

    RRDINSTANCE *ri     = (RRDINSTANCE *)oldv;
    RRDINSTANCE *ri_new = (RRDINSTANCE *)newv;

    bool changed = false;

    if(uuid_compare(ri->uuid, ri_new->uuid) != 0) {
        uuid_copy(ri->uuid, ri_new->uuid);
        changed = true;
    }

    if(ri->id != ri_new->id) {
        STRING *old = ri->id;
        ri->id = string_dupz(string2str(ri_new->id));
        string_freez(old);
        changed = true;
    }

    if(ri->name != ri_new->name) {
        STRING *old = ri->name;
        ri->name = string_dupz(string2str(ri_new->name));
        string_freez(old);
        changed = true;
    }

    if(ri->title != ri_new->title) {
        STRING *old = ri->title;
        ri->title = string_dupz(string2str(ri_new->title));
        string_freez(old);
        changed = true;
    }

    if(ri->units != ri_new->units) {
        STRING *old = ri->units;
        ri->units = string_dupz(string2str(ri_new->units));
        string_freez(old);
        changed = true;
    }

    if(ri->priority != ri_new->priority) {
        ri->priority = ri_new->priority;
        changed = true;
    }

    if(ri->update_every != ri_new->update_every) {
        ri->update_every = ri_new->update_every;
        changed = true;
    }

    if(ri->rrdset != ri_new->rrdset) {
        ri->rrdset = ri_new->rrdset;
        changed = true;

        if(ri->flags & RRDINSTANCE_FLAG_OWN_LABELS) {
            DICTIONARY *old = ri->rrdlabels;
            ri->rrdlabels = ri->rrdset->state->chart_labels;
            rrdlabels_destroy(old);
            ri->flags &= ~RRDINSTANCE_FLAG_OWN_LABELS;
        }
    }

    if(ri->rrdlabels_version != dictionary_stats_version(ri->rrdlabels)) {
        ri->rrdlabels_version = dictionary_stats_version(ri->rrdlabels);
        changed = true;
    }

    if(changed) {
        ri->flags |= RRDINSTANCE_FLAG_UPDATED;
        rrdinstance_updated(ri);
    }

    // TODO all these STRINGs in ri_new should not be freed if they are STRINGs in RRDSET
    // instead we should string_dupz() them in `ri`
    rrdinstance_free(ri_new);
}

void rrdinstances_create(RRDHOST *host) {
    if(unlikely(!host)) return;
    if(likely(host->rrdinstances)) return;
    host->rrdinstances = (RRDINSTANCES *)dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback((DICTIONARY *)host->rrdinstances, rrdinstance_insert_callback, (void *)host);
    dictionary_register_delete_callback((DICTIONARY *)host->rrdinstances, rrdinstance_delete_callback, (void *)host);
    dictionary_register_conflict_callback((DICTIONARY *)host->rrdinstances, rrdinstance_conflict_callback, (void *)host);
}

void rrdinstances_destroy(RRDHOST *host) {
    if(unlikely(!host)) return;
    if(likely(host->rrdinstances)) return;

    dictionary_destroy((DICTIONARY *)host->rrdinstances);
    host->rrdinstances = NULL;
}

static inline void rrdinstance_release(RRDHOST *host, RRDINSTANCE_ACQUIRED *rca) {
    dictionary_acquired_item_release((DICTIONARY *)host->rrdinstances, (DICTIONARY_ITEM *)rca);
}

static inline RRDINSTANCE *rrdinstance_acquired_value(RRDHOST *host, RRDINSTANCE_ACQUIRED *ria) {
    return dictionary_acquired_item_value((DICTIONARY *)host->rrdinstances, (DICTIONARY_ITEM *)ria);
}

static void rrdinstance_updated(RRDINSTANCE *ri) {
    if(ri->rca)
        fatal("RRDINSTANCE: '%s' does not have an RRDCONTEXT_ACQUIRED", string2str(ri->id));

    rrdcontext_updated(rrdcontext_acquired_value(ri->rrdhost, ri->rca));
}

static inline RRDINSTANCE_ACQUIRED *rrdinstance_from_rrdset(RRDSET *st, RRDCONTEXT_ACQUIRED *rca) {
    rrdinstances_create(st->rrdhost);

    DICTIONARY *rrdinstances = (DICTIONARY *)st->rrdhost->rrdinstances;
    RRDINSTANCE tmp = {
        // TODO all these STRINGs should already be STRINGs in RRDSET
        .id = string_dupz(st->id),
        .name = string_dupz(st->name),
        .units = string_dupz(st->units),
        .title = string_dupz(st->title),
        .priority = st->priority,
        .update_every = st->update_every,
        .rca = rca,
        .rrdset = st,
    };
    uuid_copy(tmp.uuid, *st->chart_uuid);
    return (RRDINSTANCE_ACQUIRED *)dictionary_set_and_acquire_item(rrdinstances, string2str(tmp.id), &tmp, sizeof(tmp));
}

static inline void rrdinstance_removed_rrdset(RRDSET *st) {
    if(unlikely(!st->rrdinstance))
        fatal("RRDINSTANCE: chart '%s' is not linked to an RRDINSTANCE", st->id);

    RRDINSTANCE *ri = rrdinstance_acquired_value(st->rrdhost, st->rrdinstance);

    if(unlikely(ri->rrdset != st))
        fatal("RRDINSTANCE: instance '%s' is not linked to chart '%s'", string2str(ri->id), st->id);

    ri->flags |= RRDINSTANCE_FLAG_DELETED|RRDINSTANCE_FLAG_UPDATED;

    if(ri->flags & RRDINSTANCE_FLAG_COLLECTED)
        ri->flags &= ~RRDINSTANCE_FLAG_COLLECTED;

    if(!(ri->flags & RRDINSTANCE_FLAG_OWN_LABELS)) {
        ri->flags &= ~RRDINSTANCE_FLAG_OWN_LABELS;
        ri->rrdlabels = rrdlabels_create();
        rrdlabels_copy(ri->rrdlabels, st->state->chart_labels);
    }
    ri->rrdlabels_version = dictionary_stats_version(ri->rrdlabels);

    rrdinstance_updated(ri);

    rrdinstance_release(st->rrdhost, st->rrdinstance);
    st->rrdinstance = NULL;
}

static inline void rrdinstance_updated_rrdset_name(RRDSET *st) {
    (void)st;
    ;
}

static inline void rrdinstance_updated_rrdset_flags(RRDSET *st) {
    (void)st;
    ;
}

static inline void rrdinstance_collected_rrdset(RRDSET *st) {
    (void)st;
    ;
}


// ----------------------------------------------------------------------------
// RRDCONTEXT

typedef enum {
    RRDCONTEXT_FLAG_NONE      = 0,
    RRDCONTEXT_FLAG_DELETED   = (1 << 0),
    RRDCONTEXT_FLAG_COLLECTED = (1 << 1),
    RRDCONTEXT_FLAG_UPDATED   = (1 << 2),
} RRDCONTEXT_FLAGS;

struct rrdcontext {
    uint64_t version;

    STRING *id;
    STRING *title;
    STRING *units;
    RRDSET_TYPE chart_type;

    size_t priority;

    time_t first_time_t;
    time_t last_time_t;
    RRDCONTEXT_FLAGS flags;

    VERSIONED_CONTEXT_DATA hub;

    RRDINSTANCES *rrdinstances;
    RRDHOST *rrdhost;
};

static void rrdcontext_freez(RRDCONTEXT *rc) {
    string_freez(rc->id);
    string_freez(rc->title);
    string_freez(rc->units);
    dictionary_destroy((DICTIONARY *)rc->rrdinstances);
}

static void check_if_we_need_to_emit_new_version(RRDCONTEXT *rc) {
    VERSIONED_CONTEXT_DATA tmp = {
        .version = rc->version,
        .id = string2str(rc->id),
        .title = string2str(rc->title),
        .units = string2str(rc->units),
        .chart_type = rrdset_type_name(rc->chart_type),
        .priority = rc->priority,
        .last_time_t = (rc->flags & RRDCONTEXT_FLAG_COLLECTED)? 0 : rc->last_time_t,
        .first_time_t = rc->first_time_t,
        .deleted = (rc->flags & RRDCONTEXT_FLAG_DELETED) ? true : false,
    };

    // it is ok to memcmp() because our strings are deduplicated
    // so the pointer of the same string has the same value
    if(memcmp(&tmp, &rc->hub, sizeof(tmp)) != 0) {
        rc->version = tmp.version = (rc->version > rc->hub.version ? rc->version : rc->hub.version) + 1;
        memcpy(&rc->hub, &tmp, sizeof(tmp));
        // TODO save to SQL and send to cloud
    }
}

void rrdcontext_insert_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    RRDCONTEXT *rc = (RRDCONTEXT *)value;

    rc->rrdhost = host;

    if(rc->hub.version) {
        // we are loading data from the SQL database

        if(rc->version)
            error("RRDCONTEXT: context '%s' is already initialized with version %lu, but it is loaded again from SQL with version %lu", string2str(rc->id), rc->version, rc->hub.version);

        // IMPORTANT
        // replace all string pointers in rc->hub with our own versions
        // the originals are coming from a tmp allocation of sqlite

        string_freez(rc->id);
        rc->id = string_dupz(rc->hub.id);
        rc->hub.id = string2str(rc->id);

        string_freez(rc->title);
        rc->title = string_dupz(rc->hub.title);
        rc->hub.title = string2str(rc->title);

        string_freez(rc->units);
        rc->units = string_dupz(rc->hub.units);
        rc->hub.units = string2str(rc->units);

        rc->chart_type = rrdset_type_id(rc->hub.chart_type);
        rc->hub.chart_type = rrdset_type_name(rc->chart_type);

        rc->version      = rc->hub.version;
        rc->priority     = rc->hub.priority;
        rc->first_time_t = rc->hub.first_time_t;
        rc->last_time_t  = rc->hub.last_time_t;

        if(rc->hub.deleted)
            rc->flags |= RRDCONTEXT_FLAG_DELETED;
    }
    else {
        // we are adding this context now for the first time
        rc->version = now_realtime_sec();
    }

    internal_error(true, "RRDCONTEXT: added context '%s' on host '%s', version %lu, title '%s', units '%s', chart type '%s' priority %zu",
                   string2str(rc->id), host->hostname, rc->version, string2str(rc->title), string2str(rc->units),
                   rrdset_type_name(rc->chart_type), rc->priority);

    check_if_we_need_to_emit_new_version(rc);
}

void rrdcontext_delete_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    (void)host;

    RRDCONTEXT *rc = (RRDCONTEXT *)value;
    rrdcontext_freez(rc);
}

static STRING *merge_titles(STRING *a, STRING *b) {
    size_t alen = string_length(a);
    size_t blen = string_length(b);
    size_t length = MAX(alen, blen);
    char buf1[length + 1], buf2[length + 1], *dst1, *dst2;
    const char *s1, *s2;

    s1 = string2str(a);
    s2 = string2str(b);
    dst1 = buf1;
    for( ; *s1 && *s2 && *s1 == *s2 ;s1++, s2++)
        *dst1++ = *s1;

    *dst1 = '\0';

    if(*s1 != '\0' || *s2 != '\0') {
        *dst1++ = 'X';

        s1 = &(string2str(a))[alen - 1];
        s2 = &(string2str(b))[blen - 1];
        dst2 = &buf2[length];
        *dst2 = '\0';
        for (; *s1 && *s2 && *s1 == *s2; s1--, s2--)
            *(--dst2) = *s1;

        strcpy(dst1, dst2);
    }

    internal_error(true, "RRDCONTEXT: merged title '%s' and title '%s' as '%s'", string2str(a), string2str(b), buf1);
    return string_dupz(buf1);
}

void rrdcontext_conflict_callback(const char *id, void *oldv, void *newv, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    (void)host;

    RRDCONTEXT *rc = (RRDCONTEXT *)oldv;
    RRDCONTEXT *rc_new = (RRDCONTEXT *)newv;

    bool changed = false;

    if(rc->title != rc_new->title) {
        STRING *old_title = rc->title;
        rc->title = merge_titles(rc->title, rc_new->title);
        string_freez(old_title);
        changed = true;
    }

    if(rc->units != rc_new->units) {
        STRING *old_units = rc->units;
        rc->units = string_dupz(string2str(rc_new->units));
        string_freez(old_units);
        changed = true;
    }

    if(rc->chart_type != rc_new->chart_type) {
        rc->chart_type = rc_new->chart_type;
        changed = true;
    }

    if(rc->priority != rc_new->priority) {
        rc->priority = rc_new->priority;
        changed = true;
    }

    // free the resources of the new one
    rrdcontext_freez(rc_new);

    // update the cloud if necessary
    if(changed)
        check_if_we_need_to_emit_new_version(rc);
}

void rrdcontexts_create(RRDHOST *host) {
    if(likely(host->rrdcontexts)) return;
    host->rrdcontexts = (RRDCONTEXTS *)dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback((DICTIONARY *)host->rrdcontexts, rrdcontext_insert_callback, (void *)host);
    dictionary_register_delete_callback((DICTIONARY *)host->rrdcontexts, rrdcontext_delete_callback, (void *)host);
    dictionary_register_conflict_callback((DICTIONARY *)host->rrdcontexts, rrdcontext_conflict_callback, (void *)host);
}

void rrdcontexts_destroy(RRDHOST *host) {
    if(unlikely(!host)) return;
    if(likely(host->rrdcontexts)) return;

    dictionary_destroy((DICTIONARY *)host->rrdcontexts);
    host->rrdcontexts = NULL;
}

void rrdcontext_updated(RRDCONTEXT *rc) {
    ;
}

static inline const char *rrdcontext_acquired_name(RRDHOST *host, RRDCONTEXT_ACQUIRED *item) {
    return dictionary_acquired_item_name((DICTIONARY *)host->rrdcontexts, (DICTIONARY_ITEM *)item);
}

static inline RRDCONTEXT *rrdcontext_acquired_value(RRDHOST *host, RRDCONTEXT_ACQUIRED *item) {
    return dictionary_acquired_item_value((DICTIONARY *)host->rrdcontexts, (DICTIONARY_ITEM *)item);
}

static inline RRDCONTEXT_ACQUIRED *rrdcontext_acquire(RRDHOST *host, const char *name) {
    return (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item((DICTIONARY *)host->rrdcontexts, name);
}

static inline void rrdcontext_release(RRDHOST *host, RRDCONTEXT_ACQUIRED *context) {
    dictionary_acquired_item_release((DICTIONARY *)host->rrdcontexts, (DICTIONARY_ITEM *)context);
}

static RRDCONTEXT_ACQUIRED *rrdcontext_from_rrdset(RRDSET *st) {
    RRDCONTEXT tmp = {
        .id = string_dupz(st->context),
        .title = string_dupz(st->title),
        .units = string_dupz(st->units),
        .priority = st->priority,
        .chart_type = st->chart_type,
        .flags = RRDCONTEXT_FLAG_NONE,
        .rrdhost = st->rrdhost,
        .first_time_t = 0,
        .last_time_t = 0,
        .rrdinstances = NULL,
    };

    rrdcontexts_create(tmp.rrdhost);
    return (RRDCONTEXT_ACQUIRED *)dictionary_set_and_acquire_item((DICTIONARY *)tmp.rrdhost->rrdcontexts, string2str(tmp.id), &tmp, sizeof(tmp));
}


// ----------------------------------------------------------------------------
// helpers


// ----------------------------------------------------------------------------
// public API

void rrdcontext_updated_rrddim(RRDDIM *rd) {
    (void)rd;
    ;
}

void rrdcontext_removed_rrddim(RRDDIM *rd) {
    (void)rd;
    ;
}

void rrdcontext_updated_rrddim_algorithm(RRDDIM *rd) {
    (void)rd;
    ;
}

void rrdcontext_updated_rrddim_multiplier(RRDDIM *rd) {
    (void)rd;
    ;
}

void rrdcontext_updated_rrddim_divisor(RRDDIM *rd) {
    (void)rd;
    ;
}

void rrdcontext_updated_rrddim_flags(RRDDIM *rd) {
    (void)rd;
    ;
}

void rrdcontext_collected_rrddim(RRDDIM *rd) {
    (void)rd;
    ;
}

void rrdcontext_updated_rrdset(RRDSET *st) {
    RRDCONTEXT_ACQUIRED *rca = rrdcontext_from_rrdset(st);
    {
        RRDCONTEXT_ACQUIRED *orca = __atomic_exchange_n(&st->rrdcontext, rca, __ATOMIC_SEQ_CST);
        if(orca) rrdcontext_release(st->rrdhost, orca);
    }

    RRDINSTANCE_ACQUIRED *ria = rrdinstance_from_rrdset(st, rca);
    {
        RRDINSTANCE_ACQUIRED *oria = __atomic_exchange_n(&st->rrdinstance, ria, __ATOMIC_SEQ_CST);
        if(oria) rrdinstance_release(st->rrdhost, oria);
    }
}

void rrdcontext_removed_rrdset(RRDSET *st) {
    rrdinstance_removed_rrdset(st);
}

void rrdcontext_updated_rrdset_name(RRDSET *st) {
    rrdinstance_updated_rrdset_name(st);
}

void rrdcontext_updated_rrdset_flags(RRDSET *st) {
    rrdinstance_updated_rrdset_flags(st);
}

void rrdcontext_collected_rrdset(RRDSET *st) {
    rrdinstance_collected_rrdset(st);
}

// ----------------------------------------------------------------------------
// load from SQL

static void rrdinstance_load_chart_callback(SQL_CHART_DATA *sc, void *data) {
    RRDHOST *host = data;
    (void)host;

    RRDINSTANCE tmp = {
        .id = string_dupz(sc->id),
        .name = string_dupz(sc->name),
        .title = string_dupz(sc->title),
        .units = string_dupz(sc->units),
        .rca = rrdcontext_acquire(host, sc->context),
    };
    uuid_copy(tmp.uuid, sc->chart_id);
    DICTIONARY_ITEM *item = dictionary_set_and_acquire_item((DICTIONARY *)host->rrdinstances, sc->id, &tmp, sizeof(tmp));
    RRDINSTANCE *ri = dictionary_acquired_item_value((DICTIONARY *)host->rrdinstances, item);

    //ctx_get_label_list(ri->uuid, dict_ctx_get_label_list_cb, NULL);
    //ctx_get_dimension_list(ri->uuid, dict_ctx_get_dimension_list_cb, NULL);
}

static void rrdcontext_load_context_callback(VERSIONED_CONTEXT_DATA *ctx_data, void *data) {
    RRDHOST *host = data;
    (void)host;

    RRDCONTEXT tmp = {
        .id = string_dupz(ctx_data->id),
        // no need to set more data here
        // we only need the hub data
    };
    memcpy(&tmp.hub, ctx_data, sizeof(VERSIONED_CONTEXT_DATA));
    dictionary_set((DICTIONARY *)host->rrdcontexts, ctx_data->id, &tmp, sizeof(tmp));
}

void rrdcontext_load_all(RRDHOST *host) {
    rrdinstances_create(host);
    rrdcontexts_create(host);

    ctx_get_context_list(&host->host_uuid, rrdcontext_load_context_callback, host);
    ctx_get_chart_list(&host->host_uuid, rrdinstance_load_chart_callback, host);
}
