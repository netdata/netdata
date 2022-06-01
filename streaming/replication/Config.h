#ifndef REPLICATION_CONFIG_H
#define REPLICATION_CONFIG_H

#include "replication-private.h"

namespace replication {

class Config {
public:
    bool EnableReplication;

    time_t SecondsToReplicateOnFirstConnection;

    size_t MaxEntriesPerGapData;

    size_t MaxNumGapsToReplicate;

    size_t MaxQueriesPerSecond;

    bool EnableLogging;

    bool EnableFillGapLogging;

    void readReplicationConfig();
};

extern Config Cfg;

} // namespace replication

#endif /* REPLICATION_CONFIG_H */
