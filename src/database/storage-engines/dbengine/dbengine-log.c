// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdengine.h"

// The sink arm of the macros in dbengine-log.h. Everything here runs only for an engine that has a sink: the
// no-sink arm never reaches this file, it is libnetdata's own call, made at the site.

bool dbengine_log_has_sink(struct dbengine_engine *engine) {
    return engine && engine->cfg.log_sink;
}

// errno is set to the site's value before the sink and restored after it: the sink may read it (the limiter's sleep
// and a contended spinlock both clear it on the way here), and a caller reading errno after an engine verb is
// unaffected by whatever the sink does
static void dbengine_log_to_sink(struct dbengine_engine *engine, ND_LOG_FIELD_PRIORITY priority,
                                 const char *file, const char *function, unsigned long line,
                                 const char *fmt, va_list ap, int saved_errno) {
    errno = saved_errno;
    engine->cfg.log_sink(engine->cfg.log_sink_data, priority, file, function, line, fmt, ap);
    errno = saved_errno;
}

void dbengine_log_emit(struct dbengine_engine *engine, ND_LOG_FIELD_PRIORITY priority,
                       const char *file, const char *function, unsigned long line,
                       const char *fmt, ...) {
    int saved_errno = errno;

    va_list ap;
    va_start(ap, fmt);
    dbengine_log_to_sink(engine, priority, file, function, line, fmt, ap, saved_errno);
    va_end(ap);
}

// The rate limiter is libnetdata's own gate (nd_log_limit_admit() / nd_log_limit_emitted(), which
// netdata_logger_with_limit() runs too), over the site's own ERROR_LIMIT - the object the no-sink arm hands to
// libnetdata - so a site's window is one object whichever arm runs. Unlike the logger it applies no priority filter
// first, because filtering is the sink's job, and it restores errno on every path out.
void dbengine_log_emit_limit(struct dbengine_engine *engine, ERROR_LIMIT *erl, ND_LOG_FIELD_PRIORITY priority,
                             const char *file, const char *function, unsigned long line,
                             const char *fmt, ...) {
    // captured before the gate, as netdata_logger_with_limit() captures it: its sleep and its lock can both clear it
    int saved_errno = errno;

    time_t now;
    if(!nd_log_limit_admit(erl, &now)) {
        errno = saved_errno;
        return;
    }

    va_list ap;
    va_start(ap, fmt);
    dbengine_log_to_sink(engine, priority, file, function, line, fmt, ap, saved_errno);
    va_end(ap);

    nd_log_limit_emitted(erl, now);
}
