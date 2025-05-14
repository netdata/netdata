// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRD_RETENTION_H
#define NETDATA_RRD_RETENTION_H

#include "libnetdata/libnetdata.h"
#include "storage-engine.h"

// Maximum number of storage tiers the system supports
#define RRD_MAX_STORAGE_TIERS 32

// Structure to hold information about each storage tier
typedef struct rrd_storage_tier {
    size_t tier;                      // Tier number
    STORAGE_ENGINE_BACKEND backend;   // Storage engine backend (RRDDIM or DBENGINE)
    size_t group_seconds;             // Granularity in seconds
    char granularity_human[32];       // Human-readable granularity string

    size_t metrics;                   // Number of metrics in this tier
    size_t samples;                   // Number of samples in this tier

    uint64_t disk_used;               // Disk space used in bytes
    uint64_t disk_max;                // Maximum available disk space in bytes
    double disk_percent;              // Disk usage percentage (0.0-100.0)

    time_t first_time_s;              // Oldest timestamp in this tier
    time_t last_time_s;               // Most recent timestamp in this tier
    time_t retention;                 // Current retention in seconds (last_time_s - first_time_s)
    char retention_human[32];         // Human-readable current retention

    time_t requested_retention;       // Configured maximum retention in seconds
    char requested_retention_human[32]; // Human-readable configured retention

    time_t expected_retention;        // Expected retention based on current usage
    char expected_retention_human[32]; // Human-readable expected retention
} RRD_STORAGE_TIER;

// Main structure to hold retention information across all tiers
typedef struct rrdstats_retention {
    size_t storage_tiers;                             // Number of available storage tiers
    RRD_STORAGE_TIER tiers[RRD_MAX_STORAGE_TIERS];    // Array of tier information
} RRDSTATS_RETENTION;

// Function to collect retention statistics
RRDSTATS_RETENTION rrdstats_retention_collect(void);

#endif // NETDATA_RRD_RETENTION_H