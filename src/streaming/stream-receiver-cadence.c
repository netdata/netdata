// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-receiver-internals.h"

static uint32_t stream_receiver_cadence_valid_update_every(int64_t update_every_s) {
    return update_every_s > 0 && update_every_s <= STREAM_RECEIVER_UPDATE_EVERY_MAX_SECONDS ?
        (uint32_t)update_every_s : 0;
}

static uint32_t stream_receiver_cadence_selected_update_every(const STREAM_RECEIVER_CADENCE *cadence) {
    uint32_t minimum_update_every_s =
        __atomic_load_n(&cadence->minimum_update_every_s, __ATOMIC_ACQUIRE);

    if(minimum_update_every_s != UINT32_MAX)
        return minimum_update_every_s;

    return __atomic_load_n(&cadence->provisional_update_every_s, __ATOMIC_RELAXED);
}

void stream_receiver_cadence_init(STREAM_RECEIVER_CADENCE *cadence) {
    __atomic_store_n(&cadence->provisional_update_every_s, 0, __ATOMIC_RELAXED);
    __atomic_store_n(&cadence->minimum_update_every_s, UINT32_MAX, __ATOMIC_RELEASE);
}

void stream_receiver_cadence_connection_start(STREAM_RECEIVER_CADENCE *cadence, int64_t handshake_update_every_s) {
    // An acquire load of the reset marker must also see its provisional handshake value.
    __atomic_store_n(
        &cadence->provisional_update_every_s,
        stream_receiver_cadence_valid_update_every(handshake_update_every_s),
        __ATOMIC_RELAXED);
    __atomic_store_n(&cadence->minimum_update_every_s, UINT32_MAX, __ATOMIC_RELEASE);
}

void stream_receiver_cadence_observe(STREAM_RECEIVER_CADENCE *cadence, int64_t update_every_s) {
    uint32_t wanted_update_every_s = stream_receiver_cadence_valid_update_every(update_every_s);
    if(!wanted_update_every_s)
        return;

    uint32_t minimum_update_every_s =
        __atomic_load_n(&cadence->minimum_update_every_s, __ATOMIC_RELAXED);
    while(wanted_update_every_s < minimum_update_every_s &&
          !__atomic_compare_exchange_n(
              &cadence->minimum_update_every_s,
              &minimum_update_every_s,
              wanted_update_every_s,
              true,
              __ATOMIC_RELEASE,
              __ATOMIC_RELAXED)) {
        ;
    }
}

uint64_t stream_receiver_cadence_application_timeout_seconds(const STREAM_RECEIVER_CADENCE *cadence) {
    uint64_t selected_s = stream_receiver_cadence_selected_update_every(cadence);
    return MAX(STREAM_RECEIVER_IDLE_TIMEOUT_MIN_SECONDS, selected_s * 2);
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
    uint64_t selected_s = stream_receiver_cadence_selected_update_every(cadence);
    uint64_t keepalive_idle_s = selected_s ? (selected_s + 1) / 2 : STREAM_RECEIVER_KEEPALIVE_IDLE_MIN_SECONDS;
    keepalive_idle_s = MAX(keepalive_idle_s, STREAM_RECEIVER_KEEPALIVE_IDLE_MIN_SECONDS);
    keepalive_idle_s = MIN(keepalive_idle_s, STREAM_RECEIVER_KEEPALIVE_IDLE_MAX_SECONDS);
    return (uint32_t)keepalive_idle_s;
}

void stream_receiver_cadence_observe_chart(struct receiver_state *rpt, RRDSET *st) {
    if(!rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE))
        stream_receiver_cadence_observe(&rpt->host->stream.rcv.cadence, st->update_every);
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

int stream_receiver_cadence_unittest(void) {
    STREAM_RECEIVER_CADENCE cadence;
    int errors = 0;

    stream_receiver_cadence_init(&cadence);
    if(!stream_receiver_cadence_unittest_check("initial fallback", &cadence, 600, 30))
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

    stream_receiver_cadence_connection_start(&cadence, 5);
    stream_receiver_cadence_observe(&cadence, 600);
    if(!stream_receiver_cadence_unittest_check("first chart replaces faster handshake", &cadence, 1200, 300)) errors++;

    stream_receiver_cadence_observe(&cadence, 1200);
    if(!stream_receiver_cadence_unittest_check("connection minimum is monotonic", &cadence, 1200, 300)) errors++;

    stream_receiver_cadence_observe(&cadence, 300);
    if(!stream_receiver_cadence_unittest_check("lower chart changes cadence", &cadence, 600, 150)) errors++;

    stream_receiver_cadence_observe(&cadence, 0);
    stream_receiver_cadence_observe(&cadence, 86401);
    if(!stream_receiver_cadence_unittest_check("invalid charts are ignored", &cadence, 600, 150)) errors++;

    stream_receiver_cadence_connection_start(&cadence, 600);
    if(!stream_receiver_cadence_unittest_check("reconnect resets a faster minimum", &cadence, 1200, 300)) errors++;

    stream_receiver_cadence_observe(&cadence, 0);
    if(!stream_receiver_cadence_unittest_check("invalid first chart preserves handshake", &cadence, 1200, 300)) errors++;

    stream_receiver_cadence_observe(&cadence, 600);

    stream_receiver_cadence_connection_start(&cadence, 5);
    stream_receiver_cadence_observe(&cadence, 5);
    if(!stream_receiver_cadence_unittest_check("600-to-5 transition", &cadence, 600, 30)) errors++;

    stream_receiver_cadence_connection_start(&cadence, 600);
    stream_receiver_cadence_observe(&cadence, 600);
    if(!stream_receiver_cadence_unittest_check("5-to-600 transition", &cadence, 1200, 300)) errors++;

    stream_receiver_cadence_connection_start(&cadence, 600);
    stream_receiver_cadence_observe(&cadence, 5);
    if(!stream_receiver_cadence_unittest_check("first chart lowers handshake", &cadence, 600, 30)) errors++;

    stream_receiver_cadence_connection_start(&cadence, 1);
    stream_receiver_cadence_observe(&cadence, 86400);
    if(!stream_receiver_cadence_unittest_check("first daily chart replaces handshake", &cadence, 172800, 3600)) errors++;

    stream_receiver_cadence_connection_start(&cadence, 86401);
    if(!stream_receiver_cadence_unittest_check("invalid handshake uses fallback", &cadence, 600, 30)) errors++;

    if(errors)
        fprintf(stderr, "STREAM RECEIVER CADENCE TESTS FAILED: %d error(s)\n", errors);
    else
        fprintf(stderr, "STREAM RECEIVER CADENCE TESTS PASSED\n");
    return errors;
}
