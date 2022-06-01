#include "replication-private.h"

using namespace replication;

Config replication::Cfg;

template <typename T>
static T clamp(const T& Value, const T& Min, const T& Max) {
    return std::max(Min, std::min(Value, Max));
}

void Config::readReplicationConfig(void) {
    const char *ConfigSectionReplication = CONFIG_SECTION_REPLICATION;

    /*
     * Enable/Disable replication
     */
    bool EnableReplication = config_get_boolean(ConfigSectionReplication, "enabled", false);

    /*
     * Backfill this many seconds on first connection of a child.
     */
    time_t SecondsToReplicateOnFirstConnection =
        config_get_number(ConfigSectionReplication, "seconds to replicate on first connection", 3600);
    SecondsToReplicateOnFirstConnection = clamp<time_t>(SecondsToReplicateOnFirstConnection, 0, 3600);

    /*
     * Send at most this amount of <timestamp, storage_number>s for a single dim.
     */
    const size_t EntriesPerPage = RRDENG_BLOCK_SIZE / sizeof(storage_number);
    size_t MaxEntriesPerGapData  =
        config_get_number(ConfigSectionReplication, "max entries for each dimension gap data", EntriesPerPage);
    MaxEntriesPerGapData = clamp<size_t>(MaxEntriesPerGapData, 128, EntriesPerPage);

    /*
     * Max number of gaps that we want parents to track for a child.
     */
    size_t MaxNumGapsToReplicate =
        config_get_number(ConfigSectionReplication, "max num gaps to replicate", 512);
    MaxNumGapsToReplicate = clamp<size_t>(MaxNumGapsToReplicate, 1, 512);

    /*
     * Max number of queries that we should perform per second
     */
    size_t MaxQueriesPerSecond =
        config_get_number(ConfigSectionReplication, "max queries per second", 256);
    MaxQueriesPerSecond = clamp<size_t>(MaxQueriesPerSecond, 64, 2048);

    /*
     * Enable logging through api/v1/replication
     */
    bool EnableLogging = config_get_boolean(ConfigSectionReplication, "log replication operations", false);

    /*
     * Enable logging of FILL_GAP command data
     */
    bool EnableFillGapLogging = config_get_boolean(ConfigSectionReplication, "log fill gap data", false);

    Cfg.EnableReplication = EnableReplication;
    Cfg.SecondsToReplicateOnFirstConnection = SecondsToReplicateOnFirstConnection;
    Cfg.MaxEntriesPerGapData = MaxEntriesPerGapData;
    Cfg.MaxNumGapsToReplicate = MaxNumGapsToReplicate;
    Cfg.MaxQueriesPerSecond = MaxQueriesPerSecond;
    Cfg.EnableLogging = EnableLogging;
    Cfg.EnableFillGapLogging = EnableFillGapLogging;
}
