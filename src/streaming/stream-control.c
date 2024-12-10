// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-control.h"
#include "stream.h"
#include "replication.h"

bool stream_ml_should_be_running(void) {
    return !rrdr_backfill_running() && !replication_queries_running();
}

bool stream_children_should_be_accepted(void) {
    return !rrdr_backfill_running() && !replication_queries_running();
}

bool stream_replication_should_be_running(void) {
    return !rrdr_backfill_running();
}

bool stream_health_should_be_running(void) {
    return !rrdr_backfill_running();
}
