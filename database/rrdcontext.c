// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext.h"
#include "sqlite/sqlite_context.h"

typedef struct rrdmetric RRDMETRIC;
typedef struct rrdinstance RRDINSTANCE;
typedef struct rrdcontext RRDCONTEXT;
typedef struct rrdmetric_dictionary RRDMETRICS;

static void rrdinstance_has_been_updated(RRDINSTANCE *ri);
static void rrdcontext_has_been_updated(RRDCONTEXT *rc);

static void rrdcontext_release(RRDHOST *host, RRDCONTEXT_ACQUIRED *rca);
static void rrdcontext_remove_rrdinstance(RRDINSTANCE *ri);
static void rrdcontext_from_rrdinstance(RRDINSTANCE *ri);
static RRDCONTEXT_ACQUIRED *rrdcontext_from_rrdset(RRDSET *st);
static inline RRDCONTEXT *rrdcontext_acquired_value(RRDHOST *host, RRDCONTEXT_ACQUIRED *rca);

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
    STRING *context;
    STRING *title;
    STRING *units;
    size_t priority;
    RRDSET_TYPE chart_type;

    int update_every;                   // data collection frequency
    RRDSET *rrdset;                     // pointer to RRDSET when collected, or NULL

    RRDINSTANCE_FLAGS flags;            // flags related to this instance

    RRDCONTEXT_ACQUIRED *rca;           // acquired rrdcontext item, or NULL
    RRDMETRICS *rrdmetrics;

    size_t rrdlabels_version;            // the version of rrdlabels the last time we checked
    DICTIONARY *rrdlabels;

    RRDHOST *rrdhost;

    netdata_rwlock_t rwlock;
};

static void rrdinstance_log(RRDINSTANCE *ri, const char *msg) {
    char uuid[UUID_STR_LEN + 1];

    uuid_unparse(ri->uuid, uuid);

    BUFFER *wb = buffer_create(1000);

    buffer_sprintf(wb,
                   "RRDINSTANCE: %s id '%s' (host '%s'), uuid '%s', name '%s', context '%s', title '%s', units '%s', priority %zu, chart type '%s', update every %d, rrdset '%s', flags %s%s%s%s, rca %s, labels version %zu",
                   msg,
                   string2str(ri->id),
                   ri->rrdhost?ri->rrdhost->hostname:"NONE",
                   uuid,
                   string2str(ri->name),
                   string2str(ri->context),
                   string2str(ri->title),
                   string2str(ri->units),
                   ri->priority,
                   rrdset_type_name(ri->chart_type),
                   ri->update_every,
                   ri->rrdset?ri->rrdset->id:"NONE",
                   ri->flags & RRDINSTANCE_FLAG_DELETED?"DELETED ":"",
                   ri->flags & RRDINSTANCE_FLAG_UPDATED?"UPDATED ":"",
                   ri->flags & RRDINSTANCE_FLAG_COLLECTED?"COLLECTED ":"",
                   ri->flags & RRDINSTANCE_FLAG_OWN_LABELS?"OWNLABELS ":"",
                   ri->rca ? "YES": "NO",
                   ri->rrdlabels_version
                   );

    buffer_strcat(wb, ", labels: { ");
    if(ri->rrdlabels) {
        if(!rrdlabels_to_buffer(ri->rrdlabels, wb, "", "=", "'", ",", NULL, NULL, NULL, NULL))
            buffer_strcat(wb, "NONE }");
        else
            buffer_strcat(wb, " }");
    }
    else
        buffer_strcat(wb, "NONE }");

    buffer_strcat(wb, ", metrics: { ");
    if(ri->rrdmetrics) {
        RRDMETRIC *v;
        int i = 0;
        dfe_start_read((DICTIONARY *)ri->rrdmetrics, v) {
            buffer_sprintf(wb, "%s%s", i?",":"", v_name);
            i++;
        }
        dfe_done(v);
    }
    else
        buffer_strcat(wb, "NONE }");

    info("%s", buffer_tostring(wb));
    buffer_free(wb);
}

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

static void rrdmetrics_create(RRDINSTANCE *ri) {
    if(unlikely(!ri)) return;
    if(likely(ri->rrdmetrics)) return;

    DICTIONARY *dict = dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback(dict, rrdmetric_insert_callback, (void *)ri);
    dictionary_register_delete_callback(dict, rrdmetric_delete_callback, (void *)ri);
    dictionary_register_conflict_callback(dict, rrdmetric_conflict_callback, (void *)ri);
}

static void rrdmetrics_destroy(RRDINSTANCE *ri) {
    if(unlikely(!ri || !ri->rrdmetrics)) return;
    dictionary_destroy((DICTIONARY *)ri->rrdmetrics);
    ri->rrdmetrics = NULL;
}

// ----------------------------------------------------------------------------
// RRDINSTANCE

static void rrdinstance_check(RRDHOST *host, RRDINSTANCE *ri) {
    if(unlikely(!ri->id))
        fatal("RRDINSTANCE: created without an id");

    if(unlikely(!ri->name))
        fatal("RRDINSTANCE: '%s' created without a name", string2str(ri->id));

    if(unlikely(!ri->context))
        fatal("RRDINSTANCE: '%s' created without a context", string2str(ri->id));

    if(unlikely(!ri->title))
        fatal("RRDINSTANCE: '%s' created without a title", string2str(ri->id));

    if(unlikely(!ri->units))
        fatal("RRDINSTANCE: '%s' created without units", string2str(ri->id));

    if(unlikely(!ri->priority))
        fatal("RRDINSTANCE: '%s' created without a priority", string2str(ri->id));

    if(unlikely(!ri->chart_type))
        fatal("RRDINSTANCE: '%s' created without a chart_type", string2str(ri->id));

    if(unlikely(!ri->update_every))
        fatal("RRDINSTANCE: '%s' created without an update_every", string2str(ri->id));

    if(unlikely(ri->rrdhost != host))
        fatal("RRDINSTANCE: '%s' has host '%s', but it should be '%s'", string2str(ri->id), ri->rrdhost->hostname, host->hostname);
}

static void rrdinstance_free(RRDINSTANCE *ri) {
    rrdmetrics_destroy(ri);

    if(ri->rca)
        rrdcontext_release(ri->rrdhost, ri->rca);

    if(ri->flags & RRDINSTANCE_FLAG_OWN_LABELS)
        dictionary_destroy(ri->rrdlabels);

    string_freez(ri->id);
    string_freez(ri->name);
    string_freez(ri->context);
    string_freez(ri->title);
    string_freez(ri->units);

    ri->id = NULL;
    ri->name = NULL;
    ri->context = NULL;
    ri->title = NULL;
    ri->units = NULL;
    ri->rca = NULL;
    ri->rrdlabels = NULL;
    ri->rrdmetrics = NULL;
    ri->rrdset = NULL;
    ri->rrdhost = NULL;
}

void rrdinstance_insert_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    RRDINSTANCE *ri = (RRDINSTANCE *)value;

    rrdinstance_check(host, ri);

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

    netdata_rwlock_init(&ri->rwlock);

    rrdinstance_log(ri, "INSERT");

    rrdcontext_from_rrdinstance(ri);

    ri->flags |= RRDINSTANCE_FLAG_UPDATED;
    rrdinstance_has_been_updated(ri);
}

void rrdinstance_delete_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data; (void)host;
    RRDINSTANCE *ri = (RRDINSTANCE *)value;

    rrdinstance_log(ri, "DELETE");
    rrdinstance_free(ri);
}

void rrdinstance_conflict_callback(const char *id, void *oldv, void *newv, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data; (void)host;

    RRDINSTANCE *ri     = (RRDINSTANCE *)oldv;
    RRDINSTANCE *ri_new = (RRDINSTANCE *)newv;

    rrdinstance_check(host, ri_new);

    netdata_rwlock_wrlock(&ri->rwlock);
    bool changed = false;
    bool update_rrdcontext = false;

    if(uuid_compare(ri->uuid, ri_new->uuid) != 0) {
        uuid_copy(ri->uuid, ri_new->uuid);
        changed = true;
    }

    if(ri->id != ri_new->id)
        fatal("RRDINSTANCE: '%s' cannot change id to '%s'", string2str(ri->id), string2str(ri_new->id));

    if(ri->name != ri_new->name) {
        STRING *old = ri->name;
        ri->name = string_dup(ri_new->name);
        string_freez(old);
        changed = true;
    }

    if(ri->context != ri_new->context) {
        // changed context - remove this instance from the old context
        rrdcontext_remove_rrdinstance(ri);

        STRING *old = ri->context;
        ri->context = string_dup(ri_new->context);
        string_freez(old);
        changed = true;
        update_rrdcontext = true;
    }

    if(ri->title != ri_new->title) {
        STRING *old = ri->title;
        ri->title = string_dup(ri_new->title);
        string_freez(old);
        changed = true;
    }

    if(ri->units != ri_new->units) {
        STRING *old = ri->units;
        ri->units = string_dup(ri_new->units);
        string_freez(old);
        changed = true;
    }

    if(ri->chart_type != ri_new->chart_type) {
        ri->chart_type = ri_new->chart_type;
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

        if(ri->rrdset)
            fatal("RRDINSTANCE: '%s' changed rrdset from '%s' to '%s'!", string2str(ri->id), ri->rrdset->id, ri_new->rrdset->id);

        ri->rrdset = ri_new->rrdset;

        if(ri->flags & RRDINSTANCE_FLAG_OWN_LABELS) {
            DICTIONARY *old = ri->rrdlabels;
            ri->rrdlabels = ri->rrdset->state->chart_labels;
            rrdlabels_destroy(old);
            ri->flags &= ~RRDINSTANCE_FLAG_OWN_LABELS;
        }

        changed = true;
        update_rrdcontext = true;
    }

    if(ri->rrdlabels_version != dictionary_stats_version(ri->rrdlabels)) {
        ri->rrdlabels_version = dictionary_stats_version(ri->rrdlabels);
        changed = true;
    }

    rrdinstance_log(ri, "UPDATE");

    if(update_rrdcontext) {
        rrdcontext_from_rrdinstance(ri);

        // no need to update the context again
        changed = false;
    }

    netdata_rwlock_unlock(&ri->rwlock);

    if(changed) {
        ri->flags |= RRDINSTANCE_FLAG_UPDATED;
        rrdinstance_has_been_updated(ri);
    }

    // free the new one
    rrdinstance_free(ri_new);
}

void rrdhost_create_rrdinstances(RRDHOST *host) {
    if(unlikely(!host)) return;
    if(likely(host->rrdinstances)) return;
    host->rrdinstances = (RRDINSTANCES *)dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback((DICTIONARY *)host->rrdinstances, rrdinstance_insert_callback, (void *)host);
    dictionary_register_delete_callback((DICTIONARY *)host->rrdinstances, rrdinstance_delete_callback, (void *)host);
    dictionary_register_conflict_callback((DICTIONARY *)host->rrdinstances, rrdinstance_conflict_callback, (void *)host);
}

void rrdhost_destroy_rrdinstances(RRDHOST *host) {
    if(unlikely(!host)) return;
    if(likely(!host->rrdinstances)) return;

    dictionary_destroy((DICTIONARY *)host->rrdinstances);
    host->rrdinstances = NULL;
}

static inline void rrdinstance_release(RRDHOST *host, RRDINSTANCE_ACQUIRED *rca) {
    dictionary_acquired_item_release((DICTIONARY *)host->rrdinstances, (DICTIONARY_ITEM *)rca);
}

static inline RRDINSTANCE *rrdinstance_acquired_value(RRDHOST *host, RRDINSTANCE_ACQUIRED *ria) {
    return dictionary_acquired_item_value((DICTIONARY *)host->rrdinstances, (DICTIONARY_ITEM *)ria);
}

static void rrdinstance_has_been_updated(RRDINSTANCE *ri) {
    if(!ri->rca)
        fatal("RRDINSTANCE: '%s' does not have an RRDCONTEXT_ACQUIRED", string2str(ri->id));

    rrdcontext_has_been_updated(rrdcontext_acquired_value(ri->rrdhost, ri->rca));
}

static inline void rrdinstance_from_rrdset(RRDSET *st) {
    rrdhost_create_rrdinstances(st->rrdhost);

    DICTIONARY *rrdinstances = (DICTIONARY *)st->rrdhost->rrdinstances;
    RRDINSTANCE tmp = {
        // TODO all these STRINGs should already be STRINGs in RRDSET
        .id = string_strdupz(st->id),
        .name = string_strdupz(st->name),
        .context = string_strdupz(st->context),
        .units = string_strdupz(st->units),
        .title = string_strdupz(st->title),
        .chart_type = st->chart_type,
        .priority = st->priority,
        .update_every = st->update_every,
        .flags = RRDINSTANCE_FLAG_NONE,
        .rrdset = st,
        .rrdhost = st->rrdhost
    };
    uuid_copy(tmp.uuid, *st->chart_uuid);

    RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *)dictionary_set_and_acquire_item(rrdinstances, string2str(tmp.id), &tmp, sizeof(tmp));

    if(st->rrdinstance && st->rrdinstance != ria)
        fatal("RRDINSTANCE: chart '%s' changed rrdinstance.", st->id);

    st->rrdinstance = ria;
}

static inline void rrdinstance_rrdset_is_freed(RRDSET *st) {
    if(unlikely(!st->rrdinstance))
        fatal("RRDINSTANCE: chart '%s' is not linked to an RRDINSTANCE", st->id);

    RRDINSTANCE *ri = rrdinstance_acquired_value(st->rrdhost, st->rrdinstance);

    if(unlikely(ri->rrdset != st))
        fatal("RRDINSTANCE: instance '%s' is not linked to chart '%s'", string2str(ri->id), st->id);

    netdata_rwlock_wrlock(&ri->rwlock);

    ri->flags |= RRDINSTANCE_FLAG_DELETED|RRDINSTANCE_FLAG_UPDATED;

    if(ri->flags & RRDINSTANCE_FLAG_COLLECTED)
        ri->flags &= ~RRDINSTANCE_FLAG_COLLECTED;

    if(!(ri->flags & RRDINSTANCE_FLAG_OWN_LABELS)) {
        ri->flags &= ~RRDINSTANCE_FLAG_OWN_LABELS;
        ri->rrdlabels = rrdlabels_create();
        rrdlabels_copy(ri->rrdlabels, st->state->chart_labels);
    }
    ri->rrdlabels_version = dictionary_stats_version(ri->rrdlabels);

    netdata_rwlock_unlock(&ri->rwlock);

    rrdinstance_has_been_updated(ri);
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

    DICTIONARY *instances;
    RRDHOST *rrdhost;
};

static void rrdcontext_freez(RRDCONTEXT *rc) {
    string_freez(rc->id);
    string_freez(rc->title);
    string_freez(rc->units);
    dictionary_destroy(rc->instances);
}

static void check_if_we_need_to_emit_new_version(RRDCONTEXT *rc) {
    if(rc->version != rc->hub.version
        || string2str(rc->id) != rc->hub.id
        || string2str(rc->title) != rc->hub.title
        || string2str(rc->units) != rc->hub.units
        || rrdset_type_name(rc->chart_type) != rc->hub.chart_type
        || rc->priority != rc->hub.priority
        || (uint64_t)rc->first_time_t != rc->hub.first_time_t
        || (uint64_t)((rc->flags & RRDCONTEXT_FLAG_COLLECTED) ? 0 : rc->last_time_t) != rc->hub.last_time_t
        || ((rc->flags & RRDCONTEXT_FLAG_DELETED) ? true : false) != rc->hub.deleted
        ) {
        rc->version = rc->hub.version = (rc->version > rc->hub.version ? rc->version : rc->hub.version) + 1;
        rc->hub.id = string2str(rc->id);
        rc->hub.title = string2str(rc->title);
        rc->hub.units = string2str(rc->units);
        rc->hub.chart_type = rrdset_type_name(rc->chart_type);
        rc->hub.priority = rc->priority;
        rc->hub.first_time_t = rc->first_time_t;
        rc->hub.last_time_t = (rc->flags & RRDCONTEXT_FLAG_COLLECTED) ? 0 : rc->last_time_t;
        rc->hub.deleted = (rc->flags & RRDCONTEXT_FLAG_DELETED) ? true : false;

        internal_error(true, "RRDCONTEXT: '%s' updated to version %zu", string2str(rc->id), rc->version);

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
        rc->id = string_strdupz(rc->hub.id);
        rc->hub.id = string2str(rc->id);

        string_freez(rc->title);
        rc->title = string_strdupz(rc->hub.title);
        rc->hub.title = string2str(rc->title);

        string_freez(rc->units);
        rc->units = string_strdupz(rc->hub.units);
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

    rc->instances = dictionary_create(DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE);

    internal_error(true, "RRDCONTEXT: INSERT '%s' on host '%s', version %lu, title '%s', units '%s', chart type '%s', priority %zu, first_time_t %ld, last_time_t %ld, options %s%s%s",
                   string2str(rc->id),
                   host->hostname,
                   rc->version,
                   string2str(rc->title),
                   string2str(rc->units),
                   rrdset_type_name(rc->chart_type),
                   rc->priority,
                   rc->first_time_t,
                   rc->last_time_t,
                   rc->flags & RRDCONTEXT_FLAG_DELETED ? "DELETED ":"",
                   rc->flags & RRDCONTEXT_FLAG_COLLECTED ? "COLLECTED ":"",
                   rc->flags & RRDCONTEXT_FLAG_UPDATED ? "UPDATED ": "");

    check_if_we_need_to_emit_new_version(rc);
}

void rrdcontext_delete_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    (void)host;

    RRDCONTEXT *rc = (RRDCONTEXT *)value;

    internal_error(true, "RRDCONTEXT: DELETE '%s' on host '%s', version %lu, title '%s', units '%s', chart type '%s', priority %zu, first_time_t %ld, last_time_t %ld, options %s%s%s",
                   string2str(rc->id),
                   host->hostname,
                   rc->version,
                   string2str(rc->title),
                   string2str(rc->units),
                   rrdset_type_name(rc->chart_type),
                   rc->priority,
                   rc->first_time_t,
                   rc->last_time_t,
                   rc->flags & RRDCONTEXT_FLAG_DELETED ? "DELETED ":"",
                   rc->flags & RRDCONTEXT_FLAG_COLLECTED ? "COLLECTED ":"",
                   rc->flags & RRDCONTEXT_FLAG_UPDATED ? "UPDATED ": "");

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
    return string_strdupz(buf1);
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
        rc->units = string_dup(rc_new->units);
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

    internal_error(true, "RRDCONTEXT: UPDATE '%s' on host '%s', version %lu, title '%s', units '%s', chart type '%s', priority %zu, first_time_t %ld, last_time_t %ld, options %s%s%s",
                   string2str(rc->id),
                   host->hostname,
                   rc->version,
                   string2str(rc->title),
                   string2str(rc->units),
                   rrdset_type_name(rc->chart_type),
                   rc->priority,
                   rc->first_time_t,
                   rc->last_time_t,
                   rc->flags & RRDCONTEXT_FLAG_DELETED ? "DELETED ":"",
                   rc->flags & RRDCONTEXT_FLAG_COLLECTED ? "COLLECTED ":"",
                   rc->flags & RRDCONTEXT_FLAG_UPDATED ? "UPDATED ": "");

    // free the resources of the new one
    rrdcontext_freez(rc_new);

    // update the cloud if necessary
    if(changed)
        check_if_we_need_to_emit_new_version(rc);
}

void rrdhost_create_rrdcontexts(RRDHOST *host) {
    if(unlikely(!host)) return;
    if(likely(host->rrdcontexts)) return;
    host->rrdcontexts = (RRDCONTEXTS *)dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback((DICTIONARY *)host->rrdcontexts, rrdcontext_insert_callback, (void *)host);
    dictionary_register_delete_callback((DICTIONARY *)host->rrdcontexts, rrdcontext_delete_callback, (void *)host);
    dictionary_register_conflict_callback((DICTIONARY *)host->rrdcontexts, rrdcontext_conflict_callback, (void *)host);
}

void rrdhost_destroy_rrdcontexts(RRDHOST *host) {
    if(unlikely(!host)) return;
    if(likely(!host->rrdcontexts)) return;

    dictionary_destroy((DICTIONARY *)host->rrdcontexts);
    host->rrdcontexts = NULL;
}

static void rrdcontext_has_been_updated(RRDCONTEXT *rc) {
    ;
}

static inline const char *rrdcontext_acquired_name(RRDHOST *host, RRDCONTEXT_ACQUIRED *rca) {
    return dictionary_acquired_item_name((DICTIONARY *)host->rrdcontexts, (DICTIONARY_ITEM *)rca);
}

static inline RRDCONTEXT *rrdcontext_acquired_value(RRDHOST *host, RRDCONTEXT_ACQUIRED *rca) {
    return dictionary_acquired_item_value((DICTIONARY *)host->rrdcontexts, (DICTIONARY_ITEM *)rca);
}

static inline RRDCONTEXT_ACQUIRED *rrdcontext_acquire(RRDHOST *host, const char *name) {
    return (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item((DICTIONARY *)host->rrdcontexts, name);
}

static inline void rrdcontext_release(RRDHOST *host, RRDCONTEXT_ACQUIRED *rca) {
    dictionary_acquired_item_release((DICTIONARY *)host->rrdcontexts, (DICTIONARY_ITEM *)rca);
}

void rrdcontext_remove_rrdinstance(RRDINSTANCE *ri) {
    if(!ri || !ri->rca) return;

    RRDCONTEXT_ACQUIRED *rca = ri->rca;
    RRDCONTEXT *rc = rrdcontext_acquired_value(ri->rrdhost, rca);
    dictionary_del(rc->instances, string2str(ri->context));

    ri->rca = NULL;
    if(ri->rrdset && ri->rrdset->rrdcontext)
        ri->rrdset->rrdcontext = NULL;

    rrdcontext_release(ri->rrdhost, rca);
}

// 1. create an RRDCONTEXT from the supplied RRDINSTANCE
// 2. add the RRDINSTANCE to the instances dictionary of the RRDCONTEXT
// 3. update the RRDINSTANCE to point to this RRDCONTEXT
// 4. update the RRDSET of the RRDINSTANCE to point to this RRDCONTEXT
void rrdcontext_from_rrdinstance(RRDINSTANCE *ri) {
    RRDCONTEXT tmp = {
        .id = string_dup(ri->context),
        .title = string_dup(ri->title),
        .units = string_dup(ri->units),
        .priority = ri->priority,
        .chart_type = ri->chart_type,
        .flags = RRDCONTEXT_FLAG_NONE,
        .rrdhost = ri->rrdhost,
        .first_time_t = 0,
        .last_time_t = 0,
        .instances = NULL,
    };

    rrdhost_create_rrdcontexts(tmp.rrdhost);
    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_set_and_acquire_item((DICTIONARY *)tmp.rrdhost->rrdcontexts, string2str(tmp.id), &tmp, sizeof(tmp));
    if(ri->rca && ri->rca != rca)
        fatal("RRDINSTANCE: '%s' is linked to a different rrdcontext - it should have already be unlinked.", string2str(ri->id));

    if(!ri->rca) {
        ri->rca = rca;

        // put it in the new context
        RRDCONTEXT *rc = rrdcontext_acquired_value(ri->rrdhost, rca);
        dictionary_set(rc->instances, string2str(ri->context), ri, sizeof(*ri));
    }

    if(ri->rrdset) {
        if(ri->rrdset->rrdcontext && ri->rrdset->rrdcontext != rca)
            fatal("RRDCONTEXT: rrdset '%s' is linked to a different rrdcontext. It should have already be unlinked.", ri->rrdset->id);

        if(!ri->rrdset->rrdcontext)
            ri->rrdset->rrdcontext = rca;
    }
}

static RRDCONTEXT_ACQUIRED *rrdcontext_from_rrdset(RRDSET *st) {
    RRDCONTEXT tmp = {
        .id = string_strdupz(st->context),
        .title = string_strdupz(st->title),
        .units = string_strdupz(st->units),
        .priority = st->priority,
        .chart_type = st->chart_type,
        .flags = RRDCONTEXT_FLAG_NONE,
        .rrdhost = st->rrdhost,
        .first_time_t = 0,
        .last_time_t = 0,
        .instances = NULL,
    };

    rrdhost_create_rrdcontexts(tmp.rrdhost);
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
    rrdinstance_from_rrdset(st);
}

void rrdcontext_removed_rrdset(RRDSET *st) {
    rrdinstance_rrdset_is_freed(st);
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
        .id = string_strdupz(sc->id),
        .name = string_strdupz(sc->name),
        .title = string_strdupz(sc->title),
        .units = string_strdupz(sc->units),
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
        .id = string_strdupz(ctx_data->id),
        // no need to set more data here
        // we only need the hub data
    };
    memcpy(&tmp.hub, ctx_data, sizeof(VERSIONED_CONTEXT_DATA));
    dictionary_set((DICTIONARY *)host->rrdcontexts, ctx_data->id, &tmp, sizeof(tmp));
}

void rrdcontext_load_all(RRDHOST *host) {
    rrdhost_create_rrdinstances(host);
    rrdhost_create_rrdcontexts(host);

    ctx_get_context_list(&host->host_uuid, rrdcontext_load_context_callback, host);
    ctx_get_chart_list(&host->host_uuid, rrdinstance_load_chart_callback, host);
}
