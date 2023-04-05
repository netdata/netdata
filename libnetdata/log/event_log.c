// SPDX-License-Identifier: GPL-3.0-or-later

#include "event_log.h"

inline EVENT_LOG_ENTRY* event_log_create_entry(
    char *name,
    char *info
) {
    EVENT_LOG_ENTRY *ee = callocz(1, sizeof(EVENT_LOG_ENTRY));
    ee->name = string_strdupz(name);
    ee->info = string_strdupz(info);
    ee->when = (uint32_t)now_realtime_sec();

    return ee;
}

inline void event_log_add_entry(RRDHOST *host, EVENT_LOG_ENTRY *ee) {
    netdata_rwlock_wrlock(&host->event_log.event_log_rwlock);
    ee->unique_id = host->event_log.max_unique_id++;
    ee->next = host->event_log.events;
    host->event_log.events = ee;
    netdata_rwlock_unlock(&host->event_log.event_log_rwlock);
}

void event_log_init(RRDHOST *host)
{
    host->event_log.count = 0;
    host->event_log.max_unique_id = (uint32_t)now_realtime_sec();
}

void event_log_info_string2json(BUFFER *wb, const char *prefix, const char *label, const char *value, const char *suffix) {
    if(value && *value) {
        buffer_sprintf(wb, "%s\"%s\":\"", prefix, label);
        buffer_strcat_htmlescape(wb, value);
        buffer_strcat(wb, "\"");
        buffer_strcat(wb, suffix);
    }
    else
        buffer_sprintf(wb, "%s\"%s\":null%s", prefix, label, suffix);
}

void event_log_entry2json_nolock(BUFFER *wb, EVENT_LOG_ENTRY *ee, RRDHOST *host) {
    buffer_sprintf(wb,
            "\n\t{\n"
                    "\t\t\"hostname\": \"%s\",\n"
                    "\t\t\"unique_id\": %u,\n"
                    "\t\t\"name\": \"%s\",\n"
                    "\t\t\"when\": %lu,\n"
                   , rrdhost_hostname(host)
                   , ee->unique_id
                   , string2str(ee->name)
                   , (unsigned long)ee->when
    );

    event_log_info_string2json(wb, "\t\t", "info", ee->info ? string2str(ee->info) : "", "\n");
    buffer_strcat(wb, "\t}");
}

void event_log2json(RRDHOST *host, BUFFER *wb) {
    buffer_strcat(wb, "[");

    unsigned int count = 0;

    netdata_rwlock_rdlock(&host->event_log.event_log_rwlock);

    EVENT_LOG_ENTRY *ee;
    for (ee = host->event_log.events; ee ; ee = ee->next) {
        if (likely(count))
            buffer_strcat(wb, ",");
        event_log_entry2json_nolock(wb, ee, host);
        count++;
    }

    netdata_rwlock_unlock(&host->event_log.event_log_rwlock);

    buffer_strcat(wb, "\n]\n");
}
