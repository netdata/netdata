// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_H
#define NETDATA_PULSE_H 1

#include "database/rrd.h"

extern bool pulse_enabled;
extern bool pulse_extended_enabled;

#include "pulse-http-api.h"
#include "pulse-queries.h"
#include "pulse-ingestion.h"
#include "pulse-ml.h"
#include "pulse-gorilla.h"
#include "pulse-daemon.h"
#include "pulse-daemon-memory.h"
#include "pulse-sqlite3.h"
#include "pulse-db-dbengine.h"
#include "pulse-db-dbengine-retention.h"
#include "pulse-db-rrd.h"
#include "pulse-string.h"
#include "pulse-heartbeat.h"
#include "pulse-dictionary.h"
#include "pulse-workers.h"
#include "pulse-trace-allocations.h"
#include "pulse-aral.h"
#include "pulse-network.h"
#include "pulse-parents.h"

void pulse_thread_main(void *ptr);
void pulse_thread_sqlite3_main(void *ptr);
void pulse_thread_workers_main(void *ptr);
void pulse_thread_memory_extended_main(void *ptr);

#define p1_add_fetch(variable, value) __atomic_add_fetch(variable, value, __ATOMIC_RELAXED)
#define p1_sub_fetch(variable, value) __atomic_sub_fetch(variable, value, __ATOMIC_RELAXED)

#define p1_fetch_add(variable, value) __atomic_fetch_add(variable, value, __ATOMIC_RELAXED)
#define p1_fetch_sub(variable, value) __atomic_fetch_sub(variable, value, __ATOMIC_RELAXED)

#define p1_store(variable, value) __atomic_store_n(variable, value, __ATOMIC_RELAXED)
#define p1_load(variable) __atomic_load_n(variable, value, __ATOMIC_RELAXED)

#define p2_add_fetch(variable, value) __atomic_add_fetch(variable, value, __ATOMIC_RELAXED)
#define p2_sub_fetch(variable, value) __atomic_sub_fetch(variable, value, __ATOMIC_RELAXED)

#define p2_fetch_add(variable, value) __atomic_fetch_add(variable, value, __ATOMIC_RELAXED)
#define p2_fetch_sub(variable, value) __atomic_fetch_sub(variable, value, __ATOMIC_RELAXED)

#define p2_store(variable, value) __atomic_store_n(variable, value, __ATOMIC_RELAXED)
#define p2_load(variable) __atomic_load_n(variable, value, __ATOMIC_RELAXED)

#endif /* NETDATA_PULSE_H */
