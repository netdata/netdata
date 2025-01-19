// SPDX-License-Identifier: GPL-3.0-or-later

#include "pulse-db-dbengine-retention.h"
#ifdef ENABLE_DBENGINE
#include "database/engine/rrdengineapi.h"

void dbengine_retention_statistics(bool extended __maybe_unused) {

    static bool init = false;
    static DBENGINE_TIER_STATS stats[RRD_STORAGE_TIERS];

    if (!localhost)
        return;

    rrdeng_calculate_tier_disk_space_percentage();

    for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
        STORAGE_ENGINE *eng = localhost->db[tier].eng;
        if (!eng || eng->seb != STORAGE_ENGINE_BACKEND_DBENGINE)
            continue;

        if (init == false) {
            char id[200];
            snprintfz(id, sizeof(id) - 1, "dbengine_retention_tier%zu", tier);
            stats[tier].st = rrdset_create_localhost(
                "netdata",
                id,
                NULL,
                "dbengine retention",
                "netdata.dbengine_tier_retention",
                "dbengine space and time retention",
                "%",
                "netdata",
                "stats",
                134900, // before "dbengine memory" (dbengine2_statistics_charts)
                10,
                RRDSET_TYPE_LINE);

            stats[tier].rd_space = rrddim_add(stats[tier].st, "space", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            stats[tier].rd_time = rrddim_add(stats[tier].st, "time", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            char tier_str[5];
            snprintfz(tier_str, 4, "%zu", tier);
            rrdlabels_add(stats[tier].st->rrdlabels, "tier", tier_str, RRDLABEL_SRC_AUTO);

            rrdset_flag_set(stats[tier].st, RRDSET_FLAG_METADATA_UPDATE);
            rrdhost_flag_set(stats[tier].st->rrdhost, RRDHOST_FLAG_METADATA_UPDATE);
            rrdset_metadata_updated(stats[tier].st);
        }

        time_t first_time_s = storage_engine_global_first_time_s(eng->seb, localhost->db[tier].si);
        time_t retention = first_time_s ? now_realtime_sec() - first_time_s : 0;

        //
        // Note: storage_engine_disk_space_used is the exact diskspace (as reported by api/v2/node_instances
        //       get_used_disk_space is used to determine if database cleanup (file rotation should happen)
        //                           and adds to the disk space used the desired file size of the active
        //                           datafile
        uint64_t disk_space = rrdeng_get_used_disk_space(multidb_ctx[tier]);
        //uint64_t disk_space = storage_engine_disk_space_used(eng->seb, localhost->db[tier].si);

        uint64_t config_disk_space = storage_engine_disk_space_max(eng->seb, localhost->db[tier].si);
        if (!config_disk_space) {
            config_disk_space = rrdeng_get_directory_free_bytes_space(multidb_ctx[tier]);
            config_disk_space += disk_space;
        }

        collected_number disk_percentage = (collected_number) (config_disk_space ? 100 * disk_space / config_disk_space : 0);

        collected_number retention_percentage = (collected_number)multidb_ctx[tier]->config.max_retention_s ?
                                                    100 * retention / multidb_ctx[tier]->config.max_retention_s :
                                                    0;

        if (retention_percentage > 100)
            retention_percentage = 100;

        rrddim_set_by_pointer(stats[tier].st, stats[tier].rd_space, (collected_number) disk_percentage);
        rrddim_set_by_pointer(stats[tier].st, stats[tier].rd_time, (collected_number) retention_percentage);

        rrdset_done(stats[tier].st);
    }
    init = true;
}
#endif
