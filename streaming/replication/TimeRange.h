#ifndef REPLICATION_TIMERANGE_H
#define REPLICATION_TIMERANGE_H

#include "replication-private.h"

namespace replication {

using TimeRange = std::pair<time_t, time_t>;

inline std::ostream& operator<<(std::ostream &OS, const TimeRange &TR) {
    OS << "[" << TR.first << ", " << TR.second << "]";
    return OS;
}

inline std::ostream& operator<<(std::ostream &OS, const std::vector<TimeRange> &TRs) {
    for (const TimeRange &TR : TRs) {
        OS << TR << ", ";
    }
    return OS;
}

std::vector<TimeRange> splitTimeRange(const TimeRange &TR, size_t Epoch);
std::vector<TimeRange> coalesceTimeRanges(std::vector<TimeRange> &TRs);

std::string serializeTimeRangesToString(const std::vector<TimeRange> &TRs);
std::vector<TimeRange> deserializeTimeRangesFromArray(const char *Buf, size_t Len);

} // namespace replication

#endif /* REPLICATION_TIMERANGE_H */
