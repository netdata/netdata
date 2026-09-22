// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdengine.h"

// The sink arm of the macros in dbengine-log.h. Everything here runs only for an engine that has a sink: the
// no-sink arm never reaches this file, it is libnetdata's own call, made at the site.

bool dbengine_log_has_sink(struct dbengine_engine *engine) {
    return engine && engine->cfg.log_sink;
}

// errno is saved and restored around the sink: dbengine-config.h promises an embedder that a caller reading errno
// after an engine verb is unaffected by whatever its sink does
static void dbengine_log_to_sink(struct dbengine_engine *engine, ND_LOG_FIELD_PRIORITY priority,
                                 const char *file, const char *function, unsigned long line,
                                 const char *fmt, va_list ap) {
    int saved_errno = errno;
    engine->cfg.log_sink(engine->cfg.log_sink_data, priority, file, function, line, fmt, ap);
    errno = saved_errno;
}

void dbengine_log_emit(struct dbengine_engine *engine, ND_LOG_FIELD_PRIORITY priority,
                       const char *file, const char *function, unsigned long line,
                       const char *fmt, ...) {
    va_list ap;
    va_start(ap, fmt);
    dbengine_log_to_sink(engine, priority, file, function, line, fmt, ap);
    va_end(ap);
}

// The rate limiter, step for step as libnetdata's netdata_logger_with_limit() runs it (nd_log.c): sleep if the
// site asks for one, count the attempt under the site's spinlock, drop it when the site logged too recently, and
// only on a line that is emitted move the window and reset the count. The ERROR_LIMIT is the site's own object -
// the same one the no-sink arm hands to libnetdata - so a site that switches arms carries its state across.
//
// It differs from the original in the two ways it must (dbengine-log.h says why): no single-threaded-child lock
// elision, and no priority pre-filter. The sleep is kept although every dbengine call site declares sleep_ut 0,
// so a site that ever sets one behaves as it would have on the old path.
void dbengine_log_emit_limit(struct dbengine_engine *engine, ERROR_LIMIT *erl, ND_LOG_FIELD_PRIORITY priority,
                             const char *file, const char *function, unsigned long line,
                             const char *fmt, ...) {
    if(erl->sleep_ut)
        sleep_usec(erl->sleep_ut);

    spinlock_lock(&erl->spinlock);

    erl->count++;
    time_t now = now_boottime_sec();
    if(now - erl->last_logged < erl->log_every) {
        spinlock_unlock(&erl->spinlock);
        return;
    }

    spinlock_unlock(&erl->spinlock);

    va_list ap;
    va_start(ap, fmt);
    dbengine_log_to_sink(engine, priority, file, function, line, fmt, ap);
    va_end(ap);

    erl->last_logged = now;
    erl->count = 0;
}
