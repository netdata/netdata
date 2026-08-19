// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_BUILTIN_H
#define NETDATA_NRPC_BUILTIN_H

#include "libnetdata/libnetdata.h"

typedef int (*nrpc_builtin_handler_cb_t)(BUFFER *wb, const char *function, BUFFER *payload, const char *source);

// Registration struct for a builtin (daemon-implemented synchronous)
// method. Deliberately narrower than struct nrpc_method_desc: sync and source
// are forced internally (true, NRPC_SOURCE_DAEMON) where no caller can
// contradict them, and the handler is the typed builtin callback. Zero
// semantics match struct nrpc_method_desc - including the priority-0
// normalization; the same must-write style rule applies (timeout_s,
// priority, version, access).
struct nrpc_builtin_desc {
    NRPC_OWNER owner;              // required: the owning host, as its owner token
    const char *name;              // required
    const char *help;              // required
    const char *tags;              // NULL normalizes to "top"; NRPC_TAG_HIDDEN derives RESTRICTED
    int timeout_s;                 // stored raw - a zero is a 0s deadline, not "default"
    int priority;                  // 0 normalizes to NRPC_PRIORITY_DEFAULT on every registration
    uint32_t version;              // 0 == NRPC_VERSION_DEFAULT
    HTTP_ACCESS access;            // HTTP_ACCESS_NONE (0) is a real, permissive value
    nrpc_builtin_handler_cb_t handler; // required
};

void nrpc_method_register_builtin(const struct nrpc_builtin_desc *desc);

#endif //NETDATA_NRPC_BUILTIN_H
