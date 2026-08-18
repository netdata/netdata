// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_CATALOG_H
#define NETDATA_NRPC_CATALOG_H

#include "rrd.h"

#define NRPC_VERSION_SEPARATOR "|"

// ----------------------------------------------------------------------------
// the iteration/visibility API: every consumer that renders or exports the
// function registry goes through nrpc_catalog_*_foreach() - nobody outside
// the module touches the registry dictionary or its entries directly

// which functions a traversal visits; EVERY filter includes the availability
// check (serving thread running, host state current, not unregistered)
typedef enum {
    NRPC_CATALOG_FILTER_USER,        // + skip DYNCFG and RESTRICTED (user-facing lists, cloud)
    NRPC_CATALOG_FILTER_STREAM_GLOBAL, // + skip DYNCFG; DYNCFG entries are COUNTED
                                            //   (the return value) for dyncfg_add_streaming()
} NRPC_CATALOG_FILTER;

// The view handed to the callback. help/tags are BYTE COPIES valid only for
// the duration of the callback - the underlying STRINGs can be swapped and
// freed by a concurrent re-registration the moment the entry's leaf lock is
// released, so the copies are taken under it.
struct nrpc_method_view {
    const char *name;
    const char *help;
    const char *tags;
    int timeout;
    HTTP_ACCESS access;
    int priority;
    uint32_t version;
    NRPC_METHOD_FLAGS options;
};

typedef void (*nrpc_method_view_cb_t)(const struct nrpc_method_view *v, void *data);

// returns the number of DYNCFG entries encountered (meaningful for
// NRPC_CATALOG_FILTER_STREAM_GLOBAL, zero otherwise)
size_t nrpc_catalog_host_foreach(RRDHOST *host, NRPC_CATALOG_FILTER filter, nrpc_method_view_cb_t cb, void *data);

// ----------------------------------------------------------------------------
// the consumers built on the iteration API

void stream_sender_send_host_functions(RRDHOST *host, BUFFER *wb, bool dyncfg, bool can_function_del);

void nrpc_catalog_host2json(RRDHOST *host, BUFFER *wb);

// help/tags receive OWNED byte copies (strdupz) - the destination dictionary's
// callbacks own freeing them (conflict losers and deleted entries)
void nrpc_catalog_host_to_dict(RRDHOST *host, DICTIONARY *dst, void *value, size_t value_size,
                            const char **help, const char **tags, HTTP_ACCESS *access, int *priority, uint32_t *version);

// the ACLK node-instance manifest content (see sqlite_aclk_node.c)
struct nrpc_manifest_entry {
    const char *help;   // owned copy
    const char *tags;   // owned copy
    HTTP_ACCESS access;
    int priority;
    uint32_t version;
};

// Returns a new dictionary, keyed by function name, holding one
// struct nrpc_manifest_entry per available, user-visible function.
// The caller owns it and must dictionary_destroy() it; the string copies are
// released by a delete callback registered on the dictionary.
DICTIONARY *nrpc_catalog_manifest_dict(RRDHOST *host);

// Content hash of a manifest dictionary plus the node identity it will be published under.
// Covers exactly the fields generate_update_node_instance_manifest_message() transmits
// (name, help, tags, access, priority, version) plus node_id/claim_id - updated_at is
// excluded because it changes on every build. Used to suppress publishing a manifest
// identical to the one already sent. Change detection only, not security-sensitive.
uint64_t nrpc_catalog_manifest_hash(DICTIONARY *dict, const char *node_id, const char *claim_id);

#endif // NETDATA_NRPC_CATALOG_H
