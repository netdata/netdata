// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext.h"
#include "sqlite/sqlite_context.h"


// ----------------------------------------------------------------------------
// RRDMETRIC

typedef enum {
    RRDMETRIC_FLAG_NONE     = 0,
    RRDMETRIC_FLAG_ARCHIVED = (1 << 0),
} RRDMETRIC_FLAGS;

struct rrdmetric {
    uuid_t uuid;

    STRING *id;
    STRING *name;

    RRDDIM *rd;

    int update_every;
    struct rrddim_tier *tiers[RRD_STORAGE_TIERS];

    RRDMETRIC_FLAGS flags;

    RRDINSTANCE_ACQUIRED *rrdinstance;
};

struct rrdinstance {
    uuid_t uuid;

    STRING *id;
    STRING *name;
    STRING *title;
    STRING *units;
    int priority;

    int update_every;                   // data collection frequency
    RRDSET *st;                         // pointer to RRDSET when collected, or NULL

    RRDCONTEXT_ACQUIRED *context;       // acquired dictionary item, or NULL

    DICTIONARY *rrdlabels;
    DICTIONARY *rrdmetrics;
};


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

DICTIONARY *rrdmetrics_create(RRDINSTANCE *ri) {
    DICTIONARY *dict = dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback(dict, rrdmetric_insert_callback, (void *)ri);
    dictionary_register_delete_callback(dict, rrdmetric_delete_callback, (void *)ri);
    dictionary_register_conflict_callback(dict, rrdmetric_conflict_callback, (void *)ri);
    return dict;
}

void rrdmetric_add(RRDINSTANCE *ri, const char *id, const char *name) {
    RRDMETRIC tmp = {
        .id = string_dupz(id),
        .name = string_dupz(name),
    };

    dictionary_set(ri->rrdmetrics, id, &tmp, sizeof(RRDMETRIC));
}


// ----------------------------------------------------------------------------
// RRDINSTANCE

void rrdinstance_insert_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    RRDINSTANCE *ri = (RRDINSTANCE *)value;
    (void)host;
    (void)ri;

    // TODO what other initializations needed?

    ri->rrdlabels = rrdlabels_create();
    ri->rrdmetrics = rrdmetrics_create(ri);
}

void rrdinstance_delete_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    (void)host;

    RRDINSTANCE *ri = (RRDINSTANCE *)value;
    dictionary_destroy(ri->rrdlabels);
    dictionary_destroy(ri->rrdmetrics);
    string_freez(ri->id);
    string_freez(ri->name);
}

void rrdinstance_conflict_callback(const char *id, void *oldv, void *newv, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    (void)host;

    RRDINSTANCE *ri_old = (RRDINSTANCE *)oldv;
    RRDINSTANCE *ri_new = (RRDINSTANCE *)newv;

    // TODO what other needs to be done here?

    string_freez(ri_new->id);
    string_freez(ri_new->name);

    (void)ri_old;
}

void rrdinstances_create(RRDHOST *host) {
    if(host->rrdinstances) return;
    host->rrdinstances = dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback(host->rrdinstances, rrdinstance_insert_callback, (void *)host);
    dictionary_register_delete_callback(host->rrdinstances, rrdinstance_delete_callback, (void *)host);
    dictionary_register_conflict_callback(host->rrdinstances, rrdinstance_conflict_callback, (void *)host);
}

void rrdinstance_add(RRDHOST *host, const char *id, const char *name) {
    RRDINSTANCE tmp = {
        .id = string_dupz(id),
        .name = string_dupz(name),
        .rrdlabels = NULL,
        .rrdmetrics = NULL
    };

    dictionary_set(host->rrdinstances, id, &tmp, sizeof(RRDINSTANCE));
}

void rrdinstance_set_label(RRDHOST *host, const char *id, const char *key, const char *value, RRDLABEL_SRC src) {
    DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(host->rrdinstances, id);
    if(!item) {
        error("RRDINSTANCE: cannot find instance '%s' on host '%s' to set label key '%s' to value '%s'", id, host->hostname, key, value);
        return;
    }

    RRDINSTANCE *ri = dictionary_acquired_item_value(host->rrdinstances, item);
    if(!ri->rrdlabels)
        ri->rrdlabels = rrdlabels_create();

    rrdlabels_add(ri->rrdlabels, key, value, src);

    dictionary_acquired_item_release(host->rrdinstances, item);
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

    RRDHOST *host;
    DICTIONARY *rrdinstances;
};

static void rrdcontext_freez(RRDCONTEXT *rc) {
    string_freez(rc->id);
    string_freez(rc->title);
    string_freez(rc->units);
    dictionary_destroy(rc->rrdinstances);
}

static void check_if_we_need_to_update_cloud(RRDCONTEXT *rc) {
    VERSIONED_CONTEXT_DATA tmp = {
        .version = rc->version,
        .id = string2str(rc->id),
        .title = string2str(rc->title),
        .units = string2str(rc->units),
        .chart_type = rrdset_type_name(rc->chart_type),
        .priority = rc->priority,
        .last_time_t = rc->last_time_t,
        .first_time_t = rc->first_time_t,
    };

    if(memcmp(&tmp, &rc->hub, sizeof(tmp)) != 0) {
        rc->version = tmp.version = (rc->version > rc->hub.version ? rc->version : rc->hub.version) + 1;
        memcpy(&rc->hub, &tmp, sizeof(tmp));
        // TODO save to SQL and send to cloud
    }
}

void rrdcontext_insert_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    (void)host;
    RRDCONTEXT *rc = (RRDCONTEXT *)value;

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
    }
    else {
        // we are adding this context now for the first time
        rc->version = now_realtime_sec();
    }

    internal_error(true, "RRDCONTEXT: added context '%s' on host '%s', version %lu, title '%s', units '%s', chart type '%s' priority %zu",
                   string2str(rc->id), host->hostname, rc->version, string2str(rc->title), string2str(rc->units),
                   rrdset_type_name(rc->chart_type), rc->priority);

    check_if_we_need_to_update_cloud(rc);
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
    size_t length = MAX(a, b);
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
        check_if_we_need_to_update_cloud(rc);
}

void rrdcontexts_create(RRDHOST *host) {
    if(host->rrdcontexts) return;
    host->rrdcontexts = dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback(host->rrdcontexts, rrdcontext_insert_callback, (void *)host);
    dictionary_register_delete_callback(host->rrdcontexts, rrdcontext_delete_callback, (void *)host);
    dictionary_register_conflict_callback(host->rrdcontexts, rrdcontext_conflict_callback, (void *)host);
}

void rrdcontext_add(RRDHOST *host, const char *id, const char *name) {
    (void)name;

    RRDCONTEXT tmp = {
        .id = string_dupz(id),
        .rrdinstances = NULL,
    };

    dictionary_set(host->rrdcontexts, id, &tmp, sizeof(RRDCONTEXT));
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
        .context = rrdcontext_acquire(host, sc->context),
    };
    uuid_copy(tmp.uuid, sc->chart_id);
    DICTIONARY_ITEM *item = dictionary_set_and_acquire_item(host->rrdinstances, sc->id, &tmp, sizeof(tmp));
    RRDINSTANCE *ri = dictionary_acquired_item_value(host->rrdinstances, item);

    //ctx_get_label_list(ri->uuid, dict_ctx_get_label_list_cb, NULL);
    //ctx_get_dimension_list(ri->uuid, dict_ctx_get_dimension_list_cb, NULL);
}

static void rrdcontext_load_context_callback(VERSIONED_CONTEXT_DATA *ctx_data, void *data) {
    RRDHOST *host = data;
    (void)host;

    RRDCONTEXT tmp = {
        .id = string_dupz(ctx_data->id),
    };
    memcpy(&tmp.hub, ctx_data, sizeof(VERSIONED_CONTEXT_DATA));
    dictionary_set(host->rrdcontexts, ctx_data->id, &tmp, sizeof(tmp));
}

void rrdcontext_load_all(RRDHOST *host) {
    rrdinstances_create(host);
    rrdcontexts_create(host);

    ctx_get_context_list(&host->host_uuid, rrdcontext_load_context_callback, host);
    ctx_get_chart_list(&host->host_uuid, rrdinstance_load_chart_callback, host);
}
