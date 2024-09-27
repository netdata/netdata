// SPDX-License-Identifier: GPL-3.0-or-later

#include "otel_chart.h"
#include "otel_utils.h"

const std::string &origMetricName(const pb::Metric &M)
{
    for (const auto &Attr : M.metadata()) {
        if (Attr.key() == "_nd_orig_metric_name") {
            return Attr.value().string_value();
        }
    }

    return M.name();
}

std::string otel::Chart::findDimensionName(const pb::NumberDataPoint &DP)
{
    if (MetricCfg) {
        const std::string *DimensionsAttribute = MetricCfg->getDimensionsAttribute();
        if (DimensionsAttribute) {
            for (const auto &Attr : DP.attributes()) {
                if (Attr.key() == *DimensionsAttribute) {
                    return Attr.value().string_value();
                }
            }
        }
    }

    return "value";
}

template <typename T> void otel::Chart::createRDs(bool Monotonic, const T &DPs)
{
    UNUSED(Monotonic);
    UNUSED(DPs);

#if 0
    RDs.clear();
    RDs.reserve(DPs.size());

    for (const auto &DP : DPs) {
        std::string Name = findDimensionName(DP);

        auto Algorithm = Monotonic ? RRD_ALGORITHM_INCREMENTAL : RRD_ALGORITHM_ABSOLUTE;
        RRDDIM *RD = rrddim_add(RS, Name.c_str(), nullptr, 1, 1000, Algorithm);

        RDs.push_back(RD);
    }
#endif
}

void otel::Chart::createRDs(const pb::Metric &M)
{
    if (M.has_gauge()) {
        createRDs(false, M.gauge().data_points());
    } else if (M.has_sum()) {
        createRDs(M.sum().is_monotonic(), M.sum().data_points());
    } else {
        std::abort();
    }
}

void otel::Chart::createRS(const pb::Metric &M, const std::string &Id)
{
    UNUSED(M);
    UNUSED(Id);

#if 0
    uint64_t UpdateEvery = (pb::findOldestCollectionTime(M) / NSEC_PER_SEC) - LastCollectionTime;
    if (UpdateEvery == 0) {
        fatal("[GVD] WTF!? alfkjalkrjwoi");
    }

    const std::string &OrigMetricName = origMetricName(M);
    const std::string ContextName = "otel." + OrigMetricName;

    RS = rrdset_create_localhost(
        "otel",                  // type
        Id.c_str(),              // id
        Id.c_str(),              // name
        ContextName.c_str(),     // family
        ContextName.c_str(),     // context
        M.description().c_str(), // title
        M.unit().c_str(),        // units
        "otel.plugin",           // plugin
        "otel.module",           // module
        666666,                  // priority
        UpdateEvery,             // update_every
        RRDSET_TYPE_LINE         // chart_type
    );

    createRDs(M);
#endif
}

void otel::Chart::updateRDs(const pb::Metric &M)
{
    if (M.has_gauge()) {
        updateRDs(M, M.gauge().data_points());
    } else if (M.has_sum()) {
        updateRDs(M, M.sum().data_points());
    } else {
        std::abort();
    }
}

template <typename T> void otel::Chart::updateRDs(const pb::Metric &M, const pb::RepeatedPtrField<T> &DPs)
{
    UNUSED(M);
    UNUSED(DPs);

    #if 0
    if (DPs.size() != static_cast<int>(RDs.size())) {
        // NOTE: We rely on the fact the RRDIM dictionary does not overwrite items.
        createRDs(M);
    }

    for (int Idx = 0; Idx != DPs.size(); Idx++) {
        const auto &DP = DPs[Idx];
        RRDDIM *RD = RDs[Idx];

        collected_number Value = 0;
        if (DP.value_case() == pb::NumberDataPoint::kAsDouble) {
            Value = DP.as_double() * 1000;
        } else if (DP.value_case() == pb::NumberDataPoint::kAsInt) {
            Value = DP.as_int() * 1000;
        } else {
            std::abort();
        }

        struct timeval PIT;
        PIT.tv_sec = pb::collectionTime(DP) / NSEC_PER_SEC;
        PIT.tv_usec = 0;

        rrddim_timed_set_by_pointer(RS, RD, PIT, Value);
    }

    rrdset_done(RS);
    #endif
}
