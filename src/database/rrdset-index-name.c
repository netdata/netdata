// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdset-index-name.h"
#include "rrdset-index-id.h"
#include "rrdset-slots.h"

static RRDSET *rrdset_index_find_name(RRDHOST *host, const char *name);

STRING *rrdset_fix_name(RRDHOST *host, const char *chart_full_id, const char *type, const char *current_name, const char *name) {
    if(!name || !*name) return NULL;

    char full_name[RRD_ID_LENGTH_MAX + 1];
    char sanitized_name[CONFIG_MAX_VALUE + 1];
    char new_name[CONFIG_MAX_VALUE + 1];

    snprintfz(full_name, RRD_ID_LENGTH_MAX, "%s.%s", type, name);
    rrdset_strncpyz_name(sanitized_name, full_name, CONFIG_MAX_VALUE);
    strncpyz(new_name, sanitized_name, CONFIG_MAX_VALUE);

    if(rrdset_index_find_name(host, new_name)) {
        netdata_log_debug(D_RRD_CALLS, "RRDSET: chart name '%s' on host '%s' already exists.", new_name, rrdhost_hostname(host));
        if(!strcmp(chart_full_id, full_name) && (!current_name || !*current_name)) {
            unsigned i = 1;

            do {
                snprintfz(new_name, CONFIG_MAX_VALUE, "%s_%u", sanitized_name, i);
                i++;
            } while (rrdset_index_find_name(host, new_name));

            //            netdata_log_info("RRDSET: using name '%s' for chart '%s' on host '%s'.", new_name, full_name, rrdhost_hostname(host));
        }
        else
            return NULL;
    }

    return string_strdupz(new_name);
}

int rrdset_reset_name(RRDSET *st, const char *name) {
    if(unlikely(!strcmp(rrdset_name(st), name)))
        return 1;

    RRDHOST *host = st->rrdhost;

    netdata_log_debug(D_RRD_CALLS, "rrdset_reset_name() old: '%s', new: '%s'", rrdset_name(st), name);

    STRING *name_string = rrdset_fix_name(host, rrdset_id(st), rrdset_parts_type(st), string2str(st->name), name);
    if(!name_string) return 0;

    if(st->name) {
        rrdset_index_del_name(host, st);
        SWAP(name_string, st->name);
        string_freez(name_string);
    }
    else
        st->name = name_string;

    rrdset_index_add_name(host, st);

    rrdset_flag_clear(st, RRDSET_FLAG_EXPORTING_SEND|RRDSET_FLAG_EXPORTING_IGNORE|RRDSET_FLAG_UPSTREAM_SEND|RRDSET_FLAG_UPSTREAM_IGNORE);
    rrdset_metadata_updated(st);

    rrdcontext_updated_rrdset_name(st);
    return 2;
}

static RRDSET *rrdset_index_find_name(RRDHOST *host, const char *name) {
    if (unlikely(!host->rrdset_root_index_name))
        return NULL;
    return dictionary_get(host->rrdset_root_index_name, name);
}

void rrdset_index_byname_init(RRDHOST *host) {
    if(!host->rrdset_root_index_name)
        host->rrdset_root_index_name = dictionary_create_view(host->rrdset_root_index);
}

void rrdset_index_add_name(RRDHOST *host, RRDSET *st) {
    if(!st->name) return;
    const DICTIONARY_ITEM *sta = dictionary_get_and_acquire_item(host->rrdset_root_index, rrdset_id(st));
    if(sta) {
        const DICTIONARY_ITEM *sta2 = dictionary_view_set_and_acquire_item(host->rrdset_root_index_name, rrdset_name(st), sta);
        if(sta2) {
            if(dictionary_acquired_item_value(sta2) == st)
                rrdset_flag_set(st, RRDSET_FLAG_INDEXED_NAME);

            dictionary_acquired_item_release(host->rrdset_root_index_name, sta2);
        }

        dictionary_acquired_item_release(host->rrdset_root_index, sta);
    }
}

void rrdset_index_del_name(RRDHOST *host, RRDSET *st) {
    if(rrdset_flag_check(st, RRDSET_FLAG_INDEXED_NAME)) {
        const DICTIONARY_ITEM *sta = dictionary_get_and_acquire_item(host->rrdset_root_index_name, rrdset_name(st));

        if(sta) {
            if(dictionary_acquired_item_value(sta) == st)
                dictionary_del(host->rrdset_root_index_name, rrdset_name(st));

            dictionary_acquired_item_release(host->rrdset_root_index_name, sta);
            dictionary_garbage_collect(host->rrdset_root_index_name);
        }

        rrdset_flag_clear(st, RRDSET_FLAG_INDEXED_NAME);
    }
}

RRDSET *rrdset_find_byname(RRDHOST *host, const char *name) {
    RRDSET *st = rrdset_index_find_name(host, name);
    return(st);
}
