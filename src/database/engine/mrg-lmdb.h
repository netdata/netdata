// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MRG_LMDB_H
#define NETDATA_MRG_LMDB_H

#include "rrdengine.h"

// Reference timestamp for time calculations (Jan 1, 2010 00:00:00 UTC)
#define METRIC_LMDB_TIME_BASE 1262304000

// Structure to hold information about datafiles for validating MRG dumps
typedef struct mrg_datafile_info {
    char filename[RRDENG_PATH_MAX];
    uint64_t file_size;
    time_t last_modified;
} MRG_DATAFILE_INFO;

// Structure for the LMDB key (just UUID as tier is implicit from DB location)
typedef struct metric_lmdb_key {
    ND_UUID uuid;     // UUID from uuidmap
} METRIC_LMDB_KEY;

// Structure for the LMDB value (metric metadata)
typedef struct metric_lmdb_value {
    uint32_t update_every;  // Collection frequency in seconds
    uint32_t first_time_s;  // First timestamp, seconds since METRIC_LMDB_TIME_BASE
    uint32_t last_time_s;   // Last timestamp, seconds since METRIC_LMDB_TIME_BASE
} METRIC_LMDB_VALUE;

// Initialize LMDB environment for a specific tier
int mrg_lmdb_init(const char *path);

// Save MRG to LMDB for a specific section
int mrg_lmdb_save(Word_t section, const char *path);

// Load MRG from LMDB for a specific section
// Returns:
//  0: Success, loaded all metrics
//  1: Loaded successfully but there are new datafiles to process
// -1: Failed to load (incompatible or missing files)
int mrg_lmdb_load(Word_t section, const char *path);

// Close LMDB environment and clean up resources
void mrg_lmdb_close(const char *path);

#endif //NETDATA_MRG_LMDB_H
