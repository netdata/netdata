// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDFUNCTIONS_EXPORTERS_H
#define NETDATA_RRDFUNCTIONS_EXPORTERS_H

#include "rrd.h"

#define RRDFUNCTIONS_VERSION_SEPARATOR "|"

void stream_sender_send_rrdset_functions(RRDSET *st, BUFFER *wb);
void stream_sender_send_global_rrdhost_functions(RRDHOST *host, BUFFER *wb, bool dyncfg);

void chart_functions2json(RRDSET *st, BUFFER *wb);
void chart_functions_to_dict(DICTIONARY *rrdset_functions_view, DICTIONARY *dst, void *value, size_t value_size);
void host_functions_to_dict(RRDHOST *host, DICTIONARY *dst, void *value, size_t value_size, STRING **help, STRING **tags,
                            HTTP_ACCESS *access, int *priority, uint32_t *version);
void host_functions2json(RRDHOST *host, BUFFER *wb);

// Snapshot of a single user-visible host function. The strings are byte copies, not STRING
// references, so the entry stays valid after the host functions read lock is released without
// touching a refcount that a concurrent re-registration may already have dropped.
// The entry is keyed by the function name.
struct rrd_function_manifest_entry {
    char *help;             // owned copy
    char *tags;             // owned copy
    HTTP_ACCESS access;
    int priority;
    uint32_t version;
};

// Returns a new dictionary, keyed by function name, holding one
// struct rrd_function_manifest_entry per available, user-visible function.
// The caller owns it and must dictionary_destroy() it; the string references
// are released by a delete callback registered on the dictionary.
DICTIONARY *host_functions_to_manifest_dict(RRDHOST *host);

#endif //NETDATA_RRDFUNCTIONS_EXPORTERS_H
