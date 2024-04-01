#include "otel_hash.hpp"

using otel::MetricHasher;
using otel::ScopeMetricsHasher;

void digestAttributes(blake3_hasher &BH, const pb::RepeatedPtrField<pb::KeyValue> &KVs)
{
    for (const auto &Attr : KVs) {
        blake3_hasher_update(&BH, Attr.key().data(), Attr.key().size());

        std::string Value = pb::anyValueToString(Attr.value());
        blake3_hasher_update(&BH, Value.data(), Value.size());
    }
}

ScopeMetricsHasher otel::ResourceMetricsHasher::hash(const pb::ResourceMetrics &RMs)
{
    blake3_hasher BH;

    blake3_hasher_init(&BH);
    blake3_hasher_update(&BH, RMs.schema_url().data(), RMs.schema_url().size());

    if (RMs.has_resource())
        digestAttributes(BH, RMs.resource().attributes());

    return ScopeMetricsHasher(BH);
}

MetricHasher otel::ScopeMetricsHasher::hash(const pb::ScopeMetrics &SMs)
{
    blake3_hasher TmpBH = BH;

    blake3_hasher_update(&TmpBH, SMs.schema_url().data(), SMs.schema_url().size());
    blake3_hasher_update(&TmpBH, SMs.scope().name().data(), SMs.scope().name().size());
    blake3_hasher_update(&TmpBH, SMs.scope().version().data(), SMs.scope().version().size());

    digestAttributes(TmpBH, SMs.scope().attributes());

    return MetricHasher(TmpBH);
}

const std::string &otel::MetricHasher::hash(const pb::Metric &M)
{
    blake3_hasher TmpBH = BH;

    blake3_hasher_update(&TmpBH, M.name().data(), M.name().size());
    blake3_hasher_update(&TmpBH, M.description().data(), M.description().size());
    blake3_hasher_update(&TmpBH, M.unit().data(), M.unit().size());

    digestAttributes(TmpBH, M.metadata());

#if 0
    switch (M.data_case()) {
        case pb::Metric::kGauge: {
            const auto &G = M.gauge();
            for (const auto &DP : G.data_points())
                digestAttributes(TmpBH, DP.attributes());
            break;
        }
        case pb::Metric::kSum: {
            const auto &S = M.sum();
            for (const auto &DP : S.data_points())
                digestAttributes(TmpBH, DP.attributes());
            break;
        }
        default:
            std::abort();
            break;
    }
#endif

    uint8_t Output[BLAKE3_OUT_LEN];
    blake3_hasher_finalize(&TmpBH, Output, BLAKE3_OUT_LEN);

    MetricId.clear();
    MetricId += M.name();
    MetricId += "-";

    for (int Idx = 0; Idx < BLAKE3_OUT_LEN; Idx++) {
        char HexValue[3];
        std::snprintf(HexValue, sizeof(HexValue), "%02x", static_cast<unsigned int>(Output[Idx]));
        MetricId.append(HexValue);
    }

    return MetricId;
}
