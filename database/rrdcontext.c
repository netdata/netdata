// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext.h"

typedef enum {
    RRDMETRIC_FLAG_NONE     = 0,
    RRDMETRIC_FLAG_ARCHIVED = (1 << 0),
} RRDMETRIC_FLAGS;

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

    DICTIONARY *rrdlabels;
    DICTIONARY *rrdmetrics;
} RRDINSTANCE;

typedef struct rrdcontext {
    STRING *context;

    usec_t version;

    time_t oldest_t;
    time_t latest_t;

    DICTIONARY *rrdinstances;
} RRDCONTEXT;


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
    DICTIONARY_ITEM *item = dictionary_acquire_item(host->rrdinstances, id);
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

void rrdcontext_insert_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    RRDCONTEXT *rc = (RRDCONTEXT *)value;
    (void)host;
    (void)rc;

    // TODO what other initializations needed?
}

void rrdcontext_delete_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    (void)host;

    RRDCONTEXT *rc = (RRDCONTEXT *)value;
    dictionary_destroy(rc->rrdinstances);
    string_freez(rc->context);
}

void rrdcontext_conflict_callback(const char *id, void *oldv, void *newv, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    (void)host;

    RRDCONTEXT *rc_old = (RRDCONTEXT *)oldv;
    RRDCONTEXT *rc_new = (RRDCONTEXT *)newv;

    // TODO what other needs to be done here?

    string_freez(rc_new->context);

    (void)rc_old;
}

DICTIONARY *rrdcontext_create_dictionary(RRDHOST *host) {
    DICTIONARY *dict = dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback(dict, rrdcontext_insert_callback, (void *)host);
    dictionary_register_delete_callback(dict, rrdcontext_delete_callback, (void *)host);
    dictionary_register_conflict_callback(dict, rrdcontext_conflict_callback, (void *)host);
    return dict;
}

void rrdcontext_add(RRDHOST *host, const char *id, const char *name) {
    (void)name;

    RRDCONTEXT tmp = {
        .context = string_dupz(id),
        .rrdinstances = NULL,
    };

    dictionary_set(host->rrdcontexts, id, &tmp, sizeof(RRDINSTANCE));
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