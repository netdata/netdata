// SPDX-License-Identifier: GPL-3.0-or-later

// Every daemon symbol the storage engine still reaches, declared here explicitly instead of
// arriving through rrd.h. This header is the burn-down list of the engine's decoupling: each
// entry names the step that removes it, and when the list is empty this header and
// rrdengine-daemon-check.c are deleted.
//
// rrdengine-daemon-check.c includes this header together with the daemon headers that really
// declare these symbols, so a signature that drifts on either side is a compile error rather
// than a silent link-time mismatch.

#ifndef NETDATA_RRDENGINE_DAEMON_H
#define NETDATA_RRDENGINE_DAEMON_H

#include "libnetdata/libnetdata.h"

// leaf daemon headers (they include nothing of the daemon themselves)
#include "daemon/libuv_workers.h"                 // UV_EVENT_* job ids, register_libuv_worker_jobs(),
                                                  // RESERVED_LIBUV_WORKER_THREADS                     -> hooks

// work the engine hands to the rest of the daemon                                                    -> hooks
void query_weights_worker_thread(void *arg);                                    // web/api/queries/weights.h
size_t populate_metrics_from_database(void *mrg, void (*populate_cb)(void *mrg, Word_t section, nd_uuid_t *uuid));
                                                                                // database/sqlite/sqlite_metadata.h

#endif // NETDATA_RRDENGINE_DAEMON_H
