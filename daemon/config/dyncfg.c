// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyncfg.h"

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_TYPE type;
    const char *name;
} dyncfg_types[] = {
    { .type = DYNCFG_TYPE_SINGLE, .name = "single" },
    { .type = DYNCFG_TYPE_TEMPLATE, .name = "template" },
    { .type = DYNCFG_TYPE_JOB, .name = "job" },
};

DYNCFG_TYPE dyncfg_type2id(const char *type) {
    if(!type || !*type)
        return DYNCFG_TYPE_SINGLE;

    size_t entries = sizeof(dyncfg_types) / sizeof(dyncfg_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(dyncfg_types[i].name, type) == 0)
            return dyncfg_types[i].type;
    }

    return DYNCFG_TYPE_SINGLE;
}

const char *dyncfg_id2type(DYNCFG_TYPE type) {
    size_t entries = sizeof(dyncfg_types) / sizeof(dyncfg_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(type == dyncfg_types[i].type)
            return dyncfg_types[i].name;
    }

    return "single";
}

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_SOURCE_TYPE source_type;
    const char *name;
} dyncfg_source_types[] = {
    { .source_type = DYNCFG_SOURCE_TYPE_STOCK, .name = "stock" },
    { .source_type = DYNCFG_SOURCE_TYPE_USER, .name = "user" },
    { .source_type = DYNCFG_SOURCE_TYPE_DYNCFG, .name = "dyncfg" },
    { .source_type = DYNCFG_SOURCE_TYPE_DISCOVERY, .name = "discovered" },
};

DYNCFG_SOURCE_TYPE dyncfg_source_type2id(const char *source_type) {
    if(!source_type || !*source_type)
        return DYNCFG_SOURCE_TYPE_STOCK;

    size_t entries = sizeof(dyncfg_source_types) / sizeof(dyncfg_source_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(dyncfg_source_types[i].name, source_type) == 0)
            return dyncfg_source_types[i].source_type;
    }

    return DYNCFG_SOURCE_TYPE_STOCK;
}

const char *dyncfg_id2source_type(DYNCFG_SOURCE_TYPE source_type) {
    size_t entries = sizeof(dyncfg_source_types) / sizeof(dyncfg_source_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(source_type == dyncfg_source_types[i].source_type)
            return dyncfg_source_types[i].name;
    }

    return "stock";
}

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_STATUS status;
    const char *name;
} dyncfg_statuses[] = {
    { .status = DYNCFG_STATUS_OK, .name = "ok" },
    { .status = DYNCFG_STATUS_DISABLED, .name = "disabled" },
    { .status = DYNCFG_STATUS_ORPHAN, .name = "orphan" },
    { .status = DYNCFG_STATUS_REJECTED, .name = "rejected" },
};

DYNCFG_STATUS dyncfg_status2id(const char *status) {
    if(!status || !*status)
        return DYNCFG_STATUS_OK;

    size_t entries = sizeof(dyncfg_statuses) / sizeof(dyncfg_statuses[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(dyncfg_statuses[i].name, status) == 0)
            return dyncfg_statuses[i].status;
    }

    return DYNCFG_STATUS_OK;
}

const char *dyncfg_id2status(DYNCFG_STATUS status) {
    size_t entries = sizeof(dyncfg_statuses) / sizeof(dyncfg_statuses[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(status == dyncfg_statuses[i].status)
            return dyncfg_statuses[i].name;
    }

    return "ok";
}

// ----------------------------------------------------------------------------

typedef struct dyncfg {
    STRING *path;
    DYNCFG_STATUS status;
    DYNCFG_TYPE type;
    DYNCFG_SOURCE_TYPE source_type;
    STRING *source;
    usec_t created_ut;
    usec_t modified_ut;
} DYNCFG;

struct {
    DICTIONARY *nodes;
} dyncfg_globals = { 0 };

static void dyncfg_cleanup(DYNCFG *v) {
    string_freez(v->path);
    v->path = NULL;

    string_freez(v->source);
    v->source = NULL;
}

static void dyncfg_normalize(DYNCFG *v) {
    usec_t now_ut = now_realtime_usec();

    if(!v->created_ut)
        v->created_ut = now_ut;

    if(!v->modified_ut)
        v->modified_ut = now_ut;
}

static void dyncfg_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    DYNCFG *v = value;
    dyncfg_cleanup(v);
}

static void dyncfg_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    DYNCFG *v = value;
    dyncfg_normalize(v);
}

static bool dyncfg_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    DYNCFG *v = old_value;
    DYNCFG *nv = new_value;

    size_t changes = 0;

    dyncfg_normalize(nv);

    if(v->path != nv->path) {
        STRING *old = v->path;
        v->path = nv->path;
        nv->path = NULL;
        string_freez(old);
        changes++;
    }

    if(v->status != nv->status) {
        v->status = nv->status;
        changes++;
    }

    if(v->type != nv->type) {
        v->type = nv->type;
        changes++;
    }

    if(v->source_type != nv->source_type) {
        v->source_type = nv->source_type;
        changes++;
    }

    if(v->source != nv->source) {
        STRING *old = v->source;
        v->source = nv->source;
        nv->source = NULL;
        string_freez(old);
        changes++;
    }

    if(v->created_ut != nv->created_ut && nv->created_ut) {
        v->created_ut = nv->created_ut;
        changes++;
    }

    if(v->modified_ut != nv->modified_ut && nv->modified_ut) {
        v->modified_ut = nv->modified_ut;
        changes++;
    }

    dyncfg_cleanup(nv);

    return changes > 0;
}

void dyncfg_init(void) {
    if(!dyncfg_globals.nodes) {
        dyncfg_globals.nodes = dictionary_create_advanced(DICT_OPTION_FIXED_SIZE | DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, sizeof(DYNCFG));
        dictionary_register_insert_callback(dyncfg_globals.nodes, dyncfg_insert_cb, NULL);
        dictionary_register_conflict_callback(dyncfg_globals.nodes, dyncfg_conflict_cb, NULL);
        dictionary_register_delete_callback(dyncfg_globals.nodes, dyncfg_delete_cb, NULL);
    }
}

void dyncfg_add(const char *id, const char *path, DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, const char *source, usec_t created_ut, usec_t modified_ut) {
    dyncfg_init();

    DYNCFG tmp = {
        .path = string_strdupz(path),
        .status = status,
        .type = type,
        .source_type = source_type,
        .source = string_strdupz(source),
        .created_ut = created_ut,
        .modified_ut = modified_ut,
    };

    dictionary_set(dyncfg_globals.nodes, id, &tmp, sizeof(tmp));
}

