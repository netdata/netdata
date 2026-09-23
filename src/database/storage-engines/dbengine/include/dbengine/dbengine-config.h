// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_CONFIG_H
#define NETDATA_DBENGINE_CONFIG_H

#include "libnetdata/libnetdata.h"

#ifdef __cplusplus
extern "C" {
#endif

struct dbengine_tier;
typedef struct dbengine_tier DBENGINE_TIER;

// The engine: its tiers and what they share (the event loop, the caches, the metrics registry, the configuration,
// the counters). Opaque outside the engine; made by dbengine_create(), stopped by dbengine_shutdown(), and released
// by dbengine_destroy() when no reference on a cache page or a registry metric remains (dbengine-api.h). A process
// may hold several. What they share is process-wide by design: the page allocator layer below and the tier page
// sizes it is built from, the page-details and extent-buffer allocators (dbengine-stats.h names them), the libuv
// worker pool (sized by the embedder, never by the engine: libuv_worker_threads below says what that means and
// what a wrong size does), and the process's file-descriptor limit, which each engine budgets against on its own.
// Calls to dbengine_create() and dbengine_destroy() are not thread-safe against one another: the embedder
// serialises them. That is all the serialisation is for; it has no bearing on the pool.
struct dbengine_engine;
typedef struct dbengine_engine DBENGINE_ENGINE;

// Receives one metric the embedder already knows, with the number of the tier it belongs to (the tier a
// dbengine_tier_init() with that number brings up; a number the engine does not have is fatal); passed to
// preload_metrics() by the engine. The embedder holds no engine while it runs (preload_metrics() is called from
// inside dbengine_create()), so it names the tier and the engine resolves it.
typedef void (*dbengine_preload_add_fn)(void *mrg, size_t tier, nd_uuid_t *uuid);

// Where the engine's diagnostics go (log_sink in the configuration below). With NULL, the default, each line keeps
// the route it had before there was a sink: netdata's logger on the daemon source, its debug source for the
// debug-flag lines, stderr for the teardown narration. With a sink, those lines go to the sink and not there.
//
// What the sink receives
// - log_sink_data verbatim; a severity (NDLP_ERR, NDLP_WARNING, NDLP_NOTICE, NDLP_INFO or NDLP_DEBUG); the __FILE__,
//   __FUNCTION__ and __LINE__ of the engine line that emitted; a printf format and its va_list. Both are valid only
//   during the call: copy the message to keep it, va_copy() to read the arguments twice.
// - The message only, nothing netdata's logger adds (such as its errno annotation). errno during the call is the
//   emitting site's; the engine restores the caller's errno after the sink returns.
// - Most formats end without a newline; the teardown narration and two file-check lines ("Not a regular file.",
//   "File length is too short.") end with one.
// - Every line past its site's own gate (below), whatever its severity and however many: no priority threshold and
//   no per-source flood limit stand in front of the sink. What to keep is the sink's decision.
//
// When and where it is called
// - From any engine thread - a caller's thread inside a public verb, the event loop, a libuv pool worker, a cache's
//   eviction thread - and calls may overlap. The sink serialises itself; the engine takes no lock for it.
// - With any engine lock possibly held: a cache queue lock, a datafile or journalfile spinlock, a tier's datafile
//   rwlock. The list is not exhaustive.
// - After dbengine_shutdown(), inside dbengine_destroy(), and, for an engine that destroy left retained, from
//   whichever thread later releases the last reference.
// - While a guarded read of a mapped journal is armed. The guard recovers a fault only at an address inside the
//   range it registered, and the engine keeps two rules that a new emission site or guarded read must keep too: no
//   line the sink receives points into a mapped journal (a site formats a value out of a mapping), and the sink is
//   never called while a guard is armed over a range the engine has unmapped (a failed unmap leaves the range
//   mapped, and is reported under its guard). A sink that faults on its own memory faults as it would unguarded.
//
// What the sink MUST NOT do
// - Call into any engine in the process.
// - Block on anything an engine thread may wait for. Blocking with an engine lock held stalls what that lock
//   guards; blocking a pool thread can hang the process (libuv_worker_threads below).
// - Take a lock that any thread may hold while it calls into an engine: that thread and the engine thread calling
//   the sink can deadlock. Its locks are leaf locks.
// - Fail to return: no longjmp(), exit() or C++ exception out of it. The engine is mid-operation - inside a rate
//   limiter's update or a guarded read, holding a lock - and cannot finish.
//
// What never reaches the sink
// - Lines with no engine to route through, which go to netdata's logger: the process-wide page-data layer, the
//   public verbs that reject a NULL storage instance, and what libnetdata logs itself while the engine uses it - a
//   fault it recovered in a guarded read (for some reads the only report of a journal the engine then skips), and
//   failures in its file-mapping and madvise() helpers, thread creation and joining, allocators and CPU detection.
// - fatal() and internal_fatal(), which end the process through netdata's logger.
// - Lines a site's own gate drops. A debug-flag site (D_RRDENGINE - not the NDLP_DEBUG lines, which every build
//   emits) emits only in a build with internal checks, and only while libnetdata's process-wide debug_flags asks for
//   it; an internal-error site only in a build with internal checks. A rate-limited site emits once per its window,
//   and the window belongs to the call site (or, for a thread-local limiter, the thread), not to an engine: one
//   engine's line can use the window another engine's same line needed, and two threads reaching a shared limiter
//   together can both get through. A site's first window is counted from boot, so on a host up for less than it the
//   first line is dropped too.
//
// How long it must live
// - Until every handle taken from the engine is released (metric, collection and query handles, anything holding a
//   cache page, the preload references dbengine_preload_release() drops) and one dbengine_destroy() has run after
//   that; destroy is called once. Its return value does not mark that point: it counts registry metrics only, and
//   an engine can stay retained for reasons that are not the embedder's. In practice, give the sink and its data
//   the lifetime of the process - in C++, storage that is never destroyed, since static destructors run at exit
//   while a retained engine's threads may still call the sink.
// - It is never called after the engine is freed.
typedef void (*dbengine_log_fn)(
        void *data,                             // log_sink_data, verbatim
        ND_LOG_FIELD_PRIORITY priority,
        const char *file, const char *function, unsigned long line,
        const char *fmt, va_list ap);

// The storage engine's configuration.
//
// Whoever embeds the engine (the daemon; a test) fills one of these from its own sources and
// hands it to dbengine_create(), which brings the engine up with it. The engine keeps a private
// copy and reads nothing else afterwards. Per-tier settings (path, quota, retention, page type)
// travel with dbengine_tier_init() instead.
// The page allocator layer's settings. The layer (the page allocators, their size classes and the gorilla
// counters) is process-wide and shared by every engine: the first engine to come up configures it with this group
// and later engines reuse it (a differing group is logged and ignored). The size classes also depend on the
// compile-time tier page sizes.
struct dbengine_allocator_config {
    size_t partitions;                          // allocator partitions; 0 = the engine's cpus
    bool arals_for_large_pages;                 // keep allocators for page sizes above 4KiB (a parent expects such pages)
    bool compression_statistics;                // count gorilla buffers and tier-0 compression bytes (pulse extended)
};

struct dbengine_config {
    // caches - read once, when dbengine_create() brings up the caches shared by all tiers
    size_t page_cache_mb;                       // [db] dbengine page cache size
    size_t extent_cache_mb;                     // [db] dbengine extent cache size
    uint64_t out_of_memory_protection_bytes;    // [db] dbengine out of memory protection; 0 disables it
    bool use_all_ram_for_caches;                // [db] dbengine use all ram for caches

    bool cache_statistics;                      // keep per-cache statistics (the daemon's pulse setting)

    // sizing of cache partitions, evictors, flushers and metric-registry loaders: this engine's concurrency width
    size_t cpus;                                // 0 = detect the system's cpus

    // the process-wide page allocator layer (see above)
    struct dbengine_allocator_config allocator;

    // files
    bool direct_io;                             // [db] dbengine use direct io
    unsigned pages_per_extent;                  // [db] dbengine pages per extent; 0 = default, > MAX_PAGES_PER_EXTENT is fatal
    bool journal_integrity_check;               // [db] dbengine enable journal integrity check
    time_t journal_v2_unmount_time_s;           // [db] dbengine journal v2 unmount time
    size_t max_reserved_file_descriptors;       // the file descriptors this engine may reserve for its tiers, a
                                                // fixed number per tier (dbengine_tier_init() returns UV_EMFILE when
                                                // one more would exceed it); 0 = a quarter of the process's soft
                                                // RLIMIT_NOFILE as libnetdata read it (rlimit_nofile), taken once when
                                                // dbengine_create() resolves the configuration. Per engine: several
                                                // engines each budget on their own against the one process table

    // runtime
    time_t default_update_every_s;              // used for a metric whose own update_every is unknown; 0 = 1, < 0 is fatal

    // The libuv worker pool. There is one per process, and libuv sizes it once, at the process's first use of it,
    // from the UV_THREADPOOL_SIZE environment variable (4 threads when unset); nothing resizes it afterwards. The
    // engine dispatches every piece of work it takes off its event loop into it: extent writes and reads, flushes,
    // the registry loads of a tier coming up, journal indexing, datafile rotation, query preparation (a query at
    // STORAGE_PRIORITY_SYNCHRONOUS runs its preparation and its reads on the caller's own thread and never touches
    // the pool; one at STORAGE_PRIORITY_SYNCHRONOUS_FIRST prepares and reads its first extent there and queues the
    // rest). The engine neither
    // sizes the pool nor can read its size: libuv_worker_threads is what the embedder tells it the size is, and it
    // must equal the real one, so the embedder sets UV_THREADPOOL_SIZE to the same number before the process first
    // touches the pool (the daemon does, from the variable it hands here: netdata-conf-global.c). The engine counts
    // the work it has in flight against that figure: once no more than reserved_libuv_worker_threads are left it
    // dispatches only its own internal-priority work (queries and extent reads wait), so that the embedder's own
    // uv_queue_work() calls find a thread. Its internal-priority work is never held back by the count, so the
    // reservation is best effort: it keeps the engine's lower-priority work off those threads, and nothing keeps
    // its flushes, loads and rotations off them.
    //
    // Why the figure must be right: two of the engine's own work items wait, on the pool thread they hold, for a
    // work item they queued behind them (a flush waits for its extent write, pagecache.c; a tier's registry load
    // waits for the per-datafile loaders it queued, rrdengine.c), two more wait there for extent writes already in
    // flight (a tier's shutdown, and a flush the caller waits on through dbengine_flush_all_wait(),
    // dbengine-tests.h; both in rrdengine.c), and all of them run at the internal priority the count
    // never holds back. On a pool smaller than libuv_worker_threads every thread can end up held by a waiting
    // parent whose child can never run, and the process hangs, silently and for good. With the two figures equal
    // the daemon is safe by its usual numbers, not by a rule the engine enforces: it asks for a pool of cpus x 6
    // threads against roughly cpus + 2 x tiers such parents, but a memory cap or the "libuv worker threads"
    // setting (netdata-conf-global.c) can bring the pool down to its floor (16; 8 on a 32-bit build) while the
    // flush parents it can have stay capped by cpus (pgc_max_flushers()), so a many-cpu host with a floor-sized
    // pool is exposed as well. The rule the engine's work items should keep, and these four do not yet: no work
    // item waits for another work item while it holds a pool thread.
    int libuv_worker_threads;                   // the pool's size, as the embedder set it; 0 = the engine's compiled
                                                // default (16; 8 on a 32-bit build), which is then the number the
                                                // embedder must have set UV_THREADPOOL_SIZE to
    int reserved_libuv_worker_threads;          // pool threads the engine must leave free for the embedder's own work

    // services the embedder may provide; NULL = not provided
    void (*on_db_rotation)(void);               // a tier deleted its oldest datafile: retention just shrank
    size_t (*preload_metrics)(void *mrg, dbengine_preload_add_fn add);
                                                // called once, when the metrics registry is created and before any
                                                // tier loads its journals: feed every metric uuid the embedder already
                                                // knows through add(), so the registry is populated in one pass instead
                                                // of metric by metric as the journals are read; returns the count

    dbengine_log_fn log_sink;                   // where this engine's diagnostics go; NULL keeps each line on its
                                                // original route. The contract is at dbengine_log_fn above
    void *log_sink_data;                        // handed to log_sink unchanged; the engine never reads it
};

#define DBENGINE_DEFAULT_PAGES_PER_EXTENT (109)

// Page types. The value is the page-type byte of the on-disk format (rrddiskprotocol.h), so an
// existing type is never renumbered; a new one takes the next value.
#define DBENGINE_PAGE_TYPE_ARRAY_32BIT    (0)
#define DBENGINE_PAGE_TYPE_ARRAY_TIER1    (1)
#define DBENGINE_PAGE_TYPE_GORILLA_32BIT  (2)

// the floor the engine enforces on a tier's disk space (dbengine_tier_init() raises a smaller value), the floor
// the embedder is expected to keep the page cache above (the engine does not check it: below it the cache split
// underflows), and the disk-space default the engine leaves to the embedder
#define DBENGINE_MIN_PAGE_CACHE_SIZE_MB (8)
#define DBENGINE_MIN_DISK_SPACE_MB (25)
#define DBENGINE_DEFAULT_TIER_DISK_SPACE_MB (1024)

#if defined(ENV32BIT)
#define DBENGINE_CONFIG_DEFAULT_PAGE_CACHE_MB (16)
#else
#define DBENGINE_CONFIG_DEFAULT_PAGE_CACHE_MB (32)
#endif

// The compiled defaults: the baseline a caller adjusts before dbengine_create(), which resolves the
// 0-means-default fields (cpus, allocator.partitions, default_update_every_s, pages_per_extent,
// libuv_worker_threads, max_reserved_file_descriptors) to concrete values.
#define DBENGINE_CONFIG_DEFAULTS {                              \
    .page_cache_mb = DBENGINE_CONFIG_DEFAULT_PAGE_CACHE_MB,     \
    .extent_cache_mb = 0,                                       \
    .out_of_memory_protection_bytes = 0,                        \
    .use_all_ram_for_caches = false,                            \
    .cache_statistics = true,                                   \
    .cpus = 0,                                                  \
    .allocator = {                                              \
        .partitions = 0,                                        \
        .arals_for_large_pages = false,                         \
        .compression_statistics = false,                        \
    },                                                          \
    .direct_io = true,                                          \
    .pages_per_extent = DBENGINE_DEFAULT_PAGES_PER_EXTENT,      \
    .journal_integrity_check = false,                           \
    .journal_v2_unmount_time_s = 120,                           \
    .max_reserved_file_descriptors = 0,                         \
    .default_update_every_s = 1,                                \
    .libuv_worker_threads = 0,                                  \
    .reserved_libuv_worker_threads = 0,                         \
    .on_db_rotation = NULL,                                     \
    .preload_metrics = NULL,                                    \
    .log_sink = NULL,                                           \
    .log_sink_data = NULL,                                      \
}

// the same defaults as a value, for a caller that cannot use the initialiser (one that fills the struct at run
// time, or from another language)
struct dbengine_config dbengine_config_defaults(void);

// One tier's configuration, handed to dbengine_tier_init(); the engine copies what it needs.
// the longest dbfiles_path a tier takes: the engine appends the names of its datafiles and journals (at most 31
// characters, "/journalfile-<tier>-<number>.njfv2") to it in buffers of FILENAME_MAX + 1, so a path that leaves
// them no room is refused by dbengine_tier_init() (UV_ENAMETOOLONG) rather than silently truncated; 64 is the room
// kept
#define DBENGINE_DBFILES_PATH_MAX (FILENAME_MAX - 64)

struct dbengine_tier_config {
    size_t tier;                                // 0 is the tier collectors write to; higher tiers aggregate the one below
    const char *dbfiles_path;                   // directory of this tier's datafiles and journals; at most
                                                // DBENGINE_DBFILES_PATH_MAX characters
    unsigned disk_space_mb;                     // 0 = no disk quota
    time_t max_retention_s;                     // 0 = no time limit
    uint8_t page_type;                          // tier 0: DBENGINE_PAGE_TYPE_GORILLA_32BIT or
                                                // DBENGINE_PAGE_TYPE_ARRAY_32BIT; higher tiers hold aggregates and
                                                // must use DBENGINE_PAGE_TYPE_ARRAY_TIER1
    size_t grouping;                            // points of tier 0 that make one point of this tier (1 for tier 0)
};

// Bring an engine up: copy cfg into it, resolving the 0-means-default fields (cpus, default_update_every_s,
// pages_per_extent, libuv_worker_threads, max_reserved_file_descriptors), then create the event loop, the caches, the metrics registry (which
// preloads through cfg->preload_metrics, so the embedder's tier count must be final by now) and the engine's
// thread, and start taking work. The only way up; tiers come after it. Fatal when the libuv pool is not larger
// than the threads reserved for the embedder, when pages_per_extent exceeds what the extent format holds, or
// when default_update_every_s is negative. Returns the engine, or NULL, with the reason logged: the libuv error
// that stopped the loop from coming up (nothing is left behind, as if never called). Another engine, live,
// stopped or retained, is no obstacle: each has its tiers of its own.
//
// A mapped journal that turns out to be truncated or unreadable raises SIGBUS or SIGSEGV while the engine reads it,
// and the engine recovers - skipping or rebuilding that file - only if the process's handler for those signals is
// installed with SA_SIGINFO and calls libnetdata's signal_protected_access_check() first, as the daemon's does
// (nd_initialize_signals()). With no handler the fault terminates the process; a handler that does not make that
// call gets no recovery either.
DBENGINE_ENGINE *dbengine_create(const struct dbengine_config *cfg);

// Release the references preload_metrics() left on the registry, once every tier has come up (after the last
// dbengine_readiness_wait()): until then they keep preloaded metrics from being evicted before their journals are
// read. A no-op when there is no engine or no registry.
void dbengine_preload_release(DBENGINE_ENGINE *engine);

#ifdef __cplusplus
}
#endif

#endif // NETDATA_DBENGINE_CONFIG_H
