// SPDX-License-Identifier: GPL-3.0-or-later

#define RRDHOST_INTERNALS
#include "rrd.h"
#include "rrd-retention.h"
#include "libnetdata/parsers/duration.h"

// The expected retention is an extrapolation of the current disk usage
// (elapsed * 100 / disk_percent). On a mostly empty quota it grows without
// bound: a 20 GiB tier with a few KiB used extrapolates to centuries. Cap it,
// so the published value stays a plausible duration. The ceiling is set high
// enough not to truncate realistic long estimates - a 20 GiB tier ingesting
// ~1 GiB/year legitimately extrapolates to ~21 years.
#define MAX_EXPECTED_RETENTION_S (25 * 365 * 86400)

// Round retention time to more human-readable values (days/hours/minutes)
static time_t round_retention(time_t retention_seconds) {
    if(retention_seconds > 60 * 86400)
        retention_seconds = HOWMANY(retention_seconds, 86400) * 86400;
    else if(retention_seconds > 86400)
        retention_seconds = HOWMANY(retention_seconds, 3600) * 3600;
    else
        retention_seconds = HOWMANY(retention_seconds, 60) * 60;

    return retention_seconds;
}

// Collect retention statistics from all tiers
RRDSTATS_RETENTION rrdstats_retention_collect(void) {
    time_t now_s = now_realtime_sec();
    
    // Initialize the retention structure
    RRDSTATS_RETENTION retention = {
        .storage_tiers = 0
    };

    rrd_rdlock();

    if(!localhost) {
        rrd_rdunlock();
        return retention;
    }

    // Count the available storage tiers
    retention.storage_tiers = nd_profile.storage_tiers;

    // Iterate through all available storage tiers
    for(size_t tier = 0; tier < retention.storage_tiers; tier++) {
        STORAGE_ENGINE *eng = localhost->db[tier].eng;
        if(!eng) 
            continue;

        RRD_STORAGE_TIER *tier_info = &retention.tiers[tier];
        tier_info->tier = tier;
        tier_info->backend = eng->seb;
        tier_info->group_seconds = get_tier_grouping(tier) * localhost->rrd_update_every;
        
        // Format human-readable granularity
        duration_snprintf_time_t(tier_info->granularity_human, sizeof(tier_info->granularity_human), (time_t)tier_info->group_seconds);

        // Get metrics and samples counts
        tier_info->metrics = storage_engine_metrics(eng->seb, localhost->db[tier].si);
        tier_info->samples = storage_engine_samples(eng->seb, localhost->db[tier].si);

        // Get disk usage information
        tier_info->disk_max = storage_engine_disk_space_max(eng->seb, localhost->db[tier].si);
        tier_info->disk_used = storage_engine_disk_space_used(eng->seb, localhost->db[tier].si);
        
#ifdef ENABLE_DBENGINE
        if(!tier_info->disk_max && eng->seb == STORAGE_ENGINE_BACKEND_DBENGINE) {
            tier_info->disk_max = rrdeng_get_directory_free_bytes_space(multidb_ctx[tier]);
            tier_info->disk_max += tier_info->disk_used;
        }
#endif

        // Calculate disk usage percentage
        if(tier_info->disk_used && tier_info->disk_max)
            tier_info->disk_percent = (double)tier_info->disk_used * 100.0 / (double)tier_info->disk_max;
        else
            tier_info->disk_percent = 0.0;

        // Get retention information
        tier_info->first_time_s = storage_engine_global_first_time_s(eng->seb, localhost->db[tier].si);
        tier_info->last_time_s = now_s;
        
        // first_time_s zero means the storage engine does not know the start of
        // the retention - rrdeng_global_first_time_s() normalizes both LONG_MAX
        // (nothing loaded yet) and negatives to zero. It is not 1970: treating
        // it as a timestamp reports the whole unix epoch as retention.
        if(tier_info->first_time_s > 0 && tier_info->first_time_s < tier_info->last_time_s) {
            tier_info->retention = tier_info->last_time_s - tier_info->first_time_s;

            // Format human-readable retention
            duration_snprintf_time_t(tier_info->retention_human, sizeof(tier_info->retention_human),
                                     round_retention(tier_info->retention));

            if(tier_info->disk_used || tier_info->disk_max) {
                // Get requested retention time
                tier_info->requested_retention = 0;
#ifdef ENABLE_DBENGINE
                if(eng->seb == STORAGE_ENGINE_BACKEND_DBENGINE)
                    tier_info->requested_retention = multidb_ctx[tier]->config.max_retention_s;
#endif

                // Format human-readable requested retention
                duration_snprintf_time_t(tier_info->requested_retention_human, sizeof(tier_info->requested_retention_human),
                                         tier_info->requested_retention);

                // Calculate expected retention based on current usage.
                // Clamp in double space, BEFORE converting: a very small
                // disk_percent (a nearly empty database on a huge disk_max) can
                // push the product past time_t's range, and an out-of-range
                // floating-point to integer conversion is undefined - it can
                // itself yield a negative value that a later clamp cannot fix.
                // The clamp bound never goes below requested_retention, so a tier
                // configured for a longer retention than MAX_EXPECTED_RETENTION_S never
                // publishes an expected retention lower than the requested one.
                time_t max_retention = MAX(MAX_EXPECTED_RETENTION_S, tier_info->requested_retention);
                time_t space_retention = 0;
                if(tier_info->disk_percent > 0) {
                    double extrapolated =
                        (double)(now_s - tier_info->first_time_s) * 100.0 / tier_info->disk_percent;

                    space_retention = (extrapolated >= (double)max_retention)
                                          ? max_retention
                                          : (time_t)extrapolated;
                }

                tier_info->expected_retention = (tier_info->requested_retention && tier_info->requested_retention < space_retention)
                                              ? tier_info->requested_retention
                                              : space_retention;

                // Format human-readable expected retention
                duration_snprintf_time_t(tier_info->expected_retention_human, sizeof(tier_info->expected_retention_human),
                                         round_retention(tier_info->expected_retention));
            }
        }
        else {
            // No data yet in this tier
            tier_info->retention = 0;
            tier_info->retention_human[0] = '\0';
            tier_info->requested_retention = 0;
            tier_info->requested_retention_human[0] = '\0';
            tier_info->expected_retention = 0;
            tier_info->expected_retention_human[0] = '\0';
        }
    }

    rrd_rdunlock();
    
    return retention;
}