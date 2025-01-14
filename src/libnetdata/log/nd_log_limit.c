// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log_limit.h"

void nd_log_limits_reset(void) {
    usec_t now_ut = now_monotonic_usec();

    spinlock_lock(&nd_log.std_output.spinlock);
    spinlock_lock(&nd_log.std_error.spinlock);

    for(size_t i = 0; i < _NDLS_MAX ;i++) {
        spinlock_lock(&nd_log.sources[i].spinlock);
        nd_log.sources[i].limits.prevented = 0;
        nd_log.sources[i].limits.counter = 0;
        nd_log.sources[i].limits.started_monotonic_ut = now_ut;
        nd_log.sources[i].limits.logs_per_period = nd_log.sources[i].limits.logs_per_period_backup;
        spinlock_unlock(&nd_log.sources[i].spinlock);
    }

    spinlock_unlock(&nd_log.std_output.spinlock);
    spinlock_unlock(&nd_log.std_error.spinlock);
}

void nd_log_limits_unlimited(void) {
    nd_log_limits_reset();
    for(size_t i = 0; i < _NDLS_MAX ;i++) {
        nd_log.sources[i].limits.logs_per_period = 0;
    }
}

bool nd_log_limit_reached(struct nd_log_source *source) {
    if(source->limits.throttle_period == 0 || source->limits.logs_per_period == 0)
        return false;

    spinlock_lock(&source->limits.spinlock);

    usec_t now_ut = now_monotonic_usec();
    if(!source->limits.started_monotonic_ut)
        source->limits.started_monotonic_ut = now_ut;

    source->limits.counter++;

    // Check if we need to reset the period
    if(now_ut - source->limits.started_monotonic_ut > (usec_t)source->limits.throttle_period * USEC_PER_SEC) {
        if(source->limits.prevented) {
            BUFFER *wb = buffer_create(1024, NULL);
            buffer_sprintf(wb,
                           "LOG FLOOD PROTECTION: resuming logging "
                           "(prevented %"PRIu32" logs in the last %"PRIu32" seconds).",
                           source->limits.prevented,
                           source->limits.throttle_period);

            if(source->pending_msg)
                freez((void *)source->pending_msg);

            source->pending_msg = strdupz(buffer_tostring(wb));
            source->pending_msgid = &log_flood_protection_msgid;
            buffer_free(wb);
        }

        // restart the period accounting
        source->limits.started_monotonic_ut = now_ut;
        source->limits.counter = 1;
        source->limits.prevented = 0;

        spinlock_unlock(&source->limits.spinlock);
        return false;
    }

    if(source->limits.counter > source->limits.logs_per_period) {
        if(!source->limits.prevented) {
            BUFFER *wb = buffer_create(1024, NULL);
            buffer_sprintf(wb,
                           "LOG FLOOD PROTECTION: too many logs (%"PRIu32" logs in %"PRId64" seconds, threshold is set to %"PRIu32" logs "
                           "in %"PRIu32" seconds). Preventing more logs from process '%s' for %"PRId64" seconds.",
                           source->limits.counter,
                           (int64_t)((now_ut - source->limits.started_monotonic_ut) / USEC_PER_SEC),
                           source->limits.logs_per_period,
                           source->limits.throttle_period,
                           program_name,
                           (int64_t)(((source->limits.started_monotonic_ut + (source->limits.throttle_period * USEC_PER_SEC) - now_ut)) / USEC_PER_SEC)
            );

            if(source->pending_msg)
                freez((void *)source->pending_msg);

            source->pending_msg = strdupz(buffer_tostring(wb));
            source->pending_msgid = &log_flood_protection_msgid;
            buffer_free(wb);
        }

        source->limits.prevented++;
        spinlock_unlock(&source->limits.spinlock);

        // prevent logging this error
#ifdef NETDATA_INTERNAL_CHECKS
        return false;
#else
        return true;
#endif
    }

    spinlock_unlock(&source->limits.spinlock);
    return false;
}
