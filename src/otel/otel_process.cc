#include "otel_process.hpp"

void otel::MetricsDataProcessor::onResourceMetrics(const pb::ResourceMetrics &RMs)
{
    SMH = RMH.hash(RMs);

    Labels.Clear();
    if (RMs.has_resource()) {
        pb::flattenResource(Labels, RMs.resource());
    }
}

void otel::MetricsDataProcessor::onScopeMetrics(const pb::ResourceMetrics &RMs, const pb::ScopeMetrics &SMs)
{
    UNUSED(RMs);

    MH = SMH.hash(SMs);

    if (SMs.has_scope()) {
        ScopeCfg = Ctx.config()->getScope(SMs.scope().name());
    } else {
        ScopeCfg = nullptr;
    }
}

void otel::MetricsDataProcessor::onMetric(
    const pb::ResourceMetrics &RMs,
    const pb::ScopeMetrics &SMs,
    const pb::Metric &M)
{
    UNUSED(RMs);
    UNUSED(SMs);

    const std::string &Id = MH.hash(M);

    auto &Charts = Ctx.charts();

    auto It = Charts.find(Id);
    if (It == Charts.end()) {
        const MetricConfig *MetricCfg = ScopeCfg ? ScopeCfg->getMetric(origMetricName(M)) : nullptr;
        It = Charts.emplace(Id, Chart(MetricCfg)).first;
    }

    It->second.update(M, Id, Labels);
}
