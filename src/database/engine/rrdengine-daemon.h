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
#include "daemon/config/netdata-conf-profile.h"   // nd_profile, netdata_conf_is_parent()            -> config struct
#include "daemon/libuv_workers.h"                 // UV_EVENT_* job ids, register_libuv_worker_jobs(),
                                                  // libuv_worker_threads                              -> config struct / hooks
#include "daemon/daemon-service.h"                // service_register()                                -> hooks

// configuration the engine reads inline                                                              -> config struct
size_t netdata_conf_cpus(void);                                                 // daemon/config/netdata-conf-global.h
size_t get_tier_grouping(size_t tier);                                          // daemon/config/netdata-conf-db.h
extern bool dbengine_use_direct_io;                                             // daemon/config/netdata-conf-db.h
extern bool pulse_enabled;                                                      // daemon/pulse/pulse.h
bool rrdhost_localhost_tier_is_dbengine(size_t tier);                           // database/rrdhost.c, a bridge until the
                                                                                // engine tracks its own active tiers

// telemetry the engine pushes into daemon/pulse                                                      -> published stats
void pulse_aral_register(ARAL *ar, const char *name);                           // daemon/pulse/pulse-aral.h
void pulse_aral_register_statistics(struct aral_statistics *stats, const char *name);
void pulse_aral_unregister_statistics(struct aral_statistics *stats);
void pulse_gorilla_hot_buffer_added(void);                                      // daemon/pulse/pulse-gorilla.h
void pulse_gorilla_tier0_page_flush(uint32_t actual, uint32_t optimal, uint32_t original);

// work the engine hands to the rest of the daemon                                                    -> hooks
void query_weights_worker_thread(void *arg);                                    // web/api/queries/weights.h
void rrdcontext_db_rotation(void);                                              // database/contexts/rrdcontext.h
size_t populate_metrics_from_database(void *mrg, void (*populate_cb)(void *mrg, Word_t section, nd_uuid_t *uuid));
                                                                                // database/sqlite/sqlite_metadata.h

#endif // NETDATA_RRDENGINE_DAEMON_H
