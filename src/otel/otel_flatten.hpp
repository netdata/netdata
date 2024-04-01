#ifndef NETDATA_OTEL_FLATTEN_HPP
#define NETDATA_OTEL_FLATTEN_HPP

#include "otel_utils.hpp"

namespace pb
{

void flattenAttributes(Arena *A, const std::string &Prefix, const KeyValue &KV, RepeatedPtrField<KeyValue> &RPF);

void flattenResource(RepeatedPtrField<KeyValue> &RPF, const Resource &R);
void flattenInstrumentationScope(RepeatedPtrField<KeyValue> &RPF, const InstrumentationScope &IS);

} // namespace pb

#endif /* NETDATA_OTEL_FLATTEN_HPP */
