// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_RRD_INTERNALS
#include "rrd.h"

typedef struct rrdfamily {
    STRING *family;
    DICTIONARY *rrdvars;
} RRDFAMILY;

// ----------------------------------------------------------------------------
// RRDFAMILY index

struct rrdfamily_constructor {
    const char *family;
};

static void rrdfamily_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdfamily, void *constructor_data) {
    RRDFAMILY *rf = rrdfamily;
    struct rrdfamily_constructor *ctr = constructor_data;

    rf->family = string_strdupz(ctr->family);
    rf->rrdvars = rrdvariables_create();
}

static void rrdfamily_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdfamily, void *rrdhost __maybe_unused) {
    RRDFAMILY *rf = rrdfamily;
    string_freez(rf->family);
    rrdvariables_destroy(rf->rrdvars);
    rf->family = NULL;
    rf->rrdvars = NULL;
}

void rrdfamily_index_init(RRDHOST *host) {
    if(!host->rrdfamily_root_index) {
        host->rrdfamily_root_index = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                                &dictionary_stats_category_rrdhealth, sizeof(RRDFAMILY));

        dictionary_register_insert_callback(host->rrdfamily_root_index, rrdfamily_insert_callback, NULL);
        dictionary_register_delete_callback(host->rrdfamily_root_index, rrdfamily_delete_callback, host);
    }
}

void rrdfamily_index_destroy(RRDHOST *host) {
    dictionary_destroy(host->rrdfamily_root_index);
    host->rrdfamily_root_index = NULL;
}


// ----------------------------------------------------------------------------
// RRDFAMILY management

const RRDFAMILY_ACQUIRED *rrdfamily_add_and_acquire(RRDHOST *host, const char *id) {
    struct rrdfamily_constructor tmp = {
        .family = id,
    };
    return (const RRDFAMILY_ACQUIRED *)dictionary_set_and_acquire_item_advanced(host->rrdfamily_root_index, id, -1, NULL, sizeof(RRDFAMILY), &tmp);
}

void rrdfamily_release(RRDHOST *host, const RRDFAMILY_ACQUIRED *rfa) {
    if(unlikely(!rfa)) return;
    dictionary_acquired_item_release(host->rrdfamily_root_index, (const DICTIONARY_ITEM *)rfa);
}

DICTIONARY *rrdfamily_rrdvars_dict(const RRDFAMILY_ACQUIRED *rfa) {
    if(unlikely(!rfa)) return NULL;
    RRDFAMILY *rf = dictionary_acquired_item_value((const DICTIONARY_ITEM *)rfa);
    return(rf->rrdvars);
}
