#ifndef OTEL_HASH_HPP
#define OTEL_HASH_HPP

#include "otel_utils.hpp"

#include "libnetdata/blake3/blake3.h"

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

} // namespace otel

#endif /* OTEL_HASH_HPP */
