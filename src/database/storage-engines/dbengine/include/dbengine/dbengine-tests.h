// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_TESTS_H
#define NETDATA_DBENGINE_TESTS_H

#include "libnetdata/libnetdata.h"
#include "database/storage-engines/storage-engine-types.h"
#include "dbengine-config.h"

// The engine's self-tests and benchmarks. Each runs to completion on the calling thread.
//
// Four are command-line modes for the embedder and return a process exit code. None of them needs a tier or the
// engine; dbengine_page_test() and dbengine_cache_unittest() create a page cache and
// dbengine_metrics_registry_unittest() a throw-away engine holding a registry, so they take the configuration from
// the embedder; dbengine_metrics_registry_retention_benchmark() needs nothing.
//
// Five are for a test driver that adds their failed-check counts to its own: dbengine_cache_floor_unittest() needs
// no engine (it checks what the caches fall back to without a main cache), dbengine_null_engine_unittest()
// needs no engine (it checks that the engine's verbs and getters take NULL as an engine with nothing in it),
// dbengine_engine_lifecycle_unittest() runs before the daemon's own engine comes up (it makes, runs, stops and
// destroys engines of its own on scratch directories, one after the other and then two at once, and checks that
// nothing of one is left for, or taken from, another; it also pins the configuration surface: the defaults
// function against the initialiser, the file descriptor budget and its refusal, the over-long path refusal),
// dbengine_allocator_unittest() runs before the engine is up too (it brings the process-wide page allocators up
// with the given configuration's allocator settings and checks that a second, different configuration leaves them
// as built; the engine that comes up afterwards reuses them), and dbengine_zero_page_cadence_unittest() collects
// into and queries a tier the embedder brought up and hands it.

#ifdef __cplusplus
extern "C" {
#endif

int dbengine_page_test(const struct dbengine_config *cfg, int argc, char *argv[]);
int dbengine_cache_unittest(const struct dbengine_config *cfg);
int dbengine_metrics_registry_unittest(const struct dbengine_config *cfg);
int dbengine_metrics_registry_retention_benchmark(void);
int dbengine_cache_floor_unittest(void);
int dbengine_null_engine_unittest(void);
int dbengine_engine_lifecycle_unittest(const struct dbengine_config *cfg, const char *scratch_dir);
int dbengine_allocator_unittest(const struct dbengine_config *cfg);
int dbengine_zero_page_cadence_unittest(DBENGINE_ENGINE *engine, STORAGE_INSTANCE *si);

// dbengine_flush_all() (dbengine-api.h) that waits: returns true once every page of the tier that was hot or dirty
// when the call was made, and every extent any flusher had in flight, is on disk (the extent and its journal
// record written), which is the guarantee dbengine_tier_exit() gives; false, having queued nothing, for a NULL
// tier or an engine that is not serving (before dbengine_create() finished, or once dbengine_shutdown() started).
//
// The caller owns collector quiescence: dbengine_store_flush() or dbengine_store_finalize() on its handles first.
// The engine does not check, a live collector's page races the flush, and on a tier still being collected into
// the wait has no bound. Pages made hot after the call are not covered. On a tier that has not been quiesced the
// written extents' metadata reaches the open cache too; after dbengine_quiesce() the engine skips the open cache
// and the indexing flag on purpose (the daemon's shutdown shape). It waits on a pool thread that itself waits for
// the extent writes in the same pool: dbengine-config.h (libuv_worker_threads) says what that costs.
//
// Test-facing for now: the daemon's shutdown approximates this by polling the cache queues after the
// fire-and-forget verbs (daemon-shutdown.c), and if that wait is ever made exact this is the verb to graduate to
// dbengine-api.h; until then no embedder contract depends on it.
bool dbengine_flush_all_wait(DBENGINE_TIER *tier);

#ifdef __cplusplus
}
#endif

#endif // NETDATA_DBENGINE_TESTS_H
