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
// destroys two engines of its own on scratch directories, one after the other and then both at once, and checks
// that nothing of one is left for, or taken from, the other),
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

#ifdef __cplusplus
}
#endif

#endif // NETDATA_DBENGINE_TESTS_H
