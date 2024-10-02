// SPDX-License-Identifier: GPL-3.0-or-later

#include "otel_config.h"
#include "otel_utils.h"
#include "otel_transform.h"

template <typename T> static std::string createGroupKey(const std::vector<std::string> *InstanceAttributes, const T &DP)
{
    std::string Key;

    for (const auto &IA : *InstanceAttributes) {
        for (const auto &Attr : DP.attributes()) {
            if (Attr.key() == IA) {
                Key += Attr.value().string_value() + "_";
                break;
            }
        }
    }

    // Remove trailing underscore
    if (!Key.empty()) {
        Key.pop_back();
    }

    return Key;
}

template <typename T>
static std::unordered_map<std::string, std::vector<T> >
groupDataPoints(const std::vector<std::string> *InstanceAttributes, const pb::RepeatedPtrField<T> &DPs)
{
    std::unordered_map<std::string, std::vector<T> > Groups;

    for (const auto &DP : DPs) {
        std::string GroupKey = createGroupKey(InstanceAttributes, DP);
        Groups[GroupKey].push_back(DP);
    }

    return Groups;
}

template <typename T, typename F>
static pb::RepeatedPtrField<pb::Metric> createNewMetrics(
    const pb::Metric &OrigMetric,
    const std::unordered_map<std::string, std::vector<T> > &GDPs,
    F setDataPoints)
{
    pb::RepeatedPtrField<pb::Metric> NewMetrics;

    for (const auto &P : GDPs) {
        pb::Metric *NewMetric = NewMetrics.Add();
        NewMetric->set_name(OrigMetric.name() + "_" + P.first);
        NewMetric->set_description(OrigMetric.description());
        NewMetric->set_unit(OrigMetric.unit());

        auto *KV = NewMetric->add_metadata();
        KV->set_key("_nd_orig_metric_name");
        KV->mutable_value()->set_string_value(OrigMetric.name());

        setDataPoints(*NewMetric, P.second);
    }

    return NewMetrics;
}

static pb::RepeatedPtrField<pb::Metric> restructureGauge(const otel::MetricConfig *CfgMetric, const pb::Metric &M)
{
    auto GDPs = groupDataPoints(CfgMetric->getInstanceAttributes(), M.gauge().data_points());
    return createNewMetrics(M, GDPs, [&](pb::Metric &NewMetric, const auto &DPs) {
        auto *G = NewMetric.mutable_gauge();
        *G->mutable_data_points() = {DPs.begin(), DPs.end()};
    });
}

static pb::RepeatedPtrField<pb::Metric> restructureSum(const otel::MetricConfig *CfgMetric, const pb::Metric &M)
{
    auto GDPs = groupDataPoints(CfgMetric->getInstanceAttributes(), M.sum().data_points());
    return createNewMetrics(M, GDPs, [&](pb::Metric &NewMetric, const auto &DPs) {
        auto *S = NewMetric.mutable_sum();
        *S->mutable_data_points() = {DPs.begin(), DPs.end()};
        S->set_aggregation_temporality(M.sum().aggregation_temporality());
        S->set_is_monotonic(M.sum().is_monotonic());
    });
}

static void transformMetrics(const otel::ScopeConfig *ScopeCfg, pb::RepeatedPtrField<pb::Metric> *RPF)
{
    if (!ScopeCfg)
        return;

    pb::RepeatedPtrField<pb::Metric> *RestructuredMetrics =
        pb::Arena::Create<pb::RepeatedPtrField<pb::Metric> >(RPF->GetArena());

    for (const auto &M : *RPF) {
        auto *MetricCfg = ScopeCfg->getMetric(M.name());
        if (!MetricCfg || MetricCfg->getInstanceAttributes()->empty()) {
            *RestructuredMetrics->Add() = M;
            continue;
        }

        if (M.has_gauge()) {
            auto NewMetrics = restructureGauge(MetricCfg, M);

            for (const auto &NewM : NewMetrics)
                *RestructuredMetrics->Add() = NewM;
        } else if (M.has_sum()) {
            auto NewMetrics = restructureSum(MetricCfg, M);

            for (const auto &NewM : NewMetrics)
                *RestructuredMetrics->Add() = NewM;
        } else {
            std::abort();
        }
    }

    RPF->Clear();
    RPF->Swap(RestructuredMetrics);
}

void pb::transformResourceMetrics(const otel::Config *Cfg, pb::RepeatedPtrField<pb::ResourceMetrics> &RPF)
{
    for (auto &RMs : RPF) {
        for (auto &SMs : *RMs.mutable_scope_metrics()) {
            if (SMs.has_scope()) {
                const auto *ScopeCfg = Cfg->getScope(SMs.scope().name());
                transformMetrics(ScopeCfg, SMs.mutable_metrics());
            }
        }
    }
}
