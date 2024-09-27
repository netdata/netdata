// SPDX-License-Identifier: GPL-3.0-or-later

#include "otel_utils.h"

#include <fstream>
#include <iomanip>

template <typename DataPoint> uint64_t pb::collectionTime(const DataPoint &DP)
{
    return DP.time_unix_nano();
}

template uint64_t pb::collectionTime<pb::NumberDataPoint>(const pb::NumberDataPoint &DP);

uint64_t pb::findOldestCollectionTime(const pb::Metric &M)
{
    uint64_t oldestTime = std::numeric_limits<uint64_t>::max();

    switch (M.data_case()) {
        case pb::Metric::kGauge:
            for (const auto &DP : M.gauge().data_points())
                oldestTime = std::min(oldestTime, collectionTime(DP));
            break;
        case pb::Metric::kSum:
            for (const auto &DP : M.sum().data_points())
                oldestTime = std::min(oldestTime, collectionTime(DP));
            break;
        case pb::Metric::kHistogram:
            for (const auto &DP : M.histogram().data_points())
                oldestTime = std::min(oldestTime, collectionTime(DP));
            break;
        case pb::Metric::kExponentialHistogram:
            for (const auto &DP : M.exponential_histogram().data_points())
                oldestTime = std::min(oldestTime, collectionTime(DP));
            break;
        case pb::Metric::kSummary:
            for (const auto &DP : M.summary().data_points())
                oldestTime = std::min(oldestTime, collectionTime(DP));
            break;
        default:
            std::abort();
    }

    return (oldestTime == std::numeric_limits<uint64_t>::max()) ? 0 : oldestTime;
}

std::string pb::anyValueToString(const pb::AnyValue &AV)
{
    switch (AV.value_case()) {
        case pb::AnyValue::kStringValue:
            return AV.string_value();
        case pb::AnyValue::kBoolValue:
            return AV.bool_value() ? "true" : "false";
        case pb::AnyValue::kIntValue:
            return std::to_string(AV.int_value());
        case pb::AnyValue::kDoubleValue:
            return std::to_string(AV.double_value());
        case pb::AnyValue::kArrayValue:
        case pb::AnyValue::kKvlistValue:
        case pb::AnyValue::kBytesValue:
            std::abort();
        default:
            std::abort();
    }
}

void pb::dumpArenaStats(const std::string &Path, const std::string &Label, const pb::Arena &A)
{
    std::ofstream OS(Path, std::ios_base::app);
    if (!OS) {
        std::cerr << "Failed to open file: " << Path << std::endl;
        return;
    }

    OS << "=== Arena Statistics " << Label << " ===" << std::endl;

    OS << "SpaceUsed: " << A.SpaceUsed() << " bytes" << std::endl;
    OS << "SpaceAllocated: " << A.SpaceAllocated() << " bytes" << std::endl;

    double UsedPct = (A.SpaceUsed() * 100.0) / A.SpaceAllocated();
    OS << std::fixed << std::setprecision(2);
    OS << "Used Percentage: " << UsedPct << "%" << std::endl;

    OS << std::endl;
    OS.close();
}
