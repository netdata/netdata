// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DYNCFG_H
#define NETDATA_DYNCFG_H

#include "../common.h"
#include "database/rrd.h"
#include "nrpc/nrpc.h"

#define DYNCFG_FUNCTIONS_VERSION 0

void dyncfg_add_streaming(BUFFER *wb);
bool dyncfg_available_for_rrdhost(RRDHOST *host);
void dyncfg_host_init(RRDHOST *host);

// Add specification: the caller's input to one configuration-node addition,
// shared by dyncfg_add_low_level() and dyncfg_add_internal(). Stack-filled by
// the caller; the callee copies what it keeps. Every field's zero keeps the
// meaning a zero positional argument had - no defaulting is added at the fill
// (the HTTP_ACCESS_NONE-to-default mapping and cmds sanitization live inside
// dyncfg_add_low_level(), exactly as before, and the intercept path keeps
// bypassing them). Style rule: policy-valued fields (status, type,
// source_type, cmds, sync, view_access, edit_access) are written explicitly
// at every site even when zero; pointer fields may be omitted when NULL.
// `transport` must be the plugin's function transport when `handler_data`
// is that same transport - i.e. only on the pluginsd CONFIG path; every other
// caller leaves it NULL. When set it becomes the node's pinned transport (see
// the DYNCFG struct).
struct nrpc_transport;
struct dyncfg_add_spec {
    RRDHOST *host;                    // required
    const char *id;                   // required
    const char *path;                 // required; UI organization
    DYNCFG_STATUS status;             // stored raw here, DYNCFG_STATUS_NONE (0) included; only
                                      // dyncfg_status_low_level() rejects NONE
    DYNCFG_TYPE type;                 // DYNCFG_TYPE_SINGLE (0) is a real value
    DYNCFG_SOURCE_TYPE source_type;   // DYNCFG_SOURCE_TYPE_INTERNAL (0) is a real value
    const char *source;               // provenance string
    DYNCFG_CMDS cmds;                 // DYNCFG_CMD_NONE (0) is a real value (low_level sanitizes)
    bool sync;
    HTTP_ACCESS view_access;          // HTTP_ACCESS_NONE (0): low_level maps to defaults; internal stores raw
    HTTP_ACCESS edit_access;          // same as view_access
    nrpc_handler_cb_t handler;        // required
    void *handler_data;               // see the transport contract above
    struct nrpc_transport *transport; // pluginsd CONFIG path only; NULL elsewhere
};

// low-level API used by plugins.d and high-level API.
bool dyncfg_add_low_level(const struct dyncfg_add_spec *spec);
void dyncfg_del_low_level(RRDHOST *host, const char *id);
void dyncfg_status_low_level(RRDHOST *host, const char *id, DYNCFG_STATUS status);
void dyncfg_init_low_level(bool load_saved);
void dyncfg_shutdown_low_level(void);

// Add specification for the high-level (inline-callback) API below: the same
// attribute bundle, with the typed dyncfg_cb_t callback pair instead of the
// low-level handler/transport surface - sync is forced (true) internally
// where no caller can contradict it. The same zero semantics and style rule
// apply.
struct dyncfg_add_inline_spec {
    RRDHOST *host;                    // required
    const char *id;                   // required
    const char *path;                 // required; UI organization
    DYNCFG_STATUS status;
    DYNCFG_TYPE type;
    DYNCFG_SOURCE_TYPE source_type;
    const char *source;               // provenance string
    DYNCFG_CMDS cmds;
    HTTP_ACCESS view_access;          // HTTP_ACCESS_NONE (0) maps to defaults in low_level
    HTTP_ACCESS edit_access;          // same as view_access
    dyncfg_cb_t cb;                   // required
    void *data;                       // passed to cb; legitimately NULL
};

// high-level API for internal modules
bool dyncfg_add(const struct dyncfg_add_inline_spec *spec);
void dyncfg_del(RRDHOST *host, const char *id);
void dyncfg_status(RRDHOST *host, const char *id, DYNCFG_STATUS status);

void dyncfg_init(bool load_saved);
void dyncfg_shutdown(void);

#endif //NETDATA_DYNCFG_H
