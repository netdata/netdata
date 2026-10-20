// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_SERVING_INTERNALS_H
#define NETDATA_NRPC_SERVING_INTERNALS_H

#include "libnetdata/libnetdata.h"

struct nrpc_serving_handle;

// also declared in the public umbrella header for consumers; declared here
// too so the implementation file keeps prototype checking while staying a
// leaf (any file including both makes the compiler verify they agree)
void nrpc_serving_started(void);
void nrpc_serving_finished(void);

struct nrpc_serving_handle *nrpc_serving_current_thread_acquire(void);
void nrpc_serving_release(struct nrpc_serving_handle *serving);
extern __thread struct nrpc_serving_handle *nrpc_thread_serving;
bool nrpc_serving_running(struct nrpc_serving_handle *serving);
pid_t nrpc_serving_tid(struct nrpc_serving_handle *serving);
bool nrpc_serving_dispatcher_acquire(struct nrpc_serving_handle *serving);
void nrpc_serving_dispatcher_release(struct nrpc_serving_handle *serving);

#endif //NETDATA_NRPC_SERVING_INTERNALS_H
