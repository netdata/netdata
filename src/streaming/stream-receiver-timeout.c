// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-receiver-internals.h"

static uint32_t stream_receiver_timeout_valid_update_every(int64_t update_every_s) {
    return update_every_s > 0 && update_every_s <= STREAM_RECEIVER_UPDATE_EVERY_MAX_SECONDS ?
        (uint32_t)update_every_s : 0;
}

static void stream_receiver_timeout_publish(STREAM_RECEIVER_TIMEOUT *timeout) {
    uint64_t update_every_s = timeout->minimum_update_every_s;
    if(!update_every_s)
        update_every_s = MAX(timeout->learned_update_every_s, timeout->provisional_update_every_s);

    uint64_t effective_timeout_s = MAX(STREAM_RECEIVER_IDLE_TIMEOUT_MIN_SECONDS, update_every_s * 2);
    __atomic_store_n(&timeout->effective_timeout_s, effective_timeout_s, __ATOMIC_RELEASE);
}

static void stream_receiver_timeout_interval_add(STREAM_RECEIVER_TIMEOUT *timeout, uint32_t update_every_s) {
    Pvoid_t *value = JudyLIns(&timeout->intervals, (Word_t)update_every_s, PJE0);
    if(unlikely(!value || value == PJERR))
        fatal("STREAM RCV: cannot maintain the receiver update interval index");

    Word_t *count = (Word_t *)value;
    if(unlikely(*count == ~(Word_t)0))
        fatal("STREAM RCV: receiver update interval chart count overflow");

    (*count)++;

    if(!timeout->minimum_update_every_s || update_every_s < timeout->minimum_update_every_s)
        timeout->minimum_update_every_s = update_every_s;

    timeout->learned_update_every_s = timeout->minimum_update_every_s;
}

static void stream_receiver_timeout_interval_remove(
    STREAM_RECEIVER_TIMEOUT *timeout,
    uint32_t update_every_s,
    bool update_learned_minimum) {
    Pvoid_t *value = JudyLGet(timeout->intervals, (Word_t)update_every_s, PJE0);
    if(unlikely(!value || value == PJERR || !*(Word_t *)value)) {
        internal_error(true, "STREAM RCV: receiver update interval index is inconsistent");
        return;
    }

    Word_t *count = (Word_t *)value;
    if(--(*count) == 0) {
        if(unlikely(JudyLDel(&timeout->intervals, (Word_t)update_every_s, PJE0) != 1)) {
            internal_error(true, "STREAM RCV: cannot remove an empty receiver update interval");
            return;
        }

        if(update_every_s == timeout->minimum_update_every_s) {
            Word_t first_interval = 0;
            Pvoid_t *first = JudyLFirst(timeout->intervals, &first_interval, PJE0);
            timeout->minimum_update_every_s = first && first != PJERR ? (uint32_t)first_interval : 0;
        }

        if(update_learned_minimum && timeout->minimum_update_every_s)
            timeout->learned_update_every_s = timeout->minimum_update_every_s;
    }
}

void stream_receiver_timeout_init(STREAM_RECEIVER_TIMEOUT *timeout) {
    *timeout = (STREAM_RECEIVER_TIMEOUT) { 0 };
    spinlock_init(&timeout->spinlock);
    stream_receiver_timeout_publish(timeout);
}

void stream_receiver_timeout_connection_start(STREAM_RECEIVER_TIMEOUT *timeout, int64_t handshake_update_every_s) {
    spinlock_lock(&timeout->spinlock);

    JudyLFreeArray(&timeout->intervals, PJE0);
    timeout->minimum_update_every_s = 0;
    timeout->provisional_update_every_s = stream_receiver_timeout_valid_update_every(handshake_update_every_s);

    if(unlikely(++timeout->generation == 0))
        timeout->generation = 1;

    stream_receiver_timeout_publish(timeout);
    spinlock_unlock(&timeout->spinlock);
}

void stream_receiver_timeout_destroy(STREAM_RECEIVER_TIMEOUT *timeout) {
    spinlock_lock(&timeout->spinlock);
    JudyLFreeArray(&timeout->intervals, PJE0);
    spinlock_unlock(&timeout->spinlock);
    *timeout = (STREAM_RECEIVER_TIMEOUT) { 0 };
}

void stream_receiver_timeout_chart_track(
    STREAM_RECEIVER_TIMEOUT *timeout,
    STREAM_RECEIVER_TIMEOUT_CHART *chart,
    int64_t update_every_s,
    bool active) {

    uint32_t wanted_update_every_s = active ? stream_receiver_timeout_valid_update_every(update_every_s) : 0;
    spinlock_lock(&timeout->spinlock);

    uint32_t old_update_every_s =
        chart->generation == timeout->generation ? chart->update_every_s : 0;

    if(old_update_every_s == wanted_update_every_s) {
        spinlock_unlock(&timeout->spinlock);
        return;
    }

    if(old_update_every_s)
        stream_receiver_timeout_interval_remove(timeout, old_update_every_s, true);

    *chart = (STREAM_RECEIVER_TIMEOUT_CHART) {
        .generation = timeout->generation,
    };

    if(wanted_update_every_s) {
        stream_receiver_timeout_interval_add(timeout, wanted_update_every_s);
        chart->update_every_s = wanted_update_every_s;
    }

    stream_receiver_timeout_publish(timeout);
    spinlock_unlock(&timeout->spinlock);
}

void stream_receiver_timeout_chart_forget(
    STREAM_RECEIVER_TIMEOUT *timeout,
    STREAM_RECEIVER_TIMEOUT_CHART *chart) {

    spinlock_lock(&timeout->spinlock);
    if(chart->generation == timeout->generation && chart->update_every_s)
        stream_receiver_timeout_interval_remove(timeout, chart->update_every_s, false);

    *chart = (STREAM_RECEIVER_TIMEOUT_CHART) { 0 };
    stream_receiver_timeout_publish(timeout);
    spinlock_unlock(&timeout->spinlock);
}

uint64_t stream_receiver_timeout_seconds(const STREAM_RECEIVER_TIMEOUT *timeout) {
    return __atomic_load_n(&timeout->effective_timeout_s, __ATOMIC_ACQUIRE);
}

usec_t stream_receiver_timeout_usec(const STREAM_RECEIVER_TIMEOUT *timeout) {
    uint64_t timeout_s = stream_receiver_timeout_seconds(timeout);
    if(timeout_s > UINT64_MAX / USEC_PER_SEC)
        return UINT64_MAX;

    return (usec_t)timeout_s * USEC_PER_SEC;
}

bool stream_receiver_timeout_expired(const STREAM_RECEIVER_TIMEOUT *timeout, usec_t idle_ut) {
    return idle_ut > stream_receiver_timeout_usec(timeout);
}

void stream_receiver_timeout_chart_refresh(struct receiver_state *rpt, RRDSET *st) {
    stream_receiver_timeout_chart_track(
        &rpt->host->stream.rcv.timeout,
        &st->stream.rcv.timeout,
        st->update_every,
        !rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE));
}

static bool stream_receiver_timeout_unittest_check(
    const char *name,
    const STREAM_RECEIVER_TIMEOUT *timeout,
    uint64_t expected_s) {

    uint64_t actual_s = stream_receiver_timeout_seconds(timeout);
    if(actual_s != expected_s) {
        fprintf(stderr, "%s: timeout is %" PRIu64 " seconds, expected %" PRIu64 "\n", name, actual_s, expected_s);
        return false;
    }

    usec_t expected_ut = expected_s > UINT64_MAX / USEC_PER_SEC ? UINT64_MAX : expected_s * USEC_PER_SEC;
    usec_t actual_ut = stream_receiver_timeout_usec(timeout);
    if(actual_ut != expected_ut) {
        fprintf(stderr, "%s: timeout is %" PRIu64 " usec, expected %" PRIu64 "\n", name, actual_ut, expected_ut);
        return false;
    }

    if(stream_receiver_timeout_expired(timeout, expected_ut)) {
        fprintf(stderr, "%s: timeout expired at the exact boundary\n", name);
        return false;
    }

    if(expected_ut < UINT64_MAX && !stream_receiver_timeout_expired(timeout, expected_ut + 1)) {
        fprintf(stderr, "%s: timeout did not expire after the boundary\n", name);
        return false;
    }

    return true;
}

static bool stream_receiver_timeout_unittest_cleanup_order(bool fast_first) {
    STREAM_RECEIVER_TIMEOUT timeout;
    STREAM_RECEIVER_TIMEOUT_CHART fast = { 0 }, daily = { 0 };
    stream_receiver_timeout_init(&timeout);
    stream_receiver_timeout_connection_start(&timeout, 0);
    stream_receiver_timeout_chart_track(&timeout, &fast, 1, true);
    stream_receiver_timeout_chart_track(&timeout, &daily, 86400, true);

    if(fast_first) {
        stream_receiver_timeout_chart_forget(&timeout, &fast);
        stream_receiver_timeout_chart_forget(&timeout, &daily);
    }
    else {
        stream_receiver_timeout_chart_forget(&timeout, &daily);
        stream_receiver_timeout_chart_forget(&timeout, &fast);
    }

    stream_receiver_timeout_connection_start(&timeout, 0);
    bool ok = stream_receiver_timeout_unittest_check(
        fast_first ? "cleanup retains learned minimum in ascending order" :
                     "cleanup retains learned minimum in descending order",
        &timeout,
        600);
    stream_receiver_timeout_destroy(&timeout);
    return ok;
}

int stream_receiver_timeout_unittest(void) {
    STREAM_RECEIVER_TIMEOUT timeout;
    STREAM_RECEIVER_TIMEOUT_CHART chart_a = { 0 }, chart_b = { 0 }, chart_c = { 0 };

    stream_receiver_timeout_init(&timeout);

    if(!stream_receiver_timeout_unittest_cleanup_order(true) ||
       !stream_receiver_timeout_unittest_cleanup_order(false))
        return 1;

    stream_receiver_timeout_connection_start(&timeout, 0);
    if(!stream_receiver_timeout_unittest_check("no learned or provisional interval", &timeout, 600))
        return 1;

    stream_receiver_timeout_connection_start(&timeout, 301);
    if(!stream_receiver_timeout_unittest_check("provisional interval above floor", &timeout, 602))
        return 1;

    stream_receiver_timeout_chart_track(&timeout, &chart_a, 86400, true);
    if(!stream_receiver_timeout_unittest_check("daily chart learned", &timeout, 172800))
        return 1;

    stream_receiver_timeout_chart_track(&timeout, &chart_b, 300, true);
    stream_receiver_timeout_chart_track(&timeout, &chart_c, 300, true);
    if(!stream_receiver_timeout_unittest_check("duplicate minima", &timeout, 600))
        return 1;

    stream_receiver_timeout_chart_track(&timeout, &chart_b, 600, true);
    if(!stream_receiver_timeout_unittest_check("one duplicate minimum remains", &timeout, 600))
        return 1;

    stream_receiver_timeout_chart_track(&timeout, &chart_c, 1200, true);
    if(!stream_receiver_timeout_unittest_check("minimum increases after redefine", &timeout, 1200))
        return 1;

    stream_receiver_timeout_chart_track(&timeout, &chart_b, 0, true);
    if(!stream_receiver_timeout_unittest_check("invalid interval removes chart", &timeout, 2400))
        return 1;

    stream_receiver_timeout_chart_track(&timeout, &chart_c, 1200, false);
    if(!stream_receiver_timeout_unittest_check("obsolete chart removed", &timeout, 172800))
        return 1;

    stream_receiver_timeout_chart_forget(&timeout, &chart_a);
    if(!stream_receiver_timeout_unittest_check("learned cadence survives chart deletion", &timeout, 172800))
        return 1;

    stream_receiver_timeout_connection_start(&timeout, 1);
    if(!stream_receiver_timeout_unittest_check("daily cadence survives reconnect before metadata", &timeout, 172800))
        return 1;

    stream_receiver_timeout_chart_track(&timeout, &chart_a, 360, true);
    if(!stream_receiver_timeout_unittest_check("new connection rebuilds cadence", &timeout, 720))
        return 1;

    stream_receiver_timeout_connection_start(&timeout, 86400);
    if(!stream_receiver_timeout_unittest_check("slower provisional cadence is safe during rebuild", &timeout, 172800))
        return 1;

    stream_receiver_timeout_chart_track(&timeout, &chart_a, 600, true);
    if(!stream_receiver_timeout_unittest_check("received chart replaces rebuild fallback", &timeout, 1200))
        return 1;

    stream_receiver_timeout_connection_start(&timeout, 86401);
    if(!stream_receiver_timeout_unittest_check("invalid provisional interval preserves learned cadence", &timeout, 1200))
        return 1;

    stream_receiver_timeout_destroy(&timeout);
    fprintf(stderr, "STREAM RECEIVER TIMEOUT TESTS PASSED\n");
    return 0;
}
