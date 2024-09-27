// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ND_OTEL_FLATTEN_H
#define ND_OTEL_FLATTEN_H

#include "otel_utils.h"

namespace pb
{

void flattenAttributes(Arena *A, const std::string &Prefix, const KeyValue &KV, RepeatedPtrField<KeyValue> &RPF);

void flattenResource(RepeatedPtrField<KeyValue> &RPF, const Resource &R);
void flattenInstrumentationScope(RepeatedPtrField<KeyValue> &RPF, const InstrumentationScope &IS);

} // namespace pb

#endif /* ND_OTEL_FLATTEN_H */
