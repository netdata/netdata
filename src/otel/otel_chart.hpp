#ifndef NETDATA_OTEL_CHART_HPP
#define NETDATA_OTEL_CHART_HPP

#include "otel_flatten.hpp"
#include "otel_utils.hpp"
#include "otel_config.hpp"
#include "otel_hash.hpp"
#include "otel_iterator.hpp"

#include "database/rrd.h"

#include <fstream>

const std::string &origMetricName(const pb::Metric &M);

namespace otel
{
class Chart {
public:
    Chart(const MetricConfig *MetricCfg) : MetricCfg(MetricCfg), RS(nullptr), RDs(), LastCollectionTime(0)
    {
    }

    void update(
        const pb::Metric &M,
        const std::string &Id,
        const pb::RepeatedPtrField<pb::KeyValue> &Labels)
    {
        if (!LastCollectionTime) {
            LastCollectionTime = pb::findOldestCollectionTime(M) / NSEC_PER_SEC;
            return;
        }

        if (!RS) {
            createRS(M, Id);
            setLabels(Labels);
        }

        updateRDs(M);
    }

    void setLabels(const pb::RepeatedPtrField<pb::KeyValue> &RPF)
    {
        if (RPF.empty())
            return;

        RRDLABELS *Labels = rrdlabels_create();

        for (const auto &KV : RPF) {
            const auto &K = KV.key();
            const auto &V = pb::anyValueToString(KV.value());

            rrdlabels_add(Labels, K.c_str(), V.c_str(), RRDLABEL_SRC_AUTO);
        }

        rrdset_update_rrdlabels(RS, Labels);
    }

    inline int numDimensions() const
    {
        return RDs.size();
    }

    void dump(std::ostream &OS) const
    {
        if (!RS) {
            OS << "RS: nullptr\n";
            return;
        }

        OS << "RS: id=" << rrdset_id(RS) << ", name=" << rrdset_name(RS) << ", LCS=" << LastCollectionTime << "\n";
        for (size_t Idx = 0; Idx != RDs.size(); Idx++) {
            OS << "RDs[" << Idx << "]: " << rrddim_id(RDs[Idx]) << "\n";
        }
    }

private:
    std::string findDimensionName(const pb::NumberDataPoint &DP);

    template <typename T> void createRDs(bool Monotonic, const T &DPs);
    void createRDs(const pb::Metric &M);

    void createRS(const pb::Metric &M, const std::string &BlakeId);

    void updateRDs(const pb::Metric &M);
    template <typename T> void updateRDs(const pb::Metric &M, const pb::RepeatedPtrField<T> &DPs);

private:
    const MetricConfig *MetricCfg;
    RRDSET *RS;
    std::vector<RRDDIM *> RDs;
    uint64_t LastCollectionTime;
};

} // namespace otel

#endif /* NETDATA_OTEL_CHART_HPP */
