// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_LOG_H
#define NETDATA_DBENGINE_LOG_H

#include "database/storage-engines/dbengine/include/dbengine/dbengine-config.h"

// The engine's diagnostics, and the one place they are decided.
//
// Every line the engine emits goes through a macro here, and every macro takes the engine that is emitting it as
// its first argument. Where the line then goes is the engine's configuration's business (log_sink,
// dbengine-config.h): with no sink - the default, and what the engine has always done - it goes to netdata's
// logger; with a sink it goes there instead, and not to the logger.
//
// The branch is at the call site, deliberately. Each macro expands to the sink test, the sink call, and *the exact
// call that stood at that site before this layer existed*, unchanged, as the other arm. So "with no sink the engine
// logs as it always did" is not an argument about two code paths being equivalent - it is the same code, reached by
// an if. The families below map one-to-one onto libnetdata's, including the ones whose behaviour is not just a
// priority: netdata_log_debug()'s own debug_flags gate and its separate NDLS_DEBUG source, error_report()'s
// errno_clear(), internal_error()'s compile-out, and the rate limiter.
//
// Two things a reader should know before converting a site:
//
// - The sink has no log source. It is the engine's own stream, and what the engine emits is all of it; the
//   NDLS_DAEMON / NDLS_DEBUG distinction survives only on the no-sink arm, where it is the original call making it.
// - The sink has no priority filter either. netdata's logger drops a line below the source's configured minimum
//   before anything else happens; the sink is handed every line and decides for itself. An embedder that wants the
//   logger's thresholds implements them in the sink.
//
// What stays outside this layer, and why: the process-wide page-data allocators (page.c) have no engine to reach
// at the sites that report a bad page - pgd_init_arals() is the exception and takes one -
// fatal() and internal_fatal() end the process rather than report to anyone, and the protected-read recovery in
// libnetdata logs on the engine's behalf from its own code. dbengine-config.h's contract tells an embedder the
// same thing.

struct dbengine_engine;

// whether this engine's lines go to a sink; false for no engine and for an engine with none. Not inline on
// purpose: it would need the engine's layout here, and this header is included by rrdengine.h, which defines it.
//
// The cost, stated honestly because the next reader will want it: one cross-TU call per emission *attempt*, not
// per line emitted. Only dbengine_log_debug and dbengine_internal_error gate before it (on the debug flag and on
// their condition); the other five reach it every time, including on an attempt a rate limiter is about to drop
// and on one the logger's priority threshold would have dropped. No site in the engine is hot enough for that to
// matter today - the ones that looked like candidates all sit behind an unlikely() failure test - but a site that
// logged on a hot path would pay it.
bool dbengine_log_has_sink(struct dbengine_engine *engine);

// hand one line to the engine's sink. Only ever called when dbengine_log_has_sink() just said yes, so the sink is
// known to be there. errno is saved and restored around the sink, as the contract promises
void dbengine_log_emit(struct dbengine_engine *engine, ND_LOG_FIELD_PRIORITY priority,
                       const char *file, const char *function, unsigned long line,
                       const char *fmt, ...) PRINTFLIKE(6, 7);

// the same, rate limited per call site, for the sink arm of dbengine_log_limit(). It carries dbengine's own copy of
// libnetdata's gate (nd_log.c, netdata_logger_with_limit) because the engine may not edit libnetdata in this
// change; it operates on the very same public ERROR_LIMIT objects the call sites already declare, so a site's
// throttling state is one object whichever arm runs. Three deliberate differences from the original, two of them
// forced: it cannot see nd_log's single-threaded-child elision, so it always takes the spinlock, and it applies no
// priority pre-filter, because filtering is the sink's job. The third is a choice - it restores errno on every
// path out, which libnetdata's does not, because the sink contract promises it
void dbengine_log_emit_limit(struct dbengine_engine *engine, ERROR_LIMIT *erl, ND_LOG_FIELD_PRIORITY priority,
                             const char *file, const char *function, unsigned long line,
                             const char *fmt, ...) PRINTFLIKE(7, 8);

// The families. The engine expression is evaluated once, and only when the family's own gate has passed.

#define dbengine_log_error(engine, args...) do {                                                    \
        struct dbengine_engine *_dbengine_log_e = (engine);                                         \
        if(unlikely(dbengine_log_has_sink(_dbengine_log_e)))                                        \
            dbengine_log_emit(_dbengine_log_e, NDLP_ERR, __FILE__, __FUNCTION__, __LINE__, ##args); \
        else                                                                                        \
            netdata_log_error(args);                                                                \
    } while(0)

#define dbengine_log_info(engine, args...) do {                                                      \
        struct dbengine_engine *_dbengine_log_e = (engine);                                          \
        if(unlikely(dbengine_log_has_sink(_dbengine_log_e)))                                          \
            dbengine_log_emit(_dbengine_log_e, NDLP_INFO, __FILE__, __FUNCTION__, __LINE__, ##args);  \
        else                                                                                         \
            netdata_log_info(args);                                                                  \
    } while(0)

// the nd_log(NDLS_DAEMON, ...) and nd_log_daemon(...) sites: an explicit priority, the daemon source
#define dbengine_log(engine, priority, args...) do {                                                \
        struct dbengine_engine *_dbengine_log_e = (engine);                                         \
        if(unlikely(dbengine_log_has_sink(_dbengine_log_e)))                                        \
            dbengine_log_emit(_dbengine_log_e, priority, __FILE__, __FUNCTION__, __LINE__, ##args); \
        else                                                                                        \
            nd_log_daemon(priority, args);                                                          \
    } while(0)

// rate limited per call site. The gate is the sink's own on the sink arm; on the no-sink arm libnetdata's
// netdata_logger_with_limit() runs whole, gate included, exactly as it does today
#define dbengine_log_limit(engine, erl, priority, args...) do {                                            \
        struct dbengine_engine *_dbengine_log_e = (engine);                                                \
        if(unlikely(dbengine_log_has_sink(_dbengine_log_e)))                                               \
            dbengine_log_emit_limit(_dbengine_log_e, erl, priority, __FILE__, __FUNCTION__, __LINE__,      \
                                    ##args);                                                               \
        else                                                                                               \
            nd_log_limit(erl, NDLS_DAEMON, priority, args);                                                \
    } while(0)

// the teardown narration dbengine_destroy() writes while the caches go down. Its no-sink arm is the fprintf(stderr)
// it has always been - not the logger - so the format keeps the trailing newline that fprintf needs, and a sink
// receives it with that newline in the format
#define dbengine_progress(engine, args...) do {                                                      \
        struct dbengine_engine *_dbengine_log_e = (engine);                                          \
        if(unlikely(dbengine_log_has_sink(_dbengine_log_e)))                                          \
            dbengine_log_emit(_dbengine_log_e, NDLP_INFO, __FILE__, __FUNCTION__, __LINE__, ##args);  \
        else                                                                                          \
            fprintf(stderr, args);                                                                    \
    } while(0)

// error_report()'s errno_clear() side effect comes first, as it does in libnetdata. Written out rather than
// wrapped around dbengine_log_error() so the two do not declare the same local in nested scopes
#define dbengine_error_report(engine, args...) do {                                                 \
        errno_clear();                                                                              \
        struct dbengine_engine *_dbengine_log_e = (engine);                                         \
        if(unlikely(dbengine_log_has_sink(_dbengine_log_e)))                                        \
            dbengine_log_emit(_dbengine_log_e, NDLP_ERR, __FILE__, __FUNCTION__, __LINE__, ##args); \
        else                                                                                        \
            netdata_log_error(args);                                                                \
    } while(0)

#ifdef NETDATA_INTERNAL_CHECKS

// netdata_log_debug()'s own gate and its separate source. The gate is first: a line the debug flags do not ask for
// is not emitted, with a sink or without one, exactly as today
#define dbengine_log_debug(engine, type, args...) do {                                                \
        if(unlikely(debug_flags & (type))) {                                                          \
            struct dbengine_engine *_dbengine_log_e = (engine);                                       \
            if(unlikely(dbengine_log_has_sink(_dbengine_log_e)))                                      \
                dbengine_log_emit(_dbengine_log_e, NDLP_DEBUG, __FILE__, __FUNCTION__, __LINE__,      \
                                  ##args);                                                            \
            else                                                                                      \
                netdata_logger(NDLS_DEBUG, NDLP_DEBUG, __FILE__, __FUNCTION__, __LINE__, ##args);     \
        }                                                                                             \
    } while(0)

#define dbengine_internal_error(engine, condition, args...) do {                                      \
        if(unlikely(condition)) {                                                                     \
            struct dbengine_engine *_dbengine_log_e = (engine);                                       \
            if(unlikely(dbengine_log_has_sink(_dbengine_log_e)))                                      \
                dbengine_log_emit(_dbengine_log_e, NDLP_DEBUG, __FILE__, __FUNCTION__, __LINE__,      \
                                  ##args);                                                            \
            else                                                                                      \
                netdata_logger(NDLS_DAEMON, NDLP_DEBUG, __FILE__, __FUNCTION__, __LINE__, ##args);    \
        }                                                                                             \
    } while(0)

#else

// as libnetdata does when the checks are off: every argument is discarded, the engine expression included, so a
// release build gains no evaluation a debug build does not have
#define dbengine_log_debug(engine, type, args...) debug_dummy()
#define dbengine_internal_error(args...) debug_dummy()

#endif

#endif // NETDATA_DBENGINE_LOG_H
