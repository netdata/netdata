// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"

#define MAX_PREPARED_THREAD_STATEMENTS (32)

static SPINLOCK JudyL_thread_stmt_lock = SPINLOCK_INITIALIZER;
static Pvoid_t JudyL_thread_stmt_pool = NULL;

// Per-thread cache of prepared statements, and the contract that governs its lifetime.
//
// THE RULES (all four are load-bearing; breaking any one reintroduces a shutdown crash):
//
//  1. NO DECISION ABOUT POOL OWNERSHIP is made on an unlocked read of
//     sqlite_databases_closed. An unlocked pre-check in
//     finalize_self_prepared_sql_statements() was the double-free: the reader could be told
//     "not closed", block on the lock, and free a pool that the teardown thread had already
//     freed. (#20731 added such a pre-check after #20536 had removed the sibling one from that
//     same function. Do not add it back.)
//     Note this rule is about POOL OWNERSHIP only. The flag is deliberately read unlocked
//     elsewhere, where a stale answer is harmless: REQUIRE_HEALTH_DB_OPEN() uses it as a hint,
//     and sqlite_close_databases() stores it before taking the lock so that new work is
//     refused as early as possible.
//  2. Every find and mutation of a JudyL_thread_stmt_pool entry happens under
//     JudyL_thread_stmt_lock. The owner DETACHES its entry under that lock and then finalizes
//     and frees the detached pool with no lock held, inside a lifetime lease (see SQLITE
//     LIFETIME below): sqlite3_finalize() takes the connection mutex and must not run under a
//     lock other threads spin on.
//  3. THE JUDYL ARRAY IS THE AUTHORITY for pool lifetime, not thread_stmt_pool. Self cleanup
//     frees only the entry the array maps for gettid_cached() and deletes that key. Nothing
//     else frees a pool at all: finalize_all_prepared_sql_statements() only REPORTS the pools
//     it finds and suppresses the teardown (see rule 4), so a pool is only ever released by
//     its own owner. thread_stmt_pool is a same-thread cache for prepare_statement(); the
//     lookup, not the cache, decides what exists.
//  4. NO TEARDOWN PATH WRITES ANOTHER THREAD'S THREAD-LOCAL STORAGE - not the pool
//     pointer, not the caller's cached sqlite3_stmt * slot. It is tempting to have the
//     finalizer NULL the owner's slots so they cannot be reused after finalization, but
//     finalize_all_prepared_sql_statements() exists precisely for pools whose owner did
//     NOT clean up, and such an owner's TLS block may no longer have a valid lifetime.
//     Holding the locks does not prolong it.
//
// WHAT PROTECTS A STATEMENT THAT IS STILL IN USE is not this struct - it is
// sqlite_teardown_unsafe (see below). When any pooled-statement owner might still be
// running, teardown is suppressed entirely and nothing is finalized out from under it.
//
// THE OWNER SET IS AN ENUMERATED INVARIANT, NOT ONE THIS CODE ENFORCES. Today the only threads
// that register here are HEALTH (the PREPARE_COMPILED_STATEMENT sites in sqlite_health.c and
// sqlite_aclk_alert.c are all gated on is_health_thread) and the ML threads - both the TRAIN[N]
// workers and the detection thread reach the direct prepare_statement() sites in ml.cc, which
// is why both call finalize_self_prepared_sql_statements() at the end of their loops. Every ML
// thread is joined by ml_stop_threads() before teardown; health cleans up from a
// CLEANUP_FUNCTION handler.
// So in a correct shutdown finalize_all_prepared_sql_statements() finds NOTHING and is a
// pure diagnostic. IF YOU ADD A PREPARE_COMPILED_STATEMENT CALLER ON A THREAD THAT IS
// NEITHER JOINED BEFORE TEARDOWN NOR GUARANTEED TO RUN
// finalize_self_prepared_sql_statements(), you MUST extend the liveness check that sets
// sqlite_teardown_unsafe, or you reintroduce the use-after-free.
struct stmt_pool_s {
    int count;
    bool overflow_reported;
    pid_t thread_id;
    char *name;
    void *stmt[MAX_PREPARED_THREAD_STATEMENTS];
};

// Same-thread cache only. Rule 3: the JudyL array, not this pointer, decides what exists.
__thread struct stmt_pool_s *thread_stmt_pool = NULL;

long long def_journal_size_limit = 16777216;

bool sqlite_library_initialized;
bool sqlite_databases_closed;

// One-way latch: "someone may still be using SQLite, so tearing it down would be a
// use-after-free". Set on every shutdown path that gives up waiting for a SQLite user
// instead of proving it finished, and checked by every teardown entry point -
// ml_fini(), sqlite_close_databases() and sqlite_library_shutdown().
//
// When it is set we deliberately leak: the METADATA, CONTEXT and ML handles stay open,
// statements stay unfinalized and the library stays initialized until the process exits and the
// OS reclaims everything. (Thread-local handles are the exception: sql_close_thread_db_safe()
// still closes those while the library is up, which is safe because they are private to a single
// thread. Once sqlite_library_shutdown() has started, it leaks them instead and latches this flag.)
// That is strictly safer than crashing inside pcache1 - the same reasoning that already
// governs sql_close_thread_db_safe() below. sqlite_databases_closed is still set in that
// case, so no NEW work is admitted; only the destruction is skipped.
static bool sqlite_teardown_unsafe = false;

// A SEPARATE, narrower signal: a connection was closed with statements still attached, so it
// became a zombie. That makes sqlite3_shutdown() unsafe - it would dismantle pcache1 while the
// zombie still references pages - but it says nothing about whether anyone is still USING
// db_meta, so it must not suppress the database closes.
//
// Keeping the two apart matters because this one is reachable at STARTUP:
// sql_close_thread_db_safe() runs on the context-load path during boot. Folding it into the
// liveness latch would let one leaked statement there suppress every teardown for the entire
// life of the process, with a single warning to show for it.
static bool sqlite_zombie_connection_created = false;

// When, and for which database, the first zombie was noted. Recorded ONLY so the eventual
// "skipping sqlite3_shutdown()" line can say whether the suppression came from this shutdown or
// from something that happened hours earlier at startup.
//
// Both are ATOMIC, and the ORDER matters: they are written BEFORE the flag is published, and
// read AFTER it is observed. sqlite_note_zombie_connection() runs with no lock held, and the
// reader in sqlite_library_shutdown() formats this message outside any lock - so a thread-local
// close noting the first zombie can run concurrently with that formatting. Publishing the flag
// first would let the reader see "a zombie exists" and then read an unwritten name and timestamp.
//
// KNOWN LIMITATION, deliberate: this latch is sticky for the life of the process even if the
// deferred close later completes (the owning thread finalizes its statements and the connection
// really does go away). It cannot be cleared safely - once sqlite3_close_v2() has been called,
// touching that handle to re-test it is undefined - and the trade is one-sided: skipping
// sqlite3_shutdown() costs a teardown the exiting process does not need, while clearing it
// wrongly reinstates the pcache1 crash it exists to prevent. The log line below is how you tell
// a stale suppression from a live one.
static usec_t sqlite_zombie_noted_ut = 0;
static const char *sqlite_zombie_noted_db = NULL;

// Every distinct reason is logged, not just the first: when several conditions fire, the list
// is what tells a triage pass which one actually drove the decision.
void sqlite_mark_teardown_unsafe(const char *reason)
{
    bool was_set = __atomic_exchange_n(&sqlite_teardown_unsafe, true, __ATOMIC_RELEASE);

    nd_log_daemon(
        NDLP_WARNING,
        "SQL: %s SQLite teardown: %s. Databases, statements and library state are leaked "
        "deliberately; the process is exiting.",
        was_set ? "also suppressing" : "suppressing",
        reason ? reason : "a SQLite user may still be running");
}

bool sqlite_teardown_is_unsafe(void)
{
    return __atomic_load_n(&sqlite_teardown_unsafe, __ATOMIC_ACQUIRE);
}

// --------------------------------------------------------------------------------------------------------------------
// SQLITE LIFETIME
//
// No global lock is held across a SQLite call. SQLite is serialized (see sqlite_note_zombie_connection), so
// every prepare, finalize and close waits for its connection's mutex; holding a process-wide lock across such
// a call let one slow statement on METADATA stall every SQLite user in the agent - including exiting ML
// threads, which turned into shutdown watchdog aborts, and 3600s spinlock deadlock fatals during normal
// running. Instead, each operation that uses a shared handle or the library takes a LEASE:
//
//  - prepare_statement() / simple_prepare_statement(): a DATABASE lease, refused once the databases are
//    closed or a teardown has started. A pooled statement is registered before the lease is released.
//  - finalize_self_prepared_sql_statements(): a CLEANUP lease, refused once a teardown has started. A refused
//    owner leaves its pool registered, and the teardown walk that runs after the gate reports it and
//    suppresses the teardown.
//  - sql_close_thread_db_safe(): a LIBRARY lease, refused once the library is shut down or shutting down. A
//    refused close of a live handle latches sqlite_teardown_unsafe, so the library is not shut down under it.
//
// sqlite_close_databases() and sqlite_library_shutdown() set their gate, then drain the leases with a deadline.
// On timeout they latch sqlite_teardown_unsafe and leak (the existing policy) instead of destroying anything
// under an operation that is still inside SQLite. sqlite_lifetime_mutex guards only the flags and the counter
// and is never held across a SQLite call, except sqlite3_initialize() and sqlite3_shutdown(), which take no
// connection mutex. Lock order: sqlite_lifetime_mutex before JudyL_thread_stmt_lock.
//
// Scope: a slow statement still delays the other users of ITS connection, which share that connection's
// mutex; it no longer delays anyone else.

#define SQLITE_LEASE_DRAIN_TIMEOUT_UT (5 * USEC_PER_SEC)

static netdata_mutex_t sqlite_lifetime_mutex;
static netdata_cond_t sqlite_lifetime_cond;
static size_t sqlite_leases = 0;
static bool sqlite_teardown_gate = false;       // sqlite_close_databases() has started draining
static bool sqlite_library_gate = false;        // sqlite_library_shutdown() has started draining

static void __attribute__((constructor)) sqlite_lifetime_init(void) {
    netdata_mutex_init(&sqlite_lifetime_mutex);
    netdata_cond_init(&sqlite_lifetime_cond);
}

// Set only by sqlite_lease_unittest(): tells it that a thread now holds a database lease, i.e. that it is entering
// sqlite3_prepare_v2(). NULL in production.
static void (*sqlite_lease_acquired_test_hook)(void) = NULL;

static bool sqlite_lease_acquire_database(void)
{
    netdata_mutex_lock(&sqlite_lifetime_mutex);
    bool ok = !__atomic_load_n(&sqlite_databases_closed, __ATOMIC_ACQUIRE) &&
              !sqlite_teardown_gate && !sqlite_library_gate;
    if (ok)
        sqlite_leases++;
    netdata_mutex_unlock(&sqlite_lifetime_mutex);

    void (*hook)(void) = __atomic_load_n(&sqlite_lease_acquired_test_hook, __ATOMIC_ACQUIRE);
    if (unlikely(ok && hook))
        hook();

    return ok;
}

static bool sqlite_lease_acquire_cleanup(void)
{
    netdata_mutex_lock(&sqlite_lifetime_mutex);
    bool ok = !sqlite_teardown_gate && !sqlite_library_gate;
    if (ok)
        sqlite_leases++;
    netdata_mutex_unlock(&sqlite_lifetime_mutex);
    return ok;
}

// Returns false when the caller must leak its handle instead of closing it.
static bool sqlite_lease_acquire_library(void)
{
    netdata_mutex_lock(&sqlite_lifetime_mutex);
    bool ok = sqlite_library_initialized && !sqlite_library_gate;
    bool refused_live = sqlite_library_initialized && sqlite_library_gate;
    if (ok)
        sqlite_leases++;
    else if (refused_live)
        // Latch under the mutex: sqlite_library_shutdown() re-checks the latch under it after its drain,
        // so it cannot miss a handle we are about to leave open.
        __atomic_store_n(&sqlite_teardown_unsafe, true, __ATOMIC_RELEASE);
    netdata_mutex_unlock(&sqlite_lifetime_mutex);

    if (refused_live)
        nd_log_daemon(NDLP_WARNING,
                      "SQL: a thread-local database was left open because the SQLite library is shutting down; "
                      "suppressing sqlite3_shutdown()");
    return ok;
}

static void sqlite_lease_release(void)
{
    netdata_mutex_lock(&sqlite_lifetime_mutex);
    if (--sqlite_leases == 0)
        netdata_cond_broadcast(&sqlite_lifetime_cond);
    netdata_mutex_unlock(&sqlite_lifetime_mutex);
}

// Must be called with sqlite_lifetime_mutex held, after a gate is set. Returns true when no lease is active.
static bool sqlite_leases_drain_locked(usec_t timeout_ut)
{
    usec_t deadline = now_monotonic_usec() + timeout_ut;
    while (sqlite_leases) {
        usec_t now = now_monotonic_usec();
        if (now >= deadline)
            break;
        (void) netdata_cond_timedwait(&sqlite_lifetime_cond, &sqlite_lifetime_mutex, (deadline - now) * NSEC_PER_USEC);
    }
    return sqlite_leases == 0;
}

SQLITE_API int sqlite3_exec_monitored(
    sqlite3 *db,                               /* An open database */
    const char *sql,                           /* SQL to be evaluated */
    int (*callback)(void*,int,char**,char**),  /* Callback function */
    void *data,                                /* 1st argument to callback */
    char **errmsg                              /* Error msg written here */
) {
    internal_fatal(!nd_thread_runs_sql(), "THIS THREAD CANNOT RUN SQL");

    int rc = sqlite3_exec(db, sql, callback, data, errmsg);
    pulse_sqlite3_query_completed(rc == SQLITE_OK, rc == SQLITE_BUSY, rc == SQLITE_LOCKED);
    return rc;
}

SQLITE_API int sqlite3_step_monitored(sqlite3_stmt *stmt) {
    internal_fatal(!nd_thread_runs_sql(), "THIS THREAD CANNOT RUN SQL");

    int rc;
    int cnt = 0;

    while (cnt++ < SQL_MAX_RETRY) {
        rc = sqlite3_step(stmt);
        switch (rc) {
            case SQLITE_DONE:
                pulse_sqlite3_query_completed(1, 0, 0);
                break;
            case SQLITE_ROW:
                pulse_sqlite3_row_completed();
                break;
            case SQLITE_BUSY:
            case SQLITE_LOCKED:
                pulse_sqlite3_query_completed(false, rc == SQLITE_BUSY, rc == SQLITE_LOCKED);
                sleep_usec(SQLITE_INSERT_DELAY * USEC_PER_MS);
                continue;
            default:
                break;
        }
        break;
    }
    return rc;
}

static bool mark_database_to_recover(sqlite3_stmt *res, sqlite3 *database, int rc)
{

    if (!res && !database)
        return false;

    if (!database)
        database = sqlite3_db_handle(res);

    if (db_meta == database) {
        char recover_file[FILENAME_MAX + 1];
        snprintfz(recover_file, FILENAME_MAX, "%s/.netdata-meta.db.%s", netdata_configured_cache_dir, SQLITE_CORRUPT == rc ? "recover" : "delete" );
        int fd = open(recover_file, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, 0600);
        if (fd >= 0) {
            close(fd);
            return true;
        }
    }
    return false;
}

int execute_insert(sqlite3_stmt *res) {
    int rc;
    rc =  sqlite3_step_monitored(res);
    if (rc == SQLITE_CORRUPT) {
        (void)mark_database_to_recover(res, NULL, rc);
        error_report("SQLite error %d", rc);
    }
    return rc;
}

int configure_sqlite_database(sqlite3 *database, int target_version, const char *description)
{
    char buf[1024 + 1] = "";
    const char *list[2] = { buf, NULL };

    const char *def_auto_vacuum = "INCREMENTAL";
    const char *def_synchronous = "NORMAL";
    const char *def_journal_mode = "WAL";
    const char *def_temp_store = "MEMORY";
    long long def_cache_size = -2000;

    // https://www.sqlite.org/pragma.html#pragma_auto_vacuum
    // PRAGMA schema.auto_vacuum = 0 | NONE | 1 | FULL | 2 | INCREMENTAL;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA auto_vacuum=%s", def_auto_vacuum);
    if (inicfg_exists(&netdata_config, CONFIG_SECTION_SQLITE, "auto vacuum"))
        snprintfz(buf, sizeof(buf) - 1, "PRAGMA auto_vacuum=%s",inicfg_get(&netdata_config, CONFIG_SECTION_SQLITE, "auto vacuum", def_auto_vacuum));
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_synchronous
    // PRAGMA schema.synchronous = 0 | OFF | 1 | NORMAL | 2 | FULL | 3 | EXTRA;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA synchronous=%s", def_synchronous);
    if (inicfg_exists(&netdata_config, CONFIG_SECTION_SQLITE, "synchronous"))
        snprintfz(buf, sizeof(buf) - 1, "PRAGMA synchronous=%s", inicfg_get(&netdata_config, CONFIG_SECTION_SQLITE, "synchronous", def_synchronous));
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_journal_mode
    // PRAGMA schema.journal_mode = DELETE | TRUNCATE | PERSIST | MEMORY | WAL | OFF
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA journal_mode=%s", def_journal_mode);
    if (inicfg_exists(&netdata_config, CONFIG_SECTION_SQLITE, "journal mode"))
        snprintfz(buf, sizeof(buf) - 1, "PRAGMA journal_mode=%s", inicfg_get(&netdata_config, CONFIG_SECTION_SQLITE, "journal mode", def_journal_mode));
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_temp_store
    // PRAGMA temp_store = 0 | DEFAULT | 1 | FILE | 2 | MEMORY;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA temp_store=%s", def_temp_store);
    if (inicfg_exists(&netdata_config, CONFIG_SECTION_SQLITE, "temp store"))
        snprintfz(buf, sizeof(buf) - 1, "PRAGMA temp_store=%s", inicfg_get(&netdata_config, CONFIG_SECTION_SQLITE, "temp store", def_temp_store));
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_journal_size_limit
    // PRAGMA schema.journal_size_limit = N ;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA journal_size_limit=%lld", def_journal_size_limit);
    if (inicfg_exists(&netdata_config, CONFIG_SECTION_SQLITE, "journal size limit")) {
        def_journal_size_limit = inicfg_get_number(&netdata_config, CONFIG_SECTION_SQLITE, "journal size limit", def_journal_size_limit);
        snprintfz(buf, sizeof(buf) - 1, "PRAGMA journal_size_limit=%lld", def_journal_size_limit);
    }
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_cache_size
    // PRAGMA schema.cache_size = pages;
    // PRAGMA schema.cache_size = -kibibytes;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA cache_size=%lld", def_cache_size);
    if (inicfg_exists(&netdata_config, CONFIG_SECTION_SQLITE, "cache size"))
        snprintfz(buf, sizeof(buf) - 1, "PRAGMA cache_size=%lld", inicfg_get_number(&netdata_config, CONFIG_SECTION_SQLITE, "cache size", def_cache_size));
    if (init_database_batch(database, list, description))
        return 1;

    snprintfz(buf, sizeof(buf) - 1, "PRAGMA user_version=%d", target_version);
    if (init_database_batch(database, list, description))
        return 1;

    snprintfz(buf, sizeof(buf) - 1, "PRAGMA optimize=0x10002");
    if (init_database_batch(database, list, description))
        return 1;

    return 0;
}

static void finalize_and_free_stmt_list(struct stmt_pool_s *stmt_list)
{
    if (!stmt_list)
        return;

    // count is bounded by prepare_statement(), which refuses to register past
    // MAX_PREPARED_THREAD_STATEMENTS instead of incrementing and dropping silently.
    for (int i = 0; i < stmt_list->count; i++) {
        if (!stmt_list->stmt[i])
            continue;
        int rc = sqlite3_finalize((sqlite3_stmt *)stmt_list->stmt[i]);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to finalize statement, rc = %d", rc);
        stmt_list->stmt[i] = NULL;
    }
    freez(stmt_list->name);
    freez(stmt_list);
}

// This must be called when the thread terminates.
//
// There is deliberately NO sqlite_databases_closed check here - see rule 1 at the top of this file.
// The only gate is the cleanup lease, which is refused once a teardown has started; that decides
// whether we may FINALIZE, never who owns the pool (the JudyL lookup below decides that).
//
// An unlocked closed check used to be at the head of this function and was the double free: this
// thread read "not closed", waited for the lock, and then freed a pool that the teardown thread had
// already freed and removed.
//
// That teardown-side free no longer exists - finalize_all_prepared_sql_statements() now only
// reports and latches - so today nothing else frees a pool and the double free is structurally
// impossible. The lookup below is still the guard, and it is what keeps that true regardless of
// who might free one in future: the JudyL mapping, never the TLS pointer, decides what exists.
void finalize_self_prepared_sql_statements()
{
    // Once a teardown has started we must not finalize: the handles may be closing. Leave the pool
    // registered - the teardown walk runs after the gate, sees it and suppresses the teardown.
    if (!sqlite_lease_acquire_cleanup())
        return;

    // Ask the authority, not our TLS cache. The JudyL mapping is what decides a pool exists;
    // the TLS pointer is only a same-thread cache and may be stale. (It is not that someone else
    // freed the pool - nothing else frees one any more - it is that the cache is not the record.)
    struct stmt_pool_s *pool = NULL;
    spinlock_lock(&JudyL_thread_stmt_lock);
    Word_t thread_id = (Word_t)gettid_cached();
    Pvoid_t *Pvalue = JudyLGet(JudyL_thread_stmt_pool, thread_id, PJE0);
    if (Pvalue && *Pvalue) {
        pool = (struct stmt_pool_s *)*Pvalue;
        (void)JudyLDel(&JudyL_thread_stmt_pool, thread_id, PJE0);
    }
    spinlock_unlock(&JudyL_thread_stmt_lock);
    thread_stmt_pool = NULL;

    // Detached, so only this thread can reach it; the lease keeps the handles open until we are done.
    finalize_and_free_stmt_list(pool);

    sqlite_lease_release();
}

// Report any statement pool whose owner did not clean up, and suppress the teardown if there
// is one.
//
// In a correct shutdown this finds NOTHING: every pool owner either was joined before we got
// here (the ML TRAIN workers, via ml_stop_threads()) or ran
// finalize_self_prepared_sql_statements() from its own cleanup handler (HEALTH, and the main
// thread just above). So the normal path walks an empty array and changes nothing.
//
// If it DOES find a pool, we cannot safely finalize it. We would be finalizing statements
// that a thread may still hold cached, and rule 4 forbids clearing that thread's slot, so
// there would be no way to stop it reusing freed memory. We cannot tell from here whether
// that owner is alive or merely sloppy, so we take the safe reading: warn, latch, and leak.
//
// That check is deliberately structural rather than a list of known owners. The liveness test
// in daemon-shutdown.c knows about HEALTH specifically; this one catches ANY future pool owner
// that is neither joined before teardown nor runs its own cleanup, without needing to be told
// about it.
void finalize_all_prepared_sql_statements()
{
    spinlock_lock(&JudyL_thread_stmt_lock);
    bool first_then_next = true;
    Pvoid_t *Pvalue = NULL;
    Word_t thread_id = 0;
    if (JudyL_thread_stmt_pool) {
        while ((Pvalue = JudyLFirstThenNext(JudyL_thread_stmt_pool, &thread_id, &first_then_next))) {
            struct stmt_pool_s *local_stmt_pool = (struct stmt_pool_s *) *Pvalue;
            if (!local_stmt_pool)
                continue;
            nd_log_daemon(
                NDLP_WARNING,
                "SQL: Pending SQL statements for thread %lu (%s), make sure thread does a proper cleanup",
                thread_id,
                local_stmt_pool->name);

            // Do NOT finalize or free it: see the note above. The pool and its statements are
            // leaked on purpose and the whole teardown is suppressed.
            sqlite_mark_teardown_unsafe(
                "a thread left cached SQL statements registered, so it may still be using them");
        }
    }
    spinlock_unlock(&JudyL_thread_stmt_lock);
}

static void init_thread_stmt_pool(void) {
    thread_stmt_pool = (struct stmt_pool_s *)mallocz(sizeof(struct stmt_pool_s));
    if (!thread_stmt_pool)
        fatal("Failed to allocate memory for statement pool");

    thread_stmt_pool->count = 0;
    thread_stmt_pool->overflow_reported = false;
    thread_stmt_pool->thread_id = gettid_cached();
    thread_stmt_pool->name = strdupz(nd_thread_tag());
    memset(thread_stmt_pool->stmt, 0, sizeof(void *) * MAX_PREPARED_THREAD_STATEMENTS);

    // Add it to the JudyL array
    spinlock_lock(&JudyL_thread_stmt_lock);
    Pvoid_t *Pvalue = JudyLIns(&JudyL_thread_stmt_pool, (Word_t)thread_stmt_pool->thread_id, PJE0);
    if (!Pvalue || Pvalue == PJERR)
        fatal("Failed to allocate memory for JudyL thread statement pool");
    struct stmt_pool_s *old_pool = *Pvalue;
    fatal_assert(old_pool == NULL);
    *Pvalue = thread_stmt_pool;
    spinlock_unlock(&JudyL_thread_stmt_lock);
}

int simple_prepare_statement(sqlite3 *database, const char *query, sqlite3_stmt **statement)
{
    if (!sqlite_lease_acquire_database())
        return SQLITE_MISUSE;

    int rc = sqlite3_prepare_v2(database, query, -1, statement, 0);
    sqlite_lease_release();
    return rc;
}

int prepare_statement(sqlite3 *database, const char *query, sqlite3_stmt **statement)
{
    if (!sqlite_lease_acquire_database())
        return SQLITE_MISUSE;

    int rc = sqlite3_prepare_v2(database, query, -1, statement, 0);
    if (rc == SQLITE_OK) {
        if (!thread_stmt_pool)
            init_thread_stmt_pool();

        // Register the statement, before the lease is released, so shutdown can account for it.
        // count is only touched by this thread, the pool's owner, so no atomic is needed.
        //
        // Do NOT increment past the limit: the old code incremented unconditionally and
        // dropped the statement silently once the array was full, so those statements were
        // never finalized and were exactly what left a zombie connection behind at
        // sqlite3_close_v2(). Refuse loudly instead - it is a real limit being hit, and the
        // caller keeps a usable statement either way.
        if (thread_stmt_pool->count < MAX_PREPARED_THREAD_STATEMENTS)
            thread_stmt_pool->stmt[thread_stmt_pool->count++] = *statement;
        else if (!thread_stmt_pool->overflow_reported) {
            // Report ONCE per thread, not per call: a full pool would otherwise log on every prepare.
            thread_stmt_pool->overflow_reported = true;
            error_report(
                "SQL: thread %s exhausted its %d cached statement slots; further statements on this "
                "thread will not be finalized at shutdown",
                thread_stmt_pool->name,
                MAX_PREPARED_THREAD_STATEMENTS);
        }
    }
    sqlite_lease_release();
    return rc;
}

char *get_database_extented_error(sqlite3 *database, int i, const char *description)
{
    const char *err = sqlite3_errstr(sqlite3_extended_errcode(database));

    if (!err)
        return NULL;

    size_t len = strlen(err)+ strlen(description) + 32;
    char *full_err = mallocz(len);

    snprintfz(full_err, len - 1, "%s: %d: %s", description, i,  err);
    return full_err;
}

int init_database_batch(sqlite3 *database, const char *batch[], const char *description)
{
    int rc;
    char *err_msg = NULL;
    for (int i = 0; batch[i]; i++) {
        rc = sqlite3_exec_monitored(database, batch[i], 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error_report("SQLite error during database initialization, rc = %d (%s)", rc, err_msg);
            error_report("SQLite failed statement %s", batch[i]);
            char *error_str = get_database_extented_error(database, i, description);
            if (error_str)
                analytics_set_data_str(&analytics_data.netdata_fail_reason, error_str);
            sqlite3_free(err_msg);
            freez(error_str);
            if (SQLITE_CORRUPT == rc || SQLITE_NOTADB == rc) {
                if (mark_database_to_recover(NULL, database, rc))
                    error_report("Database is corrupted will attempt to fix");
                return SQLITE_CORRUPT;
            }
            return 1;
        }
    }
    return 0;
}

// Return 0 OK
// Return 1 Failed
// sqlite_rc - if not NULL, it will be set to the return code of the sqlite3_exec_monitored call
int db_execute(sqlite3 *db, const char *cmd, int *sqlite_rc)
{
    int rc;
    int cnt = 0;

    if (unlikely(!db))
        return 1;

    while (cnt < SQL_MAX_RETRY) {
        char *err_msg = NULL;
        rc = sqlite3_exec_monitored(db, cmd, 0, 0, &err_msg);
        if (likely(rc == SQLITE_OK))
            break;

        ++cnt;
        nd_log_daemon(NDLP_WARNING, "Failed to execute '%s', rc = %d (%s) -- attempt %d", cmd, rc, err_msg ? err_msg : "unknown", cnt);
        if (err_msg) {
            sqlite3_free(err_msg);
        }

        if (likely(rc == SQLITE_BUSY || rc == SQLITE_LOCKED)) {
            sleep_usec(SQLITE_INSERT_DELAY * USEC_PER_MS);
            continue;
        }

        if (rc == SQLITE_CORRUPT)
            mark_database_to_recover(NULL, db, rc);
        break;
    }
    if (sqlite_rc)
        *sqlite_rc = rc;

    return (rc != SQLITE_OK);
}

// Utils
int bind_text_null(sqlite3_stmt *res, int position, const char *text, bool can_be_null)
{
    if (likely(text))
        return sqlite3_bind_text(res, position, text, -1, SQLITE_STATIC);
    if (!can_be_null)
        return 1;
    return sqlite3_bind_null(res, position);
}

#define SQL_DROP_TABLE "DROP table %s"

void sql_drop_table(const char *table)
{
    if (!table)
        return;

    char wstr[255];
    snprintfz(wstr, sizeof(wstr) - 1, SQL_DROP_TABLE, table);

    int rc = sqlite3_exec_monitored(db_meta, wstr, 0, 0, NULL);
    if (rc != SQLITE_OK) {
        error_report("DES SQLite error during drop table operation for %s, rc = %d", table, rc);
    }
}

static int get_pragma_value(sqlite3 *database, const char *sql)
{
    sqlite3_stmt *res = NULL;
    int result = -1;
    if (PREPARE_STATEMENT(database, sql, &res)) {
        if (likely(sqlite3_step_monitored(res) == SQLITE_ROW))
            result = sqlite3_column_int(res, 0);
        SQLITE_FINALIZE(res);
    }
    return result;
}

int get_free_page_count(sqlite3 *database)
{
    return get_pragma_value(database, "PRAGMA freelist_count");
}

int get_database_page_count(sqlite3 *database)
{
    return get_pragma_value(database, "PRAGMA page_count");
}

uint64_t sqlite_get_db_space(sqlite3 *db)
{
    if (!db)
        return 0;

    uint64_t page_size = (uint64_t) get_pragma_value(db, "PRAGMA page_size");
    uint64_t page_count = (uint64_t) get_pragma_value(db, "PRAGMA page_count");

    return page_size * page_count;
}

/*
 * Close the sqlite database
 */

// Called immediately before every sqlite3_close_v2() in this file.
//
// sqlite3_close_v2() does not fail when statements are still attached: it marks the
// connection a ZOMBIE and defers the real close until whatever later finalize or close makes
// it non-busy. If sqlite3_shutdown() runs in between it dismantles pcache1 while the zombie
// still references pages, and the deferred close then faults inside pcache1RemoveFromHash -
// after main() has returned, which is why these crashes have no netdata frames.
//
// sqlite3_next_stmt() measures the condition directly rather than inferring it from whether
// some thread failed to clean up: it sees untracked statements (PREPARE_STATEMENT /
// simple_prepare_statement) as well as the pooled ones, and untracked statements are the ones
// most likely to still be attached here.
//
// NOTE, because this reads like a contradiction otherwise: we detect the zombie and then close
// ANYWAY. That is deliberate, and it is safe for reasons that are worth spelling out, because
// the obvious justification is WRONG. It is NOT true that everyone is provably finished by the
// time we get here: the pooled-statement owners are (that is what the liveness latch
// establishes), but UNTRACKED users - the PREPARE_STATEMENT / simple_prepare_statement callers
// on web and API threads - are only bounded by service_wait_exit(~0, 20s), which proceeds
// whether or not they finished, and they set no latch.
//
// Closing over such a user is nevertheless safe, and this is the actual argument:
//  - sqlite3_close_v2() on a BUSY connection frees nothing. It marks the connection a zombie
//    and defers the real close until the last statement is finalized, so a thread mid-step
//    keeps operating on live memory.
//  - The handles reaching this function from sqlite_close_databases() and ml_fini() come from
//    plain sqlite3_open(), and nothing in this tree overrides SQLITE_THREADSAFE or calls
//    sqlite3_config(), so they are fully serialized - the deferred close cannot race a step.
//  - Nobody can START new work afterwards: prepare_statement() and simple_prepare_statement()
//    take a database lease, which is refused once the databases are closed or the teardown gate
//    is set, and return SQLITE_MISUSE. sqlite_close_databases() drains the admitted ones first.
//  - The caller NULLs db_meta / db_context_meta after this returns, so a late user gets
//    SQLITE_MISUSE from a NULL handle rather than a dangling one.
//  - What genuinely is unsafe afterwards is sqlite3_shutdown(), which would dismantle pcache1
//    while that deferred connection still references pages. That single thing is what this flag
//    suppresses.
// Known gap, pre-existing and not introduced here: db_execute() / sqlite3_exec_monitored() do
// not check the closed flag, so they are not covered by the third bullet.
//
// Runs with no lifetime lock held: from sqlite_close_databases() via sql_close_database() after
// the lease drain, from sql_close_thread_db_safe() inside a library lease, and from ml_fini().
//
// Thread-safety of sqlite3_next_stmt() here rests on the same assumption the sqlite3_close_v2()
// one line later already makes: the caller is the sole owner of this handle at this point. That
// matters because the handles passed from sql_close_thread_db_safe() are opened
// SQLITE_OPEN_NOMUTEX, so the call is not internally serialized for them.
static void sqlite_note_zombie_connection(sqlite3 *database, const char *database_name)
{
    if (unlikely(!database))
        return;

    if (sqlite3_next_stmt(database, NULL)) {
        // Only sqlite3_shutdown() is unsafe from here - see sqlite_zombie_connection_created.
        // Do NOT set the liveness latch: this runs at startup too.
        // Record WHICH database and WHEN, because this flag can be set at startup (the
        // context-load path closes thread-local handles) and is then reported hours later at
        // exit. Without provenance the exit message looks like a shutdown problem.
        // Populate BEFORE publishing, and publish with RELEASE - see the note at the
        // declarations. Two threads racing to record the first zombie may overwrite each
        // other's values, which is harmless for a diagnostic; what must not happen is the flag
        // becoming visible ahead of the data.
        if (!__atomic_load_n(&sqlite_zombie_connection_created, __ATOMIC_ACQUIRE)) {
            __atomic_store_n(&sqlite_zombie_noted_db, database_name, __ATOMIC_RELAXED);
            __atomic_store_n(&sqlite_zombie_noted_ut, now_monotonic_usec(), __ATOMIC_RELAXED);
        }
        __atomic_store_n(&sqlite_zombie_connection_created, true, __ATOMIC_RELEASE);
        nd_log_daemon(
            NDLP_WARNING,
            "SQL: the %s database still has prepared statements attached; closing it leaves a zombie "
            "connection, so sqlite3_shutdown() will be skipped at exit (recorded %s)",
            database_name ? database_name : "sqlite",
            __atomic_load_n(&sqlite_databases_closed, __ATOMIC_ACQUIRE) ? "during shutdown" : "before shutdown began");
    }
}

void sql_close_database(sqlite3 *database, const char *database_name)
{
    int rc;
    if (unlikely(!database))
        return;

    (void)db_execute(database, "PRAGMA optimize", NULL);

    netdata_log_info("%s: Closing sqlite database", database_name);

#ifdef NETDATA_DEV_MODE
    int t_count_used = 0, t_count_hit = 0, t_count_miss = 0, t_count_full = 0, dummy;
    (void) sqlite3_db_status(database, SQLITE_DBSTATUS_LOOKASIDE_USED, &dummy, &t_count_used, 0);
    (void) sqlite3_db_status(database, SQLITE_DBSTATUS_LOOKASIDE_HIT, &dummy,&t_count_hit, 0);
    (void) sqlite3_db_status(database, SQLITE_DBSTATUS_LOOKASIDE_MISS_SIZE, &dummy,&t_count_miss, 0);
    (void) sqlite3_db_status(database, SQLITE_DBSTATUS_LOOKASIDE_MISS_FULL, &dummy,&t_count_full, 0);

    netdata_log_info("%s: Database lookaside allocation statistics: Used slots %d, Hit %d, Misses due to small slot size %d, Misses due to slots full %d", database_name,
                     t_count_used,t_count_hit, t_count_miss, t_count_full);

    (void) sqlite3_db_release_memory(database);
#endif

    sqlite_note_zombie_connection(database, database_name);

    rc = sqlite3_close_v2(database);
    if (unlikely(rc != SQLITE_OK))
        error_report("%s: Error while closing the sqlite database: rc %d, error \"%s\"", database_name, rc, sqlite3_errstr(rc));

    // NOTE: the caller's handle is NOT cleared here - this parameter is by value. Callers
    // that keep a global (db_meta, db_context_meta, ml_db) must NULL it themselves, and they
    // do; see sqlite_close_databases() and ml_fini().
}

extern sqlite3 *db_context_meta;

// Close a thread-local sqlite3 handle while serializing against sqlite_library_shutdown().
// If the SQLite library is no longer initialized, the handle is leaked deliberately; the OS
// will reclaim it at process exit, which is strictly safer than crashing inside pcache1.
void sql_close_thread_db_safe(sqlite3 **database)
{
    if (unlikely(!database || !*database))
        return;

    if (sqlite_lease_acquire_library()) {
        sqlite_note_zombie_connection(*database, "thread-local");
        (void) sqlite3_close_v2(*database);
        sqlite_lease_release();
    }

    *database = NULL;
}

void sqlite_close_databases(void)
{
    // In case we have statements in the main thread (we should not).
    // This is the main thread's own pool, so it is safe regardless of the latch below.
    finalize_self_prepared_sql_statements();

    // Refuse new work from this point, whether or not we go on to destroy anything.
    __atomic_store_n(&sqlite_databases_closed, true, __ATOMIC_RELEASE);

    // Stop admitting leases and wait for the admitted ones (prepares, statement cleanups) to leave SQLite.
    // When the teardown is already suppressed nothing below is destroyed, so waiting would only spend the
    // shutdown watchdog's budget.
    netdata_mutex_lock(&sqlite_lifetime_mutex);
    sqlite_teardown_gate = true;
    bool drained = sqlite_teardown_is_unsafe() || sqlite_leases_drain_locked(SQLITE_LEASE_DRAIN_TIMEOUT_UT);
    size_t pending = sqlite_leases;
    netdata_mutex_unlock(&sqlite_lifetime_mutex);

    if (!drained) {
        nd_log_daemon(NDLP_WARNING, "SQL: %zu SQLite operation(s) still running after %llu s", pending,
                      (unsigned long long)(SQLITE_LEASE_DRAIN_TIMEOUT_UT / USEC_PER_SEC));
        sqlite_mark_teardown_unsafe("a SQLite prepare or statement cleanup did not finish");
    }

    // Always run the diagnostic walk, even when teardown is already suppressed. It is the only
    // thing that NAMES the thread that failed to clean up, and it is precisely when the
    // teardown is being suppressed that a triage pass needs that name. It frees nothing, so it
    // is safe to run on any path; it latches if it finds a pool.
    finalize_all_prepared_sql_statements();

    if (sqlite_teardown_is_unsafe()) {
        // Someone may still be inside SQLite with these handles. Finalizing their statements or
        // closing the connections underneath them is a use-after-free; leaking is not.
        //
        // The databases keep an un-checkpointed WAL, which SQLite replays on the next open. The
        // process is exiting, so nothing in it observes the difference. PRAGMA optimize is also
        // skipped, since it lives in sql_close_database().
        nd_log_daemon(
            NDLP_WARNING,
            "SQL: skipping statement finalization and the METADATA/CONTEXT database closes - "
            "SQLite teardown is not safe on this shutdown path");
        return;
    }

    sql_close_database(db_context_meta, "CONTEXT");
    db_context_meta = NULL;
    sql_close_database(db_meta, "METADATA");
    db_meta = NULL;
}

uint64_t get_total_database_space(void)
{
    return 0;

/*
    if (!new_dbengine_defaults)
        return 0;

    uint64_t database_space = sqlite_get_meta_space() + sqlite_get_context_space();
#ifdef ENABLE_ML
    database_space +=  sqlite_get_ml_space();
#endif
    return database_space;
*/
}

#define SQLITE_HEAP_HARD_LIMIT (256 * 1024 * 1024)
#define SQLITE_HEAP_SOFT_LIMIT (32 * 1024 * 1024)

int sqlite_library_init(void)
{
    netdata_mutex_lock(&sqlite_lifetime_mutex);
    int rc = sqlite3_initialize();
    netdata_mutex_unlock(&sqlite_lifetime_mutex);

    // Outside the lifetime mutex: it guards only flags and the lease counter.
    if (rc == SQLITE_OK) {

        (void )sqlite3_hard_heap_limit64(SQLITE_HEAP_HARD_LIMIT);
        int64_t hard_limit_bytes = sqlite3_hard_heap_limit64(-1);

        (void) sqlite3_soft_heap_limit64(SQLITE_HEAP_SOFT_LIMIT);
        int64_t soft_limit_bytes = sqlite3_soft_heap_limit64(-1);

        const char sqlite_hard_limit_mb[32];
        size_snprintf_bytes((char *)sqlite_hard_limit_mb, sizeof(sqlite_hard_limit_mb), hard_limit_bytes);

        const char sqlite_soft_limit_mb[32];
        size_snprintf_bytes((char *)sqlite_soft_limit_mb, sizeof(sqlite_soft_limit_mb), soft_limit_bytes);

        nd_log_daemon(
            NDLP_INFO, "SQLITE: heap memory hard limit %s, soft limit %s", sqlite_hard_limit_mb, sqlite_soft_limit_mb);
    }

    netdata_mutex_lock(&sqlite_lifetime_mutex);
    __atomic_store_n(&sqlite_databases_closed, false, __ATOMIC_RELEASE);
    // Re-arm the LIVENESS latch only. It is about threads that were still running during a
    // previous shutdown, and those are gone by the time we re-initialize, so carrying it forward
    // would silently turn every later sqlite_library_shutdown() into a no-op (only the -W
    // unittest drivers re-initialize the library in one process).
    __atomic_store_n(&sqlite_teardown_unsafe, false, __ATOMIC_RELEASE);

    // The ZOMBIE flag is deliberately NOT cleared. It records that a connection was closed with
    // statements still attached, so its real close is deferred - that is a property of the
    // process, not of a library lifetime. The connection survives re-initialization, and calling
    // sqlite3_shutdown() with it outstanding is exactly the pcache1 teardown crash this flag
    // exists to prevent.
    sqlite_teardown_gate = false;
    sqlite_library_gate = false;
    sqlite_library_initialized = true;
    netdata_mutex_unlock(&sqlite_lifetime_mutex);

    return (SQLITE_OK != rc);
}

int sqlite_release_memory(int bytes)
{
    return sqlite3_release_memory(bytes);
}

void sqlite_library_shutdown(void)
{
    // The check lives here rather than at the call sites so that every caller is covered by
    // construction, and it comes BEFORE the release-memory drain below - that drain mutates
    // the same global allocator state sqlite3_shutdown() tears down.
    //
    // Suppressing the handle close without suppressing this would not fix anything: it would
    // move the fault out of a Vdbe and into pcache1 one line later in the shutdown sequence.

    // Say WHERE the zombie came from. A suppression traced to a close that happened long before
    // shutdown is a different problem from one caused by this teardown, and without this the two
    // are indistinguishable in the log.
    char zombie_note[160];
    if (__atomic_load_n(&sqlite_zombie_connection_created, __ATOMIC_ACQUIRE)) {
        // The ACQUIRE above pairs with the RELEASE publish in sqlite_note_zombie_connection(),
        // so these two are guaranteed written by the time we get here.
        const char *noted_db = __atomic_load_n(&sqlite_zombie_noted_db, __ATOMIC_RELAXED);
        usec_t noted_ut = __atomic_load_n(&sqlite_zombie_noted_ut, __ATOMIC_RELAXED);
        snprintfz(
            zombie_note, sizeof(zombie_note),
            "a zombie %s connection was noted %llu s ago and cannot be proven closed",
            noted_db ? noted_db : "sqlite",
            noted_ut ? (unsigned long long)((now_monotonic_usec() - noted_ut) / USEC_PER_SEC) : 0ULL);
    }
    else
        zombie_note[0] = '\0';

    if (sqlite_teardown_is_unsafe() || __atomic_load_n(&sqlite_zombie_connection_created, __ATOMIC_ACQUIRE)) {
        // Fast path only - the authoritative re-check happens under sqlite_lifetime_mutex below,
        // after the lease drain, because a thread can close a handle (creating a zombie) after this point.
        nd_log_daemon(
            NDLP_WARNING,
            "SQL: skipping sqlite3_shutdown() (%s%s%s). The library stays initialized until the "
            "process exits.",
            sqlite_teardown_is_unsafe() ? "a SQLite user may still be running" : "",
            (sqlite_teardown_is_unsafe() && __atomic_load_n(&sqlite_zombie_connection_created, __ATOMIC_ACQUIRE))
                ? " and " : "",
            __atomic_load_n(&sqlite_zombie_connection_created, __ATOMIC_ACQUIRE) ? zombie_note : "");
        return;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    int bytes;
    do {
        bytes = sqlite_release_memory(1024 * 1024);
        netdata_log_info("SQLITE: Released %d bytes of memory", bytes);
    } while (bytes);
#endif
    netdata_mutex_lock(&sqlite_lifetime_mutex);
    if (!sqlite_library_initialized) {
        netdata_mutex_unlock(&sqlite_lifetime_mutex);
        return;
    }

    // Stop admitting leases of any kind and wait for the admitted ones to leave SQLite. From here no leased
    // prepare, statement cleanup or thread-local close can be inside the library when we shut it down; direct
    // sqlite3_prepare_v2() callers, including health, migrations, and ML model load, take no lease and rely on
    // shutdown ordering, as before.
    sqlite_library_gate = true;
    bool drained = sqlite_leases_drain_locked(SQLITE_LEASE_DRAIN_TIMEOUT_UT);

    // A registered pool belongs to an owner that did not clean up and, with the gate set, never will:
    // its statements are still attached to their connections. (sqlite_close_databases() latches this
    // too, but the -W unittest drivers shut the library down without it.)
    bool pools_registered;
    spinlock_lock(&JudyL_thread_stmt_lock);
    pools_registered = JudyL_thread_stmt_pool != NULL;
    spinlock_unlock(&JudyL_thread_stmt_lock);

    if (!drained || pools_registered)
        __atomic_store_n(&sqlite_teardown_unsafe, true, __ATOMIC_RELEASE);

    // Re-check under the lock, after the drain, and immediately before sqlite3_shutdown(). A thread-local
    // close refused by the gate latches under this same mutex, so it cannot slip past this check.
    //
    // SCOPE, so this is not read as a stronger guarantee than it is: this is authoritative against every
    // lease holder. It is NOT authoritative against ml_fini(), whose close takes no lease (see
    // sqlite_note_zombie_connection). That is sufficient only because ml_fini() and this function are
    // strictly ordered on the one shutdown thread.
    if (sqlite_teardown_is_unsafe() || __atomic_load_n(&sqlite_zombie_connection_created, __ATOMIC_ACQUIRE)) {
        // The library stays initialized, so re-admit thread-local closes instead of leaking every handle.
        sqlite_library_gate = false;
        netdata_mutex_unlock(&sqlite_lifetime_mutex);
        nd_log_daemon(
            NDLP_WARNING,
            "SQL: skipping sqlite3_shutdown() - %s. The library stays initialized until the process exits.",
            !drained ? "a SQLite operation was still running after the drain deadline" :
            pools_registered ? "a thread left cached SQL statements registered" :
                               "a SQLite user or a zombie connection appeared while we were tearing down");
        return;
    }

    sqlite_library_initialized = false;
    (void) sqlite3_shutdown();
    netdata_mutex_unlock(&sqlite_lifetime_mutex);
}

// --------------------------------------------------------------------------------------------------------------------
// Regression test for the global stall: a statement that holds one connection's mutex for a long time must delay
// only that connection's users. Before the lifetime leases, a prepare waiting on that connection held a
// process-wide spinlock, so prepares on every other database and the thread-exit statement cleanup (the ML
// threads' path at shutdown) waited too - until the 3600s deadlock detector, or the shutdown watchdog, fired.

// Only a hang guard for a broken implementation: the pass conditions compare against the holder's recorded
// release, so this just has to outlast the handshake and the measurements on the slowest (sanitizer) runner.
#define SQLITE_LEASE_TEST_MAX_HOLD_UT (60 * USEC_PER_SEC)

struct sqlite_lease_test_op {
    usec_t started_ut;
    usec_t finished_ut;
    int rc;
    bool ok;
};

struct sqlite_lease_test {
    pid_t blocked_tid;                     // the waiting worker, published before it prepares
    bool blocked_leased;                   // set by the lease hook on the waiting worker's own acquisition
    sqlite3 *busy_db;
    sqlite3 *other_db;
    bool holding;
    bool release;                          // set by the test when it is done measuring
    usec_t released_ut;                    // when the holder actually let go of the busy connection
    struct sqlite_lease_test_op blocked;   // the prepare that waits for the busy connection
    struct sqlite_lease_test_op cleanup;   // pooled prepare + thread-exit cleanup on the other database
};

static void sqlite_lease_test_holder(void *arg)
{
    struct sqlite_lease_test *t = arg;
    // Stand-in for a slow statement: SQLite holds this mutex for the whole of a sqlite3_step(). It is held until
    // the test has measured everything, so "finished before the release" is the pass condition, not a time limit.
    // The cap only keeps a broken implementation from hanging the test.
    sqlite3_mutex_enter(sqlite3_db_mutex(t->busy_db));
    __atomic_store_n(&t->holding, true, __ATOMIC_RELEASE);
    usec_t started = now_monotonic_usec();
    while (!__atomic_load_n(&t->release, __ATOMIC_ACQUIRE) &&
           now_monotonic_usec() - started < SQLITE_LEASE_TEST_MAX_HOLD_UT)
        sleep_usec(1 * USEC_PER_MS);
    __atomic_store_n(&t->released_ut, now_monotonic_usec(), __ATOMIC_RELEASE);
    sqlite3_mutex_leave(sqlite3_db_mutex(t->busy_db));
}

static struct sqlite_lease_test *sqlite_lease_test_running = NULL;

// Runs on whichever thread just took a database lease; only the waiting worker's own acquisition counts.
static void sqlite_lease_test_hook(void)
{
    struct sqlite_lease_test *t = __atomic_load_n(&sqlite_lease_test_running, __ATOMIC_ACQUIRE);
    if (t && gettid_cached() == __atomic_load_n(&t->blocked_tid, __ATOMIC_ACQUIRE))
        __atomic_store_n(&t->blocked_leased, true, __ATOMIC_RELEASE);
}

static void sqlite_lease_test_blocked_prepare(void *arg)
{
    struct sqlite_lease_test *t = arg;
    sqlite3_stmt *res = NULL;
    // Waits for the busy connection, as the ACLK worker did behind the slow health query.
    __atomic_store_n(&t->blocked_tid, gettid_cached(), __ATOMIC_RELEASE);
    __atomic_store_n(&t->blocked.started_ut, now_monotonic_usec(), __ATOMIC_RELEASE);
    t->blocked.rc = simple_prepare_statement(t->busy_db, "SELECT 1", &res);
    __atomic_store_n(&t->blocked.finished_ut, now_monotonic_usec(), __ATOMIC_RELEASE);
    t->blocked.ok = t->blocked.rc == SQLITE_OK;
    if (res)
        sqlite3_finalize(res);
}

static void sqlite_lease_test_pooled_cleanup(void *arg)
{
    struct sqlite_lease_test *t = arg;
    sqlite3_stmt *res = NULL;
    // A pooled prepare and the thread-exit cleanup, as an ML thread does when it stops.
    t->cleanup.started_ut = now_monotonic_usec();
    t->cleanup.rc = prepare_statement(t->other_db, "SELECT 2", &res);
    bool registered = t->cleanup.rc == SQLITE_OK && thread_stmt_pool && thread_stmt_pool->count == 1;
    finalize_self_prepared_sql_statements();
    t->cleanup.finished_ut = now_monotonic_usec();

    // The JudyL mapping, not the TLS cache, is the authority on whether the pool still exists.
    spinlock_lock(&JudyL_thread_stmt_lock);
    Pvoid_t *Pvalue = JudyLGet(JudyL_thread_stmt_pool, (Word_t)gettid_cached(), PJE0);
    bool removed = !Pvalue || !*Pvalue;
    spinlock_unlock(&JudyL_thread_stmt_lock);

    t->cleanup.ok = registered && removed;
}

static size_t sqlite_lease_test_leases(void)
{
    netdata_mutex_lock(&sqlite_lifetime_mutex);
    size_t leases = sqlite_leases;
    netdata_mutex_unlock(&sqlite_lifetime_mutex);
    return leases;
}

// Judged after the holder and the blocked worker are joined, so every timestamp is final: the operation must have
// started while the blocked prepare was waiting and finished before the busy connection was released.
static bool sqlite_lease_test_while_busy(struct sqlite_lease_test *t, usec_t started_ut, usec_t finished_ut)
{
    return t->blocked.started_ut && t->blocked.started_ut < started_ut && finished_ut < t->released_ut;
}

int sqlite_lease_unittest(void)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__);

    int errors = 0;
    size_t baseline_leases = sqlite_lease_test_leases();
    struct sqlite_lease_test t = { 0 };
    if (sqlite3_open(":memory:", &t.busy_db) != SQLITE_OK || sqlite3_open(":memory:", &t.other_db) != SQLITE_OK) {
        fprintf(stderr, "SQLITE LEASE TEST: cannot open the test databases\n");
        sqlite3_close_v2(t.busy_db);
        sqlite3_close_v2(t.other_db);
        return 1;
    }

    ND_THREAD *holder = nd_thread_create("SQLTEST_HOLD", NETDATA_THREAD_OPTION_DONT_LOG, sqlite_lease_test_holder, &t);
    if (!holder) {
        fprintf(stderr, "SQLITE LEASE TEST: cannot create the holder thread\n");
        sqlite3_close_v2(t.busy_db);
        sqlite3_close_v2(t.other_db);
        return 1;
    }
    for (int i = 0; i < 5000 && !__atomic_load_n(&t.holding, __ATOMIC_ACQUIRE); i++)
        sleep_usec(1 * USEC_PER_MS);
    if (!__atomic_load_n(&t.holding, __ATOMIC_ACQUIRE)) {
        fprintf(stderr, "SQLITE LEASE TEST: the holder thread did not take the busy connection within 5 s\n");
        __atomic_store_n(&t.release, true, __ATOMIC_RELEASE);
        nd_thread_join(holder);
        sqlite3_close_v2(t.busy_db);
        sqlite3_close_v2(t.other_db);
        return 1;
    }

    __atomic_store_n(&sqlite_lease_test_running, &t, __ATOMIC_RELEASE);
    __atomic_store_n(&sqlite_lease_acquired_test_hook, sqlite_lease_test_hook, __ATOMIC_RELEASE);
    ND_THREAD *blocked =
        nd_thread_create("SQLTEST_WAIT", NETDATA_THREAD_OPTION_DONT_LOG, sqlite_lease_test_blocked_prepare, &t);
    if (!blocked) {
        fprintf(stderr, "SQLITE LEASE TEST: cannot create the waiting thread\n");
        __atomic_store_n(&sqlite_lease_acquired_test_hook, NULL, __ATOMIC_RELEASE);
        __atomic_store_n(&sqlite_lease_test_running, NULL, __ATOMIC_RELEASE);
        __atomic_store_n(&t.release, true, __ATOMIC_RELEASE);
        nd_thread_join(holder);
        sqlite3_close_v2(t.busy_db);
        sqlite3_close_v2(t.other_db);
        return 1;
    }

    // Handshake: wait until the waiting worker itself holds its database lease. The lease is taken immediately
    // before sqlite3_prepare_v2(), so from here it is entering, or already waiting inside, the prepare. The margin
    // makes it all but certain that it is waiting by the time we measure; it is not proof - SQLite exposes no
    // "a thread is waiting on this mutex" signal. Check 3 proves that the prepare did wait for the release.
    for (int i = 0; i < 5000 && !__atomic_load_n(&t.blocked_leased, __ATOMIC_ACQUIRE); i++)
        sleep_usec(1 * USEC_PER_MS);
    bool handshake = __atomic_load_n(&t.blocked_leased, __ATOMIC_ACQUIRE);
    sleep_usec(200 * USEC_PER_MS);

    // 1. A prepare on an unrelated database must not wait for the busy one.
    sqlite3_stmt *res = NULL;
    usec_t started = now_monotonic_usec();
    int rc = simple_prepare_statement(t.other_db, "SELECT 1", &res);
    usec_t finished = now_monotonic_usec();
    if (res)
        sqlite3_finalize(res);

    // 2. A pooled prepare followed by the thread-exit statement cleanup must not wait for it either.
    ND_THREAD *cleanup =
        nd_thread_create("SQLTEST_CLEAN", NETDATA_THREAD_OPTION_DONT_LOG, sqlite_lease_test_pooled_cleanup, &t);
    if (cleanup)
        nd_thread_join(cleanup);

    // Only now let the busy connection go.
    __atomic_store_n(&t.release, true, __ATOMIC_RELEASE);
    nd_thread_join(blocked);
    nd_thread_join(holder);
    __atomic_store_n(&sqlite_lease_acquired_test_hook, NULL, __ATOMIC_RELEASE);
    __atomic_store_n(&sqlite_lease_test_running, NULL, __ATOMIC_RELEASE);

    fprintf(stderr, "SQLITE LEASE TEST: the waiting worker held its database lease before the measurements: %s\n",
            handshake ? "OK" : "FAILED");
    errors += !handshake;

    bool while_busy = sqlite_lease_test_while_busy(&t, started, finished);
    bool ok = rc == SQLITE_OK && while_busy;
    fprintf(stderr, "SQLITE LEASE TEST: prepare on another database while one is busy: rc %d, %llu ms%s: %s\n",
            rc, (unsigned long long)((finished - started) / USEC_PER_MS),
            while_busy ? "" : " (did not complete while the other database was busy)", ok ? "OK" : "FAILED");
    errors += !ok;

    while_busy = cleanup && sqlite_lease_test_while_busy(&t, t.cleanup.started_ut, t.cleanup.finished_ut);
    ok = cleanup && t.cleanup.ok && while_busy;
    fprintf(stderr, "SQLITE LEASE TEST: thread-exit statement cleanup while a database is busy: rc %d, %llu ms%s: %s\n",
            t.cleanup.rc,
            cleanup ? (unsigned long long)((t.cleanup.finished_ut - t.cleanup.started_ut) / USEC_PER_MS) : 0ULL,
            !cleanup ? " (thread not created)" :
            while_busy ? "" : " (did not complete while the other database was busy)",
            ok ? "OK" : "FAILED");
    errors += !ok;

    // 3. The prepare that waited for the busy connection completes once it is released, and it really waited.
    ok = t.blocked.ok && t.blocked.finished_ut >= t.released_ut;
    fprintf(stderr, "SQLITE LEASE TEST: prepare on the busy database returned rc %d %s the release: %s\n",
            t.blocked.rc, t.blocked.finished_ut >= t.released_ut ? "after" : "BEFORE", ok ? "OK" : "FAILED");
    errors += !ok;

    // 4. No lease is left behind.
    size_t leases = sqlite_lease_test_leases();
    ok = leases == baseline_leases;
    fprintf(stderr, "SQLITE LEASE TEST: active leases after the test: %zu (before: %zu): %s\n",
            leases, baseline_leases, ok ? "OK" : "FAILED");
    errors += !ok;

    sqlite3_close_v2(t.busy_db);
    sqlite3_close_v2(t.other_db);
    fprintf(stderr, "SQLITE LEASE TEST: %s\n", errors ? "FAILED" : "OK");
    return errors ? 1 : 0;
}

// --------------------------------------------------------------------------------------------------------------------
// Teardown gates: sqlite_close_databases() and sqlite_library_shutdown() must refuse new leases, wait for the
// admitted ones, and suppress the teardown when a live handle was refused. This runs the real teardown, so it
// MUST be the last SQLite work of its process: the end of -W unittest, or -W sqlite-lease-test.

struct sqlite_teardown_test {
    bool holding;
    bool done;             // set by the test once the teardown call returned: stop waiting for the gate
    bool refused;          // the leases tried once the gate was up were all refused
    usec_t released_ut;    // when the holder let go of its lease
};

static bool sqlite_teardown_test_gate_is_set(bool *gate)
{
    netdata_mutex_lock(&sqlite_lifetime_mutex);
    bool set = *gate;
    netdata_mutex_unlock(&sqlite_lifetime_mutex);
    return set;
}

// Waits until *gate is set by the teardown running on the main thread. Returns false when the teardown returned
// without setting it, or on the hang guard.
static bool sqlite_teardown_test_wait_gate(struct sqlite_teardown_test *t, bool *gate)
{
    usec_t started = now_monotonic_usec();
    while (!sqlite_teardown_test_gate_is_set(gate)) {
        if (__atomic_load_n(&t->done, __ATOMIC_ACQUIRE) ||
            now_monotonic_usec() - started >= SQLITE_LEASE_TEST_MAX_HOLD_UT)
            return false;
        sleep_usec(1 * USEC_PER_MS);
    }
    return true;
}

// Lets go of the held lease a little after the gate went up, so the drain demonstrably waited for it.
static void sqlite_teardown_test_release(struct sqlite_teardown_test *t)
{
    sleep_usec(100 * USEC_PER_MS);
    __atomic_store_n(&t->released_ut, now_monotonic_usec(), __ATOMIC_RELEASE);
    sqlite_lease_release();
}

// Holds a database lease across the start of sqlite_close_databases().
static void sqlite_teardown_test_database_holder(void *arg)
{
    struct sqlite_teardown_test *t = arg;
    if (!sqlite_lease_acquire_database())
        return;
    __atomic_store_n(&t->holding, true, __ATOMIC_RELEASE);

    bool gate = sqlite_teardown_test_wait_gate(t, &sqlite_teardown_gate);
    bool database = sqlite_lease_acquire_database();
    if (database)
        sqlite_lease_release();
    bool cleanup = sqlite_lease_acquire_cleanup();
    if (cleanup)
        sqlite_lease_release();
    __atomic_store_n(&t->refused, gate && !database && !cleanup, __ATOMIC_RELEASE);

    sqlite_teardown_test_release(t);
}

// Holds a library lease across the start of sqlite_library_shutdown(), then tries to close a thread-local
// handle while it drains: that close is refused and must suppress the shutdown.
static void sqlite_teardown_test_library_holder(void *arg)
{
    struct sqlite_teardown_test *t = arg;
    if (!sqlite_lease_acquire_library())
        return;
    __atomic_store_n(&t->holding, true, __ATOMIC_RELEASE);

    bool gate = sqlite_teardown_test_wait_gate(t, &sqlite_library_gate);
    bool library = sqlite_lease_acquire_library();
    if (library)
        sqlite_lease_release();
    __atomic_store_n(&t->refused, gate && !library, __ATOMIC_RELEASE);

    sqlite_teardown_test_release(t);
}

// Starts a holder thread and waits until it holds its lease. Returns NULL when it could not take one.
static ND_THREAD *sqlite_teardown_test_start(const char *tag, void (*holder)(void *), struct sqlite_teardown_test *t)
{
    ND_THREAD *thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DONT_LOG, holder, t);
    if (!thread)
        return NULL;

    usec_t started = now_monotonic_usec();
    while (!__atomic_load_n(&t->holding, __ATOMIC_ACQUIRE) &&
           now_monotonic_usec() - started < 5 * USEC_PER_SEC)
        sleep_usec(1 * USEC_PER_MS);

    if (!__atomic_load_n(&t->holding, __ATOMIC_ACQUIRE)) {
        nd_thread_join(thread);
        return NULL;
    }
    return thread;
}

int sqlite_lease_teardown_unittest(void)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__);
    int errors = 0;

    // 1. sqlite_close_databases() refuses new leases and waits for the admitted one, then tears down.
    {
        struct sqlite_teardown_test t = { 0 };
        ND_THREAD *holder = sqlite_teardown_test_start("SQLTEST_GATE", sqlite_teardown_test_database_holder, &t);
        sqlite_close_databases();
        usec_t returned_ut = now_monotonic_usec();
        __atomic_store_n(&t.done, true, __ATOMIC_RELEASE);
        if (holder)
            nd_thread_join(holder);

        bool waited = holder && returned_ut >= __atomic_load_n(&t.released_ut, __ATOMIC_ACQUIRE);
        bool ok = holder && t.refused && waited && !sqlite_teardown_is_unsafe() && sqlite_lease_test_leases() == 0;
        fprintf(stderr, "SQLITE TEARDOWN TEST: closing the databases while a lease is held: %s%s%s%s%s\n",
                ok ? "OK" : "FAILED",
                holder ? "" : " - no lease taken",
                holder && !t.refused ? " - new leases admitted after the gate" : "",
                holder && !waited ? " - did not wait for the lease" : "",
                sqlite_teardown_is_unsafe() ? " - teardown suppressed" : "");
        errors += !ok;
    }

    // 2. The drain gives up at its deadline while a lease is still held.
    {
        bool leased = sqlite_lease_acquire_library();
        usec_t started = now_monotonic_usec();
        netdata_mutex_lock(&sqlite_lifetime_mutex);
        bool drained_held = sqlite_leases_drain_locked(50 * USEC_PER_MS);
        netdata_mutex_unlock(&sqlite_lifetime_mutex);
        usec_t waited_ut = now_monotonic_usec() - started;
        if (leased)
            sqlite_lease_release();

        netdata_mutex_lock(&sqlite_lifetime_mutex);
        bool drained_free = sqlite_leases_drain_locked(50 * USEC_PER_MS);
        netdata_mutex_unlock(&sqlite_lifetime_mutex);

        bool ok = leased && !drained_held && waited_ut >= 50 * USEC_PER_MS && drained_free;
        fprintf(stderr, "SQLITE TEARDOWN TEST: drain deadline with a lease held: %s after %llu ms, "
                        "then %s with none: %s\n",
                drained_held ? "drained" : "timed out", (unsigned long long)(waited_ut / USEC_PER_MS),
                drained_free ? "drained" : "timed out", ok ? "OK" : "FAILED");
        errors += !ok;
    }

    // 3. sqlite_library_shutdown() waits for the admitted lease; a close refused by its gate suppresses it.
    // When the teardown is already suppressed (-W unittest closes METADATA with statements still attached, which
    // leaves a zombie connection), the shutdown returns before its gate: then only check that it stayed up.
    if (sqlite_teardown_is_unsafe() || __atomic_load_n(&sqlite_zombie_connection_created, __ATOMIC_ACQUIRE)) {
        sqlite_library_shutdown();

        netdata_mutex_lock(&sqlite_lifetime_mutex);
        bool still_initialized = sqlite_library_initialized;
        netdata_mutex_unlock(&sqlite_lifetime_mutex);

        fprintf(stderr, "SQLITE TEARDOWN TEST: library shutdown with the teardown already suppressed: %s%s\n",
                still_initialized ? "OK" : "FAILED", still_initialized ? "" : " - library shut down anyway");
        errors += !still_initialized;
    }
    else {
        struct sqlite_teardown_test t = { 0 };
        ND_THREAD *holder = sqlite_teardown_test_start("SQLTEST_LIB", sqlite_teardown_test_library_holder, &t);
        sqlite_library_shutdown();
        usec_t returned_ut = now_monotonic_usec();
        __atomic_store_n(&t.done, true, __ATOMIC_RELEASE);
        if (holder)
            nd_thread_join(holder);

        netdata_mutex_lock(&sqlite_lifetime_mutex);
        bool still_initialized = sqlite_library_initialized;
        bool gate_reopened = !sqlite_library_gate;
        netdata_mutex_unlock(&sqlite_lifetime_mutex);

        bool waited = holder && returned_ut >= __atomic_load_n(&t.released_ut, __ATOMIC_ACQUIRE);
        bool ok = holder && t.refused && waited && sqlite_teardown_is_unsafe() && still_initialized &&
                  gate_reopened && sqlite_lease_test_leases() == 0;
        fprintf(stderr, "SQLITE TEARDOWN TEST: library shutdown with a refused thread-local close: %s%s%s%s%s%s\n",
                ok ? "OK" : "FAILED",
                holder ? "" : " - no lease taken",
                holder && !t.refused ? " - a lease was admitted after the gate" : "",
                holder && !waited ? " - did not wait for the lease" : "",
                !sqlite_teardown_is_unsafe() ? " - refusal did not latch" : "",
                !still_initialized ? " - library shut down anyway" : "");
        errors += !ok;
    }

    fprintf(stderr, "SQLITE TEARDOWN TEST: %s\n", errors ? "FAILED" : "OK");
    return errors ? 1 : 0;
}
