// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TELEMETRY_H
#define NETDATA_TELEMETRY_H 1

#include "database/rrd.h"

extern bool telemetry_enabled;
extern bool telemetry_extended_enabled;

#include "telemetry-http-api.h"
#include "telemetry-queries.h"
#include "telemetry-ingestion.h"
#include "telemetry-ml.h"
#include "telemetry-gorilla.h"
#include "telemetry-daemon.h"
#include "telemetry-daemon-memory.h"
#include "telemetry-sqlite3.h"
#include "telemetry-dbengine.h"
#include "telemetry-string.h"
#include "telemetry-heartbeat.h"
#include "telemetry-dictionary.h"
#include "telemetry-workers.h"
#include "telemetry-trace-allocations.h"

void *telemetry_thread_main(void *ptr);
void *telemetry_thread_sqlite3_main(void *ptr);

#endif /* NETDATA_TELEMETRY_H */
