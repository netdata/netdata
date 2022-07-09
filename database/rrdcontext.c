// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext.h"
#include "sqlite/sqlite_context.h"

typedef enum {
    RRDMETRIC_FLAG_NONE     = 0,
    RRDMETRIC_FLAG_ARCHIVED = (1 << 0),
} RRDMETRIC_FLAGS;

typedef struct rrdcontext RRDCONTEXT;


typedef struct rrdmetric {
    uuid_t uuid;

    STRING *id;
    STRING *name;

    RRDDIM *rd;

    int update_every;
    struct rrddim_tier *tiers[RRD_STORAGE_TIERS];

    RRDMETRIC_FLAGS flags;
} RRDMETRIC;

typedef struct rrdinstance {
    STRING *id;
    STRING *name;

    RRDSET *st;

    RRDCONTEXT *context;
    DICTIONARY *rrdlabels;
    DICTIONARY *rrdmetrics;
} RRDINSTANCE;


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

DICTIONARY *rrdmetric_create_dictionary(RRDINSTANCE *ri) {
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

DICTIONARY *rrdinstance_create_dictionary(RRDHOST *host) {
    DICTIONARY *dict = dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback(dict, rrdinstance_insert_callback, (void *)host);
    dictionary_register_delete_callback(dict, rrdinstance_delete_callback, (void *)host);
    dictionary_register_conflict_callback(dict, rrdinstance_conflict_callback, (void *)host);
    return dict;
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
} RRDCONTEXT_FLAGS;

struct rrdcontext {
    uint64_t version;

    STRING *id;
    STRING *title;
    STRING *units;

    size_t priority;

    time_t first_time_t;
    time_t last_time_t;
    RRDCONTEXT_FLAGS flags;

    VERSIONED_CONTEXT_DATA hub;
    VERSIONED_CONTEXT_DATA current;

    RRDHOST *host;
    DICTIONARY *rrdinstances;
};

void rrdcontext_insert_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    RRDCONTEXT *rc = (RRDCONTEXT *)value;
    (void)host;

    if(rc->hub.version) {
        // we are loading data from the SQL database

        if(rc->version)
            error("RRDCONTEXT: context '%s' is already initialized with version %lu, but it is loaded again from SQL with version %lu", string2str(rc->id), rc->version, rc->hub.version);

        string_freez(rc->id);
        rc->id = string_dupz(rc->hub.id);

        string_freez(rc->title);
        rc->title = string_dupz(rc->hub.title);

        string_freez(rc->units);
        rc->units = string_dupz(rc->hub.units);

        rc->priority     = rc->hub.priority;
        rc->version      = rc->hub.version;
        rc->first_time_t = rc->hub.first_time_t;
        rc->last_time_t  = rc->hub.last_time_t;
    }
    else {
        // we are adding this context now
        rc->current.version = now_realtime_sec();

        // TODO save to SQL
    }

    rc->hub.id        = string2str(rc->id);
    rc->hub.title     = string2str(rc->title);
    rc->hub.units     = string2str(rc->units);

    rc->current.id    = string2str(rc->id);
    rc->current.title = string2str(rc->title);
    rc->current.units = string2str(rc->units);

    // TODO what other initializations needed?
}

void rrdcontext_delete_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    (void)host;

    RRDCONTEXT *rc = (RRDCONTEXT *)value;
    dictionary_destroy(rc->rrdinstances);
    string_freez(rc->id);
}

void rrdcontext_conflict_callback(const char *id, void *oldv, void *newv, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    (void)host;

    RRDCONTEXT *rc_old = (RRDCONTEXT *)oldv;
    RRDCONTEXT *rc_new = (RRDCONTEXT *)newv;

    // TODO what other needs to be done here?

    string_freez(rc_new->id);

    (void)rc_old;
}

DICTIONARY *rrdcontext_create(RRDHOST *host) {
    DICTIONARY *dict = dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback(dict, rrdcontext_insert_callback, (void *)host);
    dictionary_register_delete_callback(dict, rrdcontext_delete_callback, (void *)host);
    dictionary_register_conflict_callback(dict, rrdcontext_conflict_callback, (void *)host);
    return dict;
}

void rrdcontext_add(RRDHOST *host, const char *id, const char *name) {
    (void)name;

    RRDCONTEXT tmp = {
        .id = string_dupz(id),
        .rrdinstances = NULL,
    };

    dictionary_set(host->rrdcontexts, id, &tmp, sizeof(RRDINSTANCE));
}

static void rrdcontext_load_context_callback(VERSIONED_CONTEXT_DATA *ctx_data, void *data) {
    RRDHOST *host = data;
    if(!host->rrdcontexts) rrdcontext_create(host);

    RRDCONTEXT tmp = {
        .id = string_dupz(ctx_data->id),
    };
    memcpy(&tmp.hub, ctx_data, sizeof(VERSIONED_CONTEXT_DATA));

    DICTIONARY_ITEM *item = dictionary_set_and_acquire_item(host->rrdcontexts, ctx_data->id, &tmp, sizeof(RRDCONTEXT));
    RRDCONTEXT *rc = dictionary_acquired_item_value(host->rrdcontexts, item);



    dictionary_acquired_item_release(host->rrdcontexts, item);
}

void rrdcontext_load_all(RRDHOST *host) {
    ctx_get_context_list(rrdcontext_load_context_callback, host);
}




// ----------------------------------------------------------------------------
// Load from SQL

/*
struct chart_load {
    uuid_t uuid;
    const char *id;
    const char *name;
    const char *context;
};


void load_chart_callback(RRDHOST *host, struct chart_load *cl) {

}

void load_everything_from_sql(void) {
    for(RRDHOST *host = localhost; host ;host = host->next) {
        sqlite_load_host_charts(host, load_chart_callback);

        RRDINSTANCE *ri;
        dfe_start_read(host->rrdinstances, ri) {
            sqlite_load_chart_labels(ri->uuid, load_chart_labels_callback, ri);
            sqlite_load_chart_dimensions(ri->uuid, load_chart_dimensions_callback, ri);
        }
        dfe_done(ri);
    }

}
*/