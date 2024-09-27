// SPDX-License-Identifier: GPL-3.0-or-later

#include "otel_flatten.h"
#include "otel_utils.h"

static std::string *createPrefixKey(pb::Arena *A, const std::string &P, const std::string &K)
{
    std::string *NP = google::protobuf::Arena::Create<std::string>(A);
    if (P.empty()) {
        *NP = K;
    } else {
        NP->reserve(P.size() + 1 + K.size());
        *NP = P;
        NP->append(".");
        NP->append(K);
    }

    return NP;
}

// TODO: verify correctness
void pb::flattenAttributes(Arena *A, const std::string &Prefix, const KeyValue &KV, RepeatedPtrField<KeyValue> &RPF)
{
    std::string *NewPrefix = createPrefixKey(A, Prefix, KV.key());

    switch (KV.value().value_case()) {
        case AnyValue::kKvlistValue: {
            for (const auto &NestedKV : KV.value().kvlist_value().values())
                flattenAttributes(A, *NewPrefix, NestedKV, RPF);
            break;
        }
        case AnyValue::kArrayValue: {
            for (int Idx = 0; Idx < KV.value().array_value().values_size(); ++Idx) {
                const std::string Position = std::to_string(Idx);

                std::string *AK = Arena::Create<std::string>(A);
                AK->reserve(NewPrefix->size() + 3 + Position.size());
                *AK = *NewPrefix;
                AK->append("[");
                AK->append(Position);
                AK->append("]");

                KeyValue *FlattenedKV = RPF.Add();
                FlattenedKV->set_key(*AK);
                *FlattenedKV->mutable_value() = KV.value().array_value().values(Idx);
            }
            break;
        }
        default:
            KeyValue *FlattenedKV = RPF.Add();
            FlattenedKV->set_key(*NewPrefix);
            *FlattenedKV->mutable_value() = KV.value();
            break;
    }
}

// TODO: How should we handle dropped_attributes_count?
void pb::flattenResource(RepeatedPtrField<KeyValue> &RPF, const Resource &R)
{
    for (const auto &Attr : R.attributes())
        flattenAttributes(R.GetArena(), "r", Attr, RPF);
}

// TODO: How should we handle dropped_attributes_count?
void pb::flattenInstrumentationScope(RepeatedPtrField<KeyValue> &RPF, const InstrumentationScope &IS)
{
    KeyValue *NameKV = RPF.Add();
    NameKV->set_key("is.name");
    NameKV->mutable_value()->set_string_value(IS.name());

    KeyValue *VersionKV = RPF.Add();
    VersionKV->set_key("is.version");
    VersionKV->mutable_value()->set_string_value(IS.version());

    for (const auto &Attr : IS.attributes())
        flattenAttributes(RPF.GetArena(), "is", Attr, RPF);
}
