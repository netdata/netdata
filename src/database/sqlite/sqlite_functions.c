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
//  2. Every find, mutate and free of a JudyL_thread_stmt_pool entry happens under
//     JudyL_thread_stmt_lock.
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

SPINLOCK sqlite_spinlock = SPINLOCK_INITIALIZER;

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
// still closes those, which is safe because they are private to a single thread.)
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
// There is deliberately NO sqlite_databases_closed check here, neither before the lock nor under
// it - see rule 1 at the top of this file. The unlocked one used to be at the head of this
// function and was the double free: this thread read "not closed", waited for the lock, and then
// freed a pool that the teardown thread had already freed and removed.
//
// That teardown-side free no longer exists - finalize_all_prepared_sql_statements() now only
// reports and latches - so today nothing else frees a pool and the double free is structurally
// impossible. The lookup below is still the guard, and it is what keeps that true regardless of
// who might free one in future: the JudyL mapping, never the TLS pointer, decides what exists.
void finalize_self_prepared_sql_statements()
{
    spinlock_lock(&sqlite_spinlock);
    spinlock_lock(&JudyL_thread_stmt_lock);

    // Ask the authority, not our TLS cache: our cached pointer may name a pool that
    // finalize_all_prepared_sql_statements() has already freed.
    Word_t thread_id = (Word_t)gettid_cached();
    Pvoid_t *Pvalue = JudyLGet(JudyL_thread_stmt_pool, thread_id, PJE0);
    if (Pvalue && *Pvalue) {
        finalize_and_free_stmt_list((struct stmt_pool_s *)*Pvalue);
        (void)JudyLDel(&JudyL_thread_stmt_pool, thread_id, PJE0);
    }
    thread_stmt_pool = NULL;

    spinlock_unlock(&JudyL_thread_stmt_lock);
    spinlock_unlock(&sqlite_spinlock);
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
    spinlock_lock(&sqlite_spinlock);
    if (__atomic_load_n(&sqlite_databases_closed, __ATOMIC_ACQUIRE)) {
        spinlock_unlock(&sqlite_spinlock);
        return SQLITE_MISUSE;
    }

    int rc = sqlite3_prepare_v2(database, query, -1, statement, 0);
    spinlock_unlock(&sqlite_spinlock);
    return rc;
}

int prepare_statement(sqlite3 *database, const char *query, sqlite3_stmt **statement)
{
    spinlock_lock(&sqlite_spinlock);
    if (__atomic_load_n(&sqlite_databases_closed, __ATOMIC_ACQUIRE)) {
        spinlock_unlock(&sqlite_spinlock);
        return SQLITE_MISUSE;
    }

    int rc = sqlite3_prepare_v2(database, query, -1, statement, 0);
    if (rc == SQLITE_OK) {
        if (!thread_stmt_pool)
            init_thread_stmt_pool();

        // Register the statement so shutdown can account for it. count is only ever
        // touched under sqlite_spinlock, so no atomic is needed.
        //
        // Do NOT increment past the limit: the old code incremented unconditionally and
        // dropped the statement silently once the array was full, so those statements were
        // never finalized and were exactly what left a zombie connection behind at
        // sqlite3_close_v2(). Refuse loudly instead - it is a real limit being hit, and the
        // caller keeps a usable statement either way.
        if (thread_stmt_pool->count < MAX_PREPARED_THREAD_STATEMENTS)
            thread_stmt_pool->stmt[thread_stmt_pool->count++] = *statement;
        else if (!thread_stmt_pool->overflow_reported) {
            // Report ONCE per thread, not per call: this runs under sqlite_spinlock, which also
            // serializes simple_prepare_statement() for every web, API and metadata thread, so an
            // unconditional log here would turn a full pool into a global throughput problem.
            thread_stmt_pool->overflow_reported = true;
            error_report(
                "SQL: thread %s exhausted its %d cached statement slots; further statements on this "
                "thread will not be finalized at shutdown",
                thread_stmt_pool->name,
                MAX_PREPARED_THREAD_STATEMENTS);
        }
    }
    spinlock_unlock(&sqlite_spinlock);
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
//    both re-check sqlite_databases_closed under sqlite_spinlock and return SQLITE_MISUSE.
//  - The caller NULLs db_meta / db_context_meta after this returns, so a late user gets
//    SQLITE_MISUSE from a NULL handle rather than a dangling one.
//  - What genuinely is unsafe afterwards is sqlite3_shutdown(), which would dismantle pcache1
//    while that deferred connection still references pages. That single thing is what this flag
//    suppresses.
// Known gap, pre-existing and not introduced here: db_execute() / sqlite3_exec_monitored() do
// not check the closed flag, so they are not covered by the third bullet.
//
// MUST NOT take sqlite_spinlock: it is non-recursive and this runs both WITH the lock held
// (from sqlite_close_databases() via sql_close_database(), and from sql_close_thread_db_safe(),
// which takes it itself) and WITHOUT it (from ml_fini(), which holds nothing).
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
    int t_count_used,t_count_hit,t_count_miss,t_count_full, dummy;
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

    spinlock_lock(&sqlite_spinlock);
    if (sqlite_library_initialized) {
        sqlite_note_zombie_connection(*database, "thread-local");
        (void) sqlite3_close_v2(*database);
    }
    spinlock_unlock(&sqlite_spinlock);

    *database = NULL;
}

void sqlite_close_databases(void)
{
    // In case we have statements in the main thread (we should not).
    // This is the main thread's own pool, so it is safe regardless of the latch below.
    finalize_self_prepared_sql_statements();

    // Refuse new work from this point, whether or not we go on to destroy anything.
    __atomic_store_n(&sqlite_databases_closed, true, __ATOMIC_RELEASE);

    spinlock_lock(&sqlite_spinlock);

    // Always run the diagnostic walk, even when teardown is already suppressed. It is the only
    // thing that NAMES the thread that failed to clean up, and it is precisely when the
    // teardown is being suppressed that a triage pass needs that name. It frees nothing, so it
    // is safe to run on any path; it latches if it finds a pool.
    finalize_all_prepared_sql_statements();

    if (sqlite_teardown_is_unsafe()) {
        spinlock_unlock(&sqlite_spinlock);

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
    spinlock_unlock(&sqlite_spinlock);
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
    spinlock_lock(&sqlite_spinlock);

    int rc = sqlite3_initialize();
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
    sqlite_library_initialized = true;
    spinlock_unlock(&sqlite_spinlock);

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
    if (sqlite_teardown_is_unsafe() || __atomic_load_n(&sqlite_zombie_connection_created, __ATOMIC_ACQUIRE)) {
        // Fast path only - the authoritative re-check happens under sqlite_spinlock below,
        // because a thread can close a handle (creating a zombie) after this point.
        nd_log_daemon(
            NDLP_WARNING,
            "SQL: skipping sqlite3_shutdown() (%s%s%s). The library stays initialized until the "
            "process exits.",
            sqlite_teardown_is_unsafe() ? "a SQLite user may still be running" : "",
            (sqlite_teardown_is_unsafe() && __atomic_load_n(&sqlite_zombie_connection_created, __ATOMIC_ACQUIRE))
                ? " and " : "",
            __atomic_load_n(&sqlite_zombie_connection_created, __ATOMIC_ACQUIRE) ? "a zombie connection exists" : "");
        return;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    int bytes;
    do {
        bytes = sqlite_release_memory(1024 * 1024);
        netdata_log_info("SQLITE: Released %d bytes of memory", bytes);
    } while (bytes);
#endif
    spinlock_lock(&sqlite_spinlock);
    if (!sqlite_library_initialized) {
        spinlock_unlock(&sqlite_spinlock);
        return;
    }

    // Re-check under the lock, and immediately before sqlite3_shutdown(). The check above is
    // unlocked, so between it and here another thread could have closed a handle with statements
    // attached, and tearing the library down over a fresh zombie is the crash we are avoiding.
    //
    // SCOPE, so this is not read as a stronger guarantee than it is: holding the lock makes this
    // authoritative against sql_close_thread_db_safe(), which takes this same lock around its
    // note-and-close. It is NOT authoritative against ml_fini(), whose close runs with no lock
    // held at all (see sqlite_note_zombie_connection). That is sufficient only because ml_fini()
    // and this function are strictly ordered on the one shutdown thread - not because the lock
    // excludes it.
    if (sqlite_teardown_is_unsafe() || __atomic_load_n(&sqlite_zombie_connection_created, __ATOMIC_ACQUIRE)) {
        spinlock_unlock(&sqlite_spinlock);
        nd_log_daemon(
            NDLP_WARNING,
            "SQL: skipping sqlite3_shutdown() - a SQLite user or a zombie connection appeared while "
            "we were tearing down. The library stays initialized until the process exits.");
        return;
    }

    sqlite_library_initialized = false;
    (void) sqlite3_shutdown();
    spinlock_unlock(&sqlite_spinlock);
}
