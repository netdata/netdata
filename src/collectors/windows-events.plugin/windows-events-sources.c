// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-sources.h"

DICTIONARY *wevt_sources = NULL;
DICTIONARY *used_hashes_registry = NULL;
static usec_t wevt_session = 0;

WEVT_SOURCE_TYPE wevt_internal_source_type(const char *value) {
    if(strcmp(value, WEVT_SOURCE_ALL_NAME) == 0)
        return WEVTS_ALL;

    if(strcmp(value, WEVT_SOURCE_ALL_ADMIN_NAME) == 0)
        return WEVTS_ADMIN;

    if(strcmp(value, WEVT_SOURCE_ALL_OPERATIONAL_NAME) == 0)
        return WEVTS_OPERATIONAL;

    if(strcmp(value, WEVT_SOURCE_ALL_ANALYTIC_NAME) == 0)
        return WEVTS_ANALYTIC;

    if(strcmp(value, WEVT_SOURCE_ALL_DEBUG_NAME) == 0)
        return WEVTS_DEBUG;

    if(strcmp(value, WEVT_SOURCE_ALL_WINDOWS_NAME) == 0)
        return WEVTS_WINDOWS;

    return WEVTS_NONE;
}

void wevt_sources_del_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    LOGS_QUERY_SOURCE *src = value;
    freez((void *)src->fullname);
    string_freez(src->source);

    src->fullname = NULL;
    src->source = NULL;
}

static bool wevt_sources_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    LOGS_QUERY_SOURCE *src_old = old_value;
    LOGS_QUERY_SOURCE *src_new = new_value;

    bool ret = false;
    if(src_new->last_scan_monotonic_ut > src_old->last_scan_monotonic_ut) {
        src_old->last_scan_monotonic_ut = src_new->last_scan_monotonic_ut;

        if (src_old->source != src_new->source) {
            string_freez(src_old->source);
            src_old->source = src_new->source;
            src_new->source = NULL;
        }
        src_old->source_type = src_new->source_type;

        src_old->msg_first_ut = src_new->msg_first_ut;
        src_old->msg_last_ut = src_new->msg_last_ut;
        src_old->msg_first_id = src_new->msg_first_id;
        src_old->msg_last_id = src_new->msg_last_id;
        src_old->entries = src_new->entries;
        src_old->size = src_new->size;

        ret = true;
    }

    freez((void *)src_new->fullname);
    string_freez(src_new->source);
    src_new->fullname = NULL;
    src_new->source = NULL;

    return ret;
}

void wevt_sources_init(void) {
    wevt_session = now_realtime_usec();

    used_hashes_registry = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);

    wevt_sources = dictionary_create_advanced(DICT_OPTION_FIXED_SIZE | DICT_OPTION_DONT_OVERWRITE_VALUE,
                                              NULL, sizeof(LOGS_QUERY_SOURCE));

    dictionary_register_delete_callback(wevt_sources, wevt_sources_del_cb, NULL);
    dictionary_register_conflict_callback(wevt_sources, wevt_sources_conflict_cb, NULL);
}

void buffer_json_wevt_versions(BUFFER *wb __maybe_unused) {
    buffer_json_member_add_object(wb, "versions");
    {
        buffer_json_member_add_uint64(wb, "sources",
                wevt_session + dictionary_version(wevt_sources));
    }
    buffer_json_object_close(wb);
}

// --------------------------------------------------------------------------------------------------------------------

int wevt_sources_dict_items_backward_compar(const void *a, const void *b) {
    const DICTIONARY_ITEM **da = (const DICTIONARY_ITEM **)a, **db = (const DICTIONARY_ITEM **)b;
    LOGS_QUERY_SOURCE *sa = dictionary_acquired_item_value(*da);
    LOGS_QUERY_SOURCE *sb = dictionary_acquired_item_value(*db);

    // compare the last message timestamps
    if(sa->msg_last_ut < sb->msg_last_ut)
        return 1;

    if(sa->msg_last_ut > sb->msg_last_ut)
        return -1;

    // compare the first message timestamps
    if(sa->msg_first_ut < sb->msg_first_ut)
        return 1;

    if(sa->msg_first_ut > sb->msg_first_ut)
        return -1;

    return 0;
}

int wevt_sources_dict_items_forward_compar(const void *a, const void *b) {
    return -wevt_sources_dict_items_backward_compar(a, b);
}

// --------------------------------------------------------------------------------------------------------------------

struct wevt_source {
    usec_t first_ut;
    usec_t last_ut;
    size_t count;
    uint64_t size;
};

static int wevt_source_to_json_array_cb(const DICTIONARY_ITEM *item, void *entry, void *data) {
    const struct wevt_source *s = entry;
    BUFFER *wb = data;

    const char *name = dictionary_acquired_item_name(item);

    buffer_json_add_array_item_object(wb);
    {
        char size_for_humans[128];
        size_snprintf(size_for_humans, sizeof(size_for_humans), s->size, "B", false);

        char duration_for_humans[128];
        duration_snprintf(duration_for_humans, sizeof(duration_for_humans),
                          (time_t)((s->last_ut - s->first_ut) / USEC_PER_SEC), "s", true);

        char info[1024];
        snprintfz(info, sizeof(info), "%zu channel%s, with a total size of %s, covering %s",
                s->count, s->count > 1 ? "s":"", size_for_humans, duration_for_humans);

        buffer_json_member_add_string(wb, "id", name);
        buffer_json_member_add_string(wb, "name", name);
        buffer_json_member_add_string(wb, "pill", size_for_humans);
        buffer_json_member_add_string(wb, "info", info);
    }
    buffer_json_object_close(wb); // options object

    return 1;
}

static bool wevt_source_merge_sizes(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value , void *data __maybe_unused) {
    struct wevt_source *old_v = old_value;
    const struct wevt_source *new_v = new_value;

    old_v->count += new_v->count;
    old_v->size += new_v->size;

    if(new_v->first_ut && new_v->first_ut < old_v->first_ut)
        old_v->first_ut = new_v->first_ut;

    if(new_v->last_ut && new_v->last_ut > old_v->last_ut)
        old_v->last_ut = new_v->last_ut;

    return false;
}

void wevt_sources_to_json_array(BUFFER *wb) {
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_NAME_LINK_DONT_CLONE|DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_register_conflict_callback(dict, wevt_source_merge_sizes, NULL);

    struct wevt_source t = { 0 };

    LOGS_QUERY_SOURCE *src;
    dfe_start_read(wevt_sources, src) {
        t.first_ut = src->msg_first_ut;
        t.last_ut = src->msg_last_ut;
        t.count = 1;
        t.size = src->size;

        dictionary_set(dict, WEVT_SOURCE_ALL_NAME, &t, sizeof(t));

        if(src->source_type & WEVTS_ADMIN)
            dictionary_set(dict, WEVT_SOURCE_ALL_ADMIN_NAME, &t, sizeof(t));

        if(src->source_type & WEVTS_OPERATIONAL)
            dictionary_set(dict, WEVT_SOURCE_ALL_OPERATIONAL_NAME, &t, sizeof(t));

        if(src->source_type & WEVTS_ANALYTIC)
            dictionary_set(dict, WEVT_SOURCE_ALL_ANALYTIC_NAME, &t, sizeof(t));

        if(src->source_type & WEVTS_DEBUG)
            dictionary_set(dict, WEVT_SOURCE_ALL_DEBUG_NAME, &t, sizeof(t));

        if(src->source_type & WEVTS_WINDOWS)
            dictionary_set(dict, WEVT_SOURCE_ALL_WINDOWS_NAME, &t, sizeof(t));

        if(src->source)
            dictionary_set(dict, string2str(src->source), &t, sizeof(t));
    }
    dfe_done(jf);

    dictionary_sorted_walkthrough_read(dict, wevt_source_to_json_array_cb, wb);
}

void wevt_sources_scan(void) {
    static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;
    LPWSTR channel = NULL;
    EVT_HANDLE hChannelEnum = NULL;

    if(spinlock_trylock(&spinlock)) {
        const usec_t now_monotonic_ut = now_monotonic_usec();

        DWORD dwChannelBufferSize = 0;
        DWORD dwChannelBufferUsed = 0;
        DWORD status = ERROR_SUCCESS;

        // Open a handle to enumerate the event channels
        hChannelEnum = EvtOpenChannelEnum(NULL, 0);
        if (!hChannelEnum) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "WINDOWS EVENTS: EvtOpenChannelEnum() failed with %" PRIu64 "\n",
                    (uint64_t)GetLastError());
            goto cleanup;
        }

        WEVT_LOG *log = wevt_openlog6();
        if(!log) goto cleanup;

        while (true) {
            if (!EvtNextChannelPath(hChannelEnum, dwChannelBufferSize, channel, &dwChannelBufferUsed)) {
                status = GetLastError();
                if (status == ERROR_NO_MORE_ITEMS)
                    break; // No more channels
                else if (status == ERROR_INSUFFICIENT_BUFFER) {
                    dwChannelBufferSize = dwChannelBufferUsed;
                    freez(channel);
                    channel = mallocz(dwChannelBufferSize * sizeof(WCHAR));
                    continue;
                } else {
                    nd_log(NDLS_COLLECTORS, NDLP_ERR,
                           "WINDOWS EVENTS: EvtNextChannelPath() failed\n");
                    break;
                }
            }

            EVT_RETENTION retention;
            if(!wevt_channel_retention(log, channel, &retention))
                continue;

            const char *name = channel2utf8(channel);
            const char *fullname = strdupz(name);
            char *slash = strchr(name, '/');
            WEVT_SOURCE_TYPE sources = WEVTS_ALL;
            if(slash) {
                *slash++ = '\0';
                if(strcasecmp(slash, "Admin") == 0)
                    sources |= WEVTS_ADMIN;
                if(strcasecmp(slash, "Operational") == 0)
                    sources |= WEVTS_OPERATIONAL;
                if(strcasecmp(slash, "Analytic") == 0)
                    sources |= WEVTS_ANALYTIC;
                if(strcasecmp(slash, "Debug") == 0)
                    sources |= WEVTS_DEBUG;
            }

            if(strcasecmp(name, "Application") == 0)
                sources |= WEVTS_WINDOWS;
            if(strcasecmp(name, "Security") == 0)
                sources |= WEVTS_WINDOWS;
            if(strcasecmp(name, "Setup") == 0)
                sources |= WEVTS_WINDOWS;
            if(strcasecmp(name, "System") == 0)
                sources |= WEVTS_WINDOWS;

            LOGS_QUERY_SOURCE src = {
                .entries = retention.entries,
                .fullname = fullname,
                .fullname_len = strlen(fullname),
                .last_scan_monotonic_ut = now_monotonic_usec(),
                .msg_first_id = retention.first_event.id,
                .msg_last_id = retention.last_event.id,
                .msg_first_ut = retention.first_event.created_ns / NSEC_PER_USEC,
                .msg_last_ut = retention.last_event.created_ns / NSEC_PER_USEC,
                .size = retention.size_bytes,
                .source_type = sources,
                .source = string_strdupz(name),
            };

            dictionary_set(wevt_sources, src.fullname, &src, sizeof(src));
        }

        wevt_closelog6(log);

        LOGS_QUERY_SOURCE *src;
        dfe_start_write(wevt_sources, src)
        {
            if(src->last_scan_monotonic_ut < now_monotonic_ut)
                dictionary_del(wevt_sources, src->fullname);
        }
        dfe_done(src);
        dictionary_garbage_collect(wevt_sources);

        spinlock_unlock(&spinlock);
    }

cleanup:
    freez(channel);
    EvtClose(hChannelEnum);
}
