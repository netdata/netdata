// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-control.h"
#include "stream.h"
#include "stream-replication-sender.h"

static struct {
    PAD64(uint32_t) backfill_runners;
    PAD64(uint32_t) replication_runners;
    PAD64(uint32_t) user_data_queries_runners;
    PAD64(uint32_t) user_weights_queries_runners;
} sc;

// --------------------------------------------------------------------------------------------------------------------
// backfilling

ALWAYS_INLINE static uint32_t backfill_runners(void) {
    return __atomic_load_n(&sc.backfill_runners, __ATOMIC_RELAXED);
}

ALWAYS_INLINE void stream_control_backfill_query_started(void) {
    __atomic_add_fetch(&sc.backfill_runners, 1, __ATOMIC_RELAXED);
}

ALWAYS_INLINE void stream_control_backfill_query_finished(void) {
    __atomic_sub_fetch(&sc.backfill_runners, 1, __ATOMIC_RELAXED);
}

// --------------------------------------------------------------------------------------------------------------------
// replication

ALWAYS_INLINE static uint32_t replication_runners(void) {
    return __atomic_load_n(&sc.replication_runners, __ATOMIC_RELAXED);
}

ALWAYS_INLINE void stream_control_replication_query_started(void) {
    __atomic_add_fetch(&sc.replication_runners, 1, __ATOMIC_RELAXED);
}

ALWAYS_INLINE void stream_control_replication_query_finished(void) {
    __atomic_sub_fetch(&sc.replication_runners, 1, __ATOMIC_RELAXED);
}

// --------------------------------------------------------------------------------------------------------------------
// user data queries

ALWAYS_INLINE static uint32_t user_data_query_runners(void) {
    return __atomic_load_n(&sc.user_data_queries_runners, __ATOMIC_RELAXED);
}

ALWAYS_INLINE void stream_control_user_data_query_started(void) {
    __atomic_add_fetch(&sc.user_data_queries_runners, 1, __ATOMIC_RELAXED);
}

ALWAYS_INLINE void stream_control_user_data_query_finished(void) {
    __atomic_sub_fetch(&sc.user_data_queries_runners, 1, __ATOMIC_RELAXED);
}

// --------------------------------------------------------------------------------------------------------------------
// user weights queries

ALWAYS_INLINE static uint32_t user_weights_query_runners(void) {
    return __atomic_load_n(&sc.user_weights_queries_runners, __ATOMIC_RELAXED);
}

ALWAYS_INLINE void stream_control_user_weights_query_started(void) {
    __atomic_add_fetch(&sc.user_weights_queries_runners, 1, __ATOMIC_RELAXED);
}

ALWAYS_INLINE void stream_control_user_weights_query_finished(void) {
    __atomic_sub_fetch(&sc.user_weights_queries_runners, 1, __ATOMIC_RELAXED);
}

// --------------------------------------------------------------------------------------------------------------------
// consumer API

ALWAYS_INLINE bool stream_control_ml_should_be_running(void) {
    return backfill_runners() == 0 &&
           replication_runners() == 0 &&
           user_data_query_runners() == 0 &&
           user_weights_query_runners() == 0;
}

ALWAYS_INLINE bool stream_control_children_should_be_accepted(void) {
    // we should not check for replication here.
    // replication benefits from multiple nodes (merges the extents)
    // and also the nodes should be close in time in the db
    // - checking for replication leaves the last few nodes locked-out (since all the others are replicating)

    return backfill_runners() == 0;
}

ALWAYS_INLINE bool stream_control_replication_should_be_running(void) {
    return backfill_runners() == 0 &&
           user_data_query_runners() == 0 &&
           user_weights_query_runners() == 0;
}

ALWAYS_INLINE bool stream_control_health_should_be_running(void) {
    return backfill_runners() == 0 &&
           // replication_runners() == 0 &&
           (user_data_query_runners() + user_weights_query_runners()) <= 1;
}
