// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_STATS_H
#define NETDATA_DBENGINE_STATS_H

// libnetdata.h provides PAD64 plus the fixed-width integer and bool
// typedefs the struct fields use.
#include "libnetdata/libnetdata.h"

// Forward decls keep this header from pulling in libnetdata's aral.h.
// Engine and daemon TUs that consume the struct already have the real
// types reachable through their own include graphs.
struct aral;
struct aral_statistics;

// Engine-published statistics, read by the daemon's pulse subsystem once
// per pulse cycle and forwarded into its existing charts. The engine
// owns the storage (file-static in rrdengineapi.c); external readers
// must treat the pointer returned by dbengine_get_stats() as read-only.
//
// This replaces direct calls from the engine into pulse_aral_*,
// pulse_gorilla_*, and pulse_enabled. Engine sources stay free of any
// pulse_ symbol.

typedef struct dbengine_aral_registration {
    struct aral *ar;                 // may be NULL for stats-only registrations
    struct aral_statistics *stats;   // NULL means the slot is unused -- either
                                     // never registered yet, or a tombstone
                                     // left by a previous unregister. Either
                                     // way, register_slot may reuse it.
    const char *name;
} dbengine_aral_registration_t;

// 13 callsites today; 16 leaves headroom for one or two more before any
// future re-tuning is needed.
#define DBENGINE_MAX_ARAL_REGISTRATIONS 16

typedef struct dbengine_stats {
    // ARAL registrations populated at engine init time. The engine
    // serializes all writes (and the daemon's snapshot reads) under an
    // internal spinlock -- callers must not touch this array directly;
    // use dbengine_stats_snapshot_arals() instead.
    dbengine_aral_registration_t arals[DBENGINE_MAX_ARAL_REGISTRATIONS];
    size_t aral_count;

    // Hot-path Gorilla counters: engine increments atomically, daemon
    // pulse-gorilla.c reads atomically. PAD64 prevents false sharing.
    struct {
        PAD64(uint64_t) hot_buffers_added;
        PAD64(uint64_t) tier0_disk_actual_bytes;
        PAD64(uint64_t) tier0_disk_optimal_bytes;
        PAD64(uint64_t) tier0_disk_original_bytes;
    } gorilla;
} dbengine_stats_t;

// Read-only view for daemon-side consumers. Pointer is stable for the
// lifetime of the process. Use dbengine_stats_snapshot_arals() for the
// ARAL array; the gorilla counters can be read directly with atomic
// loads.
const dbengine_stats_t *dbengine_get_stats(void);

// One element of an ARAL registration snapshot.
typedef struct dbengine_aral_snapshot {
    struct aral_statistics *stats;
    const char *name;
} dbengine_aral_snapshot_t;

// Atomically snapshot the live (non-tombstone) ARAL registrations into
// out_buf, which must point to an array of at least
// DBENGINE_MAX_ARAL_REGISTRATIONS elements. Returns the number of
// entries written. Takes the engine's internal lock for the duration
// of the copy so each (stats, name) pair in the snapshot is consistent;
// the caller iterates the local snapshot without further locking.
size_t dbengine_stats_snapshot_arals(dbengine_aral_snapshot_t *out_buf);

// Mirrors the daemon's pulse_enabled flag into the engine. Called by
// the daemon at the same points it sets pulse_enabled. Defaults to true
// so a standalone link (no daemon) collects stats unless told otherwise.
void dbengine_set_collect_stats(bool enabled);

// Read at cache creation to decide whether to maintain expensive
// per-page statistics. Replaces the engine's previous direct read of
// pulse_enabled.
bool dbengine_collect_stats(void);

// Mirrors the daemon's pulse_extended_enabled flag into the engine.
// Gates the gorilla hot-path counters so they only perform atomic
// increments when the daemon is collecting extended pulse data.
// Defaults to false to match the daemon's pulse_extended_enabled
// startup default; a standalone link that wants gorilla counters
// active must call this setter explicitly.
void dbengine_set_collect_extended_stats(bool enabled);

// Read on the gorilla hot path. Cheap RELAXED load.
bool dbengine_collect_extended_stats(void);

// Engine-internal helpers, called from engine init paths and hot paths
// in place of the previous pulse_aral_* / pulse_gorilla_* calls.
void dbengine_stats_register_aral(struct aral *ar, const char *name);
void dbengine_stats_register_aral_statistics(struct aral_statistics *stats, const char *name);
void dbengine_stats_unregister_aral_statistics(struct aral_statistics *stats);
void dbengine_stats_gorilla_hot_buffer_added(void);
void dbengine_stats_gorilla_tier0_page_flush(uint32_t actual, uint32_t optimal, uint32_t original);

#endif // NETDATA_DBENGINE_STATS_H
