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

// Where the engine's diagnostics go (log_sink in the configuration below). NULL, the default, is the behaviour the
// engine has always had: every line goes to netdata's logger on the daemon's source. A non-NULL sink takes them
// instead, and none of those lines reach the logger.
//
// - Called with log_sink_data verbatim, a severity, the emission site inside the engine (the __FILE__,
//   __FUNCTION__ and __LINE__ of the line that emitted, not of any wrapper), a printf format and its arguments as
//   a va_list. Format and va_list are valid for the call and not after it; a sink that keeps the message formats
//   its own copy, and one that reads the arguments twice va_copy()s first. Most formats carry no trailing
//   newline; the handful the engine writes while tearing its caches down do, because without a sink they are an
//   fprintf(stderr) and always have been. A sink that adds its own line ending should not assume either way.
// - Called from any thread the engine uses - the caller's own thread inside a public verb, the engine's event
//   loop, a libuv pool worker, a cache's eviction thread - and two calls may overlap. The sink serialises itself;
//   the engine takes no lock for it.
// - May be called with any engine lock held. A cache queue lock, a datafile users spinlock, a journalfile data
//   spinlock and a tier's datafile rwlock (held as a reader, while the engine walks a tier's datafile list) are
//   the ones to picture, but the list is not exhaustive and is not meant to be: treat it as "a lock is held".
//   A sink MUST NOT call back into the engine, and MUST NOT block on anything an engine thread can wait for: a
//   sink that blocks while a lock is held stalls whatever that lock guards, and one that blocks on a pool thread
//   can hang the process for good, for the reason libuv_worker_threads below gives.
// - Receives the message and nothing netdata's logger adds around it. In particular the logger annotates a line
//   on the daemon source with the caller's errno; a sink gets the format and the arguments only. It may read
//   errno itself during the call - the save and restore below make that well defined.
// - Must return. A sink that longjmp()s out, or ends the process, does it from wherever the engine happened to
//   be: mid-way through a rate limiter's window update, inside a guarded read of a mapped journal, holding a
//   cache queue lock. The engine has no way to finish what it was doing.
// - The engine saves and restores errno around the call, so a caller that reads errno after an engine verb is
//   unaffected by the sink.
// - May be called after dbengine_shutdown(), from inside dbengine_destroy(), and - when the engine was retained
//   because something was still referenced - from whichever thread later releases the last of it. The sink and
//   its data must stay callable until the embedder has released every handle it took from the engine (metric
//   handles, collection and query handles, anything holding a cache page, and the preload references
//   dbengine_preload_release() drops) AND a dbengine_destroy() has run after that. The return value alone does
//   not say when that is: it counts registry metrics only, so it can be 0 while a cache the engine still points
//   at holds referenced pages, and that cache can emit. Nothing else reports it either, so "nothing retained" is
//   not a state the embedder can observe - and an engine can be retained for a reason that is not the embedder's
//   doing. The rule an embedder can actually follow, and the one the engine's own test suite follows, is to give
//   the sink and its data the lifetime of the process.
// - Never called after the engine is freed, and never for what the engine cannot survive: fatal() and
//   internal_fatal() end the process through netdata's logger whatever this field holds.
// - Does not receive every line the engine's work produces, for two separate reasons.
//
//   Some lines have no engine to route through, and go to netdata's logger: the process-wide page-data layer,
//   the engine's file and decompression primitives, which sit below the level where an engine is in hand, the
//   public verbs that reject a NULL storage instance - which is exactly when there is no engine to ask - and a
//   failed protected read of a mapped journal, which libnetdata's own recovery reports.
//
//   That recovery is the host's to provide, and it is worth knowing you have it. Reading a mapped journal that
//   turns out to be truncated or unreadable raises SIGBUS, and the engine survives it only because a signal
//   handler routes the fault to libnetdata's protected-access check first (nd_initialize_signals() does this for
//   the daemon). An embedder that installs no such handler does not get a recovered read and a logged fault: it
//   gets the process, on a damaged file the engine would otherwise have skipped.
//
//   A sink is never handed a pointer into memory the engine is reading under a fault guard, and no line it
//   receives points into a mapped journal. That is what lets the engine emit while such a guard is armed: the
//   guard recovers only a fault inside the mapping it registered, so a sink faulting on its own memory is not
//   diverted into the engine's recovery path. A new emission site must keep it that way - format a value out of
//   a mapping rather than passing a pointer into one.
//
//   And some lines are dropped before the sink is reached, by the gate the emitting site has always had: a rate
//   limited site emits at most once per its own window, a debug site emits only when the matching debug flag is
//   set, and a build without internal checks compiles its internal-error sites out entirely. The sink decides
//   what to do with what it is given; it does not see what the site itself did not emit.
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

    dbengine_log_fn log_sink;                   // where this engine's diagnostics go; NULL = netdata's logger, as
                                                // it has always been. The contract is at dbengine_log_fn above
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
DBENGINE_ENGINE *dbengine_create(const struct dbengine_config *cfg);

// Release the references preload_metrics() left on the registry, once every tier has come up (after the last
// dbengine_readiness_wait()): until then they keep preloaded metrics from being evicted before their journals are
// read. A no-op when there is no engine or no registry.
void dbengine_preload_release(DBENGINE_ENGINE *engine);

#ifdef __cplusplus
}
#endif

#endif // NETDATA_DBENGINE_CONFIG_H
