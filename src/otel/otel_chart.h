// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ND_OTEL_CHART_H
#define ND_OTEL_CHART_H

#include "otel_flatten.h"
#include "otel_utils.h"
#include "otel_config.h"
#include "otel_hash.h"
#include "otel_iterator.h"

#include <fstream>

const std::string &origMetricName(const pb::Metric &M);

namespace otel
{
class Chart {
public:
    Chart(const MetricConfig *MetricCfg) : MetricCfg(MetricCfg)
    {
    }

    void update(const pb::Metric &M, const std::string &Id, const pb::RepeatedPtrField<pb::KeyValue> &Labels)
    {
        UNUSED(Id);
        UNUSED(Labels);

        if (!LastCollectionTime) {
            LastCollectionTime = pb::findOldestCollectionTime(M) / NSEC_PER_SEC;
            return;
        }

        if (!DefinedChart) {
            createNetdataChart(M, Id);
            setLabels(Labels);
        }

        updateRDs(M);
    }

    void setLabels(const pb::RepeatedPtrField<pb::KeyValue> &RPF)
    {
        UNUSED(RPF);

#if 0
        if (RPF.empty())
            return;

        RRDLABELS *Labels = rrdlabels_create();

        for (const auto &KV : RPF) {
            const auto &K = KV.key();
            const auto &V = pb::anyValueToString(KV.value());

            rrdlabels_add(Labels, K.c_str(), V.c_str(), RRDLABEL_SRC_AUTO);
        }

        rrdset_update_rrdlabels(RS, Labels);
#endif
    }

    inline int numDimensions() const
    {
#if 0
        return RDs.size();
#endif
        return 0;
    }

    void dump(std::ostream &OS) const
    {
#if 0
        if (!RS) {
            OS << "RS: nullptr\n";
            return;
        }

        OS << "RS: id=" << rrdset_id(RS) << ", name=" << rrdset_name(RS) << ", LCS=" << LastCollectionTime << "\n";
        for (size_t Idx = 0; Idx != RDs.size(); Idx++) {
            OS << "RDs[" << Idx << "]: " << rrddim_id(RDs[Idx]) << "\n";
        }
#endif
        UNUSED(OS);
    }

private:
    std::string findDimensionName(const pb::NumberDataPoint &DP);

    template <typename T> void createRDs(bool Monotonic, const T &DPs);
    void createRDs(const pb::Metric &M);

    void createNetdataChart(const pb::Metric &M, const std::string &BlakeId);

    void updateRDs(const pb::Metric &M);
    template <typename T> void updateRDs(const pb::Metric &M, const pb::RepeatedPtrField<T> &DPs);

private:
    const MetricConfig *MetricCfg = nullptr;
    uint64_t LastCollectionTime = 0;
    bool DefinedChart = false;
#if 0
    RRDSET *RS;
    std::vector<RRDDIM *> RDs;
#endif
};

} // namespace otel

#endif /* ND_OTEL_CHART_H */
