// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDFUNCTIONS_EXPORTERS_H
#define NETDATA_RRDFUNCTIONS_EXPORTERS_H

#include "rrd.h"

#define RRDFUNCTIONS_VERSION_SEPARATOR "|"

// ----------------------------------------------------------------------------
// the iteration/visibility API: every consumer that renders or exports the
// function registry goes through rrd_functions_*_foreach() - nobody outside
// the module touches the registry dictionary or its entries directly

// which functions a traversal visits; EVERY filter includes the availability
// check (collector running, host state current, not unregistered)
typedef enum {
    RRD_FUNCTIONS_FILTER_EXPORTABLE,        // + skip DYNCFG and RESTRICTED (user-facing lists, cloud)
    RRD_FUNCTIONS_FILTER_STREAMABLE_CHART,  // + skip DYNCFG (chart functions stream, RESTRICTED included)
    RRD_FUNCTIONS_FILTER_STREAMABLE_GLOBAL, // + skip LOCAL and DYNCFG; DYNCFG entries are COUNTED
                                            //   (the return value) for dyncfg_add_streaming()
} RRD_FUNCTIONS_FILTER;

// The view handed to the callback. help/tags are BYTE COPIES valid only for
// the duration of the callback - the underlying STRINGs can be swapped and
// freed by a concurrent re-registration the moment the entry's leaf lock is
// released, so the copies are taken under it.
struct rrd_function_view {
    const char *name;
    const char *help;
    const char *tags;
    int timeout;
    HTTP_ACCESS access;
    int priority;
    uint32_t version;
    RRD_FUNCTION_OPTIONS options;
};

typedef void (*rrd_function_view_cb_t)(const struct rrd_function_view *v, void *data);

// both return the number of DYNCFG entries encountered (meaningful for
// RRD_FUNCTIONS_FILTER_STREAMABLE_GLOBAL, zero otherwise)
size_t rrd_functions_host_foreach(RRDHOST *host, RRD_FUNCTIONS_FILTER filter, rrd_function_view_cb_t cb, void *data);
size_t rrd_functions_rrdset_foreach(RRDSET *st, RRD_FUNCTIONS_FILTER filter, rrd_function_view_cb_t cb, void *data);

// destroy the per-chart view of the registry (rrdset teardown)
void rrd_functions_rrdset_view_destroy(RRDSET *st);

// ----------------------------------------------------------------------------
// the consumers built on the iteration API

void stream_sender_send_rrdset_functions(RRDSET *st, BUFFER *wb);
void stream_sender_send_global_rrdhost_functions(RRDHOST *host, BUFFER *wb, bool dyncfg, bool can_function_del);

void chart_functions2json(RRDSET *st, BUFFER *wb);
void host_functions2json(RRDHOST *host, BUFFER *wb);

// preserves the historical host==NULL availability semantics (no host-state
// check) for instances resolved through the contexts index
void chart_functions_to_dict(RRDSET *st, DICTIONARY *dst, void *value, size_t value_size);

// help/tags receive OWNED byte copies (strdupz) - the destination dictionary's
// callbacks own freeing them (conflict losers and deleted entries)
void host_functions_to_dict(RRDHOST *host, DICTIONARY *dst, void *value, size_t value_size,
                            const char **help, const char **tags, HTTP_ACCESS *access, int *priority, uint32_t *version);

// the ACLK node-instance manifest content (see sqlite_aclk_node.c)
struct rrd_function_manifest_entry {
    const char *help;   // owned copy
    const char *tags;   // owned copy
    HTTP_ACCESS access;
    int priority;
    uint32_t version;
};

// Returns a new dictionary, keyed by function name, holding one
// struct rrd_function_manifest_entry per available, user-visible function.
// The caller owns it and must dictionary_destroy() it; the string copies are
// released by a delete callback registered on the dictionary.
DICTIONARY *host_functions_to_manifest_dict(RRDHOST *host);

// Content hash of a manifest dictionary plus the node identity it will be published under.
// Covers exactly the fields generate_update_node_instance_manifest_message() transmits
// (name, help, tags, access, priority, version) plus node_id/claim_id - updated_at is
// excluded because it changes on every build. Used to suppress publishing a manifest
// identical to the one already sent. Change detection only, not security-sensitive.
uint64_t manifest_dict_hash(DICTIONARY *dict, const char *node_id, const char *claim_id);

#endif // NETDATA_RRDFUNCTIONS_EXPORTERS_H
