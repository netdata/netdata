// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ND_OTEL_HASH_H
#define ND_OTEL_HASH_H

#include "otel_utils.h"

#include "libnetdata/blake3/blake3.h"

using BlakeId = std::array<uint8_t, BLAKE3_OUT_LEN / 2>;

namespace otel
{

class ScopeMetricsHasher;
class MetricHasher;

class ResourceMetricsHasher {
    friend void digestAttributes(const pb::RepeatedPtrField<pb::KeyValue> &KVs);

public:
    ScopeMetricsHasher hash(const pb::ResourceMetrics &RMs);
};

class ScopeMetricsHasher {
    friend void digestAttributes(const pb::RepeatedPtrField<pb::KeyValue> &KVs);

public:
    ScopeMetricsHasher() = default;

    ScopeMetricsHasher(blake3_hasher &BH)
    {
        this->BH = BH;
    }

    MetricHasher hash(const pb::ScopeMetrics &SMs);

private:
    blake3_hasher BH;
};

class MetricHasher {
    friend void digestAttributes(const pb::RepeatedPtrField<pb::KeyValue> &KVs);

public:
    MetricHasher() = default;

    MetricHasher(blake3_hasher &BH)
    {
        this->BH = BH;
    }

    const std::string &hash(const pb::Metric &M);

private:
    blake3_hasher BH;
    std::string MetricId;
};

void hashAnyValue(blake3_hasher &H, const pb::AnyValue &AV);
void hashKeyValue(blake3_hasher &H, const pb::KeyValue &KV);
void hashKeyValues(blake3_hasher &H, const pb::RepeatedPtrField<pb::KeyValue> &KVs);
void hashInstrumentationScope(blake3_hasher &H, const pb::InstrumentationScope &IS);
void hashResource(blake3_hasher &H, const pb::Resource &R);
void hashMetric(blake3_hasher &H, const pb::Metric &M);

} // namespace otel

#endif /* ND_OTEL_HASH_H */
