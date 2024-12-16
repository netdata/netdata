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
#include "pulse-db-rrd.h"
#include "pulse-string.h"
#include "pulse-heartbeat.h"
#include "pulse-dictionary.h"
#include "pulse-workers.h"
#include "pulse-trace-allocations.h"
#include "pulse-aral.h"

void *pulse_thread_main(void *ptr);
void *pulse_thread_sqlite3_main(void *ptr);

#endif /* NETDATA_PULSE_H */
