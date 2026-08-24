// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-receiver-internals.h"

static uint32_t stream_receiver_cadence_valid_update_every(int64_t update_every_s) {
    return update_every_s > 0 && update_every_s <= STREAM_RECEIVER_UPDATE_EVERY_MAX_SECONDS ?
        (uint32_t)update_every_s : 0;
}

static void stream_receiver_cadence_publish(STREAM_RECEIVER_CADENCE *cadence) {
    uint64_t selected_s = cadence->minimum_update_every_s;
    if(!selected_s)
        selected_s = MAX(cadence->learned_update_every_s, cadence->provisional_update_every_s);

    uint64_t application_timeout_s = MAX(STREAM_RECEIVER_IDLE_TIMEOUT_MIN_SECONDS, selected_s * 2);
    uint64_t keepalive_idle_s = selected_s ? (selected_s + 1) / 2 : STREAM_RECEIVER_KEEPALIVE_IDLE_MIN_SECONDS;
    keepalive_idle_s = MAX(keepalive_idle_s, STREAM_RECEIVER_KEEPALIVE_IDLE_MIN_SECONDS);
    keepalive_idle_s = MIN(keepalive_idle_s, STREAM_RECEIVER_KEEPALIVE_IDLE_MAX_SECONDS);

    __atomic_store_n(&cadence->automatic_keepalive_idle_s, (uint32_t)keepalive_idle_s, __ATOMIC_RELEASE);
    __atomic_store_n(&cadence->application_timeout_s, application_timeout_s, __ATOMIC_RELEASE);
}

static void stream_receiver_cadence_interval_add(STREAM_RECEIVER_CADENCE *cadence, uint32_t update_every_s) {
    Pvoid_t *raw_count = JudyLIns(&cadence->intervals, (Word_t)update_every_s, PJE0);
    if(raw_count == (Pvoid_t *)PJERR) {
        fatal("STREAM RCV: cannot maintain the receiver update interval index");
        return;
    }
    if(!raw_count) {
        fatal("STREAM RCV: cannot allocate the receiver update interval index");
        return;
    }
    Word_t *count = (Word_t *)raw_count;

    if(unlikely(*count == ~(Word_t)0))
        fatal("STREAM RCV: receiver update interval chart count overflow");

    (*count)++;

    if(!cadence->minimum_update_every_s || update_every_s < cadence->minimum_update_every_s)
        cadence->minimum_update_every_s = update_every_s;

    cadence->learned_update_every_s = cadence->minimum_update_every_s;
}

static void stream_receiver_cadence_interval_remove(
    STREAM_RECEIVER_CADENCE *cadence,
    uint32_t update_every_s,
    bool update_learned_minimum) {

    Pvoid_t *raw_count = JudyLGet(cadence->intervals, (Word_t)update_every_s, PJE0);
    if(raw_count == (Pvoid_t *)PJERR) {
        internal_error(true, "STREAM RCV: receiver update interval lookup failed");
        return;
    }
    if(!raw_count) {
        internal_error(true, "STREAM RCV: receiver update interval index is inconsistent");
        return;
    }
    Word_t *count = (Word_t *)raw_count;
    if(!*count) {
        internal_error(true, "STREAM RCV: receiver update interval index contains an empty count");
        return;
    }

    if(--(*count) == 0) {
        if(unlikely(JudyLDel(&cadence->intervals, (Word_t)update_every_s, PJE0) != 1)) {
            internal_error(true, "STREAM RCV: cannot remove an empty receiver update interval");
            return;
        }

        // A Judy traversal is needed only when the last chart at the cached minimum disappears.
        if(update_every_s == cadence->minimum_update_every_s) {
            Word_t first_interval = 0;
            Pvoid_t *first = JudyLFirst(cadence->intervals, &first_interval, PJE0);
            cadence->minimum_update_every_s = first && first != PJERR ? (uint32_t)first_interval : 0;
        }

        if(update_learned_minimum && cadence->minimum_update_every_s)
            cadence->learned_update_every_s = cadence->minimum_update_every_s;
    }
}

void stream_receiver_cadence_init(STREAM_RECEIVER_CADENCE *cadence) {
    *cadence = (STREAM_RECEIVER_CADENCE) { 0 };
    spinlock_init(&cadence->spinlock);
    stream_receiver_cadence_publish(cadence);
}

void stream_receiver_cadence_connection_start(STREAM_RECEIVER_CADENCE *cadence, int64_t handshake_update_every_s) {
    spinlock_lock(&cadence->spinlock);

    JudyLFreeArray(&cadence->intervals, PJE0);
    cadence->minimum_update_every_s = 0;
    cadence->provisional_update_every_s = stream_receiver_cadence_valid_update_every(handshake_update_every_s);

    if(unlikely(++cadence->generation == 0))
        cadence->generation = 1;

    stream_receiver_cadence_publish(cadence);
    spinlock_unlock(&cadence->spinlock);
}

void stream_receiver_cadence_destroy(STREAM_RECEIVER_CADENCE *cadence) {
    spinlock_lock(&cadence->spinlock);
    JudyLFreeArray(&cadence->intervals, PJE0);
    spinlock_unlock(&cadence->spinlock);
    *cadence = (STREAM_RECEIVER_CADENCE) { 0 };
}

void stream_receiver_cadence_chart_track(
    STREAM_RECEIVER_CADENCE *cadence,
    STREAM_RECEIVER_CADENCE_CHART *chart,
    int64_t update_every_s,
    bool active) {

    uint32_t wanted_update_every_s = active ? stream_receiver_cadence_valid_update_every(update_every_s) : 0;
    spinlock_lock(&cadence->spinlock);

    uint32_t old_update_every_s = chart->generation == cadence->generation ? chart->update_every_s : 0;
    if(old_update_every_s == wanted_update_every_s) {
        spinlock_unlock(&cadence->spinlock);
        return;
    }

    if(old_update_every_s)
        stream_receiver_cadence_interval_remove(cadence, old_update_every_s, true);

    *chart = (STREAM_RECEIVER_CADENCE_CHART) {
        .generation = cadence->generation,
    };

    if(wanted_update_every_s) {
        stream_receiver_cadence_interval_add(cadence, wanted_update_every_s);
        chart->update_every_s = wanted_update_every_s;
    }

    stream_receiver_cadence_publish(cadence);
    spinlock_unlock(&cadence->spinlock);
}

void stream_receiver_cadence_chart_forget(
    STREAM_RECEIVER_CADENCE *cadence,
    STREAM_RECEIVER_CADENCE_CHART *chart) {

    spinlock_lock(&cadence->spinlock);
    if(chart->generation == cadence->generation && chart->update_every_s)
        stream_receiver_cadence_interval_remove(cadence, chart->update_every_s, false);

    *chart = (STREAM_RECEIVER_CADENCE_CHART) { 0 };
    stream_receiver_cadence_publish(cadence);
    spinlock_unlock(&cadence->spinlock);
}

uint64_t stream_receiver_cadence_application_timeout_seconds(const STREAM_RECEIVER_CADENCE *cadence) {
    return __atomic_load_n(&cadence->application_timeout_s, __ATOMIC_ACQUIRE);
}

usec_t stream_receiver_cadence_application_timeout_usec(const STREAM_RECEIVER_CADENCE *cadence) {
    uint64_t timeout_s = stream_receiver_cadence_application_timeout_seconds(cadence);
    if(timeout_s > UINT64_MAX / USEC_PER_SEC)
        return UINT64_MAX;

    return (usec_t)timeout_s * USEC_PER_SEC;
}

bool stream_receiver_cadence_application_timeout_expired(const STREAM_RECEIVER_CADENCE *cadence, usec_t idle_ut) {
    return idle_ut > stream_receiver_cadence_application_timeout_usec(cadence);
}

uint32_t stream_receiver_cadence_automatic_keepalive_idle_seconds(const STREAM_RECEIVER_CADENCE *cadence) {
    return __atomic_load_n(&cadence->automatic_keepalive_idle_s, __ATOMIC_ACQUIRE);
}

void stream_receiver_cadence_chart_refresh(struct receiver_state *rpt, RRDSET *st) {
    stream_receiver_cadence_chart_track(
        &rpt->host->stream.rcv.cadence,
        &st->stream.rcv.cadence,
        st->update_every,
        !rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE));
}

static bool stream_receiver_cadence_unittest_check(
    const char *name,
    const STREAM_RECEIVER_CADENCE *cadence,
    uint64_t expected_timeout_s,
    uint32_t expected_keepalive_s) {

    uint64_t actual_timeout_s = stream_receiver_cadence_application_timeout_seconds(cadence);
    uint32_t actual_keepalive_s = stream_receiver_cadence_automatic_keepalive_idle_seconds(cadence);
    if(actual_timeout_s != expected_timeout_s || actual_keepalive_s != expected_keepalive_s) {
        fprintf(stderr,
                "%s: timeout/keepalive are %" PRIu64 "/%u seconds, expected %" PRIu64 "/%u\n",
                name, actual_timeout_s, actual_keepalive_s, expected_timeout_s, expected_keepalive_s);
        return false;
    }

    usec_t expected_ut = expected_timeout_s > UINT64_MAX / USEC_PER_SEC ?
        UINT64_MAX : expected_timeout_s * USEC_PER_SEC;
    if(stream_receiver_cadence_application_timeout_usec(cadence) != expected_ut ||
       stream_receiver_cadence_application_timeout_expired(cadence, expected_ut) ||
       (expected_ut < UINT64_MAX && !stream_receiver_cadence_application_timeout_expired(cadence, expected_ut + 1))) {
        fprintf(stderr, "%s: application timeout boundary failed\n", name);
        return false;
    }

    return true;
}

static bool stream_receiver_cadence_unittest_cleanup_order(bool fast_first) {
    STREAM_RECEIVER_CADENCE cadence;
    STREAM_RECEIVER_CADENCE_CHART fast = { 0 }, daily = { 0 };
    stream_receiver_cadence_init(&cadence);
    stream_receiver_cadence_connection_start(&cadence, 0);
    stream_receiver_cadence_chart_track(&cadence, &fast, 1, true);
    stream_receiver_cadence_chart_track(&cadence, &daily, 86400, true);

    if(fast_first) {
        stream_receiver_cadence_chart_forget(&cadence, &fast);
        stream_receiver_cadence_chart_forget(&cadence, &daily);
    }
    else {
        stream_receiver_cadence_chart_forget(&cadence, &daily);
        stream_receiver_cadence_chart_forget(&cadence, &fast);
    }

    stream_receiver_cadence_connection_start(&cadence, 0);
    bool ok = stream_receiver_cadence_unittest_check(
        fast_first ? "cleanup retains learned minimum in ascending order" :
                     "cleanup retains learned minimum in descending order",
        &cadence, 600, 30);
    stream_receiver_cadence_destroy(&cadence);
    return ok;
}

static bool stream_receiver_cadence_unittest_chart_lifecycle(void) {
    STREAM_RECEIVER_CADENCE cadence;
    STREAM_RECEIVER_CADENCE_CHART fast = { 0 }, slow = { 0 };
    bool ok = true;

    stream_receiver_cadence_init(&cadence);
    stream_receiver_cadence_connection_start(&cadence, 0);
    stream_receiver_cadence_chart_track(&cadence, &slow, 600, true);
    stream_receiver_cadence_chart_track(&cadence, &fast, 120, true);
    ok &= stream_receiver_cadence_unittest_check("lower-period insertion", &cadence, 600, 60);

    stream_receiver_cadence_chart_track(&cadence, &slow, 1200, true);
    ok &= stream_receiver_cadence_unittest_check("non-minimum redefine", &cadence, 600, 60);

    stream_receiver_cadence_chart_track(&cadence, &slow, 1200, false);
    ok &= stream_receiver_cadence_unittest_check("non-minimum obsoletion", &cadence, 600, 60);

    stream_receiver_cadence_chart_track(&cadence, &slow, 600, true);
    ok &= stream_receiver_cadence_unittest_check("non-minimum reactivation", &cadence, 600, 60);

    stream_receiver_cadence_chart_track(&cadence, &fast, 120, false);
    ok &= stream_receiver_cadence_unittest_check("minimum obsoletion", &cadence, 1200, 300);

    stream_receiver_cadence_chart_track(&cadence, &fast, 120, true);
    ok &= stream_receiver_cadence_unittest_check("minimum reactivation", &cadence, 600, 60);

    stream_receiver_cadence_chart_forget(&cadence, &slow);
    ok &= stream_receiver_cadence_unittest_check("non-minimum deletion", &cadence, 600, 60);

    stream_receiver_cadence_chart_forget(&cadence, &fast);
    ok &= stream_receiver_cadence_unittest_check("last-chart deletion", &cadence, 600, 60);

    stream_receiver_cadence_destroy(&cadence);
    return ok;
}

int stream_receiver_cadence_unittest(void) {
    STREAM_RECEIVER_CADENCE cadence;
    STREAM_RECEIVER_CADENCE_CHART chart_a = { 0 }, chart_b = { 0 }, chart_c = { 0 };
    int errors = 0;

    stream_receiver_cadence_init(&cadence);
    if(!stream_receiver_cadence_unittest_cleanup_order(true) ||
       !stream_receiver_cadence_unittest_cleanup_order(false) ||
       !stream_receiver_cadence_unittest_chart_lifecycle())
        errors++;

    static const struct {
        int64_t update_every_s;
        uint64_t timeout_s;
        uint32_t keepalive_s;
    } boundaries[] = {
        { 0, 600, 30 }, { 1, 600, 30 }, { 60, 600, 30 }, { 120, 600, 60 },
        { 299, 600, 150 }, { 300, 600, 150 }, { 301, 602, 151 }, { 360, 720, 180 },
        { 600, 1200, 300 }, { 86400, 172800, 3600 }, { 86401, 600, 30 },
        { INT32_MAX, 600, 30 },
    };
    for(size_t i = 0; i < _countof(boundaries); i++) {
        stream_receiver_cadence_connection_start(&cadence, boundaries[i].update_every_s);
        if(!stream_receiver_cadence_unittest_check(
               "provisional cadence boundary", &cadence, boundaries[i].timeout_s, boundaries[i].keepalive_s))
            errors++;
    }

    stream_receiver_cadence_connection_start(&cadence, 301);
    stream_receiver_cadence_chart_track(&cadence, &chart_a, 86400, true);
    if(!stream_receiver_cadence_unittest_check("daily chart learned", &cadence, 172800, 3600)) errors++;

    stream_receiver_cadence_chart_track(&cadence, &chart_b, 300, true);
    stream_receiver_cadence_chart_track(&cadence, &chart_c, 300, true);
    if(!stream_receiver_cadence_unittest_check("duplicate minima", &cadence, 600, 150)) errors++;

    stream_receiver_cadence_chart_track(&cadence, &chart_b, 600, true);
    if(!stream_receiver_cadence_unittest_check("one duplicate minimum remains", &cadence, 600, 150)) errors++;

    stream_receiver_cadence_chart_track(&cadence, &chart_c, 1200, true);
    if(!stream_receiver_cadence_unittest_check("minimum increases after redefine", &cadence, 1200, 300)) errors++;

    stream_receiver_cadence_chart_track(&cadence, &chart_b, 0, true);
    if(!stream_receiver_cadence_unittest_check("invalid interval removes chart", &cadence, 2400, 600)) errors++;

    stream_receiver_cadence_chart_track(&cadence, &chart_c, 1200, false);
    if(!stream_receiver_cadence_unittest_check("obsolete chart removed", &cadence, 172800, 3600)) errors++;

    stream_receiver_cadence_chart_forget(&cadence, &chart_a);
    if(!stream_receiver_cadence_unittest_check("learned cadence survives deletion", &cadence, 172800, 3600)) errors++;

    stream_receiver_cadence_connection_start(&cadence, 1);
    if(!stream_receiver_cadence_unittest_check("learned cadence survives reconnect", &cadence, 172800, 3600)) errors++;

    stream_receiver_cadence_chart_track(&cadence, &chart_a, 360, true);
    if(!stream_receiver_cadence_unittest_check("current chart replaces fallback", &cadence, 720, 180)) errors++;

    stream_receiver_cadence_connection_start(&cadence, 86400);
    if(!stream_receiver_cadence_unittest_check("slower provisional cadence is safe", &cadence, 172800, 3600)) errors++;

    stream_receiver_cadence_chart_track(&cadence, &chart_a, 600, true);
    if(!stream_receiver_cadence_unittest_check("received chart replaces rebuild fallback", &cadence, 1200, 300)) errors++;

    stream_receiver_cadence_connection_start(&cadence, 86401);
    if(!stream_receiver_cadence_unittest_check("invalid provisional preserves learned cadence", &cadence, 1200, 300)) errors++;

    stream_receiver_cadence_destroy(&cadence);
    if(errors)
        fprintf(stderr, "STREAM RECEIVER CADENCE TESTS FAILED: %d error(s)\n", errors);
    else
        fprintf(stderr, "STREAM RECEIVER CADENCE TESTS PASSED\n");
    return errors;
}
