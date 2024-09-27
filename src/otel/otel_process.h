// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ND_OTEL_PROCESS_H
#define ND_OTEL_PROCESS_H

#include "otel_chart.h"
#include "otel_config.h"
#include "otel_hash.h"
#include "otel_iterator.h"

namespace otel
{
class ProcessorContext {
public:
    ProcessorContext(const Config *Cfg) : Cfg(Cfg)
    {
    }

    inline const Config *config() const
    {
        return Cfg;
    }

    inline std::unordered_map<std::string, Chart> &charts()
    {
        return Charts;
    }

private:
    const Config *Cfg;
    std::unordered_map<std::string, Chart> Charts;
};

class MetricsDataProcessor : public Processor {
public:
    MetricsDataProcessor(ProcessorContext &Ctx) : Ctx(Ctx)
    {
    }

    void onResourceMetrics(const pb::ResourceMetrics &RMs) override;
    void onScopeMetrics(const pb::ResourceMetrics &RMs, const pb::ScopeMetrics &SMs) override;
    void onMetric(const pb::ResourceMetrics &RMs, const pb::ScopeMetrics &SMs, const pb::Metric &M) override;

    virtual ~MetricsDataProcessor()
    {
    }

private:
    ProcessorContext &Ctx;

    ResourceMetricsHasher RMH;
    ScopeMetricsHasher SMH;
    MetricHasher MH;

    const ScopeConfig *ScopeCfg = nullptr;

    pb::RepeatedPtrField<pb::KeyValue> Labels;
};

} // namespace otel

#endif /* ND_OTEL_PROCESS_H */
